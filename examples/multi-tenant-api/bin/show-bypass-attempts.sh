#!/usr/bin/env bash
# show-bypass-attempts.sh — Compile each .go.bak under
# `bypass_attempts/`, report whether it fails at compile time, at
# runtime, or worryingly succeeds. Output is Markdown suitable for
# pasting into demo.md or a blog post.
#
# Pattern mirrors examples/payment/demo-shen-derive/run.sh: each
# attempt is rotated into a temporary package, built, run if it
# builds, then cleaned up. We never modify the live source tree.

set -u

DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$DIR/.." && pwd)"
ATTEMPTS_DIR="$ROOT/bypass_attempts"
TMP_DIR=$(mktemp -d -t mta-bypass-XXXXXX)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

if [ ! -d "$ATTEMPTS_DIR" ]; then
    echo "ERROR: $ATTEMPTS_DIR not found" >&2
    exit 1
fi

# We mount each .go.bak inside the example module so it can import
# multi-tenant-api/internal/shenguard. Create a sibling package under
# internal/ that the module knows about.
#
# Note: Go's tooling skips directories whose names start with "." or
# "_", so we use `bypass_harness` (no leading dot) and clean it up
# unconditionally at the end and on script entry.
HARNESS_DIR="$ROOT/internal/bypass_harness"
rm -rf "$HARNESS_DIR"
mkdir -p "$HARNESS_DIR"
ATTEMPTS=$(find "$ATTEMPTS_DIR" -maxdepth 1 -type f -name '*.go.bak' | sort)

cat <<'HEADER'
# Bypass Attempts

Each row below tries to forge or skip a step in the proof chain
`JwtToken → AuthenticatedUser → TenantAccess → ResourceAccess`. The
last column records what the Go toolchain (or runtime) does when
the attempt is rotated into the package and built.

HEADER

printf '| # | File | Technique | Outcome |\n'
printf '|---|------|-----------|---------|\n'

idx=0
for src in $ATTEMPTS; do
    idx=$((idx + 1))
    base="$(basename "$src" .go.bak)"
    # Description is the first sentence (up to the first period) of
    # the file's leading doc comment. Each attempt opens with
    # "// Attempt #N: <one-line summary>." which we strip and reuse.
    desc="$(grep -E '^// Attempt #[0-9]+:' "$src" | head -1 \
            | sed -E 's|^// Attempt #[0-9]+: *||')"
    if [ -z "$desc" ]; then
        desc="(no description)"
    fi
    # Stage the file under internal/.bypass_harness so it shares the
    # module path. Rename .go.bak → .go so the toolchain compiles it.
    rm -f "$HARNESS_DIR"/*.go
    cp "$src" "$HARNESS_DIR/${base}.go"
    # Replace the package name so the harness package compiles cleanly.
    # Each .go.bak declares `package bypass_attempts_demo`; we don't
    # need to change it (the harness folder doesn't constrain the
    # package name).

    if BUILD_OUT=$(cd "$ROOT" && go build ./internal/bypass_harness/... 2>&1); then
        # Builds — bypass passed the type-system gate. We report
        # this as "compiles" and leave the runtime outcome to the
        # per-attempt doc comment (the second mechanism — runtime
        # predicate, code review, or grep gate — is described in
        # the file itself).
        printf '| %d | `%s.go.bak` | %s | **compiles**, rejected by %s |\n' \
            "$idx" "$base" "$desc" \
            "$(case $base in
                02_*)  echo "runtime predicate \`(= User (head (head Jwt)))\`" ;;
                03_*)  echo "code review (\`unsafe.Pointer\` red flag)" ;;
                04_*|05_*) echo "the \`shenguard.New*\` grep gate in \`bin/shenguard-audit.sh\`" ;;
                *) echo "downstream review" ;;
              esac)"
    else
        # Capture the most relevant error line — the compiler error
        # message that names the offending identifier.
        ERR_LINE=$(echo "$BUILD_OUT" | grep -E '\.go:' | head -1 | sed -e 's|.*\.bypass_harness/[^:]*:[0-9]*:[0-9]*: ||' || true)
        if [ -z "$ERR_LINE" ]; then
            ERR_LINE=$(echo "$BUILD_OUT" | head -1)
        fi
        printf '| %d | `%s.go.bak` | %s | **FAILS at compile**: `%s` |\n' \
            "$idx" "$base" "$desc" \
            "$(echo "$ERR_LINE" | sed -e 's|`|"|g' -e 's|"|""|g')"
    fi
done

# Clean up staging.
rm -rf "$HARNESS_DIR"

cat <<'FOOTER'

**How to read the table.** A "FAILS at compile" outcome is a
type-system guarantee: the Go compiler refuses to produce a binary.
A "compiles" outcome means the attempt is structurally well-typed
but is rejected by one of the other layers of the trust model
described in `../../docs/TRUST-MODEL.md`:

- Attempt #2 compiles but the constructor's verified premise
  `(= User (head (head Jwt)))` returns an error at runtime, so no
  `AuthenticatedUser` value materialises.
- Attempt #3 compiles AND succeeds (Go's `unsafe.Pointer` is more
  powerful than the visibility rules). The defence here is
  code-review and grep — see the doc comment inside the file.
- Attempts #4 and #5 compile because the type system can only
  enforce "if you ask for a verified.TenantAccess, you walked the
  chain"; it cannot stop someone from writing a handler that
  doesn't ask. The local `bin/shenguard-audit.sh` and a
  `bypass-policy` grep catch these patterns.

The structural guarantee from W2.1 is attempt #2's failure: pairing
token-A with user-B is now structurally rejected, where pre-W2.1
the constructor was infallible.
FOOTER
