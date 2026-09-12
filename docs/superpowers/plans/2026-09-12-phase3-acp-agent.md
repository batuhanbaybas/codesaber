# Phase 3 — ACP Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Talk to any ACP harness (opencode first, Claude Code / others behind config) from the Agent tab: per-project sessions, streaming chat, sensible permission handling for agent tool-use.

**Architecture:** New `backend/acp` engine: generic JSON-RPC 2.0 client/server over stdio; spawns the harness process per session. ACP methods used: `initialize`, `session/new`, `session/prompt`, `session/update` (notification), `session/request_permission`, agent→client `fs/read_text_file`/`fs/write_text_file`. Facade `backend/app.go` RPCs + `acp.*` events. Frontend: ChatPanel replaces the Agent tab; sessions keyed by project, persisted chat transcript per project in app config dir.

**Protocol reference:** https://agentclientprotocol.com (official ACP spec). Harness flags (verify at execution):
- opencode: `opencode acp`
- claude code: `claude-code-acp` adapter (npm) or `claude --acp` if supported by installed version
- gemini: `gemini --experimental-acp` (optional)

**Design decisions locked:**
- ONE ACP connection per project, one harness process per project. Restart harness on crash, transcript survives.
- MVP permission model: agent `session/request_permission` → surface as inline buttons (Allow once / Always / Reject). Reply `outcome = allow|reject` with `optionId` if the harness sent options; otherwise plain allow/reject.
- fs/write_text_file accepted ONLY after permission grant; fs/read_text_file always allowed (agent needs to read code to work).
- Chat persistence: append JSONL transcript file per project at `os.UserConfigDir()/aide/chats/<projectId>.jsonl`; restored in Agent tab on window open.

**Tech Stack:** stdlib (encoding/json, os/exec, bufio), no extra deps. Frontend: existing providers pattern.

Standing requirement: update `docs/backlog.md` at phase end.

---

## File Structure

```
backend/acp/codec.go        # JSON-RPC frame types + Request/Response indirection, ID gen (atomic)
backend/acp/codec_test.go   # TDD: framing, direction decoding both roles
backend/acp/connection.go   # Child process + two-direction dispatch (bufio scan over \n\n JSON)
backend/acp/acp.go          # ACP protocol model: capabilities, session ids, msg items, update types
backend/acp/harness.go      # spawn profiles: harness config {name, argv}; profiles built into binary defaults + app-config override
backend/acp/client.go       # Session: prompt, update stream, permission request handling
backend/acp/client_test.go  # test harness w/ fake ACP agent (dumb binary speaking the protocol — small os/exec script via `go run` testdata or subprocess ping-pong via net.Pipe pairing)
backend/agent/store.go      # transcript append + read (JSONL), TDD
backend/app.go              # facade: acp RPCs + events
frontend/src/state/agent.tsx# AgentProvider per project
frontend/src/components/AgentPanel.tsx # chat UI
```

Implementation notes: ACP over stdio uses newline-delimited JSON-RPC (confirm spec §transport: each message = single line JSON, "\n" delimited — matches spec "messages are delimited by newlines").

---

### Task 1: JSON-RPC codec (TDD)

**Files:** `backend/acp/codec.go`, `backend/acp/codec_test.go`.

- [ ] **Step 1: Failing tests** — cover: (a) MarshalRequest→ line, string window entry; (b) DecodeFrame roundtrip: distinguish request(`/`notification / response by presence of id+/method/+/result/+/error; (c) concurrent ID counter.

```go
func TestFrameTypes(t *testing.T) {
	req := NewRequest(3, "session/prompt", map[string]any{"sessionId": "s1"})
	line := req.MarshalLine()
	if !strings.Contains(line, `"jsonrpc":"2.0"`) || !strings.Contains(line, `session/prompt`) {
		t.Fatalf("request line: %s", line)
	}
	notif := NewNotify("session/update", "s1")
	f, err := DecodeFrame([]byte(notif))
	if err != nil || f.Method != "session/update" && f.ID != nil {
		t.Fatalf("notif decode: %+v %v", f, err)
	}
	resp, err := DecodeFrame([]byte(req.MarshalResponse(map[string]any{"stopReason": "end_turn"})))
	if err != nil {
		t.Fatalf("resp: %v", err)
	}
}
```

Concretely in-code: `Frame{ID any, Method string, Params any, Result any, Error *RPCError}` with helpers. Implement, PASS, `feat(acp): json-rpc codec`.

### Task 2: Agent data model (TDD-lite)

**Files:** `backend/acp/acp.go`.

- [ ] **Step 1:** types: `Capabilities{Agent: struct { LoadSession bool }...whatever minimal}`, `PromptItem {type:"text", text}` (pointer opts), `SessionNewResult{SessionID}`, `Update{SessionUpdate string: agent_message_chunk | tool_call | tool_call_update | plan; fields: content chunks {text}, tool title/kind/status/content[] }`, `PermissionOutcome`.
- [ ] **Step 2:** implement + `go build`, PASS `feat(acp): protocol model`.

### Task 3: connection + spawn (TDD with fake agent)

**Files:** `backend/acp/connection.go`, `backend/acp/connection_test.go`, `backend/acp/harness.go`.

- [ ] **Step 1: failing test:** fake agent binary: a small Go program under `backend/acp/testdata` (`fakeagent.go`) that on stdin `initialize` → responds `{protocolVersion, agentCapabilities{LoadSession:true}}` etc; connection test: Spawn + SendRequest wait response + receive notify (`session/update` echo on prompt) and fs/read_text_file request from agent → client handler returns content.
- [ ] **Step 2: Connection** struct: cmd pipeline, two reader goroutines (in/out), request map[uint64]chan; Req(method, params) synchronously returns result/err; Notify out; OnNotify(handler); client request handler dispatched to a `Handlers` struct: `ReadTextFile(path)`, `WriteTextFile(path, content)`. Spawn: harness config `{Name, Command []string}` per defaults documented at top of file (profiles: opencode `opencode acp`, claude `claude-code-acp` adapter command name "claude-acp" — trim to accurate post-verification), overridable in app config `aide.json`.
- [ ] Implement, `feat(acp): stdio connection + spawn profiles`.

### Task 4: Session client + transcript store

**Files:** `backend/acp/client.go`, `backend/agentstore/store.go` (+test).

- [ ] SessionClient: wraps connection; `Initialize(clientInfo, agentInfo)`, `NewSession(projectRoot)→ID`, `Prompt(text)` → response {stopReason}; handles `session/update` notifications by calling a registered `OnUpdate(Update)`.
- [ ] backend/agentstore: `Append(projectID, entry)`, `Read(projectID) []Entry` — JSONL w/ Entry{Role,Content,Entities,Time}; mutex; file at os.UserConfigDir()/aide/chats/<id>.jsonl. TDD both.
- [ ] Commit: `feat(acp): session client + transcript store`.

### Task 5: Facade + frontend chat

**Files:** `backend/app.go`, `frontend/src/state/agent.tsx`, `frontend/src/components/AgentPanel.tsx`, `frontend/src/windows/Workspace.tsx`.

- [ ] Facade:
```go
EventACPMessage="acp.msg"; EventACPUpdate="acp.update"; EventACPPermission="acp.permission";EventACPHarness="acp.harness"
func (a *App) ACPStartHarness(projectID, harness string) error   // spawn per project; emits enabled harnesses list stored acp.harnesses
func (a *App) ACPNewSession(projectID string) error // initialize+session/new, transcript prefill loaded when new session started
func (a *App) ACPSendPrompt(projectID, text string) // full turn sync-ish: sends session/prompt; emit acp.msg user "user" message; updates → EventACPUpdate {projectId, sessionId, update}; changes to session/request_permission surface EventACPPermission {projectId, sessionId, options[]}
func (a *App) ACPRespondPermission(projectID, requestID, outcome string) error
func (a *App) ACPSpawnProfiles() []HarnessInfo  // names + availability (binary "which"-style check: exec.LookPath)
func (a *App) ACPStop(projectID string) error
func (a *App) ACPTerminate(projectID string) error  // kill child process, keep per-project ACP state cleared
```
- Transcripts: backed by agentstore; persisted entries written via call hooks on update + response; window AgentPanel prompt-box disabled until session ready; crash → setState emit recaps.
- Frontend AgentPanel: harness picker (SpawnProfiles), chat pane with bubbles (user right / agent left dim block margins), streaming chunk appends, permission request cards with 2-3 buttons (Reject / Allow once / Always allow), session start persistent across window reload (state lives in backend side anyway).
- Events: reconcile update types: agent_message_chunk → bubble stream; tool_call → tool call card with tiny status dot; permission → interactive card.
- Commit at "feat(acp): agent tab chat (opencode-first)".

### Task 6: Acceptance + backlog update

- [ ] **Manual with opencode binary** — run harness spawn → session → send small prompt like "What's in this repo?"—stream works; allow-once permission flow shows and resolves; process killed mid-chat → restart button in Agent tab appears; old transcript visible after restart.
- [ ] backlog: mark phase-3 ✅/near-done items; commit docs.

## Self-review
- Spec coverage: harness spawn ✓, streaming chat ✓, per-project isolation ✓ (one connection/project), transcript persistence ✓, permission flow ✓, crash-respwn ✓.
- Type consistency: acp.msg/acp.update Event payloads carry {projectId}; SessionID naming "sessionId" (camel).
