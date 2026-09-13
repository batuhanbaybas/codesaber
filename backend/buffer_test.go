package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBufferFacadeKeepsDraftWithoutSaving(t *testing.T) {
	app, _ := newTestApp(t)
	root := t.TempDir()
	p, err := app.reg.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "file.go")
	if err := os.WriteFile(path, []byte("saved"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := app.BufferRead(p.ID, path)
	if err != nil {
		t.Fatal(err)
	}
	b.Content, b.Version = "unsaved", 1
	if err := app.BufferUpdate(p.ID, path, b); err != nil {
		t.Fatal(err)
	}
	got, err := app.BufferRead(p.ID, filepath.Join(root, ".", "file.go"))
	if err != nil || got != b {
		t.Fatalf("BufferRead lost draft: %+v, %v", got, err)
	}
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != "saved" {
		t.Fatalf("draft update wrote to disk: %q, %v", disk, err)
	}
	app.BufferClose(p.ID, path, b.ID)
	reopened, err := app.BufferRead(p.ID, path)
	if err != nil || reopened.ID == b.ID || reopened.Content != "saved" {
		t.Fatalf("closed draft was restored: %+v, %v", reopened, err)
	}
	if err := app.RemoveProject(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.buf.Buffer(p.ID, path); ok {
		t.Fatal("RemoveProject retained buffer")
	}
	if _, err := app.BufferRead(p.ID, path); err == nil {
		t.Fatal("read accepted removed project")
	}
	if err := app.BufferUpdate(p.ID, path, reopened); err == nil {
		t.Fatal("update accepted removed project")
	}
}

func TestBufferReadRespectsFileReadErrors(t *testing.T) {
	app, _ := newTestApp(t)
	p, err := app.reg.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.BufferRead("missing", p.Root); err == nil {
		t.Fatal("unknown project accepted")
	}
	for _, path := range []string{p.Root, filepath.Join(p.Root, "missing.go")} {
		if _, err := app.BufferRead(p.ID, path); err == nil {
			t.Fatalf("read succeeded for %s", path)
		}
		if _, ok := app.buf.Buffer(p.ID, path); ok {
			t.Fatalf("failed read retained an empty buffer for %s", path)
		}
	}
}
