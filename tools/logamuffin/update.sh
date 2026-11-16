#!/usr/bin/env bash
# update.sh — refresh bundled logamuffin binaries from fordaj/logamuffin releases.
#
# Thin wrapper around ../update.sh (the shared implementation; see
# tools/update.sh.md). Usage and flags are identical to the other bundles:
#   tools/logamuffin/update.sh              # download latest, replace old versions
#   tools/logamuffin/update.sh --dry-run    # resolve tag + list assets, change nothing
#   tools/logamuffin/update.sh --help       # usage
#
# Only the binary is kept. Env:
#   LOGAMUFFIN_REPO  Repository slug (default: fordaj/logamuffin)
#   LOGAMUFFIN_TAG   Pinned tag (default: latest release)

set -euo pipefail

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
export UPDATE_TOOL=logamuffin
export UPDATE_BIN=logamuffin
export UPDATE_REPO=fordaj/logamuffin
export UPDATE_ASSET_SUFFIX=
export UPDATE_EXTRAS=
export UPDATE_REQUIRE_CHECKSUMS=0
export TAG="${LOGAMUFFIN_TAG:-}"
export REPO="${LOGAMUFFIN_REPO:-$UPDATE_REPO}"
exec bash "$HERE/../update.sh" "$@"
