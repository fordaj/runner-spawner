# The logamuffin bundle (`tools/logamuffin/`)

```mermaid
flowchart TD
    Upd("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>tools/logamuffin/update.sh</u>") --> Tag("<img src='barnacle:tag.svg' width='20' alt=''/><br/>resolve tag<br/>latest release or --tag")
    Tag --> DL("<img src='barnacle:download.svg' width='20' alt=''/><br/>gh release download<br/>platform archives")
    DL --> Verify{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>checksums.txt<br/>present?"}}
    Verify -->|yes| OK("<img src='barnacle:check.svg' width='20' alt=''/><br/>verify SHA-256")
    Verify -->|no| Warn("<img src='barnacle:triangle-exclamation-point.svg' width='20' alt=''/><br/>warn · continue")
    OK --> Stage("<img src='barnacle:folder.svg' width='20' alt=''/><br/>stage into bundle temp dir")
    Warn --> Stage
    Stage --> Swap("<img src='barnacle:package.svg' width='20' alt=''/><br/>atomic swap<br/>delete superseded v*/")
    Entry("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>scripts/entry.sh log<br/>forwards to this binary</u>") -.-> Upd
    click Upd href "../README.md"
    click Entry href "../../scripts/entry.sh.md"
    class Upd,Entry shFile
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

[Logamuffin](https://github.com/fordaj/logamuffin) is the structured logger used by this repository's scripts: timestamped, leveled `log <level> <message>` records. The scripts' `log()` wrapper ([scripts/entry.sh.md](../../scripts/entry.sh.md)) forwards to the pinned binary when it is present and falls back to an equivalent `printf` implementation when it is not, so a fresh clone works without it.

The scripts themselves document the contract (`log INFO "..."`), the `LOG_LEVEL`/`LOGAMUFFIN_LEVEL` filter, and the `LOG_SOURCE` caller-context field that entry.sh injects.

## Bundled platforms

`v0.1.3.0.35/macos-arm64`, `macos-x86_64`, `linux-arm64`, `linux-x86_64`, `windows-arm64`, `windows-x86_64` (the scripts pick the host's platform via `LOGAMUFFIN_BIN`).

## `update.sh` in detail

Refreshes from `fordaj/logamuffin` releases via the shared implementation in [../update.sh.md](../update.sh.md). Bundle-specific behaviour: accepts every platform archive, maps its `<os>-<arch>` suffix onto the bundle layout (e.g. `darwin-amd64` → `macos-x86_64`), verifies against the release's `checksums.txt`/`SHA256SUMS.txt` when present — warning rather than fatal when absent. Only the binary is kept. Smoke-tests the host binary.

Environment overrides: `LOGAMUFFIN_REPO`, `LOGAMUFFIN_TAG`. `--dry-run` lists planned assets without touching anything.

## Related pages

- [../README.md](../README.md) — the bundle index
- [../../scripts/entry.sh.md](../../scripts/entry.sh.md) — the `log()` wrapper that calls it
