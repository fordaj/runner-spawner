# runner-spawner

Install and manage repository-scoped self-hosted GitHub Actions
runners for a locally cloned repository, from a single cross-compiled
Go binary.

`runner-spawner` (previously `GithubActions`) replaces the bash family
`_create_runner.sh` / `_remove_runner.sh` / `_start_runner.sh` /
`_stop_runner.sh` / `create_all_runners.sh` … with one tool:

```bash
runner-spawner --help
runner-spawner create --name runner01
runner-spawner create            # all names from .runner-spawner.json
runner-spawner remove --name runner01
runner-spawner start --name runner01
runner-spawner stop --name runner01
runner-spawner status --name runner01
runner-spawner list              # config names + on-disk dirs
```

## How it works

For each runner name, `create` resolves the target repo (see
[Configuration](#configuration)), downloads the pinned
`actions/runner` release for the host (`GOOS`/`GOARCH`) into
`<runnersDir>/cache/` (first run only), extracts it into
`<runnersDir>/<name>/` atomically (staging dir + rename), runs
`config.sh`/`config.cmd` with `--unattended --labels <auto> --name
<host>-<name> --replace`, and installs/starts the service (`svc.sh`
on macOS, `--runasservice` + `sc.exe` on Windows). `remove` stops the
service and waits for it to settle, uninstalls it (verifying it's
really gone), deregisters from GitHub, then deletes the directory.
`start`/`stop` are status-aware and idempotent, and `status`/`list`
report `configured` + service state plus a **remediation hint** for
anything unhealthy without mutating anything.

### Resuming after an interruption

Every verb is designed to converge from **any** partial state: just
re-run the same command. State is detected from disk (the runner dir,
its `.runner` file, and a `.runner-spawner.version` marker) plus a
service probe — never from a journal file. `create` therefore:

- re-extracts atomically when the install is missing, empty, or
  partial (e.g. the download was interrupted) — stale files from an
  older runner version never survive;
- skips `config.sh` when the runner is already configured (the classic
  interruption between `config.sh` and `svc.sh install`, issue #3) and
  just installs + starts the service;
- starts an installed-but-stopped service instead of reinstalling;
- no-ops on an already-running runner.

If GitHub still has a runner with the same name, `create` passes
`--replace` to `config.sh`, so the stale registration is superseded
in place instead of erroring.

`remove` does the same in reverse: it waits for the service to stop,
handles already-stopped/already-uninstalled states, tolerates an empty
or partial runner dir (deleting it after a best-effort remote
deregistration), and completes a previously failed GitHub
deregistration on re-run — falling back to the admin API when the
dir is gone. If deregistration is impossible (no token, network down,
or the runner is already gone → 404), the **local** removal still
proceeds and the output says exactly what may remain on GitHub — or
pass `--local-only` to skip GitHub entirely.

Mutating verbs on the same runner are serialized with a per-runner
lock file (`<runnersDir>/.locks/<name>.lock`). An interrupted run
cannot wedge a runner: a lock older than 15 minutes is taken over
automatically (on Unix it is also taken over when the owning process
is gone).

Post-conditions are verified before success is reported: after
`create`, `.runner` exists and the service reports running; after
`remove`, the directory and the service are gone. Errors name the
failed step and the exact command to re-run.

The token comes from `--token` >
`RUNNER_SPAWNER_TOKEN` > `GH_TOKEN`/`GITHUB_TOKEN` > `gh api
repos/<owner>/<repo>/actions/runners/registration-token -X POST -q
.token`. It is never printed.

## Installation / usage from a repo

Source the env cascade, which resolves the newest bundled binary:

```bash
source scripts/env.sh            # or: eval "$(bash scripts/env.sh)"
"$RUNNER_SPAWNER_BIN" --help
"$RUNNER_SPAWNER_BIN" --version
```

Verify locally first — `--dry-run` prints URLs, labels, and commands
without touching the network or disk:

```bash
"$RUNNER_SPAWNER_BIN" create --dry-run
"$RUNNER_SPAWNER_BIN" list
"$RUNNER_SPAWNER_BIN" status --name runner01 --format json
```

### First-time setup for a target repo

1. Add a config file to the target repo — either copy the committed
   [`.runner-spawner.json`](.runner-spawner.json) as a starting point, or run
   `runner-spawner init` inside the repo (it detects owner/repo from the git
   remote and writes the file for you):

   ```json
   {
     "owner": "fordaj",
     "repo": "AirFrame",
     "runnerNames": ["runner01", "runner02"],
     "runnersDir": ".runners",
     "runnerGroup": "Default",
     "runnerVersion": "2.338.2"
   }
   ```

   Commit `.runner-spawner.json` — it contains no secrets.

2. Authenticate: `gh auth login`, or export `GH_TOKEN`.
3. Create: `"$RUNNER_SPAWNER_BIN" create`
4. Later: `"$RUNNER_SPAWNER_BIN" list|status|start|stop|remove`

Details:

- `runnersDir` is the root under which each `<name>/` lives
  (default `.runners` inside `--repo-dir`); archives are cached in
  `cache/` under it.
- Labels default to `self-hosted,<os>,<arch>,<host>,<host>-<name>`
  (e.g. `self-hosted,macos,x64,maestro,maestro-runner01`). Override
  with `--labels` / `RUNNER_SPAWNER_LABELS` / `"labels": [...]`.
- `.runners/` is already git-ignored in `AirFrame` (verify in any
  other target repo before creating).

## Configuration

Precedence, lowest to highest: built-in defaults, git remote
detection (owner/repo only), `.runner-spawner.json`,
`RUNNER_SPAWNER_*` env vars, CLI flags.

| JSON key          | Env var                               | Flag                  | Default              |
| ----------------- | ------------------------------------- | --------------------- | -------------------- |
| `repoDir`         | `RUNNER_SPAWNER_REPO_DIR`             | `--repo-dir`          | `.`                  |
| `owner`           | `RUNNER_SPAWNER_OWNER`                | `--owner`             | git `origin`         |
| `repo`            | `RUNNER_SPAWNER_REPO`                 | `--repo`              | git `origin`         |
| `runnerNames`     | `RUNNER_SPAWNER_RUNNER_NAMES`         | `--name` (repeatable) | —                    |
| `runnersDir`      | `RUNNER_SPAWNER_RUNNERS_DIR`          | `--runners-dir`       | `.runners`           |
| `runnerGroup`     | `RUNNER_SPAWNER_RUNNER_GROUP`         | `--runner-group`      | `Default`            |
| `runnerVersion`   | `RUNNER_SPAWNER_RUNNER_VERSION`       | `--runner-version`    | `2.338.2`            |
| `archiveCacheDir` | `RUNNER_SPAWNER_ARCHIVE_CACHE_DIR`    | `--archive-cache-dir` | `<runnersDir>/cache` |
| `labels`          | `RUNNER_SPAWNER_LABELS`               | `--labels`            | auto                 |
| `windowsUsername` | `RUNNER_SPAWNER_WINDOWS_USERNAME`     | `--windows-username`  | —                    |
| —                 | `RUNNER_SPAWNER_WINDOWS_PASSWORD`     | `--windows-password`  | —                    |
| —                 | `RUNNER_SPAWNER_TOKEN` (+ `GH_TOKEN`) | `--token`             | `gh api`             |

- Config discovery: `--config` > `RUNNER_SPAWNER_CONFIG` >
  `<repoDir>/.runner-spawner.json` (then `./.runner-spawner.json`).
  Unknown JSON keys fail fast so typos are caught on first run.
- Owner/repo fallback: if config/env/flags still have no owner/repo,
  `git -C <repoDir> remote get-url origin` is parsed
  (`git@github.com:owner/repo.git`, `https://…`, `ssh://…`).
- Passwords: prefer env vars / interactive prompt over flags (they
  appear in process lists). The interactive prompt asks for the
  _Windows service account_ the runner service runs as. On Windows the
  password is passed to `config.cmd` as an argument, so it is briefly
  visible to other processes on the machine while it runs — this is a
  residual exposure of the runner's `--runasservice` mechanism, not
  something the tool can fully eliminate. The password is never
  written to `.runner-spawner.json`, never printed, and never included
  in `--format json` output or dry-run output.
- Output: `--format json` (`status`/`list`, and structured per-runner
  results for `create`/`remove`/`start`/`stop`); `--dry-run` prints
  commands without executing; `--quiet` suppresses stderr progress.
- `--local-only` (`remove` only): skip GitHub deregistration and
  remove locally; use when GitHub is unreachable or the runner was
  already deregistered. Without it, a missing token is a warning, not
  a hard failure — local removal proceeds and the output names the
  remaining GitHub-side cleanup (if any).
- Locks: mutating verbs take `<runnersDir>/.locks/<name>.lock`
  (stale after 15 min) so concurrent or interrupted runs cannot
  interleave.
- Exit codes: `0` success, `1` runtime error, `2` bad flags/config.

## Platforms

macOS (`darwin/amd64`, `darwin/arm64`) and Windows
(`windows/amd64`, `windows/arm64`) are supported. Linux reports an
explicit unsupported error for the mutating verbs; the binary is still
cross-compiled for Linux so the bundle layout stays uniform.

Windows requires an elevated prompt (checked via `net session`, like
the old `_admin_check.sh`; the check is exit-code based and therefore
locale-independent). Service state is read from the numeric `STATE`
code in `sc.exe query` output (1 = stopped, 2/3/5/6 = pending, 4 =
running, 7 = paused) which is stable across localized Windows builds;
`sc.exe stop`/`delete` pending states are waited out with bounded
polling (and a 404-class "service does not exist" is tolerated).
`net session`, `sc.exe` and the numeric-state parsing were verified
to be locale-independent; the only remaining substring match is a
documented English fallback in `sc.exe query` parsing. macOS uses the
runner's `svc.sh` plus the LaunchAgent plist's existence as the
installed/not-installed signal (svc.sh's status output text varies
between runner versions). macOS service ops run as the current user
via the runner's `svc.sh` and each requires confirmation on first
use (LaunchAgent load prompts).

## Development

```bash
make build|test|vet|cross   # `cross` emits dist/<goos>-<goarch>/
./release.bash --dry-run    # compute next tag + build, touch nothing remote
./format.bash               # format: prettier + goimports + shfmt
./ci/format_check.bash      # check formatting without modifying (CI)
```

`go test ./...` runs fully offline: lifecycle/crash-resume tests use
a fake executor (`fake_exec_test.go`) that records every
`svc.sh`/`config.sh`/`sc.exe`/`gh` invocation and simulates
`.runner`/service state, so the whole create/remove state matrix is
covered without network or real services.

Formatting is delegated to
[formattington](https://github.com/fordaj/formattington) via
`./format.bash` / `./ci/format_check.bash`. Scope (`.formattington.json`):
`*.md`/`*.json` → `prettier`, `*.go` → `goimports` (gofmt + import
fixing), `*.sh`/`*.bash` → `shfmt -i 4` (with `scripts/env.sh`
excluded: it intentionally contains zsh-only syntax that no
bash-compatible parser can read, so `shfmt` is pointed at the other
shell files explicitly). Formattington is pinned in
`pyproject.toml` (`dbed7479…`, the first commit with Go support) and
locked in `uv.lock`; `uv run` creates `.venv/` automatically. Style is
configured by `.prettierrc`. Prerequisites (`uv`, Go with
`goimports` on `PATH`, `shfmt`, `prettier`) are listed in `format.bash`.

## Repository layout

```
/            # Go module (typical Go layout; `package main` at root)
  go.mod, main.go, config.go, git.go, platform.go, archive.go,
  token.go, runner.go, verbs.go, script.go, *_test.go
  Makefile  release.bash  .runner-spawner.json  .versioner.json
  format.bash  ci/format_check.bash  pyproject.toml  uv.lock
  .formattington.json  .prettierrc  .python-version  LICENSE
  scripts/  env.sh + .common.env + .<platform>.env (cascade + log())
  tools/
    versioner/    v*/<platform>/versioner[.exe]   + update.bash
    logamuffin/   v*/<platform>/logamuffin[.exe]  + update.bash
    runner-spawner/ v*/<platform>/runner-spawner[.exe]  + update.bash
    barnacle/     v*/<platform>/barnacle[.exe]    + update.bash
    formattington-lite/ v*/<platform>/formattington[.exe] + update.bash
```

- `make build|test|vet|cross`. `cross` emits `dist/<goos>-<goarch>/`.
- `release.bash` computes the next tag with the bundled `versioner`,
  cross-compiles with `ldflags toolVersion=$TAG`, packs
  `RunnerSpawner_<tag>_<arch>.tar.gz` (`.zip` on Windows; + checksums), tags, and publishes
  the GitHub release. `--dry-run` touches nothing remote.
- `tools/runner-spawner/update.bash` downloads the latest release,
  verifies checksums, extracts each platform binary into
  `tools/runner-spawner/<tag>/<platform>/`, and prunes old versions —
  leaving the bundle intact on failure. `scripts/env.sh` then exposes
  `RUNNER_SPAWNER_BIN` (fallback: `runner-spawner` on `PATH`).

## Repository name

This repo was renamed from `fordaj/GithubActions` to
`fordaj/runner-spawner` (the local `origin` now points at the new URL).
The Go module (`github.com/fordaj/runner-spawner`), the
`RUNNER_SPAWNER_REPO` default, and the bundle paths all use the new
name.

## CI

`.github/workflows/ci.yml` runs on **self-hosted runners only** (this repo
is the tool that provisions them — no GitHub-hosted compute, and no Linux
job). Two jobs: `Check Formatting` (`./ci/format_check.bash` after installing
uv, prettier, shfmt and goimports) and `Test` (`go vet ./...`, a `gofmt`
check, `go test ./...` on macOS and Windows). Jobs target
`runs-on: [self-hosted, macOS]` / `[self-hosted, Windows]`; create
the runners with this tool itself. On this repo:

```bash
runner-spawner create --repo-dir <repo> --owner fordaj --repo runner-spawner --name ci-macos
runner-spawner create --repo-dir <repo> --owner fordaj --repo runner-spawner --name ci-windows
```

(The committed `.runner-spawner.json` already lists `ci-macos` and
`ci-windows`.) The workflow is `pull_request`- and `push`-triggered.

## Not yet done

- Linux service support (systemd/`svc.sh` semantics differ; v1
  supports macOS + Windows only).
- `.runner-spawner.json` schema validation beyond unknown-key
  rejection (no `runnerGroup` existence check against the GitHub API).
- Integrity checking of the downloaded actions/runner archive (#6).
- GitHub-side orphan registrations (runner registered on GitHub, local
  dir deleted) are not visible to `status`/`list` — that needs an
  authenticated API call. Re-running `runner-spawner remove --name
<name>` completes the job (a second run deregisters via the admin
  API when the dir is already gone).
- A hostname change orphans the previous service registration
  (`displayName` is `hostname-name`); `create` will register a new
  service under the new label. Remove the old service manually
  (`sc.exe delete` / `launchctl remove` with the old label).

## History

Known bash-era issues fixed by the rewrite: the repo dir is now a
flag/env instead of values baked into `Utilities/.env`; each runner
dir is just `<name>` (no `$HOST` prefix); `remove` always stops the
service before deregistering, so removing a running runner no longer
breaks midway; and the Windows prompt now names the _Windows service
account_ instead of a bare "account".
