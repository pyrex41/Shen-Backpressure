---
date: 2026-04-24T00:00:00Z
researcher: reuben
git_commit: eb3be68899aab73594f82192d640663d5a77c742
branch: main
repository: pyrex41/Shen-Backpressure
topic: "shengen-ts parity with Go reference"
tags: [handoff, shengen, shengen-ts, typescript, port, parity]
status: ready
last_updated: 2026-04-24
last_updated_by: reuben
type: implementation_prompt
---

# Handoff: bring `shengen-ts` to parity with the Go reference

## Your task

`cmd/shengen/main.go` (1885 lines, Go) is the reference implementation of
shengen — the tool that parses Shen sequent-calculus spec files and emits
guard types. `cmd/shengen-ts/shengen.ts` (978 lines, TS) is a partial port.
It handles most datatype categories but has structural gaps that make it
unusable for real specs.

Your job is to bring `shengen-ts` to behavioral parity with the Go version
on everything up to and including `(define ...)` support, set up the
missing npm build scaffolding, port the Go test suite, and produce a
TypeScript end-to-end example under `examples/`.

When you're done, the following should all pass:

1. `make build-shengen-ts` succeeds (today it fails — see §1 below).
2. `npm test` in `cmd/shengen-ts/` passes a ported version of
   `cmd/shengen/main_test.go`'s 17 test functions.
3. `./bin/shengen-ts --lang ts --spec examples/payment/specs/core.shen
   --out examples/ts-payment/src/guards.ts --pkg guards` produces a file
   whose class definitions mirror the Go output's semantics and whose
   emitted `mustAmount`, `mustTransaction`, etc. free functions let
   `shen-derive-ts verify` run end-to-end against it.
4. `examples/ts-payment/` has a committed `guards.ts`, a small Vitest
   or node:test suite that constructs valid and invalid instances, and
   an `sb.toml` with `lang = "ts"`.
5. The three correctness bugs called out in §3 are fixed and have
   regression tests.

## Why this work matters (downstream context)

There's a downstream project at `/Users/reuben/projects/nw/merkle/` that
extends nowhere (the URL-fragment-encoded website tool) with
content-addressed composition — Merkle DAGs of fragments referenced by
hash. Its specs (`merkle/specs/codec.shen`, `crypto.shen`, `compose.shen`,
`resolve.shen`, ~1100 lines total) rely heavily on user-defined predicates
(`covers-all-refs?`, `acyclic?`, `content-address-of`, …). Today those
compile to `/* TODO */ true` no-ops — the guards are rubber stamps. The
merkle project is blocked on this parity work.

See `/Users/reuben/projects/nw/merkle/notes/shengen-ts-parity.md` for the
same gap list expressed as merkle-side acceptance criteria. They agree.

## Context to read first

In order:

1. `cmd/shengen/main.go` — the reference. Read start-to-finish once, paying
   attention to: `parseFile` (1159), `parseDefine` (1242), `SymbolTable`
   (100-110), `verifiedToGo` dispatch (456-472), `translateDefineCall`
   (692-710), `generateDefineHelpers` (940-992), `generateOneDefine`
   (1010-1122), `generateWrapper/Constrained/Composite/Guarded`
   (1614-1705), `structuralMatchFallback` (527-585), `inferTargetFields`
   (606-626), CLI parsing (1818-1831).
2. `cmd/shengen/main_test.go` — 588 lines, 17 tests. Your porting target.
   The `parseFile_string` helper at line 563 lets you test without
   touching the filesystem; replicate this pattern in TS.
3. `cmd/shengen-ts/shengen.ts` — the current TS. Same five-pass
   architecture. Understand where it diverges from Go before editing.
4. `cmd/shen-derive-ts/shen-derive.ts` + `verify/harness.ts` +
   `verify/samples.ts`. `harness.ts:357` and `samples.ts:180,251` are
   where `shenguard.mustXxx(...)` calls appear — they tell you the shape
   that your `must*` exports must match.
5. `shen-derive/verify/harness.go:597-636` — the `emitHelper` function
   that documents what each `must*` wrapper should do per category.
6. `examples/payment/specs/core.shen` and
   `examples/payment/internal/shenguard/guards_gen.go` — the Go output is
   the oracle for your TS output's semantics (not byte-for-byte, but
   behavior-for-behavior).
7. `thoughts/shared/handoffs/general/2026-04-10_17-04-28_shen-derive-ts-port-prompt.md`
   — the earlier TS port handoff. Similar style to what you're doing.

## §1 Preliminary: TS build scaffolding does not exist

`cmd/shengen-ts/` contains `shengen.ts` and nothing else. There is no
`package.json`, no `tsconfig.json`, no `README.md`. `make
build-shengen-ts` (at `Makefile:31-32`) runs `npm install && npm run
build` in a directory with no manifest — it fails silently or loudly
depending on your npm version. The same is true of `cmd/shen-derive-ts/`,
which has subdirectories but no top-level manifest.

**First action:** set up a minimal TS package.

- `cmd/shengen-ts/package.json` — name `@shen-backpressure/shengen-ts`,
  scripts: `build` (`tsc`), `test` (`node --test` or `vitest run`), `exec`
  (`tsx shengen.ts`). Dev deps: `typescript`, `tsx`, `@types/node`, and
  whichever test runner you pick.
- `cmd/shengen-ts/tsconfig.json` — `target: ES2022`, `module: ESNext`,
  `moduleResolution: Bundler`, `strict: true`, `outDir: dist`.
- Entry point should produce a `bin/shengen-ts` shebang wrapper or
  expose via `tsx` directly. Prefer `tsx` at runtime to match the
  precedent set in `cmd/shen-derive-ts/` — no compiled output
  committed.
- `Makefile` target: update `build-shengen-ts` to reflect your choice.
  If you stay tsx-only, replace the npm commands with `npm install`.
  If you compile, keep `npm run build`.

**Pick a test runner.** The existing `cmd/shen-derive-ts/verify/harness.test.ts`
uses `import { test } from "node:test"` with `import assert from "node:assert/strict"`.
Match that. No Vitest dependency needed; runs on Node 18+.

Do the same scaffolding for `cmd/shen-derive-ts/` if it's missing — check
`package.json` / `tsconfig.json` presence there.

## §2 P0 — feature gaps that block real use

### 2.1 `(define ...)` parsing and helper-function emission (large)

Every user-defined predicate in a `:verified` premise is currently a
no-op. `shengen.ts:439`:

```ts
// unknown op — emit TODO
return [`/* TODO: ${sexprToStr(e)} */ true`, ...];
```

The Go equivalent dispatches at `main.go:468-470`:

```go
if _, ok := st.Defines[op]; ok {
    return st.translateDefineCall(expr, varMap, true)
}
```

Your work, in order:

1. **AST.** Add `DefineClause` and `Define` TS types mirroring Go at
   `main.go:90-99`. A `Define` has a name, a signature (parsed from
   `{t1 --> t2 --> bool}` syntax), and one or more clauses (pattern +
   body s-expr).
2. **Parser.** Extend `parseFile` to return `{ datatypes: Datatype[]; defines: Define[] }`.
   Study `main.go:1242-1330` for `parseDefine`. The parser has to
   recognize `(define NAME {TYPESIG} CLAUSE_1 CLAUSE_2 ...)` form and
   split it into clauses separated by the `->` arrow.
3. **Symbol table.** Extend `SymbolTable` with `defines:
   Map<string, Define>` and `defineResolved: Map<string, ResolvedBody>`.
   Port `resolveTransitiveDefines` (`main.go:890-938`) to compute the
   dispatch tree for each define.
4. **Dispatch.** Extend `verifiedToTs` (`shengen.ts:418`) with a branch
   that matches `st.defines.has(op)` and emits
   `translateDefineCall(expr, varMap)`. Port `translateDefineCall`
   (`main.go:692-710`).
5. **Helper emitter.** Port `generateDefineHelpers` (`main.go:940-992`)
   and `generateOneDefine` (`main.go:1010-1122`). The output per define
   is a TypeScript function that walks the AST of the define's body,
   translating Shen s-expressions into TS expressions. The key
   substitutions are `foldr/foldl/scanl` → Array methods, lambda
   → arrow functions, `(val X)` → `X.val()`, `(head L)` → `L[0]`, etc.
   Study `shen-derive-ts/core/eval.ts` — it already evaluates these
   forms as an interpreter; you can crib the translation table.

**Acceptance:** `examples/payment/specs/core.shen`'s
`(define processable ...)` compiles to a working TS function. Running
`shen-derive-ts verify` against it emits a test file whose assertions
call the real predicate, not a stubbed true.

### 2.2 `must*` free-function exports (small, unblocks shen-derive-ts)

`shen-derive-ts`'s generated test files import from the shenguard module
and call `shenguard.mustAmount(x)`, `shenguard.mustTransaction(a,b,c)`,
etc. shengen-ts emits `Amount.create(x)` and nothing else — so the test
file throws at runtime with "`shenguard.mustAmount is not a function`."

For each generated class, also emit a free exported function at the
module level:

```ts
export function mustAmount(x: number): Amount {
  const r = Amount.create(x);
  if (r instanceof Error) throw r;
  return r;
}
```

See `shen-derive/verify/harness.go:597-636` (`emitHelper`) for the Go
pattern of what each category's `must*` body looks like:

- **Wrapper**: `(x) => X.create(x)` — never fails.
- **Constrained**: throw on `Error` return.
- **Composite/guarded**: `(a0, a1, ...) => X.create(a0, a1, ...)`;
  throw on error.

This one is small (~50 LOC addition) and unlocks shen-derive-ts
end-to-end testing. Do it first.

### 2.3 `--pkg` flag (small)

Go accepts `--pkg shenguard` at `main.go:1827-1831` and emits
`package shenguard`. TS accepts no such flag. The merkle-nowhere
`sb.toml` sets `pkg = "guards"`; shengen-ts needs to honor it.

For TS, the analogous concept is the module-level namespace or the
filename. Two options:

- Emit a `namespace ${pkg} { ... }` wrapper. Works in ambient-module
  consumers; limiting for ESM.
- Emit each class as `export` at the module top level, and let the
  `pkg` value propagate only to the output filename. Simpler, more
  idiomatic TS.

Recommend option 2. Document the choice in the TS output header
comment.

### 2.4 Multi-file spec input (medium)

Both Go and TS read exactly one file (`main.go:1821-1826`,
`shengen.ts:964`). The merkle-nowhere specs span four files that
reference each other's types.

Minimum: extend the CLI to accept `--spec path1 --spec path2 ...` or a
comma list. Read all in order, concatenate the resulting
`Datatype[]` / `Define[]`, de-dupe by name, emit one output file.

Stretch: an `(import "other.shen")` directive inside a spec file.
Study how `shen-web-tools` splits specs across files if this gets
complicated.

## §3 P1 — correctness bugs in what's already ported

### 3.1 Alias classification missing `!isSumVariant` guard

`shengen.ts:125-131`:

```ts
// alias branch — classifies single-premise non-wrapped conclusions
if (/* ... */) {
  kind = 'alias';
  ...
}
```

Go at `main.go:167-170` has the additional `&& !isSumVariant` constraint.
`isSumVariant` is computed at `main.go:159` by checking whether the
conclusion type appears as a conclusion of more than one datatype block.

**Bug.** A sum-type variant with a single non-primitive premise gets
classified as an alias. The alias emitter at `shengen.ts:910-914`
produces `export type Foo = Bar`, losing the variant's field data.

**Fix.** One-line: port the `isSumVariant` computation and add
`&& !isSumVariant` to the alias branch.

**Regression test.** Write a spec with a sum type whose variant wraps
a struct, compile it, assert the variant emits as a class (or branded
type) not a type alias.

### 3.2 `static create` infallible return type but throws internally

At `shengen.ts:845,901` the signature is `static create(x: T):
ClassName` — declared infallible. Internally, failing premises throw.
Callers have no type-level signal that construction can fail.

Go returns `(T, error)` explicitly at `main.go:1631,1679`.

**Decision needed before fixing.** Three options:

- Change return to `ClassName | Error`. Breaks fluent chaining, is
  type-honest.
- Keep `ClassName`, rename throwing variant to `createOrThrow`, add
  separate `tryCreate(): ClassName | Error`.
- Discriminated union `Result<T, E>`. Overengineered for this codebase.

Recommend option 1 for new code and a migration path for existing
callers via a temporary `createUnchecked` alias. Tag the decision in a
PR description before shipping.

### 3.3 `inferTargetFields` missing in structural-match fallback

`main.go:527-585` (`structuralMatchFallback`) calls `inferTargetFields`
at `main.go:606-626`, which counts tail-operation depth to narrow
which fields are being compared before the shared-type scan.

`shengen.ts:552-583` skips that step. In specs where multiple fields
share a type, the fallback is less precise.

**Fix.** Port `inferTargetFields` and plumb it through
`structuralMatchFallback`.

**Regression test.** Construct a composite type with two `string`
fields; write a premise that accesses `(head (tail X))`; assert the
correct field is compared in the generated code.

## §4 P2 — polish and DX

- **Port the test suite.** `cmd/shengen/main_test.go` has 17 functions.
  Map 1:1. Use `parseFile_string` pattern for filesystem-free tests.
  Do this in parallel with the feature work; many P1 bugs surface as
  failing ports.
- **TS end-to-end example.** Create `examples/ts-payment/` with the
  same `core.shen` as `examples/payment/`, committed `guards.ts`, an
  `sb.toml` with `lang = "ts"`, a small test suite, and a generated
  shen-derive test. This is your adoption proof.
- **`--db-wrappers` (low urgency).** Go generates proof-carrying
  DB-row wrappers at `main.go:1746-1811`. No TS consumer wants this
  yet; skip unless time permits.

## §5 Guardrails

- **Do not regress the Go path.** All changes land in `cmd/shengen-ts/`
  and (for scaffolding) the `Makefile`. Go code stays untouched.
- **Do not edit committed generated files.** `examples/payment/internal/shenguard/guards_gen.go`
  is regenerated — leave it alone except where you verify your TS
  output against it.
- **Follow the existing style.** The shen-web-tools conventions in the
  repo use `\* ... *\` block comments in specs; TS files use standard
  `//` and `/* */`. Match what's in `shengen.ts` today; don't
  reformat.
- **Node test runner, not Vitest/Jest.** Matches the precedent in
  `cmd/shen-derive-ts/`.
- **Single-file tsx execution.** Don't introduce a compile step unless
  you must. `npx tsx shengen.ts` is the execution model; keep it.

## §6 Suggested order

Do these in order. Each is a landable PR on its own.

1. TS build scaffolding (`package.json`, `tsconfig.json`, fix Makefile).
   Ship an empty test that just proves the build works.
2. Port `main_test.go` tests against *current* shengen.ts. Red-tests
   will surface 3.1-3.3 for free. Fix those bugs.
3. `must*` export emission (§2.2). Ship with a regression test.
4. `--pkg` flag (§2.3).
5. Multi-file spec input (§2.4).
6. `(define ...)` support (§2.1). Largest chunk. Study Go deeply before
   writing TS.
7. `examples/ts-payment/` end-to-end. Adoption proof. Closes the loop.

Each step should leave `make build-all` green and all existing Go
example outputs unchanged.

## §7 When to ask before proceeding

- Before changing the `static create` return type (§3.2). This is a
  breaking change to the emitted API; the user wants to weigh in on
  option 1 vs option 2.
- Before adding runtime dependencies to shengen-ts or shen-derive-ts.
  The precedent is zero runtime deps (shebang + tsx + stdlib).
- Before choosing a multi-file import syntax (§2.4) if you find it
  interacts with `(define ...)` resolution. There may be scoping
  questions.

## §8 Done signal

```
cd ~/projects/Shen-Backpressure
make build-all          # green
cd cmd/shengen-ts && npm test    # green
cd ../shen-derive-ts && npm test # green

# End-to-end: TS guards + TS shen-derive on the payment example.
./bin/shengen-ts --spec examples/ts-payment/specs/core.shen \
  --out examples/ts-payment/src/guards.ts --pkg guards
./bin/shen-derive-ts verify \
  --spec examples/ts-payment/specs/core.shen \
  --func processable \
  --impl-pkg ./impl --impl-func processable \
  --guard-pkg ./guards \
  --out examples/ts-payment/src/processable.spec.test.ts
cd examples/ts-payment && npm test    # green
```

When all of the above pass, the merkle-nowhere project is unblocked
and this handoff is complete.

## Where to record findings

- Ongoing notes: `thoughts/shared/research/` for any architecture
  writeups you produce.
- The parity view from the merkle-nowhere side:
  `/Users/reuben/projects/nw/merkle/notes/shengen-ts-parity.md`.
  Update it if your work changes priorities.
