# Pre-registered staged-frontier experiment

Status before execution: specified, not yet run.

## Motivation

The initial paired experiment exposed every failing assertion in one prompt.
Luna repaired all nine defects in one turn in both arms, producing a ceiling
effect. This follow-up tests the narrower claim that JEV can improve the order
in which a bounded verification frontier is investigated.

## Hypothesis

When only one failing evidence domain is projected into each repair turn, JEV
routing will reduce turns or model work to deterministic acceptance compared
with a simple deterministic first-failure scheduler.

## Arms

- **Control:** run every acceptance gate, then expose only the first failing
  gate in manifest order.
- **JEV:** run every acceptance gate, give the complete failure set to
  `sb assess`, then expose only the gate selected by JEV. If JEV chooses
  `targeted:new-check` or a non-failing gate, fall back to the first failing
  gate and record the fallback.

Both arms receive the selected gate name and its complete deterministic output.
They do not receive failures from other domains during that turn.

## Fixed conditions

- Benchmark seed: the same nine checked-in defects used by the initial test.
- Model: OpenAI `gpt-5.6-luna` through Pi's `openai-codex` provider.
- Reasoning effort: low.
- Maximum repair turns: four.
- Acceptance: all five existing deterministic gates pass.
- Fresh Pi session on every turn.
- Identical immutable-contract checks and repair prompt.
- Two paired runs, with arm order reversed in the second pair.

The prompt will instruct the model to repair the selected evidence domain and
avoid proactively repairing unrelated domains. This restriction is identical
in both arms.

## Outcomes

Primary:

1. deterministic acceptance within four turns;
2. repair turns to acceptance.

Secondary:

- Pi wall time;
- total Pi tokens and Pi-reported cost;
- number and order of selected gates;
- failures resolved per turn;
- JEV token and latency overhead;
- immutable-contract violations; and
- scheduler fallbacks.

## Interpretation rule

JEV is favored only if it improves the primary outcomes consistently across the
two counterbalanced pairs. Timing, tokens, or cost alone are exploratory at
this sample size. Equal acceptance and turn count is a null result even if one
arm happens to be faster.

## Known limitations

- Two pairs cannot estimate model variance reliably.
- The same defect seed is reused.
- Gate groups contain different numbers of defects.
- A model may inspect files outside the selected domain despite the prompt.
- The control scheduler is intentionally simple; a dependency-aware
  deterministic scheduler remains a necessary future baseline.
