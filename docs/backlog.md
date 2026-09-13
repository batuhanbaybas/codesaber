# aide IDE — Feature Backlog

Last updated: 2026-09-12 · Plans live in `docs/superpowers/plans/`
Legend: ✅ done · 🚧 in progress · ⬜ todo · 📌 deferred (was requested, scheduled later)

## Phase 1 — Editor Core ✅ (shipped, acceptance in plans/2026-09-12-phase1-acceptance.md)

- ✅ Wails v3 + Go + React + CodeMirror 6 stack, modular monolith backend
- ✅ Multi-project sidebar (add/remove/active), per-project state namespace
- ✅ Welcome window (recents with branch + last-used, forget)
- ✅ File tree (lazy expansion, >200 child truncation)
- ✅ Per-project editor tabs, CodeMirror highlighting (Go/TS/CSS/HTML/JSON)
- ✅ Save (atomic) + external-change reconcile (auto-reload / banner)
- ✅ Quick-open ⌘P (fuzzy file search via IndexFiles RPC)
- ✅ Collapsible panels (⌘B sidebar / ⌘J terminal / ⌘D dock, persisted)
- ✅ Drag-resizable panels with persisted sizes
- ✅ Command palette ⌘⇧P (skeleton), engine status pills
- ✅ Custom titlebar with macOS traffic-light inset
- ✅ Engine isolation (goroutines, panic recovery/restart scaffolding)
- 🐛 Fixed: runtime-created windows invisible without explicit Show() (beta.20)

## Phase 2 — Git Panel ✅ (2026-09-12)

- ✅ go-git engine: status/stage/unstage, branch listing/switching, diff, log
- ✅ Live fsnotify-driven git status refresh
- ✅ Git tab: changed files (U/M/A colors, staged/unstaged groups)
- ✅ Stage/unstage per file (click, group actions)
- ✅ Commit message box + commit
- ✅ Branch pill in titlebar wired to real branch, branch switcher
- ✅ Inline diff viewer (selected file, staged vs unstaged)
- 📌 Backdrop: feed diffs to Phase 3 ACP accept/reject UI

## Phase 3 — ACP Agent ✅ (2026-09-12)

- ✅ ACP client engine (stdio JSON-RPC), harness registry (opencode first, Claude Code verified)
- ✅ Agent tab chat: streaming messages, session per project, persistence across restarts
- ✅ Tool-call display; diff proposals with accept/reject (reuses Phase 2 diffs)
- ✅ Harness crash → respawn, chat preserved; engine status pills wired

## Phase 4 — gopls LSP ✅ (2026-09-12)

- ✅ LSP client framework, one gopls server per project
- ✅ Go-to-definition, hover, diagnostics gutter, document symbols
- 📌 Config entries for future TS/Rust/Java servers (framework ready, Phase 6)

## Phase 5 — Real Terminal ✅ (2026-09-12)

- ✅ creack/pty sessions per tab, cwd = project root
- ✅ xterm.js frontend, streaming over ⌘P-style event channel, resize propagation
- ✅ Multiple tabs

## Phase 6 — Design & Polish 🚧 (visual pass done 2026-09-12)

- ✅ Syntax theme pass (readable JetBrains-dark across lezer + legacy modes) · 📌 light theme ("Milk") still pending
- ⬜ tree-sitter (WASM) symbol index + text search replacing IndexFiles walk
- ⬜ Error surfacing for stale recents / duplicate project opens
- ⬜ Welcome screen refinement, keymap review

## Deferred park

- ⬜ New Project… / Clone Repo… on welcome screen (Phase 2+ target)
- ⬜ Rust/Java language support (grammar config)
- ⬜ Multi-window per project, detachable agent window
- ⬜ ReadFile/SaveFile path containment (tracked MVP trade-off)

## Phase 6.5 — Design polish round (2026-09-12, per user mockups)

- ✅ Git panel redesign: cards/sections, language chips, per-file +/− stats + binary sizes, branch box w/ upstream sync↑↓, Fetch/Push, History tab w/ recent commits, AI commit-message gen (via running harness), split Commit & Push
- ✅ Activity rail icons (files/search/git+badge/terminal/settings/apps)
- ✅ Titlebar: breadcrumb (project › branch › file), ⌘P search capsule, panel toggle icons
- ✅ File-type icons in tree + editor tabs; dock tab icons w/ badge
- ✅ Minimap column; readable dark syntax theme
- ✅ Status bar: Ln/Col cursor, diag counters (● errors/▲ warnings), Spaces 2, UTF-8, language label, aide v0.2.4
- 📌 Pending from spec: light "Milk" theme, tree-sitter search/index, welcome-screen refinements, error surfacing for stale recents
## Phase 7 — Night build (2026-09-12, next-five recommendations)

- ✅ 7.1 AI edit diff review: side-by-side accept/reject on agent fs-writes (diff preview inside permission card; reject = deny + keep buffer)
- ✅ 7.2 ⌘T symbol search: web-tree-sitter (WASM) symbol index per project (go/ts/js grammars), quick-open integration ("sym:" prefix + ⌘T), index invalidated on save/fs change
- ✅ 7.3 Search refinements: case-sensitive toggle, include/exclude globs
- ✅ 7.4 Editor niceties: bracket pair colorization toggle, scrollPastEnd, breadcrumbs-under-tabs showing file path
- ✅ 7.5 Perf HUD ⌘⇧H: live RSS, per-engine latency counters, terminal count, err counts
- ✅ Compare doc: docs/ide-comparison-2026-09-12.md (vs Cursor/Trae/Zed, must-have gap list)

## Phase 8 — Feature batch (2026-09-13)
- 🚧 8.1 Markdown preview toggle (top-right button on .md files), ⌘W closes tab (not window), project-section folder collapse
- 🚧 8.2 Settings UI (gear rail icon opens dock tab; toggles + editor/terminal/search settings persisted)
- 🚧 8.3 AI UX: prompt history (up/down recall), inline ⌘K edit-at-cursor via harness
- 🚧 8.4 Hunk-level git staging (post-file row expansion; git apply --cached via binary when available)
- ⬜ Parked: light theme, SSH remote, DAP debug, more LSPs (see compare doc)
