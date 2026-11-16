#!/usr/bin/env bash
# update.sh — shared implementation behind the per-bundle update.sh wrappers
# in tools/<name>/. Do not run this script directly; use the wrapper for the
# bundle you want to refresh (for example tools/barnacle/update.sh).
#
# The wrapper resolves its own directory, exports the bundle parameters
# below, and execs this script with the caller's arguments. Everything
# else (release resolution, checksum verification, staged atomic swap,
# old-version pruning, host smoke test) is implemented here once.
#
# Bundle parameters (exported by the wrapper):
#   UPDATE_TOOL              bundle directory name under tools/ (e.g. barnacle)
#   UPDATE_BIN               binary basename inside the archives (e.g. barnacle)
#   UPDATE_REPO              default upstream repository slug
#   UPDATE_ASSET_SUFFIX      optional asset-stem suffix assets must carry
#                            (e.g. -lite; empty accepts every platform archive)
#   UPDATE_EXTRAS            space-separated extra paths copied next to each
#                            binary when present (e.g. "LICENSE" or
#                            "NOTICE.md LICENSES"); copied with cp -pr
#   UPDATE_REQUIRE_CHECKSUMS 1 to abort when the release ships no checksums
#                            file, 0 to warn and continue
#   TAG                      pinned tag (empty = latest release)
#   REPO                     repository slug (default: UPDATE_REPO)
#
# Prerequisites: gh (https://cli.github.com/) and tar/unzip (.7z assets
# additionally need 7z/7zz). Logging uses log <level> <message> from
# scripts/entry.sh (logamuffin, with printf fallback).
#
# Layout after a successful run (example for v0.1.2.3):
#   tools/<name>/
#     update.sh
#     v0.1.2.3/
#       macos-arm64/<bin>
#       macos-x86_64/<bin>
#       linux-arm64/<bin>
#       linux-x86_64/<bin>
#       windows-x86_64/<bin>

set -euo pipefail

: "${UPDATE_TOOL:?update.sh must be run through a tools/<name>/update.sh wrapper (UPDATE_TOOL not set)}"
: "${UPDATE_BIN:?UPDATE_BIN must be set by the wrapper}"
: "${UPDATE_REPO:?UPDATE_REPO must be set by the wrapper}"
UPDATE_ASSET_SUFFIX="${UPDATE_ASSET_SUFFIX:-}"
UPDATE_EXTRAS="${UPDATE_EXTRAS:-}"
UPDATE_REQUIRE_CHECKSUMS="${UPDATE_REQUIRE_CHECKSUMS:-0}"

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
REPO_ROOT_FOR_ENV=$(cd -- "$SCRIPT_DIR/.." &>/dev/null && pwd)
if [[ ! -f "$REPO_ROOT_FOR_ENV/scripts/entry.sh" ]]; then
    printf 'update.sh: error: entry point not found: %s/scripts/entry.sh\n' "$REPO_ROOT_FOR_ENV" >&2
    exit 1
fi
# shellcheck disable=SC1091
. "$REPO_ROOT_FOR_ENV/scripts/entry.sh"

DRY_RUN=0
TAG="${TAG:-}"
REPO="${REPO:-$UPDATE_REPO}"
WRAPPER="tools/$UPDATE_TOOL/update.sh"

usage() {
    cat <<EOF
Usage: $WRAPPER [--dry-run] [--tag TAG] [--repo REPO]

Download the latest $UPDATE_TOOL release from GitHub via gh, extract each
platform binary into tools/$UPDATE_TOOL/<tag>/<platform>/ (platform is the
bundled <os>-<arch> layout, e.g. darwin-amd64 -> macos-x86_64), and delete old
version folders. Only the binary is kept${UPDATE_EXTRAS:+ (plus: $UPDATE_EXTRAS)}.

Flags:
  --dry-run, -n   Resolve tag + list planned assets, change nothing.
  --tag TAG       Pin a specific release tag (default: latest release).
  --repo REPO     Repository slug (default: $UPDATE_REPO).
  --help, -h      Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
    --dry-run | -n)
        DRY_RUN=1
        shift
        ;;
    --help | -h)
        usage
        exit 0
        ;;
    --tag)
        [[ $# -ge 2 ]] || {
            log ERROR "--tag requires a value"
            usage >&2
            exit 2
        }
        TAG="$2"
        shift 2
        ;;
    --tag=*)
        TAG="${1#--tag=}"
        shift
        ;;
    --repo)
        [[ $# -ge 2 ]] || {
            log ERROR "--repo requires a value"
            usage >&2
            exit 2
        }
        REPO="$2"
        shift 2
        ;;
    --repo=*)
        REPO="${1#--repo=}"
        shift
        ;;
    *)
        log ERROR "unknown argument: $1"
        usage >&2
        exit 2
        ;;
    esac
done

[[ -n "$REPO" ]] || {
    log ERROR "repository must not be empty"
    exit 2
}

die() {
    log ERROR "$*" || true
    exit 1
}
need_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

# keep_asset <stem> reports whether the archive stem belongs to this bundle
# (formattington-lite, for example, only accepts -lite assets).
keep_asset() {
    [[ "$UPDATE_ASSET_SUFFIX" == "" || "$1" == *"$UPDATE_ASSET_SUFFIX" ]]
}

# strip_archive_ext drops the archive extension (.tar.gz/.zip/.7z) from a name.
strip_archive_ext() {
    local name="$1"
    case "$name" in
    *.tar.gz) printf '%s' "${name%.tar.gz}" ;;
    *.zip) printf '%s' "${name%.zip}" ;;
    *.7z) printf '%s' "${name%.7z}" ;;
    *) printf '%s' "$name" ;;
    esac
}

# parse_stem_to_platform <stem> sets PLATFORM (e.g.
# Versioner_v0.0.3.0.5_darwin-arm64 -> macos-arm64) or returns 1 for
# unknown values. Accepts any single separator before the os-arch suffix and
# normalizes x64 -> x86_64, amd64 -> x86_64, x86_64 -> x86_64, darwin -> macos.
parse_stem_to_platform() {
    local stem="$1"
    # stem may end in -lite/-full; the regex below matches it directly.
    if [[ "$stem" =~ [-_](darwin-amd64|darwin-arm64|darwin-x86_64|macos-x86_64|macos-arm64|macos-x64|macos-amd64|linux-x86_64|linux-amd64|linux-x64|linux-arm64|windows-x86_64|windows-amd64|windows-x64|windows-arm64)(-lite|-full)?$ ]]; then
        case "${BASH_REMATCH[1]}" in
        darwin-amd64 | darwin-x86_64 | macos-x86_64 | macos-x64 | macos-amd64) PLATFORM="macos-x86_64" ;;
        darwin-arm64 | macos-arm64) PLATFORM="macos-arm64" ;;
        linux-x86_64 | linux-amd64 | linux-x64) PLATFORM="linux-x86_64" ;;
        linux-arm64) PLATFORM="linux-arm64" ;;
        windows-x86_64 | windows-amd64 | windows-x64) PLATFORM="windows-x86_64" ;;
        windows-arm64) PLATFORM="windows-arm64" ;;
        *) return 1 ;;
        esac
        VARIANT="${BASH_REMATCH[2]#-}"
        return 0
    fi
    return 1
}

# host_platform echoes this machine's bundle-platform name (e.g. macos-arm64).
host_platform() {
    local os_raw arch_raw host_os host_arch
    os_raw="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch_raw="$(uname -m | tr '[:upper:]' '[:lower:]')"
    case "$os_raw" in
    darwin*) host_os="macos" ;;
    linux*) host_os="linux" ;;
    msys* | mingw* | cygwin* | windows* | win*) host_os="windows" ;;
    *) host_os="$os_raw" ;;
    esac
    case "$arch_raw" in
    x86_64 | amd64) host_arch="x86_64" ;;
    arm64 | aarch64) host_arch="arm64" ;;
    *) host_arch="$arch_raw" ;;
    esac
    printf '%s-%s' "$host_os" "$host_arch"
}

# print_dry_run lists the tag's assets and the old version folders that
# would be removed, without changing anything.
print_dry_run() {
    log INFO "DRY RUN: no download, extraction, or deletion will happen"
    log INFO "release $TAG assets:"
    local assets_list
    assets_list=$(gh release view "$TAG" --repo "$REPO" --json assets -q '.assets[].name' ||
        die "could not list assets for $TAG in $REPO")
    [[ -n "$assets_list" ]] || die "no assets found for $TAG in $REPO"
    local asset stem asset_count=0
    while IFS= read -r asset; do
        [[ -n "$asset" ]] || continue
        case "$asset" in
        *.tar.gz | *.zip | *.7z)
            stem="$(strip_archive_ext "$asset")"
            if ! keep_asset "$stem"; then
                log INFO "  $asset (ignored: not a $UPDATE_ASSET_SUFFIX archive)"
                continue
            fi
            asset_count=$((asset_count + 1))
            if parse_stem_to_platform "$stem"; then
                log INFO "  $asset -> $TAG/$PLATFORM"
            else
                log INFO "  $asset -> unknown platform (would fail)"
            fi
            ;;
        checksums.txt | SHA256SUMS.txt)
            log INFO "  $asset (checksums)"
            ;;
        *)
            log INFO "  $asset (ignored)"
            ;;
        esac
    done <<<"$assets_list"
    ((asset_count > 0)) || die "no platform archives found for $TAG in $REPO"
    shopt -s nullglob
    local old_found=0 dir
    for dir in "$TOOL_ROOT"/v*/; do
        [[ -d "$dir" ]] || continue
        local dir_name
        dir_name="$(basename "$dir")"
        if [[ "$dir_name" != "$TAG" ]]; then
            log INFO "would remove old $dir_name"
            old_found=1
        fi
    done
    if [[ "$old_found" -eq 0 ]]; then
        log INFO "no old version folders to remove"
    fi
    log INFO "dry run complete: would update $TOOL_ROOT/$TAG"
}

# verify_checksums checks the downloaded archives against the release's
# checksums file, when the release ships one. A missing file aborts the
# refresh when UPDATE_REQUIRE_CHECKSUMS=1 (the runner-spawner bundle).
verify_checksums() {
    local checksums_file=""
    local candidate
    for candidate in checksums.txt SHA256SUMS.txt; do
        if [[ -f "$TMP_DIR/$candidate" ]]; then
            checksums_file="$TMP_DIR/$candidate"
            break
        fi
    done
    if [[ -z "$checksums_file" ]]; then
        if [[ "$UPDATE_REQUIRE_CHECKSUMS" == "1" ]]; then
            die "no checksums.txt/SHA256SUMS.txt in $TAG release; refusing to install unverified binaries"
        fi
        log WARNING "no checksums in $TAG release; skipping verification"
        return
    fi
    log INFO "verifying checksums ($(basename "$checksums_file"))..."
    # The checksums file may be cumulative across releases (or list every
    # uploaded asset), so verify only the archives downloaded for this run.
    local checksum_tool=""
    if command -v shasum >/dev/null 2>&1; then
        checksum_tool="shasum -a 256"
    elif command -v sha256sum >/dev/null 2>&1; then
        checksum_tool="sha256sum"
    fi
    [[ -n "$checksum_tool" ]] || die "checksums file present but neither shasum nor sha256sum found"
    local verify_list="$TMP_DIR/.verify-list"
    : >"$verify_list"
    local archive base
    for archive in "${archives[@]}"; do
        base="$(basename "$archive")"
        grep -F " $base" "$checksums_file" >>"$verify_list" ||
            die "no checksum entry for downloaded asset $base in $(basename "$checksums_file")"
    done
    # Entries are "<sha>  <name>" (or with a leading *), relative to $TMP_DIR.
    (cd "$TMP_DIR" && $checksum_tool -c "$verify_list") || die "checksum verification failed for $TAG"
    log INFO "checksums verified"
}

# extract_archive unpacks one archive into a destination directory,
# format-agnostic (.tar.gz, .zip, legacy .7z).
extract_archive() {
    local archive="$1" dest="$2"
    case "$archive" in
    *.tar.gz)
        tar -xzf "$archive" -C "$dest"
        ;;
    *.zip)
        need_cmd unzip
        unzip -q "$archive" -d "$dest"
        ;;
    *.7z)
        if command -v 7zz >/dev/null 2>&1; then
            7zz x -y "$archive" -o"$dest" >/dev/null
        elif command -v 7z >/dev/null 2>&1; then
            7z x -y "$archive" -o"$dest" >/dev/null
        else
            die "legacy .7z asset $(basename "$archive") requires 7z/7zz (this release predates platform-native archives)"
        fi
        ;;
    *)
        die "unsupported archive format: $archive"
        ;;
    esac
}

# install_archive extracts one downloaded archive and installs the binary
# (plus any UPDATE_EXTRAS) into the staging directory.
install_archive() {
    local archive="$1"
    local base stem extract_dir bin_src cand
    base="$(basename "$archive")"
    stem="$(strip_archive_ext "$base")"
    if ! parse_stem_to_platform "$stem"; then
        die "unknown platform in asset '$base' (expected <os>-<arch> such as darwin-arm64)"
    fi
    if [[ -e "$STAGE_DIR/$PLATFORM" ]]; then
        die "duplicate asset for platform $PLATFORM (from $base)"
    fi
    log INFO "extracting $base -> $TAG/$PLATFORM"
    extract_dir="$TMP_DIR/extract-$PLATFORM"
    mkdir -p "$extract_dir"
    extract_archive "$archive" "$extract_dir"
    bin_src=""
    for cand in "$extract_dir/$UPDATE_BIN" "$extract_dir/$UPDATE_BIN.exe"; do
        if [[ -f "$cand" ]]; then
            bin_src="$cand"
            break
        fi
    done
    if [[ -z "$bin_src" ]]; then
        # Some archives nest everything under a single top-level directory.
        for cand in "$extract_dir"/*/"$UPDATE_BIN" "$extract_dir"/*/"$UPDATE_BIN.exe"; do
            if [[ -f "$cand" ]]; then
                bin_src="$cand"
                break
            fi
        done
    fi
    [[ -n "$bin_src" ]] || die "no $UPDATE_BIN binary in $base (expected $UPDATE_BIN)"
    local bin_name target_dir extra
    bin_name="$(basename "$bin_src")"
    target_dir="$STAGE_DIR/$PLATFORM"
    mkdir -p "$target_dir"
    cp -p "$bin_src" "$target_dir/$bin_name"
    chmod +x "$target_dir/$bin_name"
    [[ -s "$target_dir/$bin_name" ]] || die "installed binary is empty: $target_dir/$bin_name"
    for extra in $UPDATE_EXTRAS; do
        if [[ -e "$extract_dir/$extra" ]]; then
            cp -pr "$extract_dir/$extra" "$target_dir/"
        fi
    done
    log INFO "installed $PLATFORM/$bin_name (from $base)"
    rm -rf -- "$extract_dir"
}

# report_host_binary smoke-tests the bundled binary for this machine and
# warns when its version does not contain the tag.
report_host_binary() {
    local tag_dir="$1"
    local platform host_bin cand
    platform="$(host_platform)"
    host_bin=""
    for cand in "$tag_dir/$platform/$UPDATE_BIN" "$tag_dir/$platform/$UPDATE_BIN.exe"; do
        if [[ -x "$cand" ]]; then
            host_bin="$cand"
        fi
    done
    if [[ -z "$host_bin" ]]; then
        log WARNING "host platform $platform not in bundle; skipped version check"
        return
    fi
    local host_ver
    host_ver="$("$host_bin" --version 2>&1 || true)"
    log INFO "host binary $platform reports: $host_ver"
    case "$host_ver" in
    *"$TAG"* | *"${TAG#v}"*) ;;
    *) log WARNING "host binary version '$host_ver' does not contain $TAG" ;;
    esac
}

need_cmd gh
need_cmd tar

TOOL_ROOT="$SCRIPT_DIR/$UPDATE_TOOL"
[[ -d "$TOOL_ROOT" ]] || die "$UPDATE_TOOL bundle directory not found: $TOOL_ROOT"
log INFO "$UPDATE_TOOL bundle directory: $TOOL_ROOT"
log INFO "repository: $REPO"

if [[ -z "$TAG" ]]; then
    TAG="$(gh release view --repo "$REPO" --json tagName -q .tagName)" ||
        die "could not determine latest release from $REPO (is gh authenticated and the repo visible?)"
fi
[[ -n "$TAG" ]] || die "could not determine release tag from $REPO"
if ! [[ "$TAG" =~ ^v[0-9]+(\.[0-9]+)+$ ]]; then
    die "invalid tag '$TAG' (expected vX.Y.Z...)"
fi
log INFO "release tag: $TAG"

if [[ "$DRY_RUN" -eq 1 ]]; then
    print_dry_run
    exit 0
fi

# --- Real update: download to a temp dir, stage the bundle, then swap. ---
TMP_DIR=""
STAGE_DIR=""
cleanup() {
    if [[ -n "${TMP_DIR:-}" && -d "$TMP_DIR" ]]; then
        rm -rf -- "$TMP_DIR"
    fi
    if [[ -n "${STAGE_DIR:-}" && -d "$STAGE_DIR" ]]; then
        rm -rf -- "$STAGE_DIR"
    fi
}
trap cleanup EXIT

TMP_DIR="$(mktemp -d)"
STAGE_DIR="$(mktemp -d "$TOOL_ROOT/.tmp.XXXXXX")"

log INFO "downloading $TAG assets from $REPO..."
gh release download "$TAG" --repo "$REPO" --dir "$TMP_DIR" --pattern '*.tar.gz' --pattern '*.zip' --pattern '*.7z' --pattern 'checksums.txt' --pattern 'SHA256SUMS.txt' --clobber

shopt -s nullglob
archives=()
for archive in "$TMP_DIR"/*.tar.gz "$TMP_DIR"/*.zip "$TMP_DIR"/*.7z; do
    [[ -e "$archive" ]] || continue
    stem="$(strip_archive_ext "$(basename "$archive")")"
    keep_asset "$stem" || continue
    archives+=("$archive")
done
((${#archives[@]} > 0)) || die "no platform archives downloaded from $REPO $TAG"
log INFO "downloaded ${#archives[@]} platform archives"

verify_checksums

for archive in "${archives[@]}"; do
    install_archive "$archive"
done

TAG_DIR="$TOOL_ROOT/$TAG"
rm -rf -- "$TAG_DIR"
mkdir -p "$TOOL_ROOT"
mv "$STAGE_DIR" "$TAG_DIR"
STAGE_DIR=""

for dir in "$TOOL_ROOT"/v*/; do
    [[ -d "$dir" ]] || continue
    dir_name="$(basename "$dir")"
    if [[ "$dir_name" != "$TAG" ]]; then
        log INFO "removing old $dir_name"
        rm -rf -- "$dir"
    fi
done

report_host_binary "$TAG_DIR"

log INFO "done: $TAG_DIR now contains:"
for dir in "$TAG_DIR"/*/; do
    [[ -d "$dir" ]] || continue
    log INFO "  $(basename "$dir")"
done
