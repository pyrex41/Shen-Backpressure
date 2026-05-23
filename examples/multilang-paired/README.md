# examples/multilang-paired — One Shen Spec, Four Languages

This example exists to answer a specific question raised on HN by
[`vrm`](https://news.ycombinator.com/) and `solomonb`:

> "Why not just use Rust newtypes / Liquid Haskell / Lean and skip this?"

The project's reply is "one Shen spec drives Go + TypeScript, build
catches drift." That claim used to rest on six static checked-in
files under `examples/payment/reference/`. After
[W2.2](../../thoughts/shared/research/2026-05-22-hn-feedback-next-steps.md)
the Python and Rust emitters became reachable from `sb gen` and
gained `(define …)` + `(list X)` coverage. W3.1 (this directory)
turns that capability into a **runnable** artifact: ONE
`specs/core.shen`, FOUR independent CLIs (Go, TypeScript, Python,
Rust), and a parity check that asserts the four CLIs produce
byte-identical JSONL on a shared input table.

```
        specs/core.shen
        ┌──────┬──────┬──────┬──────┐
        │      │      │      │      │
    shengen  shengen-ts  shengen-py  shengen-rs
        │      │      │      │
    guards_gen.go  .ts   .py    .rs
        │      │      │      │
       go/main  ts/main  py/main  rs/main
        │      │      │      │
        ▼      ▼      ▼      ▼
        └──── parity-check.sh ─── byte-equal JSONL?
```

## The contract

The spec models a shopping cart discount decision. A
`DiscountEligible` proof object exists exactly when a cart has at
least `min-items` line items AND a subtotal at least `min-subtotal`:

```shen
(datatype discount-eligible
  Cart : cart;
  ItemCount : number;
  MinSubtotal : number;
  MinItems : number;
  (>= (head Cart) MinSubtotal) : verified;
  (>= ItemCount MinItems) : verified;
  ============================================
  [Cart ItemCount MinSubtotal MinItems] : discount-eligible;)
```

`(head Cart)` resolves to `Cart.subtotal` via the destructure-by-
position lowering all four emitters share — see
[`examples/shen-web-tools/specs/core.shen:92`](../shen-web-tools/specs/core.shen)
and [`examples/payment/specs/core.shen:29`](../payment/specs/core.shen)
for prior art.

## Running the parity check

```console
$ cd examples/multilang-paired
$ ./bin/parity-check.sh
==> Building Go CLI...
==> Running Go CLI...
==> Running TS CLI (npx tsx)...
==> Running Py CLI (python3)...
==> Building Rs CLI (rustc, no crates)...
==> Running Rs CLI...
  PASS  Go == TS
  PASS  Go == Py
  PASS  Go == Rs
  PASS  TS == Py
  PASS  TS == Rs
  PASS  Py == Rs

OK: all 4 languages produced byte-identical JSONL on 20 fixture rows.
```

The 20 rows in `fixture-inputs.jsonl` cover:

- Happy paths (eligible carts) at exact-threshold and above-threshold
- Rejections from the subtotal premise (just-under, far-under)
- Rejections from the item-count premise (single-item, empty)
- Rejections from upstream constructor failures (negative subtotal,
  zero or fractional quantity)
- Edge cases around `(>= Qty 5)` for the bulk-line predicate

## What each CLI does

Each CLI reads JSONL from stdin, applies the generated guard
constructors, and emits one JSON decision row per input. The output
shape is

```json
{"caseId":"c01-eligible-minimal","ok":true,
 "outcome":"eligible","itemCount":2,"hasBulkLine":false}
```

`outcome` is one of `eligible`, `rejected-subtotal`, `rejected-items`,
`rejected-cart-build`, `rejected-item-build`. The error MESSAGES
differ across languages (Go: `cart.subtotal must be >= minSubtotal`;
Py: `cart.subtotal() must be >= min_subtotal`) — the parity check
compares the structured `outcome` field, not the message, which is
the portable contract.

## `sb gates` and the multi-language manifest

`sb.toml` declares `lang = "go"` in `[project]` because the
manifest's current schema models one canonical language per project.
The TS / Py / Rs emits are driven by additional `[[gates]]` entries
(`shengen-ts`, `shengen-py`, `shengen-rs`), each of which invokes the
language-specific codegen script in `bin/`. Each language also gets
its own `tcb-audit-<lang>` gate calling
`bin/shenguard-audit.sh --lang <lang>` to drift-check the committed
`guards_gen.<ext>`. A richer manifest schema where `[project]` carries
an array of language targets is a separate work item — see W3 in
[`thoughts/shared/plans/this-is-great-let-s-crispy-stallman.md`](../../thoughts/shared/plans/this-is-great-let-s-crispy-stallman.md).

`sb gates` runs all 13 gates in dependency order:

| Gate | Purpose |
|---|---|
| `shengen`              | Regenerate `go/multilang_paired/guards_gen.go` |
| `shengen-ts`           | Regenerate `ts/guards_gen.ts` |
| `shengen-py`           | Regenerate `py/guards_gen.py` |
| `shengen-rs`           | Regenerate `rs/guards_gen.rs` |
| `build-go`             | `go build ./...` |
| `build-ts`             | `npx tsc --noEmit` |
| `build-py`             | `python3 ast.parse(…)` on both files |
| `shen-check-datatypes` | `(tc +)` against `specs/datatypes-only.shen` |
| `tcb-audit-go`         | `shenguard-audit.sh --lang go` |
| `tcb-audit-ts`         | `shenguard-audit.sh --lang ts` |
| `tcb-audit-py`         | `shenguard-audit.sh --lang py` |
| `tcb-audit-rs`         | `shenguard-audit.sh --lang rs` |
| `parity`               | `bin/parity-check.sh` |

`shen-check-datatypes` runs against
`specs/datatypes-only.shen` (the datatype blocks from `core.shen`
stripped of the helper define) rather than against `specs/core.shen`
itself because the bundled Shen runtime rejects list-destructure
defines with arithmetic guards — the same limitation that takes down
`examples/payment/specs/core.shen`'s `processable` define under
`(tc +)`. The four shengen emitters parse and lower the define
without issue (verified by the `parity` gate); only the SHEN port's
strict tc+ rejects this idiom. See
[`thoughts/shared/research/2026-05-22-hn-feedback-next-steps.md`](../../thoughts/shared/research/2026-05-22-hn-feedback-next-steps.md)
section (b) for the wider state of multi-language emit coverage.

## Known emitter-coverage gaps

Two W2.2 emitter gaps surface in this demo's `hasBulkLine` helper.
Both have the same fix (small parser corrections in the emitter); the
W3.1 worktree deliberately does not touch the emitters so the demo
exercises today's actual capabilities, not a hypothetical hardened
version.

**Gap 1 — Go (`cmd/shengen`).** `generateDefineHelpers` only emits
defines that are reachable from a `verified` premise; free-standing
defines like `has-bulk-line?` aren't emitted. The Go CLI implements
`hasBulkLine` inline, mirroring the spec's clause-by-clause shape.

**Gap 2 — TS (`cmd/shengen-ts`).** The define parser at
`cmd/shengen-ts/shengen.ts:1129` searches for ` where ` in the
post-result remainder rather than in the full segment; this drops
the `where`-guard when it appears in the LAST clause of a define
written in Py-style (`pattern -> result where (guard)`). The
emitted `hasBulkLine` consequently returns `true` for any non-empty
list, ignoring the `qty >= 5` guard. The TS CLI implements
`hasBulkLine` inline.

**Py and Rs do call the generated `has_bulk_line(…)` helper directly**
— their emitters lower the `(>= Qty 5.0)` guard correctly. So this
demo also stress-tests the W2.2 closures (`(list X)` + `(define …)`
support in Py/Rs) end-to-end.

The behavioural contract is preserved by the inline-or-generated
split because each implementation computes the same boolean
`qty >= 5` test per item; the parity check enforces the contract.

## File layout

```
multilang-paired/
├── README.md                        — this file
├── sb.toml                          — 13-gate manifest (project lang=go, others via [[gates]])
├── specs/
│   ├── core.shen                    — the spec the four emitters consume
│   └── datatypes-only.shen          — same spec minus the helper define (shen-check fodder)
├── fixture-inputs.jsonl             — 20-row shared input table
├── bin/
│   ├── codegen-{go,ts,py,rs}.sh     — per-language regen scripts
│   ├── check-py.sh                  — ast.parse smoke gate
│   └── parity-check.sh              — runs all four CLIs and pairwise-diffs
├── go/
│   ├── go.mod
│   ├── main.go                      — CLI
│   └── multilang_paired/guards_gen.go
├── ts/
│   ├── tsconfig.json
│   ├── package.json
│   ├── main.ts                      — CLI
│   └── guards_gen.ts
├── py/
│   ├── main.py                      — CLI
│   └── guards_gen.py
└── rs/
    ├── main.rs                      — CLI
    └── guards_gen.rs
```

## What this answers, what it doesn't

**Answers:** "Can one spec really drive four languages?" Yes — the
build now catches drift on all four via per-language `tcb-audit-*`,
and the runtime contract is verified by `bin/parity-check.sh` across
20 fixture rows.

**Does not answer:** "Why not just use the host language's existing
type system (Liquid Haskell, Rust newtypes, Lean)?" Per the HN
follow-up, the answer there is composition cost — a single Shen
spec is the shared source for systems that already span multiple
languages (e.g. a Go backend + a TS frontend), and the spec serves as
the audit surface for both. Using Liquid Haskell would require
porting the spec into the host system, breaking the
one-spec-many-consumers premise.

## Further reading

- HN feedback synthesis:
  [`thoughts/shared/research/2026-05-22-hn-feedback-next-steps.md`](../../thoughts/shared/research/2026-05-22-hn-feedback-next-steps.md)
- W3.1 plan section:
  [`thoughts/shared/plans/this-is-great-let-s-crispy-stallman.md`](../../thoughts/shared/plans/this-is-great-let-s-crispy-stallman.md)
- Structural emitter parity (Go integration test):
  [`cmd/shengen/parity_test.go`](../../cmd/shengen/parity_test.go)
