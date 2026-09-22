#!/bin/bash
set -euo pipefail

# shen-check.sh — Gate 4 for this example: typecheck the spec via tc+.
#
# W6 collapsed three near-identical copies of this script into one
# implementation. The host resolution order, the flag form and the
# typed intrinsic prelude all live in `sb shen-check`
# (cmd/sb/shen_check.go); this file only says which spec to check.
#
# Run it by hand with a different spec: ./bin/shen-check.sh other.shen
# Point it at a specific host with: SHEN=/path/to/shen ./bin/shen-check.sh

SPEC="${1:-specs/core.shen}"
exec ../../bin/shen-check.sh "$SPEC"
