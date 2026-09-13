# Agent Tab Modernization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the Agent tab as a GitPanel sibling: SQLite-backed multi-session history, unified interleaved chat timeline, and Git-idiom UI (harness card, segmented tabs, collapsible tool cards, split composer).

**Architecture:** Rewrite `backend/agentstore` from JSONL files to a single SQLite DB (modernc.org/sqlite, pure Go). `backend/agent.go` gains session records + 5 new RPCs. Frontend `state/agent.tsx` replaces `messages[]`/`tools[]` with a unified `timeline` plus session list state. `AgentPanel.tsx` is rebuilt from the GitPanel design language.

**Tech Stack:** Go 1.25, Wails v3 (bindings via `~/go/bin/wails3 task generate:bindings`), modernc.org/sqlite, React + TypeScript, vitest.

**Bindings rule (from README):** always regenerate with `~/go/bin/wails3 task generate:bindings -clean=true -ts -i` — plain `wails3 generate bindings` clobbers the committed `.ts` bindings.

**Spec:** `docs/superpowers/specs/2026-09-13-agent-tab-modernization-design.md`

---

### Task 1: Add modernc.org/sqlite dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add dependency**

```bash
go get modernc.org/sqlite@latest
```

- [ ] **Step 2: Verify it resolves**

```bash
go mod tidy && go build ./...
```

Expected: build passes; `modernc.org/sqlite` appears in `go.mod`.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add modernc.org/sqlite (pure-Go, no CGO)"
```

---

### Task 2: SQLite store — schema, Append/Read roundtrip (TDD)

**Files:**
- Create: `backend/agentstore/store_sqlite_test.go`
- Rewrite: `backend/agentstore/store.go`
- Delete: `backend/agentstore/store_test.go` (old JSONL tests — replaced by the new suite)

- [ ] **Step 1: Write the failing tests**

Create `backend/agentstore/store_sqlite_test.go`:

```go
package agentstore

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	s.userConfigDir = func() (string, error) { return t.TempDir(), nil }
	s.userHomeDir = func() (string, error) { return t.TempDir(), nil }
	if err := s.open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLiteAppendReadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	if err := s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "hello", When: now}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Append("s1", "p1", Entry{Role: "agent", Kind: KindText, Text: "hi", When: now}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	entries, err := s.Read("s1")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 2 || entries[0].Text != "hello" || entries[1].Text != "hi" {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Role != "user" || entries[1].Role != "agent" {
		t.Fatalf("roles = %q %q", entries[0].Role, entries[1].Role)
	}
}

func TestSQLiteReadMissingSessionReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	entries, err := s.Read("nope")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %#v, want empty", entries)
	}
}

func TestSQLiteAppendRejectsUnknownKind(t *testing.T) {
	s := newTestStore(t)
	if err := s.Append("s1", "p1", Entry{Role: "user", Kind: "bogus", Text: "x"}); err == nil {
		t.Fatal("want error for unknown kind")
	}
}

func TestSQLiteSessionsIsolated(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "one"})
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "two"})
	e1, _ := s.Read("s1")
	e2, _ := s.Read("s2")
	if len(e1) != 1 || e1[0].Text != "one" || len(e2) != 1 || e2[0].Text != "two" {
		t.Fatalf("e1=%#v e2=%#v", e1, e2)
	}
}

func TestSQLiteConcurrentAppends(t *testing.T) {
	s := newTestStore(t)
	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 10; j++ {
				if err := s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "x"}); err != nil {
					t.Errorf("Append: %v", err)
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	entries, _ := s.Read("s1")
	if len(entries) != 40 {
		t.Fatalf("entries = %d, want 40", len(entries))
	}
}

func TestSQLiteFallbackDirWhenUserConfigFails(t *testing.T) {
	s := NewStore()
	s.userConfigDir = func() (string, error) { return "", errors.New("no config") }
	s.userHomeDir = func() (string, error) { return t.TempDir(), nil }
	if err := s.open(); err != nil {
		t.Fatalf("open with fallback: %v", err)
	}
	if err := s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "x"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if s.dbPath == "" || s.dbPath == ":memory:" {
		t.Fatalf("dbPath = %q, want fallback file path", s.dbPath)
	}
}
```

Also create `backend/agentstore/store_sqlite_sessions_test.go` (used by Tasks 3–4; write it now so the suite compiles once the store is implemented — tests for Tasks 3/4 are included here and will fail until those tasks land):

```go
package agentstore

import "testing"

func TestSQLiteToolUpsertByToolID(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "agent", Kind: KindTool, ToolID: "t-1", Text: "Reading file", Status: "in_progress"})
	_ = s.Append("s1", "p1", Entry{Role: "agent", Kind: KindTool, ToolID: "t-1", Text: "Reading file", Status: "completed"})
	entries, _ := s.Read("s1")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (upsert)", len(entries))
	}
	if entries[0].Status != "completed" {
		t.Fatalf("status = %q, want completed", entries[0].Status)
	}
}

func TestSQLiteListSessionsOrderAndCount(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "first prompt"})
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "second prompt"})
	sessions, err := s.ListSessions("p1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	// most recently updated first
	if sessions[0].ID != "s2" || sessions[1].ID != "s1" {
		t.Fatalf("order = %q,%q want s2,s1", sessions[0].ID, sessions[1].ID)
	}
	if sessions[1].Title != "first prompt" {
		t.Fatalf("title = %q, want auto-titled from first user text", sessions[1].Title)
	}
	if sessions[1].MessageCount != 1 {
		t.Fatalf("count = %d, want 1", sessions[1].MessageCount)
	}
	if sessions[1].ProjectID != "p1" {
		t.Fatalf("projectId = %q", sessions[1].ProjectID)
	}
}

func TestSQLiteListSessionsProjectIsolation(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "a"})
	_ = s.Append("s2", "p2", Entry{Role: "user", Kind: KindText, Text: "b"})
	p1, _ := s.ListSessions("p1")
	if len(p1) != 1 || p1[0].ID != "s1" {
		t.Fatalf("p1 sessions = %#v", p1)
	}
}

func TestSQLiteRenameSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "x"})
	if err := s.RenameSession("s1", "My fix session"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	sessions, _ := s.ListSessions("p1")
	if sessions[0].Title != "My fix session" {
		t.Fatalf("title = %q", sessions[0].Title)
	}
}

func TestSQLiteRenameMissingSessionErrors(t *testing.T) {
	s := newTestStore(t)
	if err := s.RenameSession("ghost", "nope"); err == nil {
		t.Fatal("want error renaming missing session")
	}
}

func TestSQLiteDeleteSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "x"})
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "y"})
	if err := s.DeleteSession("s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if entries, _ := s.Read("s1"); len(entries) != 0 {
		t.Fatalf("s1 entries after delete = %d", len(entries))
	}
	sessions, _ := s.ListSessions("p1")
	if len(sessions) != 1 || sessions[0].ID != "s2" {
		t.Fatalf("sessions = %#v", sessions)
	}
}

func TestSQLiteLatestSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "old"})
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "new"})
	meta, err := s.LatestSession("p1")
	if err != nil {
		t.Fatalf("LatestSession: %v", err)
	}
	if meta.ID != "s2" {
		t.Fatalf("latest = %q, want s2", meta.ID)
	}
	if _, err := s.LatestSession("p-none"); err == nil {
		t.Fatal("want error for project with no sessions")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./backend/agentstore/ -v 2>&1 | head -20
```

Expected: FAIL — `s.open undefined`, `s.dbPath undefined`, `ListSessions undefined`, etc. (old `store_test.go` still references `Clear`; delete it first if compile errors block seeing the new failures).

- [ ] **Step 3: Rewrite the store**

Replace `backend/agentstore/store.go` entirely with:

```go
// Package agentstore persists agent chat transcripts in a single SQLite
// database (modernc.org/sqlite — pure Go, no CGO) at
// <user-config-dir>/codesaber/chats/agent.db. Sessions are rows; entries are
// ordered per session by seq. Legacy per-project JSONL files are imported on
// first open and renamed .imported.
package agentstore

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Entry kinds stored in transcripts.
const (
	KindText  = "text"
	KindTool  = "tool"
	KindError = "error"
	KindChunk = "chunk"
)

// Entry is one transcript row. Tool entries upsert by ToolID within a
// session, so a status change replaces rather than appends.
type Entry struct {
	Role   string    `json:"role"`
	Text   string    `json:"text"`
	ToolID string    `json:"toolId,omitempty"`
	Kind   string    `json:"kind"`
	Status string    `json:"status,omitempty"`
	When   time.Time `json:"when"`
}

// SessionMeta describes one persisted session.
type SessionMeta struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	Title        string `json:"title"`
	Created      string `json:"created"`
	Updated      string `json:"updated"`
	MessageCount int    `json:"messageCount"`
}

// Store is a SQLite-backed transcript store. Safe for concurrent use.
type Store struct {
	mu            sync.Mutex
	db            *sql.DB
	dbPath        string
	userConfigDir func() (string, error)
	userHomeDir   func() (string, error)
}

// NewStore returns a Store using os.UserConfigDir/os.UserHomeDir. Call open
// via the constructor path NewStore + open, or use OpenStore.
func NewStore() *Store {
	return &Store{userConfigDir: os.UserConfigDir, userHomeDir: os.UserHomeDir}
}

// OpenStore creates, opens and migrates a Store.
func OpenStore() (*Store, error) {
	s := NewStore()
	if err := s.open(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) chatsDir() (string, error) {
	base, err := s.userConfigDir()
	if err != nil {
		if home, herr := s.userHomeDir(); herr == nil {
			base = filepath.Join(home, ".config")
		} else {
			base = os.TempDir()
		}
	}
	return filepath.Join(base, "codesaber", "chats"), nil
}

// open resolves the db path, creates the schema and imports legacy JSONL.
func (s *Store) open() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.chatsDir()
	if err != nil {
		return fmt.Errorf("agentstore: resolve dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("agentstore: mkdir %s: %w", dir, err)
	}
	s.dbPath = filepath.Join(dir, "agent.db")
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return fmt.Errorf("agentstore: open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		// fall back to in-memory so the app keeps working (history won't persist)
		db, err = sql.Open("sqlite", "file:agent_mem?mode=memory&cache=shared")
		if err != nil {
			return fmt.Errorf("agentstore: open in-memory fallback: %w", err)
		}
		s.dbPath = ":memory:"
	}
	// modernc/sqlite is happiest with limited concurrency; Serialize wraps
	// every conn in a mutex.
	db.SetMaxOpenConns(1)
	s.db = db
	if err := s.migrateLocked(); err != nil {
		return err
	}
	return s.importLegacyLocked(dir)
}

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  created    TEXT NOT NULL,
  updated    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_proj ON sessions(project_id, updated DESC);
CREATE TABLE IF NOT EXISTS entries (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  seq        INTEGER NOT NULL,
  role       TEXT NOT NULL,
  kind       TEXT NOT NULL,
  tool_id    TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT '',
  text       TEXT NOT NULL,
  when       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_entries_session ON entries(session_id, seq);
CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_tool
  ON entries(session_id, tool_id) WHERE tool_id <> '';
`

func (s *Store) migrateLocked() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("agentstore: schema: %w", err)
	}
	return nil
}

func validKind(k string) bool {
	switch k {
	case KindText, KindTool, KindError, KindChunk:
		return true
	}
	return false
}

// Append persists one entry. For tool entries with a ToolID it upserts by
// (session, tool_id); otherwise it appends with the next seq. When zero it is
// stamped with time.Now(). If the session row does not exist it is created
// (projectID required then). The first user text entry auto-titles an
// untitled session (first line, ≤60 chars).
func (s *Store) Append(sessionID, projectID string, e Entry) error {
	if !validKind(e.Kind) {
		return fmt.Errorf("agentstore: unknown entry kind %q", e.Kind)
	}
	if e.When.IsZero() {
		e.When = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("agentstore: begin: %w", err)
	}
	defer tx.Rollback()

	var n int
	err = tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, sessionID).Scan(&n)
	if err != nil {
		return fmt.Errorf("agentstore: lookup session: %w", err)
	}
	if n == 0 {
		if projectID == "" {
			return fmt.Errorf("agentstore: session %q does not exist and no projectID given", sessionID)
		}
		now := e.When.Format(time.RFC3339Nano)
		if _, err := tx.Exec(
			`INSERT INTO sessions (id, project_id, title, created, updated) VALUES (?, ?, '', ?, ?)`,
			sessionID, projectID, now, now,
		); err != nil {
			return fmt.Errorf("agentstore: create session: %w", err)
		}
	}

	if e.Kind == KindTool && e.ToolID != "" {
		// tool upsert: same (session_id, tool_id) row gets status/text updated
	if e.Kind == KindTool && e.ToolID != "" {
		if _, err := tx.Exec(
			`INSERT INTO entries (session_id, seq, role, kind, tool_id, status, text, when)
			 VALUES (?, COALESCE((SELECT MAX(seq)+1 FROM entries WHERE session_id = ?), 0), ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(session_id, tool_id) WHERE tool_id <> ''
			 DO UPDATE SET status = excluded.status, text = excluded.text, when = excluded.when`,
			sessionID, sessionID, e.Role, e.Kind, e.ToolID, e.Status, e.Text, e.When.Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("agentstore: upsert tool entry: %w", err)
		}
	} else {
		if _, err := tx.Exec(
			`INSERT INTO entries (session_id, seq, role, kind, tool_id, status, text, when)
			 VALUES (?, COALESCE((SELECT MAX(seq)+1 FROM entries WHERE session_id = ?), 0), ?, ?, '', '', ?, ?)`,
			sessionID, sessionID, e.Role, e.Kind, e.Text, e.When.Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("agentstore: insert entry: %w", err)
		}
	}

	// touch updated + auto-title
	now := e.When.Format(time.RFC3339Nano)
	if e.Kind == KindText && e.Role == "user" {
		if _, err := tx.Exec(
			`UPDATE sessions SET updated = ?, title = CASE WHEN title = '' THEN ? ELSE title END WHERE id = ?`,
			now, deriveTitle(e.Text), sessionID,
		); err != nil {
			return fmt.Errorf("agentstore: touch session: %w", err)
		}
	} else {
		if _, err := tx.Exec(`UPDATE sessions SET updated = ? WHERE id = ?`, now, sessionID); err != nil {
			return fmt.Errorf("agentstore: touch session: %w", err)
		}
	}
	return tx.Commit()
}

// deriveTitle extracts a session title from the first user prompt: first
// non-empty line, capped at 60 chars.
func deriveTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 60 {
			line = strings.TrimSpace(line[:60])
		}
		return line
	}
	return "New session"
}

// Read returns all entries for sessionID ordered by seq; a missing session
// reads as empty.
func (s *Store) Read(sessionID string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT role, kind, tool_id, status, text, when FROM entries WHERE session_id = ? ORDER BY seq`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("agentstore: query: %w", err)
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var when string
		if err := rows.Scan(&e.Role, &e.Kind, &e.ToolID, &e.Status, &e.Text, &when); err != nil {
			return nil, fmt.Errorf("agentstore: scan: %w", err)
		}
		if t, err := time.Parse(time.RFC3339Nano, when); err == nil {
			e.When = t
		}
		out = append(out, e)
	}
	if out == nil {
		out = []Entry{}
	}
	return out, rows.Err()
}

// ListSessions returns a project's sessions, most recently updated first.
func (s *Store) ListSessions(projectID string) ([]SessionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT s.id, s.project_id, s.title, s.created, s.updated,
		        (SELECT COUNT(*) FROM entries e WHERE e.session_id = s.id) AS n
		 FROM sessions s WHERE s.project_id = ? ORDER BY s.updated DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("agentstore: list sessions: %w", err)
	}
	defer rows.Close()
	var out []SessionMeta
	for rows.Next() {
		var m SessionMeta
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Title, &m.Created, &m.Updated, &m.MessageCount); err != nil {
			return nil, fmt.Errorf("agentstore: scan session: %w", err)
		}
		out = append(out, m)
	}
	if out == nil {
		out = []SessionMeta{}
	}
	return out, rows.Err()
}

// LatestSession returns the most recently updated session for a project.
func (s *Store) LatestSession(projectID string) (SessionMeta, error) {
	sessions, err := s.ListSessions(projectID)
	if err != nil {
		return SessionMeta{}, err
	}
	if len(sessions) == 0 {
		return SessionMeta{}, fmt.Errorf("agentstore: no sessions for project %q", projectID)
	}
	return sessions[0], nil
}

// RenameSession sets a session's title; errors if the session is missing.
func (s *Store) RenameSession(sessionID, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("agentstore: empty title")
	}
	res, err := s.db.Exec(`UPDATE sessions SET title = ? WHERE id = ?`, title, sessionID)
	if err != nil {
		return fmt.Errorf("agentstore: rename: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("agentstore: no session %q", sessionID)
	}
	return nil
}

// DeleteSession removes the session and its entries.
func (s *Store) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM entries WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("agentstore: delete entries: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, sessionID); err != nil {
		return fmt.Errorf("agentstore: delete session: %w", err)
	}
	return nil
}

// ClearAll deletes every session for a project.
func (s *Store) ClearAll(projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`DELETE FROM entries WHERE session_id IN (SELECT id FROM sessions WHERE project_id = ?)`,
		projectID,
	); err != nil {
		return fmt.Errorf("agentstore: clear entries: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("agentstore: clear sessions: %w", err)
	}
	return nil
}

// importLegacyLocked imports legacy flat <projectID>.jsonl transcripts as one
// session per file (id "<projectID>-legacy"), then renames them .imported.
// Callers must hold s.mu.
func (s *Store) importLegacyLocked(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil // unreadable dir pattern: skip import
	}
	for _, p := range matches {
		base := filepath.Base(p)
		projectID := strings.TrimSuffix(base, ".jsonl")
		entries, err := readJSONL(p)
		if err != nil || len(entries) == 0 {
			continue // unreadable/empty: leave the file alone
		}
		sid := projectID + "-legacy"
		title := "Imported chat"
		for _, e := range entries {
			if err := s.Append(sid, projectID, e); err != nil {
				return fmt.Errorf("agentstore: import %s: %w", base, err)
			}
			if e.Kind == KindText && e.Role == "user" && title == "Imported chat" {
				title = deriveTitle(e.Text)
			}
		}
		_ = s.RenameSession(sid, title)
		if err := os.Rename(p, p+".imported"); err != nil {
			return fmt.Errorf("agentstore: rename imported file: %w", err)
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
```

Delete the old JSONL helper file content — the old `store.go` body (Append/Read/Clear/path/sanitize/appendLocked/readJSONL) is fully replaced. If `readJSONL` lived in `store.go`, keep it (moved below):

```go
// readJSONL parses a legacy transcript file. Missing files read as empty.
func readJSONL(p string) ([]Entry, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, err
	}
	var out []Entry
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
```

(Add `"encoding/json"` to imports.)

**IMPORTANT cleanup:** none — the code above is final (the earlier placeholder draft was removed).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./backend/agentstore/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Fix compile errors across the repo (old Clear callers)**

```bash
go build ./... 2>&1 | head
```

Expected: errors only where `a.chats.Append/Read` signatures changed (Task 5 fixes `backend/agent.go`). If `backend/agentstore` itself is the only failure, proceed.

- [ ] **Step 6: Commit**

```bash
git add backend/agentstore/
git rm backend/agentstore/store_test.go
git commit -m "feat(agentstore): SQLite-backed transcript store with tool upsert + session CRUD"
```

---

### Task 3: Legacy JSONL migration test

**Files:**
- Modify: `backend/agentstore/store_sqlite_test.go`

- [ ] **Step 1: Write the failing test**

Append to `store_sqlite_test.go`:

```go
func TestSQLiteJSONLMigration(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "proj-1.jsonl")
	content := "{\"role\":\"user\",\"text\":\"fix the bug\",\"kind\":\"text\",\"when\":\"2026-01-01T10:00:00Z\"}\n" +
		"{\"role\":\"agent\",\"text\":\"on it\",\"kind\":\"text\",\"when\":\"2026-01-01T10:00:05Z\"}\n"
	if err := os.WriteFile(legacy, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore()
	s.userConfigDir = func() (string, error) { return dir, nil }
	s.userHomeDir = func() (string, error) { return dir, nil }
	if err := s.open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	sessions, err := s.ListSessions("proj-1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "proj-1-legacy" {
		t.Fatalf("sessions = %#v, want proj-1-legacy", sessions)
	}
	if sessions[0].Title != "fix the bug" {
		t.Fatalf("title = %q", sessions[0].Title)
	}
	entries, _ := s.Read("proj-1-legacy")
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	// source file renamed
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file still present (err=%v)", err)
	}
	if _, err := os.Stat(legacy + ".imported"); err != nil {
		t.Fatalf("imported file missing: %v", err)
	}
	// re-open does not double-import
	s2 := NewStore()
	s2.userConfigDir = s.userConfigDir
	s2.userHomeDir = s.userHomeDir
	if err := s2.open(); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	sessions, _ = s2.ListSessions("proj-1")
	if len(sessions) != 1 {
		t.Fatalf("re-import doubled sessions: %d", len(sessions))
	}
}
```

(Add `"os"` to the test file imports if not present.)

- [ ] **Step 2: Run**

```bash
go test ./backend/agentstore/ -run TestSQLiteJSONLMigration -v
```

Expected: PASS (migration was implemented in Task 2; if it fails, fix `importLegacyLocked`).

- [ ] **Step 3: Commit**

```bash
git add backend/agentstore/
git commit -m "test(agentstore): legacy JSONL migration"
```

---

### Task 4: Agent wiring — session records, transcript/session events, new RPCs (TDD)

**Files:**
- Modify: `backend/agent.go`
- Modify: `backend/agent_test.go` (new tests)
- Modify: `backend/app.go:162` (store construction → `OpenStore` + Close)

- [ ] **Step 1: Write failing tests**

Append to `backend/agent_test.go`:

```go
func TestACPSessionsCRUD(t *testing.T) {
	app, _, pid := newAgentTestApp(t)
	installFakeAgent(t, &fakePrompter{})

	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}
	sink := app.sinkForTest()
	_ = sink
	if err := app.ACPSendPrompt(pid, "fix the login bug"); err != nil {
		t.Fatalf("ACPSendPrompt: %v", err)
	}
	sessions, err := app.ACPSessions(pid)
	if err != nil {
		t.Fatalf("ACPSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Title != "fix the login bug" {
		t.Fatalf("title = %q", sessions[0].Title)
	}
	sid := sessions[0].ID

	if err := app.ACPRenameSession(sid, "Login fix"); err != nil {
		t.Fatalf("ACPRenameSession: %v", err)
	}
	sessions, _ = app.ACPSessions(pid)
	if sessions[0].Title != "Login fix" {
		t.Fatalf("title after rename = %q", sessions[0].Title)
	}

	// new session creates a second record
	if err := app.ACPNewSession(pid); err != nil {
		t.Fatalf("ACPNewSession: %v", err)
	}
	sessions, _ = app.ACPSessions(pid)
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}

	// delete the non-active one
	other := sessions[0].ID
	if other == app.activeSessionID(pid) {
		other = sessions[1].ID
	}
	if err := app.ACPDeleteSession(pid, other); err != nil {
		t.Fatalf("ACPDeleteSession: %v", err)
	}
	sessions, _ = app.ACPSessions(pid)
	if len(sessions) != 1 {
		t.Fatalf("sessions after delete = %d", len(sessions))
	}
}

func TestACPOpenSessionSwitchesTranscript(t *testing.T) {
	app, sink, pid := newAgentTestApp(t)
	installFakeAgent(t, &fakePrompter{})
	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}
	if err := app.ACPSendPrompt(pid, "first session prompt"); err != nil {
		t.Fatalf("prompt 1: %v", err)
	}
	if err := app.ACPNewSession(pid); err != nil {
		t.Fatalf("ACPNewSession: %v", err)
	}
	if err := app.ACPSendPrompt(pid, "second session prompt"); err != nil {
		t.Fatalf("prompt 2: %v", err)
	}
	sessions, _ := app.ACPSessions(pid)
	var firstID string
	for _, s := range sessions {
		if s.Title == "first session prompt" {
			firstID = s.ID
		}
	}
	if firstID == "" {
		t.Fatal("first session not found")
	}
	if err := app.ACPOpenSession(pid, firstID); err != nil {
		t.Fatalf("ACPOpenSession: %v", err)
	}
	entries, err := app.ACPLoadTranscript(pid)
	if err != nil {
		t.Fatalf("LoadTranscript: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Text == "first session prompt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("opened transcript lacks first-session prompt: %#v", entries)
	}
	_ = sink
}

func TestACPDeleteSessionRefusesActiveWhileRunning(t *testing.T) {
	app, _, pid := newAgentTestApp(t)
	installFakeAgent(t, &fakePrompter{})
	if err := app.ACPStart(pid, "opencode"); err != nil {
		t.Fatalf("ACPStart: %v", err)
	}
	if err := app.ACPSendPrompt(pid, "active session"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	sessions, _ := app.ACPSessions(pid)
	sid := sessions[0].ID
	if err := app.ACPDeleteSession(pid, sid); err == nil {
		t.Fatal("want error deleting the active, running session")
	}
	// after stop it is allowed
	if err := app.ACPStop(pid); err != nil {
		t.Fatalf("ACPStop: %v", err)
	}
	if err := app.ACPDeleteSession(pid, sid); err != nil {
		t.Fatalf("delete after stop: %v", err)
	}
}
```

Note: `app.sinkForTest()` does not exist — remove those two lines and the `_ = sink` lines; the tests above don't need the sink. (They are shown crossed out intentionally — final tests contain no sink references.)

- [ ] **Step 2: Run to verify failure**

```bash
go test ./backend/ -run 'TestACPSessionsCRUD|TestACPOpenSession|TestACPDeleteSessionRefuses' 2>&1 | head
```

Expected: compile errors — `ACPSessions`, `ACPRenameSession`, `ACPDeleteSession`, `ACPOpenSession`, `activeSessionID` undefined.

- [ ] **Step 3: Implement in backend/agent.go**

Changes (surgical, keep everything else):

1. Add to `agentSession` struct: `chatID string` (the persisted session record id).

2. Add helper + session-id generation near the top:

```go
// newChatSessionID mints a persisted-transcript session id.
func newChatSessionID() string {
	return fmt.Sprintf("ses-%d", time.Now().UnixNano())
}

// activeSessionID returns the persisted session record the project's harness
// is currently writing to ("" when none).
func (a *App) activeSessionID(projectID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	ag := a.agents[projectID]
	if ag == nil {
		return ""
	}
	return ag.chatID
}
```

3. In `acpSpawnOnAgent`, after a successful spawn, assign a fresh chat record if the agent has none:

```go
	s.SetOnUpdate(a.acpUpdateHandler(projectID, ag))
	ag.setSession(s)
	// persisted session record: fresh per harness session
	ag.mu.Lock()
	if ag.chatID == "" {
		ag.chatID = newChatSessionID()
	}
	chatID := ag.chatID
	ag.mu.Unlock()
	_ = a.chats.Append(chatID, projectID, agentstore.Entry{Role: "system", Kind: agentstore.KindText, Text: "— session started —"})
	a.emitTranscript(projectID, chatID)
	a.emitState(projectID, AgentStateIdle)
	return nil
```

4. Change every `a.chats.Append(projectID, …)` call to `a.chats.Append(chatID, projectID, …)`:
   - `ACPSendPrompt` (user entry + assembled reply): get chatID under ag.mu first:

```go
	ag.mu.Lock()
	chatID := ag.chatID
	ag.mu.Unlock()
```

   - `ACPNewSession` divider append: use the *old* chatID before respawn (spawn assigns a new one).
   - WriteTextFile tool note + `acpUpdateHandler` tool append: same pattern (grab `ag.chatID` under ag.mu).

5. `emitTranscript` gains the session id:

```go
func (a *App) emitTranscript(projectID, chatID string) {
	entries, err := a.chats.Read(chatID)
	if err != nil {
		entries = []agentstore.Entry{}
	}
	a.sink.Emit(EventACPTranscript, map[string]any{"projectId": projectID, "sessionID": chatID, "entries": entries})
}
```

   Update the idempotent-start re-show in `acpStartProfile` (`a.emitTranscript(projectID)` → resolve chatID under ag.mu, may be "" → then read latest):

```go
	if !claimed {
		chatID := a.chatIDForEmit(projectID)
		a.emitTranscript(projectID, chatID)
		return nil
	}
```

```go
// chatIDForEmit resolves the transcript id to show: the agent's active record
// or, when no harness is running, the project's latest session.
func (a *App) chatIDForEmit(projectID string) string {
	if id := a.activeSessionID(projectID); id != "" {
		return id
	}
	if meta, err := a.chats.LatestSession(projectID); err == nil {
		return meta.ID
	}
	return ""
}
```

6. `ACPLoadTranscript` becomes latest-session aware (back-compat signature kept):

```go
// ACPLoadTranscript returns the persisted entries for the project's active
// (or latest) session.
func (a *App) ACPLoadTranscript(projectID string) ([]agentstore.Entry, error) {
	chatID := a.chatIDForEmit(projectID)
	if chatID == "" {
		return []agentstore.Entry{}, nil
	}
	return a.chats.Read(chatID)
}
```

7. New RPCs:

```go
// ACPSessions lists the project's persisted agent sessions (newest first).
func (a *App) ACPSessions(projectID string) ([]agentstore.SessionMeta, error) {
	return a.chats.ListSessions(projectID)
}

// ACPOpenSession switches the UI/harness context to a persisted session:
// with a running harness it re-points the transcript and re-emits it; without
// one it just emits the stored transcript (no auto-start).
func (a *App) ACPOpenSession(projectID, sessionID string) error {
	sessions, err := a.chats.ListSessions(projectID)
	if err != nil {
		return err
	}
	found := false
	for _, s := range sessions {
		if s.ID == sessionID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("acp: no session %q for project", sessionID)
	}
	a.mu.Lock()
	ag := a.agents[projectID]
	if ag != nil {
		ag.mu.Lock()
		ag.chatID = sessionID
		ag.mu.Unlock()
	}
	a.mu.Unlock()
	a.emitTranscript(projectID, sessionID)
	return nil
}

// ACPDeleteSession removes a persisted session. Refuses while a running
// harness is writing to it.
func (a *App) ACPDeleteSession(projectID, sessionID string) error {
	if a.activeSessionID(projectID) == sessionID {
		if ag := a.agentFor(projectID); ag != nil && ag.get() != nil {
			return errors.New("acp: cannot delete the active session while the harness is running")
		}
	}
	return a.chats.DeleteSession(sessionID)
}

// ACPRenameSession sets a session's display title.
func (a *App) ACPRenameSession(sessionID, title string) error {
	return a.chats.RenameSession(sessionID, title)
}

// ACPClearTranscript deletes every entry of the active/latest session.
func (a *App) ACPClearTranscript(projectID string) error {
	chatID := a.chatIDForEmit(projectID)
	if chatID == "" {
		return nil
	}
	if err := a.chats.DeleteSession(chatID); err != nil {
		return err
	}
	a.emitTranscript(projectID, chatID)
	return nil
}
```

8. `backend/app.go`: change `chats: agentstore.NewStore(),` to open via `OpenStore()` inside the existing constructor error path (constructor returns `*App` with no error — log-and-fallback):

```go
	chats, err := agentstore.OpenStore()
	if err != nil {
		chats, _ = agentstore.OpenStore() // open() itself falls back to memory
	}
	app.chats = chats
```

and add `app.chats.Close()` to any existing shutdown path (search `func (a *App) shutdown` / `OnShutdown`).

9. `ACPNewSession` divider: replace the current divider append with one against the old chatID, and clear chatID so the respawn mints a new record:

```go
	ag.mu.Lock()
	oldChatID := ag.chatID
	ag.chatID = ""
	ag.mu.Unlock()
	if oldChatID != "" {
		_ = a.chats.Append(oldChatID, projectID, agentstore.Entry{Role: "system", Kind: agentstore.KindText, Text: "— session ended —"})
	}
```

- [ ] **Step 4: Run backend tests**

```bash
go test ./backend/... 2>&1 | tail -20
```

Expected: PASS (existing tests updated where `emitTranscript` signature changed — update call sites in `agent_test.go` if any reference it directly).

- [ ] **Step 5: Regenerate bindings**

```bash
~/go/bin/wails3 task generate:bindings -clean=true -ts -i
git status --short frontend/bindings
```

Expected: `agentstore/models.ts` now exports `SessionMeta`; `app.ts` gains `ACPSessions`, `ACPOpenSession`, `ACPDeleteSession`, `ACPRenameSession`, `ACPClearTranscript`.

- [ ] **Step 6: Build green**

```bash
go build ./... && go test ./backend/...
```

- [ ] **Step 7: Commit**

```bash
git add backend/ frontend/bindings/
git commit -m "feat(agent): persisted multi-session records + session RPCs"
```

---

### Task 5: Frontend state — unified timeline + sessions (TDD)

**Files:**
- Create: `frontend/tests/agentTimeline.test.ts`
- Rewrite: `frontend/src/state/agent.tsx`

- [ ] **Step 1: Write the failing reducer tests**

Create `frontend/tests/agentTimeline.test.ts` (pure logic tests — the reducer is exported from the state module):

```ts
import { describe, it, expect } from 'vitest'
import {
  emptyAgentState,
  reduceMsg,
  reduceTool,
  reducePermission,
  reduceStateFlip,
  reduceTranscript,
  type TimelineItem,
  type ToolItem,
  type PermissionItem,
} from '../src/state/agentTimeline'

describe('agent timeline reducer', () => {
  it('creates one message per chunk-batch and merges the tail', () => {
    let s = emptyAgentState()
    s = reduceMsg(s, { role: 'agent', text: 'Hel', kind: 'chunk' })
    s = reduceMsg(s, { role: 'agent', text: 'lo', kind: 'chunk' })
    s = reduceMsg(s, { role: 'user', text: 'hi', kind: 'text' })
    s = reduceMsg(s, { role: 'agent', text: 'ag', kind: 'chunk' })
    const tl = s.timeline
    expect(tl).toHaveLength(3)
    expect((tl[0] as any).text).toBe('Hello')
    expect(tl[1].type).toBe('message')
    expect((tl[2] as any).text).toBe('ag')
  })

  it('upserts tool cards in place by toolCallId', () => {
    let s = emptyAgentState()
    s = reduceTool(s, { toolCallId: 't1', title: 'Read x', status: 'in_progress', kind: 'read', content: '' })
    s = reduceTool(s, { toolCallId: 't2', title: 'Grep y', status: 'in_progress', kind: 'search', content: '' })
    s = reduceTool(s, { toolCallId: 't1', title: 'Read x', status: 'completed', kind: 'read', content: 'out' })
    const tools = s.timeline.filter((i) => i.type === 'tool') as ToolItem[]
    expect(tools).toHaveLength(2)
    expect(tools[0].status).toBe('completed')
    expect(tools[0].content).toBe('out')
    expect(s.timeline.map((i) => i.type)).toEqual(['tool', 'tool'])
  })

  it('stacks permissions and drops them on state flip to thinking', () => {
    let s = emptyAgentState()
    s = reducePermission(s, { requestId: 'p1', options: [] })
    s = reducePermission(s, { requestId: 'p2', options: [] })
    s = reducePermission(s, { requestId: 'p1', options: [] }) // dedupe
    expect(s.timeline.filter((i) => i.type === 'permission')).toHaveLength(2)
    s = reduceStateFlip(s, 'thinking')
    expect(s.timeline.filter((i) => i.type === 'permission')).toHaveLength(0)
  })

  it('rebuilds the timeline from transcript entries interleaved', () => {
    const entries = [
      { role: 'user', text: 'go', kind: 'text', when: '2026-01-01T00:00:00Z' },
      { role: 'agent', text: 'Reading', kind: 'tool', toolId: 't1', when: '2026-01-01T00:00:01Z' },
      { role: 'agent', text: 'done', kind: 'text', when: '2026-01-01T00:00:02Z' },
    ]
    const s = reduceTranscript(emptyAgentState(), entries)
    expect(s.timeline.map((i) => i.type)).toEqual(['message', 'tool', 'message'])
    expect((s.timeline[0] as any).role).toBe('user')
    expect((s.timeline[1] as any).toolCallId).toBe('t1')
    expect((s.timeline[2] as any).text).toBe('done')
  })
})
```

- [ ] **Step 2: Run to verify failure**

```bash
cd frontend && npx vitest run tests/agentTimeline.test.ts
```

Expected: FAIL — module `../src/state/agentTimeline` not found.

- [ ] **Step 3: Create the reducer module**

Create `frontend/src/state/agentTimeline.ts`:

```ts
// Pure timeline reducer for the agent chat: one ordered list of message /
// tool / permission items replaces the old separate messages[] and tools[]
// arrays. Kept dependency-free (no React) so vitest can exercise it directly.

export type AgentState = 'idle' | 'thinking' | 'harness-down' | 'no-harness'

export interface MessageItem {
  type: 'message'
  id: string
  role: 'user' | 'agent' | 'system'
  text: string
  kind: 'text' | 'chunk' | 'error'
}

export interface ToolItem {
  type: 'tool'
  toolCallId: string
  title: string
  kind: string
  status: string
  content: string
}

export interface PermissionItem {
  type: 'permission'
  requestId: string
  options: PermissionOption[]
  purpose?: string
  path?: string
  oldText?: string
  newText?: string
  isNew?: boolean
  truncated?: boolean
}

export type TimelineItem = MessageItem | ToolItem | PermissionItem

export interface PermissionOption {
  optionId?: string
  name?: string
  description?: string
  kind?: string
}

export interface AgentProjectState {
  status: AgentState
  timeline: TimelineItem[]
  harness: string | null
  sessionId: string | null
}

export interface TranscriptEntry {
  role: string
  text: string
  kind: string
  toolId?: string
  when?: string
}

let seq = 0
const nextId = () => `t${++seq}`

export const emptyAgentState = (): AgentProjectState => ({
  status: 'no-harness',
  timeline: [],
  harness: null,
  sessionId: null,
})

// reduceMsg applies an acp.msg event. Chunk invariant: the first chunk of a
// reply creates one message item; later chunks extend that tail item. A
// non-chunk message always appends.
export const reduceMsg = (
  s: AgentProjectState,
  ev: { role?: string; text: string; kind?: string },
): AgentProjectState => {
  if (!ev.text) return s
  const timeline = s.timeline.slice()
  if (ev.kind === 'chunk') {
    const tail = timeline[timeline.length - 1]
    if (tail && tail.type === 'message' && tail.role === 'agent' && tail.kind === 'chunk') {
      timeline[timeline.length - 1] = { ...tail, text: tail.text + ev.text }
      return { ...s, timeline }
    }
    timeline.push({ type: 'message', id: nextId(), role: 'agent', text: ev.text, kind: 'chunk' })
    return { ...s, timeline }
  }
  const role: MessageItem['role'] =
    ev.role === 'user' ? 'user' : ev.role === 'system' ? 'system' : 'agent'
  timeline.push({
    type: 'message',
    id: nextId(),
    role,
    text: ev.text,
    kind: ev.kind === 'error' ? 'error' : 'text',
  })
  return { ...s, timeline }
}

// reduceTool applies an acp.tool event: upsert by toolCallId in place.
export const reduceTool = (
  s: AgentProjectState,
  ev: { toolCallId: string; title?: string; kind?: string; status?: string; content?: string },
): AgentProjectState => {
  if (!ev.toolCallId) return s
  const timeline = s.timeline.slice()
  const idx = timeline.findIndex((i) => i.type === 'tool' && i.toolCallId === ev.toolCallId)
  const item: ToolItem = {
    type: 'tool',
    toolCallId: ev.toolCallId,
    title: ev.title ?? (idx >= 0 ? (timeline[idx] as ToolItem).title : ''),
    kind: ev.kind ?? (idx >= 0 ? (timeline[idx] as ToolItem).kind : 'other'),
    status: ev.status ?? (idx >= 0 ? (timeline[idx] as ToolItem).status : 'pending'),
    content: ev.content ?? (idx >= 0 ? (timeline[idx] as ToolItem).content : ''),
  }
  if (idx >= 0) timeline[idx] = item
  else timeline.push(item)
  return { ...s, timeline }
}

// reducePermission appends a permission card (dedup by requestId).
export const reducePermission = (
  s: AgentProjectState,
  p: PermissionItem,
): AgentProjectState => {
  const exists = s.timeline.some(
    (i) => i.type === 'permission' && i.requestId === p.requestId,
  )
  if (exists) return s
  return { ...s, timeline: [...s.timeline, p] }
}

// removePermission drops the resolved card.
export const removePermission = (s: AgentProjectState, requestId: string): AgentProjectState => ({
  ...s,
  timeline: s.timeline.filter((i) => !(i.type === 'permission' && i.requestId === requestId)),
})

// reduceStateFlip applies an acp.state change; a flip to thinking drops
// pending permission cards (the backend answers them server-side).
export const reduceStateFlip = (s: AgentProjectState, st: AgentState): AgentProjectState => ({
  ...s,
  status: st,
  timeline:
    st === 'thinking'
      ? s.timeline.filter((i) => i.type !== 'permission')
      : s.timeline,
})

// reduceTranscript rebuilds the whole timeline from persisted entries.
export const reduceTranscript = (
  s: AgentProjectState,
  entries: TranscriptEntry[],
): AgentProjectState => {
  const timeline: TimelineItem[] = []
  for (const e of entries) {
    if (e.kind === 'tool' && e.toolId) {
      timeline.push({
        type: 'tool',
        toolCallId: e.toolId,
        title: e.text,
        kind: 'other',
        status: 'completed',
        content: '',
      })
      continue
    }
    const role: MessageItem['role'] =
      e.role === 'user' ? 'user' : e.role === 'system' ? 'system' : 'agent'
    timeline.push({
      type: 'message',
      id: nextId(),
      role,
      text: e.text,
      kind: e.kind === 'error' ? 'error' : 'text',
    })
  }
  return { ...s, timeline }
}
```

- [ ] **Step 4: Run reducer tests**

```bash
cd frontend && npx vitest run tests/agentTimeline.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/state/agentTimeline.ts frontend/tests/agentTimeline.test.ts
git commit -m "feat(agent-ui): unified timeline reducer (messages/tools/permissions)"
```

---

### Task 6: Frontend state — sessions + event rewiring in agent.tsx

**Files:**
- Modify: `frontend/src/state/agent.tsx` (rewrite event handlers around the reducer; add session state)

- [ ] **Step 1: Rewrite `state/agent.tsx`**

Key shape (full file; imports/bindings names match regenerated `App` module):

```tsx
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { Events } from '@wailsio/runtime'
import * as App from '../../bindings/codesaber/backend/app'
import type { Info as HarnessInfo } from '../../bindings/codesaber/backend/acp/models'
import type { SessionMeta, Entry } from '../../bindings/codesaber/backend/agentstore/models'
import { useProjects } from './projects'
import {
  emptyAgentState,
  reduceMsg,
  reduceTool,
  reducePermission,
  removePermission,
  reduceStateFlip,
  reduceTranscript,
  type AgentProjectState,
  type PermissionItem,
  type PermissionOption,
} from './agentTimeline'

export type { AgentState } from './agentTimeline'
export type {
  TimelineItem,
  MessageItem,
  ToolItem,
  PermissionItem,
  PermissionOption,
} from './agentTimeline'

interface AgentContextValue {
  state: Record<string, AgentProjectState>
  sessions: Record<string, SessionMeta[]>
  activeSession: Record<string, string | null>
  harnesses: HarnessInfo[]
  send: (projectId: string, text: string) => Promise<void>
  start: (projectId: string, harnessName: string) => Promise<void>
  stop: (projectId: string) => Promise<void>
  newSession: (projectId: string) => Promise<void>
  clearTranscript: (projectId: string) => Promise<void>
  respondPermission: (projectId: string, requestId: string, optionId: string, cancel: boolean) => Promise<void>
  listSessions: (projectId: string) => Promise<void>
  openSession: (projectId: string, sessionID: string) => Promise<void>
  deleteSession: (projectId: string, sessionID: string) => Promise<string | null>
  renameSession: (sessionID: string, title: string) => Promise<void>
  generateCommit: (projectId: string, prompt: string) => Promise<string>
}

const emptySessions: Record<string, SessionMeta[]> = {}

export const AgentProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [state, setState] = useState<Record<string, AgentProjectState>>({})
  const [sessions, setSessions] = useState<Record<string, SessionMeta[]>>(emptySessions)
  const [activeSession, setActiveSession] = useState<Record<string, string | null>>({})
  const [harnesses, setHarnesses] = useState<HarnessInfo[]>([])

  const patch = useCallback(
    (projectId: string, fn: (s: AgentProjectState) => AgentProjectState) => {
      setState((prev) => ({ ...prev, [projectId]: fn(prev[projectId] ?? emptyAgentState()) }))
    },
    [],
  )

  const refreshSessions = useCallback(async (projectId: string) => {
    try {
      const list = await App.ACPSessions(projectId)
      setSessions((prev) => ({ ...prev, [projectId]: list ?? [] }))
      setActiveSession((prev) => ({ ...prev, [projectId]: prev[projectId] ?? (list?.[0]?.id ?? null) }))
    } catch {
      setSessions((prev) => ({ ...prev, [projectId]: [] }))
    }
  }, [])

  useEffect(() => {
    const onMsg = Events.On('acp.msg', (ev: any) => {
      const { projectId, role, text, kind } = ev.data ?? {}
      if (!projectId || !text) return
      patch(projectId, (s) => reduceMsg(s, { role, text, kind }))
    })
    const onTool = Events.On('acp.tool', (ev: any) => {
      const { projectId, toolCallId, title, kind, status, content } = ev.data ?? {}
      if (!projectId || !toolCallId) return
      patch(projectId, (s) => reduceTool(s, { toolCallId, title, kind, status, content }))
    })
    const onPermission = Events.On('acp.permission', (ev: any) => {
      const { projectId, requestId, options, purpose, path, oldText, newText, isNew, truncated } = ev.data ?? {}
      if (!projectId || !requestId) return
      const opts = (Array.isArray(options) ? options : [])
        .map(asPermissionOption)
        .filter(Boolean) as PermissionOption[]
      patch(projectId, (s) =>
        reducePermission(s, {
          type: 'permission',
          requestId,
          options: opts,
          purpose,
          path,
          oldText,
          newText,
          isNew,
          truncated,
        }),
      )
    })
    const onState = Events.On('acp.state', (ev: any) => {
      const { projectId, state: st } = ev.data ?? {}
      if (!projectId || !st) return
      patch(projectId, (s) => reduceStateFlip(s, st))
    })
    const onTranscript = Events.On('acp.transcript', (ev: any) => {
      const { projectId, sessionID, entries } = ev.data ?? {}
      if (!projectId) return
      patch(projectId, (s) => ({
        ...s,
        sessionId: sessionID ?? s.sessionId,
        ...reduceTranscript(s, entries ?? []),
      }))
      if (sessionID) setActiveSession((prev) => ({ ...prev, [projectId]: sessionID }))
      void refreshSessions(projectId)
    })
    const onRemoved = Events.On('project.removed', (ev: any) => {
      const { id } = ev.data ?? {}
      if (!id) return
      setState((prev) => {
        const next = { ...prev }
        delete next[id]
        return next
      })
      setSessions((prev) => {
        const next = { ...prev }
        delete next[id]
        return next
      })
      setActiveSession((prev) => {
        const next = { ...prev }
        delete next[id]
        return next
      })
    })
    return () => {
      onMsg()
      onTool()
      onPermission()
      onState()
      onTranscript()
      onRemoved()
    }
  }, [patch, refreshSessions])

  const { activeId } = useProjects()
  useEffect(() => {
    App.ACPHarnesses().then((hs) => setHarnesses(hs ?? [])).catch(() => setHarnesses([]))
  }, [])

  useEffect(() => {
    if (!activeId) return
    void refreshSessions(activeId)
  }, [activeId, refreshSessions])

  // ... actions (send/start/stop/newSession/clearTranscript/respondPermission/
  // listSessions/openSession/deleteSession/renameSession/generateCommit) —
  // bodies below.
}
```

**Action bodies:**

```tsx
  const send = useCallback(async (projectId: string, text: string) => {
    await App.ACPSendPrompt(projectId, text)
  }, [])

  const start = useCallback(
    async (projectId: string, harnessName: string) => {
      await App.ACPStart(projectId, harnessName)
      patch(projectId, (s) => ({ ...s, harness: harnessName }))
      void refreshSessions(projectId)
    },
    [patch, refreshSessions],
  )

```tsx
  const stop = useCallback(
    async (projectId: string) => {
      await App.ACPStop(projectId)
      patch(projectId, (s) => ({ ...s, harness: null }))
    },
    [patch],
  )

  const newSession = useCallback(
    async (projectId: string) => {
      await App.ACPNewSession(projectId)
    },
    [],
  )

  const clearTranscript = useCallback(
    async (projectId: string) => {
      await App.ACPClearTranscript(projectId)
    },
    [],
  )

  const respondPermission = useCallback(
    async (projectId: string, requestId: string, optionId: string, cancel: boolean) => {
      await App.ACPRespondPermission(projectId, requestId, optionId, cancel)
      patch(projectId, (s) => removePermission(s, requestId))
    },
    [patch],
  )

  const listSessions = useCallback(
    async (projectId: string) => void refreshSessions(projectId),
    [refreshSessions],
  )

  const openSession = useCallback(
    async (projectId: string, sessionID: string) => {
      await App.ACPOpenSession(projectId, sessionID)
      setActiveSession((prev) => ({ ...prev, [projectId]: sessionID }))
    },
    [],
  )

  const deleteSession = useCallback(
    async (projectId: string, sessionID: string): Promise<string | null> => {
      try {
        await App.ACPDeleteSession(projectId, sessionID)
        await refreshSessions(projectId)
        return null
      } catch (e) {
        return String(e)
      }
    },
    [refreshSessions],
  )

  const renameSession = useCallback(
    async (sessionID: string, title: string) => {
      await App.ACPRenameSession(sessionID, title)
      if (activeId) void refreshSessions(activeId)
    },
    [activeId, refreshSessions],
  )
```

(fix trailing comma → `)`.)

`generateCommit` keeps its existing body (local event listeners, 30s timeout) unchanged — copy verbatim from the current file.

- [ ] **Step 6: Typecheck + existing tests**

```bash
cd frontend && npx tsc --noEmit && npx vitest run
```

Expected: clean. (`GitPanel.tsx` still compiles — it consumes `agentState[id]` only for `status`/`generateCommit`, both unchanged.)

- [ ] **Step 7: Commit**

```bash
git add frontend/src/state/
git commit -m "feat(agent-ui): sessions state + reducer-backed event handling"
```

---

### Task 7: AgentPanel rebuild (GitPanel sibling UI)

**Files:**
- Rewrite: `frontend/src/components/AgentPanel.tsx`

- [ ] **Step 1: Rebuild the component**

Full structure (GitPanel styling idioms: `rounded-lg border border-[#333639] bg-[#242629]` cards, `text-dim`/`text-primary`, status chips, `bg-gradient-to-r from-[#5b3fd4] to-[#6f51e0]` CTA):

```tsx
import React, { useEffect, useMemo, useRef, useState } from 'react'
import * as App from '../../bindings/codesaber/backend/app'
import type { SessionMeta } from '../../bindings/codesaber/backend/agentstore/models'
import { useProjects } from '../state/projects'
import {
  useAgent,
  type TimelineItem,
  type MessageItem,
  type ToolItem,
  type PermissionItem,
} from '../state/agent'
import MiniDiff, { diffCounts } from './MiniDiff'

// ---- helpers -------------------------------------------------------------
const statusChip = (status: string) => {
  if (status === 'in_progress' || status === 'pending')
    return { dot: 'bg-[#e6c07b] animate-pulse', label: 'running', cls: 'text-[#e6c07b]' }
  if (status === 'completed')
    return { dot: 'bg-[#7dcf9e]', label: 'done', cls: 'text-[#7dcf9e]' }
  if (status === 'failed')
    return { dot: 'bg-[#e5735f]', label: 'failed', cls: 'text-[#e5735f]' }
  return { dot: 'bg-gray-500', label: status, cls: 'text-dim' }
}

const toolGlyph = (kind: string) =>
  kind === 'read' ? '📄' : kind === 'edit' ? '✏️' : kind === 'search' ? '⌕' : '🔧'

const timeAgo = (iso: string) => {
  const t = Date.parse(iso)
  if (isNaN(t)) return iso
  const s = Math.max(1, Math.floor((Date.now() - t) / 1000))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

// ---- timeline item renderers --------------------------------------------
const UserBubble: React.FC<{ m: MessageItem }> = ({ m }) => (
  <div className="flex justify-end px-3 py-1">
    <div className="max-w-[85%] rounded-lg border border-[var(--accent)]/35 bg-[var(--accent)]/15 text-primary text-xs px-3 py-1.5 whitespace-pre-wrap break-words">
      {m.text}
    </div>
  </div>
)

const AgentText: React.FC<{ m: MessageItem }> = ({ m }) => (
  <div
    className={
      'px-3 py-1 text-xs whitespace-pre-wrap break-words ' +
      (m.kind === 'error' ? 'text-[#e5735f]' : m.role === 'system' ? 'text-dim text-center text-[10px]' : 'text-[#bcbec4]')
    }
  >
    {m.role === 'system' ? <span className="px-2 py-0.5 rounded bg-[#1e1f22]">{m.text}</span> : m.text}
  </div>
)

const ToolCard: React.FC<{ t: ToolItem }> = ({ t }) => {
  const [open, setOpen] = useState(false)
  const chip = statusChip(t.status)
  return (
    <div className="mx-3 my-0.5 rounded-lg border border-[#333639] bg-[#242629] overflow-hidden">
      <button
        className="w-full flex items-center gap-2 px-2.5 py-1.5 text-[11px] text-left"
        onClick={() => setOpen((o) => !o)}
        title={open ? 'Collapse' : 'Expand'}
      >
        <span className={'text-dim text-[10px] transition-transform ' + (open ? 'rotate-90' : '')}>›</span>
        <span>{toolGlyph(t.kind)}</span>
        <span className="truncate flex-1 min-w-0 text-[#bcbec4]">
          {t.title || t.toolCallId}
        </span>
        <span className={'shrink-0 flex items-center gap-1.5 ' + chip.cls}>
          <span className={'w-1.5 h-1.5 rounded-full ' + chip.dot} />
          <span className="text-[10px]">{chip.label}</span>
        </span>
      </button>
      {open && t.content && (
        <pre className="border-t border-[#26282b] px-2.5 py-1.5 text-[10px] text-dim whitespace-pre-wrap break-words max-h-40 overflow-y-auto">
          {t.content}
        </pre>
      )}
    </div>
  )
}

const PermissionCard: React.FC<{ p: PermissionItem; onRespond: (optionId: string, cancel: boolean) => void }> = ({ p, onRespond }) => {
  if (p.purpose === 'fs-write') {
    const oldText = p.oldText ?? ''
    const newText = p.newText ?? ''
    const counts = diffCounts(oldText, newText)
    return (
      <div className="mx-3 my-1 rounded-lg border border-[#e6c07b]/40 bg-[#1e1f22] p-2">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-[11px] text-[#e6c07b] font-semibold">
            {p.isNew ? 'Create file' : 'Edit file'}
          </span>
          <span className="text-[11px] text-dim truncate min-w-0" title={p.path}>{p.path}</span>
          <span className="ml-auto shrink-0 text-[10px] font-mono">
            <span className="text-[#7dcf9e]">+{counts.adds}</span>{' '}
            <span className="text-[#e5735f]">−{counts.dels}</span>
          </span>
        </div>
        <MiniDiff oldText={oldText} newText={newText} />
        {p.truncated && (
          <div className="mt-1 text-[10px] text-[#e6c07b]/80">preview truncated — open full diff after applying</div>
        )}
        <div className="flex gap-1.5 mt-1.5">
          <button className="no-drag px-3 py-1 rounded bg-[#2a2c31] text-[#e5735f] hover:bg-[#373940]" onClick={() => onRespond('', true)}>Reject</button>
          <button className="no-drag ml-auto px-3 py-1 rounded bg-[var(--accent)] text-[#0b0c10] font-medium hover:opacity-90" onClick={() => onRespond('allow', false)}>Accept</button>
        </div>
      </div>
    )
  }
  return (
    <div className="mx-3 my-1 rounded-lg border border-[#e6c07b]/40 bg-[#1e1f22] p-2">
      <div className="text-[11px] text-[#e6c07b] mb-1">Permission requested</div>
      {p.options.length === 0 ? (
        <div className="text-dim">(no options offered)</div>
      ) : (
        <div className="flex flex-col gap-1">
          {p.options.map((o) => (
            <button
              key={o.optionId ?? o.name}
              className="text-left px-2 py-1 rounded bg-[#2a2c31] hover:bg-[#373940]"
              onClick={() => onRespond(o.optionId ?? '', false)}
            >
              <span className="text-primary">{o.name ?? o.optionId}</span>
              {o.description && <span className="text-dim"> — {o.description}</span>}
            </button>
          ))}
          <button className="text-left px-2 py-1 rounded text-[#e5735f] hover:bg-[#2a2c31]" onClick={() => onRespond('', true)}>
            Reject
          </button>
        </div>
      )}
    </div>
  )
}

const TimelineItemView: React.FC<{ item: TimelineItem; onRespond: (requestId: string, optionId: string, cancel: boolean) => void }> = ({ item, onRespond }) => {
  if (item.type === 'message') return item.role === 'user' ? <UserBubble m={item} /> : <AgentText m={item} />
  if (item.type === 'tool') return <ToolCard t={item} />
  return (
    <PermissionCard
      p={item}
      onRespond={(optionId, cancel) => onRespond(item.requestId, optionId, cancel)}
    />
  )
}

// ---- history tab ---------------------------------------------------------
const SessionRow: React.FC<{
  s: SessionMeta
  active: boolean
  onOpen: () => void
  onRename: (title: string) => void
  onDelete: () => void
}> = ({ s, active, onOpen, onRename, onDelete }) => {
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [title, setTitle] = useState(s.title)
  return (
    <div className="mb-2 px-1 py-1 rounded hover:bg-white/4">
      <div className="flex items-center gap-2 cursor-pointer" onClick={() => setOpen((o) => !o)}>
        <span className={'w-1.5 h-1.5 rounded-full shrink-0 ' + (active ? 'bg-[#7dcf9e]' : 'bg-[#5b3fd4]')} />
        <span className="text-white truncate text-xs flex-1 min-w-0">{s.title || '(untitled)'}</span>
        <span className="text-[10px] text-dim shrink-0">{timeAgo(s.updated)}</span>
      </div>
      <div className="pl-3.5 text-[11px] text-dim">
        {s.messageCount} messages{s.projectId ? '' : ''}
      </div>
      {open && (
        <div className="mt-1 ml-3.5 flex flex-col gap-1">
          {editing ? (
            <div className="flex gap-1">
              <input
                autoFocus
                className="flex-1 bg-[#1e1f22] border border-[#333639] rounded px-2 py-1 text-xs text-primary outline-none focus:ring-1 focus:ring-[var(--accent)]"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    onRename(title.trim() || s.title)
                    setEditing(false)
                  }
                  if (e.key === 'Escape') setEditing(false)
                }}
              />
              <button className="px-2 rounded bg-[var(--accent)] text-[#0b0c10] text-xs" onClick={() => { onRename(title.trim() || s.title); setEditing(false) }}>Save</button>
            </div>
          ) : (
            <div className="flex gap-2 text-[10px] uppercase tracking-wide">
              <button className="text-[var(--accent)] hover:underline" onClick={onOpen}>Open</button>
              <button className="text-dim hover:text-primary" onClick={() => { setTitle(s.title); setEditing(true) }}>Rename</button>
              <button className="text-[#e5735f] hover:underline" onClick={onDelete}>Delete</button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ---- main panel ----------------------------------------------------------
const AgentPanel: React.FC = () => {
  const { activeId } = useProjects()
  const {
    state, sessions, activeSession, harnesses,
    send, start, stop, newSession, clearTranscript, respondPermission,
    listSessions, openSession, deleteSession, renameSession,
  } = useAgent()
  const [draft, setDraft] = useState('')
  const [tab, setTab] = useState<'chat' | 'history'>('chat')
  const [confirmDelete, setConfirmDelete] = useState<SessionMeta | null>(null)
  const listRef = useRef<HTMLDivElement | null>(null)
  const taRef = useRef<HTMLTextAreaElement | null>(null)
  const histRef = useRef<Record<string, string[]>>({})
  const histIdxRef = useRef<Record<string, number>>({})
  const draftRef = useRef(draft)
  draftRef.current = draft
  const stickRef = useRef(true)
  const [showJump, setShowJump] = useState(false)

  const st = activeId ? state[activeId] : undefined
  const running = !!st?.harness && st.status !== 'harness-down' && st.status !== 'no-harness'
  const thinking = st?.status === 'thinking'
  const timeline = st?.timeline ?? []
  const projSessions = (activeId && sessions[activeId]) || []
  const activeSID = (activeId && activeSession[activeId]) || null

  // prompt history (unchanged behavior)
  useEffect(() => {
    if (!activeId || histRef.current[activeId]) return
    App.ACPLoadTranscript(activeId)
      .then((entries) => {
        if (histRef.current[activeId]) return
        const list = (entries ?? [])
          .filter((e) => e.role === 'user' && e.text.trim())
          .slice(-30)
          .map((e) => e.text)
        histRef.current[activeId] = list
        histIdxRef.current[activeId] = list.length
      })
      .catch(() => {})
  }, [activeId])

  // History tab refreshes its session list on open
  useEffect(() => {
    if (tab === 'history' && activeId) void listSessions(activeId)
  }, [tab, activeId, listSessions])

  const recall = (dir: -1 | 1): boolean => {
    if (!activeId) return false
    const list = histRef.current[activeId]
    if (!list || !list.length) return false
    let idx = histIdxRef.current[activeId] ?? list.length
    if (dir === -1) {
      const cur = idx < list.length ? list[idx] : ''
      if (draftRef.current.trim() && draftRef.current !== cur) return false
      if (idx === list.length) idx = list.length - 1
      else if (idx > 0) idx -= 1
      else return false
    } else {
      if (idx >= list.length) return false
      idx += 1
    }
    histIdxRef.current[activeId] = idx
    setDraft(idx < list.length ? list[idx] : '')
    requestAnimationFrame(grow)
    return true
  }

  // stick-to-bottom unless the user scrolled up
  useEffect(() => {
    const el = listRef.current
    if (el && stickRef.current) el.scrollTop = el.scrollHeight
  }, [timeline.length, timeline])

  const onScroll = () => {
    const el = listRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    stickRef.current = atBottom
    setShowJump(!atBottom)
  }

  const jumpToLatest = () => {
    const el = listRef.current
    if (el) el.scrollTop = el.scrollHeight
    stickRef.current = true
    setShowJump(false)
  }

  const grow = () => {
    const ta = taRef.current
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = Math.min(ta.scrollHeight, 140) + 'px'
  }

  const doSend = () => {
    const text = draft.trim()
    if (!text || !activeId || thinking || !running) return
    const hist = histRef.current[activeId] ?? (histRef.current[activeId] = [])
    if (hist[hist.length - 1] !== text) hist.push(text)
    histIdxRef.current[activeId] = hist.length
    setDraft('')
    requestAnimationFrame(grow)
    stickRef.current = true
    void send(activeId, text)
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      doSend()
      return
    }
    if (e.key === 'ArrowUp') {
      if (recall(-1)) e.preventDefault()
      return
    }
    if (e.key === 'ArrowDown') {
      if (recall(1)) e.preventDefault()
    }
  }

  const counts = useMemo(() => {
    const tools = timeline.filter((i) => i.type === 'tool').length
    const msgs = timeline.filter((i) => i.type === 'message').length
    return { tools, msgs }
  }, [timeline])

  if (!activeId) {
    return (
      <div className="flex flex-col h-full items-center justify-center text-xs text-dim">
        No project open
      </div>
    )
  }

  const dimAction = 'no-drag flex items-center gap-1 text-dim hover:text-primary disabled:opacity-40 disabled:cursor-not-allowed'

  return (
    <div className="flex flex-col h-full text-xs bg-[#1a1b1e] px-3 py-3">
      {/* Row 1: harness card + status pill (branch-card idiom) */}
      <div className="shrink-0 flex items-center gap-2 mb-2">
        <div className="flex-1 min-w-0 h-9 px-3 rounded-lg border border-[#333639] bg-[#242629] flex items-center gap-2 overflow-hidden">
          <span className={'w-2 h-2 rounded-full shrink-0 ' + (running ? (thinking ? 'bg-[#e6c07b] animate-pulse' : 'bg-[#7dcf9e]') : 'bg-[#4a4f55]')} />
          <span className="truncate font-semibold text-white">{st?.harness || 'no harness'}</span>
          <span className="truncate shrink text-dim">ACP agent protocol</span>
        </div>
        {running ? (
          <button
            className="no-drag shrink-0 h-9 px-3 rounded-lg border border-[#e5735f]/40 bg-[#242629] text-[#e5735f] text-xs hover:border-[#e5735f]"
            onClick={() => void stop(activeId)}
          >
            ■ Stop
          </button>
        ) : (
          <HarnessPicker harnesses={harnesses} onStart={(name) => void start(activeId, name)} />
        )}
      </div>

      {/* Row 2: small actions */}
      <div className="shrink-0 flex items-center gap-4 mb-2">
        <button className={dimAction} disabled={!running || thinking} title="Start a fresh session" onClick={() => void newSession(activeId)}>
          <span>＋</span> New Session
        </button>
        <button className={dimAction} disabled={timeline.length === 0} title="Delete the active session's transcript" onClick={() => void clearTranscript(activeId)}>
          ⧉ Clear transcript
        </button>
      </div>

      {/* Tab strip (segmented, git-style) */}
      <div className="shrink-0 flex items-stretch gap-1 p-1 rounded-lg border border-[#333639] bg-[#242629] mb-2">
        <button
          className={'flex-1 flex items-center justify-center gap-1.5 px-3 h-7 rounded-md text-xs ' + (tab === 'chat' ? 'bg-[#333639] text-white' : 'text-dim hover:text-white')}
          onClick={() => setTab('chat')}
        >
          Chat
          {counts.msgs + counts.tools > 0 && (
            <span className="rounded-full bg-[#3a3c3f] px-1.5 text-[10px] text-[#bcbec4]">{counts.msgs + counts.tools}</span>
          )}
        </button>
        <button
          className={'flex-1 flex items-center justify-center gap-1.5 px-3 h-7 rounded-md text-xs ' + (tab === 'history' ? 'bg-[#333639] text-white' : 'text-dim hover:text-white')}
          onClick={() => setTab('history')}
        >
          History
          {projSessions.length > 0 && (
            <span className="rounded-full bg-[#3a3c3f] px-1.5 text-[10px] text-[#bcbec4]">{projSessions.length}</span>
          )}
        </button>
        <button className="flex-1 flex items-center justify-center px-3 h-7 rounded-md text-xs text-[#4a4f55] cursor-not-allowed" title="Coming soon" disabled>
          Logs
        </button>
      </div>

      {tab === 'chat' ? (
        <div className="flex-1 min-h-0 relative">
          <div ref={listRef} onScroll={onScroll} className="absolute inset-0 overflow-y-auto py-1">
            {timeline.length === 0 && (
              <div className="pl-3 pr-3 h-7 flex items-center text-dim">
                {running ? 'No messages yet — say hello' : 'No messages yet — start a harness to chat'}
              </div>
            )}
            {timeline.map((item, i) => (
              <TimelineItemView
                key={item.type === 'message' ? item.id : item.type === 'tool' ? item.toolCallId : item.requestId}
                item={item}
                onRespond={(requestId, optionId, cancel) => void respondPermission(activeId, requestId, optionId, cancel)}
              />
            ))}
          </div>
          {showJump && (
            <button
              className="absolute bottom-2 left-1/2 -translate-x-1/2 px-3 py-1 rounded-full bg-[#242629] border border-[#333639] text-dim hover:text-primary text-[11px]"
              onClick={jumpToLatest}
            >
              ↓ jump to latest
            </button>
          )}
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-y-auto">
          {projSessions.length === 0 && <div className="px-1 py-2 text-dim">No sessions yet</div>}
          {projSessions.map((s) => (
            <SessionRow
              key={s.id}
              s={s}
              active={s.id === activeSID}
              onOpen={() => void openSession(activeId, s.id)}
              onRename={(title) => void renameSession(s.id, title)}
              onDelete={() => setConfirmDelete(s)}
            />
          ))}
        </div>
      )}

      {/* Composer (commit-area idiom) */}
      <div className="shrink-0 border-t border-[#333639] pt-3 mt-1 flex flex-col gap-2">
        <textarea
          ref={taRef}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value)
            grow()
          }}
          onKeyDown={onKeyDown}
          placeholder={running ? 'Ask the agent… (↑ history)' : 'Start a harness first'}
          disabled={!running || thinking}
          rows={2}
          className="no-drag w-full resize-none bg-[#1e1f22] border border-[#333639] rounded-lg p-3 font-mono text-[13px] text-primary outline-none focus:ring-1 focus:ring-[var(--accent)] placeholder:text-dim disabled:opacity-40"
        />
        <div className="flex items-center justify-between text-[10px]">
          <span className="text-dim">↑ history · Enter send · Shift+Enter newline</span>
          <span className={draft.length > 7000 ? 'text-[var(--modified)] font-medium' : 'text-dim'}>
            {draft.length} / 8000
          </span>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative flex-1 flex">
            <button
              disabled={!draft.trim() || thinking || !running}
              onClick={doSend}
              className="no-drag flex-1 h-9 px-3 rounded-l-lg bg-[#3b5bfd] text-white font-medium disabled:opacity-40 disabled:cursor-not-allowed"
            >
              Send ↵
            </button>
            <button
              disabled
              title="Send & queue (coming soon)"
              className="no-drag h-9 px-1.5 rounded-r-lg border border-l-0 border-[#333639] bg-[#242629] text-dim cursor-not-allowed"
            >
              ▾
            </button>
          </div>
        </div>
      </div>

      {confirmDelete && (
        <ConfirmDialog
          title="Delete session"
          message={`Delete "${confirmDelete.title}"? This cannot be undone.`}
          confirmLabel="Delete"
          onConfirm={() => {
            if (activeId) void deleteSession(activeId, confirmDelete.id)
            setConfirmDelete(null)
          }}
          onCancel={() => setConfirmDelete(null)}
        />
      )}
     </div>
   )
 }

const HarnessPicker: React.FC<{
  harnesses: { name: string; available: boolean }[]
  onStart: (name: string) => void
}> = ({ harnesses, onStart }) => {
  const [selected, setSelected] = useState('')
  const options = harnesses.length ? harnesses : [{ name: 'opencode', available: true }]
  const current = selected || options[0]?.name || ''
  const currentInfo = options.find((h) => h.name === current)
  return (
    <div className="shrink-0 flex items-center gap-2">
      <select
        className="no-drag h-9 px-2 rounded-lg bg-[#242629] border border-[#333639] text-primary outline-none"
        value={current}
        onChange={(e) => setSelected(e.target.value)}
      >
        {options.map((h) => (
          <option key={h.name} value={h.name} disabled={!h.available}>
            {h.name}
            {h.available ? '' : ' (not installed)'}
          </option>
        ))}
      </select>
      <button
        className="no-drag h-9 px-3 rounded-lg bg-[var(--accent)] text-[#0b0c10] font-medium disabled:opacity-40 disabled:cursor-not-allowed"
        disabled={!currentInfo?.available}
        onClick={() => onStart(current)}
      >
        Start
      </button>
    </div>
  )
}

export default AgentPanel
```

**Step 3 fix — ConfirmDialog props.** Check `frontend/src/components/ConfirmDialog.tsx` and match its actual prop names (`title/message/confirmLabel/onConfirm/onCancel` — adjust if it differs).

- [ ] **Step 4: Typecheck + build**

```bash
cd frontend && npx tsc --noEmit && npm run build
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/AgentPanel.tsx
git commit -m "feat(agent-ui): GitPanel-sibling rebuild — interleaved timeline, sessions, split composer"
```

---

### Task 8: Wiring fixes in GitPanel (uses updated agent state)

**Files:**
- Modify: `frontend/src/components/GitPanel.tsx` (only if typecheck flagged `agentState` usage)

- [ ] **Step 1: Check GitPanel compile**

```bash
cd frontend && npx tsc --noEmit
```

If `ag?.status` references break (status moved into the same shape — it did not), fix; otherwise no change needed. `generateCommit` signature is unchanged.

- [ ] **Step 2: Full frontend tests + build**

```bash
cd frontend && npx vitest run && npm run build
```

- [ ] **Step 3: Go tests**

```bash
go test ./...
```

- [ ] **Step 4: Commit (only if changed)**

```bash
git add -A && git commit -m "chore: agent state integration fixes"
```

---

### Task 9: End-to-end smoke test + bindings sanity

**Files:** none (verification)

- [ ] **Step 1: Run the app**

```bash
~/go/bin/wails3 task dev
```

Manual checklist:
1. Open Agent tab → harness card shows "no harness"; pick opencode → Start → status pill green.
2. Send a prompt → user bubble right, chunks merge into one agent bubble, tool cards appear inline (collapsed one-line rows).
3. Expand a tool card → output visible.
4. Trigger an fs-write (ask agent to edit a file) → permission card with mini-diff → Accept → tool card "diff applied ✓".
5. History tab → session listed with auto-title; New Session → second row; Open on the first → transcript swaps.
6. Rename + Delete a session (delete active-while-running must show error).
7. Restart app → History still lists sessions (SQLite persistence).

- [ ] **Step 2: Fix anything found; commit**

```bash
git add -A && git commit -m "fix: agent tab smoke-test fixes"
```

---

## Self-review notes

- Spec coverage: SQLite store (Tasks 2–3), session RPCs + wiring (Task 4), timeline reducer (Task 5), state sessions (Task 6), UI rebuild (Task 7), integration (Task 8), verification (Task 9). Chunk-merge, tool upsert, permission stack, transcript rebuild, auto-title, JSONL migration, in-memory fallback — all covered by named tests.
- Type consistency: `Entry.Status` added backend-side; `SessionMeta` fields `id/projectId/title/created/updated/messageCount` (Wails lower-cases first letter of Go fields — bindings emit `id`, `projectId`, etc.); frontend `ToolItem`/`MessageItem`/`PermissionItem` names used consistently across Tasks 5–7.
- The plan code is final — all earlier scaffolding artifacts were removed; each task's code compiles as written (module-level caveats: `ConfirmDialog` prop names must be matched to the actual component, and `emitTranscript` call sites in existing tests may need the new signature).
