# Cedar Policy Generation from Shen Specs (Post-2 Direction)

**Status**: Early scaffolding / direction note. The real working implementation of this pattern currently lives in the sibling `shen-rust` repository (`examples/shen-cedar-authz`).

This directory exists to capture the "shengen generating Cedar" story for the runtime backpressure follow-up post.

## The Idea

One of the three main ways we're exploring to get runtime backpressure from Shen specs:

1. **Single spec, compile + runtime** (current strength in `multi-tenant-api` via `:runtime-via`)
2. **Shen logic generates Cedar policies** (this direction)
3. **Embedded Shen runtime** (shen-rust + direct evaluation)

In this pattern:
- A Shen spec (or pure Shen functions, like the role DAG + transitive closure logic) is the source of truth.
- At build/generation time, the Shen engine evaluates the spec and produces a Cedar `PolicySet`.
- The generated Cedar is strict-validated against a schema.
- Compile-time guards (from the same or a related Shen spec) ensure that sensitive code paths **must** consult the resulting Cedar decision.
- At runtime, Cedar provides fast, production-grade authorization.

Benefits:
- Expressive policy authoring and analysis in Shen (recursive closures, hierarchy reasoning, etc.) that Cedar itself doesn't provide as cleanly.
- Cedar's verified evaluator + schema for the hot path.
- No translation layer between the "what the spec says" and the runtime artifact.
- The compile-time guard makes skipping the runtime policy check structurally difficult.

## Current Best Reference

The working demonstration is in the `shen-rust` repo:

- `examples/shen-cedar-authz/examples/generate.rs`
- `spec/authz.shen` (defines base grants + role inheritance DAG + `expand-all` that computes transitive closure)
- The served Shen VM evaluates the spec → host renders Cedar permits → strict schema validation → written as build artifact → enforced.

See the `generate` example output for the exact shape:
- Shen computes the closure.
- Cedar parses + `Validator::Strict` validates it.
- Enforcement examples (Admin inherits, Lead does not, etc.).

Also relevant:
- `verify` example: Shen reasons *about* Cedar policies (hierarchy-aware shadowing/overlap detection that Cedar's per-request evaluator doesn't do).
- `gate` example: Cedar as a gate in front of Shen evaluation.

## How This Composes with the Rest of Shen-Backpressure

- You can still have `shengen` lower parts of the same (or sibling) Shen spec into guard types in your host language.
- The generated Cedar becomes the runtime policy engine.
- The guard types ensure the Cedar decision is consulted.
- This gives you the "compile-time forces the runtime call" property that was discussed in the original HN thread.

## Next Steps (for this repo)

- [ ] Capture a clean, self-contained transcript of running the `generate` example from shen-rust.
- [ ] Create a minimal "what would the tenant-access policy spec look like" example here that could generate Cedar permits.
- [ ] Explore using the existing `shengen` machinery (or a new emitter) to help drive Cedar policy generation from sequent rules.
- [ ] Show the full loop: spec change → regenerated guards + regenerated Cedar policy → gates pass.

## Relationship to Other Work

- Complements the `:runtime-via` work (Way 1) in `multi-tenant-api`.
- The shen-rust port gives us a production-viable embedded Shen engine that can perform this generation (and later, live evaluation if desired).
- This is the natural answer to "what about OPA/Rego/Cedar?" questions while keeping the single-spec substrate.

---

This is the second major leg of the "runtime backpressure from Shen" story. The third (full embedding) is already partially demonstrated via shen-rust's served mode + the existing runtime-via evaluator host.

Real captured output (including both the `generate` and `verify` examples, with analysis) lives in `CAPTURED-OUTPUT.md`.

### The three shen-rust + Cedar patterns (as of this writing)

| Example | Direction                  | Post-2 Relevance |
|---------|----------------------------|------------------|
| `generate` | Cedar generated *from* Shen | Core "shengen generating Cedar" story |
| `verify`   | Shen reasons *about* Cedar  | Powerful static analysis layer over policies |
| `gate`     | Cedar gates Shen evaluation | Runtime enforcement front for Shen execution |

These three, combined with the compile-time guard types from the main Shen-Backpressure work, give a very rich set of surfaces for structural backpressure.