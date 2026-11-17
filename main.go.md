# Command entry point (`main.go`)

```mermaid
flowchart TD
    Main("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>main") --> Run("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>run")
    Run --> Help{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>--help / -h / help?"}}
    Help -->|yes| Exit0("<img src='barnacle:check.svg' width='20' alt=''/><br/>exit 0")
    Help -->|no| Version{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>--version?"}}
    Version -->|yes| Print("<img src='barnacle:check.svg' width='20' alt=''/><br/><u>print toolVersion · exit 0</u>")
    Version -->|no| Verb{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>subcommand?"}}
    Verb -->|create remove start stop status list| VerbRun("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>runVerb")
    Verb -->|init| Init("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>runInit · init.go.md</u>")
    Verb -->|other| Usage("<img src='barnacle:circle-exclamation-point.svg' width='20' alt=''/><br/>exit 2 · usage error")
    VerbRun --> Layer("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>layer config<br/>defaults → file → env → flags</u>")
    GitRemote("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>git.go.md<br/>only while owner/repo unknown</u>") --> Layer
    Layer --> Resolve("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>resolveScope · scope.go.md</u>")
    Resolve -->|local| Verbs("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>verbs.go.md</u>")
    Resolve -->|GitHub inventory| Inv("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>inventory.go.md</u>")
    click Init href "init.go.md"
    click Resolve href "scope.go.md"
    click Verbs href "verbs.go.md"
    click Inv href "inventory.go.md"
    click Layer href "config.go.md"
    click GitRemote href "git.go.md"
    click Print href "version.go.md"
    class Main,Run,VerbRun,Init,Layer,GitRemote,Resolve,Verbs,Inv goFile
    class Help,Version,Verb hexNode
    class Exit0,Print checkNode
    class Usage errNode
    classDef goFile fill:#8fd8ef,stroke:#0277a8,color:#08303d
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef checkNode fill:#9fe8c9,stroke:#0f766e,color:#052e22
    classDef errNode fill:#f6b8b8,stroke:#b91c1c,color:#450a0a
```

`main.go` is the CLI itself: subcommand dispatch, flag parsing, config layering, and output. The binary is `runner-spawner`; `toolVersion` (see [version.go.md](version.go.md)) is stamped at link time.

## Shape

- `main()` maps `run()`'s error to the exit code via `exitCode()`. `codedError` carries an explicit code; everything else is `1`. `reportedError` marks errors whose message was already printed (flag package output), so `run` does not print twice.
- `runInner` handles `--help`/`-h`/`help`, `--version`/`-version`/`version`, the verb subcommands (`create`, `remove`, `start`, `stop`, `status`, `list`), `init`, and usage errors for anything else. `printUsage` prints the `usageText` table line by line (help and subcommand-error paths share it).
- `runVerb` parses the shared flag set (`--config`, `--repo-dir`, `--owner`, `--repo`, `--name` [repeatable/comma-separated via `nameFlag`], `--scope`, `--org`, `--github`, plus runner-shaping knobs), then layers configuration: built-in defaults → git remote (owner/repo only, [git.go.md](git.go.md)) → `.runner-spawner.json` ([config.go.md](config.go.md)) → `RUNNER_SPAWNER_*` env vars → CLI flags. Format is validated (`text`|`json`) before dispatch.
- `resolveScope` (see [scope.go.md](scope.go.md)) decides local vs. GitHub-inventory mode; inventory mode is dispatched to `runInventory` (see [inventory.go.md](inventory.go.md)), otherwise list/status/mutation logic lives in [verbs.go.md](verbs.go.md).

## Exit codes

`0` success, `1` runtime error (including inventory/batches with reported failures), `2` bad flags/config (`usageErrf`; a `codedError{code: 2}`). Errors are printed to stderr as `runner-spawner: <err>`; `--quiet` suppresses status logs, never errors.

## Output

`writeStatus` emits the `list`/`status` table (or `{scope, runners[], repo?}` JSON); `writeMutationJSON` emits `{scope, results[], repo?}` for the mutating verbs. In `directory` scope the classic `repo` key is kept for compatibility. All JSON output goes through `writeJSONIndent`. The `list`/`status`/`create`/`remove`/`start`/`stop` paths share `simpleTargets` (directory-scope targets bound to one `Config`) and `statusForTargets` (the per-target `statusOne` loop); scoped "no managed installations found" results go through `writeEmptyScopeStatus`.

## Related pages

- [config.go.md](config.go.md) — configuration layering and `DefaultConfig`
- [verbs.go.md](verbs.go.md) — the mutating verbs and status assembly
- [scope.go.md](scope.go.md) — `--scope` resolution and batch dispatch
- [inventory.go.md](inventory.go.md) — `status --scope repo|org|owner` (GitHub, read-only)
