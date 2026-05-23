#!/bin/bash
# check-py.sh — Verify that py/main.py and py/guards_gen.py parse as
# Python via the stdlib `ast` module. There is no separate "type
# checker" gate here; mypy / pyright are out of scope. This is the
# equivalent of `go build` or `tsc --noEmit` for the Python leg.
set -euo pipefail

cd "$(dirname "$0")/.."

python3 -c '
import ast, sys
for path in ("py/guards_gen.py", "py/main.py"):
    try:
        ast.parse(open(path).read())
    except SyntaxError as exc:
        print(f"FAIL: {path}: {exc}", file=sys.stderr)
        sys.exit(1)
print("OK: py/guards_gen.py and py/main.py parse as Python")
'
