---
date: 2026-09-22T00:00:00Z
researcher: claude
git_commit: 6b9dde0
branch: claude/shen-backpressure-robustness-w5f5q6
repository: pyrex41/Shen-Backpressure
topic: "Verifier-throughput roadmap: binding, path-completeness, flow, falsification, certificates"
tags: [plan, roadmap, shengen, shen-derive, scip, smt, gdp, pcc, sb-engine]
status: proposed
last_updated: 2026-09-22
last_updated_by: claude
---

# Verifier-throughput roadmap

## Thesis

The Ralph loop is counterexample-guided inductive synthesis (CEGIS)
with an LLM as the synthesizer. In that setting the loop's progress
per iteration is bounded by the information the verifier returns,
not by the proposer's intelligence. Every workstream below either
makes the verifier return more bits per gate run, or brings a new
class of property under a verifier at all.

Six workstreams, ordered by value divided by risk. W6 was not in the
original five; it was added once it became clear that all five had
recorded the same blocking gap — no Shen host — and that several of
their claims had therefore never been executed at all.

| # | Workstream | Theory | Closes |
|---|-----------|--------|--------|
| W1 | Proof binding (GDP brands) | Ghosts of Departed Proofs; Curry-Howard | Zero-value forgery, unpaired proof forgery |
| W2 | Path-complete spec sampling + vacuity | Symbolic execution, SMT, vacuity checking | "sampled" evidence upgraded to "every spec path"; uninhabited rules |
| W3 | Flow premises over SCIP facts | Noninterference (VSI), Datalog code analysis | Grep gates; "every handler asks for a proof" |
| W4 | Falsifier + gate strength | Incorrectness logic, mutation analysis, fuzz corpora | Unmeasured gate strength; manual bypass attempts |
| W5 | Certificates | Proof-carrying code, gradual verification with blame | Report is not independently checkable; failures do not assign blame |
| W6 | shen-go as the Shen host | Differential testing; two oracles for one spec | Gate 4 red everywhere; W3's Prolog engine never executed; W5's `lowering` blame unreachable |

Each workstream ships behind config so existing examples keep passing
until they opt in. Every workstream ends with the payment and
multi-tenant examples regenerated and their committed audit reports
refreshed. That is the acceptance test for the whole plan: the
reports must get strictly stronger and remain readable.

## Ground truth this plan rests on

Verified in the tree at the commit above:

- `examples/payment/internal/shenguard/guards_gen.go:74-86`.
  `shenguard.BalanceChecked{}` is a legal expression from any
  package. Unexported fields prevent naming fields, not building the
  zero value. The error path also returns `BalanceChecked{}, err`, so
  a dropped error yields a forged proof.
- `guards_gen.go:120-125`. `NewSafeTransfer(tx, check)` never checks
  that `check.tx == tx`. This is the same class as the token-A user-B
  bug closed by W2.1 in the multi-tenant example, and the fix there
  was per-spec, not structural.
- `examples/multi-tenant-api/bypass_attempts/01_direct_struct_literal.go.bak`
  tests a literal with named fields, which the compiler rejects. It
  does not test the empty literal, which the compiler accepts.
- `examples/multi-tenant-api/bin/shenguard-audit.sh:58` enforces
  caller discipline with a regex over source text. An aliased import
  or a method value evades it.
- `shen-derive/verify/harness.go:37-45`. Behavioral evidence is a
  deterministic boundary pool plus optional seeded random draws.
- `shen-derive/core/eval.go:118`. A concrete evaluator for the
  `(define …)` subset already exists over `core.Sexpr`. W2 builds a
  symbolic evaluator over the same AST.
- `cmd/sb/discharge.go:96`. `discharge_basis` is a free string and the
  schema memo reserves `prover-z3`, `flow-analysis`,
  `runtime-assertion`, and `prover`. W2, W3, and W5 fill those slots
  without a schema bump.
- `cmd/sb/config.go:17-18`. Gate kinds are `command` and `derive`.
  W3 and W4 add kinds; the fixed five-gate shape stays untouched.

---

## W1. Proof binding: GDP brands in shengen

### Status (W1 implemented, default flip deferred)

Steps 1–5 and 7's documentation are done; `--brands` is **opt-in** and
the default is unchanged.

| Step | State |
|------|-------|
| 1. `brand_inference.go` + golden tables | done |
| 2. Go emitter behind `--brands`; payment regenerated | done |
| 3. Bypass attempts 06, 07; multi-tenant regenerated | done |
| 4. shengen-ts + tests | done; shen-web-tools NOT migrated (see gaps) |
| 5. `tcb-audit` rejects a missing witness | done (`bin/shenguard-audit.sh --brands`) |
| 6. Make `--brands` the default | **DEFERRED** |
| 7. TRUST-MODEL section, examples regenerated | done except the audit reports (see gaps) |

**Step 6 is deferred deliberately.** Flipping the default changes the
generated output of every project that has not threaded brands through
its own signatures, which is a breaking change for consumers outside
this repo, and the migration is not mechanical: a value that crosses
`context.Value`, a `JSON.parse`, or any other type-erasing boundary
needs a concrete brand chosen by hand (see
`examples/multi-tenant-api/internal/apibrand`), and a constructor whose
premise is a Shen sum type needs explicit instantiation because Go
infers nothing from an interface-typed argument. Flip it after W4's
forgery gate exists, so the flip is protected by a measured corpus
rather than by two hand-written attempts.

Gaps recorded at implementation time:

- `examples/shen-web-tools` is not migrated to `--brands`. Its 762-line
  generated module is consumed by six hand-written TypeScript modules
  with their own tests, and the environment has no `node_modules` for
  it. `examples/payment/reference/guards_gen_branded.ts` is the
  committed TypeScript reference instead.
- Python and Rust reference emitters are untouched; the plan's `NewType`
  and `PhantomData` lowering is not implemented.
- ~~The committed `transcript/discharge_report.json` and
  `transcript/audit_report.md` are not refreshed, so payment's report
  does not yet carry a `guard-brand-bound` discharge basis.~~
  **Closed by W5.4.** The basis token is defined in
  `shen-derive/report/schema.go`, `shengen --brand-table` exports the
  inference, and both examples' transcripts are refreshed. Payment's
  report carries `guard-brand-bound` on `safe-transfer`'s two premises
  and on `balance-invariant`'s transaction premise; multi-tenant's
  carries it across the whole authorization chain.
- shen-derive does not run the brand inference, so its generated
  harness writes unparameterized type names. payment routes the derive
  gate through `internal/guardcompat`, which pins one shared brand; the
  package documents what that gives up.

### Goal

Make "this proof belongs to that value" a type error in every
generated language, without the spec author writing anything new.

### Design

Every wrapper datatype gets a phantom brand parameter. The
constructor mints a fresh brand. A conclusion that mentions a
premise variable in two positions (as in `safe-transfer`, where `Tx`
appears both directly and inside `Check`) is lowered so both
positions must carry the same brand.

Shengen already knows the sharing structure: `SymbolTable.Build`
(`cmd/shengen/main.go:135`) records fields per conclusion, and the
verified-premise resolver walks `(head Tx)` paths. The brand
inference rule is:

1. Each premise variable of wrapper type introduces a brand variable.
2. If two premises are related by a verified premise or by a nested
   field path, their brand variables unify.
3. The conclusion type is parameterized by the brands of its
   constituents that survive unification.

Go lowering (Go 1.18+ generics, phantom parameter, private witness
method to defeat zero values):

```go
type brand interface{ brand() }
type Transaction[B brand] struct { valid witness; amount Amount; from, to AccountId }
type BalanceChecked[B brand] struct { valid witness; bal float64; tx Transaction[B] }
func NewSafeTransfer[B brand](tx Transaction[B], check BalanceChecked[B]) SafeTransfer[B]
```

The `witness` type is unexported with an unexported field and a
method; every accessor and every consuming constructor calls
`v.valid.mustBeMinted()`, which panics on the zero value. The panic
is the runtime half; the compiler half is that the empty literal
`BalanceChecked[X]{}` now needs a brand argument the caller cannot
name, because brands are unexported types minted inside the package.

TypeScript lowering is direct: `declare const brand: unique symbol`
per constructor call site via a generic `Brand<T, B>` with `B extends
symbol`, and the constructor returns `Brand<Tx, typeof fresh>`.
Python and Rust reference emitters get `NewType` and a phantom
`PhantomData<B>` respectively; Rust additionally gets `#[must_use]`
and no `Default`.

### Failed-constructor return

Constructors that fail must not return a zero value alongside the
error. Go: return `*T` or `(T, bool)`; decision below picks the
zero-value-with-witness route so call sites do not change shape.
The witness panics if the zero value is ever read, which converts a
silent forgery into a loud crash at first use.

### Steps

1. Add `brand_inference.go` to `cmd/shengen`: compute brand variables
   and unification from `[]Datatype`. Unit-test on payment and
   multi-tenant specs; golden-test the inferred brand table.
2. Emit generics in the Go emitter behind `--brands` flag. Regenerate
   payment. Fix the example's hand-written code to thread brand
   parameters (mostly type inference does it).
3. Add bypass attempts `06_empty_literal.go.bak` and
   `07_unpaired_proof.go.bak` to multi-tenant. Both must compile
   before this workstream and fail to compile after.
4. Same for shengen-ts and the shen-web-tools example.
5. Extend `tcb-audit` allowlist checks to reject any generated file
   that lacks the witness field on a wrapper type.
6. Make `--brands` the default; keep `--no-brands` for one release.
   **Deferred — see Status above.**
7. Regenerate all examples; refresh audit reports; add a section to
   `docs/TRUST-MODEL.md` naming the witness panic as a runtime
   member of the TCB.

### Acceptance

- `go vet ./...` and `go build ./...` clean on every example.
- Bypass attempts 06 and 07 fail to compile.
- Payment `discharge_report.json` gains a `guard-brand-bound`
  discharge basis on `safe-transfer` premises, with `code_references`
  pointing at the generic signature.

### Risk

Go generics and method sets: methods on a generic type cannot
themselves be generic, and interface satisfaction with phantom
parameters needs care. Prototype on payment before touching the
emitter's general path. Estimated 2 to 3 weeks.

---

## W2. Path-complete spec sampling and vacuity

### Goal

Upgrade shen-derive's evidence from "boundary pool plus random draws"
to "one concrete input per feasible path of the spec, plus the
boundary pool," and flag uninhabited datatypes.

### Design

A symbolic evaluator over `core.Sexpr`, sibling to
`core.Eval`. It walks a `(define …)` body with symbolic arguments,
collecting a path condition at every `if`, `cond`, guard clause, and
pattern-match fallthrough. Recursion over lists is unrolled to a
configurable depth (default 4), which is the standard bounded
approach and matches the boundary pool's list lengths.

Path conditions go to an SMT solver. Decision: use Z3 through its
`smt-lib2` text interface over a subprocess, not the Go binding, so
the dependency is a binary on `PATH` and the gate degrades to the
existing sampler when absent. Theories needed for the current spec
subset: linear integer and real arithmetic, strings with length,
algebraic datatypes for lists. All are in Z3's core.

Each SAT path yields a model, which is decoded into a shen-derive
sample and appended to the pool with a provenance tag
`path:<n>`. UNSAT paths are reported as dead spec branches, which is
useful backpressure on the spec author.

Vacuity: for each `(datatype …)` with verified premises, ask the
solver whether the conjunction of premises is satisfiable over the
premise types. UNSAT means the datatype has no values. The rule gets
status `vacuous` in the discharge report and the gate fails, since
an uninhabited guard makes every downstream claim empty.

### Steps

1. `shen-derive/symbolic/`: path enumerator with a tiny constraint
   AST; printer to smt-lib2; Z3 driver with timeout; model decoder.
2. Wire into `verify.Harness` as a third sample source with its own
   provenance column in the generated test file header.
3. Discharge classification: when path sampling ran, emit
   `discharge_basis: prover-z3-path-cover` and record
   `paths_total`, `paths_feasible`, `paths_dead`. Schema memo already
   reserves the token; add the three counters as optional fields
   (v1.x additive change).
4. Vacuity pass in `shen-derive` at spec load; new premise status
   `vacuous`. Test with a deliberately contradictory datatype fixture.
5. Equivalence in the other direction, Go only, as a stretch: compile
   the impl function's branches to path conditions via
   `golang.org/x/tools/go/ssa` and ask Z3 for an input where spec and
   impl disagree. Bounded, pure functions over numbers and lists
   only. Ship behind `--differential-smt`.
6. Run on payment `processable` and every `(define …)` in the
   examples; commit the enlarged spec tests.

### Acceptance

- Every spec path in payment's `processable` is covered by at least
  one committed sample, visible in the test file header.
- A fixture with an unsatisfiable datatype fails `sb derive` with a
  `vacuous` rule and a readable message.
- Gate runtime on payment stays under 5 seconds with Z3 present.

### Risk

Shen's `(define …)` subset that shen-derive accepts is small, so the
symbolic evaluator is bounded work. The Go-side differential step is
the open-ended part; it is explicitly a stretch. Estimated 3 weeks
without step 5.

---

## W3. Flow premises: Shen over SCIP facts

### Goal

Replace every grep gate with a discharged premise, and make "every
handler obtains a verified value before reaching a sink" a checkable,
language-independent property.

### Design

Three layers.

Facts. A new `sb index` step runs the language's SCIP indexer
(`scip-go`, `scip-typescript`) and converts `index.scip` into a flat
fact file: `def(sym, file, range)`, `ref(sym, file, range,
enclosing_def)`, `call(caller_sym, callee_sym)`. Enclosing definition
is computed by range containment against definition
`enclosing_range`. Facts are emitted as Shen s-expressions, so the
next layer needs no parser.

Rules. Shen has a Prolog engine in its kernel (`defprolog`). Flow
rules are written as Prolog over the facts. A small standard library
ships with sb:

```shen
(defprolog reaches
  X X <--;
  X Z <-- (call X Y) (reaches Y Z);)

(defprolog unsanctioned-caller
  Sym Caller <-- (ref Sym _ _ Caller) (not (allowed-caller Sym Caller));)
```

Spec authors declare flow premises in `core.shen` alongside datatypes:

```shen
(flow tenant-access-discipline
  (constructor-only shenguard.NewTenantAccess verified.CheckTenantAccess)
  (must-pass-through http.Handler verified.TenantAccess db.Exec))
```

`constructor-only` lowers to `unsanctioned-caller`. `must-pass-through
Src Proof Sink` lowers to: for every definition that is a `Src`, every
path in `reaches` to `Sink` contains a reference to `Proof`. That is
the noninterference statement with constructors as declassifiers.

Evaluation. `sb gates` runs a new gate kind `flow` that loads facts
plus rules into the Shen host already used for `:runtime-via :eval`
(profile B) and asks for all violations. Each violation becomes a
discharge-report counterexample with `file:line` for the offending
reference and the shortest violating path.

Fallback. When no SCIP indexer is on `PATH`, the gate runs the
legacy grep with a warning and marks the premise `unproven` with
basis `grep-fallback`, so the report is honest about what ran.

### Steps

1. `cmd/sb/index.go`: detect language, run indexer, parse
   `index.scip` with `github.com/sourcegraph/scip/bindings/go/scip`,
   emit `.sb/facts.shen`. Golden-test on multi-tenant.
2. Prolog standard library in `sb/flow/stdlib.shen`; unit tests run
   in the Shen host against hand-written fact fixtures.
3. Parse `(flow …)` forms in shengen and shen-derive spec loaders
   (they are ignored by codegen; only sb consumes them).
4. Gate kind `flow` in `cmd/sb/gates_run.go`; discharge basis
   `flow-analysis`.
5. Port multi-tenant's grep gate to a `(flow …)` premise. Add bypass
   attempt `08_aliased_import.go.bak` which must pass the old grep
   and fail the new gate. Attempt 04 (handler skips check) must now
   be caught by `must-pass-through` rather than by review.
6. Port shen-web-tools with `scip-typescript`.
7. TRUST-MODEL section: the indexer is now in the TCB for flow
   premises; say so, and note that SCIP indexers run after the
   language type checker so aliasing is resolved.

### Acceptance

- Multi-tenant audit report shows the two discipline premises as
  `discharged` with `flow-analysis` basis and cites resolved
  locations.
- Attempt 08 caught. Attempt 04 caught by the flow gate.
- Same rule text runs unchanged against the TypeScript example.

### Risk

Indexing time per iteration. `scip-go` on the examples is seconds,
but budget it: cache by tree hash in `.sb/`, and run the gate in a
parallel group. Prolog performance in Shen on a few thousand facts
is fine; on a large monorepo it will not be, and the escape hatch is
to emit Soufflé instead of Shen Prolog from the same rule text.
Estimated 4 weeks.

---

## W4. Falsifier agent and gate strength

### Status (W4 implemented; TypeScript operators deferred)

Steps 1–5 are done. The forgery gate, `sb mutate`, and
`sb loop --falsify` all ship; the TypeScript half of step 1 is
deliberately not implemented.

| Step | State |
|------|-------|
| 1. `cmd/sb/mutate.go`, five operators over `go/ast` | done |
| 1b. TypeScript operators via the compiler API | **DEFERRED** (see gaps) |
| 2. `evidence.mutation_score` + audit-report "Gate strength" | done |
| 3. Gate kind `forgery`; `bypass_attempts/` migrated | done |
| 4. `sb loop --falsify` + `sb/FALSIFIER_PROMPT.md` | done |
| 5. Run on both examples; publish the first kill rate | done |

**First published kill rates.**

| Example | Spec / impl | Score | Mutants | Forgery corpus |
|---|---|---|---|---|
| payment | `processable` / `Processable` | 100% | 3 caught, 0 survived, 0 equivalent, 0 invalid | 3/3 as declared, 0 succeeding |
| multi-tenant-api | `same-user?` / `SameUser` | 100% | 2 caught, 0 survived, 0 equivalent, 0 invalid | 9/9 as declared, 1 succeeding |

Both implementations are a handful of lines, so the operator set has
few sites to act on and a small mutant count is a weak measurement even
at a perfect rate. The value of publishing it now is the baseline, and
the number worth watching is what happens to it as the implementations
grow. There were no survivors, so no samples were added and the
`[derive.mutation] equivalent` list stays empty in both examples.

**What the forgery gate verifies end to end**, which is the part W3
left to review. Every expectation in the closed set is exercised by at
least one corpus entry, and all of them were measured, not asserted:

| Expectation | Entry | Was previously checked by |
|---|---|---|
| `compile-error` | multi-tenant 01, 06, 07 | a doc comment |
| `runtime-error` | multi-tenant 02 | a doc comment |
| `runtime-panic` | multi-tenant 09 (new) | nothing — attempt 06 stops at the compiler |
| `flow-violation` | multi-tenant 04, 05 | **review only** (W3 step 5 gap) |
| `grep-miss-flow-catch` | multi-tenant 08 | **review only** (W3 step 5 gap) |
| `derive-catch` | payment bug1, bug2, bug3 | `demo-shen-derive/run.sh`, run by hand |
| `succeeds (documented TCB limit)` | multi-tenant 03 | a doc comment |

Two corpus repairs fell out of standing the claims up. Attempt 08 had
stopped compiling under W1 — it names generic guard types without a
brand — so the gate it was written to demonstrate had never actually
run against it; it now threads a brand and verifiably passes
`bin/shenguard-audit.sh --grep-only` while failing `sb flow`. Attempt
03 claimed `unsafe` fully defeats the structural guarantee but did not
mint the W1 witness, so the forgery it built would in fact have panicked
on first read; it now writes the witness bool through `unsafe` too,
which is the honest version of that limit.

Gaps recorded at implementation time:

- **TypeScript operators are not implemented.** `cmd/shengen-ts` does
  carry `typescript` in `devDependencies`, which is the condition the
  plan set — but no example has a Go-side `[[derive.specs]]` entry in
  TypeScript to measure against, and the environment has no
  `node_modules` for `examples/shen-web-tools`, so a second
  implementation of the operator set could not be run even once. An
  unverifiable implementation is worth less than a recorded gap.
  `sb mutate` makes the gap loud rather than silent: a spec whose
  `lang` is not `go` produces a `gaps` entry in the report and a `GAP:`
  line on stderr instead of being skipped.
- **The falsifier phase is wired but has never been run against a live
  model** in this environment. Prompt hydration, corpus ingestion and
  the sample append are unit-tested with fixtures, and the sample
  source is verified end to end on payment with a hand-written entry
  (decoded through the composite `transaction`, evaluated by the spec to
  `true`, emitted with its provenance and note). What is untested is
  whether a model given this prompt finds anything.
- **`sb mutate` is not wired into `sb gates`.** A survivor is news, not
  a build break — the right response is usually a new sample rather
  than a revert — and a per-mutant test run is too slow for every
  iteration. It is run on demand and its result is carried in the
  report.
- The `guard-brand-bound` discharge basis from W1's acceptance
  criteria is still absent. Refreshing the transcripts here did not
  add it: the W1 status note defers it pending a schema-level
  decision, and a transcript refresh is the wrong place to take one.
- Mutants are generated per-file and evaluated one at a time with no
  parallelism. Fine at 3 and 2 mutants; it will need attention before
  it is fine at 300.

### Goal

Turn `bypass_attempts/` from a hand-written demo into a measured,
growing corpus, and publish a kill rate for the behavioral gate.

### Design

Two measurements and one agent.

Mutation score. A `sb mutate` command applies a fixed operator set to
the implementation package named in `[[derive.specs]]`: flip
comparison operators, off-by-one on numeric literals, drop a
conjunct, swap list head and tail, return the zero value. For each
mutant it runs only the derive tests and records caught or survived.
Survivors are the bits the sampler is not returning, and each one is
a concrete prompt for W2's path sampler and for the spec author. The
score goes into the discharge report as
`evidence.mutation_score` with the operator breakdown.

Forgery corpus. A `forgeries/` directory per example with the same
`.go.bak` convention as `bypass_attempts/`, but run automatically by
a gate kind `forgery` that expects every file to fail to compile or
to panic on the witness. A file that succeeds is a gate failure. The
existing attempts move here, plus W1's 06 and 07 and W3's 08.

Falsifier. An optional second loop in `sb loop --falsify` that runs
after the main iteration passes. Its prompt is the inverse of the
main one: given the spec, the generated guards, and the current
forgery corpus, write one new program that obtains a guard value
without the constructor, or one input where spec and impl disagree.
Anything it finds is appended to `forgeries/` or the sample pool and
becomes permanent. The theory is incorrectness logic: the falsifier
under-approximates, proving bugs present; the main loop
over-approximates, proving them absent. Both are needed to know the
gate's strength.

### Steps

1. `cmd/sb/mutate.go` with the operator set above over `go/ast`;
   TypeScript via the compiler API in `cmd/shengen-ts`.
2. Report fields and an audit-report section "Gate strength".
3. Gate kind `forgery`; migrate `bypass_attempts/`.
4. `sb loop --falsify` with its own prompt template in `sb/`.
5. Run the falsifier for a fixed budget on both examples and commit
   what it finds. The commit message is the first published kill
   rate.

### Acceptance

- Payment mutation score reported; any survivor is either fixed by a
  new sample or documented as an equivalent mutant.
- `forgery` gate green on all examples; a hand-inserted successful
  forgery turns it red.

### Risk

Equivalent mutants inflate false survivors; keep the operator set
small and let the author mark equivalents in `sb.toml`. Estimated 3
weeks.

---

## W5. Certificates: proof-carrying reports with blame

### Status (W5 implemented)

All five steps are done. `schema_version` stays 1; every field added is
additive and `omitempty`, and a report carrying none of them marshals
byte-identically to a pre-W5 one (pinned by
`TestW5FieldsAreOmittedWhenAbsent`).

| Step | State | Notes |
|------|-------|-------|
| 1. Reproducible build settings; `toolchain` block | done | `-trimpath -buildvcs=false` everywhere, `toolchain go1.24.7` pinned in all three `go.mod`s, header nondeterminism fixed |
| 2. `sb verify-report` | done | 8 checks; `UNVERIFIED` is distinct from `FAIL`; both tamper tests fail naming the premise |
| 3. Signature via ed25519 key file; `--cosign` mode | done | cosign is a subprocess, not a dependency; `sb-canonical-json-v1` documented |
| 4. Precision + blame + `guard-brand-bound` | done | `shengen --brand-table` added; payment and multi-tenant reports carry all three |
| 5. Audit-report renderer + `sb context` blame-first | done | precision column, blame in counter-examples, toolchain + signature sections, verification recipe |
| 6. Examples refreshed; verified from a clean checkout | done | see the table below |

#### Acceptance results

Verified from a `git archive HEAD` export at `/tmp/clean2` — no `.sb/`
directory, no git metadata, nothing a previous run left behind:

| Example | `sb verify-report --in transcript/discharge_report.json` | Exit |
|---|---|---|
| `examples/payment` | 7 PASS, 1 SKIP (no flow premises), 0 FAIL — including `static discharges` re-deriving `guards_gen.go` byte-identically and `path cover` agreeing with the committed test header | 0 |
| `examples/multi-tenant-api` | 7 PASS, 1 SKIP (no path-cover premises), 0 FAIL — including 3 flow premises re-evaluated clean from a **freshly built** scip-go index | 0 |

Both runs report `toolchain: re-derived with the same tools the report
records`, which is only true because `-buildvcs=false` landed: with
`-trimpath` alone the emitter hashed differently inside and outside a
git checkout, and the recorded hash was a timestamp rather than an
identifier. `shengen` now builds to
`4a7d38acf282002e50cc85431a73e838edb0e659514e6a26943b85858b106088`
from either tree.

Tamper tests (the plan's second acceptance criterion):

| Tamper | Result |
|---|---|
| one byte in `internal/shenguard/guards_gen.go` (`struct {` → `struct  {`) | `FAIL static discharges`, first difference located at line 54, and all 12 static premises listed by ID as having lost their basis |
| one committed sample flipped (`case_00` `want: true` → `false`) | `FAIL sampled evidence`, with the failing case echoed and `processable.oracle-spec-equiv` named |

New in the reports:

- payment — `guard-brand-bound` on `safe-transfer`'s two premises and
  on `balance-invariant`'s transaction premise, satisfying **W1's**
  outstanding acceptance criterion, which had been blocked on "a
  schema-level decision about the new basis token";
- multi-tenant — `guard-brand-bound` across the whole authorization
  chain (6 premises);
- both — `precision` on every premise, a `toolchain` block, and a
  "How to Verify This Report" section that is literally the command.

Gaps recorded at implementation time:

- ~~**Blame is `evaluator-only` everywhere**, because no Shen host is
  installed. The `lowering` blame value is implemented and unit-tested
  but cannot be produced in this environment: distinguishing a
  lowering bug from an implementation bug requires a second oracle.
  Wiring a host makes `AssignBlame` return `evaluator-and-host` (or
  `lowering`) with no further code change.~~
  **Closed by W6.D**, though the last sentence was wrong on both
  counts. `detectShenRuntimeHost` was asking about the spec's
  `:runtime-via` markers rather than about a host, and
  `evaluator-and-host` was a label rather than a measurement: nothing
  consulted a second oracle, so the basis promised something the code
  never did. W6 makes the promise good — each failing case is
  re-evaluated on the host and compared — and the `lowering` path now
  fires against a live host in
  `TestSecondOracleDisagreesAndBlamesLowering`.
- **Path-cover re-derivation compares counters rather than re-running
  the solver.** `sb derive` already diffs the committed test file
  against a fresh regeneration, so re-solving inside `verify-report`
  would duplicate that work; what it adds is the check that the
  report and the committed test file are the same document. A z3 that
  disagreed about *feasibility* while the test file was unchanged
  would not be caught here — it would be caught by the derive gate.
- **`--cosign` is implemented but untested end-to-end**: there is no
  cosign binary and no OIDC identity in this environment. The
  key-file path is covered by `TestSignAndVerifyRoundTrip`, including
  the tamper and the re-indent cases.
- **`shengen-ts` emits no brand table**, so a TypeScript project gets
  precision and blame but not `guard-brand-bound`. The
  `shengen_ts_version` field in the toolchain block is reserved and
  currently unpopulated.
- The **TypeScript derive path writes no discharge report** at all
  (pre-existing), so `examples/shen-web-tools` has nothing for
  `verify-report` to check.

### Goal

Make the discharge report independently checkable without re-running
the agent, and make failures name a responsible party.

### Design

Reproducibility. `sb gen` output must be a pure function of spec
hash and shengen version. Add `-trimpath`, pin the Go toolchain in
`go.mod`, and record the shengen binary hash in the report. A `sb
verify-report` command re-derives every `static` discharge from the
committed guards file and spec alone, and re-runs the committed
sample tests. It never calls a model. This is Necula's asymmetry:
expensive to produce, cheap to check.

Signing. Fill the reserved `signature` field with a detached
signature over the canonical JSON, using `sigstore/cosign` keyless by
default so CI identity is the signer. `sb verify-report --require-sig`
refuses unsigned reports.

Blame. Extend each premise with `precision` in `{static, path-cover,
sampled, runtime, unproven}` (a total order) and each counterexample
with `blame` in `{spec, impl, wrapper, lowering}`. Assignment rules:
a disagreement on a path-cover sample where the spec's own evaluator
and the Shen host agree blames `impl`; where they disagree blames
`lowering`; a `:runtime-via` failure blames `wrapper`; a vacuous rule
blames `spec`. `sb context` leads its summary with the blamed party,
which is the single most useful bit for the next prompt.

### Steps

1. Reproducible build settings; report gains `toolchain` block.
2. `sb verify-report`; run it in CI on every example's committed
   report.
3. Signature field via cosign; document the trust boundary.
4. Precision and blame fields (v1.x additive); rules in
   `shen-derive/report/classify.go`.
5. Audit-report renderer: precision column, blame in counterexample
   tables, and a verification recipe section that is literally the
   `sb verify-report` command.

### Acceptance

- A fresh clone can run `sb verify-report` on each example and get
  green without network access or a model.
- Tampering with one byte of a guards file makes `verify-report`
  fail with the premise that lost its basis.

### Risk

Reproducibility across Go minor versions is real but manageable with
a pinned toolchain. Estimated 2 weeks.

---

## W6. shen-go as the Shen host

### Status (W6 implemented)

Every workstream W1–W5 recorded the same gap in different words: *no
Shen host in this container*. Gate 4 was red in every example, the W3
Prolog rules had never executed, and W5's `lowering` blame value was
unreachable. W6 installs a host — the Go port, which builds from one
`make` target on a machine that already has Go — and then finds out
what four workstreams of unexecuted claims were hiding.

| Step | State |
|------|-------|
| A. One host resolver; `make shen-go`; corrected `eval -q` flag form | done |
| B. Gate 4 green on both examples, with a typed intrinsic prelude | done |
| C. W3's Shen Prolog engine executed; `sb flow --engine go\|shen\|both` | done |
| D. W5's second oracle; the `lowering` blame path fired | done |
| E. `sb verify-report` re-runs tc+ through the host | done |
| F. Both examples' transcripts refreshed | done |
| G. Docs | done |

#### What was actually broken

Nothing here was a missing feature. Every item was a claim the
documentation made that no execution had ever tested.

| Claim | What running it showed |
|---|---|
| "gate 4 runs `tc +` on the spec" | The invocation was written `shen -q -e '(tc +)' -l spec`. `-q` is a flag of the `eval` subcommand; no Shen port accepts that form. The gate would have failed with a host installed. |
| "gate 4 passes on the examples" | It had never passed on `examples/payment`. `(define processable …)` calls `val`, the `amount` accessor and `scanl` — shen-derive evaluator intrinsics, not Shen functions — so tc+ rejected the spec on sight. |
| `sb/flow/stdlib.shen` is "the primary engine" | It could not be loaded: it defined `(define call …)`, and `call` is a Shen system function of arity 5. |
| "the two flow engines implement the same rules" | Unfalsifiable while one of them could not run. Running it found three further bugs in the Shen rules, below. |
| `blame_basis: evaluator-and-host` | `detectShenRuntimeHost` was `detectShenRuntime(cfg) == "shen-sbcl"`, which reports what the *spec's* `:runtime-via` markers say. A project beside a working host was told it had none; a project naming a checker got the two-oracle basis with nothing having asked a host anything. |

#### The prelude, and one real spec bug

A spec's `(define …)` bodies are not plain Shen. `shen-derive prelude`
emits typed declarations for exactly the intrinsics a given spec uses,
derived from its own type table, in three files — declares (axioms,
loaded before `(tc +)`, since Shen's `declare` cannot run under the
typechecker), combinator definitions (loaded after it, so the host
checks them), and runnable bodies for the second oracle. The tc+ claim
becomes "well-typed **given these intrinsic signatures**", and
`docs/TRUST-MODEL.md` §5b names the prelude as a TCB member with the
signature table and the evaluator code each row is derived from.

The prelude did not make payment's spec pass on its own, and it should
not have. `processable` applied `val` — the `amount` destructor — to
the running balances `scanl` produces. Those are plain numbers, and
the whole point of the predicate is that one of them may be
*negative*, which is exactly what an `amount` may not be. shen-derive's
evaluator tolerates it because it binds `val` to the identity
function; Shen's types do not. Removing the two applications is a
semantics-preserving fix: `sb derive` regenerates
`processable_spec_test.go` byte-identically and all 44 cases still
pass. The alternative — widening `val`'s declared type until the
mistake typechecked — would have been the prelude covering for the
spec, which is the failure mode this whole workstream exists to avoid.
`shen-derive/prelude` pins the other direction too: a deliberately
ill-typed define is still rejected, in a real host.

#### Four bugs in the Prolog rules, found by running them once

| Bug | Effect had the others been fixed alone |
|---|---|
| `(define call …)`; `call` is a system function | The file does not load. |
| `glob?` had no clause for an exhausted pattern with input left over — reached whenever a premise pattern is a strict prefix of a symbol, i.e. on every real fact base | The `*` clause's guard evaluates `(pos "" 0)` and the host aborts the run. |
| `drop-descriptor-tail` dropped one character per guard instead of the whole `().` suffix; `descriptor-part` ignored backtick quoting | A method symbol canonicalises to `pkg/NewAccess()`, no pattern matches anything, **every verdict is a silent vacuous pass**. |
| Every side condition written `(is V (f …)) (when V)`; both negations written `(when (not (prolog? (p X Y))))` | The first form always fails, so every guarded predicate is unsatisfiable. The second hands the helper *fresh* variables — `prolog?` reads uppercase symbols in its goal as new ones — and the run dies on `mustString`. |

The third is the instructive one: the first two are loud, and the
third is the kind of bug that makes a gate green for the wrong reason.

#### Results

| Example | Gates before | Gates after | `verify-report` |
|---|---|---|---|
| `examples/payment` | 6/7 | **7/7** | OK, `shen typecheck` PASS |
| `examples/multi-tenant-api` | 10/11 | **11/11** | OK, `shen typecheck` PASS |

- **The two flow engines agree on real facts.** multi-tenant's three
  premises and shen-web-tools' two were each evaluated by the Shen
  Prolog rules and by the Go engine, over `scip-go` and
  `scip-typescript` indexes respectively, with no disagreement. That
  is also the first evidence for W3's "same rule text, two languages"
  claim that does not come from the Go side alone.
- **The `lowering` path fires**, in
  `TestSecondOracleDisagreesAndBlamesLowering`, against a live host.
- `examples/shen-web-tools`' spec typechecks under `tc +` as well; the
  example had no `bin/shen-check.sh` at all, so its gate 4 had never
  run. One was added.

Gaps recorded at implementation time:

- **`val` is declarable at one type per spec.** Shen has no
  overloading, so the generator declares it at the spec's unique
  *constrained* wrapper (the single-field datatype carrying a
  `verified` premise — an unconstrained wrapper is a transparent alias
  the spec never needs to destructure). A spec with two constrained
  wrappers gets a `GAP:` comment and no declaration, and gate 4 then
  fails on the first `val`. Neither example is affected. The clean fix
  is per-wrapper destructor names in the spec language, which is a
  spec-format change and not this workstream's.
- **The Shen engine returns verdicts, not violations.** `prolog?`
  answers satisfiability, so the Go engine remains the one that
  produces `file:line` and the shortest violating path. `--engine
  both` compares verdicts, which is the part that says the two decide
  the same thing; it would not catch the two agreeing on *which*
  premise fails while disagreeing about *where*.
- **Shen's Prolog will not scale.** It is a backtracking search over a
  list-shaped fact base. `sb flow` reports a host timeout with the
  documented escape hatch (emit Soufflé from the same rule text)
  rather than hanging, but nothing here makes the primary engine
  usable on a monorepo.
- **The lowering disagreement is staged, not naturally occurring.**
  The unit test forces it on the evaluator's side of the record,
  because the alternative is shipping an evaluator with a real bug in
  it. No genuine divergence between the two oracles exists in either
  example — which is the outcome you want and also means the path has
  been exercised rather than observed.
- **shen-go is not vendored.** `make shen-go` clones and builds it, so
  a fully air-gapped runner needs `SHEN_GO_CACHE` pre-populated or a
  host installed some other way.
- `examples/shen-web-tools` is still missing `bin/shengen-codegen.sh`,
  so its gate 1 cannot run. Pre-existing and out of scope here.

### Goal

Give the repository a Shen host, and find out which of the claims that
depended on one were true.

### Design

One resolver (`cmd/sb/shenhost.go`): `$SHEN`, then `[shen] bin` in
`sb.toml`, then `shen-sbcl` / `shen-scheme` / `shen` on `PATH`, each
confirmed by `--version` so a wrapper script pointing at a missing
runtime does not count. `make shen-go` builds the Go port into
`bin/shen` with `GOTOOLCHAIN=auto`, which lets shen-go's Go 1.27
requirement coexist with this repository's pinned `go1.24.7`. The host
and its version go in every report's toolchain block, unconditionally:
gate 4 is one of the fixed five, so every report makes a tc+ claim and
should say which typechecker backed it.

The second oracle needs the failing case's inputs as Shen text, which
the generated Go test does not carry — its cases are Go constructor
calls. `shen-derive verify --shen-samples-out` writes them again as
Shen literals, refusing values it cannot render faithfully rather than
approximating them.

### Steps

1. `cmd/sb/shenhost.go`, `[shen] bin`, `make shen-go`, one
   `bin/shen-check.sh` the examples delegate to.
2. `shen-derive/prelude` + `shen-derive prelude` + `sb shen-check`;
   host test over both example specs, plus the ill-typed negative.
3. `calls` rename end to end; the Shen engine driver;
   `sb flow --engine`; `flow_engine` in the report.
4. `core.ShenLiteral`, the sample sidecar, `cmd/sb/oracle.go`,
   `detectShenRuntimeHost` asking about the host.
5. `verifyShenTypecheck` in `sb verify-report`; host rows in the audit
   report's toolchain section.
6. Transcripts refreshed; docs.

### Acceptance

- Gate 4 green on both examples, and an ill-typed define still red.
- `sb flow --engine both` green on real facts in both languages.
- A `lowering` blame produced by a real host.
- `sb verify-report` OK on both committed reports with `shen
  typecheck` PASS.

---

## Sequencing

```
W1 brands ─────┐
               ├── W4 forgery gate + mutation ──┐
W2 path cover ─┘                                ├── W5 certificates
W3 flow (independent, longest) ─────────────────┘
```

W1 and W2 can proceed in parallel; both are self-contained inside
the emitters and shen-derive. W3 is independent and the longest, so
it starts alongside them. W4 depends on W1 for the forgery corpus
and on W2 for the mutation survivors to be meaningful. W5 is last
because it certifies whatever the others produce.

Twelve to fourteen weeks for one person; about eight with two.

## Decisions taken in this plan

- Z3 over subprocess, not a Go binding. Reason: zero build
  dependency, graceful degradation.
- Shen Prolog, not Soufflé, as the first flow engine. Reason: the
  spec file stays the single source of truth and the Shen host
  already exists. Soufflé is the documented escape hatch.
- Witness panic rather than pointer-returning constructors for
  zero-value defense in Go. Reason: no call-site churn in existing
  examples; the panic is loud and documented in the TCB.
- Cosign keyless for signatures. Reason: CI identity without key
  management.

## Not in this plan

- Full dependent types or linear types in any target language.
- Runtime enforcement of flow properties. Flow is a build-time gate.
- Any change to `schema_version`. Everything here is additive.
