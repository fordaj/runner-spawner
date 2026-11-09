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
`<runnersDir>/<name>/`, runs `config.sh`/`config.cmd` with
`--unattended --labels <auto> --name <host>-<name> --replace`, and
installs/starts the service (`svc.sh` on macOS, `--runasservice` +
`sc.exe` on Windows). `remove` stops + uninstalls the service first,
then runs `config.sh remove`, then deletes the directory — removing a
running runner can no longer fail partway through. `start`/`stop` are
status-aware and idempotent, and `status`/`list` report
`configured` + service state without mutating anything.

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

1. Add a config file to the target repo (defaults shown; copy
   `runner-spawner.example.json`):

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
  _Windows service account_ the runner service runs as.
- Output: `--format json` (`status`/`list`); `--dry-run` prints
  commands without executing; `--quiet` suppresses stderr progress.
- Exit codes: `0` success, `1` runtime error, `2` bad flags/config.

## Platforms

macOS (`darwin/amd64`, `darwin/arm64`) and Windows
(`windows/amd64`, `windows/arm64`) are supported. Linux reports an
explicit unsupported error for the mutating verbs; the binary is still
cross-compiled for Linux so the bundle layout stays uniform.

Windows requires an elevated prompt (checked via `net session`, like
the old `_admin_check.sh`). macOS service ops run as the current user
via the runner's `svc.sh` and each requires confirmation on first use
(LaunchAgent load prompts).

## Development

```bash
make build|test|vet|cross   # `cross` emits dist/<goos>-<goarch>/
./release.bash --dry-run    # compute next tag + build, touch nothing remote
./format.bash               # format: prettier + goimports + shfmt
./ci/format_check.bash      # check formatting without modifying (CI)
```

Formatting is delegated to
[formattington](https://github.com/fordaj/formattington) via
`./format.bash` / `./ci/format_check.bash`. Scope (`.formattington.json`):
`*.md`/`*.json` → `prettier`, `*.go` → `goimports` (gofmt + import
fixing), `*.sh`/`*.bash` → `shfmt -i 4`. Formattington is pinned in
`pyproject.toml` (`dbed7479…`, the first commit with Go support) and
locked in `uv.lock`; `uv run` creates `.venv/` automatically. Style is
configured by `.prettierrc`. Prerequisites (`uv`, Go with
`goimports` on `PATH`, `shfmt`, `prettier`) are listed in `format.bash`.

## Repository layout

```
/            # Go module (typical Go layout; `package main` at root)
  go.mod, main.go, config.go, git.go, platform.go, archive.go,
  token.go, runner.go, verbs.go, script.go, *_test.go
  Makefile  release.bash  runner-spawner.example.json
  format.bash  ci/format_check.bash  pyproject.toml  uv.lock
  .formattington.json  .prettierrc  .python-version
  scripts/  env.sh + .common.env + .<platform>.env (cascade + log())
  tools/
    versioner/    v*/<platform>/versioner[.exe]   + update.bash
    logamuffin/   v*/<platform>/logamuffin[.exe]  + update.bash
    runner-spawner/ v*/<platform>/runner-spawner[.exe]  + update.bash
```

- `make build|test|vet|cross`. `cross` emits `dist/<goos>-<goarch>/`.
- `release.bash` computes the next tag with the bundled `versioner`,
  cross-compiles with `ldflags toolVersion=$TAG`, packs
  `RunnerSpawner_<tag>_<arch>.7z` (+ checksums), tags, and publishes
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

## Not yet done

- Linux service support (systemd/`svc.sh` semantics differ; v1
  supports macOS + Windows only).
- `.runner-spawner.json` schema validation beyond unknown-key
  rejection (no `runnerGroup` existence check against the GitHub API).
- Concurrent `create`/`remove` (verbs run sequentially, like the old
  `create_all_runners.sh` loop).

## History

Known bash-era issues fixed by the rewrite: the repo dir is now a
flag/env instead of values baked into `Utilities/.env`; each runner
dir is just `<name>` (no `$HOST` prefix); `remove` always stops the
service before deregistering, so removing a running runner no longer
breaks midway; and the Windows prompt now names the _Windows service
account_ instead of a bare "account".
