# Phase 4 — gopls LSP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** LSP client framework + gopls provider: go-to-definition, hover, diagnostics in gutter, document symbols for Go files.

**Architecture:** `backend/lsp` engine: generic LSP-over-stdio client (JSON-RPC, reuse the codec knowledge from acp but a separate lean codec to avoid coupling), one gopls process per project, spawned lazily on first Go-file interaction. Text sync via `lsp: didOpen/didChange/didSave` driven from frontend editor events (full document sync — TextDocumentSyncKind(1)). Facade RPCs: definition/hover/symbols/diagnostics; diagnostics stream down via `lsp.diag` events. Frontend: CodeMirror gutter markers + hover tooltips + ⌘-click definition.

**Tech Stack:** gopls (assumed preinstalled; harness exec.LookPath check), stdlib JSON-RPC over stdio.

Standing requirement: update `docs/backlog.md` at phase end.

---

## File Structure

```
backend/lsp/lsp.go          # protocol types: Position, Range, Location, Hover, Diagnostic, SymbolInformation/DocumentSymbol subset
backend/lsp/client.go       # stdio JSON-RPC client: Initialize, didOpen/didChange, request/response plumbing, server→client notification handling (publishDiagnostics)
backend/lsp/provider.go     # GoplsProvider glue: per-project process mgmt + config
backend/lsp/client_test.go  # TDD against a fake LSP server (os.Pipe pairs, same pattern as acp tests)
backend/app.go              # facade RPCs: LSPDefinition, LSPHover, LSPDidOpen(LSPDidChangeVersion, etc), doc symbols; editor buffers registry LSP buffer state per project ui open
frontend/src/components/Editor.tsx  # gutter diag markers, hover render, ⌘-click goto, didOpen/didChange hooks over RPC
frontend/src/components/GotoDef.tsx # modal-less: click → jump (open file tab + reveal line)
frontend/src/lsp.ts         # thin typed wrapper over bindings
```

---

### Task 1: LSP protocol types + stdio client core (TDD)

**Files:** `backend/lsp/lsp.go`, `backend/lsp/client.go`, `backend/lsp/client_test.go`.

- [ ] **Step 1: TDD the client plumbing独立 (pipe-pairs, no exec):** fake server goroutine reading lines; check: initialize initialize request result cap检查; didOpen notification framing; publishDiagnostics notification; definition request → result Location(s); hover → Marks; shutdown/exit sequence tests. Reuse patterns from backend/acp/connection_test.go.
- [ ] **Step 2: lsp.go types** — LSP basic: Position{line,character}, Range, Location{uri,range}, Diagnostic{range,severity,message,source}, Hover{contents}, DocumentSymbol {name,kind,range,selectionRange,children[]}. UTF-16 code units caution: Go rows are byte offsets; UI must convert (JS side does code-unit offsets naturally — note in editor bridge: CodeMirror uses UTF-16 code units = LSP character offsets; document the match).
- [ ] **Step 3: client.go** — initialize handshake (rootUri, capabilities: textDocument{definition{linkSupport:false}}); servers keys per doc URI; shut down + kill on Close; response map like acp; notification map: publishDiagnostics → handler.
- [ ] Gates/test: `go test -race ./backend/lsp/...`, `feat(lsp): stdio client + protocol types`.

### Task 2: Gopls provider (spawn per project + lifecycle)

**Files:** `backend/lsp/provider.go`, `backend/lsp/provider_test.go`.

- [ ] Step 1: Provider: per project — lazy Start() (exec.LookPath gopls); didOpen(projectID, path, version, text); didChange(path, version, text full sync); didSave/didClose; Definition(path, line, col) ([]Location); Hover(path, line, col) (*Hover); DocumentSymbols(path) ([]DocumentSymbol); all guarded per project (mutex map[projectID]*Client); Stop(projectID); on RemoveProject stop gopls. Server-not-found: facade returns error with hint text "(install gopls)".
- [ ] Gate: tests with fake server (spawnable binary "gopls" not available in CI — Provider test spin a fake binary exactly like acp's pipe pattern using a subprocess? subprocess fake: small built-in test uses `go build` of testdata/lspfake or direct os/exec of `sh -c` echo server — acceptable). Gate `feat(lsp): gopls provider`.

### Task 3: Facade wiring

**Files:** `backend/app.go`, `backend/app_test.go`, regenerate bindings.

- [ ] RPCs: `LSPDidOpen/DidSave/DidChange/DidClose(projectID, path, version, content)`, `LSPDefinition(projectID, path, line, col)`, `LSPHover(...)`, `LSPSymbols(...)`, `LSPDiagnosticsCurrent(projectID)`. Events: `lsp.diag` {projectId, path, diagnostics} (throttled per project similar to git.status: 200ms), `lsp.state` {projectId, running}.
- [ ] bridge to forwardWatcher? No — didOpen flows from frontend editor openHouse; tests with fake: give a Go fixture project (no Go module needed for symbol-less server to start; document that gopls requires go.mod — if missing emit lsp.state {running:false,reason:"no go.mod"}).
- [ ] Gate: `feat(app): lsp facade + diagnostics events`.

### Task 4: Frontend integration

**Files:** `frontend/src/components/Editor.tsx`, `frontend/src/state/tabs.tsx`, new `frontend/src/components/GutterDiagnostics.tsx`, `frontend/src/components/HoverTooltip.tsx`, modified `frontend/App.css`-zk style.

- [ ] Step 1: CodeMirror extension wiring: `lspGutter` (diag markers: severity → dot color: error red, warn amber, info blue; click → tooltip text w/ message + source), hover (⌘ hover shows hover content in CodeMirror hover tooltip), gotoDef binding Mod-click (requires LSP call; on returned uri/path → openFile + scrollTo line:col).
- [ ] Step 2: didOpen lifecycle: on tab activate of a .go file → LSPDidOpen (initial content) w/ increasing per-version seq; on content dispatch → write didDidChange, debounced 300ms; save → didSave.
- [ ] Gate: `feat(editor): go integration — diagnostics gutter/hover/goto`.

### Task 5: Acceptance + backlog

- [ ] Manual: open a Go project (mentoring has none — create a disposable Go module in temp: cmd/main.go with a func + comment hover; verify diagnostics appear when typing a syntax error; hover gives docs; ⌘click goes to definition; symbols list in palette (optional: skip palette; MVP skip).
- [ ] backlog: mark phase 4 ✅; commit docs.

## Self-review
- Spec coverage: gopls ✓ (go-to-def/hover/diag/symbols), servers as config for later languages noted in provider docs.
- Type consistency: LSP positions in UTF-16 code units match CodeMirror's ch offset; facade names align with lsp.* event namespacing.
