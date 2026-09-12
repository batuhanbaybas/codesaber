package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aide/backend/acp"
	"aide/backend/agentstore"
)

// ACP agent event names (backend→UI).
const (
	EventACPMsg        = "acp.msg"        // {projectId, role, text, kind}
	EventACPTool       = "acp.tool"       // {projectId, toolCallId, title, kind, status, content}
	EventACPPermission = "acp.permission" // {projectId, requestId, options}
	EventACPState      = "acp.state"      // {projectId, state: idle|thinking|harness-down|no-harness}
	EventACPTranscript = "acp.transcript" // {projectId, entries}
)

// Agent status values carried by acp.state.
const (
	AgentStateIdle        = "idle"
	AgentStateThinking    = "thinking"
	AgentStateHarnessDown = "harness-down"
	AgentStateNoHarness   = "no-harness"
)

// acpPermissionTimeout bounds how long a permission request blocks the agent
// before it is answered as cancelled.
const acpPermissionTimeout = 60 * time.Second

// acpPrompter is the seam over acp.Session the facade drives (swap for fakes
// in tests).
type acpPrompter interface {
	Prompt(ctx context.Context, text string) (acp.PromptResponse, error)
	Close() error
	SetOnUpdate(func(acp.SessionUpdate))
}

// sessionPrompter adapts *acp.Session to acpPrompter.
type sessionPrompter struct{ s *acp.Session }

func (p *sessionPrompter) Prompt(ctx context.Context, text string) (acp.PromptResponse, error) {
	return p.s.Prompt(ctx, text)
}
func (p *sessionPrompter) Close() error { return p.s.Close() }
func (p *sessionPrompter) SetOnUpdate(f func(acp.SessionUpdate)) {
	p.s.OnUpdate = f
}

// acpStartSession spawns an ACP session; swapped out in tests.
var acpStartSession = func(ctx context.Context, root string, profile acp.Info, handlers acp.ClientHandlers) (acpPrompter, error) {
	s, err := acp.StartSession(ctx, root, profile, handlers)
	if err != nil {
		return nil, err
	}
	return &sessionPrompter{s}, nil
}

// acpPermissionChoice is the user's answer to one pending permission request.
type acpPermissionChoice struct {
	optionID string
	cancel   bool
}

// agentSession is the per-project ACP harness state: the live session, the
// harness profile it was started with, pending permission requests and the
// in-flight prompt turn accumulator.
type agentSession struct {
	mu      sync.Mutex
	session acpPrompter
	harness acp.Info
	root    string
	pending map[string]chan acpPermissionChoice
	nextID  int

	turnMu   sync.Mutex
	turnText strings.Builder
	turnSeen map[string]bool
}

func (ag *agentSession) get() acpPrompter {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	return ag.session
}

func (ag *agentSession) setSession(s acpPrompter) {
	ag.mu.Lock()
	ag.session = s
	ag.mu.Unlock()
}

func (ag *agentSession) appendTurn(text string) {
	ag.turnMu.Lock()
	ag.turnText.WriteString(text)
	ag.turnMu.Unlock()
}

// noteTool records a tool call id and reports whether it was new (used to
// persist each tool call once).
func (ag *agentSession) noteTool(id string) bool {
	ag.turnMu.Lock()
	defer ag.turnMu.Unlock()
	if ag.turnSeen == nil {
		ag.turnSeen = map[string]bool{}
	}
	if ag.turnSeen[id] {
		return false
	}
	ag.turnSeen[id] = true
	return true
}

func (ag *agentSession) resetTurn() {
	ag.turnMu.Lock()
	ag.turnText.Reset()
	ag.turnSeen = map[string]bool{}
	ag.turnMu.Unlock()
}

func (ag *agentSession) takeTurnText() string {
	ag.turnMu.Lock()
	defer ag.turnMu.Unlock()
	s := ag.turnText.String()
	ag.turnText.Reset()
	return s
}

// ACPHarnesses lists known agent profiles with PATH availability.
func (a *App) ACPHarnesses() []acp.Info {
	return acp.DefaultProfiles()
}

// ACPStart resolves the harness profile and spawns one ACP session for the
// project (one session/connection MVP). After start the persisted transcript
// is emitted (acp.transcript) followed by an idle state.
func (a *App) ACPStart(projectID, harnessName string) error {
	profile, err := acp.Resolve(harnessName)
	if err != nil {
		return err
	}
	return a.acpStartProfile(projectID, *profile)
}

func (a *App) acpStartProfile(projectID string, profile acp.Info) error {
	root, err := a.resolveRoot(projectID)
	if err != nil {
		return err
	}

	a.mu.Lock()
	if cur, ok := a.agents[projectID]; ok && cur.get() != nil {
		// already running: idempotent, just re-show the transcript
		a.mu.Unlock()
		a.emitTranscript(projectID)
		return nil
	}
	ag := &agentSession{root: root, harness: profile, pending: map[string]chan acpPermissionChoice{}}
	a.agents[projectID] = ag
	a.mu.Unlock()

	s, err := acpStartSession(context.Background(), root, profile, acp.ClientHandlers{
		ReadTextFile: func(_, path string) (string, error) {
			p, cerr := containedPath(root, path)
			if cerr != nil {
				return "", cerr
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return "", fmt.Errorf("acp: read %s: %w", path, rerr)
			}
			return string(b), nil
		},
		WriteTextFile: func(_, path, content string) error {
			p, cerr := containedPath(root, path)
			if cerr != nil {
				return cerr
			}
			if derr := os.MkdirAll(filepath.Dir(p), 0o755); derr != nil {
				return fmt.Errorf("acp: mkdir %s: %w", filepath.Dir(p), derr)
			}
			if werr := os.WriteFile(p, []byte(content), 0o644); werr != nil {
				return fmt.Errorf("acp: write %s: %w", path, werr)
			}
			return nil
		},
		RequestPermission: func(params map[string]any) (any, error) {
			return a.acpRequestPermission(projectID, ag, params)
		},
	})
	if err != nil {
		a.mu.Lock()
		delete(a.agents, projectID)
		a.mu.Unlock()
		a.emitState(projectID, AgentStateHarnessDown)
		return err
	}
	s.SetOnUpdate(a.acpUpdateHandler(projectID, ag))
	ag.setSession(s)
	a.emitTranscript(projectID)
	a.emitState(projectID, AgentStateIdle)
	return nil
}

// ACPSendPrompt runs one synchronous prompt turn: the user entry is persisted
// and emitted up front, session/update notifications stream out as acp.msg /
// acp.tool events while the turn is open, and the assembled agent reply is
// persisted once the turn completes.
func (a *App) ACPSendPrompt(projectID, text string) error {
	ag := a.agentFor(projectID)
	if ag == nil {
		return errors.New("acp: no agent session for project; start a harness first")
	}
	s := ag.get()
	if s == nil {
		return errors.New("acp: agent session is down; start a harness first")
	}

	if err := a.chats.Append(projectID, agentstore.Entry{Role: "user", Kind: agentstore.KindText, Text: text}); err != nil {
		return fmt.Errorf("acp: persist user entry: %w", err)
	}
	a.sink.Emit(EventACPMsg, map[string]any{"projectId": projectID, "role": "user", "text": text, "kind": "text"})
	a.emitState(projectID, AgentStateThinking)

	ag.resetTurn()
	_, err := s.Prompt(context.Background(), text)
	if reply := ag.takeTurnText(); reply != "" {
		_ = a.chats.Append(projectID, agentstore.Entry{Role: "agent", Kind: agentstore.KindText, Text: reply})
	}
	if err != nil {
		// surface the failure in the chat; the conn is typically dead after
		// this (agent crashed / stream closed)
		a.sink.Emit(EventACPMsg, map[string]any{"projectId": projectID, "role": "agent", "text": err.Error(), "kind": "error"})
		a.emitState(projectID, AgentStateIdle)
		return nil
	}
	a.emitState(projectID, AgentStateIdle)
	return nil
}

// ACPRespondPermission resolves a pending permission request: optionId picks
// one of the offered options; cancel=true answers {"outcome":"cancelled"}.
func (a *App) ACPRespondPermission(projectID, requestID, optionID string, cancel bool) error {
	ag := a.agentFor(projectID)
	if ag == nil {
		return errors.New("acp: no agent session for project")
	}
	ag.mu.Lock()
	ch, ok := ag.pending[requestID]
	ag.mu.Unlock()
	if !ok {
		return fmt.Errorf("acp: no pending permission %q", requestID)
	}
	ch <- acpPermissionChoice{optionID: optionID, cancel: cancel}
	return nil
}

// ACPNewSession closes the current harness connection and spawns a fresh
// session with the same harness. The transcript is kept; a divider note entry
// marks the boundary.
func (a *App) ACPNewSession(projectID string) error {
	ag := a.agentFor(projectID)
	if ag == nil || ag.get() == nil {
		return errors.New("acp: no running harness for project")
	}
	harness := ag.harness
	if s := ag.get(); s != nil {
		_ = s.Close()
	}
	ag.setSession(nil)
	if err := a.chats.Append(projectID, agentstore.Entry{Role: "system", Kind: agentstore.KindText, Text: "— new session —"}); err != nil {
		return fmt.Errorf("acp: persist session divider: %w", err)
	}
	return a.acpStartProfile(projectID, harness)
}

// ACPStop closes the harness connection (terminating the child process) and
// reports harness-down.
func (a *App) ACPStop(projectID string) error {
	a.mu.Lock()
	ag, ok := a.agents[projectID]
	delete(a.agents, projectID)
	a.mu.Unlock()
	if !ok || ag == nil {
		return nil
	}
	if s := ag.get(); s != nil {
		_ = s.Close()
	}
	a.emitState(projectID, AgentStateHarnessDown)
	return nil
}

// ACPLoadTranscript returns the persisted chat entries for the project (used
// by the Agent panel on mount).
func (a *App) ACPLoadTranscript(projectID string) ([]agentstore.Entry, error) {
	return a.chats.Read(projectID)
}

func (a *App) agentFor(projectID string) *agentSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.agents[projectID]
}

// acpRequestPermission registers a pending request, surfaces it to the UI and
// blocks for the user's choice (or the timeout, which answers cancelled).
func (a *App) acpRequestPermission(projectID string, ag *agentSession, params map[string]any) (any, error) {
	ag.mu.Lock()
	ag.nextID++
	id := fmt.Sprintf("perm-%d", ag.nextID)
	ch := make(chan acpPermissionChoice, 1)
	ag.pending[id] = ch
	ag.mu.Unlock()
	defer func() {
		ag.mu.Lock()
		delete(ag.pending, id)
		ag.mu.Unlock()
	}()

	a.sink.Emit(EventACPPermission, map[string]any{
		"projectId": projectID,
		"requestId": id,
		"options":   params["options"],
	})

	select {
	case c := <-ch:
		if c.cancel {
			return nil, nil
		}
		return c.optionID, nil
	case <-time.After(acpPermissionTimeout):
		return nil, nil
	}
}

// acpUpdateHandler translates session/update notifications into UI events and
// transcript persistence for the open prompt turn.
func (a *App) acpUpdateHandler(projectID string, ag *agentSession) func(acp.SessionUpdate) {
	return func(u acp.SessionUpdate) {
		switch u.SessionUpdate {
		case acp.UpdateAgentMessageChunk:
			if u.Content == nil || u.Content.Text == "" {
				return
			}
			ag.appendTurn(u.Content.Text)
			a.sink.Emit(EventACPMsg, map[string]any{
				"projectId": projectID, "role": "agent",
				"text": u.Content.Text, "kind": "chunk",
			})
		case acp.UpdateToolCall, acp.UpdateToolCallUpdate:
			id, title := u.ToolCallID, u.Title
			if u.ToolCall != nil {
				if id == "" {
					id = u.ToolCall.ID
				}
				if title == "" {
					title = u.ToolCall.Title
				}
			}
			if id == "" {
				return
			}
			if ag.noteTool(id) && title != "" {
				_ = a.chats.Append(projectID, agentstore.Entry{
					Role: "agent", Kind: agentstore.KindTool, Text: title, ToolID: id,
				})
			}
			a.sink.Emit(EventACPTool, map[string]any{
				"projectId": projectID, "toolCallId": id, "title": title,
				"kind": u.Kind, "status": u.Status, "content": toolContentText(u),
			})
		}
	}
}

func (a *App) emitTranscript(projectID string) {
	entries, err := a.chats.Read(projectID)
	if err != nil {
		entries = []agentstore.Entry{}
	}
	a.sink.Emit(EventACPTranscript, map[string]any{"projectId": projectID, "entries": entries})
}

func (a *App) emitState(projectID, state string) {
	a.sink.Emit(EventACPState, map[string]any{"projectId": projectID, "state": state})
}

// containedPath resolves path for agent fs access, refusing anything outside
// the project root (path traversal guard).
func containedPath(root, path string) (string, error) {
	if root == "" {
		return "", errors.New("acp: no project root for fs access")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("acp: path %q: %w", path, err)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("acp: root %q: %w", root, err)
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("acp: path %q is outside the project root", path)
	}
	return abs, nil
}

// toolContentText joins the text content blocks of a tool call update.
func toolContentText(u acp.SessionUpdate) string {
	blocks := u.ContentItems
	if u.ToolCall != nil && len(u.ToolCall.Content) > 0 {
		blocks = append(append([]acp.ContentBlock(nil), blocks...), u.ToolCall.Content...)
	}
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == acp.BlockTypeText && b.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}
