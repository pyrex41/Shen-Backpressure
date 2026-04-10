# Side-by-side demo: shengen vs shen-derive

This directory contains a runnable demo showing what shen-derive
adds over shengen alone. It rotates three hand-picked buggy versions
of `internal/derived/processable.go` into place, runs the Go
compiler (the shengen-enforced type gate) and `make shen-derive-verify`
(the spec-equivalence gate), and records which gate caught each bug.

## TL;DR

Every bug compiles cleanly. shengen's guard types say nothing about
the *behavior* of `Processable` — only about the shape of its
parameters (`Amount` is non-negative, `Transaction` has valid fields).
shen-derive evaluates the Shen spec on 35 sampled inputs and fails
the build on any case where the hand-written Go disagrees with the
spec's output.

## Run it

```bash
cd examples/payment
./demo-shen-derive/run.sh
```

Expected summary (values vary in case counts depending on which
cases hit each bug):

```
bug                     go build        shen-derive verify
----------------------  --------------  ----------------------
bug1_sign_flip          PASS            FAIL
bug2_b0_truncate        PASS            FAIL
bug3_strict_zero        PASS            FAIL
```

The original `processable.go` is backed up to `.processable.orig`
during the run and restored on exit (including Ctrl+C, via `trap`).

## What the bugs are

### Bug 1 — Sign flip (`bug1_sign_flip.go.bak`)

```go
balance += tx.Amount().Val()  // BUG: should be -=
```

The most obvious possible bug. The Go compiler is happy — `+=` is
just as legal as `-=` on a `float64`. A hand-written unit test that
happens to only cover "sum of amounts ≤ balance" cases could even
pass, because the flipped-sign impl coincidentally returns `true` for
those.

**shen-derive result**: 14 cases disagreed with the spec. Every case
where the real running balance would have crossed zero returns the
opposite answer under the flipped sign.

### Bug 2 — Silent truncation of `b0` (`bug2_b0_truncate.go.bak`)

```go
balance := float64(int64(b0.Val())) // BUG: truncates fractional part
```

Someone (or an LLM) "optimized" the code to avoid float arithmetic,
ran a bunch of integer-only test cases, and shipped. The Go compiler
is happy: the double cast is legal, the types match, and `Amount`'s
`Val()` still returns a `float64`. Integer-only unit tests pass
cleanly.

**shen-derive result**: exactly one case disagrees — `case_25`, which
pairs `mustAmount(2.5)` as the initial balance with a transaction of
amount `2.5`. The spec computes `2.5 - 2.5 = 0`, which is `>= 0`, so
`true`. The buggy impl starts from `float64(int64(2.5)) = 2.0`,
subtracts `2.5`, gets `-0.5`, returns `false`.

This is the argument for constraint-aware sampling with boundary
values like `2.5` in the pool. A naive sampler using only
`{0, 1, 100}` would never notice the truncation bug.

### Bug 3 — Strict-zero boundary (`bug3_strict_zero.go.bak`)

```go
if balance <= 0 {  // BUG: should be `< 0`
    return false
}
```

The spec says every running balance must be `(>= X 0) : verified`.
Exactly zero is legal. The buggy impl treats zero as a failure.

**shen-derive result**: 5 cases disagree. All of them are cases where
the running balance lands exactly on zero — e.g. `b0=1` with
transactions that subtract to exactly zero, or `b0=0` with an empty
transaction list followed by a zero-amount transaction.

This is the argument for including `0` in the sample pool and for
generating composite variations that exercise the boundary of the
`verified` predicate.

## Why shengen can't catch any of these

shengen generates Go types (`Amount`, `Transaction`, `AccountId`)
with opaque fields and validating constructors. The guarantee is
*structural*: every value of type `Amount` is non-negative because
`NewAmount` is the only constructor and it returns an error if
`(>= X 0)` fails. That guarantee is enforced by the Go compiler:
you literally cannot create an `Amount` with a struct literal.

But none of the three bugs in this demo are about creating an
invalid `Amount`. They're about *what the function does with valid
Amounts*. Bug 1 flips the sign. Bug 2 truncates a valid float. Bug 3
rejects a valid zero. In every case the inputs are well-typed, the
outputs are well-typed, and the Go compiler has nothing to object to.

shengen is the wrong tool for function-behavior claims. That's what
shen-derive is for.

## Why the unit tests alone don't catch them either

The existing hand-written tests in `internal/derived` only cover a
handful of cases — whatever the author thought to write when they
first built the function. Bug 2 (truncation) in particular is almost
impossible to catch with hand-written tests unless you specifically
remember to use a fractional value *and* design the case so the
fractional part matters. Most humans don't. Neither do most LLMs.

shen-derive's sample pool is deliberately hostile in the ways that
matter: `0`, `1`, `-1` (filtered out for constrained types but
retained for unconstrained), `5`, `2.5` (the fractional), `100`.
Composite types get one variation per field-sample index, so the
fractional amount shows up inside a `Transaction` inside a
`[]Transaction`. The cartesian product across the parameter pool
is 35 cases. That's enough to catch all three bugs without hand-
written help.

## What this demo doesn't prove

shen-derive's guarantee is sample-based, not a global proof. The
three bugs above are all caught by the current sample pool, but a
bug that only manifests outside the pool would slip through. For
example, a bug that only fires when `b0 = 777.3` (not in the pool)
would pass.

Two answers to that:

1. **Seeded random sampling**: `shen-derive verify ... --seed 42
   --random-draws 16` layers 16 random draws on top of the boundary
   set per primitive type. With enough draws you'll probabilistically
   hit most out-of-pool cases.
2. **Native Go fuzzing**: the core evaluator can be used as an oracle
   inside a `func FuzzSpec_Processable(f *testing.F)` for deeper
   exploration under `go test -fuzz`. This isn't wired up yet but the
   harness shape supports it.

For the class of bugs this demo targets — sign flips, truncations,
boundary-condition off-by-ones — the deterministic boundary pool is
more than enough. That's the point: shen-derive makes the *obvious*
failure modes visible cheaply, so reviewers and CI can focus on the
interesting ones.

## Files

```
demo-shen-derive/
  DEMO.md                    this file
  run.sh                     the demo runner (rotates, builds, verifies, restores)
  bug1_sign_flip.go.bak      bug 1 full impl
  bug2_b0_truncate.go.bak    bug 2 full impl
  bug3_strict_zero.go.bak    bug 3 full impl
```

Each `.go.bak` file is a complete replacement for
`examples/payment/internal/derived/processable.go`. The demo script
copies it into place, runs the gates, and restores the original.
