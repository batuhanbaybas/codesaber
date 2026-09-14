# Automated PR Testing Flow — Design

Date: 2026-09-14
Status: Approved

## Context

CodeSaber (`mstrYoda/codesaber`) is a Wails v3 desktop IDE (Go backend + React/TypeScript frontend). The repo currently has **no CI at all** — no `.github/workflows`, no `task test`. Outside contributor PRs are expected, and manual testing of every PR is not sustainable.

Goal: an automated PR validation flow so the maintainer only touches PRs that pass automated checks and where the AI review flags nothing serious.

Decisions made:
- **Depth:** Fast gate (lint, tests, typecheck, build) + AI code review on every PR.
- **Review model:** OpenCodeReview (OCR) via its official GitHub Action, using the maintainer's OpenCode **Go** subscription with **GLM-5.3-Flash** as the review model.
- **Enforcement:** Everything is **advisory** — checks show red/green but nothing blocks merge. The maintainer still merges manually.
- **Runners:** Ubuntu only (Go tests, tsc, vite build all run on Linux; macOS-specific build issues surface in local builds).

## Design

### 1. CI gate — `.github/workflows/ci.yml`

Triggers on `pull_request` (opened, synchronize, reopened), plus optional `push` to `main`. Runs on `ubuntu-latest`.

Two jobs run in parallel:

**Backend job**
- Setup Go (version from `go.mod`)
- Cache Go module + build caches
- `go vet ./...`
- `go test ./...` (from repo root; covers `backend/` packages since the module root is the repo root)

**Frontend job**
- Setup Node LTS (20+)
- `npm ci` in `frontend/` (npm cache enabled)
- `tsc --noEmit` (typecheck without emitting)
- `npm run build` (vite production build — proves the frontend compiles without requiring the wails3 toolchain, which would need heavy GTK/webkit system deps on Linux)

**Path filtering:** skip both jobs when only docs/assets change (`docs/**`, `**/*.md`, `.gitignore`, `README.md`). Implemented with a `paths-ignore` on the workflow trigger.

**Runtime target:** ~2–5 minutes per PR.

### 2. AI review — `.github/workflows/ocr-review.yml`

- Trigger: `pull_request_target` with types `[opened, synchronize]`. This exposes secrets to fork PRs; it is safe because the OCR action only reads the diff and never executes code from the PR (same pattern OCR's own repo uses).
- Concurrency: group per PR number, `cancel-in-progress: true` — a force-push cancels the stale in-flight review.
- Permissions: `contents: read`, `pull-requests: write`.
- Step: `uses: alibaba/open-code-review@v1` with:
  - `llm_url: https://opencode.ai/zen/go/v1/chat/completions`
  - `llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}`
  - `llm_model: glm-5.3-flash`
  - `llm_use_anthropic: 'false'` (Zen Go endpoint is OpenAI-compatible)
  - `github_token: ${{ secrets.GITHUB_TOKEN }}`
  - `sticky_summary: 'true'` — one summary comment per PR, updated in place on re-review
- Timeout: 30 minutes.

**Secrets to configure once (repo Settings → Secrets → Actions):**
- `OCR_LLM_AUTH_TOKEN` — the OpenCode Go API key from https://opencode.ai/auth

### 3. PR experience

1. Contributor opens a PR.
2. Status checks appear: backend (vet + tests), frontend (typecheck + build), OpenCodeReview.
3. OCR posts inline review comments plus a sticky summary comment; re-review runs on each push.
4. Maintainer skims the AI summary and check statuses, then merges manually when satisfied.

### Error handling

- OCR failure (rate limit, API outage, bad key) fails only the review job — it never affects mergeability and is advisory anyway.
- CI gate failure is a red X the maintainer can override (nothing is a required check).
- No auto-merge anywhere; a human always clicks merge.

### Testing

- Open a test PR touching backend code → verify `go vet`/`go test` run and pass.
- Open (or edit) a PR touching frontend code → verify tsc/build run.
- Introduce a deliberate failing test → verify red X, no merge block.
- Verify OCR posts a review comment and inline findings on the test PR; force-push and verify the stale review is cancelled and a new one runs.
- Verify fork PR behaves the same (pull_request_target path).

## Out of scope (future)

- Multi-OS build matrix, frontend unit tests, smoke-run of the built binary
- Required checks / branch protection, auto-merge
- Local `task test` wrapper (can be added alongside CI later)
