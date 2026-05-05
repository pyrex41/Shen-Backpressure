#!/bin/bash
set -euo pipefail

# Build sb CLI with embedded skill/command data.
#
# Skill bundle dedup pattern:
#   sb/                       canonical source of truth (edited directly)
#   cmd/sb/skilldata/         checked-in mirror, rebuilt from sb/ before
#                             every binary build (here, and via
#                             `make sync-skilldata`)
#
# The mirror is kept under git so a fresh clone embeds the right bundle
# without first running `make`. Drift between the two is caught by
# `make check-skilldata` in CI; the canonical fix is always to edit sb/
# and re-run sync.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Clean and recreate skilldata as a structural mirror of sb/.
# `cp -R sb/. skilldata/` propagates new files, new directories, and
# non-.md assets without per-file enumeration; if sb/ ever grows a
# second skill or a HELPERS.md, the mirror tracks automatically.
rm -rf "$SCRIPT_DIR/skilldata"
mkdir -p "$SCRIPT_DIR/skilldata"
cp -R "$REPO_ROOT/sb/." "$SCRIPT_DIR/skilldata/"

echo "Copied skill data from sb/ into skilldata/"

# Build
cd "$SCRIPT_DIR"
go build -o "$REPO_ROOT/bin/sb" .
echo "Built bin/sb"
