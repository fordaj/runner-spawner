# The barnacle bundle (`tools/barnacle/`)

[Barnacle](https://github.com/fordaj/barnacle) is a documentation-site generator: it renders the `.md` and `.mmd` files scattered through a repository into a static, navigable HTML site. This repository uses it to render *this* documentation; the site you may be reading was built by it.

## Layout

```text
tools/barnacle/
  update.bash                     # refresh from fordaj/barnacle releases
  v0.0.1.0.5/
    macos-arm64/barnacle
    macos-x86_64/barnacle
    linux-arm64/barnacle
    linux-x86_64/barnacle
    windows-arm64/barnacle.exe
    windows-x86_64/barnacle.exe
```

All six platforms are bundled. The pinned v0.0.1.0.5 bundle in this repository currently carries only the binaries (no `LICENSE` files in the tree).

## `update.bash` in detail

The script:

1. Resolves the tag (latest release, or `--tag vX.Y.Z...`), validating the tag shape (`^v[0-9]+(\.[0-9]+)+$`).
2. Downloads via `gh` (`*.tar.gz`, `*.zip`, legacy `*.7z`, plus `checksums.txt`/`SHA256SUMS.txt`), then verifies the downloaded archives against the checksums file (`shasum -a 256 -c` or `sha256sum -c`); a missing checksums file is a warning, not fatal.
3. Extracts each archive and copies the binary into `v<tag>/<platform>/`. When an extracted archive ships a `LICENSE` file, it is copied next to the binary in the same platform directory.
4. Swaps in the new version directory and deletes superseded ones, then smoke-tests the host binary (`barnacle --version`), warning if the reported version does not contain the tag.

Environment overrides: `BARNACLE_REPO`, `BARNACLE_TAG`. `--dry-run` lists assets and planned deletions without touching anything.

Note this bundle is refreshed independently of the runner-spawner release pipeline — run `tools/barnacle/update.bash` after a docs-tooling bump, not as part of `scripts/release.sh`.

## How the repository uses it

[scripts/docs.sh](../../scripts/docs.md) resolves `BARNACLE_BIN` through the [env cascade](../../scripts/env.md) and generates `./site/` from the repo root; the output is git-ignored.

## Related pages

- [../README.md](../README.md) — the bundle index
- [../../scripts/README.md](../../scripts/README.md) — the scripts that consume the bundle
- [../../scripts/docs.md](../../scripts/docs.md)
