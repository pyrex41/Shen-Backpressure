# tla-mutex

A TLA+-style state-machine spec written in Shen, checked two ways:
exhaustively against the design, and against traces from a real Go
implementation. See
`thoughts/shared/research/2026-09-26-tla-style-state-machines.md` for the
design behind it.

```
specs/mutex.shen        the spec: init, next, invariants (lock taken atomically)
specs/mutex-racy.shen   the same design with check-then-take in two steps
impl/main.go            two goroutines on a real lock, writing a JSONL trace
```

## Check the design

This runs a breadth-first search over every reachable state, as TLC does.

```sh
make -C ../.. build-shen-derive          # builds bin/shen-derive
../../bin/shen-derive check specs/mutex.shen \
  --inv mutex,lock-matches-crit --step-inv release-by-holder
# 8 distinct states, 14 transitions, depth 3
# OK: every reachable state satisfies the invariants and every temporal property holds

../../bin/shen-derive check specs/mutex-racy.shen --inv mutex
# FAIL: invariant mutex violated
#     0  <init>         ["idle", "idle", false]
#     1  p1             ["want", "idle", false]
#     2  p1             ["saw-free", "idle", false]
#     3  p2             ["saw-free", "want", false]
#     4  p2             ["saw-free", "saw-free", false]
#     5  p1             ["crit", "saw-free", true]
#     6  p2             ["crit", "crit", true]
```

## Check the implementation

Record a run, then check that every recorded step is one the spec
allows.

```sh
go run ./impl > /tmp/good.jsonl
../../bin/shen-derive trace specs/mutex.shen --trace /tmp/good.jsonl \
  --inv mutex,lock-matches-crit --step-inv release-by-holder
# OK: 1201-step trace is a behaviour of the spec

go run ./impl -racy > /tmp/bad.jsonl
../../bin/shen-derive trace specs/mutex.shen --trace /tmp/bad.jsonl
# FAIL: trace step 673: action "p2" does not take ["crit", "want", true] to ["crit", "crit", true]
```

The racy run is nondeterministic. It is usually caught within a few
hundred steps. If it isn't, run it again or raise `-rounds`. A passing
trace is evidence about one interleaving. The `check` run above is the
exhaustive result for the design.

Exit codes for both commands:

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | violation |
| 2 | usage or spec error |
| 3 | `check` stopped at `--max-states` without finishing |
