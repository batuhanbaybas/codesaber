# aide — Personal IDE Design

Date: 2026-09-12
Status: Approved design (pre-implementation spec)

## Vision

A personal, resource-lean software development IDE: fast start, low memory, macOS-native look with JetBrains-quality organization. Code editor + git panel + AI agent harness integration (ACP) + tree-sitter indexing/search + minimal Go LSP + minimal terminal. Swap AI harnesses (opencode, Claude Code, others) without losing the IDE experience.

## Decisions

- **Stack:** Go + Wails v3 (single binary, webview UI)
- **UI:** React + Tailwind in the webview; CodeMirror 6 as the editor component
- **Theme:** Dark (Darcula-modern, softened) default; "Milk" light (JetBrains New UI-flavored) as switchable second theme. SF Pro / system fonts, native-feeling spacing, custom titlebar with macOS traffic lights
- **Isolation:** modular monolith; each backend engine runs in supervised goroutines with panic recovery and restart; ACP harnesses and LSP servers are external child processes by protocol
- **Projects:** multi-project in one window (Cursor/Zed-style left-pane sections); JetBrains-style dedicated welcome window with recents; one window per workspace session, additional projects added in place
- **Languages:** first-class Go + TypeScript/React (+HTML/CSS/JSON grammars); Rust and Java added later via tree-sitter grammar config; MVP ships gopls LSP only (framework extensible)

## Architecture

One Go binary, modular monolith with bounded engines behind interfaces:

```
backend/
  project/  # workspace registry: per-project state namespace, recents
  git/      # go-git: status/diff/branch/commit/log; fsnotify watcher -> events
  editor/   # buffer service: read/write/save (temp+rename), dirty state truth
  index/    # tree-sitter symbol index + search, per project
  lsp/      # LSP client framework; gopls provider (one server per project)
  acp/      # ACP client: spawn/attach harnesses (stdio JSON-RPC), session per project
  terminal/ # PTY sessions (creack/pty), cwd = project root
  events/   # typed pub-sub; bridges to Wails events for all windows
```

- Every engine: own goroutine(s), `defer recover()` at engine boundary, supervisor restarts on panic and emits `engine.status` so UI panels degrade gracefully.
- Per-project engine instances, keyed by project ID. No global mutable state.
- Session/recents state: `~/Library/Application Support/aide/`.

### Windows

1. **Welcome window** — shown at launch (no projects) and on demand: actions rail (New/Open/Clone) + recent projects list (name, path, branch, last opened).
2. **Workspace window** — the IDE: left = activity rail + unified sidebar (project sections, each with file tree); center = CodeMirror editor tabs; right dock = Agent / Git / tabs; bottom strip = terminal. Command palette (⌘⇧P) skeleton in MVP.

### Component map (UI)

- `ProjectProvider` — one per project section; owns subscriptions namespaced by project ID.
- `FileTreePanel`, `EditorTabs`, `AgentPanel`, `GitPanel`, `TerminalStrip`, `StatusBar` (branch, engine health pills, dirty state).

## MVP Phases (each phase ends working)

1. **Editor core** — project registry + welcome window, file tree, tabs, CodeMirror 6 with tree-sitter highlighting (Go, TS/React, HTML, CSS, JSON), save + fsnotify reconcile, command palette skeleton, custom titlebar.
2. **Git panel** — status/branch/diff/stage/commit, live fsnotify updates, gutter diff markers, diff viewer.
3. **ACP agent** — harness picker (opencode first; Claude Code verified), per-project streaming chat, tool-call and diff display with accept/reject, session persistence; harness crash = respawn without losing chat.
4. **gopls LSP** — LSP client framework, go-to-definition, hover, diagnostics, document symbols for Go.
5. **Terminal** — bottom strip PTY tabs, cwd = project root.
6. **Polish** — welcome screen refinement, theme system completion, JetBrains-grade spacing/typography pass, keymap review.

Explicitly out of MVP: TS/Rust/Java LSPs (config entries later), debugging, extensions marketplace, multi-monitor detach, full Java support.

## Data flow

- **Events (Backend → UI), project-namespaced:** `git.status`, `git.diff`, `acp.msg`, `acp.diff`, `lsp.diag`, `fs.change`, `term.data`, `engine.status`, `project.updated`.
- **RPC (UI → Backend, typed):** `project.open/list/remove`, `buffer.read/save`, `git.stage/unstage/commit/checkoutBranch`, `acp.start/send/acceptDiff/rejectDiff/stop`, `lsp.definition/hover/symbols`, `term.start/write/resize/stop`, `welcome.recent/list/forget`.
- **Buffers** are the single source of truth for dirty/content state; git and agent diffs reconcile against buffer content before display and before applying AI edits (no clobbering).

## Error handling

- Engine panic → supervisor restart, `engine.status(down)` → panel shows "restarting…" state.
- Child process crash (agent, gopls) → teardown, respawn, status pill; agent chat history preserved.
- ACP protocol errors → error bubble in chat; session stays usable.
- Saves: temp file + rename; git ops pure go-git (no shell parsing); failed AI edits leave buffers untouched.
- All panics funnel through typed event dispatcher safety net; unhandled-panic audits in CI.

## Testing

- Engines are pure Go interfaces → table-driven unit tests.
- Git layer: fixture repos; ACP: recorded JSON-RPC traces; LSP: fake in-process server.
- Manual acceptance checklist per phase (run together) — no E2E framework in MVP.

## Non-goals / risks

- Wails v3 is pre-1.0: API churn risk; mitigation: isolate Wails imports to a thin adapter package.
- ACP harness differences (opencode vs Claude Code capabilities): capability negotiation logic kept in `acp` engine, behind interface.
- tree-sitter in webview (WASM) vs CGo: decide during Phase 1 planning based on index/search needs; interface-first so it can swap.
