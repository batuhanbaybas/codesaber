package agentstore

import (
	"errors"
	"sync"
	"testing"
)

func TestStoreAppendReadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	e1 := Entry{Role: "user", Text: "hello", Kind: KindText}
	e2 := Entry{Role: "agent", Text: "hi", Kind: KindText}
	e3 := Entry{Role: "agent", Text: "read f.go", ToolID: "tool-1", Kind: KindTool}

	if err := s.Append("proj-1", e1); err != nil {
		t.Fatalf("Append e1: %v", err)
	}
	if err := s.Append("proj-1", e2); err != nil {
		t.Fatalf("Append e2: %v", err)
	}
	if err := s.Append("proj-1", e3); err != nil {
		t.Fatalf("Append e3: %v", err)
	}

	got, err := s.Read("proj-1")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Role != "user" || got[0].Text != "hello" || got[0].Kind != KindText {
		t.Errorf("got[0] = %#v, want user/hello", got[0])
	}
	if got[2].ToolID != "tool-1" || got[2].Kind != KindTool {
		t.Errorf("got[2] = %#v, want tool entry", got[2])
	}
	for i, e := range got {
		if e.When.IsZero() {
			t.Errorf("got[%d].When is zero, want appended timestamp", i)
		}
		if i > 0 && got[i].When.Before(got[i-1].When) {
			t.Errorf("got[%d].When out of order", i)
		}
	}
}

func TestStoreReadMissingFileReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Read("nope")
	if err != nil {
		t.Fatalf("Read missing: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %#v, want empty", got)
	}
}

func TestStoreClear(t *testing.T) {
	s := newTestStore(t)
	if err := s.Append("proj-1", Entry{Role: "user", Text: "x", Kind: KindText}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Clear("proj-1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	got, err := s.Read("proj-1")
	if err != nil {
		t.Fatalf("Read after Clear: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %#v, want empty", got)
	}
}

func TestStoreProjectsAreIsolated(t *testing.T) {
	s := newTestStore(t)
	if err := s.Append("a", Entry{Role: "user", Text: "a-entry", Kind: KindText}); err != nil {
		t.Fatalf("Append a: %v", err)
	}
	if err := s.Append("b", Entry{Role: "user", Text: "b-entry", Kind: KindText}); err != nil {
		t.Fatalf("Append b: %v", err)
	}
	gotA, _ := s.Read("a")
	gotB, _ := s.Read("b")
	if len(gotA) != 1 || gotA[0].Text != "a-entry" {
		t.Fatalf("a entries = %#v", gotA)
	}
	if len(gotB) != 1 || gotB[0].Text != "b-entry" {
		t.Fatalf("b entries = %#v", gotB)
	}
}

func TestStoreConcurrentAppends(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if err := s.Append("p", Entry{Role: "user", Text: "m", Kind: KindText}); err != nil {
					t.Errorf("Append: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	got, err := s.Read("p")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 40 {
		t.Fatalf("len = %d, want 40", len(got))
	}
}

func TestStoreAppendRejectsUnknownKind(t *testing.T) {
	s := newTestStore(t)
	if err := s.Append("p", Entry{Role: "user", Text: "x", Kind: "weird"}); err == nil {
		t.Fatal("want error for unknown kind, got nil")
	}
}

func TestStoreFallbackDirWhenUserConfigFails(t *testing.T) {
	s := NewStore()
	s.userConfigDir = func() (string, error) { return "", errors.New("no config") }
	home := t.TempDir()
	s.userHomeDir = func() (string, error) { return home, nil }
	if err := s.Append("p", Entry{Role: "user", Text: "x", Kind: KindText}); err != nil {
		t.Fatalf("Append with fallback: %v", err)
	}
	if got, _ := s.Read("p"); len(got) != 1 {
		t.Fatalf("got = %#v, want 1 entry", got)
	}
}

// newTestStore builds a Store rooted at a temp dir.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	dir := t.TempDir()
	s.userConfigDir = func() (string, error) { return dir, nil }
	return s
}
