package project

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecentsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recents.json")
	s := NewStore(path)
	s.Remember("abc", "/tmp/proj1", "main")
	s.Remember("def", "/tmp/proj2", "feat/x")

	got := NewStore(path).List()
	if len(got) != 2 {
		t.Fatalf("got %d recents, want 2", len(got))
	}
	if got[0].ID != "def" || got[0].Branch != "feat/x" {
		t.Fatalf("most recent must be first: %+v", got[0])
	}
	s.Forget("/tmp/proj1")
	if len(NewStore(path).List()) != 1 {
		t.Fatal("forget failed")
	}
	if got := NewStore(path).List(); got[0].Root != "/tmp/proj2" {
		t.Fatalf("wrong survivor: %+v", got[0])
	}
}

func TestRememberRefreshesExistingRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recents.json")
	s := NewStore(path)
	s.Remember("old-id", "/tmp/proj", "main")
	first := s.List()[0]

	time.Sleep(5 * time.Millisecond)
	s.Remember("new-id", "/tmp/proj", "feat/x")
	got := s.List()
	if len(got) != 1 {
		t.Fatalf("got %d recents, want 1 (no duplicate)", len(got))
	}
	r := got[0]
	if r.ID != "new-id" || r.Branch != "feat/x" {
		t.Fatalf("entry not refreshed: %+v", r)
	}
	if !r.LastUsed.After(first.LastUsed) {
		t.Fatalf("LastUsed not updated: %+v vs %+v", r.LastUsed, first.LastUsed)
	}

	// refreshed entry sorts first among multiple
	s.Remember("other-id", "/tmp/other", "dev")
	time.Sleep(5 * time.Millisecond)
	s.Remember("newer-id", "/tmp/proj", "main")
	if s.List()[0].Root != "/tmp/proj" {
		t.Fatalf("refreshed entry should sort first: %+v", s.List()[0])
	}
}

func TestForgetUnknownRootIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recents.json")
	s := NewStore(path)
	s.Remember("id", "/tmp/proj", "main")
	s.Forget("/tmp/unknown")
	if len(s.List()) != 1 {
		t.Fatal("forget of unknown root must not remove entries")
	}
}
