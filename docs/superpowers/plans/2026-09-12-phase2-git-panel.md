# Phase 2 — Git Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Git panel: live status (staged/unstaged/untracked groups), stage/unstage, commit, branch pill + switcher, inline diff viewer.

**Architecture:** New `backend/git` engine wrapping go-git (pure Go, no shell), projecting git changes from the existing per-project fswatch events. Facade RPCs in `backend/app.go` project-namespaced like everything else. Right-dock "Git" tab in the frontend renders the tree; diff viewer renders unified patch hunks via CodeMirror set on the active diff.

**Tech Stack:** go-git v5 + slash-refs; existing fswatch + bridge plumbing; React + Tailwind.

**Standing requirement:** after each phase completes, update `docs/backlog.md` statuses and commit.

---

## File Structure

```
backend/git/git.go          # types: Change{Path,Staging,Status}, Status; DiffPair
backend/git/repo.go         # Engine: Status/Stage/Unstage/Commit/Branches/CheckoutBranch/Diff (go-git)
backend/git/repo_test.go    # fixture-repo driven TDD
backend/git/diff_test.go    # patch capture tests
backend/app.go              # facade RPCs + git-change → "git.status" event re-emit
frontend/src/state/git.tsx  # GitProvider per project: status cache, ops
frontend/src/components/GitPanel.tsx  # replace Git tab placeholder
frontend/src/components/DiffViewer.tsx# unified diff render (plain pre with +/- coloring)
```

Phase-2 acceptance: docs/superpowers/plans/2026-xx-xx-phase2-acceptance.md appended to backlog.

---

### Task 1: Git engine — status (TDD)

**Files:** Create `backend/git/git.go`, `backend/git/repo.go`, `backend/git/repo_test.go`.

- [ ] **Step 1: Failing test** (fixture repo via go-git itself — pure stdlib+go-git; helper `initRepo(t)` creates worktree repo):

```go
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
	os.WriteFile(filepath.Join(dir, "modified.txt"), []byte("mod"), 0o644)   // untracked
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("one-t"), 0o644)     // unstaged modify

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
	f.WriteString("changed") // make line-level diff
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
```

- [ ] **Step 2: Run `go test ./backend/git/` → FAIL** (undefined: New)

- [ ] **Step 3: Implement.** `git.go`:

```go
package git

type ChangeStatus byte

const (
	ChangeAdded      ChangeStatus = 'A'
	ChangeModified   ChangeStatus = 'M'
	ChangeDeleted    ChangeStatus = 'D'
	ChangeUntracked  ChangeStatus = 'U'
)

type Change struct {
	Path    string       `json:"path"`
	Status  ChangeStatus `json:"status"`
}

type Status struct {
	Branch    string   `json:"branch"`
	Staged    []Change `json:"staged"`
	Unstaged  []Change `json:"unstaged"`
	Untracked []Change `json:"untracked"`
}
```

`repo.go` — Engine struct with `*git2.Repository` from PlainOpen; New(dir) opens and returns Engine. Methods (all re-sort by Path):

- `Status() (Status, error)` — `r.Worktree().Status()` (PorterFilter); for each FileStaging tri-state: Staged if Staging != Untracked/Unmodified — map Added/Deleted → ChangeAdded/ChangeDeleted, else Modified; Unstaged if Worktrack status differs (Modified/Deleted); Untracked (Untracked status); branch via `r.Head()` (fallback: "(none)" — no commits).
- `Stage(paths []string) error` — for each: if untracked/deleted check exists, worktree.Add(path) works for both add & stage-mods; deleted files: use `worktree.Remove` — check existence first.
- `Unstage(paths []string) error` — worktree.Reset([]string{path}).

- [ ] **Step 3b: `go get github.com/go-git/go-git/v5`**

- [ ] **Step 4: Run test → PASS.** Commit: `feat(git): status/stage/unstage engine`

### Task 2: Git engine — commit, branches, diff (TDD)

**Files:** Modify `backend/git/repo.go`, `backend/git/git.go` (types), `backend/git/repo_test.go`, create `backend/git/diff_test.go`.

- [ ] **Step 1: Failing tests** (append to repo_test; diff fixture: staged modification of init.txt):

```go
import gitv5 "github.com/go-git/go-git/v5"

func TestCommit(t *testing.T) {
	dir := initRepo(t)
	e, err := New(dir)
	if err != nil { t.Fatal(err) }
	if err := e.Stage([]string{"init.txt"}); err != nil {
		t.Fatal(err)
	}
	// dirty init.txt first:
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("three"), 0o644)
	if err := e.Stage([]string{"init.txt"}); err != nil { t.Fatal(err) }
	if err := e.Commit("bump", "codesaber <codesaber@local>"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	st, _ := e.Status()
	if len(st.Staged)+len(st.Unstaged)+len(st.Untracked) != 0 {
		t.Fatalf("should be clean: %+v", st)
	}
	nodes, _ := e.Log(5)
	if len(nodes) < 2 || nodes[0].Message != "bump" {
		t.Fatalf("log: %+v", nodes)
	}
}

func TestBranchesAndCheckout(t *testing.T) {
	dir := initRepo(t)
	e, _ := New(dir)
	if err := e.CreateBranch("feature/x"); err != nil { t.Fatal(err) }
	if err := e.CheckoutBranch("feature/x"); err != nil { t.Fatal(err) }
	st, _ := e.Status()
	if st.Branch != "feature/x" {
		t.Fatalf("branch: %q", st.Branch)
	}
}

func TestDiffUnstagedPatch(t *testing.T) {
	dir := initRepo(t)
	// modify init.txt so worktree diverges from HEAD
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
```

`diff_test.go` — capture-level patch test using go-git object.DiffTree etc: skip (plumbing exercised via repo.DiffUnstaged lata wiring with working default behavior comment omitted — YAGNI).

- [ ] **Step 2 Run → FAIL.** (undefined CreateBranch etc.)

- [ ] **Step 3: Implement** — `git.go` add:

```go
type DiffHunk struct {
	Header    string   `json:"header"`
	Lines     []string `json:"lines"`
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
}

type DiffPatch struct {
	OldPath  string      `json:"oldPath"`
	NewPath  string      `json:"newPath"`
	Hunks    []DiffHunk  `json:"hunks"`
}

type LogEntry struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Message string `json:"message"`
	When    string `json:"time"`
}
```

`repo.go` add:
- `Commit(msg, author string) error` — split author "name <email>", regex-stubb: parse with strings helpers; worktree.Commit with `AllowEmptyCommands: false` (use default), empty staging → error.
- `Log(n int) ([]LogEntry, error)` — walk HEAD backwards to n (parent iteration), return latest-first.
- `CreateBranch(name) error` — `r.CreateBranch(name, ref)` — from HEAD hash.
- `CheckoutBranch(name string) error` — `r.Worktree().Checkout(&git2.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(name), Create: false})`. Create only when requested branch doesn't exist: `CheckoutBranch` checks first; keep facade CreateBranch + CheckoutBranch separated.
- `DiffStaged(path string) (DiffPatch, error)` — diff HEAD-tree vs index for the path (go-git `object.DiffTree` on HEAD tree vs Index-actually-renamed worktree? Staged diff = index vs HEAD — register via `git2.WritePatch`? go-git exposes plumbing... implement via `gogit_diff` helper: build patch object via `object.DiffTree(from, to, &object.DiffTreeOptions{FileFlags...})`; the diff results can produce a unified patch via `plumbing/format/diff` (go-git has `diff/unified` internal — use `object.Change`→patch via `object.Patch` `object.PatchEncode`? — precise API: (1) get HEAD tree, (2) index tree comparison only available via `repo.Storer.Index()` — MVP simplest: for DiffStaged use `diff.Do` from `github.com/go-git/go-git/v5/plumbing/format/diff` comparing HEAD blob content vs INDEX blob content (worktree added blobs): fetch index entry & blob, do diff.Do on byte slices.
- `DiffUnstaged(path) (DiffPatch, error)` — same `diff.Do` with worktree file content vs index blob content. Parse unified patch text into []DiffHunk (simple line parser: `@@` headers, +/- counting).
- Nonexistent blob (new file) → empty content source.

- [ ] **Step 4: PASS = repo tests & diff test.** Commit: `feat(git): commit/branches/log/diff engine`

### Task 3: Facade wiring + event fanout

**Files:** Modify `backend/app.go`, `backend/app_test.go`; regenerate bindings.

- [ ] **Step 1: facade** — add methods (mirror OpenProject shape; each takes projectId, resolves project):

```go
const (
	EventGitStatus = "git.status"
	EventGitError  = "git.error"
)

func (a *App) GitStatus(projectId string) (git.Status, error)
func (a *App) GitStage(projectId string, paths []string) error
func (a *App) GitUnstage(projectId string, paths []string) error
func (a *App) GitCommit(projectId, message string) error
func (a *App) GitBranches(projectId string) ([]string, error)
func (a *App) GitCheckout(projectId, name string) error
func (a *App) GitDiff(projectId, path string, staged bool) (git.DiffPatch, error)
func (a *App) GitLog(projectId string, n int) ([]git.LogEntry, error)
```

- Per-project git.Engine cached in `a.gits map[string]*git.Engine` guarded by a.mu; created lazily on first call. Engine to be closed? Engines are stateless (PlainOpen each op is fine and simpler) — no lifecycle, no cache; create per call via project.Root. (YAGNI: go-git PlainOpen is cheap.)
- Wire `forwardWatcher` — after emitting fs.change, also trigger emitGitStatus(projectID): read Status; on error emit EventGitError; else emit EventGitStatus with status JSON. Use a single-flight-ish "isEmitting" flag per project (mutex map) to avoid stampede bursts; skip if flag busy (need debounce — the fs event storm is real; MVP: time-based throttle: emit at most once/500ms per project (per-project time ref with mutex); drop rather than queue (a subsequent fs event re-triggers).

- [ ] **Step 2: bindings regen:** `wails3 generate bindings -clean=true -ts -i`; `wails3 build` green.

Commit: `feat(app): git facade RPCs + throttled git.status events`

### Task 4: Git frontend — state + panel UI

**Files:** Create `frontend/src/state/git.tsx`, `frontend/src/components/GitPanel.tsx`; Modify `frontend/src/windows/Workspace.tsx`, remove placeholder left tree "Phase 1 · placeholder" strip.

- [ ] **Step 1: GitProvider** — per active project: `{status, loading, stage(path), unstage, commit(msg), refresh(), diff(path, staged)→patch}`; subscribes `git.status` events filtered by projectId; initial fetch on project activation + on `project.added`. Draft commit message state local to panel.

- [ ] **Step 2: GitPanel component** (replaces Agent/Git tab placeholder when Git tab active):
- Sections: "Staged to Commit" (green tint), "Changed" (amber), "Untracked" (grey).
- Per row: status letter badge (A/M/D/U) colored, rel path, click = toggle stage←→stage/U extraction (staged row click=unstage; unstaged row click=stage; untracked row click=stage). Hover: unstage/stage button; open diff on click? two-click UX: click row = stage/unstage toggle; click filename-text = open diff (unified viewer in dock area or editor-tab-mode = selected file's diff rendered in center pane (editor tab won't have doc). Use dock double-height? Simplest: selected file's diff opens in the center area in a special diff tab from tab store — newальный 'dirty-free diff virtual tab'.
- Commit area: bottom of dock: message textarea + Commit button (only enabled if staged non-empty) + "Commit & Push" disabled-in-MVP placeholder hidden? (skip push; YAGNI... show only Commit).
- Branch pill row at panel top: branch name pill + dropdown listing GitBranches; switch → GitCheckout → status refresh + event emit; branch creation unreachable in MVP (skip CreateBranch exposure in UI; backend method remains for tests).

- [ ] **Step 3: Branch pill in the titlebar** — replace dead "(unknown)" pill (currently any shitt): statusBar branch comes from git state of active project (read ProjectsProvider→GitProvider), fallback "(unknown)" literal if no git repo/daemon at head; prompt: **backend/app.go's `Branch` hook** (project/registry Branch stub) is replaced: on OpenProject call a tiny gitBranch helper from backend/git (expose `func BranchAt(root string) string`) and set Project.Branch. Registry's `Branch` var stays (tests override), the facade uses git engine now.

Commit: `feat(ui): git panel with status/stage and commit`

### Task 5: Diff viewer

**Files:** Create `frontend/src/components/DiffViewer.tsx`; modify tabs.tsx (diff tab mode), Workspace.

- [ ] **Step 1: DiffMode tab:** `Tab` gains `kind: "diff" | "file", diffStaged?: boolean`; diff tab: fetch `GitDiff(projectId,path,staged)`; `DiffViewer` render: file header (`path`, +/- summary), hunks: header row `--bg-panel`, add/minus lines colored (bg --added/--danger at 15% alpha, plus/minus glyph), context neutral, monospace 11-12px; scrollable, line numbers per hunk from `@@ -a,b +c,d` parsing (display start line at hunk top; keep MVP simple: line numbers omitted if a naive parse fails — never break rendering).
- [ ] **Step 2:** row click on filename in GitPanel opens diff tab (title `Δ path`), cols replace standard pre Editor for its kind in Editor.tsx central area (conditional wrapper).
- Cap patch size 2MB (backend truncation flag `patch` field note; skip - YAGNI, backend ReadFile guard equivalent patch: at 10MB already heavier; note skip).

Commit: `feat(git): inline diff viewer for file changes`

### Task 6: Acceptance + backlog update

- [ ] Manual: create temp repo fixture during dev (`init repo with file, modify, verify status list updates ~1s after edits`)
- [ ] Stage via click, commit via box → clean state; branch switch via dropdown → branch pill changes; untracked .env appears; staging persists across status events
- [ ] Update `docs/backlog.md` Phase 2 items to ✅ and commit (`docs: phase 2 acceptance + backlog sync`)

## Self-review
- Spec coverage: status✓ stage/unstage✓ commit✓ branch pill✓ switcher✓ diff✓ live refresh (fsnotify→throttle git.status)✓.
- Type consistency: Change/Status/DiffPatch JSON tags camelCase like other types; GitProvider shape mirrors TabsProvider pattern.
