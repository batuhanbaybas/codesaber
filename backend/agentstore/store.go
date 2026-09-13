// Package agentstore persists agent chat transcripts in a single SQLite
// database (modernc.org/sqlite — pure Go, no CGO) at
// <user-config-dir>/codesaber/chats/agent.db. Sessions are rows; entries are
// ordered per session by seq. Legacy per-project JSONL files are imported on
// first open and renamed .imported.
package agentstore

import (
	"database/sql"
	"encoding/json"
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
  "when"     TEXT NOT NULL
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
	return s.appendEntryLocked(sessionID, projectID, e)
}

// appendEntryLocked persists one entry. Callers must hold s.mu. For tool
// entries with a ToolID it upserts by (session, tool_id); otherwise it
// appends with the next seq. If the session row does not exist it is created
// (projectID required then). The first user text entry auto-titles an
// untitled session (first line, ≤60 chars).
func (s *Store) appendEntryLocked(sessionID, projectID string, e Entry) error {
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
		if _, err := tx.Exec(
			`INSERT INTO entries (session_id, seq, role, kind, tool_id, status, text, "when")
			 VALUES (?, COALESCE((SELECT MAX(seq)+1 FROM entries WHERE session_id = ?), 0), ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(session_id, tool_id) WHERE tool_id <> ''
			 DO UPDATE SET status = excluded.status, text = excluded.text, "when" = excluded."when"`,
			sessionID, sessionID, e.Role, e.Kind, e.ToolID, e.Status, e.Text, e.When.Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("agentstore: upsert tool entry: %w", err)
		}
	} else {
		if _, err := tx.Exec(
			`INSERT INTO entries (session_id, seq, role, kind, tool_id, status, text, "when")
			 VALUES (?, COALESCE((SELECT MAX(seq)+1 FROM entries WHERE session_id = ?), 0), ?, ?, ?, '', ?, ?)`,
			sessionID, sessionID, e.Role, e.Kind, e.ToolID, e.Text, e.When.Format(time.RFC3339Nano),
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
		`SELECT role, kind, tool_id, status, text, "when" FROM entries WHERE session_id = ? ORDER BY seq`,
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
	return s.renameSessionLocked(sessionID, title)
}

// renameSessionLocked sets a session's title. Callers must hold s.mu.
func (s *Store) renameSessionLocked(sessionID, title string) error {
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
			if err := s.appendEntryLocked(sid, projectID, e); err != nil {
				return fmt.Errorf("agentstore: import %s: %w", base, err)
			}
			if e.Kind == KindText && e.Role == "user" && title == "Imported chat" {
				title = deriveTitle(e.Text)
			}
		}
		_ = s.renameSessionLocked(sid, title)
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
