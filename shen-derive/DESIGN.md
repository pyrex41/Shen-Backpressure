# shen-derive: Design Notes

## Architecture

shen-derive has two layers with different portability characteristics:

### Derivation engine (language-agnostic)

The spec parser, type checker, rewrite engine, law catalog, and
side-condition discharge logic live in `core/`, `laws/`, and `shen/`.
None of these packages know about Go as a target language. They operate
on a typed lambda calculus AST with list/tuple combinators and apply
named algebraic laws from the Bird-Meertens catalog.

### Code generator (language-specific)

`codegen/` lowers the AST to Go source code. This is where
language-specific judgment lives: `foldr` becomes a reverse-iterating
`for` loop, `foldl` becomes `for _, x := range`, `map` becomes a
`make` + indexed loop, etc. Producing idiomatic target code requires
understanding the target language's idioms — these aren't mechanical
translations.

## Future direction: multi-language support

The natural path to supporting multiple target languages is:

1. **Port the derivation engine to Rust** as a shared library, exposed
   via C FFI / PyO3 / napi / wasm as needed.
2. **Keep codegen as thin, per-language backends.** Each backend can
   live in its target language (a Go backend in Go calling the Rust
   core via CGo, a Python backend in Python via PyO3, etc.).

The codegen layer is small (~600 lines for Go) relative to the core
(~3600 lines), so the duplication cost per language is low. The value
is in producing idiomatic output: Rust backends would emit
`iter().fold()`, Python would emit comprehensions, C would emit
explicit index loops with malloc.

**Current guidance:** don't port yet. The derivation engine API is
still stabilizing — the law catalog is small (3 laws), the
side-condition prover is heuristic-only for quantified obligations, and
usage patterns are still emerging. Porting now means guessing at
abstractions. Keep iterating in Go, and when the core API settles,
the port will be straightforward because the core packages are already
free of Go-specific assumptions.

## Package layout

```
shen-derive/
  core/       Typed lambda calculus AST, parser, evaluator, type checker
  laws/       Rewrite rule catalog, pattern matching, substitution
  shen/       Shen runtime bridge for side-condition validation
  codegen/    Go code generator (the only Go-specific package)
  runtime/    Helper types for generated Go code
  demo/       End-to-end derivation examples
  main.go     CLI entry point
```
