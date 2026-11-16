#!/usr/bin/env bash
# update.sh — refresh bundled Versioner binaries from fordaj/Versioner releases.
#
# Thin wrapper around ../update.sh (the shared implementation; see
# tools/update.sh.md). Usage and flags are identical to the other bundles:
#   tools/versioner/update.sh              # download latest, replace old versions
#   tools/versioner/update.sh --dry-run    # resolve tag + list assets, change nothing
#   tools/versioner/update.sh --help       # usage
#
# Only the binary is kept. Env:
#   VERSIONER_REPO  Repository slug (default: fordaj/Versioner)
#   VERSIONER_TAG   Pinned tag (default: latest release)

set -euo pipefail

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
export UPDATE_TOOL=versioner
export UPDATE_BIN=versioner
export UPDATE_REPO=fordaj/Versioner
export UPDATE_ASSET_SUFFIX=
export UPDATE_EXTRAS=
export UPDATE_REQUIRE_CHECKSUMS=0
export TAG="${VERSIONER_TAG:-}"
export REPO="${VERSIONER_REPO:-$UPDATE_REPO}"
exec bash "$HERE/../update.sh" "$@"
