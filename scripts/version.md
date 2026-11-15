# The version script (`scripts/version.bash`)

Prints the version of this repository as computed by the pinned [versioner](../tools/versioner/update.md) — the same computation the release pipeline uses, so a local build and the published tag cannot drift apart.

## Usage

```bash
scripts/version.bash                 # VERSION=<v> and TAG=<t> lines (default)
scripts/version.bash --format text   # same
scripts/version.bash --format shell  # export VERSION=... and export TAG=...
scripts/version.bash --format json   # the raw versioner JSON document
```

`--format` requires a value; `--help`/`-h` prints usage. Unknown arguments are a usage error (exit `2`); a missing or non-executable `VERSIONER_BIN` fails with exit `1`.

## How it works

Sources [entry.md](entry.md) for the cascade and `log()`. If `VERSIONER_BIN` is unset or not executable, it fails with `no versioner binary found (run tools/versioner/update.bash or set VERSIONER_BIN)`. Otherwise it runs `versioner --repo-dir <root> --quiet [--format json]`; the repository root defaults to `VERSIONER_REPO_DIR` or the parent of `scripts/` (anchored via `REPO_ROOT_DIR`).

## Consumers

- `scripts/release.sh` — computes the next tag, stamps it into the binaries, pushes the tag (see [release.md](release.md)).
- The release binary itself — see [../version.md](../version.md) for how `--version` resolves at run time.

## Related pages

- [entry.md](entry.md) · [env.md](env.md)
- [release.md](release.md)
- [../tools/versioner/update.md](../tools/versioner/update.md)
