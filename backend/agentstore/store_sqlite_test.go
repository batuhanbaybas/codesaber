package agentstore

import (
	"errors"
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
