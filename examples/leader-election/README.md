# leader-election

This is the running example from Reasonable's
["The internet discovers TLA+. Now what?"](https://reasonable.io/blog/tla-tutorial/),
written as a Shen state machine and checked with `shen-derive check`.
Three computers, a, b and c, must agree on a leader. The spec is
`specs/election.shen`. The design behind it is in
`thoughts/shared/research/2026-09-26-tla-style-state-machines.md`.

Build the checker first:

```sh
make -C ../.. build-shen-derive
alias sd=../../bin/shen-derive
```

`--no-deadlock` is used throughout because an election is meant to
finish. A finished election is a state with no successors, and TLC would
report it as a deadlock too.

## Safety: never two leaders

```sh
sd check specs/election.shen --inv one-leader --no-deadlock
# 38 distinct states, 57 transitions, depth 4
# OK
```

That is the 38 states the article reports. Now let a computer vote
twice (the article's level 2):

```sh
sd check specs/election.shen --inv one-leader --no-deadlock --const 'double-vote?=true'
# FAIL: invariant one-leader violated
#     0  <init>      ...
#     1  start a     ...
#     2  start b     ...
#     3  a votes b   ...
#     4  b votes a   ...
#     5  a wins      ...
#     6  b wins      [["leader", "leader", "follower"], ...]
```

## Liveness: someone is eventually leader

```sh
sd check specs/election.shen --no-deadlock --eventually has-leader
# FAIL: liveness <>has-leader violated
#     1  start a / 2  start b / 3  start c
#        (stays here forever: no action is enabled)
```

The vote can split: each computer votes for itself and nobody can win.
Letting split votes retry (`timeouts?`) turns the dead end into a
livelock:

```sh
sd check specs/election.shen --no-deadlock --eventually has-leader --const 'timeouts?=true'
#        (back to state 0, and repeats forever)
```

Fairness decides whether that loop counts:

- **Weak fairness on voting is not enough.** In the loop, votes become
  impossible once everyone has voted. An action that keeps getting
  disabled never has to happen under WF.
- **Strong fairness is enough.** Voting is enabled again and again, so
  SF says it must eventually happen.

```sh
sd check specs/election.shen --no-deadlock --eventually has-leader --const 'timeouts?=true' --wf '* votes *'
# FAIL (same livelock)
sd check specs/election.shen --no-deadlock --eventually has-leader --const 'timeouts?=true' --sf '* votes *'
# OK
```

The article's level 3 is a typo that stops anything useful from
happening. Safety can't see it, but liveness can:

```sh
sd check specs/election.shen --inv one-leader --no-deadlock --const 'timeouts?=true' --const quorum=4
# OK   (no two leaders, because there are no leaders)
sd check specs/election.shen --no-deadlock --eventually has-leader --const 'timeouts?=true' --const quorum=4 --sf '* votes *'
# FAIL: liveness <>has-leader violated
```

## Beyond linear time

The article notes that TLA+ cannot say "from any state, a new election
can still be started". That is the CTL property `AG EF can-start`.
Because `check` holds the whole state graph, it can check it:

```sh
sd check specs/election.shen --no-deadlock --possible can-start
# FAIL: possibility AG EF can-start violated: from the last state no behaviour ever reaches it
```

The model is a single-shot election, and once every computer has voted,
nothing can start a new one. That is true of this model, and it is the
kind of property the article says needs more than LTL.
