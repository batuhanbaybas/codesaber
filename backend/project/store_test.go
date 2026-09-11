package project

import (
	"path/filepath"
	"testing"
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
	s.Forget("abc")
	if len(NewStore(path).List()) != 1 {
		t.Fatal("forget failed")
	}
}
