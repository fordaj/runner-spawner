# Command entry point (`main.go`)

`main.go` is the CLI itself: subcommand dispatch, flag parsing, config layering, and output. The binary is `runner-spawner`; `toolVersion` (see [version.md](version.md)) is stamped at link time.

## Shape

- `main()` maps `run()`'s error to the exit code via `exitCode()`. `codedError` carries an explicit code; everything else is `1`. `reportedError` marks errors whose message was already printed (flag package output), so `run` does not print twice.
- `runInner` handles `--help`/`-h`/`help`, `--version`/`-version`/`version`, the verb subcommands (`create`, `remove`, `start`, `stop`, `status`, `list`), `init`, and usage errors for anything else.
- `runVerb` parses the shared flag set (`--config`, `--repo-dir`, `--owner`, `--repo`, `--name` [repeatable/comma-separated via `nameFlag`], `--scope`, `--org`, `--github`, plus runner-shaping knobs), then layers configuration: built-in defaults → git remote (owner/repo only, [git.md](git.md)) → `.runner-spawner.json` ([config.md](config.md)) → `RUNNER_SPAWNER_*` env vars → CLI flags. Format is validated (`text`|`json`) before dispatch.
- `resolveScope` (see [scope.md](scope.md)) decides local vs. GitHub-inventory mode; inventory mode is dispatched to `runInventory` (see [inventory.md](inventory.md)), otherwise list/status/mutation logic lives in [verbs.md](verbs.md).

## Exit codes

`0` success, `1` runtime error (including inventory/batches with reported failures), `2` bad flags/config (`configErrf`/`usageErrf`). Errors are printed to stderr as `runner-spawner: <err>`; `--quiet` suppresses status logs, never errors.

## Output

`writeStatus` emits the `list`/`status` table (or `{scope, runners[], repo?}` JSON); `writeMutationJSON` emits `{scope, results[], repo?}` for the mutating verbs. In `directory` scope the classic `repo` key is kept for compatibility.

## Related pages

- [config.md](config.md) — configuration layering and `DefaultConfig`
- [verbs.md](verbs.md) — the mutating verbs and status assembly
- [scope.md](scope.md) — `--scope` resolution and batch dispatch
- [inventory.md](inventory.md) — `status --scope repo|org|owner` (GitHub, read-only)
