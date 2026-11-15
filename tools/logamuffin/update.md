# The logamuffin bundle (`tools/logamuffin/`)

[Logamuffin](https://github.com/fordaj/logamuffin) is the structured logger used by this repository's scripts: timestamped, leveled `log <level> <message>` records. The scripts' `log()` wrapper ([scripts/entry.md](../../scripts/entry.md)) forwards to the pinned binary when it is present and falls back to an equivalent `printf` implementation when it is not, so a fresh clone works without it.

The scripts themselves document the contract (`log INFO "..."`), the `LOG_LEVEL`/`LOGAMUFFIN_LEVEL` filter, and the `LOG_SOURCE` caller-context field that entry.sh injects.

## Bundled platforms

`v0.1.3.0.35/macos-arm64`, `macos-x86_64`, `linux-arm64`, `linux-x86_64`, `windows-arm64`, `windows-x86_64` (the scripts pick the host's platform via `LOGAMUFFIN_BIN`).

## `update.bash` in detail

Refreshes from `fordaj/logamuffin` releases: platform archives whose stem ends in an `<os>-<arch>` suffix (`darwin-arm64`, `windows-x86_64`, … — mapped onto the bundle layout, e.g. `darwin-amd64` → `macos-x86_64`), in `.tar.gz` on macOS/Linux and `.zip` on Windows (legacy `.7z` accepted), verified against the release's `checksums.txt`/`SHA256SUMS.txt` when present — warning rather than fatal when absent. Only the binary is kept. Stages into a temporary directory inside the bundle, swaps in atomically, prunes superseded `v*/` folders only after the new one is staged, and smoke-tests the host binary. Environment overrides: `LOGAMUFFIN_REPO`, `LOGAMUFFIN_TAG`. See [../README.md](../README.md) for the shared bundle contract (`--dry-run`, `--tag`, staged atomic swap).

## Related pages

- [../README.md](../README.md) — the bundle index
- [../../scripts/entry.md](../../scripts/entry.md) — the `log()` wrapper that calls it
