package git

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	git2 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	utilsdiff "github.com/go-git/go-git/v5/utils/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
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

func parseAuthor(author string) *object.Signature {
	name, email, _ := strings.Cut(author, "<")
	email = strings.TrimSuffix(strings.TrimSpace(email), ">")
	return &object.Signature{Name: strings.TrimSpace(name), Email: email}
}

func (e *Engine) Commit(msg, author string) error {
	files, err := e.wt.Status()
	if err != nil {
		return err
	}
	if files.IsClean() {
		return fmt.Errorf("nothing to commit")
	}
	_, err = e.wt.Commit(msg, &git2.CommitOptions{Author: parseAuthor(author)})
	return err
}

func (e *Engine) Log(n int) ([]LogEntry, error) {
	head, err := e.r.Head()
	if err != nil {
		return nil, err
	}
	entries := []LogEntry{}
	hash := head.Hash()
	for len(entries) < n {
		commit, err := e.r.CommitObject(hash)
		if err != nil {
			return nil, err
		}
		entries = append(entries, LogEntry{
			Hash:    hash.String(),
			Author:  commit.Author.Name,
			Message: commit.Message,
			Time:    commit.Author.When.Format("2006-01-02 15:04:05"),
		})
		if commit.NumParents() == 0 {
			break
		}
		hash = commit.ParentHashes[0]
	}
	return entries, nil
}

func (e *Engine) CreateBranch(name string) error {
	head, err := e.r.Head()
	if err != nil {
		return err
	}
	ref := plumbing.NewBranchReferenceName(name)
	if err := e.r.CreateBranch(&config.Branch{
		Name:   name,
		Remote: ref.String(),
		Merge:  ref,
	}); err != nil {
		return err
	}
	return e.r.Storer.SetReference(plumbing.NewHashReference(ref, head.Hash()))
}

func (e *Engine) CheckoutBranch(name string) error {
	return e.wt.Checkout(&git2.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
	})
}

func blobContent(e *Engine, h plumbing.Hash) []byte {
	if h.IsZero() {
		return nil
	}
	blob, err := e.r.BlobObject(h)
	if err != nil {
		return nil
	}
	r, err := blob.Reader()
	if err != nil {
		return nil
	}
	defer r.Close()
	data, _ := io.ReadAll(r)
	return data
}

func indexEntryHash(e *Engine, path string) plumbing.Hash {
	idx, err := e.r.Storer.Index()
	if err != nil {
		return plumbing.ZeroHash
	}
	entry, err := idx.Entry(path)
	if err != nil {
		return plumbing.ZeroHash
	}
	return entry.Hash
}

func headBlobHash(e *Engine, path string) plumbing.Hash {
	head, err := e.r.Head()
	if err != nil {
		return plumbing.ZeroHash
	}
	commit, err := e.r.CommitObject(head.Hash())
	if err != nil {
		return plumbing.ZeroHash
	}
	tree, err := commit.Tree()
	if err != nil {
		return plumbing.ZeroHash
	}
	file, err := tree.File(path)
	if err != nil {
		return plumbing.ZeroHash
	}
	return file.Blob.Hash
}

func buildPatch(oldPath, newPath string, from, to []byte) DiffPatch {
	patch := DiffPatch{OldPath: oldPath, NewPath: newPath, Hunks: []DiffHunk{}}
	if bytes.Equal(from, to) {
		return patch
	}
	ud := utilsdiff.Do(string(from), string(to))
	var sb strings.Builder
	if err := diff.NewUnifiedEncoder(&sbWriter{&sb}, 3).Encode(&chunkPatch{
		fromPath: oldPath,
		toPath:   newPath,
		chunks:   ud,
	}); err != nil {
		return patch
	}
	return parseUnified(patch, sb.String())
}

type sbWriter struct{ sb *strings.Builder }

func (w *sbWriter) Write(p []byte) (int, error) { return w.sb.Write(p) }

type chunkPatch struct {
	fromPath string
	toPath   string
	chunks   []diffmatchpatch.Diff
}

func (p *chunkPatch) FilePatches() []diff.FilePatch {
	return []diff.FilePatch{p}
}

func (p *chunkPatch) Message() string { return "" }

func (p *chunkPatch) IsBinary() bool { return false }

func (p *chunkPatch) Files() (diff.File, diff.File) {
	return &chunkFile{path: p.fromPath}, &chunkFile{path: p.toPath}
}

func (p *chunkPatch) Chunks() []diff.Chunk {
	out := make([]diff.Chunk, 0, len(p.chunks))
	for _, c := range p.chunks {
		var op diff.Operation
		switch c.Type {
		case diffmatchpatch.DiffInsert:
			op = diff.Add
		case diffmatchpatch.DiffDelete:
			op = diff.Delete
		}
		out = append(out, chunkChunk{content: c.Text, op: op})
	}
	return out
}

type chunkFile struct {
	path string
}

func (f *chunkFile) Hash() plumbing.Hash     { return plumbing.ZeroHash }
func (f *chunkFile) Mode() filemode.FileMode { return filemode.Regular }
func (f *chunkFile) Path() string            { return f.path }

type chunkChunk struct {
	content string
	op      diff.Operation
}

func (c chunkChunk) Content() string      { return c.content }
func (c chunkChunk) Type() diff.Operation { return c.op }

func parseUnified(p DiffPatch, text string) DiffPatch {
	var cur *DiffHunk
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ "):
			continue
		case strings.HasPrefix(line, "@@"):
			p.Hunks = append(p.Hunks, DiffHunk{Header: line, Lines: []string{}})
			cur = &p.Hunks[len(p.Hunks)-1]
		case cur != nil:
			if strings.HasPrefix(line, "+") {
				cur.Additions++
			} else if strings.HasPrefix(line, "-") {
				cur.Deletions++
			}
			cur.Lines = append(cur.Lines, line)
		}
	}
	return p
}

func (e *Engine) DiffStaged(path string) (DiffPatch, error) {
	return buildPatch(path, path, blobContent(e, headBlobHash(e, path)), blobContent(e, indexEntryHash(e, path))), nil
}

func (e *Engine) DiffUnstaged(path string) (DiffPatch, error) {
	idxHash := indexEntryHash(e, path)
	if idxHash.IsZero() {
		return DiffPatch{OldPath: path, NewPath: path, Hunks: []DiffHunk{}}, nil
	}
	data, err := os.ReadFile(e.wt.Filesystem.Join(e.dir, path))
	if err != nil {
		data = nil
	}
	return buildPatch(path, path, blobContent(e, idxHash), data), nil
}
