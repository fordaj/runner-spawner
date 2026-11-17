# The versioner bundle (`tools/versioner/`)

```mermaid
flowchart TD
    Upd("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>tools/versioner/update.sh</u>") --> Tag("<img src='barnacle:tag.svg' width='20' alt=''/><br/>resolve tag<br/>latest release or --tag")
    Tag --> DL("<img src='barnacle:download.svg' width='20' alt=''/><br/>gh release download<br/>platform archives")
    DL --> Map("<img src='barnacle:arrow-left-right.svg' width='20' alt=''/><br/>darwin-amd64 to macos-x86_64<br/>darwin-arm64 to macos-arm64")
    Map --> Verify{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>checksums.txt<br/>present?"}}
    Verify -->|yes| OK("<img src='barnacle:check.svg' width='20' alt=''/><br/>verify SHA-256")
    Verify -->|no| Warn("<img src='barnacle:triangle-exclamation-point.svg' width='20' alt=''/><br/>warn · continue")
    OK --> Stage("<img src='barnacle:folder.svg' width='20' alt=''/><br/>stage into bundle temp dir")
    Warn --> Stage
    Stage --> Swap("<img src='barnacle:package.svg' width='20' alt=''/><br/>atomic swap<br/>delete superseded v*/")
    VersionSh("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>scripts/version.sh<br/>prints what this computes</u>") -.-> Upd
    click Upd href "../README.md"
    click VersionSh href "../../scripts/version.sh.md"
    class Upd,VersionSh shFile
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

[Versioner](https://github.com/fordaj/Versioner) computes the version of this repository from its Git history, under the rules in `.versioner.json` (`defaultBranchVersion: 254`, `exemptBranches: { main: 0 }`, `tagPattern: v[0-9]*`). The release pipeline asks versioner for the next version, stamps it into every cross-compiled binary, tags the commit, and creates the GitHub release.

The pinned binary selection goes through the [env cascade](../../scripts/env.sh.md) (`VERSIONER_VERSION_DIR`/`VERSIONER_BIN`); [scripts/version.sh](../../scripts/version.sh.md) prints what it computes, and the release pipeline uses the same computation so `runner-spawner --version` agrees with the published tag (see [../../version.go.md](../../version.go.md)).

## Bundled platforms

`v0.0.3.0.17/macos-arm64`, `macos-x86_64`, `linux-arm64`, `linux-x86_64`, `windows-x86_64`. There is **no windows-arm64 bundle upstream** — the windows-arm64 platform file pins `VERSIONER_BIN` at the `windows-x86_64` binary and records this in a comment; set `VERSIONER_BIN` to a versioner on PATH if you need something else there.

## `update.sh` in detail

Refreshes from `fordaj/Versioner` releases via the shared implementation in [../update.sh.md](../update.sh.md). Bundle-specific behaviour: accepts every platform archive, maps its `<os>-<arch>` suffix onto the bundle layout (e.g. `darwin-amd64` → `macos-x86_64`), verifies against the release's `checksums.txt`/`SHA256SUMS.txt` when present — warning rather than fatal when absent. Only the binary is kept. Smoke-tests the host binary (`versioner --version`). Environment overrides: `VERSIONER_REPO`, `VERSIONER_TAG`. `--dry-run` lists planned assets without touching anything.

## Related pages

- [../README.md](../README.md) — the bundle index
- [../../scripts/version.sh.md](../../scripts/version.sh.md)
- [../../scripts/release.sh.md](../../scripts/release.sh.md)
