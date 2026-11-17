# Installation markers and the machine registry (`install.go`)

```mermaid
flowchart TD
    Disco("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>discoverInstallations<br/>scope.go calls this</u>") --> Reg("<img src='barnacle:braces.svg' width='20' alt=''/><br/>installations.json<br/>machine-wide index")
    Reg --> Valid{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>validRegistryEntry<br/>dir exists, marker parses<br/>matches registry, tool = runner-spawner"}}
    Valid -->|no| Warn("<img src='barnacle:triangle-exclamation-point.svg' width='20' alt=''/><br/>warn + never manage")
    Valid -->|yes| Match{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>repo / pc scope<br/>repoMatches"}}
    Root("<img src='barnacle:folder.svg' width='20' alt=''/><br/>configured runners-dir<br/>always in scope") --> Union("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>sortDiscovered")
    Match -->|match| Union
    Match -->|no| Skip("<img src='barnacle:circle-slash.svg' width='20' alt=''/><br/>out of scope")
    Union --> Filter("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>filterByNames<br/>--name filter")
    Filter --> Targets("<img src='barnacle:file-code.svg' width='20' alt=''/><br/><u>mutationTarget per runner<br/>scopedConfig</u>")
    click Disco href "scope.go.md"
    click Targets href "verbs.go.md"
    class Disco,Union,Filter,Targets goFile
    class Reg jsonFile
    class Valid,Match hexNode
    class Root folder
    classDef goFile fill:#8fd8ef,stroke:#0277a8,color:#08303d
    classDef jsonFile fill:#cdb9f7,stroke:#6d28d9,color:#2e1065
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef folder fill:#f0c558,stroke:#8a6d1c,color:#332a05
```

`install.go` implements scope-aware discovery of runner-spawner-managed installations on the local machine — the machinery behind `--scope repo` and `--scope pc`.

## Markers vs. registry

Every installation this tool creates carries `.runner-spawner-install.json` inside the runner dir. The marker is **authoritative**: it records the runner name, display name, owning repository (`owner`/`repo`/`owner/repo`), the clone's `repoDir`, install dir, platform/architecture, registration scope, service registration, and timestamps (`InstallationMeta`, `ServiceMeta` with `type` of `svc-sh`|`launchd`|`windows-service`|`unsupported`).

A machine-wide index, `<user-config-dir>/runner-spawner/installations.json` (e.g. `~/Library/Application Support/runner-spawner/installations.json` on macOS, `%AppData%\runner-spawner\installations.json` on Windows), lists installation dirs so `pc`-wide operations can find installations outside the current repo's `.runners/`. It is a convenience index, **not** a source of truth — losing it just limits discovery. A lock file (`installations.lock`) serializes registry writes (`withRegistryLock` → `atomicWriteFile`).

## Validation

`validRegistryEntry` keeps an entry only when the dir exists and its marker parses, matches the registry (name, installation dir, repo), and was written by runner-spawner (`Tool == "runner-spawner"`, `Version == 1`). Stale or unverifiable entries are reported as warnings and **never** managed. Installations under the configured root are always in scope and need no marker.

`discoverInstallations` selects registry matches for `repo` (repository or `repoDir` match — `repoMatches`/`repoFullNamesEqual`/`pathEqual`) or all valid entries for `pc`, unions them with the configured root (`rootInstallations`), sorts deterministically (`sortDiscovered`), applies the `--name` filter (`filterByNames`), and builds per-target configs (`scopedConfig`). Pre-marker installations are adopted via `synthesizedMeta` when found through the configured root.

`create` writes the marker and registry entry (failure warns, does not fail — see [verbs.go.md](verbs.go.md)); `remove` unregisters the installation.

## Related pages

- [scope.go.md](scope.go.md) — the scopes that consume this discovery
- [verbs.go.md](verbs.go.md) — `recordInstallation` call site
