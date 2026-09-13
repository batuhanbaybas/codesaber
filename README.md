# CodeSaber ⚔️

> It's like a lightsaber, but for developers.

CodeSaber is a lightweight, AI-native IDE built with **Wails v3 + Go + React + CodeMirror 6**. A modular monolith Go backend powers the editor, git, terminal, and AI agent engines — one tool to ignite your workflow.

<!-- TODO: hero screenshot here -->
<!-- ![CodeSaber hero](docs/screenshots/hero.png) -->

## ✨ Features

### 🗂 Editor Core
- **Multi-project workspace** — add/remove projects in the sidebar, each with its own state namespace
- **Full editor experience** — per-project tabs, syntax highlighting (Go, TypeScript, CSS, HTML, JSON, Markdown with preview), minimap, bracket pair colorization, breadcrumbs
- **Welcome window** — recent projects with branch + last-used info
- **Atomic save + external-change reconciliation** — auto-reload or banner when files change outside
- **Quick-open (`⌘P`)** — fuzzy file search
- **Collapsible, drag-resizable panels** (`⌘B` sidebar / `⌘J` terminal / `⌘D` dock) with persisted sizes

### 🔀 Git Panel
- Real-time status via **fsnotify** — never press refresh again
- Stage/unstage per file or per hunk, with `+/−` line stats
- Commit & Push workflow with **AI-generated commit messages**
- Inline diff viewer, branch switcher, History tab with recent commits
- Upstream sync indicators (`↑↓`), Fetch/Pull/Push actions

### 🤖 ACP AI Agent
Built on the **Agent Client Protocol (ACP)** — bring your own agent harness:
- **opencode** and **Claude Code** supported (verified), more to come
- Streaming chat, tool-call display, per-project sessions persisted across restarts
- **Side-by-side diff review** — accept or reject agent file edits and inline prompt proposals
- Crash-safe: harness dies → auto-respawn, chat survives
- **`⌘K` inline edit-at-cursor** and prompt history (↑/↓ recall)

### 🧠 Language Intelligence
- **gopls-powered LSP** — go-to-definition, hover, diagnostics gutter, document symbols
- Framework is server-agnostic; TS/Rust/Java grammar configs in the roadmap
- **`⌘T` symbol search** — web-tree-sitter (WASM) symbol index per project, invalidated on save/filesystem change

### 💻 Real Terminal
- True PTY sessions (`creack/pty`) per tab, `cwd` = project root
- **xterm.js** frontend with streaming over an event channel and resize propagation

### ⚙️ Performance & Polish
- **Perf HUD (`⌘⇧H`)** — live RSS memory, per-engine latency counters, terminal count, error stats
- Custom macOS titlebar with traffic-light inset
- Status bar: cursor position (Ln/Col), diagnostic counters, indentation, encoding, language
- **Settings UI** — editor/terminal/search preferences, persisted
- Engine isolation: goroutines with panic recovery and restart scaffolding

## 🚀 Usage Examples

**Open a project**
```
wails3 dev          # development mode with hot-reload
wails3 build        # production build
```

**Daily workflow cheat sheet**

| Action | Shortcut |
|---|---|
| Quick-open file | `⌘P` |
| Symbol search | `⌘T` (`sym:` prefix in quick-open) |
| Command palette | `⌘⇧P` |
| Inline AI edit | `⌘K` |
| Toggle sidebar / terminal / dock | `⌘B` / `⌘J` / `⌘D` |
| Perf HUD | `⌘⇧H` |
| Close tab | `⌘W` |

**Ask the agent to change code, then review:**
1. Open the Agent tab and chat with opencode or Claude Code
2. Proposed edits appear as diff cards — accept or reject side-by-side
3. The AI commit message generator picks it up for your Git panel commit

**Ship faster with git + AI:**
1. Git tab shows live changes the moment you save
2. Stage files, click **Generate commit message** (AI, via your running harness)
3. Commit & Push — done

## 🗺 Roadmap

- Light "Milk" theme
- tree-sitter text search replacing index walk
- SSH remote development · DAP debugging · more LSP servers
- New Project… / Clone Repo… on the welcome screen
- Multi-window per project, detachable agent window

See [`docs/backlog.md`](docs/backlog.md) for the full backlog and status.

## 🏗 Development

```
wails3 task generate:bindings   # regenerate TS bindings (always -ts!)
wails3 dev
```

> Bindings note: always use `wails3 task generate:bindings` — plain `wails3 generate bindings` emits `.js` and clobbers the committed `.ts` bindings.

## 📜 License

<!-- TODO: add license -->
