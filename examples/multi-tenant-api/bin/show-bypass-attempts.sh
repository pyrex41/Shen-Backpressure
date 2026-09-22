#!/usr/bin/env bash
# show-bypass-attempts.sh — render the forgery corpus as a Markdown
# table, suitable for pasting into demo.md or a blog post.
#
# This used to be the corpus runner: 150 lines of staging, building and
# a `case` statement that mapped each attempt's filename to a sentence
# about what catches it. That sentence was written by hand, which meant
# the table said what someone believed in the month they wrote it. W1
# changed the shape of every guard type and W3 added a gate, and the
# table kept reporting the old story until a human re-read it.
#
# The corpus is now a gate. Each `forgeries/*.go.bak` declares its own
# expected outcome in a header line, `sb forgery` stages it and runs
# the check that expectation names, and the Markdown below carries the
# *measured* outcome. The script is a thin wrapper so the two can never
# disagree: there is only one implementation.
#
#   ./bin/show-bypass-attempts.sh        # the table
#   sb forgery                           # the same run, as a gate
#   sb forgery -only 08                  # one entry
#
# The corpus also runs as gate "forgery" in sb.toml, so `sb gates`
# re-measures it on every iteration.

set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$DIR/.." && pwd)"

SB="${SB:-$ROOT/../../bin/sb}"
if [ ! -x "$SB" ]; then
    if command -v sb >/dev/null 2>&1; then
        SB="$(command -v sb)"
    else
        echo "ERROR: sb not found. Build it with 'make build-sb' at the repo root," >&2
        echo "       or set SB=/path/to/sb." >&2
        exit 1
    fi
fi

cd "$ROOT"
exec "$SB" forgery -markdown "$@"
