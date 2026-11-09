#!/usr/bin/env bash
# Format the runner-spawner repository in place with formattington.
#
#   ./format.bash              # format all matching files in place
#   ./format.bash --series     # format sequentially (ordered output)
#   ./format.bash --verbose    # show which formatter executables were resolved
#
# Formattington (https://github.com/fordaj/formattington) is pinned in
# pyproject.toml (git commit) and locked in uv.lock. This project uses
# Python 3.14 (see .python-version), the latest version it supports.
# No pip/venv setup is needed: `uv run` creates .venv/ automatically.
#
# Scope (see .formattington.json):
#   markdown (*.md) / json (*.json)   -> prettier
#   Go (*.go)                         -> goimports (gofmt + import fixing)
#   shell (*.sh, *.bash)              -> shfmt (-i 4)
#
# Prerequisites (formatter executables must be on PATH; formattington
# exits 127 if one is missing):
#   * uv (https://docs.astral.sh/uv/)
#   * Go (for goimports; `go install golang.org/x/tools/cmd/goimports@latest`)
#   * shfmt, prettier (e.g. via your OS package manager;
#     prettier: `npm install -g prettier`)
#
# Style is configured by .prettierrc at the repo root. The bundled
# binaries under tools/ are skipped (as are dist/ and other generated
# locations).
#
# To check formatting without modifying files (CI):
#   ./ci/format_check.bash
#
# Extra flags are forwarded to `formattington` before `all`
# (e.g. --series, --verbose). --config is managed here; do not pass it.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$SCRIPT_DIR"
cd "$REPO_ROOT"

if ! command -v uv >/dev/null 2>&1; then
    echo "ERROR: uv not found (https://docs.astral.sh/uv/)" >&2
    exit 1
fi

# goimports installed via `go install` lands in $(go env GOPATH)/bin,
# which is not always on PATH. Prepend it when Go is available.
if command -v go >/dev/null 2>&1; then
    GOPATH_BIN="$(go env GOPATH)/bin"
    if [[ -d "$GOPATH_BIN" && ":$PATH:" != *":$GOPATH_BIN:"* ]]; then
        export PATH="$GOPATH_BIN:$PATH"
    fi
fi

exec uv run formattington "$@" --config "$REPO_ROOT/.formattington.json" all
