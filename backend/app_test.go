package backend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aide/backend/git"
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

func TestIndexFiles_SkipsAndCaps(t *testing.T) {
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
	mkfile("main.go")
	mkfile("src/deep/deeper/best.go") // full recursion, no depth limit
	mkfile("node_modules/pkg/index.js")
	mkfile("dist/bundle.js")
	mkfile(".git/HEAD")
	mkfile(".hidden")

	app, _ := newTestApp(t)
	files, err := app.IndexFiles(root)
	if err != nil {
		t.Fatal(err)
	}

	got := ""
	for _, f := range files {
		got += filepath.ToSlash(f) + "\n"
	}
	if !containsPrefix(got, "/main.go\n") || !containsPrefix(got, "src/deep/deeper/best.go") {
		t.Fatalf("IndexFiles missing expected files:\n%s", got)
	}
	if containsPrefix(got, "node_modules") || containsPrefix(got, "dist/") {
		t.Fatalf("IndexFiles should skip node_modules/dist:\n%s", got)
	}
	if containsPrefix(got, ".git") || containsPrefix(got, ".hidden") {
		t.Fatalf("IndexFiles should skip dotfiles:\n%s", got)
	}
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

func TestOpenProject_EmitsEngineStatus(t *testing.T) {
	app, sink := newTestApp(t)
	p, err := app.OpenProject(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ev := sink.waitFor(t, "engine.status", 5*time.Second)
	payload, ok := ev.payload.(map[string]any)
	if !ok {
		t.Fatalf("unexpected engine.status payload type %T", ev.payload)
	}
	if payload["engine"] != "fswatch" || payload["ok"] != true || payload["projectId"] != p.ID {
		t.Fatalf("bad engine.status payload: %+v", payload)
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

// initRepoGit creates a temp git repo with one committed file, ready for
// Engine/facade tests. Minimal re-creation of the backend/git test fixture.
func initRepoGit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("checkout", "-q", "-b", "master")
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return dir
}

func TestGitFacadeStatusAndStage(t *testing.T) {
	root := initRepoGit(t)
	app, _ := newTestApp(t)
	p, err := app.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.GitStatus("missing-id"); err == nil {
		t.Fatal("unknown project should error")
	}

	st, err := app.GitStatus(p.ID)
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	if st.Branch != "master" {
		t.Fatalf("branch = %q, want master", st.Branch)
	}
	if len(st.Untracked)+len(st.Unstaged)+len(st.Staged) != 0 {
		t.Fatalf("expected clean tree, got %+v", st)
	}

	// untracked file shows up
	newFile := filepath.Join(p.Root, "work.txt")
	if err := os.WriteFile(newFile, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = app.GitStatus(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Untracked) != 1 || st.Untracked[0].Path != "work.txt" {
		t.Fatalf("untracked: %+v", st.Untracked)
	}

	// facade GitLog on initial commit
	log, err := app.GitLog(p.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || strings.TrimSpace(log[0].Message) != "init" {
		t.Fatalf("GitLog: %+v", log)
	}

	// GitBranches
	branches, err := app.GitBranches(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 1 || branches[0] != "master" {
		t.Fatalf("GitBranches: %v", branches)
	}

	// stage → staged change
	if err := app.GitStage(p.ID, []string{"work.txt"}); err != nil {
		t.Fatal(err)
	}
	st, err = app.GitStatus(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 1 || st.Staged[0].Path != "work.txt" {
		t.Fatalf("staged: %+v", st.Staged)
	}

	// diff staged shows the addition
	diff, err := app.GitDiff(p.ID, "work.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Hunks) != 1 || len(diff.Hunks[0].Lines) == 0 {
		t.Fatalf("GitDiff: %+v", diff)
	}

	// commit → clean
	if err := app.GitCommit(p.ID, "work"); err != nil {
		t.Fatal(err)
	}
	st, err = app.GitStatus(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Untracked)+len(st.Unstaged)+len(st.Staged) != 0 {
		t.Fatalf("expected clean after commit, got %+v", st)
	}
	log, err = app.GitLog(p.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 || strings.TrimSpace(log[0].Message) != "work" {
		t.Fatalf("GitLog after commit: %+v", log)
	}

	// modify → unstaged; unstage via stage+unstage roundtrip
	if err := os.WriteFile(newFile, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = app.GitStatus(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Unstaged) != 1 || st.Unstaged[0].Status != 'M' {
		t.Fatalf("unstaged: %+v", st.Unstaged)
	}
	if err := app.GitStage(p.ID, []string{"work.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := app.GitUnstage(p.ID, []string{"work.txt"}); err != nil {
		t.Fatal(err)
	}
	st, err = app.GitStatus(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 0 {
		t.Fatalf("staged should be empty after unstage: %+v", st.Staged)
	}

	// unstaged diff
	diff, err = app.GitDiff(p.ID, "work.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Hunks) == 0 {
		t.Fatalf("GitDiff unstaged empty: %+v", diff)
	}
}

func TestGitFacadeBranchesCommitLog(t *testing.T) {
	root := initRepoGit(t)
	app, _ := newTestApp(t)
	p, err := app.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.GitCreateBranch(p.ID, "feature"); err != nil {
		t.Fatal(err)
	}
	if err := app.GitCheckout(p.ID, "feature"); err != nil {
		t.Fatal(err)
	}
	st, err := app.GitStatus(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "feature" {
		t.Fatalf("branch = %q after checkout, want feature", st.Branch)
	}
	branches, err := app.GitBranches(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 || branches[0] != "feature" || branches[1] != "master" {
		t.Fatalf("GitBranches: %v", branches)
	}
	if err := app.GitCheckout(p.ID, "master"); err != nil {
		t.Fatal(err)
	}
	st, _ = app.GitStatus(p.ID)
	if st.Branch != "master" {
		t.Fatalf("branch = %q, want master", st.Branch)
	}
}

func TestEmitGitStatusPayloadAndThrottle(t *testing.T) {
	root := initRepoGit(t)
	app, sink := newTestApp(t)
	p, err := app.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	pid := p.ID

	countStatus := func() int {
		n := 0
		for _, ev := range sink.snapshot() {
			if ev.name == "git.status" {
				n++
			}
		}
		return n
	}

	app.emitGitStatus(pid)
	ev := sink.waitFor(t, "git.status", 5*time.Second)
	payload, ok := ev.payload.(map[string]any)
	if !ok {
		t.Fatalf("unexpected git.status payload type %T", ev.payload)
	}
	if payload["projectId"] != pid {
		t.Fatalf("bad projectId: %+v", payload)
	}
	st, ok := payload["status"].(git.Status)
	if !ok {
		t.Fatalf("bad status type %T", payload["status"])
	}
	if st.Branch != "master" {
		t.Fatalf("branch = %q", st.Branch)
	}

	// immediate second call inside throttle window is dropped
	app.emitGitStatus(pid)
	if n := countStatus(); n != 1 {
		t.Fatalf("throttle should drop immediate second emit, got %d events", n)
	}

	// reset throttle map → next emit passes
	app.mu.Lock()
	delete(app.lastGitEmit, pid)
	app.mu.Unlock()
	app.emitGitStatus(pid)
	if n := countStatus(); n != 2 {
		t.Fatalf("expected 2 git.status events after throttle reset, got %d", n)
	}
}

func TestEmitGitStatusErrorEvent(t *testing.T) {
	// project root without a git dir → New fails → git.error
	app, sink := newTestApp(t)
	p, err := app.OpenProject(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.emitGitStatus(p.ID)
	ev := sink.waitFor(t, "git.error", 5*time.Second)
	payload, ok := ev.payload.(map[string]string)
	if !ok {
		t.Fatalf("unexpected git.error payload type %T", ev.payload)
	}
	if payload["projectId"] != p.ID || payload["message"] == "" {
		t.Fatalf("bad git.error payload: %+v", payload)
	}
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
