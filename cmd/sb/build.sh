#!/bin/bash
set -euo pipefail

# Build sb CLI with embedded skill/command data.
# Copies from sb/ (source of truth) into skilldata/ for go:embed.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Clean and recreate skilldata
rm -rf "$SCRIPT_DIR/skilldata"
mkdir -p "$SCRIPT_DIR/skilldata/commands" "$SCRIPT_DIR/skilldata/skills/shen-backpressure"

# Copy skill bundle
cp "$REPO_ROOT/sb/skills/shen-backpressure/SKILL.md" "$SCRIPT_DIR/skilldata/skills/shen-backpressure/"
cp "$REPO_ROOT/sb/AGENT_PROMPT.md" "$SCRIPT_DIR/skilldata/"

# Copy commands
for f in "$REPO_ROOT/sb/commands/"*.md; do
    cp "$f" "$SCRIPT_DIR/skilldata/commands/"
done

echo "Copied skill data from sb/ into skilldata/"

# Build
cd "$SCRIPT_DIR"
go build -o "$REPO_ROOT/bin/sb" .
echo "Built bin/sb"
