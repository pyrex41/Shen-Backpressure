---
date: 2026-04-07T00:00:00Z
researcher: reuben
git_commit: pre-wave-bend-feasibility
branch: main
repository: pyrex41/Shen-Backpressure
topic: "Feasibility study — Shen sequent calculus × Bend interaction nets"
tags: [research, feasibility, cross-project, bend, hvm2, linear-logic]
status: feasibility-brief
last_updated: 2026-04-16
last_updated_by: reuben
last_updated_note: "Moved from /research/shen-bend-feasibility-prompt.md during demo-readiness cleanup. This remains an open investigation brief referencing three codebases (Shen-Backpressure, bend, shen-cl)."
---

# Feasibility Study: Shen Sequent Calculus × Bend Interaction Nets

## Context

Three codebases are involved:

- **Shen-Backpressure** (`~/projects/Shen-Backpressure`): A formal verification framework that uses Shen's Turing-complete sequent calculus type system to generate compiler-enforced guard types. The codegen tool (`shengen`) parses Shen `(datatype ...)` blocks — which are sequent calculus inference rules — and emits opaque types in Go, TypeScript, Rust, and Python. A separate Shen runtime (`tc+`) verifies spec consistency. Rust backend: `cmd/shengen-rs/shengen.py`.

- **Bend** (`~/projects/bend`): A functional language that compiles to HVM2 interaction nets for automatic parallel execution on CPUs and GPUs. ~17k lines of Rust. Compilation pipeline: Bend source → 24 desugaring passes → `book_to_hvm()` → HVM2 interaction net → reduction. Key node types: Con (constructor), Dup (duplicator), Era (eraser), Opr (operation), Swi (switch). Currently has optional Hindley-Milner type checking but no proof checker or dependent types.

- **shen-cl** (`~/projects/shen-cl`): pyrex41's fork of the Common Lisp Shen implementation, upgraded to kernel 22.3 with HAMT-based Prolog bindings for the type checker. The sequent calculus engine lives in `kernel/klambda/sequent.kl`, compiled to `compiled/sequent.lsp`. Runs on SBCL. 24/24 kernel tests pass.

### The Mathematical Connection

Jean-Yves Girard's work on linear logic established a deep correspondence:
- **Sequent calculus** (Shen's type system) and **interaction nets** (Bend's runtime) are two representations of the same underlying mathematics
- Cut elimination in sequent calculus corresponds to interaction net reduction
- Proof nets (a graphical syntax for linear logic proofs) are interaction nets
- This is not an analogy — it's a formal correspondence (see Girard 1987, Lafont 1990)

---

## Investigation 1: Parallel Proof Checking via Interaction Nets

### Hypothesis

Shen type derivations (sequent calculus proof trees) can be compiled into HVM2 interaction nets and executed on Bend's parallel runtime, including the CUDA/GPU backend. This would enable massively parallel proof checking.

### Concrete Questions to Answer

1. **Can sequent calculus derivations be faithfully encoded as interaction nets?**
   - Examine Shen's sequent calculus engine: `~/projects/shen-cl/kernel/klambda/sequent.kl`
   - This file implements Shen's type checker using Prolog-style backtracking with unification
   - The key functions are `shen.sequent`, the rule application engine, and the unification/binding machinery
   - Map these operations to HVM2 node types: Can Prolog unification be expressed as interaction net reduction?
   - Specifically: Can backtracking search (Shen's `shen.th*` function) be represented as parallel redex exploration in an interaction net?

2. **What is the compilation path?**
   - Route A: Compile Shen's KLambda IR (`.kl` files) to Bend source, then to HVM2
   - Route B: Compile sequent calculus derivation trees directly to interaction nets, bypassing Bend's surface language
   - Route C: Write a Shen-to-HVM2 compiler that maps `(datatype ...)` proof obligations directly to nets
   - Evaluate which route preserves the parallelism structure best

3. **What's the parallelism payoff?**
   - Shen's type checker currently runs sequentially with backtracking
   - pyrex41 attempted OR-parallel type checking (commit `129c013` in shen-cl) using lparallel but reverted it — the overhead wasn't worth it on CPU
   - BUT: HVM2 on GPU handles thousands of parallel threads natively. Would the same OR-parallelism that failed on CPU succeed on GPU?
   - Profile `sequent.kl` on a real Shen spec (e.g., `~/projects/Shen-Backpressure/examples/dosage-calculator/specs/core.shen`) to estimate branching factor and parallelizable work

4. **What are the blockers?**
   - Shen's Prolog engine uses mutable bindings (now HAMT-based in pyrex41's fork — `src/overwrite.lsp` lines 100-180). HVM2 is purely functional. Can HAMT bindings map to Dup nodes?
   - Shen uses `(freeze ...)` for lazy evaluation. HVM2 has native laziness via interaction nets. Compatible?
   - Shen's `sequent.kl` relies on `shen.deref` chains for variable resolution. How does this map to net topology?

### Files to Read

- `~/projects/shen-cl/kernel/klambda/sequent.kl` — the sequent calculus engine
- `~/projects/shen-cl/kernel/klambda/prolog.kl` — Prolog unification (used by sequent)
- `~/projects/shen-cl/src/overwrite.lsp` — HAMT binding overrides (lines 100-180)
- `~/projects/bend/src/fun/term_to_net.rs` — how Bend terms become HVM2 nets
- `~/projects/bend/src/fun/net_to_term.rs` — readback from nets to terms
- `~/projects/bend/docs/compilation-and-readback.md` — compilation explanation

### Success Criteria

Demonstrate one of:
- A Shen `(datatype ...)` block whose proof obligations compile to a valid HVM2 net
- A Prolog unification step expressed as interaction net reduction
- A benchmark showing OR-parallel sequent derivation on HVM2 outperforming sequential Shen `tc+`

---

## Investigation 2: Shen as Linear Type Layer for Bend's Parallelism

### Hypothesis

Shen's sequent calculus can express linear logic typing judgments. Bend's automatic parallelism relies on structural independence in the interaction net, but currently lacks a formal linearity discipline beyond basic affine variable checking (`linearize_vars.rs`). Shen could provide a principled linear type layer that proves parallelism safety.

### Concrete Questions to Answer

1. **What linearity checking does Bend currently do?**
   - Read `~/projects/bend/src/fun/transform/linearize_vars.rs` — this handles variable usage (affine: used 0 or 1 times)
   - Read `~/projects/bend/src/fun/transform/linearize_matches.rs` — linearizes match bindings
   - What invariants does this enforce? What can slip through?
   - Specifically: can a Bend program create a sharing violation that the current linearization pass misses?

2. **Can Shen express linear logic typing rules?**
   - Shen's `(datatype ...)` blocks ARE sequent calculus rules. Sequent calculus is the home of linear logic
   - Write a Shen spec that expresses a linear typing judgment: "this resource is consumed exactly once"
   - Example target: express that a Bend `Dup` node correctly duplicates a term (both copies are independent)
   - Can Shen's `verified` premises express the key linear logic connectives: ⊗ (tensor), ⅋ (par), ! (of course), ? (why not)?

3. **How would Shen linear types integrate with Bend's compilation?**
   - Option A: Pre-compilation check — run Shen `tc+` on annotated Bend source before compilation
   - Option B: Use `shengen-rs` to generate Rust guard types that Bend's compiler must satisfy
   - Option C: Extend Bend's type checker (`~/projects/bend/src/fun/check/type_check.rs`) with Shen-derived linear type rules
   - Which option requires the least modification to Bend's pipeline?

4. **What would this catch that Bend currently misses?**
   - Construct a concrete example: a Bend program that compiles and runs but has a subtle sharing/parallelism bug
   - Show that a Shen linear type spec would reject this program
   - Or: show that Bend's existing linearization is already sufficient and this idea adds no value

### Files to Read

- `~/projects/bend/src/fun/transform/linearize_vars.rs`
- `~/projects/bend/src/fun/transform/linearize_matches.rs`
- `~/projects/bend/src/fun/check/type_check.rs` — current HM type checker
- `~/projects/bend/src/fun/mod.rs` — Term enum (lines 131-237), see variable usage tracking
- `~/projects/Shen-Backpressure/examples/multi-tenant-api/specs/core.shen` — proof chain example
- Any linear logic literature on sequent calculus encodings (Girard 1987, "Linear Logic")

### Success Criteria

Demonstrate one of:
- A Shen spec that encodes a linear typing judgment for a Bend program construct
- A concrete Bend program with a parallelism bug that Shen linear types would catch
- A proof that Bend's existing linearization is equivalent to linear typing (negative result — still valuable)

---

## Investigation 3: Mutual Verification Bootstrap

### Hypothesis

Shen and Bend can cross-verify each other: Shen's sequent calculus verifies properties of Bend's interaction net reduction rules, while Bend's (future) proof checker verifies Shen's type derivations. Neither system trusts itself — each trusts the other.

### Concrete Questions to Answer

1. **What is each system's Trusted Computing Base (TCB)?**
   - Shen's TCB: `sequent.kl` (sequent calculus engine) + `prolog.kl` (unification) + SBCL runtime
   - Bend's TCB: HVM2 reduction rules (external crate `hvm = "=2.0.22"`) + the Rust compiler
   - How many lines of code in each TCB? What are the known attack surfaces?

2. **Can Shen verify HVM2's reduction rules?**
   - HVM2 has specific interaction rules: annihilation (same-type nodes cancel), commutation (different-type nodes pass through each other)
   - These rules correspond to cut elimination steps in sequent calculus
   - Write Shen `(datatype ...)` specs that express the invariants each HVM2 reduction rule must preserve
   - Example: "after a Con-Con annihilation, the resulting net has the same denotational semantics as the original"
   - Can this be checked by `tc+`?

3. **Can Bend verify Shen's derivations?**
   - Bend doesn't have a proof checker yet (Bend2 will)
   - But: can we express "this sequent calculus derivation is valid" as a Bend program that type-checks?
   - The derivation tree is a data structure. Checking it means verifying each inference step
   - Write a Bend program that takes a serialized Shen derivation and checks it

4. **What does disagreement look like?**
   - If Shen says "this HVM2 reduction preserves semantics" but Bend's execution produces a different result → bug in HVM2
   - If Bend says "this Shen derivation is invalid" but Shen's `tc+` accepts it → bug in Shen's type checker
   - Design a test harness that runs both checks and reports disagreements

5. **Is this actually stronger than self-verification?**
   - The value proposition: two independent implementations of the same mathematics checking each other
   - But: if both implementations share a conceptual error (misunderstanding of the theory), mutual verification won't catch it
   - What class of bugs does mutual verification catch that self-testing doesn't?

### Files to Read

- `~/projects/shen-cl/kernel/klambda/sequent.kl` — Shen's proof engine
- `~/projects/shen-cl/kernel/klambda/prolog.kl` — unification
- `~/projects/bend/src/hvm/eta_reduce.rs` — an HVM2 optimization pass (verify it preserves semantics)
- `~/projects/bend/src/hvm/add_recursive_priority.rs` — another pass to verify
- HVM2 source (external): `https://github.com/HigherOrderCO/hvm` — the actual reduction rules
- `~/projects/Shen-Backpressure/sb/AGENT_PROMPT.md` — TCB audit methodology

### Success Criteria

Demonstrate one of:
- A Shen spec that encodes a correctness property of an HVM2 reduction rule
- A Bend program that validates a Shen type derivation
- A concrete example where the mutual check catches a bug that neither system catches alone

---

## Investigation 4: Shen as Specification Language for Bend2's Type Theory

### Hypothesis

Instead of Bend2 designing a proof language from scratch, it could use Shen's sequent calculus as its logical foundation. Shen already has a working, Turing-complete type system. Bend provides the execution substrate (parallel interaction net reduction). Division of labor: Shen says what's true, Bend proves it fast.

### Concrete Questions to Answer

1. **What does Bend2's type theory need?**
   - Bend currently has HM type inference (`~/projects/bend/src/fun/check/type_check.rs`)
   - Bend2 plans to add a "complete proof checker" — what properties must it verify?
   - At minimum: type safety, termination (for total functions), memory safety (no dangling refs in nets)
   - Does Shen's sequent calculus have the expressiveness to state these properties?

2. **Can shengen-rs generate guard types for Bend's internal Rust data structures?**
   - Bend's core data structure is `Term` (enum with ~30 variants, `~/projects/bend/src/fun/mod.rs` lines 131-237)
   - Bend's HVM representation uses `Tree` nodes (Con, Dup, Era, Opr, Swi)
   - Write Shen specs for Bend's internal invariants:
     - "A well-formed net has no dangling ports"
     - "Every Dup node has a unique label"
     - "After linearization, every variable is used exactly once"
   - Run `shengen-rs` to generate Rust guard types. Can Bend's compiler use them?

3. **What's the integration architecture?**
   - Option A: **Spec-first development** — Write Shen specs for Bend2's type rules, generate Rust guard types with `shengen-rs`, Bend2's compiler must construct these types (proving the invariant)
   - Option B: **Shen as meta-language** — Bend2's type rules are WRITTEN in Shen syntax, then compiled to Bend/HVM2 for execution. The rules themselves are Shen `(datatype ...)` blocks
   - Option C: **Shen-in-Bend** — Port Shen's sequent calculus engine to Bend source code, getting parallel proof checking "for free"
   - Evaluate engineering effort for each option

4. **What are the expressiveness gaps?**
   - Shen's type system is based on Horn clauses with unification (Prolog-derived)
   - Modern proof assistants (Lean, Coq) use dependent type theory (CIC/CoC)
   - What can dependent types express that Shen's sequent calculus cannot?
   - Conversely: Shen is Turing-complete (unlike Coq/Lean which require termination). Is this an advantage or a liability for Bend2?
   - Can Shen express inductive types? Coinductive types? Universe polymorphism?

5. **How does this compare to what HigherOrderCO might build from scratch?**
   - HigherOrderCO's research is rooted in interaction combinators and optimal reduction
   - Their natural choice might be a type theory based on linear logic / proof nets (matching HVM2's foundation)
   - Shen's sequent calculus IS linear-logic-adjacent. But it's also Prolog-based, which adds non-determinism
   - Would Shen be a natural fit or a conceptual mismatch?

### Files to Read

- `~/projects/bend/src/fun/mod.rs` — Term enum, Book struct, core data types
- `~/projects/bend/src/fun/check/type_check.rs` — current type checker (what exists)
- `~/projects/bend/src/fun/term_to_net.rs` — how terms become nets (invariants to preserve)
- `~/projects/Shen-Backpressure/cmd/shengen-rs/shengen.py` — Rust codegen (what it can generate)
- `~/projects/Shen-Backpressure/examples/dosage-calculator/specs/core.shen` — complex spec with recursive helpers
- `~/projects/Shen-Backpressure/sb/commands/create-shengen.md` — full shengen algorithm

### Success Criteria

Demonstrate one of:
- Shen specs for 3+ Bend internal invariants, with generated Rust guard types that compile
- A side-by-side comparison: same property expressed in Shen vs. in dependent type theory
- A prototype of Bend type rules written as Shen `(datatype ...)` blocks
- An honest assessment of where Shen falls short for this use case

---

## Methodology

For each investigation:

1. **Read first, speculate second.** Read every file listed before forming conclusions. The mathematical correspondence is real, but engineering feasibility depends on implementation details.

2. **Build small proofs of concept.** Don't design a complete system — write the smallest possible artifact that demonstrates feasibility or infeasibility.

3. **Be honest about negative results.** "This doesn't work because X" is as valuable as "this works." The goal is feasibility assessment, not advocacy.

4. **Track the TCB.** For every proposed integration, identify what must be trusted and what is verified. A verification approach that enlarges the TCB more than it verifies is net negative.

5. **Consider HigherOrderCO's perspective.** They're building Bend2 with specific design goals. Any Shen integration must be something they'd actually want, not something imposed from outside.

## Key References

- Girard, J-Y. (1987). "Linear Logic." *Theoretical Computer Science*, 50(1), 1-102.
- Lafont, Y. (1990). "Interaction Nets." *POPL '90*.
- Lafont, Y. (1995). "From Proof-Nets to Interaction Nets." *Advances in Linear Logic*.
- Shen language specification: https://shenlanguage.org
- HVM2 paper/docs: https://github.com/HigherOrderCO/hvm
- Tarau, P. (2009). "Compact Serialization of Prolog Terms" (relevant to Investigation 3 — serializing derivations)
