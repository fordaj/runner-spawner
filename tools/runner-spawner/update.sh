#!/usr/bin/env bash
# update.sh — refresh bundled runner-spawner binaries from fordaj/runner-spawner releases.
#
# Thin wrapper around ../update.sh (the shared implementation; see
# tools/update.sh.md). Usage and flags are identical to the other bundles:
#   tools/runner-spawner/update.sh              # download latest, replace old versions
#   tools/runner-spawner/update.sh --dry-run    # resolve tag + list assets, change nothing
#   tools/runner-spawner/update.sh --help       # usage
#
# Only the binary is kept. Verification is mandatory: a release with no
# checksums.txt/SHA256SUMS.txt aborts the refresh. Env:
#   RUNNER_SPAWNER_TOOL_REPO  Repository slug (default: fordaj/runner-spawner);
#                             named deliberately to not collide with the
#                             CLI's own RUNNER_SPAWNER_REPO (target repo)
#   RUNNER_SPAWNER_TAG        Pinned tag (default: latest release)

set -euo pipefail

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
export UPDATE_TOOL=runner-spawner
export UPDATE_BIN=runner-spawner
export UPDATE_REPO=fordaj/runner-spawner
export UPDATE_ASSET_SUFFIX=
export UPDATE_EXTRAS=
export UPDATE_REQUIRE_CHECKSUMS=1
export TAG="${RUNNER_SPAWNER_TAG:-}"
export REPO="${RUNNER_SPAWNER_TOOL_REPO:-$UPDATE_REPO}"
exec bash "$HERE/../update.sh" "$@"
