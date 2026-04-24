# shengen-ts

TypeScript port of `shengen`. Parses Shen sequent-calculus spec files
(`(datatype …)` blocks plus `(define …)` helper predicates) and emits
a TypeScript module with guarded classes, `must<Type>` free-function
helpers, and translated define bodies.

Parallel to `cmd/shen-derive-ts/` — same ESM / `node:test` module
style. Runtime is zero-install: Node 22.6+ strips TypeScript types on
the fly (`--experimental-strip-types`), and `./bin/shengen-ts` wraps
the entry point. `npm install` at this directory pulls in `typescript`
+ `@types/node` strictly for `npm run typecheck` / `npm test`; the
bin wrapper never needs the installed deps at runtime.

The Go version at `cmd/shengen/` is the reference for most parser and
symbol-table behavior. The TS port diverges in a few places —
[see below](#divergences-from-the-go-reference).

## Usage

```
./bin/shengen-ts [--spec FILE]... [--pkg NAME] [--out FILE] [--dry-run] [positional.shen]
```

- `--spec FILE` — repeatable; merges datatype + define blocks across
  files, rejecting cross-file redefinitions.
- `--pkg NAME` — documented in the header comment; does not alter
  class emission. TS has no package declaration, so consumers choose
  the binding at their import site (e.g.
  `import * as guards from "./guards.ts"`).
- `--out FILE` — write output to a file instead of stdout.
- `--dry-run` — parse + build the symbol table, but skip generation.

A bare positional arg is equivalent to a single `--spec`. If neither
is supplied the CLI defaults to `specs/core.shen`.

### Example

```
./bin/shengen-ts \
  --spec examples/payment/specs/core.shen \
  --pkg guards \
  --out /tmp/guards.ts
```

Produces `/tmp/guards.ts` containing:
- One `export class` per datatype (wrapper / constrained / composite /
  guarded), plus `export type` sum-type unions and plain aliases.
- `createOrThrow`, `tryCreate`, `val`, and per-field accessors on each
  class.
- `export function must<Type>(…): <Type>` free-function helpers that
  delegate to `createOrThrow`. These are the entry points
  shen-derive-ts's generated tests call via `shenguard.mustAmount(5)`.
- `export function <snake>` helpers translated from every `(define …)`
  block, with TS type signatures pulled from the Shen
  `{t1 --> t2 --> ret}` signature.

## Testing

From this directory:

```
npm install          # once, for dev deps
npm test             # node --test --experimental-strip-types **/*.test.ts
npm run typecheck    # tsc --noEmit
```

Tests cover the parser, symbol table, S-expression parser, accessor
resolution, `verifiedToTs` dispatch, the type propagator, and
end-to-end code generation (including runtime imports of generated
modules in a temp dir).

## Divergences from the Go reference

The behaviors below are deliberate deviations from `cmd/shengen`
(documented in `shengen.ts` comments and covered by tests).

- **Static factory API:** classes expose `createOrThrow(x): T` (throws
  on failure) and `tryCreate(x): T | Error` (error-as-value). Go has
  only `New<Type>(x): (T, error)`. Rationale: JS/TS callers prefer
  throwing ergonomics, but the type-honest alternative is one method
  name away. Top-level `must<Type>` helpers bridge back to the
  throwing convention used by shen-derive-ts.
- **Define emission:** shengen-ts emits a TS helper for every
  `(define …)` block, regardless of whether any `:verified` premise
  references it. The Go version skips defines that aren't referenced
  by a verified premise.
- **Parser fix:** `where`-guarded clauses attach to the clause they
  actually belong to, not the preceding clause. The Go parser has
  a latent off-by-one here that doesn't surface in its tests because
  no multi-clause guarded define is covered.
- **Defensive `val`:** define-body emissions route `(val X)` through
  a runtime `__val(x)` helper that is identity on primitives, so
  specs that apply `val` to an already-unwrapped accumulator (e.g.
  `(scanl (lambda A (lambda T (+ (val A) …))) (val B0) Txs)`) don't
  crash. Guard-path emission still uses the typed `.val()` form
  where the symbol table confirms the target is wrapped.

## File map

- `shengen.ts` — single-file CLI + library. Sections: AST, symbol
  table, S-expr parser, resolver, `verifiedToTs`, structural-match
  fallback, `parseDatatype` / `parseDefine`, `inferShenType` type
  propagator, `generateTs` emitter.
- `shengen.test.ts` — 65 tests covering parser, symbol table, codegen,
  runtime-imported generated modules.
- `package.json` / `tsconfig.json` — strict TS with
  `allowImportingTsExtensions`; no runtime deps.
