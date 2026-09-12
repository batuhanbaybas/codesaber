package backend

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"aide/backend/acp"
	"aide/backend/agentstore"
)

// fakePrompter is a controllable acpPrompter: Prompt blocks until released
// (if gated) and records calls.
type fakePrompter struct {
	mu      sync.Mutex
	prompts []string
	closed  bool
	block   chan struct{}
	onUpd   func(acp.SessionUpdate)
}

func (f *fakePrompter) Prompt(ctx context.Context, text string) (acp.PromptResponse, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, text)
	f.mu.Unlock()
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return acp.PromptResponse{}, ctx.Err()
		}
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (f *fakePrompter) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakePrompter) SetOnUpdate(fn func(acp.SessionUpdate)) {
	f.mu.Lock()
	f.onUpd = fn
	f.mu.Unlock()
}

func (f *fakePrompter) emit(u acp.SessionUpdate) {
	f.mu.Lock()
	fn := f.onUpd
	f.mu.Unlock()
	if fn != nil {
		fn(u)
	}
}

// installFakeAgent swaps acpStartSession for a fake returning fp; restores on
// test cleanup.
func installFakeAgent(t *testing.T, fp *fakePrompter) {
	t.Helper()
	prev := acpStartSession
	acpStartSession = func(ctx context.Context, root string, profile acp.Info, handlers acp.ClientHandlers) (acpPrompter, error) {
		return fp, nil
	}
	t.Cleanup(func() { acpStartSession = prev })
}

func newAgentTestApp(t *testing.T) (*App, *fakeSink, string) {
	t.Helper()
	app, sink := newTestApp(t)
	root := t.TempDir()
	if _, err := app.OpenProject(root); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	pid := app.reg.List()[0].ID
	return app, sink, pid
}

func TestACPStartEmitsTranscriptAndState(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	installFakeAgent(t, &fakePrompter{})

	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}
	tr := sink.waitFor(t, EventACPTranscript, 2*time.Second)
	m := tr.payload.(map[string]any)
	if m["projectId"] != pid {
		t.Errorf("transcript projectId = %v, want %v", m["projectId"], pid)
	}
	if _, ok := m["entries"].([]agentstore.Entry); !ok {
		t.Errorf("transcript entries type = %T, want []agentstore.Entry", m["entries"])
	}
	st := sink.waitFor(t, EventACPState, 2*time.Second)
	sm := st.payload.(map[string]any)
	if sm["state"] != AgentStateIdle {
		t.Errorf("state = %v, want idle", sm["state"])
	}
}

func TestACPStartUnknownHarness(t *testing.T) {
	app, _, pid := newAgentTestApp(t)
	if err := app.ACPStart(pid, "does-not-exist"); err == nil {
		t.Fatal("want error for unknown harness")
	}
}

func TestACPSendPromptStreamsAndPersists(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	fp := &fakePrompter{}
	installFakeAgent(t, fp)

	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}
	sink.snapshot() // drain start events

	fp.block = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- app.ACPSendPrompt(pid, "hello") }()
	time.Sleep(50 * time.Millisecond)
	fp.emit(acp.SessionUpdate{
		SessionUpdate: acp.UpdateAgentMessageChunk,
		Content:       &acp.ContentBlock{Type: acp.BlockTypeText, Text: "hi "},
	})
	fp.emit(acp.SessionUpdate{
		SessionUpdate: acp.UpdateAgentMessageChunk,
		Content:       &acp.ContentBlock{Type: acp.BlockTypeText, Text: "there"},
	})
	fp.emit(acp.SessionUpdate{
		SessionUpdate: acp.UpdateToolCall,
		ToolCallID:    "t-1",
		Title:         "read file",
		Kind:          acp.ToolKindRead,
		Status:        acp.ToolStatusInProgress,
	})
	close(fp.block) // let the prompt turn complete
	<-done

	var msgs, tools int
	var lastUser, chunks string
	var toolPayload map[string]any
	for _, ev := range sink.snapshot() {
		switch ev.name {
		case EventACPMsg:
			msgs++
			m := ev.payload.(map[string]any)
			if m["role"] == "user" && m["text"] == "hello" {
				lastUser = "ok"
			}
			if m["role"] == "agent" && m["kind"] == "chunk" {
				chunks += m["text"].(string)
			}
		case EventACPTool:
			tools++
			toolPayload = ev.payload.(map[string]any)
		case EventACPState:
			m := ev.payload.(map[string]any)
			if m["state"] != AgentStateThinking && m["state"] != AgentStateIdle {
				t.Errorf("unexpected state %v", m["state"])
			}
		}
	}
	if msgs != 3 || lastUser != "ok" {
		t.Errorf("msgs=%d lastUser=%q, want 3 user-ok", msgs, lastUser)
	}
	if chunks != "hi there" {
		t.Errorf("chunks = %q, want %q", chunks, "hi there")
	}
	if tools != 1 || toolPayload["toolCallId"] != "t-1" || toolPayload["status"] != acp.ToolStatusInProgress {
		t.Errorf("tool payload = %#v, want t-1 in_progress", toolPayload)
	}

	// transcript persisted: user entry + assembled agent reply
	entries, err := app.ACPLoadTranscript(pid)
	if err != nil {
		t.Fatalf("LoadTranscript: %v", err)
	}
	var roles []string
	for _, e := range entries {
		roles = append(roles, e.Role)
	}
	if len(roles) != 3 || roles[0] != "user" || roles[2] != "agent" {
		t.Fatalf("transcript roles = %#v, want [user agent agent]", roles)
	}
	if entries[2].Text != "hi there" {
		t.Errorf("agent entry text = %q", entries[2].Text)
	}
}

func TestACPSendPromptWithoutSession(t *testing.T) {
	app, _, pid := newAgentTestApp(t)
	if err := app.ACPSendPrompt(pid, "x"); err == nil {
		t.Fatal("want error without running harness")
	}
}

func TestACPPermissionRespondSelection(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	fp := &fakePrompter{}
	installFakeAgent(t, fp)

	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}

	opts := []any{map[string]any{"optionId": "allow-once", "name": "Allow", "kind": "allow_once"}}
	var handlerErr error
	resCh := make(chan any, 1)
	go func() {
		// reach into the agent session's permission handler through the app
		ag := app.agentFor(pid)
		res, err := app.acpRequestPermission(pid, ag, map[string]any{"options": opts})
		handlerErr = err
		resCh <- res
	}()
	ev := sink.waitFor(t, EventACPPermission, 2*time.Second)
	m := ev.payload.(map[string]any)
	reqID, _ := m["requestId"].(string)
	if reqID == "" {
		t.Fatalf("permission event missing requestId: %#v", m)
	}
	if err := app.ACPRespondPermission(pid, reqID, "allow-once", false); err != nil {
		t.Fatalf("ACPRespondPermission: %v", err)
	}
	select {
	case res := <-resCh:
		if res != "allow-once" {
			t.Fatalf("permission result = %v, want allow-once", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for permission resolution")
	}
	if handlerErr != nil {
		t.Fatalf("handler err: %v", handlerErr)
	}
}

func TestACPPermissionRespondCancel(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	fp := &fakePrompter{}
	installFakeAgent(t, fp)
	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}

	resCh := make(chan any, 1)
	go func() {
		ag := app.agentFor(pid)
		res, _ := app.acpRequestPermission(pid, ag, map[string]any{"options": []any{}})
		resCh <- res
	}()
	ev := sink.waitFor(t, EventACPPermission, 2*time.Second)
	reqID := ev.payload.(map[string]any)["requestId"].(string)
	if err := app.ACPRespondPermission(pid, reqID, "", true); err != nil {
		t.Fatalf("ACPRespondPermission: %v", err)
	}
	select {
	case res := <-resCh:
		if res != nil {
			t.Fatalf("cancelled result = %v, want nil", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestACPRespondPermissionUnknownRequest(t *testing.T) {
	app, _, pid := newAgentTestApp(t)
	fp := &fakePrompter{}
	installFakeAgent(t, fp)
	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}
	if err := app.ACPRespondPermission(pid, "nope", "allow", false); err == nil {
		t.Fatal("want error for unknown request id")
	}
}

func TestACPNewSessionClosesOldAndKeepsTranscript(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	fp1 := &fakePrompter{}
	installFakeAgent(t, fp1)
	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}
	if err := app.ACPSendPrompt(pid, "first turn"); err != nil {
		t.Fatalf("ACPSendPrompt: %v", err)
	}
	_ = sink

	fp2 := &fakePrompter{}
	installFakeAgent(t, fp2)
	if err := app.ACPNewSession(pid); err != nil {
		t.Fatalf("ACPNewSession: %v", err)
	}
	if !fp1.closed {
		t.Error("old session was not closed")
	}
	if fp2.closed {
		t.Error("new session should be running")
	}
	entries, err := app.ACPLoadTranscript(pid)
	if err != nil {
		t.Fatalf("LoadTranscript: %v", err)
	}
	if len(entries) == 0 || entries[len(entries)-1].Text != "— new session —" {
		t.Fatalf("transcript tail = %#v, want divider entry", entries)
	}
}

func TestACPNewSessionWithoutHarness(t *testing.T) {
	app, _, pid := newAgentTestApp(t)
	if err := app.ACPNewSession(pid); err == nil {
		t.Fatal("want error without harness")
	}
}

func TestACPStopClosesSession(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	fp := &fakePrompter{}
	installFakeAgent(t, fp)
	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}
	if err := app.ACPStop(pid); err != nil {
		t.Fatalf("ACPStop: %v", err)
	}
	if !fp.closed {
		t.Error("session not closed by ACPStop")
	}
	st := sink.snapshot()
	last := st[len(st)-1]
	if last.name != EventACPState || last.payload.(map[string]any)["state"] != AgentStateHarnessDown {
		t.Fatalf("last event = %v %#v, want harness-down", last.name, last.payload)
	}
	// stopping again is a no-op
	if err := app.ACPStop(pid); err != nil {
		t.Fatalf("second ACPStop: %v", err)
	}
}

func TestACPStartSpawnFailureReportsHarnessDown(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	prev := acpStartSession
	acpStartSession = func(ctx context.Context, root string, profile acp.Info, handlers acp.ClientHandlers) (acpPrompter, error) {
		return nil, errors.New("spawn failed")
	}
	t.Cleanup(func() { acpStartSession = prev })

	if err := app.ACPStart(pid, "opencode"); err == nil {
		t.Fatal("want error from failed spawn")
	}
	st := sink.waitFor(t, EventACPState, 2*time.Second)
	if st.payload.(map[string]any)["state"] != AgentStateHarnessDown {
		t.Fatalf("state = %#v, want harness-down", st.payload)
	}
}

func TestContainedPath(t *testing.T) {
	root := t.TempDir()
	p, err := containedPath(root, filepath.Join(root, "a", "b.txt"))
	if err != nil || p != filepath.Join(root, "a", "b.txt") {
		t.Fatalf("containedPath inside = %v, %v", p, err)
	}
	if _, err := containedPath(root, "/etc/passwd"); err == nil {
		t.Error("want error for path outside root")
	}
	if _, err := containedPath(root, filepath.Join(root, "..", "escape")); err == nil {
		t.Error("want error for traversal")
	}
	if _, err := containedPath("", "x"); err == nil {
		t.Error("want error for empty root")
	}
}

func TestToolContentText(t *testing.T) {
	u := acp.SessionUpdate{
		ContentItems: []acp.ContentBlock{{Type: acp.BlockTypeText, Text: "line1"}},
		ToolCall: &acp.ToolCall{
			Content: []acp.ContentBlock{{Type: acp.BlockTypeText, Text: "line2"}},
		},
	}
	if got := toolContentText(u); got != "line1\nline2" {
		t.Fatalf("toolContentText = %q", got)
	}
	if got := toolContentText(acp.SessionUpdate{}); got != "" {
		t.Fatalf("empty toolContentText = %q", got)
	}
}

// TestACPPromptErrorSurfacedInChat checks a Prompt failure lands as an error
// chat entry instead of a failed RPC.
func TestACPPromptErrorSurfacedInChat(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	fp := &fakePrompter{}
	installFakeAgent(t, fp)
	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}

	// make the fake fail
	boom := &boomPrompter{}
	app.agentFor(pid).setSession(boom)
	sink.snapshot()

	if err := app.ACPSendPrompt(pid, "go"); err != nil {
		t.Fatalf("ACPSendPrompt returned error: %v (want chat-surfaced)", err)
	}
	var sawErr bool
	for _, ev := range sink.snapshot() {
		if ev.name == EventACPMsg {
			m := ev.payload.(map[string]any)
			if m["kind"] == "error" {
				sawErr = true
			}
		}
	}
	if !sawErr {
		t.Fatal("no error chat event emitted")
	}
}

type boomPrompter struct{}

func (b *boomPrompter) Prompt(ctx context.Context, text string) (acp.PromptResponse, error) {
	return acp.PromptResponse{}, errors.New("agent exploded")
}
func (b *boomPrompter) Close() error { return nil }
func (b *boomPrompter) SetOnUpdate(func(acp.SessionUpdate)) {}

// TestACPStartUserEntryPersisted verifies json round-trip of agentstore.Entry
// through the transcript path (guards the emit payload shape).
func TestACPTranscriptEntryJSONRoundtrip(t *testing.T) {
	e := agentstore.Entry{Role: "agent", Kind: agentstore.KindTool, Text: "t", ToolID: "x", When: time.Now()}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back agentstore.Entry
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ToolID != "x" || back.Kind != agentstore.KindTool {
		t.Fatalf("roundtrip mismatch: %#v", back)
	}
}

// ensure os import used (keep imports honest if tests evolve)
var _ = os.Getenv
