package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSearchFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("main.go", "package main\n// Go is fun\nfunc main() {}\n")
	mk("docs/readme.md", "The GO language\ngopher\n")
	mk(".git/hidden.txt", "go here\n")
	return root
}

func TestSearchFacade_FindsMatchesSkippingGitDir(t *testing.T) {
	root := writeSearchFixture(t)
	app, _ := newTestApp(t)
	p, err := app.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.SearchText(p.ID, "go", false, false, "", "")
	if err != nil {
		t.Fatalf("SearchText: %v", err)
	}
	if res.Matches != 3 || res.Files != 2 {
		t.Fatalf("matches=%d files=%d, want 3/2: %+v", res.Matches, res.Files, res)
	}
	for _, fm := range res.FilesMatches {
		if strings.Contains(filepath.ToSlash(fm.Path), "/.git/") {
			t.Fatalf(".git file leaked: %s", fm.Path)
		}
	}
}

func TestSearchFacade_TruncationFlag(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 0; i < 2001; i++ {
		b.WriteString("go\n")
	}
	if err := os.WriteFile(filepath.Join(root, "many.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	app, _ := newTestApp(t)
	p, err := app.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.SearchText(p.ID, "go", false, false, "", "")
	if err != nil {
		t.Fatalf("SearchText: %v", err)
	}
	if !res.Truncated {
		t.Fatal("expected Truncated=true for 2001 matches")
	}
	if res.Matches != 2000 {
		t.Fatalf("matches=%d, want capped 2000", res.Matches)
	}
}

func TestSearchFacade_UnknownProject(t *testing.T) {
	app, _ := newTestApp(t)
	if _, err := app.SearchText("nope", "go", false, false, "", ""); err == nil {
		t.Fatal("expected error for unknown project")
	}
}

func TestSearchFacade_SequentialCallsAndRemoveCleanup(t *testing.T) {
	root := writeSearchFixture(t)
	app, _ := newTestApp(t)
	p, err := app.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		res, err := app.SearchText(p.ID, "go", false, false, "", "")
		if err != nil {
			t.Fatalf("SearchText #%d: %v", i, err)
		}
		if res.Matches != 3 {
			t.Fatalf("matches=%d, want 3", res.Matches)
		}
	}
	if err := app.RemoveProject(p.ID); err != nil {
		t.Fatalf("RemoveProject: %v", err)
	}
}
