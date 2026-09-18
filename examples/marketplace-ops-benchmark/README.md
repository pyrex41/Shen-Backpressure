# Marketplace operations Ralph/JEV benchmark

## Purpose

A deliberately broad, seeded-defect application used to compare a plain
hand-built Ralph loop with the same loop augmented by advisory JEV routing.
The Shen specification spans 28 datatypes across identity, catalog, inventory,
pricing, orders, payments, shipping, support, risk, idempotency, and audit.

This is an experimental fixture, not a reference implementation. The checked-in
production functions intentionally contain defects. Passing tests would mean
the seed has accidentally been repaired and the experiment is no longer
starting from its declared baseline.

## Surface area

The fixture contains a 203-line Shen specification, generated Go guard types,
and application contracts covering:

- tenant membership and cross-tenant isolation;
- inventory availability and reservation accounting;
- per-unit pricing and basis-point discounts;
- tenant-scoped, usage-limited promotions;
- legal order state transitions;
- capture and cumulative-refund bounds;
- multi-warehouse allocation after reservations;
- private support-ticket access;
- bounded risk scoring; and
- request idempotency.

Nine defects are seeded across `internal/app/tenant.go`, `commerce.go`, and
`workflow.go`; `services.go` contains shared domain types. The split permits
experiments to enforce domain-scoped repair turns. The tests and Shen spec are
treated as immutable acceptance contracts during an experiment.

## Verification topology

The five gates are:

1. `shengen-drift` — regenerate guards from the Shen spec and reject drift.
2. `tenant-contracts` — authorization and support isolation.
3. `commerce-contracts` — inventory, pricing, promotions, refunds, and shipping.
4. `workflow-contracts` — order transitions, risk, and idempotency.
5. `build` — compile every Go package.

The gate topology intentionally separates evidence domains so an investigation
router has meaningful bounded choices. All gates remain mandatory in both
experimental arms.

## Seeded-defect baseline

At baseline, guard generation and compilation pass while nine behavioral
contract tests fail. This models the architectural boundary under study:
structural Shen-generated types remain valid, but hand-written behavior can
still violate cross-field, cumulative, or adapter-level obligations.

## Running the experiment

Run the paired experiment from the repository root:

```bash
experiments/jev-ralph/run.sh
```

The runner copies this fixture into isolated work directories, so the checked-in
seeded defects remain unchanged.

The methodology and conclusions live in
[`experiments/jev-ralph`](../../experiments/jev-ralph/README.md).
