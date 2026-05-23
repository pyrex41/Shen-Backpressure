# `:runtime-via` — composing the compile-time gate with a live runtime check

> **Status**: prototype (W3.2). Go + TS emitters; Python + Rust not yet
> covered. The shen-web-tools demo is the only example that uses this
> today.

## TL;DR

A regular `verified` premise gets inlined into the generated
constructor — Shen lowers the predicate, shengen emits the
corresponding Go/TS expression, and the constructor refuses to build
a value that fails the check.

`:runtime-via <fname>` keeps the compile-time non-skippability
(the constructor is still the only path to a value of the type) but
swaps the *implementation* of the check for a named function the
caller supplies. The generated constructor calls `<fname>(ctx,
"<predicate-name>", [args…])` instead of inlining the predicate.

The compile-time gate is enforced by a witness declaration in the
generated module — `var _ runtimeChecker = <fname>` (Go) or `const _:
RuntimeChecker = <fname>` (TS). If the named function is absent or has
the wrong signature, the build fails. The runtime call is therefore
non-skippable: there is no path through the constructor that does not
consult the checker.

This document covers when to use the annotation, the trust-model
implications, the trade-offs vs OPA-style policy engines, and the
worked shen-web-tools example.

---

## When to use `:runtime-via`

Use the annotation when **the predicate's true meaning lives somewhere
shengen cannot lower**. Three concrete cases:

1. **Database-backed checks.** "This `tenant-id` actually exists in
   the tenants table" cannot be lowered statically. Today
   `examples/multi-tenant-api` handles this via `verified.Check*`
   wrappers in a separate package. `:runtime-via` is the more uniform
   alternative: annotate the spec premise, the generated constructor
   calls a named function, the wrapper layer disappears.

2. **Policy-engine evaluation.** "This action is permitted under the
   current org policy" wants to call out to OPA / Cedar / a Rego
   bundle. `:runtime-via opa-eval` (with `opa-eval` defined to do
   exactly that) gives you the OPA-behind-the-constructor pattern.

3. **Late-bound spec composition.** "The Shen runtime is online; ask
   it to evaluate this predicate against the current
   `(define …)`-blocks rather than the lowered Go translation." This
   is what the shen-web-tools demo does: the predicate
   `(> (length X) 0)` is lowered to Go/TS once, but the
   *evaluation* is delegated to a live SBCL host running the Shen
   tc+'d spec.

Conversely, **do not use it** when the predicate can be discharged
statically. Inlining is faster, easier to audit, and removes a
network round-trip from your hot path. `:runtime-via` should be a
last-resort tool, not the default.

---

## Syntax

The annotation is a trailing Shen block comment on the verified
premise's line:

```shen
(datatype query-text
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : query-text;)
```

The marker grammar:

- Open: `\*`
- Body: optional `:` then `runtime-via <name>` (one whitespace)
- Close: `*\`

Shen treats `\* ... *\` as a block comment, so its `tc+` ignores the
marker entirely. shengen reads the line, extracts the name, attaches
it to the matching verified premise, and emits the runtime-via
constructor variant.

The annotation is opt-in and backward-compatible: existing specs with
plain `: verified` continue to parse and emit exactly as before.

---

## Generated code shape

Two examples — one constrained (single-field wrapped) type, one
guarded (multi-field composite). The Go emission is shown; TS mirrors
it.

### Constrained type

Spec:

```shen
(datatype query-text
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : query-text;)
```

Generated Go:

```go
import (
    "context"
    "errors"
    "fmt"
)

// Compile-time witness — build fails if shenEval is missing or
// wrong-shaped. This is what makes the runtime call non-skippable.
type runtimeChecker func(ctx context.Context, predicate string, args ...any) (bool, error)
var _ runtimeChecker = shenEval

var errRuntimeCheckRejected = errors.New("runtime check rejected")

type QueryText struct{ v string }

func NewQueryText(ctx context.Context, x string) (QueryText, error) {
    ok, err := shenEval(ctx, "query-text", x)
    if err != nil {
        return QueryText{}, fmt.Errorf("shenEval rejected query-text: %w", err)
    }
    if !ok {
        return QueryText{}, fmt.Errorf("shenEval rejected query-text: %w", errRuntimeCheckRejected)
    }
    return QueryText{v: x}, nil
}

func (t QueryText) Val() string { return t.v }
```

Generated TS (slightly abbreviated):

```ts
import { shenEval } from "./runtime_checkers.js";

export interface RuntimeCheckCtx { signal?: AbortSignal; meta?: Record<string, unknown>; }
export type RuntimeChecker = (
  ctx: RuntimeCheckCtx, predicate: string, args: unknown[]
) => Promise<{ ok: boolean; error?: string }>;
const _runtimeChecker_shenEval: RuntimeChecker = shenEval;
void _runtimeChecker_shenEval;

export class QueryText {
  private readonly _v: string;
  private constructor(v: string) { this._v = v; }
  static async createOrThrow(ctx: RuntimeCheckCtx, x: string): Promise<QueryText> {
    const _check_shenEval = await shenEval(ctx, "query-text", [x]);
    if (!_check_shenEval.ok) {
      throw new Error(`shenEval rejected query-text: ${_check_shenEval.error ?? "runtime check failed"}`);
    }
    return new QueryText(x);
  }
  // tryCreate: same shape, returns QueryText | Error
  val(): string { return this._v; }
}
```

### Guarded composite

Spec:

```shen
(datatype tag-rule
  Token : string;
  UserId : string;
  (= Token UserId) : verified; \* :runtime-via policyCheck *\
  ===========================================
  [Token UserId] : tag-rule;)
```

Constructor:

```go
func NewTagRule(ctx context.Context, token string, userId string) (TagRule, error) {
    ok, err := policyCheck(ctx, "tag-rule", token, userId)
    ...
}
```

The positional args are the camelCased Shen variable names, in the
order they appear on the premise list. The predicate-name string
passed to the checker is the spec-side datatype block name.

---

## Trust model implications

`docs/TRUST-MODEL.md` defines the TCB as `Check*` wrappers + predicate
implementations + JWT parser + DB queries. `:runtime-via` adds one
more node to that TCB:

- **The runtime checker function** (`shenEval`, `policyCheck`, etc.)
  IS now part of the TCB. Every value of the annotated type was
  produced by a constructor that consulted this function; the
  semantics of the type depend on the function's correctness.

- **The wire format** between the constructor and the checker (the
  `predicate` string + `args` slice) is part of the TCB. A malformed
  envelope is a security-relevant bug.

- **The dispatch surface** behind the checker (e.g. the
  `*runtime-predicates*` table in shen-web-tools'
  `backend/shen-interop.lisp`) IS part of the TCB. It must be an
  explicit allowlist; the checker is not a remote eval gateway.

In exchange the TS/Go side of the type system gains a **new, stronger
invariant**: there is no path that produces a value of the annotated
type without the checker being consulted. The compile-time gate
makes this enforceable; the runtime call delivers the actual
semantics.

The relationship to the rest of `docs/TRUST-MODEL.md`:

- "What's structurally enforced": the constructor is still the only
  producer of the type. The witness declaration tightens this — the
  constructor itself is the only producer that compiles.
- "What's runtime-checked": shifts from "the predicate inlined into
  the constructor" to "the predicate inside the named checker." The
  *binding* (constructor → check) is still compile-time.
- "What's assumed": the checker is correct. This is the same
  assumption you make about every TCB component.

---

## Trade-offs vs OPA / Rego

OPA is a runtime policy engine with its own DSL (Rego), its own
bundle format, its own evaluation semantics, and its own ergonomics.
`:runtime-via` is a much thinner mechanism: it points at *whatever
function you supply*. If you supply an OPA client, you have OPA. If
you supply a Shen-runtime evaluator, you have a Shen-runtime
evaluator. If you supply a hand-rolled SQL query, you have a SQL
query.

|                            | OPA                          | `:runtime-via`              |
|----------------------------|------------------------------|-----------------------------|
| Policy language            | Rego                         | whatever the checker uses   |
| Policy distribution        | OPA bundles                  | whatever the checker uses   |
| Compile-time binding       | wrapper convention           | spec annotation + witness   |
| Non-skippability           | by convention                | structural (build fails)    |
| Mechanism complexity       | OPA agent, bundle pipeline   | one function pointer        |
| Vendor neutrality          | OPA-shaped                   | provider-agnostic           |

The HN comment from `se4u` proposed: "compile-time assertion that
handler code calls the runtime assertion, with OPA behind the
constructor." `:runtime-via opa-eval` is exactly that, with the
"compile-time assertion" half automated by shengen.

Use OPA when you want a policy DSL with its own ecosystem; use
`:runtime-via` when you want the compile-time non-skippability and
don't care which engine evaluates the predicate. They compose.

---

## Worked example: `examples/shen-web-tools/`

The shen-web-tools backend boots a live SBCL host that loads the Shen
language at startup and `tc+`'s `specs/core.shen` and
`specs/medicare.shen`. Pre-W3.2 this runtime was load-time only:
nothing in the hot path delegated to it.

W3.2 changes one verified premise to runtime-via:

```shen
(datatype query-text
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : query-text;)
```

The TS side gains `runtime/runtime_checkers.ts` with the `shenEval`
function. `runtime/guards_gen.ts` (regenerated by shengen-ts) imports
`shenEval` from that module, binds it to the
`RuntimeChecker`-typed witness, and the new
`QueryText.createOrThrow(ctx, x)` awaits a call to it before
constructing the value.

`shenEval` POSTs `{predicate: "query-text", args: [x]}` to
`/api/eval-predicate` on the SBCL backend. The backend
(`backend/server.lisp::api-eval-predicate`) dispatches via the
`*runtime-predicates*` allowlist defined in
`backend/shen-interop.lisp`. The `query-text` predicate is a CL
closure that, when the Shen runtime is loaded, asks Shen to evaluate
`(> (length "<x>") 0)` directly — the *same predicate the spec
declares*, evaluated by the *same runtime* that tc+'d the spec.

When Shen isn't available (CL-only mode, tests), the predicate falls
back to a CL transliteration so smoke tests still pass.

### End-to-end smoke procedure

The automated path (mocked fetcher) is covered by
`runtime/runtime_checkers.test.ts`. The live-backend path is a
manual smoke documented here:

```bash
# 1. Boot the SBCL backend (in one terminal).
cd examples/shen-web-tools
make run  # or: sbcl --script backend/load.lisp

# 2. In another terminal, exercise the endpoint.
curl -sX POST -H 'Content-Type: application/json' \
  -d '{"predicate":"query-text","args":["medicare advantage plans"]}' \
  http://localhost:8080/api/eval-predicate
# Expected: {"ok":true}

curl -sX POST -H 'Content-Type: application/json' \
  -d '{"predicate":"query-text","args":[""]}' \
  http://localhost:8080/api/eval-predicate
# Expected: {"ok":false}

# 3. Verify the backend logged the Shen predicate eval.
curl -sX GET 'http://localhost:8080/api/logs?since=0' | jq '.entries[] | select(.cat == "runtime-via")'
```

### Build-fail-on-missing-checker procedure

A separate manual smoke that verifies the compile-time witness is
doing its job:

```bash
# 1. Delete the runtime_checkers.ts file.
mv examples/shen-web-tools/runtime/runtime_checkers.ts /tmp/

# 2. Try to type-check the project.
cd examples/shen-web-tools
npx tsc --noEmit -p .
# Expected: error TS2307: Cannot find module './runtime_checkers.js'
#           in runtime/guards_gen.ts

# 3. Restore.
mv /tmp/runtime_checkers.ts examples/shen-web-tools/runtime/
```

The runtime call is non-skippable because the constructor is the only
path through the type AND the constructor cannot be compiled without
the checker module. Both halves of the gate are enforced by `tsc`.

For the Go side the same shape applies, enforced by `go build` — the
generated `var _ runtimeChecker = shenEval` declaration won't compile
without a matching function in the same package.

---

## Discharge report

The discharge report's `tools.shen_runtime` field is populated when
`sb derive` detects a runtime-via annotation in any spec referenced
by `[[derive.specs]]`:

```json
{
  "tools": {
    "sb_version": "0.3.0",
    "shen_runtime": "shen-sbcl",
    "shen_runtime_available": true,
    ...
  }
}
```

This is additive — projects with no runtime-via continue to emit
`shen_runtime: null`, the historical default. See
`thoughts/shared/research/2026-05-22-schema-v1-additions.md` (W3.2
addendum).

---

## Limitations + future work

- **TS constructors become async.** This is a real ergonomic cost. A
  caller that previously did `const qt = QueryText.createOrThrow(x)`
  now does `const qt = await QueryText.createOrThrow(ctx, x)`. There
  is no sync escape hatch by design — letting one would defeat the
  non-skippability.

- **No Python / Rust support.** The annotation parses uniformly via
  the Shen-comment marker, but the Python and Rust emitters
  (`cmd/shengen-py/`, `cmd/shengen-rs/`) don't yet emit the
  runtime-via constructor variants. Their emission paths skip
  unrecognised annotations. Closing this gap is straightforward and
  left for follow-up.

- **The wire format is hardcoded.** All runtime-via checkers share
  the `{predicate, args} → {ok, error?}` envelope. A future version
  could let the spec name the wire format (`:runtime-via opa://`,
  `:runtime-via sql://`, …) and dispatch on the URL scheme.

- **One runtime per project.** `detectShenRuntime` returns a single
  string. When a project mixes runtime hosts (some predicates against
  SBCL, some against a Python policy engine), a richer scheme is
  needed.

- **No spec-level type checking of the checker.** Shen's tc+ doesn't
  know the checker exists. If you rename `shenEval` in the spec but
  not in `runtime_checkers.ts`, the build fails at compile time
  (witness mismatch), not at spec time. That's fine for a prototype
  but would deserve better diagnostics in a hardened version.

---

## References

- `cmd/shengen/main.go::extractRuntimeViaComment` — parser
- `cmd/shengen/main.go::emitRuntimeViaCheckGo` — Go emitter
- `cmd/shengen-ts/shengen.ts::emitRuntimeViaCheckTs` — TS emitter
- `examples/shen-web-tools/specs/core.shen` — annotated spec
- `examples/shen-web-tools/runtime/runtime_checkers.ts` — checker
- `examples/shen-web-tools/backend/shen-interop.lisp` — dispatch table
- `examples/shen-web-tools/backend/server.lisp::api-eval-predicate` — endpoint
- `docs/TRUST-MODEL.md` — TCB definition (this doc extends it)
- `thoughts/shared/research/2026-05-22-hn-feedback-next-steps.md` §R4 —
  HN-derived motivation
- `thoughts/shared/research/2026-05-22-schema-v1-additions.md` (W3.2
  addendum) — discharge-report field reuse
