# Version stamping (`version.go`)

`version.go` holds exactly one declaration: `toolVersion`, the string printed by `runner-spawner --version` (`runner-spawner <version>`) and read at startup by [main.md](main.md).

## How it resolves

`toolVersion` is `"dev"` in source and is replaced at link time with `-ldflags="-X main.toolVersion=<tag>"`. Both build paths inject it:

- `make build` / `make cross` (Makefile `LDFLAGS`)
- `scripts/release.sh` (see [scripts/release.md](scripts/release.md)), which computes the next tag with the bundled versioner and stamps it into every cross-compiled binary

A plain `go build` therefore reports `runner-spawner dev`; release artifacts report their tag. The pinned bundle under `tools/runner-spawner/` is built the same way (see [tools/runner-spawner/update.md](tools/runner-spawner/update.md)).

## Related pages

- [main.md](main.md) — the `--version` subcommand
- [scripts/version.md](scripts/version.md) — prints the computed tag without building
- [scripts/release.md](scripts/release.md) — where the `-X` injection happens for releases
