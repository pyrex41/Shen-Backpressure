# shengen-py — Experimental / WIP

> **Status: experimental.** Not dispatched by `sb gen`. Not part of the verified hot path.
>
> The Go (`cmd/shengen/`) and TypeScript (`cmd/shengen-ts/`) shengen variants are the supported codegen targets. Python and Rust shengen exist as reference implementations produced via `/sb:create-shengen` (see top-level README) and have not been continuously validated against the spec surface used in `examples/payment/`, `examples/multi-tenant-api/`, or `examples/shen-web-tools/`.

## What it does

Generates Python guard types from Shen sequent-calculus specs. Supports `--mode standard` (dataclass with validating `create`) and `--mode hardened` (closure-based "vaults" — see `examples/payment/reference/py_hardened/` for output shape).

## Usage (direct, not via sb)

```bash
python3 cmd/shengen-py/shengen.py path/to/spec.shen --out path/to/out/guards_gen.py [--mode standard|hardened]
```

## What's missing vs. Go/TS

- Not wired into `sb gen` dispatch — the `lang = "py"` field in `sb.toml`'s `[[derive.specs]]` is recognized only for `shen-derive`, not for `shengen`.
- No integration tests in the focused examples tree. Existing Python outputs under `examples/payment/reference/py/` and `examples/payment/reference/py_hardened/` are the reference.
- Hardened mode surface is smaller than Go/TS equivalents (no equivalent to Rust's move-semantics or Go's branded strings — see the enforcement-spectrum posts in `../my-blog/drafts/` for the nuance).

## If you want to graduate it

1. Mirror the dispatch in `cmd/sb/gen.go` the way Go and TS shengen are wired.
2. Add a `[[derive.specs]] lang = "py"` integration test against a real example.
3. Rewrite in Go (same as `cmd/shengen-ts/`) if you want to keep `sb` static-binary and stdlib-only.
