---
date: 2026-05-22T22:16:46-05:00
researcher: reuben
git_commit: 81ddf671a482f8b325db11acd04c72ec4182af03
branch: main
repository: Shen-Backpressure
topic: "Next steps responding to HN discussion of Shen-Backpressure"
tags: [research, demos, jwt, multi-language, runtime-gates, tcb-audit, discharge-report, hn-feedback]
status: complete
last_updated: 2026-05-22
last_updated_by: reuben
---

# Research: Next steps responding to HN discussion of Shen-Backpressure

**Date**: 2026-05-22T22:16:46-05:00
**Researcher**: reuben
**Git Commit**: 81ddf671a482f8b325db11acd04c72ec4182af03
**Branch**: main
**Repository**: Shen-Backpressure

## Research Question

Based on the HN discussion of "Formal Verification Gates for AI Coding Loops" (Shen-Backpressure), what are concrete next-step recommendations for the codebase and demos? Focus on:

(a) Hardening the JWT/auth example so it's production-credible.
(b) Demonstrating multi-language emit as more than a claim.
(c) The runtime-gate composition story (Shen-as-runtime, or OPA behind constructor).
(d) Whether `tcb-audit` exists and how visible the audit/discharge story is in demos.

Grounded in what actually exists in the repo today, not generic ideas.

## Summary

The repo is in a stronger state than the HN comments imply, but the demos undersell several pieces of infrastructure that are already shipped — and the JWT example's spec is genuinely thin in the way `singron` and `max_unbearable` describe. The runtime Go code in `multi-tenant-api` does most of what those commenters were asking for (real HMAC verification, expiry check, claim → guard binding in the middleware), but **none of that work is reflected in the spec**, so a reader looking at `specs/core.shen` sees only `(not (= X ""))` and concludes the guard chain is hollow.

Most concrete next steps therefore split into two buckets:

1. **Tighten the spec/guard surface so the published demo matches the runtime substance** — strengthen JWT predicates, encode the token↔user binding structurally, expose only `Check*` wrappers from the `shenguard` package, and add a `bin/show-bypass-attempts.sh` that turns failed type-check attempts into part of the demo.
2. **Surface the artifacts that already exist** — commit a sample `discharge_report.json` and rendered `sb audit-report` Markdown alongside the demos, add a dedicated auditor walkthrough, and extend `tcb-audit` + `sb gen` dispatch so the multi-language emit story can be demonstrated end-to-end (today the Python and Rust emitters are unreachable from `sb gen`, and the audit script is Go-only).

The four focus areas have concrete, scoped work items each. None of these are speculative — they all replace existing surfaces with stronger ones.

## Detailed Findings

### (a) JWT/auth example — current state

**Spec** (`examples/multi-tenant-api/specs/core.shen`)

Nine datatypes, predicates: `(not (= X ""))` on `jwt-token`, `(> Exp Now)` on `token-expiry`, `(= IsMember true)` on `tenant-access`, `(= IsOwned true)` on `resource-access`. `authenticated-user` has **no `verified` premise of its own** — it structurally aggregates a `jwt-token`, a `token-expiry`, and a `user-id` but does not assert any binding between them.

**Runtime trust boundary** (`internal/auth/`)

The Go runtime is substantially stronger than the spec implies:

- `jwt.go:52-82` — hand-rolled HMAC-SHA256 verify using `crypto/hmac.Equal` (constant-time), then JSON-decode claims, then expiry check. Real signature verification, not stub. Tests include tampered-payload rejection (`jwt_test.go` `TestParseTamperedPayload`).
- `middleware.go:35-51` — extracts `result.Claims.Sub` from the parsed JWT, wraps it via `shenguard.NewUserId(result.Claims.Sub)`, then assembles `NewAuthenticatedUser(jwtToken, expiry, userID)`. The `user-id` carried in the guard **does come from the JWT claim**.
- `tenant.go:15-34` — `CheckTenantAccess(db, principal, userID, tenantID)` performs a DB lookup against `tenant_memberships` and calls `NewTenantAccess(... isMember)`.
- `tenant.go:36-49` — `CheckResourceAccess(db, access, resourceID)` uses `access.Tenant().Val()` from the proof object, so the tenant dimension is structurally bound.

**The two genuine gaps surfaced on HN**

1. **No structural binding token ↔ user.** `NewAuthenticatedUser(token JwtToken, expiry TokenExpiry, user UserId) AuthenticatedUser` is **infallible** (`guards_gen.go:97`). A caller in possession of any `JwtToken` and any `UserId` can pair them. The middleware does it correctly by convention; the type system does not enforce it. This is precisely `singron`'s point.
2. **`CheckTenantAccess` takes a separate `userID string` parameter** (`tenant.go:15`) rather than extracting it from `principal`. The DB query uses the string parameter, not anything from the proof object. Handler code threads `human.Auth().User().Val()` through (`handlers.go:84`), but again that's convention, not enforcement.

Constructors are all **exported** (`NewJwtToken`, `NewTenantAccess`, etc.) — the "package-private raw constructor, only export `Check*` wrappers" hardening pattern the author described on HN isn't applied in the demo.

### (b) Multi-language emit — current state

**Coverage matrix:**

| Construct | Go (`cmd/shengen`) | TS (`cmd/shengen-ts`) | Py (`cmd/shengen-py`) | Rs (`cmd/shengen-rs`) |
|---|---|---|---|---|
| Datatype (wrapper/constrained/composite/guarded) | ✓ | ✓ | ✓ | ✓ |
| Sum types (multi-block) | ✓ | ✓ | ✓ (Union alias) | ✓ (sealed trait) |
| Verified predicates (subset: `>=, <=, >, <, =, not, element?, shen.mod, length`) | ✓ | ✓ | ✓ | ✓ |
| `(list X)` parametric types | ✓ | ✓ | ✗ (PRIMITIVES table missing) | ✗ (same) |
| `(define …)` pure-function emission | ✓ | ✓ | ✗ (word "define" absent from emitter) | ✗ (same) |
| Reachable from `sb gen` | ✓ | ✓ | ✗ | ✗ |

`cmd/sb/gen.go:52-66` only dispatches on `"go"` and `"ts"`. The Python and Rust emitters are standalone Python scripts (note: `cmd/shengen-rs/shengen.py` — yes, a `.py` file generating Rust) with no callers in the build.

**Reference outputs** (`examples/payment/reference/`)

Six checked-in static files: `guards_gen.{go,ts,py,rs}` plus `guards_gen_hardened.{py,rs}`. They look uniform because the `examples/payment/specs/core.shen` spec uses no `(define …)` blocks that get emitted and no `(list X)` types — i.e., the spec sits inside the intersection of all four emitters. Any richer spec would expose the gap. There is no automation that regenerates them all from one button.

**tcb-audit is Go-only.** `bin/shenguard-audit.sh:53-85` invokes the Go shengen binary directly, only scans `*.go` files in `shenguard/`, and diffs byte-for-byte against `guards_gen.go`. `examples/shen-web-tools/sb.toml` declares `lang = "ts"` and `tcb-audit` together — the audit script will not catch drift in the TypeScript output. There is no `shenguard-audit-ts.sh`.

**No cross-emitter equivalence test exists.** `cmd/shengen-ts/shengen.test.ts:1-7` was ported from `cmd/shengen/main_test.go` but they assert against their own outputs independently — there's no oracle that says "same spec must produce semantically equivalent guards across all four languages."

### (c) Runtime gate composition — current state

**Shen at runtime exists in exactly one place: the `shen-web-tools` backend.** `examples/shen-web-tools/backend/shen-interop.lisp:23-50` calls `(ql:quickload :shen)` at boot, runs `(tc +)` typechecking against `specs/core.shen` and `specs/medicare.shen` (`shen-interop.lisp:172-183`), and loads the application `.shen` files. However, this is **load-time only**. The hot path in `server.lisp:88-131` is plain Common Lisp; `shen-research` is wired but not the default route. The TypeScript bridge (`runtime/bridge.ts:52-96`) is just an HTTP client — no Shen runs in the browser.

**Generated guards already validate at runtime in both Go and TS.** They are not purely structural smart constructors:

- Go: `NewAmount(x float64) (Amount, error) { if !(x >= 0) { return Amount{}, fmt.Errorf(...) } ... }` (`examples/payment/internal/shenguard/guards_gen.go:26-33`).
- TS: `QueryText.createOrThrow(x: string)` evaluates the predicate at call time and throws if false (`examples/shen-web-tools/runtime/guards_gen.ts:17-29`). `GroundedSource.createOrThrow` (`guards_gen.ts:253-255`) checks `page.url() === hit.url()` — a relational invariant across fields, dynamically evaluated.

**No OPA/Rego integration anywhere.** Grep across `.go`, `.ts`, `.lisp`, `.sh` returns zero. The closest sketch is in `examples/.archive/shenguard-bolt-on/` — a `PolicyCompliant` boolean witness pattern that the caller supplies. Not active code.

**Discharge report has placeholder fields for runtime Shen.** `shen-derive/report/build.go:73-75` always sets `ShenRuntime: nil` and `ShenRuntimeAvailable: false`. The "runtime sampling" classification is build-time table-driven testing using the Go-embedded mini-evaluator at `shen-derive/core/eval.go`, not a running Shen process.

### (d) tcb-audit + discharge report visibility

**tcb-audit is active in all three examples** (`examples/payment/sb.toml:36-38`, `examples/multi-tenant-api/sb.toml:40-43`, `examples/shen-web-tools/sb.toml:35-38`). The script does live re-generation + `diff -q` plus an "unexpected file in `shenguard/`" reject (`bin/shenguard-audit.sh:47-79`). `sb context` lists the gate by name (`cmd/sb/context.go:300-309`) but does not surface latest pass/fail.

**Discharge reports are richly schema'd.** Per-rule + per-premise classification (`static` / `runtime-sample` / `unproven`), counter-examples with `case_id`, `input`, `spec_output`, `impl_output`, and a ready-to-paste `go test -run …` reproducer. `discharged_since_commit` is computed by walking `.sb/history/` backward (`cmd/sb/discharge.go:589-629`). Schema is locked at v1 (`thoughts/shared/research/2026-05-05-discharge-report-schema.md`).

**No sample reports are committed.** `.gitignore:29-31` excludes `.sb/` ("Wave 4 discharge reports — local-only audit trail"). A reader cannot inspect a real `discharge_report.json` or rendered `sb audit-report` Markdown without first cloning, installing the Shen runtime, and running gates.

**Visibility split across the three demos:**

- `examples/payment/README.md:91-136` walks through `cat .sb/discharge_report.json | jq '.summary'` and `sb audit-report | head -40`. **This is the only place** the full discharge story is shown.
- `examples/multi-tenant-api/demo.md` (19KB) mentions tcb-audit (`demo.md:153-163, 169-183`) but **never mentions `discharge_report.json`, `sb audit-report`, or `.sb/`**.
- `examples/shen-web-tools/` — no equivalent walkthrough.

**No auditor-workflow document exists.** The only thing approaching one is the `auditAppendix` constant in `cmd/sb/audit_report.go:277-321`, which is the "How to read this report" appendix appended to every rendered audit report. There is no top-level guide that says: "you're a security reviewer, here is what you open first, here is how to verify the spec hash, here is how to re-run the gates at the recorded commit."

## Recommendations

These are scoped to address the HN critiques directly, ordered by ROI given the current codebase.

### R1. Harden the multi-tenant-api spec & demo (highest ROI for blog credibility)

The HN critique that gets most engagement-per-character is "the JWT predicates are too weak." Fixing this transforms the canonical proof-chain demo from "illustrating shape" to "production-credible." Concrete work:

1. **Strengthen `jwt-token` predicates in `specs/core.shen`.** Add separate datatypes for the three-segment structure, the verified signature, the issuer/audience claims. The shengen emitter already supports cross-field predicates (the spec currently only uses single-field ones, but the Go emitter handles them — see `examples/shen-web-tools/runtime/guards_gen.ts:253-255` for a relational example).

2. **Bind token ↔ user structurally.** Make `authenticated-user`'s `verified` premise a cross-field predicate that asserts `User` equals the `Sub` claim extracted from `Token`. This will require the spec to expose a `(claim-sub Token)` accessor — easiest path is to model JWT as a composite with the parsed claims as fields, not a raw string. The compile-time gate then prevents `NewAuthenticatedUser(token_for_alice, ..., user_for_bob)`.

3. **Apply the package-private constructor hardening.** Make `NewTenantAccess` and `NewResourceAccess` unexported (`newTenantAccess`, `newResourceAccess`), and export only `CheckTenantAccess` / `CheckResourceAccess` from the `shenguard` package itself (move those wrappers from `auth/tenant.go` into the guard package). Then `handlers.go` can only obtain a `TenantAccess` via the DB-verifying wrapper. This is the author's own HN suggestion; the demo doesn't yet show it.

4. **Make `CheckTenantAccess` derive userID from the principal.** Drop the `userID string` parameter (`tenant.go:15`); the proof object should be the only input.

5. **Add a "negative space" demo.** Create `examples/multi-tenant-api/bypass_attempts/` with three or four `.go.bak` files that try to forge a `TenantAccess` — direct struct literal, reflection, etc. — and a `bin/show-bypass-attempts.sh` that compiles each and captures the type-system error. This makes the "physically cannot bypass" claim visible, not just stated. Pattern mirrors `examples/payment/demo-shen-derive/`.

6. **Make `demo.md` walk through the discharge report**, not just tcb-audit. The 19KB file currently stops at `sb gates` output. Adding ~50 lines that `cat .sb/discharge_report.json | jq` and show the per-premise discharge classification (especially the `static` discharge of the new token↔user binding) is the moment a reader sees how the proof chain actually composes.

### R2. Ship a committed sample of the discharge artifact

`.sb/` being gitignored is the right default for live projects but the wrong default for demos. Concrete work:

1. **Commit a sample under each demo as `transcript/discharge_report.json` and `transcript/audit_report.md`** (the `transcript/` directory already exists in `multi-tenant-api` and is a natural home). Update `.gitignore` to allow these paths under `examples/*/transcript/`. A reader on the HN-linked page can then open the artifact in their browser and see counter-examples, premise tables, the "How to read this report" appendix, etc., before deciding whether to clone.

2. **Add `AUDIT.md` to each example**, the auditor-workflow document that doesn't exist today: "1. Verify spec hash matches the committed file: `sha256sum specs/core.shen`. 2. Read `transcript/audit_report.md`. 3. To re-run: `git checkout <commit>; sb gates`. 4. Each rule's `discharged_since_commit` field tells you how stable that invariant has been." This is the "spec-as-audit-surface" story the post promises but doesn't deliver in demo form.

3. **Optional**: add a top-level `docs/AUDITING.md` describing the workflow once, and link from each example's `AUDIT.md`.

### R3. Make the multi-language story end-to-end runnable

Today the "one spec, four languages" line rests on six static files in `examples/payment/reference/`. The HN response to `vrm`'s Rust question explicitly cited "build catches drift" — but no build catches drift on Python or Rust output today. Concrete work:

1. **Add `"py"` and `"rs"` dispatch cases in `cmd/sb/gen.go:52-66`** so `sb gen` can target them. Marking them experimental is fine; making them unreachable from the engine is what makes the claim hollow.

2. **Close the Python/Rust emitter coverage gaps.** Add `(list X)` to their `PRIMITIVES` tables (`cmd/shengen-py/shengen.py:78`, `cmd/shengen-rs/shengen.py:78`). Add `(define …)` emission. Without this, any non-trivial spec produces broken/missing Rust or Python output. A minimal path is to port the Go define-translator (`cmd/shengen/main.go:760-820`) directly — it's already debugged in Go and TS.

3. **Generalize `bin/shenguard-audit.sh` to accept a language flag**, or split into `shenguard-audit-{go,ts,py,rs}.sh`. shen-web-tools should be checking drift on its TypeScript output today and isn't.

4. **Add a cross-emitter equivalence test** — one fixture spec, four emitter runs, four `_test.{go,ts,py,rs}` files asserting the same input-output table over the constructed values. This is the test that turns "we ported the emitter" into "the four outputs implement the same logic."

5. **Add a `cmd/sb/multi-target` demo example**: same `specs/core.shen`, gates emit Go and TS guards, both impls exposed via small HTTP/CLI surfaces. This is the demo that would have answered `vrm`'s "why not just Rust newtypes" question with a runnable artifact.

### R4. Compose compile-time + runtime gates (the OPA/se4u story)

The author's HN reply to `se4u` proposed: "compile-time assertion that the code calls the runtime assertion, with OPA sitting behind the constructor." Today the codebase has both halves but they're not composed:

- Generated guards already run predicate checks at runtime (Go and TS).
- shen-web-tools backend already loads a live Shen runtime at startup.
- Nothing yet generates a guard whose validator calls a *named external check*.

Concrete work, in order of ambition:

1. **Spec annotation `:runtime-via <name>` on a `verified` premise**, declaring the predicate should be discharged by a named runtime function rather than inlined. The generated constructor becomes `NewX(ctx, ...) (X, error) { ok, err := check(ctx, ...); if !ok { return X{}, err }; return X{...}, nil }`. The compile-time gate enforces that the package exports a function with the right signature; the runtime call is non-skippable because it's the only path through the constructor. Even without OPA wiring this delivers the "compile-time gate mandates a runtime check" composition primitive.

2. **Single-file shen-web-tools tweak**: have one `verified` premise on a guard be discharged by a call into the live SBCL backend instead of an inlined predicate. This is the smallest concrete demo of "same Shen spec, runtime-evaluated" — uses infrastructure that already exists. Possibly the cleanest place to add it is in the `medicare.shen` resolver path where the bridge already round-trips.

3. **Optional OPA integration**: implement `:runtime-via opa://<bundle>/<rule>` as the named check, with a small OPA client. This is net-new and lower-priority than (1) and (2).

### R5. Light-touch wins worth grouping

- **The discharge report's `ShenRuntime` and `ShenRuntimeAvailable` fields are placeholders** (`shen-derive/report/build.go:73-75`). Once R4.2 is shipped, populate them — the schema already reserves the room.
- **`sb context` doesn't surface latest gate pass/fail** (`cmd/sb/context.go:300-309` only lists topology). Adding a `LastResult` field to `GateInfo` would close the agent-loop feedback gap and is mostly a wiring change.
- **README's "Two Tools, One Spec File" table** is currently the clearest pedagogical artifact; consider adding an equivalent "Where the proof ends" table that explicitly maps which premises in `multi-tenant-api/specs/core.shen` are discharged statically vs runtime-sample vs unproven. That table is exactly the rebuttal to `singron`'s critique — without it the critic has to dig.

## Priority Recommendation

If picking only one thing: **R1 + R2 together**, as one PR series. They share an artifact surface (the demo), are mutually reinforcing (a hardened spec produces a more interesting discharge report, a committed discharge report makes the hardening visible), and respond directly to the comments that drove the most engagement on HN. R3 and R4 are larger but deliver claims the project already makes; if a follow-up post is planned, those become its substance.

## Code References

- `examples/multi-tenant-api/specs/core.shen:30-99` — current thin predicates
- `examples/multi-tenant-api/internal/auth/jwt.go:52-82` — real HMAC verification (stronger than spec)
- `examples/multi-tenant-api/internal/auth/middleware.go:35-51` — Sub → UserId binding done in code, not type
- `examples/multi-tenant-api/internal/auth/tenant.go:15-34` — `CheckTenantAccess` takes redundant `userID string`
- `examples/multi-tenant-api/internal/shenguard/guards_gen.go:97` — `NewAuthenticatedUser` infallible
- `cmd/sb/gen.go:52-66` — emitter dispatch (only go/ts)
- `cmd/shengen-py/shengen.py:78` — Py emitter missing `(list X)` and `(define …)`
- `cmd/shengen-rs/shengen.py:78` — same gap
- `bin/shenguard-audit.sh:47-85` — Go-only drift gate
- `examples/payment/reference/` — six static checked-in language outputs, no regeneration tooling
- `examples/shen-web-tools/backend/shen-interop.lisp:23-50, 172-183` — live Shen runtime, load-time only
- `cmd/sb/discharge.go:44-114` — discharge report schema
- `shen-derive/report/build.go:73-75` — `ShenRuntime` placeholders
- `cmd/sb/audit_report.go:277-321` — `auditAppendix` (only published auditor instructions)
- `examples/payment/README.md:91-136` — only example walking through discharge
- `examples/multi-tenant-api/demo.md:153-183` — mentions tcb-audit, doesn't mention discharge
- `.gitignore:29-31` — `.sb/` excluded (no committed sample reports)

## Architecture Documentation

The engine architecture is described in `README.md:152-174`: `sb` is the canonical deterministic knowledge surface, `sb.toml` declares gate topology, the agentic surface consumes CLI output. Wave-1/2/3 design memos under `thoughts/shared/research/2026-05-05-wave-*.md` document the manifest-driven gates evolution; the v1 discharge schema is locked at `thoughts/shared/research/2026-05-05-discharge-report-schema.md`.

The five-gate topology (six with `shen-derive`) is hardcoded in legacy mode (`cmd/sb/gates.go:137-143`) and reconstructable via `[[gates]]` in manifest mode. All three demos are on the manifest path.

## Historical Context (from thoughts/)

- `thoughts/shared/research/2026-05-05-discharge-report-schema.md` — v1 locked schema, additive-evolution rule
- `thoughts/shared/research/2026-05-05-wave-4-discharge-reports.md` — wave-4 design memo for the discharge artifact
- `thoughts/shared/research/2026-05-05-feature-mixed-evidence-report.md` — feature memo touching static/runtime-sample classification
- `thoughts/shared/research/2026-04-09-shen-derive-vision-gap-analysis.md` — earlier gap analysis on shen-derive coverage
- `thoughts/shared/research/2026-03-29-cross-language-enforcement-spectrum.md` — multi-language emit context

## Related Research

- `thoughts/shared/research/2026-04-16-demo-readiness-buttoning-up.md` — prior demo-readiness pass
- `thoughts/shared/research/2026-05-05-tag-resolver-finish-line.md` — in-flight shen-web-tools work
- `thoughts/shared/research/2026-03-31-full-codebase-exploration.md` — earlier full-codebase walk

## Open Questions

- Is there appetite for breaking `[[gates]]` topology (R3.3 splitting `tcb-audit` into language-specific scripts) or should the existing single script grow language switches?
- For R4.1 (`:runtime-via` annotation), is the right mechanism a Shen syntax extension or a sidecar `runtime-checks.toml`? Spec syntax has the benefit of staying inside the audit surface.
- Should `examples/multi-tenant-api/` graduate to demonstrating the cross-emitter story (Go HTTP + TS frontend hitting the same guards), or should that live in a fourth example to keep the JWT demo focused?
