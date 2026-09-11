package fswatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherReportsWrites(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.txt")
	os.WriteFile(file, []byte("a"), 0o644)

	w, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()
	events := w.Events()

	os.WriteFile(file, []byte("bb"), 0o644)

	select {
	case ev := <-events:
		if filepath.Base(ev.Path) != "x.txt" {
			t.Fatalf("unexpected path %q", ev.Path)
		}
		if ev.Op != "write" && ev.Op != "create" {
			t.Fatalf("unexpected op %q", ev.Op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no fs event within 2s")
	}
}

func TestCloseWithoutReaderExitsForwarder(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "spam.txt")
	os.WriteFile(file, []byte("0"), 0o644)

	w, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = w.Events() // subscribe, never read
	for i := 0; i < 200; i++ {
		b, _ := os.ReadFile(file)
		os.WriteFile(file, append(b, []byte("data\n")...), 0o644)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-w.internalDone():
	case <-time.After(2 * time.Second):
		t.Fatal("forwarder did not exit after Close")
	}
	// a reader drains buffered events, then gets the closed sentinel
	for {
		select {
		case _, ok := <-w.Events():
			if !ok {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatal("events channel not closed within 2s of Close")
		}
	}
}
