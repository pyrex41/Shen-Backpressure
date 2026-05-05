# Feature Design Prompt: Counterexample Traces

You are designing counterexample traces for Shen-Backpressure. This is a design task, not an implementation task. Produce an implementation-ready design document, but do not edit code.

## Context

Verification failures are only useful if the user can understand them. A gate failure that says "derive test failed" is less useful than a concrete witness: these inputs were generated from the Shen spec, the spec expected this result, the target implementation returned that result, and this command reproduces it.

Shen-Backpressure already has partial counterexample material:

- `shen-derive` evaluates Shen specs over generated samples.
- emitted Go/TS tests compare target implementation output to expected Shen oracle output.
- failed generated tests contain inputs, expected values, and actual values.
- guard constructors can fail on invalid values.
- shengen audit drift can identify generated artifact mismatch.

The goal is to make these failures first-class, structured, stable artifacts.

## Goal

Design **Counterexample Traces** for Shen-Backpressure.

A counterexample trace is a durable, machine-readable and human-readable artifact that explains why a verification obligation failed using concrete inputs, expected behavior, actual behavior, and reproduction instructions.

## Desired User Experience

When a derive check fails, the user should see something like:

```text
Counterexample: processable fractional amount

Spec:
- specs/core.shen :: processable

Input:
- balance: 0
- transactions:
  - amount: 2.5

Expected from Shen oracle:
- false

Actual from Go implementation:
- true

Why this matters:
- The implementation truncated 2.5 to 2 before checking balance.

Reproduce:
- go test ./internal/derived -run TestProcessableSpec/fractional_amount
```

The agent should receive a compact version in the next loop iteration.

## Design Requirements

### 1. Counterexample Schema

Design a JSON schema for `counterexamples.json` or embedded counterexamples inside `evidence.json`.

Each trace should include:

- `id`
- `source` (`derive`, `target_test`, `guard_constructor`, `shen_typecheck`, `audit`, future `prover`)
- `symbol`
- `spec_file`
- `spec_reference`
- `implementation_file`
- `implementation_reference`
- `input`
- `expected`
- `actual`
- `failure_message`
- `reproduction_command`
- `seed`
- `sample_index`
- `minimal`
- `related_evidence_id`

Support nested values such as guard types, lists, structs, maps, and sum types.

### 2. v0 Sources

Start with sources available today.

Design extraction for:

- `shen-derive` generated test failures.
- `shen-derive-ts` generated `node:test` failures.
- target test output when it includes generated test names.
- guard constructor failures surfaced in tests.
- drift failures where expected generated output differs from committed output.

Do not require reverse proof search in v0.

### 3. Test Naming and Reproduction

Counterexamples are only useful if they can be reproduced.

Design naming conventions for generated tests:

- stable case names
- sample index
- predicate/function name
- optional short hash of input

Examples:

```text
TestProcessableSpec/case_014_fractional_amount_2_5
test("processable case 014 fractional amount 2.5", ...)
```

Explain how the reproduction command is generated for Go and TypeScript.

### 4. Shrinking Strategy

Design shrinking as a staged feature.

v0 may not shrink. It can report the generated failing case as-is.

v1/v2 should consider:

- removing irrelevant list elements
- shrinking numbers toward boundaries
- simplifying strings
- simplifying sum-type variants
- preserving Shen type constraints while shrinking

Explain how Shen predicates can validate candidate shrinks.

### 5. Agent-Facing Explanation

Design a compact explanation format for `sb loop`.

It should tell the agent:

- the function or invariant that failed
- the exact input
- expected vs actual
- likely file to edit
- command to rerun

Avoid verbose proof theory. The agent needs a repair target.

### 6. Human-Facing Explanation

Design a richer markdown format for humans.

Include:

- spec excerpt or pointer
- implementation pointer
- generated input
- expected/actual
- why the case was generated, if known
- whether the case is minimal
- reproduction command

### 7. Future Prover Counterexamples

Design, but do not implement, future counterexamples from failed Shen proof search.

Consider:

- work backwards from failed premise
- enumerate bounded witnesses from datatype domains
- use Prolog relations to produce witness assignments
- ask an LLM to propose candidate witnesses and validate them with Shen
- import SMT/model-checker counterexamples

Stage this clearly as future work.

## Guardrails

- Do not claim every failure can produce a counterexample.
- Do not parse arbitrary test output with brittle regexes unless there is a stable generated-test convention.
- Do not hide the original target-language failure.
- Do not let the LLM fabricate counterexamples without validation.
- Do not require users to read generated code to understand a failing case.

## Deliverable

Write a detailed design document with:

1. Problem statement.
2. Counterexample JSON schema.
3. Source-specific extraction design for Go and TypeScript derive tests.
4. Test naming and reproduction strategy.
5. Human and agent output mockups.
6. Shrinking roadmap.
7. Future proof-search counterexample roadmap.
8. A concrete walkthrough using a failing `examples/payment/` `processable` case.
