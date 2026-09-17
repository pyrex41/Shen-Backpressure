# JEV vs plain Ralph experiment

## Research question

Does a bounded JEV assessment help a small coding model repair a broad,
Shen-constrained application more efficiently than deterministic backpressure
alone?

The proposed benefit is investigation routing, not weaker verification. JEV
may identify the most informative failing obligation, but both arms must pass
the same deterministic gates. A JEV probability is never accepted as evidence.

## Experimental treatment

`run.sh` creates two isolated copies of the marketplace benchmark and runs a
hand-built Ralph loop against each using Pi with OpenAI `gpt-5.6-luna` at low
reasoning effort.

- `control`: deterministic gate failures only.
- `jev`: the identical prompt and failures plus an advisory `sb assess`
  projection. Acceptance gates are identical.

The projection names one investigation priority, one failure category, and the
probability that additional targeted investigation would be useful. The JEV arm
still receives the full deterministic failure output. JEV cannot skip a gate,
edit acceptance policy, or construct checked evidence.

## Controlled conditions

Each pair uses:

- the same immutable seeded source tree;
- the same initial prompt and maximum iteration count;
- the same Pi harness, Luna model, and reasoning effort;
- the same five ordered verification gates;
- fresh model sessions for every iteration;
- independent work directories; and
- hash checks protecting the Shen spec, generated guards, tests, manifest, and
  generator script.

The second pair reverses arm order to reduce a simple control-first ordering
effect. This is counterbalancing, not randomization.

## Loop sequence

For each arm and iteration, the runner:

1. Executes every deterministic gate and records the complete output.
2. Stops if all gates pass.
3. In the JEV arm only, submits the failure output and obligation frontier to
   `sb assess` and renders a compact advisory projection.
4. Constructs the Pi prompt from the common task, optional projection, and the
   same gate failures.
5. Runs Luna with editing tools enabled.
6. Records the Pi event stream, usage, cost, latency, and contract integrity.
7. Repeats until acceptance or the iteration budget is exhausted.

## Measurements

The primary outcomes are deterministic acceptance and model turns to
acceptance. Secondary measurements are Pi wall time, input/cache/output tokens,
Pi-reported cost, JEV latency and tokens, and immutable-contract violations.

Pi cost does not include TypeSafe/JEV billing. The raw JEV usage is retained so
it can be accounted for separately.

## Reproduction

The run records Pi JSON traces, token usage, cost, gate output, JEV assessments,
immutable-contract violations, and a paired summary beneath
`.experiments/jev-ralph/<timestamp>/`.

Environment:

```bash
JEV_API_KEY=... experiments/jev-ralph/run.sh
ARM_ORDER=jev-first MAX_ITER=4 experiments/jev-ralph/run.sh
```

The direct `openai` Pi provider requires a valid `OPENAI_API_KEY`. This runner
uses the authenticated `openai-codex` provider while preserving the exact
requested model ID, `gpt-5.6-luna`.

See [RESULTS.md](RESULTS.md) for the initial result and its limitations.
