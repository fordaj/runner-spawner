# The barnacle bundle (`tools/barnacle/`)

```mermaid
flowchart TD
    Upd("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>tools/barnacle/update.sh</u>") --> Tag("<img src='barnacle:tag.svg' width='20' alt=''/><br/>resolve tag<br/>latest release or --tag")
    Tag --> DL("<img src='barnacle:download.svg' width='20' alt=''/><br/>gh release download<br/>macos linux windows × x86_64 arm64")
    DL --> Verify("<img src='barnacle:check.svg' width='20' alt=''/><br/>verify against<br/>checksums.txt")
    Verify --> Stage("<img src='barnacle:folder.svg' width='20' alt=''/><br/>stage into bundle temp dir")
    Stage --> Swap("<img src='barnacle:package.svg' width='20' alt=''/><br/>swap in v‹tag›/<br/>delete superseded")
    Swap --> Smoke("<img src='barnacle:check.svg' width='20' alt=''/><br/>host binary --version<br/>must contain tag")
    ClickSite("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>scripts/docs.sh<br/>uses this bundle</u>") -.-> Upd
    click ClickSite href "../../scripts/docs.sh.md"
    click Upd href "../README.md"
    class Upd,ClickSite shFile
    class Tag tagNode
    class DL dlNode
    class Verify,Smoke checkNode
    class Stage folder
    class Swap boxNode
    classDef shFile fill:#abe07a,stroke:#3f6212,color:#16280a
    classDef tagNode fill:#f6b8d4,stroke:#be185d,color:#500724
    classDef dlNode fill:#93e2e2,stroke:#0f766e,color:#042f2e
    classDef checkNode fill:#9fe8c9,stroke:#0f766e,color:#052e22
    classDef folder fill:#f0c558,stroke:#8a6d1c,color:#332a05
    classDef boxNode fill:#e8cf9d,stroke:#a16207,color:#33240a
```

[Barnacle](https://github.com/fordaj/barnacle) is a documentation-site generator: it renders the `.md` and `.mmd` files scattered through a repository into a static, navigable HTML site. This repository uses it to render *this* documentation; the site you may be reading was built by it.

## Layout

```text
tools/barnacle/
  update.sh                     # refresh from fordaj/barnacle releases
  v0.0.2.1.25/
    macos-arm64/barnacle
    macos-x86_64/barnacle
    linux-arm64/barnacle
    linux-x86_64/barnacle
    windows-arm64/barnacle.exe
    windows-x86_64/barnacle.exe
```

All six platforms are bundled. The pinned v0.0.2.1.25 bundle in this repository currently carries only the binaries (no `LICENSE` files in the tree).

## `update.sh` in detail

The script is a thin wrapper around [../update.sh.md](../update.sh.md) (the shared implementation). Bundle-specific behaviour: it refreshes from `fordaj/barnacle` releases, accepts every platform archive, keeps the `barnacle[.exe]` binary, copies the upstream MIT `LICENSE` next to each binary when present, and warns (rather than aborts) when a release ships no checksums file.

Environment overrides: `BARNACLE_REPO`, `BARNACLE_TAG`. `--dry-run` lists assets and planned deletions without touching anything.

Note this bundle is refreshed independently of the runner-spawner release pipeline — run `tools/barnacle/update.sh` after a docs-tooling bump, not as part of `scripts/release.sh`.

## How the repository uses it

[scripts/docs.sh](../../scripts/docs.sh.md) resolves `BARNACLE_BIN` through the [env cascade](../../scripts/env.sh.md) and generates `./site/` from the repo root; the output is git-ignored.

## Related pages

- [../README.md](../README.md) — the bundle index
- [../../scripts/README.md](../../scripts/README.md) — the scripts that consume the bundle
- [../../scripts/docs.sh.md](../../scripts/docs.sh.md)
