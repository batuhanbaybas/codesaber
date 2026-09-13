package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrGitMissing reports that the git binary is unavailable; hunk-level
// staging shells out to `git diff`/`git apply`, which go-git cannot express.
var ErrGitMissing = fmt.Errorf("git binary not found")

const stderrTailCap = 500

// lookGit resolves the git binary, ErrGitMissing when absent.
func lookGit() (string, error) {
	p, err := exec.LookPath("git")
	if err != nil {
		return "", ErrGitMissing
	}
	return p, nil
}

// runGitErr runs git in dir, returning stderr tail (capped) on failure.
func runGitErr(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tail := stderr.String()
		if len(tail) > stderrTailCap {
			tail = tail[len(tail)-stderrTailCap:]
		}
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, tail)
	}
	return nil
}

// gitOutput runs git in dir capturing stdout; stderr tail on failure.
func gitOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		tail := stderr.String()
		if len(tail) > stderrTailCap {
			tail = tail[len(tail)-stderrTailCap:]
		}
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, tail)
	}
	return out, nil
}

// splitPatch separates a unified diff into file-top headers and hunks. A
// hunk starts at a "@@" line; everything before the first hunk (diff --git,
// index, ---/+++ lines) is the header block.
func splitPatch(text string) (header string, hunks []string) {
	var cur strings.Builder
	inHunk := false
	flush := func() {
		if inHunk {
			hunks = append(hunks, cur.String())
		}
		cur.Reset()
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "@@") {
			flush()
			inHunk = true
		} else if !inHunk {
			header += line + "\n"
			continue
		}
		cur.WriteString(line)
		cur.WriteString("\n")
	}
	flush()
	// Trailing empty hunk from a final newline split artifact.
	if n := len(hunks); n > 0 && strings.TrimSpace(hunks[n-1]) == "" {
		hunks = hunks[:n-1]
	}
	return header, hunks
}

// isUntracked reports whether path is untracked in the repo at root.
func isUntracked(root, path string) (bool, error) {
	out, err := gitOutput(root, "ls-files", "--error", "--", path)
	if err != nil {
		// Older git lacks --error semantics differing; fall back to status.
		out2, err2 := gitOutput(root, "status", "--porcelain", "--", path)
		if err2 != nil {
			return false, err2
		}
		line := strings.TrimSpace(strings.SplitN(string(out2), "\n", 2)[0])
		return strings.HasPrefix(line, "??"), nil
	}
	return len(bytes.TrimSpace(out)) == 0, nil
}

// StageHunks stages selected hunks (by 0-based index into `git diff
// --unified=3 -- <path>`) of path in the repo at root. Untracked files fall
// back to a whole-file `git add` (go-git cannot partial-stage; the git CLI
// drives the index here).
func StageHunks(root, path string, hunkIdx []int) error {
	if _, err := lookGit(); err != nil {
		return err
	}
	if untracked, err := isUntracked(root, path); err == nil && untracked {
		// MVP: untracked file hunk-stage = full add (nothing to diff against).
		return runGitErr(root, "add", "--", path)
	}
	out, err := gitOutput(root, "diff", "--unified=3", "--", path)
	if err != nil {
		return err
	}
	header, hunks := splitPatch(string(out))
	if len(hunks) == 0 {
		return fmt.Errorf("no hunks in diff for %s", path)
	}
	for _, i := range hunkIdx {
		if i < 0 || i >= len(hunks) {
			return fmt.Errorf("hunk index %d out of range (%d hunks)", i, len(hunks))
		}
		patch := header + hunks[i]
		if err := applyPatch(root, patch, false); err != nil {
			return err
		}
	}
	return nil
}

// UnstageHunks reverts selected hunks (by 0-based index into `git diff
// --cached --unified=3 -- <path>`) out of the index via `git apply --cached
// --reverse`.
func UnstageHunks(root, path string, hunkIdx []int) error {
	if _, err := lookGit(); err != nil {
		return err
	}
	out, err := gitOutput(root, "diff", "--cached", "--unified=3", "--", path)
	if err != nil {
		return err
	}
	header, hunks := splitPatch(string(out))
	if len(hunks) == 0 {
		return fmt.Errorf("no staged hunks for %s", path)
	}
	for _, i := range hunkIdx {
		if i < 0 || i >= len(hunks) {
			return fmt.Errorf("hunk index %d out of range (%d staged hunks)", i, len(hunks))
		}
		patch := header + hunks[i]
		if err := applyPatch(root, patch, true); err != nil {
			return err
		}
	}
	return nil
}

// applyPatch applies one hunk patch from the repo root; reverse flips it
// (index→worktree-direction removal for unstaging).
func applyPatch(root, patch string, reverse bool) error {
	args := []string{"apply", "--cached", "--unidiff-zero"}
	if reverse {
		args = append(args, "--reverse")
	}
	args = append(args, "-")
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tail := stderr.String()
		if len(tail) > stderrTailCap {
			tail = tail[len(tail)-stderrTailCap:]
		}
		// Retry without --unidiff-zero for git versions that dislike it on
		// context-trailing hunks.
		return fmt.Errorf("git apply: %w: %s", err, tail)
	}
	return nil
}

// ensure path is slash-separated for git CLI (Windows-friendly).
var _ = filepath.ToSlash
var _ = os.Environ
