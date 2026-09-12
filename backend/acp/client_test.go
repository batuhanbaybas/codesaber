package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// agentScript routes a fake agent: it gets our request id, method and params,
// and returns a response line (or writes notifications before responding).
type agentScript func(id any, method string, params map[string]any) string

// fakeAgent exposes lines of ours the script did not answer, e.g. our
// session/cancel notifications.
type fakeAgent struct{ unhandled chan string }

// serveAgentScript runs a minimal fake agent loop over the harness pipes:
// reads request lines written by the Conn and responds via script. Lines the
// script does not answer (notifications) are surfaced on returned.unhandled.
func (h *harnessConn) serveAgentScript(t *testing.T, script agentScript) *fakeAgent {
	t.Helper()
	fa := &fakeAgent{unhandled: make(chan string, 64)}
	go func() {
		sc := bufio.NewScanner(h.clientOut)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			f, err := DecodeLine(sc.Bytes())
			if err != nil {
				continue
			}
			if f.ID == nil || f.Method == "" {
				fa.unhandled <- string(sc.Bytes())
				continue
			}
			if resp := script(f.ID, f.Method, f.Params.(map[string]any)); resp != "" {
				h.agentIn.Write([]byte(resp + "\n"))
			}
		}
	}()
	return fa
}

func TestStartSessionHandshake(t *testing.T) {
	h := newHarness(t, ClientHandlers{})
	var initParams, newParams map[string]any
	h.serveAgentScript(t, func(id any, method string, params map[string]any) string {
		switch method {
		case MethodInitialize:
			b, _ := json.Marshal(params)
			json.Unmarshal(b, &initParams)
			return fmtResponse(id, map[string]any{"protocolVersion": 1})
		case MethodSessionNew:
			b, _ := json.Marshal(params)
			json.Unmarshal(b, &newParams)
			return fmtResponse(id, map[string]any{"sessionId": "s-1"})
		}
		return fmtResponse(id, map[string]any{})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, err := startSessionOnConn(ctx, h.conn, "/proj/root", ClientHandlers{})
	if err != nil {
		t.Fatalf("startSessionOnConn: %v", err)
	}
	if s.id != "s-1" {
		t.Fatalf("session id = %q, want s-1", s.id)
	}

	// initialize wire shape
	if got, _ := initParams["protocolVersion"].(float64); got != 1 {
		t.Errorf("initialize.protocolVersion = %v, want 1", initParams["protocolVersion"])
	}
	clientInfo, _ := initParams["clientInfo"].(map[string]any)
	if clientInfo == nil || clientInfo["name"] != "aide" || clientInfo["version"] != "0.1.0" {
		t.Errorf("initialize.clientInfo = %#v, want {name:aide,version:0.1.0}", initParams["clientInfo"])
	}
	caps, _ := initParams["clientCapabilities"].(map[string]any)
	fs, _ := caps["fs"].(map[string]any)
	if fs == nil {
		t.Fatalf("initialize.clientCapabilities.fs missing: %#v", initParams)
	}
	if fs["readTextFile"] != false || fs["writeTextFile"] != false {
		t.Errorf("fs caps = %#v, want {readTextFile:false,writeTextFile:false}", fs)
	}

	// session/new wire shape
	if newParams["cwd"] != "/proj/root" {
		t.Errorf("session/new.cwd = %v, want /proj/root", newParams["cwd"])
	}
	servers, _ := newParams["mcpServers"].([]any)
	if servers == nil || len(servers) != 0 {
		t.Errorf("session/new.mcpServers = %#v, want []", newParams["mcpServers"])
	}
}

func TestSessionDrainsPromptTurnWithoutUpdates(t *testing.T) {
	h := newHarness(t, ClientHandlers{})
	var promptParams map[string]any
	var turns int
	h.serveAgentScript(t, func(id any, method string, params map[string]any) string {
		switch method {
		case MethodInitialize:
			return fmtResponse(id, map[string]any{"protocolVersion": 1})
		case MethodSessionNew:
			return fmtResponse(id, map[string]any{"sessionId": "s-1"})
		case MethodSessionPrompt:
			turns++
			b, _ := json.Marshal(params)
			json.Unmarshal(b, &promptParams)
			return fmtResponse(id, map[string]any{"stopReason": StopReasonEndTurn})
		}
		return fmtResponse(id, map[string]any{})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, err := startSessionOnConn(ctx, h.conn, "/proj/root", ClientHandlers{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	out, err := s.Prompt(ctx, "hello")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if out.StopReason != StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want end_turn", out.StopReason)
	}
	if turns != 1 {
		t.Fatalf("turns = %d, want 1", turns)
	}

	// session/prompt wire shape
	if promptParams["sessionId"] != "s-1" {
		t.Errorf("prompt.sessionId = %v, want s-1", promptParams["sessionId"])
	}
	items, _ := promptParams["prompt"].([]any)
	if len(items) != 1 {
		t.Fatalf("prompt blocks = %#v, want 1", promptParams["prompt"])
	}
	blk, _ := items[0].(map[string]any)
	if blk["type"] != "text" || blk["text"] != "hello" {
		t.Errorf("prompt block = %#v, want {type:text,text:hello}", blk)
	}
}

func TestSessionSurfacesUpdateNotifications(t *testing.T) {
	h := newHarness(t, ClientHandlers{})
	h.serveAgentScript(t, func(id any, method string, params map[string]any) string {
		switch method {
		case MethodInitialize:
			return fmtResponse(id, map[string]any{"protocolVersion": 1})
		case MethodSessionNew:
			return fmtResponse(id, map[string]any{"sessionId": "s-1"})
		case MethodSessionPrompt:
			notif := NewNotify(MethodSessionUpdate, map[string]any{
				"sessionId": "s-1",
				"update": map[string]any{
					"sessionUpdate": UpdateAgentMessageChunk,
					"content":       map[string]any{"type": "text", "text": "chunk1"},
				},
			})
			h.agentIn.Write(notif.MarshalLine())
			notif2 := NewNotify(MethodSessionUpdate, map[string]any{
				"sessionId": "s-1",
				"update": map[string]any{
					"sessionUpdate": UpdateAgentMessageChunk,
					"content":       map[string]any{"type": "text", "text": "chunk2"},
				},
			})
			h.agentIn.Write(notif2.MarshalLine())
			return fmtResponse(id, map[string]any{"stopReason": StopReasonEndTurn})
		}
		return fmtResponse(id, map[string]any{})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, err := startSessionOnConn(ctx, h.conn, "/proj/root", ClientHandlers{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	var got []string
	s.OnUpdate = func(u SessionUpdate) {
		if u.Content != nil {
			got = append(got, u.Content.Text)
		}
	}
	if _, err := s.Prompt(ctx, "go"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("OnUpdate chunks = %#v, want [chunk1 chunk2]", got)
	}
	if got[0] != "chunk1" || got[1] != "chunk2" {
		t.Fatalf("chunks = %#v, want in order", got)
	}
}

func TestSessionPromptRejectsConcurrentPrompt(t *testing.T) {
	h := newHarness(t, ClientHandlers{})
	block := make(chan struct{})
	h.serveAgentScript(t, func(id any, method string, _ map[string]any) string {
		switch method {
		case MethodInitialize:
			return fmtResponse(id, map[string]any{"protocolVersion": 1})
		case MethodSessionNew:
			return fmtResponse(id, map[string]any{"sessionId": "s-1"})
		case MethodSessionPrompt:
			<-block
			return fmtResponse(id, map[string]any{"stopReason": StopReasonEndTurn})
		}
		return fmtResponse(id, map[string]any{})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s, err := startSessionOnConn(ctx, h.conn, "/proj/root", ClientHandlers{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.Prompt(ctx, "first")
		done <- err
	}()
	// Let the first prompt win the flag race.
	time.Sleep(50 * time.Millisecond)
	if _, err := s.Prompt(ctx, "second"); err == nil {
		t.Fatal("want concurrent prompt error, got nil")
	}
	close(block)
	if err := <-done; err != nil {
		t.Fatalf("first prompt: %v", err)
	}
}

func TestSessionCancelNotification(t *testing.T) {
	h := newHarness(t, ClientHandlers{})
	fa := h.serveAgentScript(t, func(id any, method string, _ map[string]any) string {
		switch method {
		case MethodInitialize:
			return fmtResponse(id, map[string]any{"protocolVersion": 1})
		case MethodSessionNew:
			return fmtResponse(id, map[string]any{"sessionId": "s-1"})
		}
		return ""
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, err := startSessionOnConn(ctx, h.conn, "/proj/root", ClientHandlers{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := s.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	raw := <-fa.unhandled
	f, err := DecodeLine([]byte(raw))
	if err != nil {
		t.Fatalf("decode cancel: %v (%s)", err, raw)
	}
	if f.Method != MethodSessionCancel {
		t.Fatalf("method = %q, want session/cancel", f.Method)
	}
	if f.ID != nil {
		t.Fatal("cancel must be a notification (no id)")
	}
	m, _ := f.Params.(map[string]any)
	if m["sessionId"] != "s-1" {
		t.Fatalf("cancel params = %#v, want sessionId=s-1", f.Params)
	}
}

func TestPromptResponseDecoding(t *testing.T) {
	if _, err := decodePromptResponse(map[string]any{"stopReason": StopReasonEndTurn}); err != nil {
		t.Errorf("valid response: %v", err)
	}
	if _, err := decodePromptResponse(map[string]any{}); err == nil {
		t.Error("missing stopReason: want error")
	}
	if _, err := decodePromptResponse("nope"); err == nil {
		t.Error("non-object result: want error")
	}
}

// fmtResponse builds a result response line for the fake agent.
func fmtResponse(id any, result map[string]any) string {
	b, _ := json.Marshal(result)
	// id round-trips as float64 through DecodeLine; normalize for output.
	rawID := id
	if f, ok := id.(float64); ok {
		rawID = json.Number(strconv.FormatFloat(f, 'f', -1, 64))
	}
	var sb strings.Builder
	sb.WriteString(`{"jsonrpc":"2.0","id":`)
	if s, ok := rawID.(json.Number); ok {
		sb.WriteString(s.String())
	} else {
		enc, _ := json.Marshal(rawID)
		sb.Write(enc)
	}
	sb.WriteString(`,"result":`)
	sb.Write(b)
	sb.WriteString(`}`)
	return sb.String()
}

func TestConnPermissionResponseSpecShaped(t *testing.T) {
	handlers := ClientHandlers{RequestPermission: func(params map[string]any) (any, error) {
		if params["sessionId"] != "s-1" {
			t.Errorf("permission params = %#v, want sessionId=s-1", params)
		}
		return "allow-once", nil
	}}
	h := newHarness(t, handlers)
	line := NewRequest(7, MethodRequestPermission, map[string]any{
		"sessionId": "s-1",
		"toolCall":  map[string]any{"toolCallId": "t-1", "title": "read"},
		"options":   []any{map[string]any{"kind": "allow_once", "name": "Allow", "optionId": "allow-once"}},
	}).MarshalLine()
	if _, err := h.agentIn.Write(line); err != nil {
		t.Fatalf("write request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	f, err := readResponse(ctx, h)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if f.Error != nil {
		t.Fatalf("want result, got error: %s", f.Error)
	}
	m, _ := f.Result.(map[string]any)
	outcome, _ := m["outcome"].(map[string]any)
	if outcome["outcome"] != "selected" || outcome["optionId"] != "allow-once" {
		t.Fatalf("outcome = %#v, want {outcome:selected,optionId:allow-once}", outcome)
	}
}

func TestConnPermissionResponseCancelledWhenHandlerReturnsNil(t *testing.T) {
	handlers := ClientHandlers{RequestPermission: func(params map[string]any) (any, error) {
		return nil, nil
	}}
	h := newHarness(t, handlers)
	line := NewRequest(7, MethodRequestPermission, map[string]any{
		"sessionId": "s-1", "toolCall": map[string]any{}, "options": []any{},
	}).MarshalLine()
	if _, err := h.agentIn.Write(line); err != nil {
		t.Fatalf("write request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	f, err := readResponse(ctx, h)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	m, _ := f.Result.(map[string]any)
	outcome, _ := m["outcome"].(map[string]any)
	if outcome["outcome"] != "cancelled" {
		t.Fatalf("outcome = %#v, want cancelled", outcome)
	}
}

// readResponse blocks until the Conn writes back a response line for id 7.
func readResponse(ctx context.Context, h *harnessConn) (Frame, error) {
	ch := make(chan Frame, 1)
	go func() {
		raw := h.readLineBlocking()
		if f, err := DecodeLine([]byte(raw)); err == nil {
			ch <- f
		}
	}()
	select {
	case f := <-ch:
		return f, nil
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

func (h *harnessConn) readLineBlocking() string {
	buf := make([]byte, 0, 4096)
	b := make([]byte, 1)
	for {
		n, err := h.clientOut.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				return string(buf)
			}
			buf = append(buf, b[0])
			continue
		}
		if err != nil {
			return string(buf)
		}
	}
}
