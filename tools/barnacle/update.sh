#!/usr/bin/env bash
# update.sh — refresh bundled barnacle binaries from fordaj/barnacle releases.
#
# Thin wrapper around ../update.sh (the shared implementation; see
# tools/update.sh.md). Usage and flags are identical to the other bundles:
#   tools/barnacle/update.sh              # download latest, replace old versions
#   tools/barnacle/update.sh --dry-run    # resolve tag + list assets, change nothing
#   tools/barnacle/update.sh --help       # usage
#
# Only the binary is kept, plus the upstream MIT LICENSE when present.
# Env:
#   BARNACLE_REPO  Repository slug (default: fordaj/barnacle)
#   BARNACLE_TAG   Pinned tag (default: latest release)

set -euo pipefail

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
export UPDATE_TOOL=barnacle
export UPDATE_BIN=barnacle
export UPDATE_REPO=fordaj/barnacle
export UPDATE_ASSET_SUFFIX=
export UPDATE_EXTRAS="LICENSE"
export UPDATE_REQUIRE_CHECKSUMS=0
export TAG="${BARNACLE_TAG:-}"
export REPO="${BARNACLE_REPO:-$UPDATE_REPO}"
exec bash "$HERE/../update.sh" "$@"
