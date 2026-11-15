# Repository script tooling (`scripts/`)

The `scripts/` directory carries the repository's build/release/docs tooling: an environment cascade the other scripts source, plus one entry point per task. Everything here is bash; nothing requires a global install beyond what each script documents.

Vernacular used in the headers (`log INFO`, `$REPO_ROOT_DIR`, `VERSIONER_BIN`): read [entry.md](entry.md) first, then [env.md](env.md).

## The cascade (why the dotfiles exist)

- `scripts/entry.sh` — sources `scripts/env.sh` and defines `log()` (logamuffin, with a printf fallback). Sourced by everything else. See [entry.md](entry.md).
- `scripts/env.sh` — locates and sources the cascade below, then anchors the repository-relative `*_BIN` tool paths to absolute paths. See [env.md](env.md).
- `scripts/.common.env`, `scripts/.<os>-<arch>.env` — the cascade proper. Hidden files, so barnacle does not render them as pages; this page and [env.md](env.md) document them. `.common.env` holds the platform-independent settings (`TOOLS_DIR=tools`, the upstream repo slugs, and the pinned `*_VERSION_DIR`s — versioner `v0.0.3.0.5`, logamuffin `v0.1.3.0.35`, runner-spawner `v0.0.1.2.6`, barnacle `v0.0.1.0.17`); each platform file sets `PROJECT_PLATFORM` and the `*_BIN` paths for its `<os>-<arch>`.

## Task scripts

- [version.sh](version.md) — print the repository version computed by versioner (`--format text|shell|json`).
- [format.sh](format.md) — format the repository in place with formattington.
- [docs.sh](docs.md) — generate the documentation site you are reading, with the bundled barnacle.
- [release.sh](release.md) — version, cross-compile, tag, and publish the GitHub release.

## Related pages

- [tools/ — pinned helper binaries](../tools/README.md)
- [../README.md](../README.md)
- [../AGENTS.md](../AGENTS.md)
