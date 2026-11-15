# The verbs (`verbs.go`)

`verbs.go` is the largest file in the module: it implements the per-installation operations behind `create`, `remove`, `start`, `stop`, `status` and `list`. Shared plumbing (scope, locks, archive, platform) lives in the sibling files; this file is the orchestration layer.

## Status assembly

- `runnerStatus` — one line of `list`/`status` output; `statusOne` wraps `DetectLifecycle` ([lifecycle.md](lifecycle.md)) and attaches the remediation `hint`.
- `queryServiceState` — best-effort, never errors: macOS probes the LaunchAgent plist then `svc.sh status` (`STARTED`/`STOPPED`/`NOT INSTALLED`, else `unknown`); Windows reads the numeric `STATE` code from `sc.exe query` (see [lifecycle.md](lifecycle.md)).
- `listNames` — union of config names and on-disk runner dirs (excluding `cache`/`.locks`), sorted — what makes a bare `list` useful.

## create

`createOne` requires a supported platform and (on Windows) elevation, takes the per-runner lock ([lock.md](lock.md)), then converges from any partial state: no-ops on an already-running runner; otherwise ensures the archive (verified version marker or missing/partial install), extracts **atomically** ([archive.md](archive.md)), skips `config.sh` when `.runner` exists (issue #3 interruption), installs and starts the service (`svc.sh` + LaunchAgent on macOS; `config.cmd --runasservice` + `sc.exe` on Windows — `ensureUnixService`/`ensureWindowsService`), records the installation marker and registry entry ([install.md](install.md)), and verifies the post-conditions (`.runner` exists + service running) before reporting success. `--replace` is passed to `config.sh` so stale GitHub registrations with the same name are superseded in place.

## remove

`removeOne` always stops the service first and waits out pending states (bounded polling; a 404-class "does not exist" is tolerated), uninstalls and verifies the service is gone (with orphan cleanup and a manual-hint fallback), then deregisters from GitHub (`deregisterRemote`; `--local-only` skips it; a missing token warns and proceeds — local removal still happens and output names the remaining GitHub-side cleanup), deletes the runner dir, and prunes the registry. Re-running completes a previously failed GitHub deregistration (admin API fallback when the dir is already gone). An empty/partial runner dir is deleted after best-effort deregistration.

## start/stop

`startOne`/`stopOne` delegate to `controlOne` — idempotent, status-aware: starting an already-running runner and stopping an already-stopped one are safe no-ops; missing or unconfigured installations get an actionable error. Mutations never reach GitHub (deregistration is `remove`'s job).

## Windows account prompt

`promptLine`/`promptPassword` drive the interactive prompts for the Windows service account (`--windows-username`/`--windows-password`, env vars, or interactive). Passwords are never printed and never written to the config file.

## Related pages

- [archive.md](archive.md) — download and atomic extraction
- [lifecycle.md](lifecycle.md) — the state machine these verbs consume
- [install.md](install.md) — markers/registry written here
- [lock.md](lock.md) — per-runner serialization
- [token.md](token.md) — registration token resolution
- [scope.md](scope.md) — batch dispatch around these verbs
