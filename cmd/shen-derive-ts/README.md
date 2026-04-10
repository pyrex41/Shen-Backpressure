# shen-derive-ts

TypeScript port of `shen-derive`. Given a `.shen` spec file containing a
`(define ...)` block, generates a TypeScript test file that asserts a
hand-written implementation matches the spec pointwise on sampled inputs.

Parallel to `cmd/shengen-ts/` — same `npx tsx` / ESM / `node:test` module
style, no build step, no npm install at this directory. Consumers invoke
via a relative path.

See `shen-derive/DESIGN.md` for the overall design and
`site/content/posts/how-shen-derive-works/index.md` for the mechanism-level
walkthrough. The Go version at `shen-derive/` is the reference.

## Usage

```
npx tsx cmd/shen-derive-ts/shen-derive.ts verify <spec.shen> \
  --func <shen-define-name> \
  --impl-module <relative TS path, e.g. ./processable> \
  --impl-func <exported TS function name> \
  --import <shengen-ts guards module path, e.g. ./guards_gen> \
  [--import-alias shenguard] \
  [--out <test file path>] \
  [--max-cases 50] \
  [--seed 0] \
  [--random-draws 0]
```

`--impl-module` is a **relative TS import path** (not a package import).
This differs from the Go version's `--impl-pkg`: Go imports by module path,
TS imports by relative file path.

## Tests

```
npx tsx --test cmd/shen-derive-ts/core/*.test.ts cmd/shen-derive-ts/specfile/*.test.ts cmd/shen-derive-ts/verify/*.test.ts
```
