# aide vs the Field — Cursor / Trae / Zed Comparison

Date: 2026-09-12 (after Phase 7 night build) · Status snapshot: `bin/aide` v0.2.4

## What aide has today (one line each)

Multi-project workspace · editable multi-language editor w/ minimal LSP (gopls) · Git panel (status/stage/commit/branch/diff/history/push) · ACP agent harnesses (opencode/Claude/Gemini, streaming chat, diff-review cards w/ accept-reject) · real PTY terminals · ⌘P files · ⌘T symbols (tree-sitter WASM index) · ⌘⇧F workspace search (case/globs/regex) · perf HUD · low resource profile.

## Comparison

| Capability | aide | Cursor | Trae | Zed |
|---|---|---|---|---|
| AI harness freedom (ACP, swap w/o losing IDE) | ✅ first-class | ❌ Cursor-only | ❌ Trae-only | △ (_extensions ecosystem growing through ACP from Zed's own incubation) |
| Per-project agent sessions + diff review cards | ✅ | ✅ (tab-only, no separate harness) | ✅ | △ (agent panel via ACP; files review similar) |
| Editor perf on huge files | ✅ (CM6, webview is lighter than Electron) | △ (Electron) | △ (Electron+forks) | ✅✅ (native GPUI — the bar) |
| LSP (settings per server, multi-language) | △ (gopls ready; TS server + config UI missing) | ✅ | ✅ | ✅✅ |
| Real terminal, per-project, tabs | ✅ | ✅ | ✅ | ✅ |
| Workspace search w/ globs + regex + case | ✅ (plain scanner) | ✅ | ✅ | ✅ (fsp) |
| Debugging (DAP: breakpoints, repl, watch) | ❌ | ✅ | ✅ | △ (debugger is bare via dapfa) |
| Remote (ssh/container) development | ❌ | ✅ | ❌ | △ (Early) |
| Command palette + keymap coverage | ✅ (skeleton; ⌘ tab icons, facer) | ✅✅ | ✅ | ✅✅ |
| Language: file icons + minimap + breadcrumbs | ✅ | ✅ | ✅ | ✅ |
| Startup + memory budget (public promise) | ✅ (RSS <150MB goal; hud to audit) | ❌ (1GB+ observed) | ❌ | ✅ |
| Git: hunk-level staging, AI-aware | △ (file-level; agent diff cards exist) | ✅ | △ | △ |
| Marketplace ecosystem | ❌ | ✅✅ | ✅ | △ (growing) |
| Linux/Windows support | ❌ (macOS MVP; Wails3 cross-compiles later) | ✅ | ✅ | ✅ |

## Must-have gaps to reach "developer top choice" (priority order)

1. **More languages with full editing support** — at minimum TS Doc-Degree editor LSP (typescript-language-server), Rust (rust-analyzer). The harness-agnostic story sells only if editors stay superb across our polyglot targets.
2. **DAP debugging** (breakpoints, step, evaluate, callstack) — non-negotiable for a daily driver. Start: delve for Go via DAP adapter.
3. **Remote development** (SSHFolders-remote or devcontainers per-launch): cursor raiders rely on it; implement at least "open folder over SSH with rcp/LSP buffered handling" — pan P2.
4. **Settings UI + sync** (JSON schema editor + theme sync + per-project overrides).
5. **Hunk-level git staging** and agent-aware commit haven+multi; "stage only lines agent changed" — Cursor doesn't do this well; genuine differentiator.
6. **AI UX polish to Cursor parity**: prompt history searchable, agent todos/plan view, inline-edit-at-cursor via harness (⌘K inline edits). Inline ⌘K is the single most-missed cursor muscle-memory.
7. **Marketplace-light extensions**: implement "commands" contributed by(ACP agents (safe: every agent already speaks ACP) — cheap, no infra; plus local JS plugins later.
8. **Cross-platform ship**: Wails v3 supports linux/windows — CI matrix + fsnotify paths; unlock Linux users (huge share of the "fast, light" crowd).
9. **Tests-on-save / task surface** (run `go test ./...` on save, terminal default commands per project type).
10. **Reliability of agent loop**: crash-resume UI, harness quota/token metering, prompt cache (session reuse).

## Deliberate non-goals (stay lean)
Marketplace plugins infra, built-in AI model hosting (stay harness-agnostic by design), Electron-class budgets, sync-services.

## Bennchmark method suggestion (next week's hard data)
- Cold start: app launch → editor ready (perfcounter in-app; target <900ms)
- RSS ideel: editor idle 5 files vs 20 files (Activity Monitor; target <120MB)
- 100MB repo: search scan wall time (target <3s all cores)
- 10MB single file: open + scroll smoothness (frame drops)
- Symbol index: 2000-file parse wall time (target <10s)
- Agent diff roundtrip: prompt → apply permission card accept latency
Record in bench/README and publish; update each release.
