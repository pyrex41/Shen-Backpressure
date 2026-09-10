# Cedar Generation Plan (for Post 2)

## Short term (this repo)
- [x] Create scaffolding directory + README explaining the concept
- [x] Add stub spec showing the intended shape
- [ ] Capture real output from shen-rust's generate example (run it and save transcript)
- [ ] Write a short comparison: "what the multi-tenant membership check looks like as a Cedar policy vs as a :runtime-via checker"

## Medium term
- Explore whether we can drive Cedar policy emission from sequent-calculus rules (not just pure functions)
- See if parts of the existing shengen lowering logic can be reused or adapted for policy generation
- Show a combined demo: one Shen theory produces both guard types *and* a Cedar policy set

## Long term / aspirational
- A single high-level Shen theory that can be projected to:
  - Compile-time guards (Go/TS/etc.)
  - Runtime policy (Cedar)
  - Live embedded evaluation (shen-rust)
  - Test oracles (shen-derive)

This would be the strongest form of "one spec, many enforcement surfaces" for AI coding loops.

## Open Questions
- How much of the policy generation should be "pure Shen functions" vs "sequent rules with :runtime-via style lowering"?
- What is the right boundary between what lives in the generated Cedar vs what is still enforced structurally by guards?
- Performance: when do we want Cedar (fast path) vs direct Shen evaluation?