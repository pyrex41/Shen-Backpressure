# shen-derive-ts

TypeScript port of `shen-derive`. Given a `.shen` spec file containing a
`(define ...)` block, generates a TypeScript test file that asserts a
hand-written implementation matches the spec pointwise on sampled inputs.

Parallel to `cmd/shengen-ts/` — same ESM / `node:test` module style.
Runtime is zero-install: Node 22.6+ strips TypeScript types on the fly
(`--experimental-strip-types`), and `./bin/shen-derive-ts` wraps the
entry point. `npm install` at this directory pulls in `typescript` +
`@types/node` strictly for `npm run typecheck` / `npm test`; the
generated wrapper never needs the installed deps at runtime.

See `shen-derive/DESIGN.md` for the overall design and
`site/content/posts/how-shen-derive-works/index.md` for the mechanism-level
walkthrough. The Go version at `shen-derive/` is the reference.

## Usage

```
./bin/shen-derive-ts verify <spec.shen> \
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

From this directory:

```
npm install   # once, for dev deps (typescript + @types/node)
npm test      # runs node --test --experimental-strip-types on *.test.ts
npm run typecheck
```
