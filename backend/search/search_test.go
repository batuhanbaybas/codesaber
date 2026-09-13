package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree builds a fixture project tree used by most tests:
//
//	root/main.go          "package main\n// Go is fun\nfunc main() {}\n"
//	root/docs/readme.md   "The GO language\ngopher\n"
//	root/.git/hidden.txt  "go here\n"          (must be skipped)
//	root/logo.png         "go binary\n"        (binary ext, must be skipped)
//	root/node_modules/pkg/index.js "go\n"      (skipDir, must be skipped)
func writeTree(t *testing.T) string {
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
	mk("logo.png", "go binary\n")
	mk("node_modules/pkg/index.js", "go\n")
	return root
}

func TestSearch_LiteralCaseInsensitive(t *testing.T) {
	root := writeTree(t)
	res, err := Search(root, Query{Term: "go"}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Files != 2 || res.Matches != 3 {
		t.Fatalf("got files=%d matches=%d, want 2/3: %+v", res.Files, res.Matches, res)
	}
	byPath := map[string]FileMatches{}
	for _, fm := range res.FilesMatches {
		byPath[fm.Path] = fm
	}
	main := byPath[filepath.Join(root, "main.go")]
	if len(main.Matches) != 1 {
		t.Fatalf("main.go matches: %+v", main.Matches)
	}
	m := main.Matches[0]
	if m.Line != 2 || m.Col != 4 {
		t.Fatalf("main.go match line/col = %d/%d, want 2/4", m.Line, m.Col)
	}
	if m.Text != "// Go is fun" {
		t.Fatalf("main.go match text = %q", m.Text)
	}
	readme := byPath[filepath.Join(root, "docs", "readme.md")]
	if len(readme.Matches) != 2 {
		t.Fatalf("readme matches: %+v", readme.Matches)
	}
	if readme.Matches[0].Line != 1 || readme.Matches[0].Col != 5 {
		t.Fatalf("readme first match = %+v, want line 1 col 5", readme.Matches[0])
	}
	if readme.Matches[1].Line != 2 || readme.Matches[1].Col != 1 {
		t.Fatalf("readme second match = %+v, want line 2 col 1", readme.Matches[1])
	}
}

func TestSearch_SkipsHiddenDirsBinariesAndVendored(t *testing.T) {
	root := writeTree(t)
	res, err := Search(root, Query{Term: "go"}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, fm := range res.FilesMatches {
		p := filepath.ToSlash(fm.Path)
		if strings.Contains(p, "/.git/") || strings.Contains(p, ".png") ||
			strings.Contains(p, "node_modules") {
			t.Fatalf("skipped path leaked into results: %s", p)
		}
	}
}

func TestSearch_AllowedDotfilesSearched(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("GO_FAST=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Search(root, Query{Term: "go_fast"}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Matches != 1 || res.Files != 1 {
		t.Fatalf("want 1 match in .env, got %+v", res)
	}
}

func TestSearch_RegexMode(t *testing.T) {
	root := writeTree(t)
	res, err := Search(root, Query{Term: `^package \w+`, Regex: true}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Matches != 1 || res.Files != 1 {
		t.Fatalf("want 1 regex match, got %+v", res)
	}
	m := res.FilesMatches[0].Matches[0]
	if m.Line != 1 || m.Col != 1 {
		t.Fatalf("regex match line/col = %d/%d, want 1/1", m.Line, m.Col)
	}
}

func TestSearch_RegexInvalidPattern(t *testing.T) {
	_, err := Search(t.TempDir(), Query{Term: "([", Regex: true}, context.Background())
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestSearch_TextCappedAt200Bytes(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("x", 300) + "needle" + strings.Repeat("y", 50)
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(long+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Search(root, Query{Term: "needle"}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Matches != 1 {
		t.Fatalf("matches: %d", res.Matches)
	}
	m := res.FilesMatches[0].Matches[0]
	if len(m.Text) != 200 {
		t.Fatalf("text len = %d, want 200", len(m.Text))
	}
}

func TestSearch_TruncationCaps(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 4; i++ {
		p := filepath.Join(root, string(rune('a'+i))+".txt")
		if err := os.WriteFile(p, []byte("hit\nhit\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Search(root, Query{Term: "hit", MaxMatches: 5, MaxFiles: 3}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !res.Truncated {
		t.Fatal("expected Truncated=true")
	}
	if res.Matches != 5 {
		t.Fatalf("matches = %d, want capped 5", res.Matches)
	}
	if res.Files != 3 {
		t.Fatalf("files = %d, want capped 3", res.Files)
	}
}

func TestSearch_NoTruncationWhenUnderCaps(t *testing.T) {
	root := writeTree(t)
	res, err := Search(root, Query{Term: "go"}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Truncated {
		t.Fatal("unexpected Truncated=true")
	}
}

func TestSearch_CanceledContext(t *testing.T) {
	root := writeTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Search(root, Query{Term: "go"}, ctx); err == nil {
		t.Fatal("expected context error")
	}
}

func TestSearch_MaxSizeSkipsLargeFiles(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("a", 2<<20) + "\nneedle\n"
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Search(root, Query{Term: "needle", MaxSizeMB: 1}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Matches != 0 {
		t.Fatalf("large file should be skipped, got %+v", res)
	}
}

func TestSearch_EmptyTerm(t *testing.T) {
	if _, err := Search(t.TempDir(), Query{Term: ""}, context.Background()); err == nil {
		t.Fatal("expected error for empty term")
	}
}

func TestSearch_MissingRoot(t *testing.T) {
	if _, err := Search(filepath.Join(t.TempDir(), "nope"), Query{Term: "x"}, context.Background()); err == nil {
		t.Fatal("expected error for missing root")
	}
}

// --- case sensitivity + include/exclude globs (feat 7.3) ---

func writeTree2(t *testing.T) string {
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
	mk("main.ts", "const Go = 1\nconst go = 2\n")
	mk("src/util.ts", "Go again\nGO again\n")
	mk("docs/readme.md", "Go here\n")
	mk("main_test.go", "Go test\n")
	return root
}

func TestSearch_CaseSensitiveToggle(t *testing.T) {
	root := writeTree2(t)
	res, err := Search(root, Query{Term: "Go", CaseSensitive: true}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// main.ts line1, src/util.ts line1 only. main_test.go line1. docs? "Go here" yes.
	if res.Matches != 4 {
		t.Fatalf("case-sensitive matches = %d, want 4: %+v", res.Matches, res)
	}
	ins, err := Search(root, Query{Term: "Go"}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// case-insensitive: everything above + "go" on main.ts line2, util.ts line2.
	if ins.Matches != 6 {
		t.Fatalf("case-insensitive matches = %d, want 6: %+v", ins.Matches, ins)
	}
}

func TestSearch_IncludeGlobFiltersFiles(t *testing.T) {
	root := writeTree2(t)
	res, err := Search(root, Query{Term: "go", Include: "*.ts, src/*.ts"}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Files != 2 || res.Matches != 4 {
		t.Fatalf("got files=%d matches=%d, want 2/4: %+v", res.Files, res.Matches, res)
	}
	for _, fm := range res.FilesMatches {
		if !strings.HasSuffix(fm.Path, ".ts") {
			t.Fatalf("non-ts file leaked: %s", fm.Path)
		}
	}
}

func TestSearch_ExcludeGlobFiltersFiles(t *testing.T) {
	root := writeTree2(t)
	res, err := Search(root, Query{Term: "go", Exclude: "**/*_test.go, **/*.md"}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// main.ts(2) + src/util.ts(2) = 4; *_test.go and *.md excluded.
	if res.Files != 2 || res.Matches != 4 {
		t.Fatalf("got files=%d matches=%d, want 2/4: %+v", res.Files, res.Matches, res)
	}
	for _, fm := range res.FilesMatches {
		if strings.HasSuffix(fm.Path, "_test.go") || strings.HasSuffix(fm.Path, ".md") {
			t.Fatalf("excluded file leaked: %s", fm.Path)
		}
	}
}

func TestSearch_IncludeExcludeKeepCapsAndRegex(t *testing.T) {
	root := writeTree2(t)
	res, err := Search(root, Query{Term: "g.", Regex: true, Include: "**/*.ts", MaxMatches: 3}, context.Background())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Matches != 3 || !res.Truncated {
		t.Fatalf("regex+include matches=%d truncated=%v, want 3/true: %+v", res.Matches, res.Truncated, res)
	}
}
