# AGENTS.md

Guidance for AI coding agents working in this repository. The user-facing contract is in [README.md](README.md) — read it first; this file covers how to work on the code and its documentation.

## Project Overview

runner-spawner is a Go CLI (`package main` at the repository root) for installing and managing repository-scoped, self-hosted GitHub Actions runners for a locally cloned repository. It replaces a family of bash scripts with one cross-compiled binary: scope-aware lifecycle management (`create`/`remove`/`start`/`stop`/`status`/`list` across `directory`|`repo`|`pc` scopes), a read-only GitHub runner inventory (`status --scope repo|org|owner`), registration-token resolution, and atomic extraction of the pinned `actions/runner` archive. **Runtime-supported platforms: macOS (darwin/amd64, darwin/arm64) and Windows (windows/amd64, windows/arm64) only.** Linux binaries are cross-compiled solely to keep the release/bundle layout uniform; the mutating verbs refuse Linux explicitly. See README.md — Platforms — for the verified platform matrix; do not widen it without implementing the Linux service path.

## Core Principles

- **Converge from any partial state.** Every mutating verb is designed to be re-run after an interruption; state is detected from disk (runner dir, `.runner`, `.runner-spawner.version`) plus a service probe — never from a journal file.
- **Never invent platform behavior.** Windows service state is read from the numeric `STATE` code (locale-independent); macOS uses `svc.sh` plus the LaunchAgent plist's existence. Linux is not supported at runtime.
- **Never print secrets.** Registration tokens flow via `--token` > `RUNNER_SPAWNER_TOKEN` > `GH_TOKEN`/`GITHUB_TOKEN` > `gh api`; they are never logged, never written to config, absent from `--format json`.
- **The machine registry is a pointer, not truth.** `--scope repo`/`pc` manage only installations whose dir exists and whose `.runner-spawner-install.json` marker parses and matches; anything else is reported, never managed.
- **Tests run offline.** The suite fakes the subprocess seam; nothing here touches the network or real services.

## Build/Test

```bash
make build|test|vet|cross   # Makefile; `cross` emits dist/<goos>-<goarch>/
go test ./...               # same as `make test`; fully offline
go vet ./...                # same as `make vet`
./format.sh               # format in place (prettier + goimports + shfmt)
./ci/format_check.sh      # check formatting only (CI)
./scripts/release.sh --dry-run   # compute next tag + cross-compile; touch nothing remote
scripts/version.sh        # print the version computed by versioner
scripts/docs.sh             # regenerate ./site (barnacle; git-ignored)
```

Tests live next to the code as `*_test.go` (standard `testing`). Lifecycle/crash-resume tests use `fake_exec_test.go`, a fake `commandExecution` that records `svc.sh`/`config.sh`/`sc.exe`/`gh` invocations; unit tests point discovery at temp trees. The suite requires no network access.

Before considering a change complete: run `make vet` and `go test ./...`; verify CLI argument parsing and exit behavior when touching `main.go`; keep `./ci/format_check.sh` clean when touching shell/markdown/Go formatting scope; check paths with spaces where filesystem or subprocess behavior changes; test missing executables and partial-state re-runs when relevant. Do not invent commands for tools that are not configured.

## Repository Layout

```
/                  # Go module root; package main
  main.go          # subcommand dispatch, flag parsing, config layering, output
  config.go        # Config, defaults, .runner-spawner.json loading (DisallowUnknownFields), env overlay
  verbs.go         # create/remove/start/stop/status/list implementation
  archive.go       # runner archive download + atomic extraction (staging dir + rename)
  ...              # one sibling <name>.md per Go file (see below)
  format.sh      # repo formatting entry point (uv + formattington)
  Makefile         # build/test/vet/cross
  scripts/         # entry.sh, env.sh, .common.env + .<platform>.env cascade, release.sh,
                   # version.sh, docs.sh
  tools/           # pinned helper binaries + update.sh: barnacle, formattington-lite,
                   # logamuffin, runner-spawner, versioner
  ci/              # format_check.sh (check-only counterpart of format.sh)
  dist/            # build output (git-ignored)
  .runner-spawner.json  # this repo's own runner config (dogfooding)
  site/            # generated documentation site (git-ignored)
```

Not committed: `.runners/` (machine-local runner state), `.venv/`, `dist/`, `site/`.

## Documentation files (barnacle)

The repository's documentation is a set of Markdown (`.md`) and Mermaid (`.mmd`) source files rendered into a static site by the pinned [barnacle](https://github.com/fordaj/barnacle) binary (`tools/barnacle/`, wired into the environment cascade as `BARNACLE_BIN`). Generate the site with `scripts/docs.sh`; the output (`./site/`) is git-ignored and never committed.

Layout — documentation lives next to the code it describes:

- One sibling `.md` per non-test Go source file at the repository root (`main.go` → [main.md](main.md), `verbs.go` → [verbs.md](verbs.md), …). **One page covers a platform split**: [lock.md](lock.md) documents `lock.go`, `lock_unix.go` and `lock_windows.go` together rather than three near-identical pages.
- One sibling `.md` per root shell script (`format.sh` → [format.md](format.md)).
- One sibling `.md` per script (`scripts/entry.sh` → `entry.md`, `scripts/env.sh` → `env.md`, `scripts/release.sh` → `release.md`, `scripts/version.sh` → `version.md`, `scripts/docs.sh` → `docs.md`), plus [scripts/README.md](scripts/README.md) as the hub.
- One `update.md` per bundle in `tools/` (`tools/<name>/update.md`), plus [tools/README.md](tools/README.md) as the hub.
- Test files (`*_test.go`) are deliberately not documented.

Rules for keeping these files current:

- When you add, rename, remove, or repurpose a documented file — a Go file, a script, or a bundle — update its sibling `.md` **in the same change**. A new file without its page is incomplete work; a stale page is worse than none.
- Source every fact: the `.go`/`.sh` file itself, this file (AGENTS.md), and README.md are the accepted sources. Never invent flags, exit codes, extensions, or bundle-platform matrices.
- Use plain relative links (`verbs.md`, `../../README.md`) with the linking file's `.md` path — barnacle rewrites them to `.html`. Do not link to files barnacle does not render (binaries, `.sh`, `.json`, `LICENSE`); link the related `.md` page (for external references, a full `https://` URL is fine).
- A `foo.md` and a `foo.mmd` may not coexist for the same stem — barnacle fails the whole site rather than overwriting.
- Hub pages (`scripts/README.md`, `tools/README.md`) list their siblings; adding a page means linking it from the hub too.
- Barnacle skips hidden files by default, so the `.common.env` and `.<platform>.env` cascade is documented inside [scripts/env.md](scripts/env.md) — keep it accurate there instead of adding hidden doc files.
- Regenerate and self-review the site before publishing: `scripts/docs.sh`, then click through the changed pages. The generator output must remain free of broken local links.

Definition of Done addition: a change that touches any documented file updates (or creates) its sibling page in the same commit.

## Definition of Done

A change is complete when it:

- Implements the requested behavior.
- Preserves the established CLI contract (flags, subcommands, exit codes 0/1/2, output shape) unless intentionally changed.
- Remains compatible with the supported platforms (macOS + Windows; Linux mutating verbs keep refusing with a clear error).
- Handles errors and subprocess execution appropriately (the `commandExecution` seam keeps tests hermetic; keep new subprocess calls behind it).
- Includes or updates tests where appropriate (`go test ./...` green).
- Updates relevant documentation — including the sibling `.md` pages above for every Go, bash, or tools/ file touched, and the hubs (`scripts/README.md`, `tools/README.md`) when pages are added or removed.
