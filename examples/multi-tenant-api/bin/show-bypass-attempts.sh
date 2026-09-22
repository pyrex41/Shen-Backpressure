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

This example is generated with `--brands` (W1), so the guard types
carry a phantom brand parameter and an unexported witness field.
Attempts #1, #2, #3 and #5 name a brand — the price of admission,
not a bypass — so each keeps demonstrating its own original
mechanism. Attempts #6 and #7 are written the way they were before
W1, deliberately: they compiled then, and the compiler rejects them
now.

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
                04_*)  echo "the \`flow\` gate's \`must-pass-through\` premise (it reaches the DB sink with no proof on the path)" ;;
                05_*)  echo "the raw-constructor discipline: \`flow\` gate \`constructor-only\`, grep fallback in \`bin/shenguard-audit.sh\`" ;;
                08_*)  echo "the \`flow\` gate's \`constructor-only\` premise — and NOT by the grep, which the alias evades" ;;
                06_*)  echo "the W1 witness panic (\`mustBeMinted\`)" ;;
                07_*)  echo "nothing — this is a real forgery, and the reason W1 exists" ;;
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
  doesn't ask. Since W3 these are caught by the `flow` gate rather
  than by review: attempt #4 by `must-pass-through`, which finds a
  call path from the handler to `DB#Query` with no reference to
  `verified.CheckTenantAccess` on it, and attempt #5 by
  `constructor-only`.
- Attempt #8 is the one that separates the two kinds of gate. It
  calls the raw constructor through an aliased import, which the
  grep in `bin/shenguard-audit.sh` cannot see and the resolved
  symbol graph resolves to the same symbol. Run
  `sb flow` with a SCIP indexer on PATH to see it caught, and
  `./bin/shenguard-audit.sh --grep-only` to see it missed.

The structural guarantee from W2.1 is attempt #2's failure: pairing
token-A with user-B is now structurally rejected, where pre-W2.1
the constructor was infallible.

The structural guarantees from W1 are attempts #6 and #7:

- Attempt #6 (empty literal) compiled and succeeded before W1:
  `shenguard.TenantAccess{}` names no field, so Go's visibility
  rules never objected, and the result was a proof no constructor
  had checked. It now fails to compile, because the type needs a
  brand argument; and if an attacker supplies one, the value panics
  on first read — the generated struct's first field is an
  unexported `valid witness` and every accessor calls
  `mustBeMinted`.
- Attempt #7 (unpaired proof) compiled and succeeded before W1:
  two honest chains, crossed, with no premise relating them. It now
  fails to compile. The brand inference read the pairing out of the
  spec's existing sharing structure — nobody wrote a new premise.

What W1 does not do: brands are named by the caller, so building
both chains at one brand gets a pair the compiler accepts again.
Brands make keeping chains apart the default and crossing them an
explicit, greppable act. Go has no existential types, so a
constructor cannot mint a brand its caller is unable to name. The
witness panic is a runtime member of the TCB. Both limits are
stated in `../../docs/TRUST-MODEL.md`.
FOOTER
