# shengen-rs — Experimental / WIP

> **Status: experimental.** Not dispatched by `sb gen`. Not part of the verified hot path.
>
> The Go (`cmd/shengen/`) and TypeScript (`cmd/shengen-ts/`) shengen variants are the supported codegen targets. Rust shengen exists as a reference implementation produced via `/sb:create-shengen` (see top-level README) and has not been continuously validated against the spec surface used in `examples/payment/`, `examples/multi-tenant-api/`, or `examples/shen-web-tools/`.
>
> Despite the `.py` filename, this emits Rust — it is a Python script that generates Rust code. The naming mirrors `shengen-py` for symmetry.

## What it does

Generates Rust guard types from Shen sequent-calculus specs. Supports `--mode standard` (struct + impl with validating `new`) and `--mode hardened` (leverages Rust's move semantics to make proofs single-use — see `examples/payment/reference/rs_hardened/` for output shape).

## Usage (direct, not via sb)

```bash
python3 cmd/shengen-rs/shengen.py path/to/spec.shen --out path/to/out/guards_gen.rs [--mode standard|hardened] [--mod shenguard]
```

## What's missing vs. Go/TS

- Not wired into `sb gen` dispatch.
- No integration tests in the focused examples tree. Existing Rust outputs under `examples/payment/reference/rs/` and `examples/payment/reference/rs_hardened/` are the reference.
- Hardened mode is arguably the most interesting target language (ownership turns proofs into linear tokens) but has no end-to-end demo yet.

## If you want to graduate it

1. Mirror the dispatch in `cmd/sb/gen.go` the way Go and TS shengen are wired.
2. Add a `[[derive.specs]] lang = "rs"` integration test.
3. Rewrite in Go for static-binary stdlib-only packaging.
