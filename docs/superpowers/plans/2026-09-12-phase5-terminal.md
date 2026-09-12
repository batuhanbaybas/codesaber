# Phase 5 — Real Terminal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Working terminal strip: real PTY (zsh) per tab under each project, xterm.js frontend, streaming I/O via Wails events, resize propagation.

**Architecture:** `backend/terminal` engine: creack/pty sessions keyed by terminalID, each wired cwd=project root, $SHELL process. Output streamed via `term.data` events per terminal; input via RPC; resize via RPC. Frontend: xterm.js (+fit addon) one Terminal instance per open tab.

**Tech Stack:** creack/pty, xterm.js + @xterm/addon-fit, @xterm/addon-web-links.

Standing requirement: update `docs/backlog.md` at phase end.

---

## File Structure

```
backend/terminal/terminal.go      # Session: PTY spawn, read pump (>5MB/s cap? no — raw stream), ID mgmt
backend/terminal/terminal_test.go # pipe-free: spawn real shell only in acceptance; unit tests via pty open+echo roundtrip (skip if no /dev/ptmx — darwin fine)
backend/app.go                    # facade: TermStart/TermInput/TermResize/TermStop RPCs + term.data event + term.exit
frontend/src/components/TerminalPanel.tsx # xterm.js wiring per tab
frontend/src/windows/Workspace.tsx        # terminal strip renders TerminalPanel (tabs + + button)
frontend/src/state/terminal.tsx           # TerminalProvider per project: open tabs map
```

---

### Task 1: PTY engine (TDD)

**Files:** `backend/terminal/terminal.go`, `backend/terminal/terminal_test.go`.

- [ ] **Step 1: Failing test** — spawn `/bin/sh -i` (POSIX, stable) through the engine: write `echo aide-pty-test-<n>\r`, read output until marker seen; termination test: Stop() then exit event. Requires no tty from test itself — creack/pty creates one. darwin arm64 works locally.

```go
func TestSessionEchoRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(SessionOpts{ID: "t1", Cwd: dir, Shell: "/bin/sh"})
	if err != nil { t.Fatal(err) }
	defer s.Close()
	data := s.Data() // <-chan byte-ish Frame {Data []byte, Exit bool}
	// write + read marker
}
```

- [ ] **Step 2: Implement** — Session: pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80}); goroutine reads into Data chan (frames of ≤4KB, coalescing read timeout 5ms? simpler: raw bytes read straight through, chan cap 64); `Input(b []byte)`; `Resize(rows, cols)`; `Wait()` after exit; Close() (SIGKILL the process group → pty has no direct; cmd.Process.Kill + pty cleanup).
- [ ] **Step 3: PASS + commit** `feat(terminal): pty session engine`.

### Task 2: Facade wiring

**Files:** `backend/app.go`, backend/app_test.go, bindings regen.

- [ ] RPCs:
```go
EventTermData = "term.data"  // {projectId, termId, data (b64)}
EventTermExit = "term.exit"  // {projectId, termId, code}
func (a *App) TermStart(projectID, termID string) error   // shell=os.Getenv("SHELL") fallback /bin/zsh; cwd=project root; registers, pump goroutine emits term.data per frame (no batching needed; 60fps worth), exit → term.exit + map cleanup
func (a *App) TermInput(termID string, data []byte) error  // route via map — CAREFUL: termIDs unique per project? make IDs globally unique (projectID + nanoid suffix)
func (a *App) TermResize(termID string, rows, cols int) error
func (a *App) TermStop(termID string) error
```
- Map key = termID (globally unique string: accountId @projectID…the unique id per terminal e.g. `<projectId>-<n>`); RouteInput via map lookup only.
- Tests: same fake-shell pattern as backend/terminal tests but via facade (spawn /bin/sh, input `echo xảy-hi`, expect data contains marker, TermStop → exit event). Facade integrity: workdir; bindings regen; commit `feat(app): terminal facade + streaming events`.

### Task 3: Frontend terminal

**Files:** `frontend/src/components/TerminalPanel.tsx`, `frontend/src/state/terminal.tsx`, `frontend/src/windows/Workspace.tsx`; npm i @xterm/xterm @xterm/addon-fit @xterm/addon-web-links.

- [ ] TerminalProvider: per project state `{open: string[]}` (termIDs); open→TermStart; close→TermStop. term.exit → drop tab (if it was last, strip stays with empty state "click + to open a terminal").
- [ ] TerminalPanel: xterm.js Terminal per open term (one component instance, key=termId): fit addon on ResizeObserver of the strip area; onResize → TermResize with cols/rows from fit; write event data into xterm (b64 decode); user input (onTERM KeyData) → TermInput. xterm theme (bg --bg-editor, JetBrains fonts, 13px), cursor blink off.
- [ ] Workspace: bottom strip (when not collapsed) renders tabs row (name = shell name + #n) + active TerminalPanel; plus "+" button spawns a terminal for the ACTIVE project.
- [ ] Gate: build + tsc. commit `feat(terminal): xterm.js panels wired to pty sessions`.

### Task 4: Acceptance + backlog

- [ ] Manual: open terminal, `ls`, `go build ./...` colored output; resize panel → reflow; ⌘J collapse/expand keeps scrollback; second tab per project; close/shell exit removes tab; two projects, two terminals; agent Terminal still placeholder ONLY IF xterm broken — verify go vet/test/build.
- [ ] backlog phase 5 ✅; commit docs.
