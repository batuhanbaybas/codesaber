# Phase 1 Acceptance Report

Date: 2026-09-12 · branch `feat/phase1-editor-core`

| Check | Result |
|---|---|
| Welcome window opens, traffic lights on custom titlebar, draggable | ✅ |
| Open Folder → project in sidebar with tree | ✅ |
| Second project adds second section | ✅ |
| Syntax highlighting Go + TS (colors need polish — legacy Go mode vs theme) | ✅ (noted) |
| ⌘S saves; no `.aide-tmp` leftovers | ✅ |
| External change: dirty tab shows "File changed on disk" banner | ✅ |
| Remove project from sidebar | ✅ |
| Relaunch → recents with last-used | ✅ |
| ⌘⇧P palette opens/filters/closes | ✅ |
| RSS < 120 MB | ✅ |

## Bugs found & fixed during acceptance

- **Workspace window never visible after runtime `NewWithOptions`** — beta.20 shows webview windows only after `WebViewDidFinishNavigation`; fixed with explicit `w.Show()` in `adapter.EnsureWorkspaceWindow` (commit `fix(adapter): explicitly Show runtime-created workspace window`).

## Known gaps (deferred)

- Stale recent click / opening already-open project: silent failure (no error surface).
- Legacy-mode Go highlighting theme mismatch — poor color contrast; address in theme polish.
- ReadFile/SaveFile have no path containment (documented MVP trade-off).
- Palette commands are Open Folder / Remove Project / About (plan variants skipped as YAGNI).

## Follow-up requests from user (accepted for next plan)

1. Collapsible/closable panels: sidebar, Agent/Git dock, terminal strip.
2. ⌘P quick-open file popup (VS Code-style fuzzy file search).
