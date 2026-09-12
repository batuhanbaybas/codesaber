package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiffUnstagedPatch(t *testing.T) {
	dir := initRepo(t)
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("one\ntwo\n"), 0o644)
	e, _ := New(dir)
	p, err := e.DiffUnstaged("init.txt")
	if err != nil {
		t.Fatalf("DiffUnstaged: %v", err)
	}
	if len(p.Hunks) == 0 || p.Hunks[0].Additions == 0 {
		t.Fatalf("hunks: %+v", p.Hunks)
	}
	if p.OldPath != "init.txt" || p.NewPath != "init.txt" {
		t.Fatalf("paths: %q %q", p.OldPath, p.NewPath)
	}
}

func TestDiffUnstagedUntrackedAllAdditions(t *testing.T) {
	dir := initRepo(t)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello\nworld\n"), 0o644)
	e, _ := New(dir)
	p, err := e.DiffUnstaged("new.txt")
	if err != nil {
		t.Fatalf("DiffUnstaged untracked: %v", err)
	}
	adds, dels := 0, 0
	for _, h := range p.Hunks {
		adds += h.Additions
		dels += h.Deletions
	}
	if adds != 2 || dels != 0 {
		t.Fatalf("expect all additions: %+v", p.Hunks)
	}
}
