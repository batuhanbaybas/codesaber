package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// requireGit skips the test when the git binary is unavailable (hunk staging
// shells out to `git diff` / `git apply`, which go-git cannot express).
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found")
	}
}

// runGit runs git in dir with test identity env, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// initHunkRepo creates a repo with one committed file whose layout produces
// at least two separated hunks when re-edited: paragraphs joined by blank
// lines, so distant edits yield distinct @@ blocks.
func initHunkRepo(t *testing.T) (string, string) {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	var sb strings.Builder
	for i := 1; i <= 30; i++ {
		sb.WriteString("para")
		sb.WriteString(strings.Repeat("a", 40))
		sb.WriteString(" line ")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
		if i%3 == 0 {
			sb.WriteString("\n")
		}
	}
	path := "big.txt"
	if err := os.WriteFile(filepath.Join(dir, path), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "checkout", "-q", "-b", "master")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "init")
	return dir, path
}

// hunkCount counts @@ blocks in a unified diff (cached → index vs HEAD).
func hunkCount(t *testing.T, dir, path string, cached bool) int {
	t.Helper()
	args := []string{"diff", "--unified=3"}
	if cached {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "@@") {
			n++
		}
	}
	return n
}

// hunkCountStr counts @@-prefixed hunk headers in a diff string.
func hunkCountStr(diff string) int {
	n := 0
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "@@") {
			n++
		}
	}
	return n
}

// cachedDiff returns `git diff --cached` output for path.
func cachedDiff(t *testing.T, dir, path string) string {
	t.Helper()
	return runGit(t, dir, "diff", "--cached", "--unified=3", "--", path)
}

func TestStageHunks_PartialsIndex(t *testing.T) {
	requireGit(t)
	dir, path := initHunkRepo(t)

	// Rewrite the file: change the first and last lines → ≥2 hunks.
	raw, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) < 10 {
		t.Fatalf("fixture too small: %d lines", len(lines))
	}
	lines[0] = "EDITED-FIRST-LINE"
	lines[len(lines)-1] = "EDITED-LAST-LINE"
	if err := os.WriteFile(filepath.Join(dir, path), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := hunkCount(t, dir, path, false); got < 2 {
		t.Fatalf("fixture should produce >=2 hunks, got %d", got)
	}

	// Stage only the first hunk.
	if err := StageHunks(dir, path, []int{0}); err != nil {
		t.Fatalf("StageHunks: %v", err)
	}

	diff := cachedDiff(t, dir, path)
	if got := hunkCountStr(diff); got != 1 {
		t.Fatalf("expected exactly 1 staged hunk, got %d:\n%s", got, diff)
	}
	if !strings.Contains(diff, "+EDITED-FIRST-LINE") {
		t.Fatalf("staged hunk missing first edit:\n%s", diff)
	}
	if strings.Contains(diff, "+EDITED-LAST-LINE") {
		t.Fatalf("second edit should remain unstaged:\n%s", diff)
	}

	// Index blob hash must differ from HEAD's blob hash for this path.
	e, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	idxHash, err := indexEntryHash(e, path)
	if err != nil {
		t.Fatal(err)
	}
	headHash, err := headBlobHash(e, path)
	if err != nil {
		t.Fatal(err)
	}
	if idxHash == headHash {
		t.Fatal("index hash should diverge from HEAD after partial stage")
	}

	// Unstage the hunk → index matches HEAD again.
	if err := UnstageHunks(dir, path, []int{0}); err != nil {
		t.Fatalf("UnstageHunks: %v", err)
	}
	if tail := strings.TrimSpace(cachedDiff(t, dir, path)); tail != "" {
		t.Fatalf("index should match HEAD after unstaging, got:\n%s", tail)
	}
}

func TestStageHunks_MultipleHunksInOrder(t *testing.T) {
	requireGit(t)
	dir, path := initHunkRepo(t)
	raw, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	lines[0] = "EDITED-FIRST-LINE"
	lines[9] = "EDITED-MID-LINE"
	lines[len(lines)-1] = "EDITED-LAST-LINE"
	if err := os.WriteFile(filepath.Join(dir, path), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n := hunkCount(t, dir, path, false)
	if n < 3 {
		t.Fatalf("fixture should produce >=3 hunks, got %d", n)
	}
	if err := StageHunks(dir, path, []int{0, 1}); err != nil {
		t.Fatalf("StageHunks: %v", err)
	}
	diff := cachedDiff(t, dir, path)
	if got := hunkCountStr(diff); got != 2 {
		t.Fatalf("expected 2 staged hunks, got %d:\n%s", got, diff)
	}
	if strings.Contains(diff, "+EDITED-LAST-LINE") {
		t.Fatalf("last hunk should remain unstaged:\n%s", diff)
	}
}

func TestStageHunks_UntrackedFileFullAdd(t *testing.T) {
	requireGit(t)
	dir, _ := initHunkRepo(t)
	unt := "untracked.txt"
	if err := os.WriteFile(filepath.Join(dir, unt), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StageHunks(dir, unt, []int{0}); err != nil {
		t.Fatalf("StageHunks untracked: %v", err)
	}
	diff := cachedDiff(t, dir, unt)
	if !strings.Contains(diff, "+hello") || !strings.Contains(diff, "+world") {
		t.Fatalf("untracked file should be fully staged:\n%s", diff)
	}
}

func TestStageHunks_GitMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	err := StageHunks(dir, "x.txt", []int{0})
	if err == nil {
		t.Fatal("expected error when git binary is missing")
	}
	if !strings.Contains(err.Error(), "git binary not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := UnstageHunks(dir, "x.txt", []int{0}); err == nil ||
		!strings.Contains(err.Error(), "git binary not found") {
		t.Fatalf("UnstageHunks missing git: %v", err)
	}
}

func TestStageHunks_OutOfRangeHunkErrors(t *testing.T) {
	requireGit(t)
	dir, path := initHunkRepo(t)
	if err := StageHunks(dir, path, []int{99}); err == nil {
		t.Fatal("expected error for out-of-range hunk index")
	}
}
