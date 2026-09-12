package git

import (
	"os"
	"path/filepath"
	"testing"

	git2 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r, err := git2.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := r.Worktree()
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("one"), 0o644)
	w.Add("init.txt")
	_, err = w.Commit("init", &git2.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestStatusTracksStagedUnstagedUntracked(t *testing.T) {
	dir := initRepo(t)
	os.WriteFile(filepath.Join(dir, "modified.txt"), []byte("mod"), 0o644)
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("one-t"), 0o644)

	e, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	st, err := e.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Branch == "" {
		t.Fatal("branch empty")
	}
	if len(st.Untracked) != 1 || st.Untracked[0].Path != "modified.txt" {
		t.Fatalf("untracked: %+v", st.Untracked)
	}
	if len(st.Unstaged) != 1 || st.Unstaged[0].Path != "init.txt" || st.Unstaged[0].Status != ChangeModified {
		t.Fatalf("unstaged: %+v", st.Unstaged)
	}
	if len(st.Staged) != 0 {
		t.Fatalf("staged should be empty: %+v", st.Staged)
	}

	f, err := os.OpenFile(filepath.Join(dir, "init.txt"), os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("changed")
	f.Close()

	if err := e.Stage([]string{"init.txt", "modified.txt"}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	st, _ = e.Status()
	if len(st.Staged) != 2 {
		t.Fatalf("staged after stage: %+v", st.Staged)
	}
	if err := e.Unstage([]string{"modified.txt"}); err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	st, _ = e.Status()
	if len(st.Staged) != 1 || st.Staged[0].Path != "init.txt" {
		t.Fatalf("staged after unstage: %+v", st.Staged)
	}
	if len(st.Unstaged) != 0 || st.Untracked[0].Path != "modified.txt" {
		t.Fatalf("post-unstage rest: %+v %+v", st.Unstaged, st.Untracked)
	}
}
