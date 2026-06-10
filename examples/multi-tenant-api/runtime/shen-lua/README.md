# shen-lua tier: the decidable fragment on the real Shen kernel

Every other tier in the policy lattice is a **lowering** of `specs/core.shen`:

```
Cedar (SMT)  ⊂  Rego (terminating)  ⊂  Decidable-Shen-fragment  ⊂  full-TC pure-Shen
   lowering        lowering              ← THIS TIER, not a lowering →
```

Even the in-process "pure-shen" evaluator in `cedar-verify` is a Go
re-implementation (`policyspec.EvalClauses`) of Shen evaluation — a lowering
that could drift, sitting inside the gate built to catch lowering drift.

This tier removes the re-implementation: `policy.lua` loads `specs/core.shen`
**itself** into [shen-lua](https://github.com/pyrex41/shen-lua) (a certified
Shen 41.1 port) and answers each authorization request by asking the kernel's
sequent-calculus typechecker whether the request's ground term **inhabits the
access type**:

```
allow(tenant, P, T, isMember)   :<=>   [principal T isMember] : tenant-access
```

The spec's `: verified` premises are discharged by one extra datatype rule:

```shen
(define sb.holds?
  Premise -> (trap-error (= true (eval Premise)) (/. E false)))
(datatype sb-discharge-verified
  if (sb.holds? Premise)
  ______________________
  Premise : verified;)
```

a ground premise holds iff it evaluates to true. This is where the
**decidable-fragment certification** (`sb policy --decidable`) earns its keep:
the fragment admits no recursion and only total ground forms (`=`, `not`,
comparisons, `element?`), so `eval` of every premise terminates — the cert is
the termination argument for the runner. Outside the fragment this tier must
not be used; that is the meaning of "decidable runtime variant".

What you get over the sketch (`checkDecidableFragment` is a syntactic
approximation; this is the real judgment):

- **No lowering in the TCB.** The policy evaluated at runtime is byte-for-byte
  the spec the auditor read.
- The full proof chain runs per request: the W2.1 cross-field binding
  `(= User (head (head Jwt)))`, the non-empty signature, exp > 0 — not just
  the membership/ownership flags the flat Cedar/Rego fragment covers.
- Measured cost: ~270 µs per `resource-access` check (full chain, allow path),
  ~70 µs for a `tenant-access` deny — OPA-class latency, from a kernel that
  warm-boots in ~30 ms and embeds anywhere Lua runs (OpenResty, Envoy,
  nginx — places Cedar/Rego need a sidecar).

## Run it

```sh
# serve the line protocol (what cedar-verify spawns)
SHEN_LUA_DIR=~/projects/shen/shen-lua luajit policy.lua

# benchmark
SHEN_LUA_DIR=~/projects/shen/shen-lua luajit policy.lua --bench
```

`SHEN_LUA_DIR` points at a shen-lua checkout; alternatively
`luarocks install shen` puts the `shen` module on the default package.path.

## The differential

`cedar-verify` auto-detects the tier (like the `cedar`/`opa` binaries) and adds
it to the n-way matrix:

```sh
cd ../.. && SHEN_LUA_DIR=~/projects/shen/shen-lua make cedar-verify
# look for: shen-lua vs guard mismatches: G-S+=0 G+S-=0
```

`G-S+` (guard denies, kernel allows) is the privilege-escalation direction and
gates strictly, same as the other tiers. Current state: **48/48 agreement,
0 mismatches** — the kernel typechecker and the generated guard constructors
implement the same judgment.

## Protocol

One request per line on stdin, tab-separated; fail-closed on anything
malformed:

```
PING                                                    -> READY
CHECK <level> <principal> <tenant> <resource> <member> <owned>  -> allow | deny
QUIT
```
