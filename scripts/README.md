# Repository script tooling (`scripts/`)

```mermaid
flowchart TD
    Scripts[task scripts] --> Entry[entry.sh<br/>env + log helper]
    Scripts --> Release[release.sh<br/>version cross-compile tag publish]
    Entry --> Env[env.sh<br/>anchors *_BIN absolute]
    Env --> Common[.common.env<br/>shared pins + repo slugs]
    Env --> Platform[.platform.env<br/>per-OS · per-arch *_BIN]
    Scripts --> Docs[docs.sh<br/>regenerate ./site]
    Scripts --> Format[format.sh<br/>formattington]
    Scripts --> Version[version.sh<br/>print versioner tag]
    Tools[tools/ pinned bundles<br/>not rendered into site] -.-> Env
    click Entry href "entry.sh.md"
    click Env href "env.sh.md"
    click Release href "release.sh.md"
    click Docs href "docs.sh.md"
    click Format href "format.sh.md"
    click Version href "version.sh.md"
```

The `scripts/` directory carries the repository's build/release/docs tooling: an environment cascade the other scripts source, plus one entry point per task. Everything here is bash; nothing requires a global install beyond what each script documents.

Vernacular used in the headers (`log INFO`, `$REPO_ROOT_DIR`, `VERSIONER_BIN`): read [entry.sh.md](entry.sh.md) first, then [env.sh.md](env.sh.md).

## The cascade (why the dotfiles exist)

- `scripts/entry.sh` — sources `scripts/env.sh` and defines `log()` (logamuffin, with a printf fallback). Sourced by everything else. See [entry.sh.md](entry.sh.md).
- `scripts/env.sh` — locates and sources the cascade below, then anchors the repository-relative `*_BIN` tool paths to absolute paths. See [env.sh.md](env.sh.md).
- `scripts/.common.env`, `scripts/.<os>-<arch>.env` — the cascade proper. Hidden files, so barnacle does not render them as pages; this page and [env.sh.md](env.sh.md) document them. `.common.env` holds the platform-independent settings (`TOOLS_DIR=tools`, the upstream repo slugs, and the pinned `*_VERSION_DIR`s — versioner `v0.0.3.0.17`, logamuffin `v0.1.3.0.35`, runner-spawner `v0.0.1.2.15`, barnacle `v0.0.1.0.17`); each platform file sets `PROJECT_PLATFORM` and the `*_BIN` paths for its `<os>-<arch>`.

## Task scripts

- [version.sh](version.sh.md) — print the repository version computed by versioner (`--format text|shell|json`).
- [format.sh](format.sh.md) — format the repository in place with formattington.
- [docs.sh](docs.sh.md) — generate the documentation site you are reading, with the bundled barnacle.
- [release.sh](release.sh.md) — version, cross-compile, tag, and publish the GitHub release.

## Related pages

- `tools/README.md` — the pinned helper binaries (excluded from the generated site)
- [../README.md](../README.md)
- [../AGENTS.md](../AGENTS.md)
