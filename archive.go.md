# Archive download and atomic extraction (`archive.go`)

```mermaid
flowchart TD
    Create("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>create verb<br/>verbs.go.md</u>") --> Ensure("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>ensureArchive<br/>cached download")
    Ensure --> URL("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>versionedAsset<br/>release URL for host</u>")
    Ensure --> Cache{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>cache hit?"}}
    Cache -->|yes| Extract
    Cache -->|no| DL("<img src='barnacle:download.svg' width='20' alt=''/><br/>download to temp file<br/>rename into cache")
    DL --> Extract("<img src='barnacle:package.svg' width='20' alt=''/><br/>extractArchiveAtomic<br/>staging dir next to target")
    Extract --> Swap("<img src='barnacle:folder.svg' width='20' alt=''/><br/>old dir to backup,<br/>staging to dir,<br/>backup removed")
    Platform("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>platform.go.md</u>") -.-> URL
    click Create href "verbs.go.md"
    click Platform href "platform.go.md"
    click URL href "config.go.md"
    class Create,Ensure,URL,Platform goFile
    class Cache hexNode
    class DL dlNode
    class Extract boxNode
    class Swap folder
    classDef goFile fill:#8fd8ef,stroke:#0277a8,color:#08303d
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef dlNode fill:#93e2e2,stroke:#0f766e,color:#042f2e
    classDef boxNode fill:#e8cf9d,stroke:#a16207,color:#33240a
    classDef folder fill:#f0c558,stroke:#8a6d1c,color:#332a05
```

`archive.go` fetches the pinned `actions/runner` release and unpacks it safely. It covers the whole path from URL to atomically-swapped install directory.

## Download

- `versionedAsset(cfg, p)` builds the asset name/URL (`actions-runner-<os>-<arch>-<version><ext>`) and the GitHub release URL from `cfg.RunnerVersion` (a leading `v` is tolerated).
- `ensureArchive` returns the cached archive path, downloading to a temporary file in the cache dir and atomically renaming it into place — interrupted downloads never poison the cache. A non-empty cached file is reused; non-200 status is an error. Callers pass `defaultHTTPClient()` (a 10-minute timeout) so a hung mirror cannot wedge `create`.
- Cache location: `cfg.ArchiveCache()` — `<runnersDir>/cache` by default (see [config.go.md](config.go.md)).

## Extraction

- `extractArchiveAtomic` unpacks into a staging dir **next to** the target, carries over `preserve` files from any previous install (`.runner`, `.credentials`, `.credentials_rsaparams` are the caller's list), stamps the `.runner-spawner.version` marker (`versionMarkerFile`, see `installedRunnerVersion`), then swaps: old dir → `<name>.old-<timestamp>`, staging → dir, backup removed. Any failure before the swap deletes the staging dir and leaves the previous install untouched; the swap itself rolls back on failure.
- `extractArchive` dispatches on extension (`.zip` → `extractZip`, otherwise `extractTarGz`) — pure Go, no external `tar`/`unzip`/7za. Archive paths are validated in `safeJoin` (absolute paths and `..` escapes are rejected); symlink entries are recreated; file modes are preserved.

## Where it is used

`create` (see [verbs.go.md](verbs.go.md)) downloads, then either skips extraction (marker version matches) or re-extracts atomically when the install is missing/partial. Crash-resume tests exercise this path with fake executors; actual runner archives are never downloaded by the test suite.

## Related pages

- [config.go.md](config.go.md) — archive cache resolution
- [platform.go.md](platform.go.md) — asset names per OS/arch
- [verbs.go.md](verbs.go.md) — the `create` flow that orchestrates this
