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
