package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git2 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// syncFixture creates a repo and a local clone of it (as the "origin" remote
// path), giving both repos an identical (shared) root commit.
func syncFixture(t *testing.T) (dir, remoteDir string) {
	t.Helper()
	dir = initRepo(t)
	originDir := filepath.Join(t.TempDir(), "origin")
	if _, err := git2.PlainClone(originDir, false, &git2.CloneOptions{URL: dir}); err != nil {
		t.Fatalf("clone: %v", err)
	}
	setRemoteRef(t, dir, "origin", originDir)
	r, _ := git2.PlainOpen(dir)
	if _, err := r.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{originDir}}); err != nil {
		t.Fatal(err)
	}
	return dir, originDir
}

func setRemoteRef(t *testing.T, dir, remote, fromDir string) {
	t.Helper()
	r, err := git2.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	src, err := git2.PlainOpen(fromDir)
	if err != nil {
		t.Fatal(err)
	}
	srcHead, err := src.Head()
	if err != nil {
		t.Fatal(err)
	}
	err = r.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewRemoteReferenceName(remote, srcHead.Name().Short()), srcHead.Hash()))
	if err != nil {
		t.Fatal(err)
	}
}

func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	r, err := git2.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := r.Worktree()
	os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	if _, err := w.Add(name); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit("c:"+name, &git2.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t"},
	}); err != nil {
		t.Fatal(err)
	}
}

func newEngine(t *testing.T, dir string) *Engine {
	t.Helper()
	e, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func TestAheadBehindFastForward(t *testing.T) {
	dir, remoteDir := syncFixture(t)
	// remote gains one commit the local repo doesn't have yet
	commitFile(t, remoteDir, "init.txt", "remote-updated\n")

	e := newEngine(t, dir)
	if err := e.Fetch(); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	ahead, behind, err := e.AheadBehind("origin")
	if err != nil {
		t.Fatalf("AheadBehind: %v", err)
	}
	if ahead != 0 || behind != 1 {
		t.Fatalf("fastforward behind: ahead=%d behind=%d", ahead, behind)
	}

	// local commits one on top → diverged
	commitFile(t, dir, "init.txt", "local-updated\n")
	ahead, behind, err = newEngine(t, dir).AheadBehind("origin")
	if err != nil {
		t.Fatalf("AheadBehind: %v", err)
	}
	if ahead != 1 || behind != 1 {
		t.Fatalf("diverged: ahead=%d behind=%d", ahead, behind)
	}
}

func TestAheadBehindNoUpstream(t *testing.T) {
	dir := initRepo(t)
	_, _, err := newEngine(t, dir).AheadBehind("origin")
	if !errors.Is(err, ErrNoUpstream) {
		t.Fatalf("want ErrNoUpstream, got %v", err)
	}
}

func TestFetchNoRemote(t *testing.T) {
	dir := initRepo(t)
	err := newEngine(t, dir).Fetch()
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "remote") == false {
		t.Fatalf("want clear remote error, got %v", err)
	}
}

func TestFetchLocalRemote(t *testing.T) {
	dir := initRepo(t)
	remoteDir := initRepo(t)
	commitFile(t, remoteDir, "init.txt", "from-remote")
	r, _ := git2.PlainOpen(dir)
	if _, err := r.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteDir},
	}); err != nil {
		t.Fatal(err)
	}
	if err := newEngine(t, dir).Fetch(); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	ref, err := r.Reference(plumbing.NewRemoteReferenceName("origin", "master"), false)
	if err != nil {
		t.Fatalf("fetch ref: %v", err)
	}
	srcr, _ := git2.PlainOpen(remoteDir)
	srcHead, _ := srcr.Head()
	if ref.Hash() != srcHead.Hash() {
		t.Fatalf("fetched %s want %s", ref.Hash(), srcHead.Hash())
	}
}

func TestAheadBehindAfterFetch(t *testing.T) {
	dir, remoteDir := syncFixture(t)
	commitFile(t, remoteDir, "init.txt", "new-remote\n")
	e := newEngine(t, dir)
	if err := e.Fetch(); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	ahead, behind, err := e.AheadBehind("origin")
	if err != nil {
		t.Fatalf("AheadBehind: %v", err)
	}
	if ahead != 0 || behind != 1 {
		t.Fatalf("after fetch: ahead=%d behind=%d", ahead, behind)
	}
}

func TestPushFastForward(t *testing.T) {
	dir := initRepo(t)
	remoteDir := filepath.Join(t.TempDir(), "origin.git")
	if _, err := git2.PlainInit(remoteDir, true); err != nil {
		t.Fatal(err)
	}
	r, _ := git2.PlainOpen(dir)
	if _, err := r.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteDir}}); err != nil {
		t.Fatal(err)
	}
	commitFile(t, dir, "init.txt", "local-forward\n")
	if err := newEngine(t, dir).Push(); err != nil {
		t.Fatalf("Push: %v", err)
	}
	rar, _ := git2.PlainOpen(remoteDir)
	rh, _ := rar.Head()
	lh, _ := r.Head()
	if rh.Hash() != lh.Hash() {
		t.Fatalf("remote head %s want %s", rh.Hash(), lh.Hash())
	}
	// remote tracking ref must be updated too → ahead 0 behind 0
	ahead, behind, err := newEngine(t, dir).AheadBehind("origin")
	if err != nil {
		t.Fatalf("AheadBehind: %v", err)
	}
	if ahead != 0 || behind != 0 {
		t.Fatalf("after push: ahead=%d behind=%d", ahead, behind)
	}
}

func TestPushNoRemote(t *testing.T) {
	dir := initRepo(t)
	err := newEngine(t, dir).Push()
	if err == nil {
		t.Fatal("want error")
	}
}

func TestStatusUntrackedSize(t *testing.T) {
	dir := initRepo(t)
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte("12345"), 0o644)
	e := newEngine(t, dir)
	st, err := e.Status()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range st.Untracked {
		if c.Path == "big.txt" {
			found = true
			if c.Size != 5 {
				t.Fatalf("big.txt size=%d want 5", c.Size)
			}
		}
	}
	if !found {
		t.Fatal("big.txt missing from untracked")
	}
}

func TestStatsCountLines(t *testing.T) {
	dir := initRepo(t)
	// staged modification of init.txt: 1 → 3 lines (+2 additions, -1)
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("one\ntwo\nthree\n"), 0o644)
	e := newEngine(t, dir)
	if err := e.Stage([]string{"init.txt"}); err != nil {
		t.Fatal(err)
	}
	// unstaged: 3 lines → "four\n" on disk: deletes 3, adds 1
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("four\n"), 0o644)
	res, err := e.Stats([]string{"init.txt"}, false)
	if err != nil {
		t.Fatalf("Stats unstaged: %v", err)
	}
	if got := res["init.txt"]; got[0] != 1 || got[1] != 3 {
		t.Fatalf("unstaged stats: %+v", got)
	}
}
