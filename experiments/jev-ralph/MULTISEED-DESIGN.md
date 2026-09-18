# Pre-registered multi-seed scheduler experiment

Status before execution: specified, not yet run.

## Question

Can JEV choose a higher-value next repair domain than a dependency-aware,
deterministic scheduler when the active defect surface changes across seeds?

## Structural staging

The application is split into `tenant.go`, `commerce.go`, and `workflow.go`.
During each turn, edits outside the selected domain are detected, restored from
the seed, and counted. Tests, specs, generated guards, and gate definitions
remain immutable.

## Seeds

- **all-domains:** tenant 2, commerce 5, workflow 2 failures.
- **tenant-workflow:** commerce is pre-repaired; tenant 2 and workflow 2 remain.
- **commerce-workflow:** tenant is pre-repaired; commerce 5 and workflow 2 remain.

## Schedulers

The deterministic dependency-aware baseline scores each failing gate by failing
contract tests per editable domain file. Ties break by security severity
(`tenant`, `commerce`, `workflow`). JEV sees the complete failure set through
`sb assess`; a passing or non-gate choice falls back to the deterministic
scheduler.

## Fixed protocol

- OpenAI `gpt-5.6-luna`, low reasoning, Pi `openai-codex` provider.
- Fresh session per turn and maximum two turns per arm/seed.
- Only the selected gate output is shown and only its domain file may retain
  edits.
- All five gates determine acceptance after every turn.
- Scheduler order alternates by seed.

## Pre-registered oracle

- all-domains → commerce (5 failures);
- tenant-workflow → tenant (2 versus 2, security tie-break);
- commerce-workflow → commerce (5 versus 2).

## Outcomes and interpretation

Primary: failing contract tests removed after the first turn. Secondary:
acceptance within two turns, turns, scheduler agreement with the oracle, Pi
usage, JEV overhead, fallbacks, and rejected cross-domain edits.

JEV is favored only if it removes more first-turn failures or reaches acceptance
in fewer turns across seeds. Equal outcomes are null. The deterministic
scheduler is favored if JEV deviates from the oracle and resolves fewer
obligations. Time and cost are secondary.
