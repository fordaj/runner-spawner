# The version script (`scripts/version.sh`)

```mermaid
flowchart LR
    Ver("<img src='barnacle:terminal.svg' width='20' alt=''/><br/>scripts/version.sh") --> Entry("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>entry.sh<br/>cascade + log</u>")
    Entry --> Bin("<img src='barnacle:box.svg' width='20' alt=''/><br/>VERSIONER_BIN<br/>pinned bundle or PATH")
    Bin --> Run("<img src='barnacle:terminal.svg' width='20' alt=''/><br/>versioner --repo-dir root<br/>--quiet")
    Run --> Fmt{{"<img src='barnacle:hexagon.svg' width='20' alt=''/><br/>--format"}}
    Fmt -->|text| Text("<img src='barnacle:tag.svg' width='20' alt=''/><br/>VERSION + TAG lines")
    Fmt -->|shell| Shell("<img src='barnacle:terminal.svg' width='20' alt=''/><br/>export VERSION TAG")
    Fmt -->|json| Json("<img src='barnacle:braces.svg' width='20' alt=''/><br/>raw versioner JSON")
    click Entry href "entry.sh.md"
    class Ver,Entry,Run,Shell shFile
    class Bin boxNode
    class Fmt hexNode
    class Text tagNode
    class Json jsonFile
    classDef shFile fill:#abe07a,stroke:#3f6212,color:#16280a
    classDef boxNode fill:#e8cf9d,stroke:#a16207,color:#33240a
    classDef hexNode fill:#ffe08a,stroke:#a16207,color:#422006
    classDef tagNode fill:#f6b8d4,stroke:#be185d,color:#500724
    classDef jsonFile fill:#cdb9f7,stroke:#6d28d9,color:#2e1065
```

Prints the version of this repository as computed by the pinned versioner — the same computation the release pipeline uses, so a local build and the published tag cannot drift apart.

## Usage

```bash
scripts/version.sh                 # VERSION=<v> and TAG=<t> lines (default)
scripts/version.sh --format text   # same
scripts/version.sh --format shell  # export VERSION=... and export TAG=...
scripts/version.sh --format json   # the raw versioner JSON document
```

`--format` requires a value; `--help`/`-h` prints usage. Unknown arguments are a usage error (exit `2`); a missing or non-executable `VERSIONER_BIN` fails with exit `1`.

## How it works

Sources [entry.sh.md](entry.sh.md) for the cascade and `log()`. If `VERSIONER_BIN` is unset or not executable, it fails with `no versioner binary found (run tools/versioner/update.sh or set VERSIONER_BIN)`. Otherwise it runs `versioner --repo-dir <root> --quiet [--format json]`; the repository root defaults to `VERSIONER_REPO_DIR` or the parent of `scripts/` (anchored via `REPO_ROOT_DIR`).

## Consumers

- `scripts/release.sh` — computes the next tag, stamps it into the binaries, pushes the tag (see [release.sh.md](release.sh.md)).
- The release binary itself — see [../version.go.md](../version.go.md) for how `--version` resolves at run time.

## Related pages

- [entry.sh.md](entry.sh.md) · [env.sh.md](env.sh.md)
- [release.sh.md](release.sh.md)
- `tools/versioner/update.sh.md` (excluded from the generated site)
