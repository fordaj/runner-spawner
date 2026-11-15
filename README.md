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

# scope-aware lifecycle management (list, status, start, stop)
runner-spawner list --scope pc
runner-spawner start --scope repo
runner-spawner stop --scope pc

# read-only GitHub runner inventory (status only)
runner-spawner status --scope repo --repo fordaj/runner-spawner
runner-spawner status --scope org --org fordaj
runner-spawner status --scope owner --owner fordaj
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
  JSON payloads carry a `scope` key (`directory`|`repo`|`pc`, or
  `repo`|`org`|`owner` for inventory); in directory scope the classic
  `repo` key is kept for compatibility, and discovered runners include
  `repo` when known.
- `--local-only` (`remove` only): skip GitHub deregistration and
  remove locally; use when GitHub is unreachable or the runner was
  already deregistered. Without it, a missing token is a warning, not
  a hard failure — local removal proceeds and the output names the
  remaining GitHub-side cleanup (if any).
- Locks: mutating verbs take `<runnersDir>/.locks/<name>.lock`
  (stale after 15 min) so concurrent or interrupted runs cannot
  interleave.
- Exit codes: `0` success, `1` runtime error (including an inventory or
  batch with reported failures), `2` bad flags/config.

## Scope-aware lifecycle management

`list`, `status`, `start` and `stop` accept `--scope` to select which
runner installations on the machine they operate on:

| Scope       | What it targets                                                                                            |
| ----------- | ---------------------------------------------------------------------------------------------------------- |
| `directory` | (default) only the runners in the configured `--runners-dir`.                                              |
| `repo`      | every installation of the current repository, including other clones/working directories of the same repo. |
| `pc`        | every runner-spawner-managed installation on this machine, regardless of repository.                       |

```bash
runner-spawner list --scope pc
runner-spawner status --scope repo
runner-spawner start --scope repo
runner-spawner stop --scope pc
```

**How discovery works.** Every installation this tool creates carries a
marker file, `.runner-spawner-install.json`, inside the runner dir. The
marker records the runner name, owning repository (`owner/repo`), the
clone's `repoDir`, the installation dir, platform/architecture, the
GitHub registration scope, and the service registration. A machine-wide
index, `<user-config-dir>/runner-spawner/installations.json` (e.g.
`~/Library/Application Support/runner-spawner/installations.json` on
macOS, `%AppData%\runner-spawner\installations.json` on Windows),
lists installations created by this tool. `repo` scope selects registry
entries whose repository or `repoDir` matches the invocation's repo;
`pc` scope selects all of them. The configured `--runners-dir` is
always in scope in addition to registry matches. Registry entries are
treated as pointers, not truth: an entry is managed only when its dir
exists and its marker parses, matches the registry (name, installation
dir, repo), and was written by runner-spawner. Stale or unverifiable
entries are reported as warnings and **never** managed — neither is
anything that isn't runner-spawner-managed, even if a similarly named
service exists.

Directory-scope behavior is unchanged from previous versions (config
names plus on-disk dirs; no registry involved). `create` writes the
marker + registry entry (a failure there warns but does not fail the
create); `remove` prunes the entry. If a pre-existing installation has
no marker yet, it is still managed in `directory` scope, and in
`repo`/`pc` scope when found via the configured root; adopt an
unmarked installation by re-running `runner-spawner create --name
<name>` (it rewrites the marker and re-registers without touching a
healthy service).

**Batch semantics.** `start`/`stop` (and `create`/`remove`) no longer
abort on the first failure: every target is attempted, each outcome is
reported individually, and an aggregate error naming the failures is
returned at the end (exit code `1`). `--format json` emits a
`results` array with per-runner `name`, `repo`, `ok` and `error`;
`--name` filters the installations a scoped `status`/`start`/`stop`
acts on (`list` reports everything in scope). Starting an already-
running runner and stopping an already-stopped one are safe no-ops.
Batch operations never touch anything outside the scope's discovery
rules, and a stopping runner is never deregistered from GitHub —
removal from GitHub is `remove`'s job.

Example:

```bash
runner-spawner list --scope pc
# runner01  maestro-runner01  repo=acme/widgets  configured=true  service=running  /Users/maestro/src/widgets/.runners/runner01
# runner02  maestro-runner02  repo=acme/other    configured=true  service=stopped  /Users/maestro/src/other/.runners/runner02  hint: stopped — ...
```

## GitHub runner inventory

`status` can also query the **GitHub API** (read-only) for the runners
registered to repositories or organizations — an inventory of
registrations, not remote machine control, and nothing here can start,
stop, create or remove a runner:

```bash
runner-spawner status --scope repo --repo fordaj/runner-spawner
runner-spawner status --scope repo                  # --repo owner/repo, or --owner + --repo
runner-spawner status --scope org --org fordaj
runner-spawner status --scope owner --owner fordaj
```

- `repo` queries `repos/{owner}/{repo}/actions/runners`. `org` queries
  `orgs/{org}/actions/runners`. `owner` aggregates: it queries the
  org-level endpoint when the owner is an organization, and — because
  GitHub has no universal "all runners of an account" endpoint —
  enumerates the repositories visible to the authenticated token
  (`users/{owner}/repos?type=owner`, or `orgs/{org}/repos?type=all`
  for organizations) and queries each repository's endpoint.
- Output lists: runner ID and name, registration scope (`repo` or
  `org`), the associated repository or organization, online/offline
  status, busy/idle state, labels and runner version where available.
  `--format json` emits `scope`, `target`, `ok`, `runners[]` and
  `failures[]`.
- Pagination is followed automatically (`gh api --paginate`); duplicate
  runners reached through multiple paths are de-duplicated.
- Inaccessible repositories/organizations are reported as **partial
  failures** (one warning per target on stderr) without aborting the
  rest of the inventory; the command exits `1` and reports what could
  and could not be queried. A 404 usually means the token cannot see
  the target; a 401/403 usually means a token scope or permission
  problem — the warnings say which, and empty inventory (zero runners
  found with no failures) is a success, distinct from failure.
- Authentication uses the `gh` CLI's existing credentials (`gh auth
login`, `GH_TOKEN` or `GITHUB_TOKEN`); runner-spawner never stores or
  prints tokens. `gh` must be installed for inventory commands.
- `--repo owner/repo` implies inventory mode; `--github` forces it.
  Without them, `--scope repo` means the local repo scope above.
  `--org` and `--owner` default from `--owner` / config / git remote.

GitHub API status is the runner's reported online/offline and
busy/idle state; it does **not** control the host machine. Use the
local scopes (`--scope directory|repo|pc`) to manage the services on
this machine, and the inventory scopes to audit registrations on
GitHub.

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
./scripts/release.sh --dry-run    # compute next tag + build, touch nothing remote
./format.bash               # format: prettier + goimports + shfmt
./ci/format_check.bash      # check formatting without modifying (CI)
```

`go test ./...` runs fully offline: lifecycle/crash-resume tests use
a fake executor (`fake_exec_test.go`) that records every
`svc.sh`/`config.sh`/`sc.exe`/`gh` invocation and simulates
`.runner`/service state (including scripted `gh api` endpoints and
per-runner-dir service failures), so the whole create/remove state
matrix, scope-aware discovery, and GitHub inventory aggregation are
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
  token.go, runner.go, verbs.go, script.go, install.go, inventory.go,
  scope.go, *_test.go
  Makefile  scripts/release.sh  .runner-spawner.json  .versioner.json
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
- `scripts/release.sh` computes the next tag with the bundled `versioner`,
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
check, `go test ./...`). Jobs currently target `runs-on: [self-hosted,
Windows]` while the macOS runners are unavailable (see
[#9](https://github.com/fordaj/runner-spawner/issues/9)); the intended
shape is one macOS + one Windows leg each, created with this tool
itself. On this repo:

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
  dir deleted) are visible in the read-only inventory
  (`runner-spawner status --scope repo --repo <owner>/<repo>` /
  `--scope owner --owner <owner>`), but there is no bulk unregister —
  re-running `runner-spawner remove --name <name>` completes a local
  removal whose GitHub deregistration failed earlier (a second run
  deregisters via the admin API when the dir is already gone).
- Owner-wide inventory visibility is limited by the token's access:
  private repos/orgs the token cannot see are reported as partial
  failures, not enumerated.
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
