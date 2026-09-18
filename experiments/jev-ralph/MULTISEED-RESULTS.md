# Multi-seed scheduler results

Date: 2026-09-18. Protocol:
[MULTISEED-DESIGN.md](MULTISEED-DESIGN.md). Model: OpenAI
`gpt-5.6-luna`, low reasoning, through Pi's `openai-codex` provider.

## Primary results

| Seed | Scheduler | First choice | Oracle choice | Failures before | Failures after turn 1 | Removed | Accepted in 2 turns |
|---|---|---|---|---:|---:|---:|---:|
| all-domains | deterministic | commerce | commerce | 9 | 4 | 5 | no |
| all-domains | JEV | tenant | commerce | 9 | 7 | 2 | no |
| tenant-workflow | deterministic | tenant | tenant | 4 | 2 | 2 | yes |
| tenant-workflow | JEV | tenant | tenant | 4 | 2 | 2 | yes |
| commerce-workflow | deterministic | commerce | commerce | 7 | 2 | 5 | yes |
| commerce-workflow | JEV | commerce | commerce | 7 | 2 | 5 | yes |

The deterministic scheduler matched the pre-registered density oracle on all
three seeds. JEV matched on two. Across seeds, deterministic scheduling removed
12 failing obligations after the first repair turn; JEV removed 9.

Both schedulers accepted two of three seeds within the two-turn budget. On the
all-domain seed, both repaired two domains and left the two workflow failures,
but deterministic removed five failures on turn one while JEV removed two.

No cross-domain edits were rejected, no scheduler fallback occurred, and the
protected contracts remained unchanged.

## JEV decisions

JEV selected:

- all-domains: tenant at 0.69, then commerce at 0.94;
- tenant-workflow: tenant at 0.87, then workflow at 0.99;
- commerce-workflow: commerce at 0.94, then workflow at 0.99.

This shows internally coherent contraction behavior, but the all-domain choice
optimized neither failure density nor first-turn evidence gain.

## Secondary measurements

| Scheduler | Pi seconds | Pi total tokens | Pi-reported cost |
|---|---:|---:|---:|
| deterministic | 149 | 21,841 | $0.002035 |
| JEV | 178 | 21,275 | $0.002415 |

The six JEV calls added 2.924 seconds of API latency, 16,057 JEV input tokens,
and 1,080 JEV output tokens. TypeSafe/JEV billing is not included in Pi cost.

These measurements remain secondary, but they provide no compensating
efficiency advantage in this run: JEV used more wall time and higher
Pi-reported cost, despite slightly fewer Pi tokens, before accounting for its
own token usage.

## Conclusion

Under the pre-registered interpretation rule, this experiment favors the
dependency-aware deterministic scheduler.

JEV did not improve acceptance within budget. It made one lower-value first
choice, causing the JEV arms to resolve 25% fewer failing obligations on the
primary first-turn outcome (9 versus 12). On the other seeds it reproduced the
deterministic choice rather than surpassing it.

The appropriate claim is narrow:

> When gate value was available as a cheap deterministic failure-density
> calculation, JEV added cost and latency without improving scheduling, and on
> the heterogeneous all-domain frontier it prioritized the lower-yield tenant
> gate over the higher-yield commerce gate.

This does not show that JEV is generally unhelpful. It shows that it should not
replace deterministic scheduling when dependency, scope, and expected evidence
gain are already computable. A more plausible role is handling ties or
classifying cases where deterministic metadata cannot distinguish candidate
investigations.

## Limitations

- One run per scheduler/seed cannot estimate model variance.
- Failure count is a crude value metric and does not encode severity.
- The tenant tie-break explicitly values isolation risk, but failure density
  dominates non-tied choices.
- Seeds share the same application and tests.
- Seed pre-repairs may make diagnostics cleaner than organic repository states.
- The two-turn budget intentionally makes ordering visible but is artificial.
