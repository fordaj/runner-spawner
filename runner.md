# Runner paths and identity (`runner.go`)

`runner.go` collects the small helpers that name and probe a runner installation on disk. They contain no lifecycle logic (that is [verbs.md](verbs.md) and [lifecycle.md](lifecycle.md)) — just path math and existence checks, built on the `platform` descriptor from [platform.md](platform.md).

## Key functions

- `runnerDirFor(cfg, name)` — `<runnersDir>/<name>` (the on-disk dir is just the name; the Actions display name is not).
- `displayName(name)` — `hostname-name` (e.g. `maestro-runner01`), the Actions runner display name.
- `runnerExists(dir)` / `isConfigured(dir)` — directory exists; `.runner` file exists (config.sh done).
- `isExtracted(dir, p)` — `config.sh` (or `config.cmd`) is a regular file, plus `svc.sh` on non-Windows. Anything less means an interrupted extraction to repair.
- `configScript(dir, p)` / `serviceScript(dir, p)` — absolute paths of the runner's scripts.
- `windowsServiceName` / `macosServiceLabel` — `actions.runner.<owner>-<repo>.<display>`, matching how the Actions runner registers itself; `macosServicePlist(homeDir, cfg, name)` — the user-level LaunchAgent plist path, using the `userHomeDir` seam so tests can redirect it.
- `runCmd` / `runCmdCapture` — thin wrappers over the `commandExecution` seam (see [script.md](script.md)) for `sc.exe`/`net`-style invocations.

## Where it is used

Every lifecycle verb: `create` consults `isExtracted`/`isConfigured` to decide what to redo; `status`/`list` report `isConfigured` via [verbs.md](verbs.md); `remove` cleans up using the same paths.

## Related pages

- [platform.md](platform.md) — the `platform` struct these helpers consume
- [lifecycle.md](lifecycle.md) — disk state turned into a resume decision
- [script.md](script.md) — the subprocess seam under `runCmd`
