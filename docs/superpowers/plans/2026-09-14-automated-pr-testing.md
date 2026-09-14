# Automated PR Testing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automated PR validation for CodeSaber: a fast CI gate (go vet/test + tsc/vite build) plus AI code review on every PR, both advisory.

**Architecture:** Two GitHub Actions workflows. `ci.yml` runs the deterministic gate on `pull_request`. `ocr-review.yml` runs OpenCodeReview (OCR) via `pull_request_target` so fork PRs get secrets access (safe: OCR only reads diffs), configured against the maintainer's OpenCode Go subscription (`https://opencode.ai/zen/go/v1/chat/completions`, model `glm-5.3-flash`).

**Tech Stack:** GitHub Actions, Go 1.25, Node 22, npm, OpenCodeReview action (`alibaba/open-code-review`).

**Spec:** `docs/superpowers/specs/2026-09-14-automated-pr-testing-design.md`

---

### Task 1: CI gate workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/ci.yml` with exactly this content:

```yaml
name: ci

on:
  pull_request:
    types: [opened, synchronize, reopened]
    paths-ignore:
      - "docs/**"
      - "**/*.md"
      - ".gitignore"
  push:
    branches: [main]
    paths-ignore:
      - "docs/**"
      - "**/*.md"
      - ".gitignore"

concurrency:
  group: ci-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  backend:
    name: go vet + test
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache-dependency-path: go.sum

      - name: Download modules
        run: go mod download

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test ./...

  frontend:
    name: typecheck + build
    runs-on: ubuntu-latest
    timeout-minutes: 15
    defaults:
      run:
        working-directory: frontend
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - name: Install dependencies
        run: npm ci

      - name: Typecheck
        run: npx tsc --noEmit

      - name: Build
        run: npm run build
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: PR gate — go vet/test + tsc/vite build (advisory)"
```

---

### Task 2: AI review workflow (OpenCodeReview + OpenCode Go)

**Files:**
- Create: `.github/workflows/ocr-review.yml`
- Modify: `docs/superpowers/specs/2026-09-14-automated-pr-testing-design.md` (no changes needed — secrets documented there)

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/ocr-review.yml` with exactly this content:

```yaml
name: ai-review

# AI code review on every PR, including forks (pull_request_target exposes
# repo secrets to fork PRs). Safe: OpenCodeReview only reads the diff and
# never executes code from the PR.
#
# Secret to configure once: OCR_LLM_AUTH_TOKEN (OpenCode Go API key from
# https://opencode.ai/auth). Everything else is hardcoded below.

on:
  pull_request_target:
    types: [opened, synchronize]

concurrency:
  group: ai-review-${{ github.event.pull_request.number }}
  cancel-in-progress: true

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    name: AI review (OCR + glm-5.3-flash)
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Run OpenCodeReview
        uses: alibaba/open-code-review@v1
        with:
          llm_url: https://opencode.ai/zen/go/v1/chat/completions
          llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
          llm_model: glm-5.3-flash
          llm_use_anthropic: "false"
          github_token: ${{ secrets.GITHUB_TOKEN }}
          sticky_summary: "true"
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ocr-review.yml
git commit -m "ci: AI PR review via OpenCodeReview + OpenCode Go (glm-5.3-flash)"
```

---

### Task 3: Push to main and configure secret

- [ ] **Step 1: Push**

```bash
git push origin HEAD:main
```

(The workflows only take effect on PRs after they exist on the default branch.)

- [ ] **Step 2: Add the secret (manual, one-time)**

User action: `gh secret set OCR_LLM_AUTH_TOKEN --body <key>` (or via repo Settings → Secrets and variables → Actions). Key comes from https://opencode.ai/auth.

- [ ] **Step 3: Verify the gate works**

Create a throwaway PR from a branch touching backend code; confirm `go vet + test` and `typecheck + build` checks appear and pass. Close the PR without merging.

- [ ] **Step 4: Verify the AI review works**

On the same throwaway PR, confirm an OCR review comment with inline findings appears. Force-push an empty-ish commit (`--allow-empty`) and confirm the stale review is cancelled and a fresh one runs. Close the PR without merging.
