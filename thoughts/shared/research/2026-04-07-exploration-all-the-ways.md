---
date: 2026-04-07T00:00:00Z
researcher: reuben
git_commit: pre-wave-cleanup
branch: main
repository: pyrex41/Shen-Backpressure
topic: "Exploration — all the directions for using Shen as compile-time backpressure"
tags: [research, exploration, examples, shengen-directions, pattern-catalog]
status: archived-reference
last_updated: 2026-04-16
last_updated_by: reuben
last_updated_note: "Moved from root EXPLORATION.md during demo-readiness cleanup. Several of the demos listed here have since been archived to examples/.archive/ as part of focusing the tree; this doc remains as historical reference for the directions that were scoped out."
---

# Shen Backpressure — Exploration: All The Ways

This document maps every direction we've identified for leveraging Shen's sequent-calculus type system as **compile-time backpressure** against invalid states. Each section describes a concept, links to its demo spec, and explains what invariant becomes *impossible to violate*.

---

## The Core Insight

Traditional systems validate at runtime: "check if X is valid, throw if not." Shen backpressure inverts this: **invalid states cannot be constructed**. The compiler itself becomes the enforcement mechanism, not tests, not linters, not code review.

Every example below follows the same pattern:
1. **Encode invariants in Shen** — sequent-calculus `datatype` rules
2. **Generate opaque guard types** — private fields, constructor-only creation
3. **Proof chains** — constructing type B requires having type A first
4. **Five-gate verification** — shengen → test → build → tc+ → audit

---

## New Demo Specs (Built)

### 1. Circuit Breaker & Resilience Patterns
**`demo/circuit-breaker/specs/core.shen`**

State machine for circuit breakers where **invalid transitions are type errors**:
- CLOSED → OPEN requires `threshold-breached` proof (failures >= threshold)
- OPEN → HALF_OPEN requires `cooldown-elapsed` proof (time check)
- HALF_OPEN → CLOSED requires `probe-success` proof
- HALF_OPEN → OPEN requires `probe-failure` proof

Also includes composable resilience primitives:
- **Rate limiting**: `rate-allowed` proof (count < limit)
- **Bulkhead**: `bulkhead-permit` proof (active < max concurrent)
- **Composite guard**: `resilience-cleared` requires ALL three proofs

**What becomes impossible**: Opening a circuit without enough failures. Closing without a successful probe. Sending requests through an open circuit.

---

### 2. RBAC & Capability-Based Authorization
**`demo/rbac-capabilities/specs/core.shen`**

Authorization as a proof chain: `Identity → Session → RoleBinding → Capability → AccessGrant`

- **Sessions** carry expiry proofs (time-bounded)
- **Roles** are set-membership types (`admin | editor | viewer | auditor`)
- **Capabilities** derived from role bindings + permission checks
- **Access grants** require capability + same-org proof
- **Delegation** lets users grant subsets of their capabilities (time-bounded)
- Every access produces an `audit-entry`

**What becomes impossible**: Privilege escalation (can't construct admin capability without admin role binding). Cross-org access (access grant requires same-org proof). Using expired sessions.

---

### 3. Data Pipeline with Schema Evolution
**`demo/data-pipeline/specs/core.shen`**

Typed ETL/streaming pipelines where schemas are first-class:
- **Schema versions** with compatibility proofs (same major, minor ≤ target)
- **Stage contracts** declare input/output schemas
- **Composition proof**: stage B follows stage A only if schemas align
- **Checkpoints** with exactly-once resume proofs (new offset > last)
- **Dead letter queue** entries carry proof of which stage rejected them

**What becomes impossible**: Processing data with the wrong schema. Composing incompatible pipeline stages. Replaying already-processed records.

---

### 4. Workflow Saga — Distributed Transaction Proofs
**`demo/workflow-saga/specs/core.shen`**

The Saga pattern with proof-carrying steps:
- **Forward steps** produce `step-completed` proofs with ordering
- **Compensations** require the corresponding forward-step proof
- **Reverse ordering** enforced: compensate step N before step N-1
- **Saga completion** requires ALL steps to have proofs
- **Rollback proof** requires compensating all completed steps
- **Idempotency keys** prevent double-execution
- **Deadline proofs** enforce timeout constraints

**What becomes impossible**: Compensating a step that never ran. Skipping compensation steps. Completing a saga with missing steps. Double-executing a step.

---

### 5. AI Output Grounding — Anti-Hallucination Types
**`demo/ai-grounding/specs/core.shen`**

Makes hallucination a **type error** for any RAG/agent system:
- **Fetched documents** carry URL + content + timestamp
- **Grounded citations** must reference actual fetched documents
- **Grounded summaries** require ≥1 source document
- **Fact-checked claims** need supporting citations
- **Confidence calibration** ties confidence scores to source count
- **Relevant retrieval** gates on relevance score ≥ threshold
- **Tool justification** requires stated reason + user intent
- Full **RAG pipeline proof**: retrieve → ground → calibrate → render

**What becomes impossible**: Rendering ungrounded AI output. Claiming high confidence with insufficient sources. Using tools without justification. Citing unfetched documents.

---

### 6. Feature Flags with Safety Proofs
**`demo/feature-flags/specs/core.shen`**

Feature flags with dependency and environment constraints:
- **Dependencies**: enabling feature X requires proof that dependency Y is enabled
- **Environment gates**: experimental features blocked from production
- **Gradual rollout**: percentage-based with user cohort hashing
- **Safe rollback**: can only disable a feature if no active features depend on it
- Full evaluation: `gated-activation` + `user-in-rollout` = `flag-evaluated`

**What becomes impossible**: Enabling a feature without its dependencies. Deploying experimental features to production. Rolling back a feature that others depend on.

---

### 7. Consensus & Quorum Protocol
**`demo/consensus-quorum/specs/core.shen`**

Voting/approval workflows with quorum enforcement:
- **Voter eligibility** proofs (authorized to vote)
- **Vote uniqueness** proofs (no double-voting)
- **Valid votes** require both eligibility + uniqueness
- **Quorum proof**: total votes ≥ required threshold
- **Approval**: quorum reached + majority approvals
- **Execution**: can only execute approved proposals
- **Veto power**: special override with mandatory reason

**What becomes impossible**: Double-voting. Acting without quorum. Executing rejected proposals. Vetoing without stating a reason.

---

### 8. Immutable Audit Trail & Compliance
**`demo/audit-trail/specs/core.shen`**

Hash-chained audit logs with tamper detection:
- **Action classification**: create/read/update/delete/approve/reject/escalate
- **Sensitivity levels**: public/internal/confidential/restricted
- **Hash chain**: each entry links to previous via hash
- **Chain continuity proofs**: entries must be time-ordered and hash-linked
- **Retention policies**: entries within retention period (proven)
- **Compliance reports**: require complete, unbroken chains
- **Access control**: reading audit logs requires elevated role
- **Tamper detection**: chain verification proofs

**What becomes impossible**: Creating entries out of order. Breaking the hash chain. Generating compliance reports from incomplete chains. Accessing audit logs without authorization.

---

## Existing Demos (Already Built)

| Demo | Domain | Key Invariant |
|------|--------|---------------|
| `demo/payment/` | Payment processor | Balance ≥ transaction amount |
| `demo/email_crud/` | Email campaigns | Profile required before personalized copy |
| `demo/dosage-calculator/` | Medical dosage | Drug interactions + dose range safety |
| `demo/multi-tenant-api/` | SaaS tenant isolation | Cross-tenant access impossible |
| `demo/shen-web-tools/` | Research assistant | AI output grounded in fetched pages |
| `demo/order-state-machine/` | E-commerce | State ordering enforced |

---

## Beyond Specs: Broader Leverage Points

### A. Shen as Verifiable Interlingua
One spec → guard types in Go, TypeScript, Rust, Python. Change the spec, all implementations must adapt. This is the "N+1 languages" solution.

### B. LLM Taming via Compiler Pressure
The Ralph loop creates genuine backpressure: LLM proposes → compiler rejects → failures fed back → LLM adapts. This is deductive (Shen tc+) + empirical (tests) pressure, not vibes.

### C. Linear Logic for Resource Protocols
Circuit breakers, rate limiters, and bulkheads as *provable properties*. "No deadlock under any consumer speed" as a type-level guarantee, not a hope.

### D. Proof-Carrying Data Across Service Boundaries
A `safe-transfer` token travels with a payment. A `grounded-summary` token travels with AI output. The proof is the data. Downstream services don't re-validate — they demand the proof type.

### E. Spec-Driven Frontend Frameworks
Shen specs drive UI layout + data grounding. Invalid renders are type errors. Already prototyped in `demo/shen-web-tools/` with Arrow.js.

### F. Formal Verification Export
Translate Shen sequents → Lean/Coq for offline machine-checked proofs on critical components. Shen stays lightweight for iteration; formal provers provide certainty.

### G. Compositional Resilience
The circuit breaker spec shows how resilience primitives compose. `resilience-cleared` requires ALL of: circuit closed + rate allowed + bulkhead permit. This generalizes to any composition of safety properties.

### H. Adaptive Backpressure
Extend rate limiting and bulkhead specs with feedback loops: if downstream is slow, tighten the bulkhead. If errors spike, trip the circuit. All transitions carry proofs — the system can't "accidentally" enter a bad state during adaptation.

### I. Schema Migration as Proof Obligation
The data pipeline spec makes schema evolution explicit. Adding a field = bumping minor version. Breaking change = bumping major version. Downstream stages that haven't adapted won't compile.

### J. Saga Orchestration for Microservices
The workflow-saga spec gives distributed transactions the same safety as local ones. Compensation ordering is proven. Idempotency is enforced. Timeouts are typed.

---

## Pattern Catalog

Every Shen backpressure spec uses combinations of these patterns:

| Pattern | What It Does | Example |
|---------|-------------|---------|
| **Wrapper** | Domain-specific type alias | `user-id`, `service-name` |
| **Constrained** | Validated primitive | `amount (>= 0)`, `rollout-pct [0,100]` |
| **Composite** | Structured type from parts | `transaction [Amount From To]` |
| **Guarded** | Invariant proof | `balance-checked (bal >= amount)` |
| **Proof Chain** | Requires prior proof | `safe-transfer` needs `balance-checked` |
| **Sum Type** | Alternatives | `authenticated-principal = Human \| Service` |
| **Set Membership** | Enum-like | `role ∈ {admin, editor, viewer}` |
| **State Machine** | Transition proofs | `closed → open` needs `threshold-breached` |
| **Composition** | AND of proofs | `resilience-cleared = circuit ∧ rate ∧ bulkhead` |
| **Chain Link** | Sequential integrity | `audit-entry → audit-entry` via hash |
| **Calibration** | Proportional constraint | `confidence ~ source-count` |

---

## What's Next (as of original writing)

1. **Run shengen** on all new specs to generate guard types in Go/TS/Rust/Python
2. **Build working demos** with HTTP handlers that use the generated types
3. **Benchmark** the proof overhead (spoiler: it's mostly zero — checks happen at construction time, not on every use)
4. **Compose specs** — combine RBAC + audit trail, or circuit breaker + saga
5. **Publish the pattern catalog** as a reference for practitioners

The surface area is large. The core insight is small: **make the compiler do your enforcement**.
