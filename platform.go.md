# Platform detection (`platform.go`)

```mermaid
flowchart TD
    GOOS("<img src='barnacle:cpu.svg' width='20' alt=''/><br/>GOOS / GOARCH<br/>from runtime") --> For("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>platformFor<br/>fill platform struct")
    For --> Supported{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>Supported?<br/>darwin amd64 arm64<br/>windows amd64 arm64"}}
    Supported -->|no| Refuse("<img src='barnacle:circle-exclamation-point.svg' width='20' alt=''/><br/>mutating verbs refuse<br/>Linux unsupported")
    Supported -->|yes| Use("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>runner asset · archive ext<br/>service script · labels</u>")
    Use --> Admin{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>Windows?"}}
    Admin -->|yes| Check("<img src='barnacle:check.svg' width='20' alt=''/><br/>adminCheck<br/>net session elevation gate")
    Admin -->|no| Mac("<img src='barnacle:terminal.svg' width='20' alt=''/><br/>svc.sh + LaunchAgent plist")
    Use --> Labels("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>Labels cfg name<br/>self-hosted os arch host</u>")
    click Use href "archive.go.md"
    click Labels href "verbs.go.md"
    class For,Use,Labels goFile
    class Supported,Admin hexNode
    class Refuse errNode
    class Check checkNode
    class Mac shFile
    classDef goFile fill:#8fd8ef,stroke:#0277a8,color:#08303d
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef errNode fill:#f6b8b8,stroke:#b91c1c,color:#450a0a
    classDef checkNode fill:#9fe8c9,stroke:#0f766e,color:#052e22
    classDef shFile fill:#abe07a,stroke:#3f6212,color:#16280a
```

`platform.go` describes what the host machine can run and how its runner is supervised. **Supported at runtime: macOS (`darwin/amd64`, `darwin/arm64`) and Windows (`windows/amd64`, `windows/arm64`) only. Linux reports an explicit unsupported error for the mutating verbs; the Linux asset names are still filled in so `status`/`list` diagnostics can describe the host, and Linux binaries are cross-compiled so the release/bundle layout stays uniform.**

## The `platform` struct

`platform{GOOS, GOARCH, RunnerAsset, ArchiveExt, ConfigBin, ServiceBin, IsWindows, Supported, OSLabel, ArchLabel, hostname}` — the Actions release asset stem (e.g. `actions-runner-osx-x64`), archive extension (`.tar.gz` Unix / `.zip` Windows), service script name per OS, and the label components reported to Actions.

`platformFor(goos, goarch, hostname)` fills it; `currentPlatform` (a var) captures the real host for tests to stub. macOS: `config.sh` + `svc.sh`; Windows: `config.cmd` + `sc.exe` (`ServiceBin` empty). Unknown GOOS/GOARCH leaves `Supported` false.

## Labels

`Labels(cfg, name)` returns `self-hosted,<os>,<arch>,<host>,<host>-<name>` by default (e.g. `self-hosted,macos,x64,maestro,maestro-runner01`); any explicit `cfg.Labels` (from `--labels` / `RUNNER_SPAWNER_LABELS` / config) replaces the auto-detection wholesale.

## Enforcement and helpers

- `requireSupported(verb)` — refuses the mutating verbs on unsupported platforms with a message naming the platform (Linux gets its own, clearer error).
- `adminCheck(p)` — Windows-only elevation gate via `net session` (the old `_admin_check.sh` equivalent); localized Windows builds are fine because the check is exit-code based. No-op elsewhere.
- `runnerURL()` — `https://github.com/<owner>/<repo>`; `hostname()` — the host's name with a `"host"` fallback.

## Related pages

- [runner.go.md](runner.go.md) — consumes `platform` for paths and script names
- [archive.go.md](archive.go.md) — uses `RunnerAsset`/`ArchiveExt` for downloads
- [verbs.go.md](verbs.go.md) — enforces `requireSupported`/`adminCheck` in create/remove/start/stop
