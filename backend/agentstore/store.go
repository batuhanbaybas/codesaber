package agentstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry kinds stored in transcripts.
const (
	KindText  = "text"
	KindTool  = "tool"
	KindError = "error"
)

// Entry is one line of a project's chat transcript.
type Entry struct {
	Role   string    `json:"role"`
	Text   string    `json:"text"`
	ToolID string    `json:"toolId,omitempty"`
	Kind   string    `json:"kind"`
	When   time.Time `json:"when"`
}

// Store persists chat transcripts as JSONL under
// <user-config-dir>/codesaber/chats/<projectID>.jsonl. Safe for concurrent use.
type Store struct {
	userConfigDir func() (string, error)
	userHomeDir   func() (string, error)

	mu sync.Mutex
}

// NewStore returns a Store using os.UserConfigDir/os.UserHomeDir.
func NewStore() *Store {
	return &Store{userConfigDir: os.UserConfigDir, userHomeDir: os.UserHomeDir}
}

// Append adds an entry (stamp timestamp if zero) to projectID's transcript.
func (s *Store) Append(projectID string, e Entry) error {
	if !validKind(e.Kind) {
		return fmt.Errorf("agentstore: unknown entry kind %q", e.Kind)
	}
	if e.When.IsZero() {
		e.When = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.appendLocked(projectID, e); err != nil {
		return err
	}
	return nil
}

// appendLocked appends one JSONL line. Callers must hold s.mu; s.path is
// resolved once so injected dir hooks stay consistent within a call.
func (s *Store) appendLocked(projectID string, e Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("agentstore: marshal: %w", err)
	}
	p := s.path(projectID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("agentstore: mkdir: %w", err)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("agentstore: open: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("agentstore: write: %w", err)
	}
	return nil
}

// Read returns all entries for projectID; a missing file reads as empty.
func (s *Store) Read(projectID string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path(projectID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("agentstore: open: %w", err)
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("agentstore: decode transcript: %w", err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("agentstore: read transcript: %w", err)
	}
	return out, nil
}

// Clear deletes projectID's transcript file.
func (s *Store) Clear(projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path(projectID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("agentstore: clear: %w", err)
	}
	return nil
}

// path resolves <config|fallback>/codesaber/chats/<projectID>.jsonl.
func (s *Store) path(projectID string) string {
	base, err := s.userConfigDir()
	if err != nil {
		if home, herr := s.userHomeDir(); herr == nil {
			base = filepath.Join(home, ".config")
		} else {
			base = os.TempDir()
		}
	}
	return filepath.Join(base, "codesaber", "chats", sanitize(projectID)+".jsonl")
}

// sanitize guards against path traversal in project IDs.
func sanitize(projectID string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", "..", "_")
	return r.Replace(projectID)
}

func validKind(k string) bool {
	switch k {
	case KindText, KindTool, KindError:
		return true
	}
	return false
}
