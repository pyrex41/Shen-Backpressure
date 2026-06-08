#!/usr/bin/env bash
# common.sh — shared scaffolding for the runtime-policy-tier demos.
#
# SOURCE this file from a demo script; do NOT execute it directly.
#   source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
#
# This library deliberately does NOT set `set -euo pipefail` — that is the
# demo script's responsibility, so that sourcing it never changes the shell's
# error-handling behavior unexpectedly.

# ---------------------------------------------------------------------------
# Paths (hardcoded absolute — simplest and reliable)
# ---------------------------------------------------------------------------
REPO_ROOT="/Users/reuben/projects/Shen-Backpressure"
EXAMPLE_DIR="/Users/reuben/projects/Shen-Backpressure/examples/multi-tenant-api"
export REPO_ROOT EXAMPLE_DIR

# All demo commands run from the example root.
cd "$EXAMPLE_DIR" || {
  echo "common.sh: cannot cd to EXAMPLE_DIR=$EXAMPLE_DIR" >&2
  return 1 2>/dev/null || exit 1
}

# ---------------------------------------------------------------------------
# sb() — run the sb CLI from the example dir against the repo's cmd/sb module.
# The example is a separate Go module, so `go run ../../cmd/sb` fails; invoking
# with the absolute path to cmd/sb runs it as its own module.
# ---------------------------------------------------------------------------
sb() {
  ( cd "$EXAMPLE_DIR" && go run "$REPO_ROOT/cmd/sb" "$@" )
}

# ---------------------------------------------------------------------------
# Color setup — auto-disable when not a tty or NO_COLOR is set.
# ---------------------------------------------------------------------------
if [ -n "${NO_COLOR:-}" ] || [ ! -t 1 ]; then
  _C_RESET=""; _C_BOLD=""; _C_DIM=""; _C_RED=""; _C_GREEN=""
  _C_YELLOW=""; _C_BLUE=""; _C_CYAN=""
else
  _C_RESET=$'\033[0m'
  _C_BOLD=$'\033[1m'
  _C_DIM=$'\033[2m'
  _C_RED=$'\033[31m'
  _C_GREEN=$'\033[32m'
  _C_YELLOW=$'\033[33m'
  _C_BLUE=$'\033[34m'
  _C_CYAN=$'\033[36m'
fi

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------

# section "Title" — bold banner line with a rule above and below.
section() {
  local title="$*"
  printf '\n%s%s===============================================================%s\n' \
    "$_C_BOLD" "$_C_CYAN" "$_C_RESET"
  printf '%s%s== %s%s\n' "$_C_BOLD" "$_C_CYAN" "$title" "$_C_RESET"
  printf '%s%s===============================================================%s\n' \
    "$_C_BOLD" "$_C_CYAN" "$_C_RESET"
}

# step "msg" — an arrow step line (blue).
step() {
  printf '%s%s==>%s %s\n' "$_C_BOLD" "$_C_BLUE" "$_C_RESET" "$*"
}

# note "msg" — dim explanatory text.
note() {
  printf '%s    %s%s\n' "$_C_DIM" "$*" "$_C_RESET"
}

# good "msg" — green check line.
good() {
  printf '%s  ✓ %s%s\n' "$_C_GREEN" "$*" "$_C_RESET"
}

# bad "msg" — red x line.
bad() {
  printf '%s  ✗ %s%s\n' "$_C_RED" "$*" "$_C_RESET"
}

# show_file_lines "path" [grep-pattern ...] — print a file, optionally only the
# lines matching any of the supplied grep patterns (with a little context).
# Simple by design; builders may also just use sed/grep directly.
show_file_lines() {
  local path="$1"; shift
  if [ ! -f "$path" ]; then
    bad "show_file_lines: no such file: $path"
    return 1
  fi
  printf '%s--- %s ---%s\n' "$_C_DIM" "$path" "$_C_RESET"
  if [ "$#" -eq 0 ]; then
    cat "$path"
  else
    local pat
    for pat in "$@"; do
      grep -n -A2 -B0 -- "$pat" "$path" 2>/dev/null
    done
  fi
}

# ---------------------------------------------------------------------------
# pause — wait for the operator when stepping through interactively.
# No-op unless DEMO_PAUSE is set AND stdin is a tty.
# ---------------------------------------------------------------------------
pause() {
  if [ -n "${DEMO_PAUSE:-}" ] && [ -t 0 ]; then
    printf '%s    (press enter to continue)%s ' "$_C_DIM" "$_C_RESET"
    read -r _ || true
  fi
}

# ---------------------------------------------------------------------------
# Snapshot / restore — safe, reversible demos.
#
# SNAP_DIR is a per-run temp dir holding copies of snapshotted files. Each
# snapshot remembers the file's ORIGINAL absolute path so it can be restored
# whether it lives inside or outside EXAMPLE_DIR.
# ---------------------------------------------------------------------------
SNAP_DIR=""
# Parallel arrays: _SNAP_ORIG[i] is the original abs path, _SNAP_COPY[i] its backup.
_SNAP_ORIG=()
_SNAP_COPY=()
_SNAP_COUNT=0

_ensure_snap_dir() {
  if [ -z "$SNAP_DIR" ] || [ ! -d "$SNAP_DIR" ]; then
    SNAP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/demo-snap.XXXXXX")"
    export SNAP_DIR
  fi
}

# _abspath <path> — resolve to an absolute path without requiring the file to
# already be canonicalizable (works for existing files).
_abspath() {
  local p="$1"
  case "$p" in
    /*) printf '%s\n' "$p" ;;
    *)  printf '%s/%s\n' "$(pwd)" "$p" ;;
  esac
}

# snapshot <file>... — back up each file into $SNAP_DIR, remembering its origin.
snapshot() {
  _ensure_snap_dir
  local f orig copy
  for f in "$@"; do
    orig="$(_abspath "$f")"
    if [ ! -f "$orig" ]; then
      bad "snapshot: no such file: $orig"
      return 1
    fi
    copy="$SNAP_DIR/snap-$_SNAP_COUNT.bak"
    cp -p "$orig" "$copy"
    _SNAP_ORIG[$_SNAP_COUNT]="$orig"
    _SNAP_COPY[$_SNAP_COUNT]="$copy"
    _SNAP_COUNT=$((_SNAP_COUNT + 1))
    note "snapshotted $orig"
  done
}

# restore_snapshots — copy every backup back to its original path. Idempotent:
# safe to call multiple times (e.g. explicitly and again from the EXIT trap).
restore_snapshots() {
  local i orig copy n=0
  i=0
  while [ "$i" -lt "$_SNAP_COUNT" ]; do
    orig="${_SNAP_ORIG[$i]}"
    copy="${_SNAP_COPY[$i]}"
    if [ -n "$copy" ] && [ -f "$copy" ]; then
      cp -p "$copy" "$orig"
      n=$((n + 1))
    fi
    i=$((i + 1))
  done
  if [ "$n" -gt 0 ]; then
    note "restored $n snapshotted file(s)"
  fi
}

# arm_restore — install a trap so snapshots are restored on EXIT / INT / TERM,
# even if the demo errors out or is interrupted with ctrl-c.
arm_restore() {
  trap restore_snapshots EXIT INT TERM
}
