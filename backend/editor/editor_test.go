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
	path := filepath.Join(t.TempDir(), "nope.go")
	if svc.Dirty(path, "") {
		t.Fatal("unknown file with empty current must not be dirty")
	}
	if svc.Dirty(path, "some content") {
		t.Fatal("untracked file must never be dirty, even with non-empty current")
	}
}

func TestSaveErrorCleansTempFile(t *testing.T) {
	dir := t.TempDir()
	// renaming a file onto an existing directory fails, exercising the rename
	// error path
	target := filepath.Join(dir, "subdir")
	os.Mkdir(target, 0o755)

	svc := New()
	if err := svc.Save(target, "new"); err == nil {
		t.Fatal("Save against a directory should fail")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name()[0] == '.' {
			t.Fatalf("temp file %s left behind on error path", e.Name())
		}
	}
}
