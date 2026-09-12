package git

import (
	"os"

	git2 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Engine struct {
	dir string
	r   *git2.Repository
	wt  *git2.Worktree
}

func New(dir string) (*Engine, error) {
	r, err := git2.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	wt, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	return &Engine{dir: dir, r: r, wt: wt}, nil
}

func mapStatus(c git2.StatusCode) ChangeStatus {
	switch c {
	case git2.Added:
		return ChangeAdded
	case git2.Deleted:
		return ChangeDeleted
	default:
		return ChangeModified
	}
}

func (e *Engine) Status() (Status, error) {
	st := Status{Branch: "(none)", Staged: []Change{}, Unstaged: []Change{}, Untracked: []Change{}}
	if head, err := e.r.Head(); err == nil {
		st.Branch = head.Name().Short()
	}
	files, err := e.wt.Status()
	if err != nil {
		return st, err
	}
	for path, fs := range files {
		switch {
		case fs.Staging == git2.Untracked:
			st.Untracked = append(st.Untracked, Change{Path: path, Status: ChangeUntracked})
		case fs.Staging != git2.Unmodified:
			st.Staged = append(st.Staged, Change{Path: path, Status: mapStatus(fs.Staging)})
		}
		if fs.Worktree == git2.Modified || fs.Worktree == git2.Deleted {
			st.Unstaged = append(st.Unstaged, Change{Path: path, Status: mapStatus(fs.Worktree)})
		}
	}
	return st, nil
}

func (e *Engine) Stage(paths []string) error {
	for _, p := range paths {
		if _, err := os.Stat(e.wt.Filesystem.Join(e.dir, p)); err != nil {
			if _, err := e.wt.Remove(p); err != nil {
				return err
			}
			continue
		}
		if _, err := e.wt.Add(p); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) Unstage(paths []string) error {
	for _, p := range paths {
		if err := e.wt.Reset(&git2.ResetOptions{Files: []string{p}}); err != nil {
			return err
		}
	}
	return nil
}

var _ = object.Signature{}
