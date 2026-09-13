# Installable Releases for CodeSaber (macOS + Linux)

Date: 2026-09-13

## Goal

Tag-driven GitHub Actions pipeline that produces installable artifacts for
macOS and Linux, and a Homebrew cask so users can install via `brew`.

## Decisions (confirmed with owner)

- Channel: CI release pipeline triggered on `v*` tags (option 1).
- macOS distribution: **cask only** (no headless CLI mode exists). Cask is
  self-hosted in this repo and auto-synced to a tap repo.
- Tap repo: `mstrYoda/homebrew-tap` (owner creates the empty repo once; CI
  pushes the cask update using a PAT secret `HOMEBREW_TAP_TOKEN`).
- Linux packages: **AppImage (amd64) + .deb + .rpm**, plus raw tarballs as a
  fallback for all linux archs.
- macOS packages: **.dmg** per arch; unsigned (no Apple Developer
  Program for now). Users right-click → Open on first launch.
- Architectures: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64.
- Builds run on free-tier GitHub Actions runners.

## Prerequisites (manual, one-time)

1. Update `build/config.yml` (productName `CodeSaber`, company, bundle ID
   `com.mstryoda.codesaber`, version `0.1.0`, description) and regenerate
   assets via `wails3 task common:update:build-assets`.
2. Create empty repo `mstrYoda/homebrew-tap`.
3. Create a fine-grained PAT with contents:write on that tap repo; store as
   repo secret `HOMEBREW_TAP_TOKEN` on `mstrYoda/codesaber`.

## Proposed implementation

1. `build/config.yml` identity fix + regen of build assets.
2. `packaging/homebrew/codesaber.rb.in` — cask template (a Ruby file with
   a URL pointing to the DMG release asset and a `sha256` placeholder that
   CI computes per release).
3. `.github/workflows/release.yml`:
   - Trigger: push tag `v[0-9]*`.
   - Job matrix: darwin/amd64, darwin/arm64 on macos runner; linux/amd64,
     linux/arm64 on ubuntu-latest.
   - Build with `go task` (Taskfile) — `wails3 package`/platform-appropriate
     package tasks, `-skipverify` for the unsigned darwin builds.
   - macOS: build `.app`, create `.dmg` via `hdiutil`.
- Linux: build binary, generate `.deb` + `.rpm` (deb/rpm packaging tasks
  in build/Taskfile.yml), AppImage via appimagetool (amd64 only), tar.gz
  for both archs.
   - Aggregate artifacts → GitHub Release.
   - Brew job: compute sha256 for each platform artifact, render cask
     template, commit & push to tap repo.
4. README "Install" section: brew cask, DMG, AppImage, deb, rpm.

## Out of scope

- Code signing / notarization
- AUR / homebrew-core / homebrew-cask submissions
- Windows installer (explicitly not requested yet)
- Headless CLI / formula for CLI-only distribution

## Risks / open questions

- AppImage bundling on CI: template may need extra dep libs; fallback is
  plain tarball if AppImage tooling flakes.
- Wails v3 is beta; the exact CLI flavor/package commands in
  build/Taskfile.yml dictate specifics — implementation plan reads these
  before writing the workflow.
- darwin builds are unsigned; `-skipverify` required in package tasks.
