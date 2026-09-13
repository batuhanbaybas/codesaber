# Agent Tab Modernization — Design

Date: 2026-09-13
Status: Approved (user confirmed layout, unified timeline, all-SQLite storage, full session history)

## Goal

Modernize the Agent tab to match the Git tab's design language and developer
experience: GitPanel-sibling visuals, interleaved chat/tool timeline, and full
multi-session management with persisted history.

## Decisions

- **Visual direction:** GitPanel sibling — same cards, segmented tabs, chips,
  badges, accent gradient buttons as `GitPanel.tsx`.
- **Tool cards:** collapsed one-line rows by default; click to expand output.
- **Session management:** full session history (list, open, rename, delete,
  auto-titles), persisted per project.
- **Storage:** all SQLite (`modernc.org/sqlite`, pure Go, no CGO).
- **State model:** unified timeline replacing separate `messages[]`/`tools[]`.

## Backend

### Storage — `backend/agentstore` (rewrite to SQLite)

Single DB at `<user-config-dir>/codesaber/chats/agent.db`.

Tables:

```sql
CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  title      TEXT NOT NULL,
  created    TEXT NOT NULL,  -- RFC3339
  updated    TEXT NOT NULL
);
CREATE INDEX idx_sessions_proj ON sessions(project_id, updated DESC);

CREATE TABLE entries (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  seq        INTEGER NOT NULL,
  role       TEXT NOT NULL,   -- user | agent | system
  kind       TEXT NOT NULL,   -- text | tool | error | chunk
  tool_id    TEXT,
  title      TEXT,
  status     TEXT,
  text       TEXT NOT NULL,
  when       TEXT NOT NULL
);
CREATE INDEX idx_entries_session ON entries(session_id, seq);
```

- `kind` validated to the existing set (`text`, `tool`, `error`) plus `chunk`.
- Tool updates **upsert** by `(session_id, tool_id)`: a status/title/content
  update replaces the existing row rather than appending.
- API: `NewStore()`, `Append(sessionID, projectID, Entry)`, `Read(sessionID)`,
  `ListSessions(projectID) ([]SessionMeta, error)`, `DeleteSession(sessionID)`,
  `RenameSession(sessionID, title)`, `Close()`.
- `SessionMeta = {ID, ProjectID, Title, Created, Updated, MessageCount}`.
- `MigrateJSONL()` — on first open, import legacy flat
  `<projectID>.jsonl` files (one session each, titled from first user text,
  id = legacy project id + `-legacy`), then rename source files `.imported`.
- Auto-title: when a session's first user text entry is appended, derive title
  (first line, ≤60 chars) if the session title is still empty.

### ACP / app RPCs

- `ACPStart(projectID, harness)` creates a DB session record keyed to the
  harness `session/new` id when available (fallback: generated uuid).
- New bindings: `ACPSessions(projectID)`, `ACPOpenSession(projectID, sessionID)`,
  `ACPDeleteSession(projectID, sessionID)`, `ACPRenameSession(sessionID, title)`.
- `ACPOpenSession` loads the transcript into the UI (`acp.transcript` event now
  carries `sessionID`); if the harness is idle it stays running and only the
  UI context switches.
- Existing `ACPLoadTranscript` keeps working (reads active/legacy session) so
  `generateCommit` and prompt-history seeding are untouched in behavior.
- Persisted entries now include tool cards (role `agent`, kind `tool`) so
  history restores interleaved timelines.

## Frontend

### State — `state/agent.tsx`

- `AgentProjectState.timeline: TimelineItem[]` replaces `messages` + `tools`.
  - `TimelineItem`: `{type:'message', id, role, text, kind}` |
    `{type:'tool', toolCallId, title, kind, status, content}` |
    `{type:'permission', …PendingPermission}`.
- Event handling:
  - `acp.msg` chunk merge appends to tail message item (same invariant as
    today: first chunk creates, later chunks extend the tail agent message).
  - `acp.tool` upserts the tool item in place (order preserved by first
    appearance).
  - `acp.permission` appends a permission item; resolved permissions removed.
  - `acp.transcript` replaces the whole timeline from persisted entries
    (messages + tool entries interleaved by `seq`).
- New state: `sessions: Record<projectID, SessionMeta[]>`,
  `activeSessionId: Record<projectID, string | null>`.
- New actions: `listSessions`, `openSession`, `deleteSession`, `renameSession`.
- `generateCommit`, prompt-history seeding (`↑` recall) keep behavior.

### UI — `AgentPanel.tsx` (rebuilt)

- **Harness card row** (branch-card idiom): harness name + "ACP agent
  protocol" subtitle + chevron → dropdown picker when stopped; right-side
  status pill: thinking (amber pulse) / Stop (red) when running, or picker
  Start button when stopped.
- **Small actions row:** `+ New Session`, `Clear transcript`, and token/turn
  counter if the harness reports it (hidden otherwise).
- **Segmented tabs** (Git styling): `Chat (n)` / `History (n)` /
  `Logs (—, disabled "coming soon")`.
- **Chat tab:** timeline interleaved — user bubble right (accent tint),
  agent/system text left, tool call cards as one-line rows (chevron, glyph,
  title with dimmed path segment, status chip: pending amber pulse, done
  green, failed red) expanding to output `<pre>`; permission cards inline
  (amber border, Create/Edit header + path + `+n/−n` counts, MiniDiff,
  Reject / Accept / Accept-all-this-session).
- **History tab:** session list like the git commit log — dot, title,
  `timeAgo · n messages` meta; expanded row shows actions Open / Rename
  (inline) / Delete (ConfirmDialog).
- **Composer:** auto-grow textarea, `↑/↓` history recall (existing),
  placeholder `Ask the agent…`, char counter `n / 8000`, split Send button
  (Send ▾ with "Send & queue" placeholder item, disabled until backend
  supports queueing). Enter sends, Shift+Enter newline.
- **Auto-scroll:** stick to bottom while streaming unless the user scrolled
  up; show "↓ jump to latest" pill when detached.
- **Empty states:** styled like git empty sections ("No messages yet — start
  a harness to chat").

## Error handling

- Store failures surface via existing error-banner pattern (red card at top).
- `ACPDeleteSession` on the active session: backend refuses while harness is
  running that session; UI shows inline error.
- SQLite open failure falls back to in-memory DB with a system message
  warning (history won't persist).

## Testing

- Go: store tests (append/read roundtrip, upsert-by-tool-id, list ordering,
  rename, delete, JSONL migration, in-memory fallback) in `backend/agentstore`.
- Frontend vitest: timeline reducer (chunk merge, tool upsert, permission
  stack, transcript rebuild), session actions.
- Manual: run app, start opencode harness, verify interleaved timeline,
  permission cards, session switch/restore.

## Out of scope

- Harness-side `session/load` reconnect (UI restores context only).
- Prompt queueing while thinking (Send ▾ placeholder only).
- Logs tab.
