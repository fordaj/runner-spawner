# Version stamping (`version.go`)

```mermaid
flowchart LR
    Src("<img src='barnacle:file-code.svg' width='20' alt=''/><br/>version.go<br/>toolVersion = dev") --> Stamp{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>build path"}}
    Stamp -->|make build / cross| Mk("<img src='barnacle:hammer.svg' width='20' alt=''/><br/>Makefile LDFLAGS<br/>-X main.toolVersion=tag")
    Stamp -->|scripts/release.sh| Rel("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>computed by versioner<br/>scripts/release.sh.md</u>")
    Mk --> Bin("<img src='barnacle:box.svg' width='20' alt=''/><br/>runner-spawner binary<br/>reports stamped tag")
    Rel --> Bin
    Bin --> Out("<img src='barnacle:check.svg' width='20' alt=''/><br/><u>runner-spawner --version<br/>main.go.md</u>")
    click Rel href "scripts/release.sh.md"
    click Out href "main.go.md"
    class Src goFile
    class Stamp hexNode
    class Rel shFile
    class Bin boxNode
    class Out checkNode
    classDef goFile fill:#8fd8ef,stroke:#0277a8,color:#08303d
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef shFile fill:#abe07a,stroke:#3f6212,color:#16280a
    classDef boxNode fill:#e8cf9d,stroke:#a16207,color:#33240a
    classDef checkNode fill:#9fe8c9,stroke:#0f766e,color:#052e22
```

`version.go` holds exactly one declaration: `toolVersion`, the string printed by `runner-spawner --version` (`runner-spawner <version>`) and read at startup by [main.go.md](main.go.md).

## How it resolves

`toolVersion` is `"dev"` in source and is replaced at link time with `-ldflags="-X main.toolVersion=<tag>"`. Both build paths inject it:

- `make build` / `make cross` (Makefile `LDFLAGS`)
- `scripts/release.sh` (see [scripts/release.sh.md](scripts/release.sh.md)), which computes the next tag with the bundled versioner and stamps it into every cross-compiled binary

A plain `go build` therefore reports `runner-spawner dev`; release artifacts report their tag. The pinned bundle under `tools/runner-spawner/` is built the same way (see `tools/runner-spawner/update.sh.md`).

## Related pages

- [main.go.md](main.go.md) — the `--version` subcommand
- [scripts/version.sh.md](scripts/version.sh.md) — prints the computed tag without building
- [scripts/release.sh.md](scripts/release.sh.md) — where the `-X` injection happens for releases
