# Initial paired results

## Test performed

Date: 2026-09-17. Model: OpenAI `gpt-5.6-luna`, low reasoning, via Pi's
authenticated `openai-codex` provider. Two paired runs were executed with arm
order reversed on the second run.

The benchmark began with nine independent defects distributed across tenant
authorization, inventory reservation, pricing, promotion scope, order state
transitions, cumulative refund accounting, shipment allocation, support-ticket
isolation, and risk-score bounds. The complete deterministic failure surface
was visible to both arms. The sole treatment difference was the JEV advisory
projection.

In both JEV runs, the assessment chose `gate:commerce-contracts` as the first
investigation with approximately 0.95 probability and classified the failures
as `implementation_defect` with probability 1.00. Each assessment took roughly
half a second and used 2,659 input and 180 output tokens.

## Observations

| Run | Arm | Passed | Model turns | Pi seconds | Total Pi tokens | Pi cost |
|---|---|---:|---:|---:|---:|---:|
| control-first | control | yes | 1 | 45 | 6,622 | $0.000465 |
| control-first | JEV | yes | 1 | 43 | 7,633 | $0.000416 |
| JEV-first | control | yes | 1 | 71 | 11,384 | $0.000799 |
| JEV-first | JEV | yes | 1 | 37 | 10,024 | $0.000338 |

Across these two pairs, both arms repaired all nine seeded defects in one model
turn with zero immutable-contract violations. Combined Pi wall time was 116s
for control and 80s for JEV. Combined Pi-reported cost was $0.001264 for
control and $0.000753 for JEV. These figures exclude TypeSafe/JEV billing; the
JEV requests and token usage are preserved in each run directory.

Both arms produced legitimate production-code patches and passed all five
acceptance gates. Neither arm modified a protected contract. Luna repaired all
nine defects in its first turn in every run.

## Conclusion

This is a **null result for repair success and iterations to acceptance**. The
test demonstrates that the JEV integration works end to end, but it does not
demonstrate that JEV improves convergence. The task exposed a broad but highly
legible set of failures simultaneously, and Luna was capable of repairing the
entire visible surface in one turn without routing assistance. The primary
outcomes therefore hit a ceiling in both arms.

The JEV arms used 80 combined Pi seconds versus 116 for control and had lower
combined Pi-reported cost. That difference is exploratory only. With two pairs,
fresh stochastic sessions, different response lengths, and materially different
prompt-cache reads, it cannot be attributed to JEV. TypeSafe/JEV cost is also
excluded from the Pi cost figures.

The appropriate claim is:

> JEV supplied stable, relevant routing while preserving deterministic
> acceptance, but this initial benchmark was too easy for routing to affect the
> measured repair outcome.

It would be incorrect to claim that JEV made the loop faster or cheaper from
these observations alone.

## Threats to validity

- Only two paired runs were performed.
- The model saw all failing tests at once, making search ordering largely
  unnecessary.
- The failures were compact and directly tied to individual functions.
- Both arms used fresh sessions, so provider sampling variance remains.
- Prompt-cache behavior differed substantially between calls.
- Arm order was reversed but not randomized.
- The same defect seed was reused.
- JEV billing was not available in the Pi cost measurement.
- The benchmark measures repair of known seeded defects, not discovery of
  unknown defects or false acceptance.

## Follow-up design

A more discriminating experiment should be specified before running it:

1. Create several independently seeded defect sets.
2. Stage or cap the visible obligation frontier per iteration so ordering can
   affect time to evidence.
3. Include noisy and misleading failures, not only direct assertion messages.
4. Add a deterministic non-JEV routing baseline, such as changed-file and gate
   dependency mapping.
5. Run enough randomized or counterbalanced pairs to estimate variance.
6. Include total JEV cost and latency in the comparison.
7. Keep acceptance gates and iteration/token budgets identical.
