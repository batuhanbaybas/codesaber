package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveIsAtomicViaTempRename(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	os.WriteFile(file, []byte("old"), 0o644)

	svc := New()
	svc.Track(file, "old")
	if !svc.Dirty(file, "changed") {
		t.Fatal("expected dirty when current differs from saved")
	}
	if svc.Dirty(file, "old") {
		t.Fatal("expected clean when current matches saved")
	}

	if err := svc.Save(file, "new"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, _ := os.ReadFile(file)
	if string(b) != "new" {
		t.Fatalf("file = %q", b)
	}
	if svc.Dirty(file, "new") {
		t.Fatal("expected clean after save")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name()[0] == '.' {
			t.Fatalf("temp file %s left behind", e.Name())
		}
	}
}

func TestDirtyUnknownFile(t *testing.T) {
	svc := New()
	if svc.Dirty(filepath.Join(t.TempDir(), "nope.go"), "") {
		t.Fatal("unknown file with empty current must not be dirty")
	}
}
