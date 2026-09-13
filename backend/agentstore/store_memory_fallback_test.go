package agentstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestOpenStoreAtUnwritableDirFallsBackToMemory verifies the last-resort
// fallback: when the on-disk db cannot be opened (here, the "dir" is a file,
// so MkdirAll fails), the store still boots in-memory and Append/Read work
// for the lifetime of the process.
func TestOpenStoreAtUnwritableDirFallsBackToMemory(t *testing.T) {
	bogus := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(bogus, []byte("i am a file"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	s, err := OpenStoreAt(bogus)
	if err != nil {
		t.Fatalf("OpenStoreAt with file-as-dir: %v (want in-memory fallback)", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	now := time.Now()
	if err := s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "hello", When: now}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	entries, err := s.Read("s1")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 || entries[0].Text != "hello" || entries[0].Role != "user" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestOpenStoreInMemoryRoundTrip(t *testing.T) {
	s, err := OpenStoreInMemory()
	if err != nil {
		t.Fatalf("OpenStoreInMemory: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	now := time.Now()
	if err := s.Append("s1", "p1", Entry{Role: "agent", Kind: KindText, Text: "hi", When: now}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	entries, err := s.Read("s1")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 || entries[0].Text != "hi" {
		t.Fatalf("entries = %#v", entries)
	}

	// In-memory store still supports session ops.
	metas, err := s.ListSessions("p1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(metas) != 1 || metas[0].ID != "s1" {
		t.Fatalf("metas = %#v", metas)
	}
	if err := s.ClearAll("p1"); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	entries, err = s.Read("s1")
	if err != nil {
		t.Fatalf("Read after ClearAll: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries after ClearAll = %#v, want empty", entries)
	}
}
