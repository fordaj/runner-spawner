# The docs generator (`scripts/docs.sh`)

```mermaid
flowchart LR
    Docs("<img src='barnacle:terminal.svg' width='20' alt=''/><br/>scripts/docs.sh") --> Entry("<img src='barnacle:terminal.svg' width='20' alt=''/><br/><u>entry.sh<br/>BARNACLE_BIN from cascade</u>")
    Entry --> Barnacle("<img src='barnacle:box.svg' width='20' alt=''/><br/>pinned barnacle<br/>tools/barnacle")
    Docs --> Root("<img src='barnacle:folder.svg' width='20' alt=''/><br/>repo root as cwd")
    Root --> Scan("<img src='barnacle:file-search.svg' width='20' alt=''/><br/>barnacle scans .md + .mmd<br/>.barnacle.json config")
    Scan --> Site("<img src='barnacle:folder.svg' width='20' alt=''/><br/><u>./site/<br/>git-ignored static HTML</u>")
    click Entry href "entry.sh.md"
    click Site href "../AGENTS.md"
    class Docs,Entry shFile
    class Barnacle boxNode
    class Root,Site folder
    classDef shFile fill:#abe07a,stroke:#3f6212,color:#16280a
    classDef boxNode fill:#e8cf9d,stroke:#a16207,color:#33240a
    classDef folder fill:#f0c558,stroke:#8a6d1c,color:#332a05
```

Generates the documentation site: it runs the pinned `barnacle` (`tools/barnacle/update.sh.md`) binary against this repository.

Barnacle scans the working tree for Markdown (`.md`) and Mermaid (`.mmd`) files, renders each as a page, and keeps relative links between them. Generated output is static HTML; it is written to `./site/` by default and is never committed (it is in `.gitignore`).

## Usage

```bash
scripts/docs.sh                       # generate into ./site
scripts/docs.sh --site ./public       # forward --site to barnacle
scripts/docs.sh --config ./docs/barnacle.json
scripts/docs.sh --help                # usage for this script
```

Any arguments other than `--help`/`-h` are forwarded to barnacle verbatim; see `barnacle --help` for `--config`/`--site` semantics. Barnacle exits non-zero and leaves no half-built site on failure; regenerating into a directory previously produced by barnacle wipes and rebuilds it.

## How it locates barnacle

Sources [entry.sh.md](entry.sh.md); `BARNACLE_BIN` comes from the [cascade](env.sh.md) (`tools/barnacle/v<version>/<platform>/barnacle[.exe]`). If the bundle is missing or not executable, the script fails with instructions to run `tools/barnacle/update.sh` (or set `BARNACLE_BIN` explicitly). It then runs barnacle with the repository root as the working directory, so barnacle's auto-discovered `./.barnacle.json` and its default `./site/` output land at the repository root regardless of the caller's cwd. The committed `.barnacle.json` sets `title` to `runner-spawner` and `exclude`s `tools` and `dist` (the `tools/` pages exist for the repository, not the site, because the pinned binaries carry their own upstream docs).

## What the site contains

Every `.md`/`.mmd` in the tree except the `tools/` bundle pages and `dist/` (see the [root README](../README.md) and [AGENTS.md](../AGENTS.md)) — 26 pages as of this writing. `.git`, `node_modules`, `vendor`, `dist`, and the output directory are always excluded by barnacle; hidden files and directories (the cascade's `.common.env` et al., `.runners/`, `.venv/`, …) are skipped unless `.barnacle.json` sets `includeHidden`. Barnacle prints a non-fatal note listing any pages with no Mermaid diagram — this repository keeps that list at zero.

## Related pages

- `tools/barnacle/update.sh.md` — the pinned binaries and how to refresh them (excluded from the generated site)
- [entry.sh.md](entry.sh.md) · [env.sh.md](env.sh.md)
