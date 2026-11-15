# Scope resolution and batch execution (`scope.go`)

`scope.go` turns the `--scope`/`--org`/`--github` flags into either a **local** scope (`directory`|`repo`|`pc`) or a **GitHub inventory** scope (`repo`|`org`|`owner`, read-only, `status` only), and implements the shared batch runner for the mutating verbs.

## Scope semantics

- `directory` (default) — only the configured `--runners-dir`; the classic pre-scope behavior.
- `repo` — every installation of the current repository, including other clones/working directories (matched via the machine registry; see [install.md](install.md)).
- `pc` — every runner-spawner-managed installation on this machine.
- `repo`/`org`/`owner` for `status` — read-only GitHub inventory (see [inventory.md](inventory.md)); `org`/`owner` are rejected for other verbs, as are `--org`/`--github`. `create`/`remove` accept only `directory`.
- `--repo owner/repo` (contains a slash) implies GitHub inventory mode for `status`; `--github` forces it.

`resolveScope` validates per verb and returns `(localScope, githubScope, err)`; `discoveredTargets` maps a non-directory scope to per-runner `mutationTarget`s via `discoverInstallations`; `printWarnings` reports skipped/stale registry entries on stderr.

## Batch semantics

`runBatch` attempts **every** target, never aborts on first failure, accumulates per-runner `mutationResult`s (`name`, `repo`, `ok`, `error`), and returns an aggregate error naming the failures (exit `1`) when any target failed. With `--format json`, output is a `results` array; without, failures are printed as they happen.

## Related pages

- [install.md](install.md) — the marker/registry that makes `repo`/`pc` discovery possible
- [inventory.md](inventory.md) — the GitHub side of the scope split
- [verbs.md](verbs.md) — the per-installation verb implementations
- [main.md](main.md) — where scope resolution is invoked
