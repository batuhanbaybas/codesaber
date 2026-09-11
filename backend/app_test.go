package backend

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeSink struct {
	mu     sync.Mutex
	events []fakeEvent
}

type fakeEvent struct {
	name    string
	payload any
}

// Emit may be called from multiple goroutines (e.g. the fswatch forwarder),
// so the fake serializes access to its event log.
func (f *fakeSink) Emit(name string, payload any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, fakeEvent{name: name, payload: payload})
}

func (f *fakeSink) snapshot() []fakeEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeEvent(nil), f.events...)
}

func (f *fakeSink) waitFor(t *testing.T, name string, timeout time.Duration) fakeEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, ev := range f.snapshot() {
			if ev.name == name {
				return ev
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for event %q", name)
	return fakeEvent{}
}

func newTestApp(t *testing.T) (*App, *fakeSink) {
	t.Helper()
	sink := &fakeSink{}
	app := NewWith(sink, filepath.Join(t.TempDir(), "recents.json"))
	t.Cleanup(func() {
		for _, p := range app.reg.List() {
			app.CloseWatcher(p.ID)
		}
	})
	return app, sink
}

func TestListTree_DepthAndDotfiles(t *testing.T) {
	root := t.TempDir()
	mkfile := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkfile("src/main.go")          // depth 2, inside dir
	mkfile("src/deep/deepest.txt") // depth 3 → beyond guard
	mkfile(".env")                 // allowed dotfile depth 1
	mkfile(".gitignore")           // allowed dotfile depth 1
	mkfile("src/.hidden.go")       // skipped dotfile at depth 2
	if err := os.Mkdir(filepath.Join(root, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}

	app, _ := newTestApp(t)
	entries, err := app.ListTree(root)
	if err != nil {
		t.Fatal(err)
	}

	got := ""
	for _, e := range entries {
		got += filepath.ToSlash(e.Path) + "\n"
	}
	wantSubstrings := []string{"/src\n", "src/main.go", "/.env", "/.gitignore"}
	for _, want := range wantSubstrings {
		if !containsPrefix(got, want) {
			t.Fatalf("ListTree missing %q in:\n%s", want, got)
		}
	}
	if containsPrefix(got, "hidden.go") {
		t.Fatalf("dotfile .hidden.go should be skipped in:\n%s", got)
	}
	if containsPrefix(got, "deep/") {
		t.Fatalf("depth 3 should not be walked in:\n%s", got)
	}

	// dirs-first: src (dir) must precede .env (file)
	srcIdx := indexPrefix(got, "/src\n")
	envIdx := indexPrefix(got, "/.env")
	if srcIdx == -1 || envIdx == -1 || srcIdx > envIdx {
		t.Fatalf("dirs should come first:\n%s", got)
	}
}

func containsPrefix(list, want string) bool { return indexPrefix(list, want) != -1 }

func indexPrefix(list, want string) int {
	for i := 0; i+len(want) <= len(list); i++ {
		if list[i:i+len(want)] == want {
			return i
		}
	}
	return -1
}

func TestFileSystemChangeEvents(t *testing.T) {
	app, sink := newTestApp(t)
	p, err := app.OpenProject(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sinkEmit(sink.snapshot(), "project.added"); !ok {
		t.Fatal("expected project.added event")
	}
	target := filepath.Join(p.Root, "hello.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := sink.waitFor(t, "fs.change", 5*time.Second)
	payload, ok := ev.payload.(map[string]string)
	if !ok {
		t.Fatalf("unexpected fs.change payload type %T", ev.payload)
	}
	if payload["projectId"] != p.ID || payload["path"] != target || payload["op"] == "" {
		t.Fatalf("bad fs.change payload: %+v", payload)
	}
}

func sinkEmit(events []fakeEvent, name string) (fakeEvent, bool) {
	for _, ev := range events {
		if ev.name == name {
			return ev, true
		}
	}
	return fakeEvent{}, false
}

func TestRemoveProject_StopsWatcher(t *testing.T) {
	app, sink := newTestApp(t)
	p, err := app.OpenProject(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RemoveProject(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := sinkEmit(sink.snapshot(), "project.removed"); !ok {
		t.Fatal("expected project.removed event")
	}
	if _, err := app.reg.Get(p.ID); err == nil {
		t.Fatal("project should be removed from registry")
	}
}
