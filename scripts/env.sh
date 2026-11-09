#!/usr/bin/env bash
# env.sh — source the runner-spawner env cascade, select the
# host-appropriate versioner, logamuffin, and runner-spawner binaries,
# and provide log() (the logamuffin helper, so scripts call
# log <level> <message> instead of calling logamuffin directly).
#
# Sourced usage (bash):
#   source scripts/env.sh   # or: . scripts/env.sh
#   "$VERSIONER_BIN" -quiet
#   "$RUNNER_SPAWNER_BIN" --help
#   log INFO "Starting application"
#
# Executed usage (prints export statements and log() for eval in any shell):
#   eval "$(bash scripts/env.sh)"
#   bash scripts/env.sh
#
# Cascade (all committed): scripts/.common.env (base) then
# scripts/.$PLATFORM.env (os-specific, e.g. .macos-x86_64.env),
# where $PLATFORM is the hyphenated <os>-<arch> from uname
# (macos/linux/windows + x86_64/arm64, matching the bundled layout).
# env.sh then resolves $VERSIONER_BIN to the newest bundled binary
# (tools/versioner/v*/$PLATFORM/versioner[.exe]; tools/versioner/update.bash
# keeps exactly one v*/ dir), falling back to `versioner` on PATH,
# and resolves $LOGAMUFFIN_BIN the same way from
# tools/logamuffin/v*/$PLATFORM/logamuffin[.exe]
# (tools/logamuffin/update.bash keeps exactly one v*/ dir), falling
# back to `logamuffin` on PATH, and resolves $RUNNER_SPAWNER_BIN
# the same way from tools/runner-spawner/v*/$PLATFORM/runner-spawner[.exe]
# (tools/runner-spawner/update.bash keeps exactly one v*/ dir), falling
# back to `runner-spawner` on PATH. A caller-provided non-empty
# VERSIONER_BIN/LOGAMUFFIN_BIN/RUNNER_SPAWNER_BIN wins and is not overwritten.
#
# Exports:
#   PROJECT_PLATFORM  host platform (e.g. macos-x86_64)
#   VERSIONER_BIN           absolute path to versioner ("" if not found)
#   VERSIONER_BIN_NAME      versioner or versioner.exe
#   VERSIONER_REPO, VERSIONER_TAG_PATTERN, VERSIONER_CONFIG_FILE
#   VERSIONER_BUNDLE_DIR (repo-relative)
#   LOGAMUFFIN_BIN          absolute path to logamuffin ("" if not found)
#   LOGAMUFFIN_BIN_NAME     logamuffin or logamuffin.exe
#   LOGAMUFFIN_REPO, LOGAMUFFIN_BUNDLE_DIR (repo-relative)
#   RUNNER_SPAWNER_BIN          absolute path to runner-spawner ("" if not found)
#   RUNNER_SPAWNER_BIN_NAME     runner-spawner or runner-spawner.exe
#   RUNNER_SPAWNER_REPO, RUNNER_SPAWNER_BUNDLE_DIR (repo-relative)
#
# Functions (when sourced; also printed for eval when executed):
#   log LEVEL [MESSAGE ...]
#     LEVEL may be a word (TRACE, INFO, WARNING, ERROR, CRITICAL)
#     or a number (50). All arguments are forwarded to logamuffin
#     when a binary is available; otherwise log falls back to
#     printf so scripts work before tools/logamuffin/update.bash
#     has run. log sets LOG_SOURCE per call from the caller's
#     file:line (plus function when available) and preserves the
#     binary's exit code. It does not override LOG_LEVEL, LOG_FILE,
#     LOG_COLOR, LOG_CONFIG, or LOG_LEVELS — export them once per
#     script and log respects them (logamuffin defaults when unset).
#     LOG_SOURCE is the exception: log always sets it per call.
#     The fallback respects LOG_LEVEL (default INFO) the same way,
#     writes levels >= ERROR to stderr, and returns 0 (2 for a
#     usage error with no level), so `set -e` is unaffected.

# Resolve this file's directory (works sourced or executed, from anywhere).
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
    _env_sh_path="${BASH_SOURCE[0]}"
else
    _env_sh_path="$0"
fi
_env_sh_dir=$(cd -- "$(dirname -- "$_env_sh_path")" &>/dev/null && pwd)
_repo_root=$(dirname -- "$_env_sh_dir")

# Detect host platform (uname-based, matching the bundled <os>-<arch> layout).
_host_os_raw=$(uname -s | tr '[:upper:]' '[:lower:]')
_host_arch_raw=$(uname -m | tr '[:upper:]' '[:lower:]')
case "$_host_os_raw" in
darwin*) _host_os="macos" ;;
linux*) _host_os="linux" ;;
msys* | mingw* | cygwin* | windows* | win*) _host_os="windows" ;;
*) _host_os="$_host_os_raw" ;;
esac
case "$_host_arch_raw" in
x86_64 | amd64) _host_arch="x86_64" ;;
arm64 | aarch64) _host_arch="arm64" ;;
*) _host_arch="$_host_arch_raw" ;;
esac
_detected_platform="$_host_os-$_host_arch"

# Source cascade (base, then platform-specific). set -a auto-exports.
set -a
# shellcheck disable=SC1090,SC1091
. "$_env_sh_dir/.common.env"
if [[ -f "$_env_sh_dir/.$_detected_platform.env" ]]; then
    # shellcheck disable=SC1090,SC1091
    . "$_env_sh_dir/.$_detected_platform.env"
else
    echo "env.sh: warning: no $_env_sh_dir/.$_detected_platform.env; using defaults" >&2
    PROJECT_PLATFORM="$_detected_platform"
    if [[ "$_host_os" == "windows" ]]; then
        VERSIONER_BIN_NAME="versioner.exe"
        LOGAMUFFIN_BIN_NAME="logamuffin.exe"
        RUNNER_SPAWNER_BIN_NAME="runner-spawner.exe"
    else
        VERSIONER_BIN_NAME="versioner"
        LOGAMUFFIN_BIN_NAME="logamuffin"
        RUNNER_SPAWNER_BIN_NAME="runner-spawner"
    fi
fi
set +a

PROJECT_PLATFORM="${PROJECT_PLATFORM:-$_detected_platform}"
VERSIONER_BIN_NAME="${VERSIONER_BIN_NAME:-versioner}"
VERSIONER_BUNDLE_DIR="${VERSIONER_BUNDLE_DIR:-tools/versioner}"
LOGAMUFFIN_BIN_NAME="${LOGAMUFFIN_BIN_NAME:-logamuffin}"
LOGAMUFFIN_BUNDLE_DIR="${LOGAMUFFIN_BUNDLE_DIR:-tools/logamuffin}"
RUNNER_SPAWNER_BIN_NAME="${RUNNER_SPAWNER_BIN_NAME:-runner-spawner}"
RUNNER_SPAWNER_BUNDLE_DIR="${RUNNER_SPAWNER_BUNDLE_DIR:-tools/runner-spawner}"

# Resolve the newest bundled binary (single v*/ dir per update.bash invariant).
# A caller-provided non-empty VERSIONER_BIN/LOGAMUFFIN_BIN wins (this
# mirrors release.bash's `if [[ -z ... ]]; then . env.sh; fi` guard).
if [[ -z "${VERSIONER_BIN:-}" ]]; then
    VERSIONER_BIN=""
    _bundle_root="$_repo_root/$VERSIONER_BUNDLE_DIR"
    for _version_dir in "$_bundle_root"/v*/; do
        if [[ ! -d "$_version_dir" ]]; then
            continue
        fi
        _cand="$_version_dir$PROJECT_PLATFORM/$VERSIONER_BIN_NAME"
        if [[ -f "$_cand" ]]; then
            VERSIONER_BIN="$_cand"
        fi
    done
    if [[ -z "$VERSIONER_BIN" || ! -x "$VERSIONER_BIN" ]]; then
        if command -v versioner >/dev/null 2>&1; then
            VERSIONER_BIN=$(command -v versioner)
        else
            VERSIONER_BIN=""
        fi
    fi
fi

# Resolve the newest bundled logamuffin (single v*/ dir per update.bash).
if [[ -z "${LOGAMUFFIN_BIN:-}" ]]; then
    LOGAMUFFIN_BIN=""
    _bundle_root="$_repo_root/$LOGAMUFFIN_BUNDLE_DIR"
    for _version_dir in "$_bundle_root"/v*/; do
        if [[ ! -d "$_version_dir" ]]; then
            continue
        fi
        _cand="$_version_dir$PROJECT_PLATFORM/$LOGAMUFFIN_BIN_NAME"
        if [[ -f "$_cand" ]]; then
            LOGAMUFFIN_BIN="$_cand"
        fi
    done
    if [[ -z "$LOGAMUFFIN_BIN" || ! -x "$LOGAMUFFIN_BIN" ]]; then
        if command -v logamuffin >/dev/null 2>&1; then
            LOGAMUFFIN_BIN=$(command -v logamuffin)
        else
            LOGAMUFFIN_BIN=""
        fi
    fi
fi

# Resolve the newest bundled runner-spawner (single v*/ dir per update.bash).
if [[ -z "${RUNNER_SPAWNER_BIN:-}" ]]; then
    RUNNER_SPAWNER_BIN=""
    _bundle_root="$_repo_root/$RUNNER_SPAWNER_BUNDLE_DIR"
    for _version_dir in "$_bundle_root"/v*/; do
        if [[ ! -d "$_version_dir" ]]; then
            continue
        fi
        _cand="$_version_dir$PROJECT_PLATFORM/$RUNNER_SPAWNER_BIN_NAME"
        if [[ -f "$_cand" ]]; then
            RUNNER_SPAWNER_BIN="$_cand"
        fi
    done
    if [[ -z "$RUNNER_SPAWNER_BIN" || ! -x "$RUNNER_SPAWNER_BIN" ]]; then
        if command -v runner-spawner >/dev/null 2>&1; then
            RUNNER_SPAWNER_BIN=$(command -v runner-spawner)
        else
            RUNNER_SPAWNER_BIN=""
        fi
    fi
fi

export PROJECT_PLATFORM VERSIONER_BIN VERSIONER_BIN_NAME
export VERSIONER_REPO VERSIONER_TAG_PATTERN VERSIONER_CONFIG_FILE VERSIONER_BUNDLE_DIR
export LOGAMUFFIN_BIN LOGAMUFFIN_BIN_NAME LOGAMUFFIN_REPO LOGAMUFFIN_BUNDLE_DIR
export RUNNER_SPAWNER_BIN RUNNER_SPAWNER_BIN_NAME RUNNER_SPAWNER_REPO RUNNER_SPAWNER_BUNDLE_DIR

unset _env_sh_path _env_sh_dir _repo_root _host_os_raw _host_arch_raw
unset _host_os _host_arch _detected_platform _bundle_root _version_dir _cand

# Resolve the logamuffin binary at call time (not source time) so tests
# and scripts can point at a build output via LOGAMUFFIN_BIN or PATH.
# Never fails at source time.
_logamuffin_bin() {
    if [[ -n "${LOGAMUFFIN_BIN:-}" ]]; then
        printf '%s\n' "$LOGAMUFFIN_BIN"
        return 0
    fi
    if command -v logamuffin >/dev/null 2>&1; then
        command -v logamuffin
        return 0
    fi
    printf '%s\n' "logamuffin"
}

# log LEVEL [MESSAGE ...]
#   LEVEL may be a word (INFO) or a number (50). When a logamuffin
#   binary is available all arguments are forwarded to it with
#   LOG_SOURCE set from the call site; otherwise a printf fallback
#   keeps the same `log LEVEL MESSAGE` call shape working (see the
#   header for filtering/stream semantics). The exit code is
#   preserved (2 for usage errors, 0 otherwise for the fallback).
log() {
    local bin
    bin="$(_logamuffin_bin)"

    # Caller context -> LOG_SOURCE="file[:function]:line". BASH_SOURCE[1]
    # is the caller's file, BASH_LINENO[0] is the line that called
    # log(), and FUNCNAME[1] is the calling function (empty at top
    # level). Defaults keep `set -u` safe.
    local caller_file="${BASH_SOURCE[1]:-}"
    local caller_line="${BASH_LINENO[0]:-1}"
    local caller_func=""
    if ((${#FUNCNAME[@]} > 1)); then
        caller_func="${FUNCNAME[1]:-}"
    fi
    if [[ -z "$caller_file" ]]; then
        caller_file="${0:-log}"
    fi

    local src
    if [[ -n "$caller_func" && "$caller_func" != "log" && "$caller_func" != "main" ]]; then
        src="${caller_file}:${caller_func}:${caller_line}"
    else
        src="${caller_file}:${caller_line}"
    fi

    if [[ ! -x "$bin" ]]; then
        # No logamuffin binary: printf fallback (bootstrap / fresh clone).
        if [[ $# -eq 0 ]]; then
            printf '%s\n' "log: usage: log LEVEL MESSAGE..." >&2
            return 2
        fi
        local level_arg="${1:-}"
        shift
        local msg="$*"
        local level_upper level_value _thr_raw _thr_upper
        if [[ "$level_arg" =~ ^-?[0-9]+$ ]]; then
            level_value="$level_arg"
        else
            level_upper="$(printf '%s' "$level_arg" | tr '[:lower:]' '[:upper:]')"
            case "$level_upper" in
            TRACE) level_value=5 ;;
            DEBUG) level_value=10 ;;
            INFO) level_value=20 ;;
            WARNING | WARN) level_value=30 ;;
            ERROR) level_value=40 ;;
            CRITICAL | FATAL) level_value=50 ;;
            ALWAYS) level_value=-1 ;;
            *) level_value=20 ;;
            esac
        fi
        _thr_raw="${LOG_LEVEL:-${LOGAMUFFIN_LEVEL:-INFO}}"
        if [[ -z "$_thr_raw" ]]; then
            _thr_raw="INFO"
        fi
        if [[ "$_thr_raw" =~ ^-?[0-9]+$ ]]; then
            local threshold_value="$_thr_raw"
        else
            _thr_upper="$(printf '%s' "$_thr_raw" | tr '[:lower:]' '[:upper:]')"
            case "$_thr_upper" in
            TRACE) threshold_value=5 ;;
            DEBUG) threshold_value=10 ;;
            INFO) threshold_value=20 ;;
            WARNING | WARN) threshold_value=30 ;;
            ERROR) threshold_value=40 ;;
            CRITICAL | FATAL) threshold_value=50 ;;
            ALWAYS) threshold_value=-1 ;;
            *) threshold_value=20 ;;
            esac
        fi
        if [[ "$level_value" -ne -1 && "$level_value" -lt "$threshold_value" ]]; then
            return 0
        fi
        if [[ "$level_value" -ge 40 ]]; then
            printf '%s: [%s] %s\n' "$src" "$level_arg" "$msg" >&2
        else
            printf '%s: [%s] %s\n' "$src" "$level_arg" "$msg"
        fi
        return 0
    fi

    LOG_SOURCE="$src" "$bin" "$@"
    # Preserve `logamuffin`'s exit code (0 success/filtered,
    # 2 usage/config errors) even when `set -e` is active.
    return $?
}

# Sourced vs executed: when executed, print export statements and log().
if [[ -n "${BASH_SOURCE[0]:-}" && "${BASH_SOURCE[0]:-}" == "$0" ]]; then
    printf 'export PROJECT_PLATFORM=%q\n' "$PROJECT_PLATFORM"
    printf 'export VERSIONER_BIN=%q\n' "$VERSIONER_BIN"
    printf 'export VERSIONER_BIN_NAME=%q\n' "$VERSIONER_BIN_NAME"
    printf 'export VERSIONER_REPO=%q\n' "${VERSIONER_REPO:-}"
    printf 'export VERSIONER_TAG_PATTERN=%q\n' "${VERSIONER_TAG_PATTERN:-}"
    printf 'export VERSIONER_CONFIG_FILE=%q\n' "${VERSIONER_CONFIG_FILE:-}"
    printf 'export VERSIONER_BUNDLE_DIR=%q\n' "${VERSIONER_BUNDLE_DIR:-}"
    printf 'export LOGAMUFFIN_BIN=%q\n' "$LOGAMUFFIN_BIN"
    printf 'export LOGAMUFFIN_BIN_NAME=%q\n' "$LOGAMUFFIN_BIN_NAME"
    printf 'export LOGAMUFFIN_REPO=%q\n' "${LOGAMUFFIN_REPO:-}"
    printf 'export LOGAMUFFIN_BUNDLE_DIR=%q\n' "${LOGAMUFFIN_BUNDLE_DIR:-}"
    printf 'export RUNNER_SPAWNER_BIN=%q\n' "$RUNNER_SPAWNER_BIN"
    printf 'export RUNNER_SPAWNER_BIN_NAME=%q\n' "$RUNNER_SPAWNER_BIN_NAME"
    printf 'export RUNNER_SPAWNER_REPO=%q\n' "${RUNNER_SPAWNER_REPO:-}"
    printf 'export RUNNER_SPAWNER_BUNDLE_DIR=%q\n' "${RUNNER_SPAWNER_BUNDLE_DIR:-}"
    declare -f _logamuffin_bin log
fi
