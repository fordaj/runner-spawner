# Configuration (`config.go`)

`config.go` defines `Config` — every tunable of the CLI — and the layering that fills it. Precedence, lowest to highest: built-in defaults → git remote detection (owner/repo only) → `.runner-spawner.json` → `RUNNER_SPAWNER_*` env vars → CLI flags (applied in [main.md](main.md)).

## Defaults

`DefaultConfig()`: `RepoDir: .`, `RunnersDir: .runners`, `RunnerGroup: Default`, `RunnerVersion: 2.338.2`, `Format: text`. The config file is `.runner-spawner.json` (`DefaultConfigFile`).

## Loading

- `FindConfigFile(repoDir)` checks `<repoDir>/.runner-spawner.json`, then `./.runner-spawner.json`; `""` when neither exists.
- `LoadFile` parses with `DisallowUnknownFields` — unknown keys fail fast, so typos in the committed file are caught on first run. `WindowsPassword` is `json:"-"` and never serialised.
- `ApplyEnv` overlays the `RUNNER_SPAWNER_*` variables (see the `Env*` constants). `RUNNER_SPAWNER_CONFIG` (file selection) and `RUNNER_SPAWNER_TOKEN` (see [token.md](token.md)) are consumed elsewhere and are intentionally not applied here. `getenv` is injectable for tests.
- `ParseList` splits comma-separated values; `ParseBool` parses boolean env values.

## Validation and resolution

- `Validate(needRepo, nameArg)` requires non-empty `RunnersDir`/`RunnerGroup`/`RunnerVersion`; owner/repo only when `needRepo` (create/remove).
- `ResolveNames(flagNames)` — explicit `--name` wins over config/env; empty means the caller errors if it needs names.
- `RunnersRoot()` — absolute `<RepoDir>/<RunnersDir>` (or `RunnersDir` if absolute); `ArchiveCache()` — `<RunnersRoot>/cache` unless overridden; `AbsRepoDir()` returns the absolute repo dir.

## Related pages

- [git.md](git.md) — owner/repo fallback from the origin remote
- [main.md](main.md) — flag layering on top of this file
- [README.md — Configuration](README.md) (repo root; table of JSON keys ↔ env vars ↔ flags)
