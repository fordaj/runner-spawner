#!/usr/bin/env bash
# update.sh — refresh bundled formattington-lite binaries from fordaj/formattington releases.
#
# Thin wrapper around ../update.sh (the shared implementation; see
# tools/update.sh.md). Usage and flags are identical to the other bundles:
#   tools/formattington-lite/update.sh              # download latest, replace old versions
#   tools/formattington-lite/update.sh --dry-run    # resolve tag + list assets, change nothing
#   tools/formattington-lite/update.sh --help       # usage
#
# Only the -lite binaries are kept (plus NOTICE.md and LICENSES/ when
# present). Env:
#   FORMATTINGTON_REPO  Repository slug (default: fordaj/formattington)
#   FORMATTINGTON_TAG   Pinned tag (default: latest release)

set -euo pipefail

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
export UPDATE_TOOL=formattington-lite
export UPDATE_BIN=formattington
export UPDATE_REPO=fordaj/formattington
export UPDATE_ASSET_SUFFIX=-lite
export UPDATE_EXTRAS="NOTICE.md LICENSES"
export UPDATE_REQUIRE_CHECKSUMS=0
export TAG="${FORMATTINGTON_TAG:-}"
export REPO="${FORMATTINGTON_REPO:-$UPDATE_REPO}"
exec bash "$HERE/../update.sh" "$@"
