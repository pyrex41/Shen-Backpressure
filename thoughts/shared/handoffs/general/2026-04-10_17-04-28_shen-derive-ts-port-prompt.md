---
date: 2026-04-10T17:04:28-0500
researcher: reuben
git_commit: b59964c
branch: claude/build-shen-derive-dh1Ti
repository: pyrex41/Shen-Backpressure
topic: "shen-derive TypeScript port — next phase prompt"
tags: [handoff, shen-derive, typescript, port, verification-gate]
status: ready
last_updated: 2026-04-10
last_updated_by: reuben
type: implementation_prompt
---

# Handoff: Port shen-derive to TypeScript

## Your task

Build a TypeScript implementation of `shen-derive` that matches the
Go version at `shen-derive/` in behavior, so projects with TypeScript
implementations can plug into the same spec-equivalence verification
gate. The Go version is the reference; the TS version is a
parallel-but-independent codebase living at `cmd/shen-derive-ts/`.

When you're done, a TS project should be able to write:

```shen
(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx))))) (val B0) Txs)))
```

and have `shen-derive-ts verify ...` generate a TS test file that
calls the hand-written TypeScript implementation and asserts its
behavior matches the spec on 35 sampled inputs.

## Context: what shen-derive does and why it matters

Read these first, in order:

1. `shen-derive/DESIGN.md` — the architecture overview. The
   verification-gate model, why it replaced the v1 rewrite engine,
   what each package does.
2. `site/content/posts/how-shen-derive-works/index.md` — the
   mechanism-level deep dive with the shengen contrast. This is the
   clearest narrative explanation; read it end-to-end before you
   start coding.
3. `examples/payment/demo-shen-derive/DEMO.md` — three concrete bugs
   that shengen alone can't catch but shen-derive does. Running
   `examples/payment/demo-shen-derive/run.sh` gives you the
   intuition for what the tool has to do.
4. `thoughts/shared/handoffs/general/2026-04-10_12-39-23_shen-derive-verification-next-steps.md`
   — the handoff that shaped Direction B (multi-clause defines) and
   explains the design choices baked into the Go version.

The big idea: **shengen** proves things about *values* (opaque
types + validating constructors). **shen-derive** proves things
about *functions* (sampled-input equivalence between a hand-written
impl and a Shen spec). Both read the same `.shen` spec file. They
are additive, not competing.

## Precedent: shengen already has a TS port

`cmd/shengen-ts/shengen.ts` (~980 lines) is the TypeScript version of
`cmd/shengen/main.go`. It's the template for what a TS port of a
Shen-Backpressure codegen tool looks like in this repo:

- Single-file TypeScript, run via `npx tsx` (no build step).
- Output: TypeScript classes with `private readonly _v`, `static
  create(x: …)` factory, `val()` accessor. Composite types get one
  field per premise with camelCase accessors.
- Entry point: `shebang + CLI parsing + readFileSync +
  writeFileSync`.
- Deps: `tsx` and `typescript` as devDependencies; no runtime deps.

Read `cmd/shengen-ts/shengen.ts` once. Note the classify/symbol-table
pattern, the `shenTypeToTs` mapping (`number → number`, `string →
string`, `(list T) → T[]`), and the `verifiedToTs` function that
converts Shen predicates to TypeScript boolean expressions.

Look at `examples/shen-web-tools/` to see a project consuming
shengen-ts end-to-end:

- `package.json` uses `"type": "module"` and runs tests with
  `node --import tsx --test runtime/*.test.ts` (Node's built-in
  test runner, not Jest or Vitest).
- `runtime/guards_gen.ts` is the generated output.
- `runtime/bridge.ts`, `bridge.test.ts` are hand-written consumers.

**That's the module style shen-derive-ts should target**: ESM,
Node's built-in test runner, `npx tsx` for execution. No build
step, no test framework dependency.

## Reference: what to mirror from the Go version

The Go version is stable at commit `b59964c` on branch
`claude/build-shen-derive-dh1Ti`. Treat it as the spec. Every TS
file below has a direct Go counterpart — read the Go first, then
write the TS, then diff the behavior via tests.

### Package layout (proposed)

```
cmd/shen-derive-ts/
  shen-derive.ts          CLI entry (mirrors shen-derive/main.go)
  core/
    sexpr.ts              s-expression types + helpers (core/sexpr.go)
    sexpr-parse.ts        parser (core/sexpr_parse.go)
    sexpr-print.ts        printer (core/sexpr_print.go)
    eval.ts               evaluator with Value variants (core/eval.go)
    match.ts              pattern matcher (core/match.go)
  specfile/
    parse.ts              .shen block extraction + datatype + define parsers (specfile/parse.go)
    typetable.ts          Shen→TS type classifier (specfile/typetable.go)
  verify/
    samples.ts            sample generation (verify/samples.go)
    harness.ts            cartesian product, clause dispatch, test emission (verify/harness.go)
  package.json
  tsconfig.json
  README.md               brief "how to run" + link to DESIGN.md
```

Match that layout one-for-one. It makes the diff with the Go version
trivial to reason about. Do not restructure unless a TS idiom
strictly requires it.

### Per-file porting notes

**`core/sexpr.ts`** — Go file: `shen-derive/core/sexpr.go` (~230 lines).

Port the `Sexpr` sum type as a discriminated union:

```ts
export type Sexpr =
  | { kind: "atom"; atomKind: AtomKind; val: string }
  | { kind: "list"; elems: Sexpr[] };

export type AtomKind = "symbol" | "int" | "float" | "string" | "bool";
```

Helper constructors: `sym`, `num` (int), `float`, `str`, `bool`,
`sList`, `lambda`, `sApply`. Inspection helpers: `isSym`,
`isMetaVar`, `headSym`, `listElems`, `atomVal`, `sexprIntVal`,
`sexprFloatVal`, `sexprBoolVal`, `symName`. Structural equality as
a recursive function (not `Object.is`). `deepCopy` as a recursive
clone.

The Go version uses pointer receivers and structs. TypeScript doesn't
need pointers; plain objects work. Be careful that helper functions
don't mutate — the rewrite engine and matcher both rely on
`DeepCopy` semantics in the Go version.

**`core/sexpr-parse.ts`** — Go file: `shen-derive/core/sexpr_parse.go`.

Port the parser character-by-character. It's a small LL(1)-style
parser, ~300 lines in Go. Handles `(lists)`, `[cons sugar]` (which
desugars to `(cons h (cons t nil))`), `"strings"` with escapes,
atoms (symbols / ints / floats / `true` / `false` / `nil`), and
line comments `\\ ...` and `-- ...`.

The only gotcha: the square-bracket parser handles the `|` tail
token specifically (`[X | Xs]`). Mirror the Go logic exactly — the
pattern matcher depends on `[X | Xs]` parsing to `(cons X Xs)` and
`[A B]` parsing to `(cons A (cons B nil))`.

**`core/eval.ts`** — Go file: `shen-derive/core/eval.go` (~670
lines; the largest file in the port).

Port `Value` as a discriminated union with all the variants:
`IntVal`, `FloatVal`, `BoolVal`, `StringVal`, `ListVal`, `TupleVal`
(Fst, Snd), `ClosureVal` (Env, Param, Body), `PrimPartial` (Op,
Args), `BuiltinFn` (Name, Fn).

`Env` is a linked list of `(name, val, parent)`. Use a class with
`extend(name, val)` and `lookup(name)` methods. The Go version uses
`EmptyEnv()` as `nil`; TS should use `null` with a small helper that
walks the chain.

`Eval(env, sexpr)` handles special forms first (`lambda`, `let`,
`if`, `@p`), then evaluates as function application. `Apply(f, arg)`
dispatches on `ClosureVal`, `PrimPartial`, `BuiltinFn`. `execPrim`
implements the primitives: `+ - * / %`, comparisons, `and or not`,
`cons concat fst snd map foldr foldl scanl filter unfoldr compose`.

**Tricky bits:**

- `numBinOp` promotes int+int → int but anything-with-float → float.
  Make sure JS's loose number semantics don't silently merge them —
  you need a real int/float distinction so the generated Go literals
  and TS literals match what the Go evaluator would produce.
  Consider tagging numbers inside `IntVal` / `FloatVal` with a
  string representation to preserve fidelity, not just `number`.
- `valEqual` treats `IntVal(1)` and `FloatVal(1.0)` as equal
  numerically but distinguishes list/tuple/primitive types. Port
  the Go logic one-for-one.
- Division by zero and modulo by zero must return errors (not
  `NaN` / `Infinity`). The Go version returns `fmt.Errorf(...)`;
  TS should throw a domain error the caller catches, or return a
  Result-style object. Pick one and be consistent.

**`core/match.ts`** — Go file: `shen-derive/core/match.go` (~100
lines).

Port `Match(pat, v) → {bindings, ok}` or `null` on structural
mismatch. Handle: wildcard `_`, uppercase variable binding, `nil`
atom for empty list, literal atoms (int / float / bool / string),
cons patterns `(cons head tail)`.

The structural-mismatch-vs-malformed-pattern distinction is
important: a pattern shape the matcher doesn't support (e.g.
lowercase non-`_`, non-`nil`, non-`cons` symbol) should be an
error, while a mismatched cons pattern against an int should just
return `ok=false`. The Go version uses a sentinel error
`errNoMatch` for this; a TS discriminated union
(`"matched" | "miss" | "error"`) reads more naturally.

**`specfile/parse.ts`** — Go file: `shen-derive/specfile/parse.go`
(~400 lines after the multi-clause extension).

Two parts:

1. **Block extraction** (`extractBlocks`). Mirrors
   `cmd/shengen-ts/shengen.ts`'s existing block extractor — you may
   be able to lift it directly.
2. **Datatype parser** (`parseDatatype`, `buildRule`). Also mirrors
   the existing shengen-ts logic. Watch for the `==` / `__`
   horizontal-line separator and the `[A B C] : type` vs `X : type`
   conclusion shapes.
3. **Define parser** (`parseDefine`, `parseDefineClauses`,
   `splitPatterns`, `extractBalancedParen`, `deriveParamNames`).
   This is new work — shengen-ts doesn't read defines today. Port
   the clause-splitting algorithm precisely from the Go version
   (see `shen-derive/specfile/parse.go:292-500`). It's fragile:
   collapse whitespace, split on `" -> "`, walk pairs, detect
   `where EXPR` guards via balanced-paren extraction. The
   dosage-calculator spec at `examples/dosage-calculator/specs/core.shen`
   is the integration test — if the parser handles
   `pair-in-list?` and `drug-clear-of-list?`, it's right.

**`specfile/typetable.ts`** — Go file:
`shen-derive/specfile/typetable.go`.

The type classifier. Given a list of datatypes, produce a table
keyed by Shen type name with entries like:

```ts
type TypeEntry = {
  shenName: string;
  tsName: string;          // camelCased PascalCase class name
  category: "wrapper" | "constrained" | "composite" | "guarded" | "alias" | "sumtype";
  shenPrim: string;        // underlying primitive for wrappers
  tsPrimType: string;      // TS type string for the underlying primitive
  verified: string[];      // verified predicates as raw strings
  varName: string;         // variable name used in verified predicates
  fields: FieldEntry[];    // for composite/guarded
  importPath: string;      // the TS import specifier for the shengen-ts module
  importAlias: string;     // e.g. "shenguard"
};
```

This mirrors `shen-derive/specfile/typetable.go` and
`cmd/shengen/main.go:161-177`. Don't share code with shengen-ts —
they should be parallel implementations, consistent with how the Go
`specfile` package is parallel to `cmd/shengen/main.go`.

**`verify/samples.ts`** — Go file: `shen-derive/verify/samples.go`
(~270 lines).

Port the sample generator:

- `Sample = { value: Value; tsExpr: string }` — two projections, one
  for the spec evaluator (the runtime `Value`) and one for the
  emitted TS test file (the source expression).
- `SampleCtx = { tt: TypeTable; rand: SeededRng | null; randomDraws: number }`.
- `GenSamples(ctx, shenType)` — walks the type string, recognizes
  `(list T)`, primitives, declared types; dispatches on the type
  table category.
- Boundary pools: numbers `{0, 1, -1, 5, 2.5, 100}`, strings
  `{"", "alice", "bob"}`, booleans `{true, false}`. These must
  match the Go pools exactly — the generated test files need to be
  comparable across the two ports.
- `wrapperSamples` evaluates `verified` predicates using the core
  evaluator to filter the primitive pool. This is critical for
  bug 2 (truncation) in the demo — `2.5` survives only because
  the `(>= X 0)` predicate passes it, and the bug is caught on
  that specific value.
- `compositeSamples` produces one variation per field-sample index.
- `listSamples` produces empty + one singleton per elem sample
  (capped at 6) + one 3-element mix. This is the "one singleton
  per elem sample" fix that makes tricky composites land inside
  lists — do not regress to "singleton of the first elem only".
- `randomNumberSamples(rnd, n)` mixes int and float draws in
  `[-1000, 1000]` with 2-decimal-place precision.
- `randomStringSamples(rnd, n)` produces lowercase alphanumeric
  strings of length 1-8.

**Seeded RNG for reproducibility**: JavaScript's `Math.random` is
not seedable. Port a minimal LCG or Mulberry32 PRNG that matches
Go's `math/rand` output is NOT a hard requirement — what matters is
that `--seed 42` is reproducible within the TS version, not that it
produces the same sequence as the Go version. The generated test
files for the same spec on the same seed will differ between
languages, and that's OK: the drift gate for each project compares
against its own committed copy.

**`verify/harness.ts`** — Go file: `shen-derive/verify/harness.go`
(~550 lines).

Port the main flow:

1. `HarnessConfig` — spec, type table, `allDefines`, impl module
   path, impl function name, import path of the shengen-ts module,
   test file package, `maxCases`, `seed`, `randomDraws`.
2. `BuildHarness(cfg)` — validate, generate samples per parameter,
   cartesian product capped at `maxCases`, build base env, iterate
   cases calling `evalSpec` + `tsLiteralFor`.
3. `buildBaseEnv(tt, defines)` — bind `val` as identity, field
   accessors as closures projecting out of list values, every
   define as a curried `BuiltinFn` for mutual recursion. The
   `envHolder` trick exists because the Go version needs a shared
   pointer the closures can dereference after the env is fully
   populated; in TS, a mutable `{ env }` container is equivalent.
4. `evalSpec / evalDefine / bindClausePatterns` — clause dispatch
   via the pattern matcher + guard.
5. `tsLiteralFor(value, shenType, tt)` — convert `Value` to a TS
   source expression. For `(list T)` emit `[...] as const` or just
   `[...]`. For `number / boolean / string` emit plain literals.
   For wrapper/constrained return types unwrap via `.val()` in the
   test comparison (mirrors the Go version's `got.Val()` pattern).
6. `Emit()` — write the TS test file with:
   - `// Code generated by shen-derive-ts. DO NOT EDIT.` header
   - The seed stamp if `seed !== 0`
   - `import { test } from "node:test"; import assert from "node:assert";`
   - `import { ... } from "<shengen-ts import path>";`
   - `import { ... } from "<impl module path>";`
   - `mustXxx` helper functions for each shengen guard type the
     cases reference
   - One `test(...)` call per case, using `assert.strictEqual`
     (with `deepStrictEqual` for array returns)

**The emitted file is the central artifact — read a real
shengen-ts test file (e.g.
`examples/shen-web-tools/runtime/bridge.test.ts`) to see the style
and match it.**

**`shen-derive.ts` (CLI)** — Go file: `shen-derive/main.go`.

One subcommand: `verify`. Flags:

```
shen-derive-ts verify <spec.shen> \
  --func <shen-define-name> \
  --impl-module <relative TS path, e.g. "./processable"> \
  --impl-func <exported TS function name> \
  --import <shengen-ts module path, e.g. "./guards_gen"> \
  --import-alias <default "shenguard"> \
  --out <test file path> \
  [--max-cases 50] \
  [--seed 0] \
  [--random-draws 0]
```

Note that `--impl-module` differs from the Go version's
`--impl-pkg`: Go imports by module path, TS imports by relative file
path. Document this clearly.

## Tests: how you'll know it's working

1. **Unit tests for core/ and specfile/** — port the Go tests file
   by file. They're small (~5-15 cases each) and they encode the
   important edge cases. Use Node's built-in test runner:
   ```ts
   import { test } from "node:test";
   import assert from "node:assert";

   test("parse cons", () => {
     const s = parseSexpr("[X | Xs]");
     // ...
   });
   ```
2. **Integration test for verify** — port
   `shen-derive/verify/harness_test.go`'s `TestBuildHarnessPayment`.
   It constructs an in-memory spec file, parses it, builds a
   harness, and checks the emitted source for key substrings.
3. **End-to-end test against a real TS example.** Pick (or create) a
   minimal TS example:
   - `examples/shen-web-tools/` already consumes shengen-ts. Add a
     `(define ...)` block to its spec for some pure function, write
     a small TypeScript implementation, run `shen-derive-ts verify`,
     commit the generated test, and run `node --import tsx --test`
     to confirm it passes.
   - Alternatively, create `examples/payment-ts/` as a parallel to
     `examples/payment/` — a fresh TS port of the balance-check
     scenario. This is the cleanest demo for a blog post but is
     more work.

   Start with shen-web-tools if possible; create payment-ts only
   if the existing project's defines are awkward.
4. **Port the demo**: `examples/payment-ts/demo-shen-derive/` (or
   similar) with a `run.sh` that rotates three buggy TS
   implementations and shows shengen-ts + shen-derive-ts catching
   the bugs. Mirror `examples/payment/demo-shen-derive/`.

## Design questions you'll hit and suggested answers

1. **Test runner**: use `node:test`. Matches shengen-ts's existing
   convention. No external dependency. Works with `tsx`. Jest and
   Vitest are fine runners but they're additional installs per
   project, and the whole point is to keep the harness integration
   as zero-friction as possible.

2. **Assertion style**: `assert.strictEqual` for primitives, booleans,
   and wrapper-unwrapped values; `assert.deepStrictEqual` for lists
   and composites. Do not use `assert.equal` (lenient coercion).

3. **`number` fidelity**: JavaScript has no int/float distinction at
   the runtime level. The Go evaluator distinguishes `IntVal(5)`
   from `FloatVal(5.0)` so that arithmetic promotion works
   correctly. For the TS port, tag the `Value` union explicitly:
   ```ts
   | { kind: "int"; val: bigint }     // or number, with a flag
   | { kind: "float"; val: number }
   ```
   If you use `bigint` you need to coerce for arithmetic with
   floats. If you use a flag on `number`, you avoid coercion but
   lose large-int support. Pick `number + flag` for v1 — the sample
   pool doesn't exceed 2^31.

4. **Emitted TS imports**: ESM with file-extension-preserved paths
   (`./processable.ts` → `./processable`). The generated test lives
   next to the impl and imports via a relative path. The shengen-ts
   import lives at `runtime/guards_gen.ts` in shen-web-tools' case;
   shen-derive-ts needs a config flag for it.

5. **Monorepo vs standalone npm package**: keep shen-derive-ts as a
   single-file CLI (`cmd/shen-derive-ts/shen-derive.ts`) like
   shengen-ts, runnable via
   `npx tsx cmd/shen-derive-ts/shen-derive.ts verify ...`. Do not
   publish to npm — consumers vendor it via `npx tsx` with a
   relative path. Same pattern shen-web-tools uses for shengen-ts.

6. **Language-agnostic `AllDefines` env**: in the Go version every
   define in the spec file is bound as a curried `BuiltinFn` in the
   base env so clause bodies can call each other (and themselves)
   by name. Port this behavior exactly — the dosage-calculator
   `pair-in-list?` integration test depends on it.

## Non-goals (don't do these)

1. **Do not port the archived v1 rewrite engine.** `shen-derive/archive/`
   contains the old codegen-from-rewrites pipeline. It's not on the
   critical path and the verification-gate model replaced it for
   good reasons. If someone asks about it, point them at the DESIGN
   doc's "Why the pivot" section.
2. **Do not unify with shengen-ts.** They share a file format, not a
   codebase. Keep them parallel so each tool's guarantee is easy to
   reason about separately.
3. **Do not add auto-discovery of defines.** Start with an explicit
   `--func` flag per invocation, mirroring the Go version. Auto-
   discovery from `(define ...)` blocks is a future feature that
   would live above this CLI, in `sb derive`.
4. **Do not try to share generated test output bit-for-bit with the
   Go version.** Same spec, different language, different
   serialization. Each language gets its own drift gate.
5. **Do not add build steps.** No `tsc` compile output, no bundler.
   `tsx` runs the source directly; the consumer runs
   `npx tsx cmd/shen-derive-ts/shen-derive.ts verify ...` and that's
   it.

## Suggested order of attack

1. **Phase 1 — core/** (sexpr, parse, print, eval, match). Get the
   unit tests green. This is the biggest chunk (~1500 lines port)
   but it's the foundation. Every edge case encoded in the Go tests
   has to pass in the TS version.

2. **Phase 2 — specfile/** (parse, typetable). Parser for datatypes
   and multi-clause defines, plus the type classifier. Integration
   test: round-trip the payment spec and the dosage-calculator spec
   through the parser, assert on structure.

3. **Phase 3 — verify/** (samples, harness). Sample generator,
   clause-dispatch evaluator, test emitter. Integration test:
   build a harness for an in-memory payment spec, emit the TS
   source, parse the emitted source with the TS compiler API (or
   just check substrings) to confirm it compiles.

4. **Phase 4 — CLI**. Thin wrapper around the verify package.
   `shen-derive-ts verify <flags>`.

5. **Phase 5 — Integration with a real TS project**. Wire it up to
   shen-web-tools (or create payment-ts), commit the generated
   test file, and confirm `node --import tsx --test` passes. Then
   break the impl and confirm the test fails with a clear
   spec-vs-impl message.

6. **Phase 6 — Demo + blog post** (optional). Port
   `examples/payment/demo-shen-derive/` to TS to show the same
   three bugs being caught. Optionally write a follow-up blog post
   that contrasts shen-derive-go and shen-derive-ts for readers
   interested in the polyglot story.

Each phase is independent enough to land as its own commit. Don't
try to land the whole port in one shot.

## Files you should read once before starting

In order, no skimming:

1. `shen-derive/DESIGN.md` — the what and why
2. `site/content/posts/how-shen-derive-works/index.md` — the how,
   narrative version
3. `shen-derive/core/sexpr.go` + `sexpr_parse.go` + `sexpr_print.go`
   — the s-expression representation
4. `shen-derive/core/eval.go` — the evaluator, the largest single
   file you'll port
5. `shen-derive/core/match.go` + `match_test.go` — the matcher
6. `shen-derive/specfile/parse.go` — the spec file parser
7. `shen-derive/specfile/typetable.go` — the type classifier
8. `shen-derive/verify/samples.go` — sample generation
9. `shen-derive/verify/harness.go` — the main harness + emitter
10. `shen-derive/verify/harness_test.go` — integration tests; the
    payment and pair-in-list cases are the ones you need to match
11. `cmd/shengen-ts/shengen.ts` — the existing TS codegen precedent
12. `examples/shen-web-tools/runtime/guards_gen.ts` — what the TS
    side of the guard types looks like
13. `examples/shen-web-tools/package.json` — the module conventions
    to mirror
14. `examples/payment/demo-shen-derive/run.sh` and `DEMO.md` — the
    bug demo you'll eventually port

Once those are in your head, start with `core/sexpr.ts`.

## Success criteria

You're done when:

1. `npx tsx cmd/shen-derive-ts/shen-derive.ts verify <spec> --func
   foo --impl-module ./foo --impl-func Foo --import ./guards_gen
   --out foo.test.ts` produces a TS test file that
   `node --import tsx --test` runs successfully.
2. All the in-memory spec fixtures from
   `shen-derive/verify/harness_test.go` have TS equivalents that
   pass the analogous assertions (35-case payment build; 4-clause
   pair-in-list? recursion; seeded sampling reproducibility).
3. At least one real project in `examples/` uses
   `shen-derive-ts verify` in its test pipeline, with the generated
   test committed and a drift gate wired up via its Makefile or
   package.json script.
4. The three-bug demo (or its TS equivalent) runs end-to-end and
   shows shengen-ts happy, shen-derive-ts catching each bug.
5. Unit test count on the TS side is at least 80% of the Go side,
   per package.

## Things to be careful about

- **Don't let JS number coercion hide int/float bugs.** Tag your
  Values explicitly. Test mixed-mode arithmetic early.
- **Preserve symbol case.** Uppercase-var pattern binding depends on
  the parser not normalizing case. Go's parser doesn't; make sure
  the TS parser doesn't either.
- **`cons` vs `@p` vs list-literals.** The Go evaluator has special
  forms for `lambda / let / if / @p`; everything else goes through
  the primitive table. Tuple construction is `(@p a b)` → a
  `TupleVal`. Port the special-form detection early.
- **Error strings in the emitted test.** Match the Go version's
  `"%s: spec says %v, impl returned %v"` pattern so reviewers who
  read both language ports see familiar failure messages.
- **Don't cargo-cult Promise/async.** The evaluator is pure and
  synchronous. File I/O at the CLI layer is the only async
  operation. Resist wrapping things in `async` for no reason.

## Open questions for the user

If any of these are unclear when you start, ask before coding:

1. **Should the TS port live at `cmd/shen-derive-ts/` (parallel to
   `cmd/shengen-ts/`) or at `shen-derive/typescript/` (nested
   inside the Go module)?** Strong preference for the former
   — matches the shengen precedent — but check first.
2. **Which TS example project is the integration target?**
   shen-web-tools is convenient but its existing defines may not be
   expressive enough. A fresh `payment-ts/` is cleaner but adds
   setup work. Confirm before spending time on either.
3. **Should `sb derive` be extended to dispatch on language
   (detect `package.json` → invoke shen-derive-ts, detect `go.mod`
   → invoke shen-derive-go), or should projects configure the
   command explicitly?** The former is neater; the latter is
   simpler. Defer to whoever owns `cmd/sb/` at that point.
