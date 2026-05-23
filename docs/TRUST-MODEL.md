# Trust Model

> Read this if you're a security reviewer, a compliance auditor, or
> anyone trying to figure out what Shen-Backpressure does and does not
> claim. The goal here is honesty about where trust lives, not
> marketing.

## Table of Contents

- [The one-sentence version](#the-one-sentence-version)
- [What's structurally enforced](#whats-structurally-enforced)
- [What's runtime-checked](#whats-runtime-checked)
- [What's sampled](#whats-sampled)
- [What's assumed (the TCB)](#whats-assumed-the-tcb)
- [What this project does NOT claim](#what-this-project-does-not-claim)
- [When to use this](#when-to-use-this)
- [Glossary: discharge categories](#glossary-discharge-categories)
- [Worked example: the multi-tenant JWT chain](#worked-example-the-multi-tenant-jwt-chain)
- [Reviewer workflow](#reviewer-workflow)

## The one-sentence version

Shen-Backpressure moves a small, well-defined set of domain
invariants from "tests we remembered to write" into "things the
target-language compiler refuses to build without"; everything else
— the predicate implementations, the I/O wrappers, the parsers, the
DB queries — is still ordinary code you have to read.

The project's value is precisely that this "small, well-defined set"
is named explicitly. The rest of this document does that naming.

## What's structurally enforced

These are the guarantees the target-language compiler (Go's static
type checker, TypeScript's `tsc`, etc.) makes for every well-typed
value, on every input, without you having to write a test.

For each generated guard type, the compiler enforces three things:

1. **Unforgeable construction.** The fields of the generated struct
   are unexported. A caller in another package literally cannot write
   `shenguard.TenantAccess{principal: p, tenant: t, isMember: true}`
   — the field names are not in scope. The only way to get a
   `TenantAccess` is to call `shenguard.NewTenantAccess(...)`.

2. **Validating constructor.** `NewTenantAccess` runs every
   `verified` predicate the Shen spec declares and returns
   `(Zero, error)` on the first failure. A "successfully constructed"
   value is, by construction, a value for which every spec premise
   held at the point of construction.

3. **Type-level chaining.** A spec rule whose premise is itself a
   guard type — for example `tenant-access` requiring an
   `authenticated-principal` — appears in the generated constructor
   as a parameter of the upstream guard type. The compiler refuses
   to call `NewResourceAccess(rawTenantString, ...)` because the
   argument type is `TenantAccess`, not `string`.

The combined effect: **a handler that demands a `ResourceAccess`
parameter cannot be reached without the caller having walked the
whole proof chain.** The proof chain is the type signature, not a
runtime check.

What "structural" means in the discharge report:

- `discharge: "static"` with basis
  `"guard-type-at-boundary"` — the premise is satisfied because the
  parameter type is the upstream guard type. (Example: in
  `tenant-access`, the premise that `Principal` is authenticated is
  static because `Principal` has type `AuthenticatedPrincipal`.)
- `discharge: "static"` with basis
  `"guard-constructor-validates"` — the premise is satisfied because
  the constructor checked the predicate at construction time, so
  every value of the guard type observably satisfies it.

Both bases are static in the sense that no runtime check happens
**at the boundary** — the compiler refuses to type-check a call site
that would violate them.

**Where this happens in code:**

- Generated constructors live in `internal/shenguard/guards_gen.go`
  (Go) and `runtime/guards_gen.ts` (TypeScript). See, for example,
  `examples/payment/internal/shenguard/guards_gen.go:26-33` for a
  validating constructor and
  `examples/shen-web-tools/runtime/guards_gen.ts:17-29` for the TS
  equivalent.

## What's runtime-checked

The validating constructor is itself a runtime check — it just
happens at construction time rather than at every call site, and
its execution path is unforgeable from outside the package.

Two things are worth pulling apart here:

- **Predicate evaluation runs at runtime.** When you call
  `NewAmount(-5)`, the body of `NewAmount` evaluates `-5 >= 0`,
  observes `false`, and returns an error. That is not a compile-time
  proof. The compile-time guarantee is that **no other code path
  produces an `Amount`** — only this constructor does. So every
  `Amount` value that exists in your program was produced by a
  predicate that returned true.

- **Cross-field predicates are also runtime, in the same sense.**
  `examples/shen-web-tools/runtime/guards_gen.ts:253-255` enforces
  the relational invariant `page.url() === hit.url()` inside
  `GroundedSource.createOrThrow`. Same story: runs at construction
  time, unforgeable from outside the package.

**Where the runtime check is not the binding.** Two patterns in
the demos are not yet maximally tight:

1. `examples/multi-tenant-api/internal/shenguard/guards_gen.go:97`
   — `NewAuthenticatedUser(token JwtToken, expiry TokenExpiry, user
   UserId) AuthenticatedUser` is **infallible**. It does not
   structurally bind `user` to the `sub` claim inside `token`. The
   middleware at
   `examples/multi-tenant-api/internal/auth/middleware.go:35-51`
   wires `result.Claims.Sub` into the `UserId` correctly, but that
   binding is convention, not type. A caller in possession of a
   `JwtToken` for Alice and a `UserId` for Bob can pair them. Wave-2
   of the follow-up plan tightens this; today, the spec premise
   `(= User (sub Claims))` does not exist.

2. `examples/multi-tenant-api/internal/auth/tenant.go:15` —
   `CheckTenantAccess(db, principal, userID string, tenantID)` takes
   `userID` as a separate parameter rather than extracting it from
   `principal`. Handlers thread the right value through
   (`handlers.go:84`), but this is again convention, not type.

We list these explicitly because the trust model is meaningless
without them. Read [What's assumed](#whats-assumed-the-tcb) for the
generalization.

## What's sampled

Some pieces of behavior do not lower to a structural guarantee. The
canonical case is a pure function whose spec is clear but whose
efficient implementation is not (think: the balance fold in
`examples/payment/specs/core.shen`).

For these, `shen-derive` generates a table-driven test that
evaluates the Shen `(define …)` block as an oracle on a
deterministic boundary pool (plus optional seeded random draws) and
asserts pointwise equality with the hand-written implementation.

- `discharge: "runtime-sample"` in the report means **every sampled
  input agreed**. The committed test file is the drift gate: change
  the spec, the impl, or the sampler without regenerating, and the
  gate fails.
- This is **sampled evidence, not an exhaustive proof.** A bug that
  only appears on inputs outside the boundary pool can slip past
  this gate. The boundary pool is designed to hit the obvious edges
  (`0`, `1`, `-1`, integer overflow boundaries, empty list, etc.);
  there is no claim that it covers every adversarial input.
- The pool composition is deterministic and inspectable. The
  generated test file is checked into the repo. A reviewer can read
  the cases and decide whether the pool is adequate for their
  threat model.

The schema reserves a third category, `runtime-assertion`, for a
future "the predicate is checked at every call site by a named
runtime function." That category is not emitted in this release.

## What's assumed (the TCB)

The **Trusted Computing Base** for a Shen-Backpressure-protected
codebase is everything the structural guarantees rest on. If any
of these is wrong, the structural guarantee is also wrong. We
enumerate them so a reviewer can audit each one:

### 1. The `Check*` wrappers

For premises that the spec declares but cannot be proven without
I/O (database lookups, network calls, cryptographic verification),
the spec declares a `verified` premise like `(= IsMember true)` and
expects the caller to supply `isMember` after running the check.
The actual check lives in a small set of **wrapper functions** —
the canonical name is `CheckTenantAccess`, `CheckResourceAccess`,
etc.

**These wrappers are the TCB for any I/O-backed premise.** If
`CheckTenantAccess` (`examples/multi-tenant-api/internal/auth/tenant.go:15-34`)
is buggy — wrong SQL query, missed permission edge case, race
window — the structural chain on top of it is still well-typed but
the conclusion `"this principal is in this tenant"` is wrong.

A reviewer's first job is to read these wrappers. They are the
small, well-defined surface that the rest of the proof chain
trusts.

The "package-private constructor + only-exported `Check*` wrapper"
hardening pattern (proposed by the author in HN comments) makes
this surface even smaller: the raw constructor `newTenantAccess`
becomes unexported, leaving `CheckTenantAccess` as the only exit
from the `shenguard` package. Today the multi-tenant demo exports
the raw constructor; Wave-2 of the HN-follow-up plan changes that.

### 2. The predicate implementations themselves

A Shen spec like `(> Exp Now) : verified` compiles to a Go
comparison. If the lowering is wrong, every value of the guarded
type is suspect. Concretely: shengen translates a Shen `>` to a Go
`>`. This translation is straightforward but it is part of the TCB.

`tcb-audit` (`bin/shenguard-audit.sh`) is the gate that re-runs
shengen and diffs against the committed `guards_gen.*` files; if
anyone hand-edits a generated guard, the gate fails. That gate
catches a specific class of tampering (post-generation edits); it
does not verify that the shengen lowering itself is correct. The
shengen test suite (`cmd/shengen/main_test.go` and
`cmd/shengen-ts/shengen.test.ts`) covers the lowering.

### 3. The JWT parser (and any other parser at a trust boundary)

`examples/multi-tenant-api/internal/auth/jwt.go:52-82` is the
hand-written JWT parser. It performs HMAC-SHA256 verification with
`crypto/hmac.Equal` (constant time), then JSON-decodes the claims,
then checks expiry. The structural chain downstream
(`JwtToken → AuthenticatedUser → TenantAccess → ResourceAccess`)
rests on this parser being correct.

If `Parse` accepted a tampered payload — and the test
`TestParseTamperedPayload` in `internal/auth/jwt_test.go` exists
because this is the obvious failure mode — the chain is well-typed
but the input it's validating is a lie. The parser is in the TCB.

### 4. The DB queries

`CheckTenantAccess` runs `SELECT COUNT(*) FROM tenant_memberships
WHERE user_id = ? AND tenant_id = ?`. If the SQL is wrong (the
classic case: `OR user_id IS NULL` accidentally introduced by a
schema change), the chain reports membership where there isn't any.
The SQL queries are in the TCB.

### 5. The Shen runtime (when present)

In `examples/shen-web-tools/`, the SBCL backend loads a live Shen
runtime at boot
(`examples/shen-web-tools/backend/shen-interop.lisp:23-50`) and
type-checks the spec files with `tc +`. If the Shen runtime mis-checks
a spec, every gate that depends on that spec is suspect. We rely on
shen-sbcl being a correct Shen implementation; that's an external
dependency we don't verify.

For the Go and TypeScript demos, the Shen runtime only runs at
build time (Gate 4: `shen tc+`) and is not in the runtime TCB.

### 6. The shengen emitter itself

shengen is the program that translates `.shen` specs into target
language code. It is part of the TCB. It is a single static
Go/TypeScript program (no plugins, no eval), and its output is
diffed by `tcb-audit`, so a reviewer can audit the output without
auditing the emitter — but the relationship "input spec ⇒ correct
output code" rests on the emitter being right.

### 7. Your build pipeline

If a CI step runs `sb gen` and uploads the result without first
running `tcb-audit`, the published binary may contain
hand-modified guard types. The gate ordering in `sb.toml` matters
— `sb gates` runs gates in declared order and stops at the first
failure. Adversarial CI configurations are out of scope of this
trust model.

## What this project does NOT claim

We list these because each one has come up in real conversations:

- **Not a SOC-2, ISO-27001, or HIPAA certification.** The discharge
  report is the kind of artifact a compliance workflow can use as
  evidence; it is not itself a certification.
- **Not signed or attested.** The `signature` field in
  `discharge_report.json` is reserved for a future signing
  integration; in this release it is always `null`. There is no
  cryptographic claim that the report came from an unaltered build.
- **Not third-party verified.** The classifications come from this
  tool's analysis of your spec and your implementation. We did not
  hire Trail of Bits.
- **Not a proof of memory safety, side-channel resistance, or
  cryptographic correctness.** Use mature libraries (`crypto/hmac`,
  `golang.org/x/crypto`, etc.) for those; Shen-Backpressure does
  not replace them.
- **Not a substitute for tests.** The structural guarantees cover
  invariants you can name in Shen. Behavior you cannot easily
  reduce to a spec — UI flows, integration with external services,
  performance — still needs tests. The framing on the README is
  "spec gates on top of tests, not instead of tests."
- **Not a runtime policy engine.** There is no OPA/Rego integration
  today. The `:runtime-via` annotation (a future feature in
  Wave-3 of the follow-up plan) gestures at one, but the
  current release does not ship it.
- **Not formal verification in the Coq/Lean sense.** Shen is a
  sequent-calculus type system; the guarantees it provides are
  exactly the guarantees its type system provides. We do not claim
  that a `BalanceChecked` value satisfies the balance invariant in
  the model-theoretic sense — we claim that the target-language
  compiler refuses to produce one without going through a
  constructor that ran the corresponding predicate.

## When to use this

Shen-Backpressure is most valuable when:

- You have a small set of **named invariants** that recur across a
  codebase (authorization scoping, balance non-negativity,
  rate-limit budgets, schema validation at a boundary).
- The invariants are **easy to state and easy to violate by
  forgetting a check** — i.e., the failure mode is
  "implementor-A's new handler skipped the check that
  implementor-B added six months ago," not "the check itself is
  hard to write."
- You're using an **AI coding loop** that benefits from
  deterministic gate failure as backpressure rather than from
  "looks plausible" empirical signal.
- You want **the proof chain to be inspectable in the repo** —
  the spec is a `.shen` file you can read, the generated guards
  are committed code you can read, the discharge report is JSON
  you can read.

It is **less valuable** when:

- The invariants live in unstructured prose ("be helpful, be safe")
  rather than properties.
- The implementation does the right work but is hard to express in
  Shen (heavy concurrency, foreign-function I/O, ML inference). The
  TCB grows and the structural slice shrinks.
- You need a third-party-attested certification rather than a
  self-produced verification artifact.

## Glossary: discharge categories

The discharge report (`discharge_report.json`) classifies every
premise of every Shen rule into one of three buckets. This glossary
maps the schema strings to plain English:

| Schema value | Plain English | What changed at build time |
|---|---|---|
| `static` | The target-language compiler refuses to call this constructor with a value that violates the premise. (`guard-type-at-boundary` — the parameter is itself a guard type; or `guard-constructor-validates` — the constructor runs the predicate before returning, so every value that exists satisfies it.) | shengen emitted a constructor whose body returns an error on predicate failure; the language compiler enforces unexported fields. |
| `runtime-sample` | The Shen `(define …)` block was evaluated as an oracle on a deterministic boundary pool plus optional seeded random draws, and the hand-written implementation matched on every sampled input. **Sampled evidence, not exhaustive.** | shen-derive generated a `go test` (or TS test) file pinning the implementation; the test was run and passed. |
| `unproven` | The tool could not classify the premise in this release. Treat the premise as outside the verified boundary. | Nothing. The premise is named in the spec but not discharged by either of the above mechanisms. |

The schema reserves `runtime-assertion` and `prover` for future use;
v1 emits only the three above.

For the canonical glossary inside the engine, see `auditAppendix`
in `cmd/sb/audit_report.go:277-321` — every rendered `audit_report.md`
ships with this glossary at the bottom. This page is the
expanded version.

The schema is locked at `schema_version: 1`; see the design memo at
`thoughts/shared/research/2026-05-05-discharge-report-schema.md`
for the rule that field additions must be additive and version-1
field meanings are immutable.

## Worked example: the multi-tenant JWT chain

`examples/multi-tenant-api/` is the proof chain
`JwtToken → AuthenticatedUser → TenantAccess → ResourceAccess`.
Here is exactly where trust lives at each link:

| Premise | Where it lives | Discharge | TCB? |
|---|---|---|---|
| The JWT signature is valid | `internal/auth/jwt.go:52-82` (`crypto/hmac.Equal`) | Not in the spec today; runtime-checked in `Parse` before `NewJwtToken` is called. | Yes — TCB |
| The JWT is non-empty | `(not (= X "")) : verified` in `specs/core.shen:32` | `static` (constructor validates) | No |
| The token is unexpired | `(> Exp Now) : verified` in `specs/core.shen:41` | `static` (constructor validates) | No |
| The `UserId` matches the JWT `sub` claim | **Not in the spec.** Middleware binding at `internal/auth/middleware.go:47-48` is by convention. | Not discharged (assumed) | Yes — TCB |
| The principal is a member of the tenant | `(= IsMember true) : verified` in `specs/core.shen:87`; the `isMember` boolean is supplied by `CheckTenantAccess` (`internal/auth/tenant.go:15-30`) | `static` for the boolean check; `CheckTenantAccess` is TCB for the SQL | Wrapper is TCB |
| The tenant owns the resource | `(= IsOwned true) : verified` in `specs/core.shen:97`; `CheckResourceAccess` supplies `isOwned` (`tenant.go:36-49`) | Same as above | Wrapper is TCB |

**Reading the table:** the structural chain is real — a handler
that demands `ResourceAccess` cannot be reached without the caller
having a `TenantAccess`, which cannot be obtained without an
`AuthenticatedPrincipal`, which cannot be obtained without going
through the JWT-validating constructor. **But** two of the
strongest claims a reader might assume — "the user identity is
bound to the token" and "the tenant membership claim is
correct" — rest on the TCB column, not the structural column.

Wave-2 of the HN-follow-up plan tightens the user-token binding by
introducing a `parsed-claims` composite and rewriting the
`authenticated-user` premise as a cross-field predicate. Until that
ships, the convention-only binding is documented here so a reviewer
sees it explicitly.

## Reviewer workflow

If you are a security reviewer opening this codebase cold:

1. **Read this document.** You're doing it.
2. **Open each example's `AUDIT.md`** — one-page workflows that
   walk the reviewer steps for that specific demo:
   `examples/payment/AUDIT.md`,
   `examples/multi-tenant-api/AUDIT.md`,
   `examples/shen-web-tools/AUDIT.md`.
3. **Verify the spec hash** matches the committed file
   (`sha256sum specs/core.shen`); the same value appears in
   `transcript/audit_report.md`'s `spec.files[].sha256`.
4. **Read `transcript/audit_report.md`** — the long-form rendering
   of the discharge report. Pay attention to the
   `discharged_since_commit` field: it tells you how stable each
   invariant has been across the project's git history.
5. **Re-run the gates at the recorded commit** to confirm the
   artifact reproduces:
   `git checkout <commit-from-report> && cd examples/<demo> && sb gates`.
6. **Read the TCB.** For each `Check*` wrapper named in [What's
   assumed](#whats-assumed-the-tcb), open the source and read it.
   These are small (10-50 lines each); reading them is the audit.
7. **Read the predicate implementations** in
   `internal/shenguard/guards_gen.go` (or `runtime/guards_gen.ts`).
   These are mechanically generated, but the generator output is
   what the compiler will enforce; spot-checking the lowering
   against the spec is the audit of the emitter.

The discharge report and audit report are the entry point; this
document is the map of where to look.

---

For the engine architecture (gate topology, manifest format,
discharge schema), see `README.md` (top-level) and
`docs/REFERENCE.md`. For per-example reviewer steps, see each
example's `AUDIT.md`.
