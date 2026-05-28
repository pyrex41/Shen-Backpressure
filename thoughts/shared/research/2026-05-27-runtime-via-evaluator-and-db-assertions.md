---
date: 2026-05-27
researcher: reuben
git_commit: 208a096
branch: claude/demo-cleanup
repository: Shen-Backpressure
topic: "Plan: expand `:runtime-via` from compile-time witness to evaluator-hosted runtime checks, sampled-equivalence drift-catch, and DB-attested assertions"
tags: [plan, runtime-via, shen-derive, multi-tenant-api, discharge-report, db-assertions]
status: draft
last_updated: 2026-05-27
last_updated_by: reuben
---

# Plan: `:runtime-via` next phase — evaluator host, sampled-equivalence, DB assertions

## Why now

Commit `2634f76` shipped `:runtime-via <fname>`. The Shen comment marker is parsed; Go and TS emitters generate a constructor that calls a named checker via a `(ctx, predicate, args…) → (ok, err)` envelope; a witness declaration (`var _ runtimeChecker = <fname>`) makes the build fail if the checker is missing or wrong-shaped. The discharge report's `tools.shen_runtime` field is populated when any referenced spec carries the marker.

That delivers **compile-time non-skippability**. It leaves three gaps that together make the difference between "interesting prototype" and "the runtime story this project keeps promising":

1. **The checker is opaque to the spec.** `shengen` only checks the function signature. There is no machinery to assert that `shenEval("query-text", ["foo"])` returns what Shen's `(> (length "foo") 0)` returns. Bespoke checkers can silently drift from the predicate they are supposed to implement.

2. **The shen-derive embedded evaluator is not exposed as a host.** `shen-derive/core/eval.go` already evaluates lowered Shen predicates and `(define …)` blocks during sampling. Nothing wires it into `:runtime-via`. The "no separate SBCL needed" claim in the README has no demo behind it.

3. **DB-backed assertions have no spec vocabulary.** `verified.CheckTenantAccess` (`examples/multi-tenant-api/internal/verified/access.go:73-94`) runs SQL, gets a bool, calls `NewTenantAccess(... isMember)`. The Shen spec sees only `(= IsMember true)`. The structural premise is hollow; the real assertion lives in the wrapper. `:runtime-via` is the natural place to absorb this — annotate the spec, drop the wrapper, surface the DB query as an audit artifact — but no demo does this yet.

This plan closes all three. It is structured as four discharge **profiles** with shared infrastructure, sequenced so each phase ships an independently useful artifact.

---

## Architecture

### Four profiles on one spectrum

| Profile | Annotation | Predicate impl | Drift-catch | Evidence in discharge report | When to use |
|---|---|---|---|---|---|
| **A — Bespoke** (existing) | `:runtime-via <fname>` | Hand-written | None — TCB by trust | `runtime-attested` (no oracle) | Transitional / when the checker is genuinely opaque |
| **B — Evaluator-hosted** | `:runtime-via :eval` | Auto-generated from spec | Trivial (same source) | `runtime-evaluator` | Predicate is pure data, want runtime evaluation without bespoke code |
| **C — Bespoke + sampled-equivalence** | `:runtime-via <fname> :sampled` | Hand-written | shen-derive test pins checker to predicate on sampled domain | `runtime-attested-sampled` + link to equivalence test | Bespoke impl exists for performance/clarity, want compile-time + runtime + drift catch |
| **D — DB-attested** | `:runtime-via <fname> :requires-db` | Hand-written, takes DB handle | Integration test against fixture data; SQL string surfaced in audit | `runtime-attested-db` + link to integration test + SQL excerpt | Predicate's truth lives in a relation outside the type system |

Profile A is what shipped. Profiles B, C, D are the additions.

### One annotation grammar with modifiers

Extend the existing comment marker. Today: `\* :runtime-via <fname> *\`. Proposed:

```
\* :runtime-via {<fname> | :eval} [:sampled] [:requires-db] *\
```

Rules:
- `:eval` and `<fname>` are mutually exclusive.
- `:sampled` is only legal with `<fname>` (the evaluator IS the oracle in Profile B; no equivalence test to write).
- `:requires-db` is only legal with `<fname>` (the evaluator does not have DB access by design).
- `:requires-db` implies the bespoke checker's signature requires a richer context, enforced at compile time by a different witness type (`runtimeCheckerDB`).
- `:sampled :requires-db` together is legal — it means "bespoke DB checker, sampled equivalence against a fixture-mocked oracle." Useful but expensive; left as a follow-up after the simpler combinations land.

Backward compatibility: existing `:runtime-via <fname>` (no modifiers) → Profile A, unchanged behavior. All evolution is additive.

### Shared infrastructure

Three new pieces of code, each one used by multiple profiles:

- **`shen-derive/runtime/evaluator-host`** (new package) — wraps `shen-derive/core/eval.go` in a `RuntimeChecker`-shaped function. Takes a spec path + predicate name, looks up the predicate's lowered expression, evaluates against `args`. Used directly by Profile B (the generator emits a thin import) and as the oracle by Profile C (sampled equivalence).

- **`shen-derive/verify/equivalence`** (extension of `shen-derive/verify/`) — emits a `*_runtime_via_test.go` (and `.test.ts`) that calls the bespoke checker with the same sampled inputs the existing `BuildHarness` already constructs, and asserts agreement with the evaluator-hosted result. The existing `samples.go` sampling pool is reused; the only new code is the equivalence assertion template.

- **`shen-derive/report` extensions** — three new `Discharge` values, three new `DischargeBasis` values, two new optional `Premise` fields (`EquivalenceTest`, `RuntimeChecker`), and a `RuntimeProfile` field on `Premise` that records `A | B | C | D`. All additive — schema stays at v1 per the additive-evolution rule in `thoughts/shared/research/2026-05-05-discharge-report-schema.md`.

---

## Phase-by-phase plan

### Phase 0 — Decisions to make before writing code

Three small architecture choices to lock in. None are deep but all are load-bearing.

**P0.1. Profile naming in the audit-facing artifact.** Internal field names use A/B/C/D, but `sb audit-report` should not show letters. Proposed labels: "bespoke checker", "evaluator-hosted", "bespoke checker + sampled equivalence", "DB-attested." Either confirm or replace before the renderer is written.

**P0.2. Where the evaluator-host package lives.** Two options: `shen-derive/runtime/` (treats the evaluator-host as a runtime concern, sibling to `core/`) or `shen-derive/host/` (emphasises "this is what hosts a runtime check, not what runs one"). Recommend `shen-derive/runtime/` — it parallels the `runtime/` directory in `examples/shen-web-tools/` and `examples/multi-tenant-api/`.

**P0.3. `:requires-db` context shape.** The Go signature has to choose between (a) extending the existing `runtimeChecker` type to take `*sql.DB` directly, (b) defining a parallel `runtimeCheckerDB` type, or (c) leaving the type the same and passing the DB through `context.Value`. Recommend (b) — separate witness type — because it makes the compile-time enforcement of "this constructor demands a DB-capable caller" visible at the signature site, and avoids the `context.Value` anti-pattern.

### Phase 1 — Schema additions (foundation)

Extend `shen-derive/report/schema.go` with new constants and optional fields. No version bump — schema stays at v1, all changes additive.

- New `Discharge` values: `runtime-attested`, `runtime-evaluator`, `runtime-attested-sampled`, `runtime-attested-db`.
- New `DischargeBasis` values: `runtime-via-witness`, `runtime-via-evaluator`, `runtime-via-sampled-equivalence`, `runtime-via-db-attested`.
- New optional `Premise` fields:
  - `RuntimeProfile string` — `"A"|"B"|"C"|"D"` (rendered as the audit-facing label in `sb audit-report`).
  - `RuntimeChecker *string` — name of the bound checker (nil for Profile B).
  - `EquivalenceTest *string` — relative path to the generated equivalence test (Profile C only).
  - `DBQueryExcerpt *string` — SQL string captured at gate time (Profile D only; populated by the checker registering a `func() string` at init).
- New `Summary` fields: `PremisesRuntimeAttested`, `PremisesRuntimeEvaluator`, `PremisesRuntimeAttestedSampled`, `PremisesRuntimeAttestedDB`. Existing `PremisesStatic` / `PremisesRuntimeSampled` / `PremisesUnproven` stay; the new fields are additive.

Migration: existing reports continue to roundtrip — every new field is optional. `roundtrip_test.go` extended with a v1-with-no-new-fields case (proves additive evolution holds).

`shen-derive/report/classify.go` extended: when a verified premise carries a `:runtime-via` marker (currently silently swallowed by `ClassifyDatatypes` — verify with grep), classify it by profile rather than always emitting `DischargeStatic`. The classifier needs access to the parsed marker, which today lives only in `cmd/shengen/main.go`. Two options: (i) move marker parsing into `shen-derive/specfile/`, or (ii) re-parse the marker in `report/`. Recommend (i) — the marker becomes a first-class field on `specfile.VerifiedPremise`, and both `shengen` and `shen-derive` read it.

**Acceptance**: existing discharge reports roundtrip unchanged; a new spec with a `:runtime-via <fname>` premise classifies as `runtime-attested` (Profile A) in the report; existing tests pass.

### Phase 2 — Profile B: evaluator-hosted checker

Implements `:runtime-via :eval`.

**Generator changes** (`cmd/shengen/main.go`, `cmd/shengen-ts/shengen.ts`):
- When marker is `:eval`, emit a different constructor body: instead of calling a user-supplied function, import a generated helper that calls into the evaluator-host package with the predicate's lowered s-expression.
- The lowered predicate is already computed during normal emission for inline checks — reuse the same lowering function, just route it differently.
- For Go: generated module gains a `_predicate_<name>` constant containing the s-expression string, and the constructor calls `evaluatorHost.Check(ctx, _predicate_<name>, args...)`.
- For TS: same pattern, with the TS port of the evaluator at `cmd/shen-derive-ts/core/`.

**Evaluator-host package** (`shen-derive/runtime/`):
- `evaluator-host/host.go` — exports `Check(ctx, sexprSrc string, args ...any) (bool, error)`. Parses the s-expression once at first call (cached), builds an `Env` from `args` zipped against the predicate's free variables, calls `core.Eval`, asserts `BoolVal` result.
- `evaluator-host/host_test.go` — exercises the same predicates the payment demo uses inline today (`(>= X 0)`, `(<= X (* 12 1000000 100))`, etc.) and confirms identical truth tables.

**Discharge classification**: Profile B premises get `discharge: "runtime-evaluator"`, `discharge_basis: "runtime-via-evaluator"`. Rationale auto-generated: "the predicate is evaluated at runtime by the shen-derive evaluator against the original spec expression; spec and runtime check are the same source."

**Demo**: extend `examples/payment/specs/core.shen` — annotate one premise on `amount` with `:runtime-via :eval`. The generated `guards_gen.go` constructor for `Amount` now imports the evaluator-host, and runtime construction goes through it. No SBCL involved. Verify by deleting the host package and watching the build fail.

**Acceptance**: `examples/payment` builds and tests pass with Profile B on at least one premise; discharge report shows `runtime-evaluator` for that premise; manual "delete the host, build fails" smoke test documented in `docs/RUNTIME-VIA.md`.

### Phase 3 — Profile C: bespoke + sampled-equivalence drift catch

Implements `:runtime-via <fname> :sampled`. This is the killer drift-catch feature — it closes the "checker can lie about implementing the predicate" gap by emitting a committed test that pins them together.

**`sb derive` extension**:
- Existing `sb derive` walks `[[derive.specs]]` and emits `*_spec_test.go` for each `(define …)` block via `verify.BuildHarness` + `verify.EmitTest`.
- Extend it to also walk verified premises carrying `:sampled`, treating the premise's lowered predicate as a synthetic single-clause `define` named `<datatype-name>__<premise-slug>__predicate`.
- Sample the premise's inputs the same way `verify.GenSamplesCtx` already does (drawing from `TypeTable`).
- Emit a `<datatype>_runtime_via_test.go` that imports the bespoke checker, calls it with each sample, calls the evaluator-host with the same sample, and asserts agreement. Counterexamples flow into the discharge report through the existing `cmd/sb/discharge.go` counterexample-collection path.

**Discharge classification**: `discharge: "runtime-attested-sampled"`, `discharge_basis: "runtime-via-sampled-equivalence"`, `equivalence_test: "internal/shenguard/<datatype>_runtime_via_test.go"`, `samples_passed: N`, `samples_failed: M`.

**Demo**: extend `examples/shen-web-tools` — annotate the existing `query-text` premise (`(> (length X) 0)`) with `:sampled` in addition to the existing `:runtime-via shenEval`. The generated test exercises both paths and confirms the SBCL backend agrees with the inline evaluator on the boundary set + 8 random draws. This is the demo that proves the SBCL bridge implements the spec, not something adjacent.

**Acceptance**: counterexample test — deliberately make `shenEval` answer `true` for empty string (e.g. via test override), confirm `sb derive` flags it and the discharge report shows a counterexample with the offending input.

### Phase 4 — Profile D: DB-attested assertions

Implements `:runtime-via <fname> :requires-db`. The DB extension.

**Witness type** (per P0.3 decision): `cmd/shengen/main.go` emits a separate `type runtimeCheckerDB func(ctx context.Context, db *sql.DB, predicate string, args ...any) (bool, error)` when `:requires-db` is present, with its own `var _ runtimeCheckerDB = <fname>` binding. The constructor signature for the gated type becomes `NewTenantAccess(ctx context.Context, db *sql.DB, principal AuthenticatedPrincipal, tenant TenantId) (TenantAccess, error)` — the DB is structurally required by the compile-time signature, not threaded through `context.Value`.

**SQL excerpt surfacing**: define a tiny optional registration interface — a checker can implement `SQLExcerpt() string` (Go) / `sqlExcerpt(): string` (TS) and `sb derive` will read it at gate time, capturing the SQL string into the discharge report's `db_query_excerpt` field. Auditors then see the actual query in `sb audit-report` without leaving the report. Optional because not every DB checker is a single SQL string (some compose multiple queries).

**Integration test linkage**: Profile D premises declare a test path via `[[derive.runtime_via]]` entry in `sb.toml`:

```toml
[[derive.runtime_via]]
checker = "checkTenantMembership"
integration_test = "internal/verified/access_test.go::TestCheckTenantMembership_FixtureData"
```

`sb derive` records this path in the discharge report's `integration_test` field per premise. The integration test is run by the existing test gate, not by `sb derive` itself.

**Discharge classification**: `discharge: "runtime-attested-db"`, `discharge_basis: "runtime-via-db-attested"`, `db_query_excerpt`, `integration_test`.

**Acceptance**: deferred to Phase 5 (the migration is the demo).

### Phase 5 — Migrate multi-tenant-api `(= IsMember true)` and `(= IsOwned true)` to Profile D

This is the headline demo. It collapses `verified.CheckTenantAccess` (currently a wrapper layer that exists to bridge SQL → bool → constructor) into a `:runtime-via :requires-db` annotation.

**Spec changes** (`examples/multi-tenant-api/specs/core.shen`):

```shen
(datatype tenant-access
  Principal : authenticated-principal;
  Tenant : tenant-id;
  (member-of Principal Tenant) : verified; \* :runtime-via checkTenantMembership :requires-db *\
  ================================
  [Principal Tenant] : tenant-access;)
```

Two structural changes from today's spec:
- `IsMember : boolean` field is dropped — the boolean is computed inside the checker, not threaded through the constructor.
- `(= IsMember true)` becomes `(member-of Principal Tenant)` — a synthetic predicate name that maps to the bespoke checker. Shen's `tc+` does not know how to evaluate `member-of`; the `:runtime-via` marker makes that fine because the predicate is never lowered.

**Go changes** (`examples/multi-tenant-api/internal/`):
- `shenguard/guards_gen.go` regenerates: `NewTenantAccess(ctx, db, principal, tenant) (TenantAccess, error)`, witness `var _ runtimeCheckerDB = checkTenantMembership`.
- `verified/access.go` shrinks dramatically — `CheckTenantAccess` becomes a thin re-export of `shenguard.NewTenantAccess`. The SQL query moves into a new `internal/verified/checkers.go::checkTenantMembership` function, which implements both the checker signature and `SQLExcerpt() string`.
- Handlers (`internal/handlers/handlers.go`) get a `db` parameter threaded through. This is a real ergonomic cost — document it.

**Resource-access**: same treatment. `(owns Tenant Resource)` → `:runtime-via checkResourceOwnership :requires-db`.

**Discharge report**: the rendered audit report now has, for `tenant-access`:

```
Premise: (member-of Principal Tenant)
  Profile: DB-attested
  Checker: checkTenantMembership
  SQL: SELECT COUNT(*) FROM tenant_memberships WHERE user_id = ? AND tenant_id = ?
  Integration test: internal/verified/access_test.go::TestCheckTenantMembership_FixtureData ✓
```

This is the audit surface the post promises. The SQL is visible to a reviewer who never opens the Go source.

**AUDIT.md update**: today says "TCB: the SQL query must be exact. Read it before trusting the chain." After this phase, the SQL is in the discharge report itself, so the auditor's workflow is "read the discharge report" rather than "find and read the SQL by grep."

**Acceptance**: end-to-end demo: `make demo` boots the API, hits the protected endpoint, request succeeds with a valid member, fails with a non-member. Discharge report shows Profile D for both `tenant-access` and `resource-access`. `bypass_attempts/` extended with one new file: an attempt to construct `TenantAccess` *without* a DB handle, which fails to compile.

### Phase 6 — TS port via `cmd/shen-derive-ts`

Phases 2 and 3 require a TS-side evaluator-host. The `cmd/shen-derive-ts/core/` directory exists (the TS port of the evaluator was already started for shen-derive's TS sampling story).

- Port `evaluator-host/host.ts` mirroring the Go version. Same `Check(ctx, sexprSrc, args)` shape; async because TS callers already get async constructors via `:runtime-via`.
- Extend `cmd/shengen-ts/shengen.ts` to emit the `:eval` variant (Phase 2 TS-side).
- Extend `cmd/shen-derive-ts` to emit `*_runtime_via.test.ts` (Phase 3 TS-side, using Vitest).
- Cross-language equivalence test: one fixture spec, generate both Go and TS Profile B emissions, assert truth tables agree on the same inputs. Lives at `cmd/shen-derive-ts/cross_lang_equivalence.test.ts`.

Phase 4 (`:requires-db`) is Go-only for now — TS apps with DB access have wildly varying DB libraries, and a uniform DB-context witness is out of scope for this phase.

**Acceptance**: `examples/shen-web-tools` runs Phase 2 + Phase 3 against TS as well as the live SBCL backend; cross-language equivalence test green.

### Phase 7 — Docs + rendering

`docs/RUNTIME-VIA.md` already covers Profile A. Extend with four sections — one per profile — showing annotation, generated code shape, discharge classification, and worked example. Crucially, the four profiles form a *decision tree* (lossy quick-reference) that the doc should lead with:

> - Predicate is pure data, no IO? Profile B.
> - Predicate is pure data but you want a bespoke impl (e.g. for perf, library reuse)? Profile C.
> - Predicate's truth lives in a DB? Profile D.
> - You have a bespoke checker that doesn't fit B/C/D (genuinely opaque, OPA, external policy engine)? Profile A.

`cmd/sb/audit_report.go::auditAppendix` — extend the "How to read this report" section with one paragraph per new profile. Renderer additions:
- Profile B: show evaluator-host import path; one line "the spec is the runtime check, no separate implementation"
- Profile C: show the equivalence test path; show samples passed/failed; if any failed, render counterexamples
- Profile D: show `db_query_excerpt` block; show integration test path with last-run pass/fail (requires test-gate cross-reference)

`docs/TRUST-MODEL.md` update: split the TCB enumeration by profile. Today it lists "Check* wrappers + predicate impls + JWT parser + DB queries" as a flat set. Restructured:
- Always TCB: the constructor itself, the Shen lowering, the witness types.
- Profile A TCB: bespoke checker, wire format.
- Profile B TCB: evaluator-host package (and through it, `shen-derive/core`).
- Profile C TCB: bespoke checker + the equivalence test (the test pins the two together but is itself code that could be wrong).
- Profile D TCB: bespoke checker + SQL string + the row-level guarantees of the DB (e.g. `tenant_memberships.user_id` has a foreign key).

**Acceptance**: a reader can determine from `docs/RUNTIME-VIA.md` alone which profile to pick for a given predicate, and `sb audit-report` shows the right per-profile evidence block for each annotated premise in the three demo projects.

---

## Open questions

**Q1. Does Profile B need a way to depend on `(define …)` blocks?** Predicates like `(> (length X) 0)` only use built-in primitives — the evaluator handles them out of the box. But a richer predicate like `(member-of-allowlist X)` where `member-of-allowlist` is a `(define …)` in the same spec would need the evaluator-host to load the `(define …)` blocks at init. Doable — the existing `verify.BuildHarness` already supports cross-define references via `AllDefines`. Whether to expose that for runtime-via in this phase is a sizing call.

**Q2. Should Profile D ever produce counterexamples?** Profile C does — the equivalence test runs at gate time and any disagreement is a counterexample. Profile D's integration test runs at gate time too, but its failure mode is usually "fixture missing" or "schema migration broke the query," not "the predicate disagrees with the spec." It might be cleaner to leave Profile D counterexamples empty and rely on the integration test's pass/fail boolean. Decide based on whether the audit report wants uniformity or accuracy.

**Q3. Profile A's place in the spectrum.** Once B/C/D exist, is there a remaining case for Profile A — a bespoke checker with no equivalence proof and no DB attestation? Yes, when the checker delegates to something opaque (OPA, a remote policy engine, an LLM). The doc should clearly say "Profile A is the right choice when the predicate's true definition lives in a system that can't be sampled deterministically." Otherwise default to C.

**Q4. Sequencing trade-off.** Phase 5 (multi-tenant migration) is the most user-visible deliverable, but it depends on Phase 4 (DB witness type). If the goal is "ship something for the follow-up blog post," Phase 2 (Profile B in the payment demo) lands fastest and tells the cleanest standalone story. Phase 5 is the bigger payoff but a heavier lift. Recommend shipping Phase 2 first as a self-contained "shen-derive evaluator now hosts runtime checks" PR, then doing Phases 3–5 as a longer arc.

**Q5. Do `:sampled` and `:requires-db` compose?** Marked as legal-but-deferred above. The implementation requires a fixture-mocked oracle (so the equivalence test can run without a DB), which is a non-trivial machinery. Most projects can live without it for the first release.

---

## Sequencing recommendation

For the follow-up post:

1. **Phase 0** decisions (one short session, lock the grammar + naming).
2. **Phase 1** schema additions (one PR, additive, low risk; unblocks everything else).
3. **Phase 2** Profile B in payment demo (one PR, headline: "the shen-derive evaluator is now a turnkey runtime host, no SBCL needed").
4. **Phase 3** Profile C in shen-web-tools (one PR, headline: "we now catch drift between the bespoke runtime checker and the spec").
5. **Phase 4 + 5** as a paired PR series — DB witness + multi-tenant migration (headline: "the JWT example now demonstrates spec-level DB attestation, the wrapper layer is gone").
6. **Phase 6** TS port (can interleave with 3 if needed for shen-web-tools).
7. **Phase 7** docs (continuous; final pass after 5 ships).

If the post is on a clock, **2 + 3 alone is a credible standalone story** ("the evaluator went from sampling-only to a first-class runtime host, and we now have committed drift catches between hand-written checkers and the spec"). 4 + 5 is the deeper second post.

---

## Code references (current state, pre-plan)

- `cmd/shengen/main.go::extractRuntimeViaComment` — marker parser to extend with `:eval` / `:sampled` / `:requires-db`
- `cmd/shengen/main.go::emitRuntimeViaCheckGo` — Go emitter to fork by profile
- `cmd/shengen-ts/shengen.ts::emitRuntimeViaCheckTs` — TS emitter, same
- `shen-derive/core/eval.go` — evaluator that becomes the Profile B/C host
- `shen-derive/specfile/` — where `VerifiedPremise.RuntimeViaMarker` should land
- `shen-derive/report/classify.go::classifyDatatypeRule` — premise classifier to extend with profile awareness
- `shen-derive/report/schema.go` — additive field additions
- `shen-derive/verify/harness.go` + `samples.go` — sampling pool reused by Profile C
- `cmd/sb/derive.go::detectShenRuntime` — already populates `ShenRuntime`; extend to discriminate profiles for the summary block
- `cmd/sb/audit_report.go::auditAppendix` — renderer extension surface
- `examples/payment/specs/core.shen` — Phase 2 demo target
- `examples/shen-web-tools/specs/core.shen` — Phase 3 demo target
- `examples/multi-tenant-api/specs/core.shen` — Phase 5 migration target
- `examples/multi-tenant-api/internal/verified/access.go` — wrapper layer that shrinks to a re-export
- `docs/RUNTIME-VIA.md` — doc to extend with four profiles
- `docs/TRUST-MODEL.md` — TCB enumeration to restructure by profile

## Related research

- `thoughts/shared/research/2026-05-22-hn-feedback-next-steps.md` §R4 — motivation
- `thoughts/shared/research/2026-05-22-schema-v1-additions.md` — additive-evolution precedent (W3.2 addendum)
- `thoughts/shared/research/2026-05-05-discharge-report-schema.md` — v1 schema lock + additive rules
- `thoughts/shared/research/2026-04-09-shen-derive-vision-gap-analysis.md` — earlier gap-analysis on shen-derive coverage
