# The formattington-lite bundle (`tools/formattington-lite/`)

```mermaid
flowchart TD
    Upd("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>tools/formattington-lite/update.sh</u>") --> Tag("<img src='barnacle:tag.svg' width='20' alt=''/><br/>resolve tag<br/>latest release or --tag")
    Tag --> Filter("<img src='barnacle:list-filter.svg' width='20' alt=''/><br/>keep only -lite assets<br/>drop -full bundles")
    Filter --> DL("<img src='barnacle:download.svg' width='20' alt=''/><br/>gh release download")
    DL --> Verify{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>checksums.txt<br/>present?"}}
    Verify -->|yes| OK("<img src='barnacle:check.svg' width='20' alt=''/><br/>verify SHA-256")
    Verify -->|no| Warn("<img src='barnacle:triangle-exclamation-point.svg' width='20' alt=''/><br/>warn · continue")
    OK --> Stage("<img src='barnacle:folder.svg' width='20' alt=''/><br/>stage into bundle temp dir")
    Warn --> Stage
    Stage --> Swap("<img src='barnacle:package.svg' width='20' alt=''/><br/>atomic swap<br/>delete superseded v*/")
    click Upd href "../README.md"
    class Upd shFile
    class Tag tagNode
    class DL dlNode
    class Verify hexNode
    class OK checkNode
    class Stage folder
    class Swap boxNode
    classDef shFile fill:#abe07a,stroke:#3f6212,color:#16280a
    classDef tagNode fill:#f6b8d4,stroke:#be185d,color:#500724
    classDef dlNode fill:#93e2e2,stroke:#0f766e,color:#042f2e
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef checkNode fill:#9fe8c9,stroke:#0f766e,color:#052e22
    classDef folder fill:#f0c558,stroke:#8a6d1c,color:#332a05
    classDef boxNode fill:#e8cf9d,stroke:#a16207,color:#33240a
```

[Formattington](https://github.com/fordaj/formattington) is the formatter orchestrator this repository uses via [`format.sh`](../../scripts/format.sh.md) — but the committed `pyproject.toml`/`uv.lock` pin a git commit for the *real* formatting runs. This bundle carries the **formattington-lite** release asset — the bare `formattington` binary without embedded formatter executables — for convenience and uniformity.

**It is not part of the env cascade** (`scripts/.common.env` sets nothing for it) and nothing in the scripts resolves it; the other four bundles are the cascade-backed ones.

## Bundled platforms

`v0.0.3.0.67/macos-arm64`, `macos-x86_64`, `linux-arm64`, `linux-x86_64`, `windows-arm64`, `windows-x86_64`.

## `update.sh` in detail

Refreshes from `fordaj/formattington` releases via the shared implementation in [../update.sh.md](../update.sh.md). Bundle-specific behaviour: only the platform archives whose stem ends in `-lite` are kept (a `-lite` suffix filter drops everything else, e.g. `-full` bundles). Checksums are verified when the release ships `checksums.txt`/`SHA256SUMS.txt` (warning, not fatal, when absent). Only the `formattington[.exe]` binary is kept, plus `NOTICE.md` and `LICENSES/` when the extracted archive carries them. Smoke-tests the host binary (`formattington --version`).

Environment overrides: `FORMATTINGTON_REPO`, `FORMATTINGTON_TAG`. `--dry-run` lists planned assets without touching anything.

## Related pages

- [../README.md](../README.md) — the bundle index
- [../../scripts/README.md](../../scripts/README.md) — the scripts that consume the cascade-backed bundles
- [../../scripts/format.sh](../../scripts/format.sh.md) — how formatting is actually wired up here
