# DEMO 2 — Sum types fan out (DNF lowering)

## The point

A **sum type** in the spec — one conclusion type that can be proved by several
rule variants — is lowered to a **disjunction** in every policy tier. The
fan-out is structural and automatic: you add one variant to the Shen spec and
Cedar *and* Rego (*and* the decidable fragment) all regrow in lockstep. You
never hand-maintain the disjunction per backend.

In `specs/core.shen`, `authenticated-principal` is a sum type proved two ways:

```
(datatype human-principal
  Auth : authenticated-user;
  ===========================
  Auth : authenticated-principal;)

(datatype service-principal
  Cred : service-credential;
  ============================
  Cred : authenticated-principal;)
```

`policyspec.BuildRuleVariants` gathers every rule that concludes
`authenticated-principal` into a list of variants; `policyspec.CollectClauses`
expands that into **DNF** (a disjunction of conjunctions), distributing AND over
OR via `crossProduct`. Each access target (`tenant-access`, `resource-access`)
therefore lowers to **one Cedar `permit` block per variant** and **one Rego
`<rule> if { ... }` body per variant**. Cedar authorizes if *any* permit
matches; Rego OR-s multiple bodies natively — that is the disjunction.

So with two variants you get, per target, two permit blocks / two Rego bodies:

| Variant            | Lowered condition (the load-bearing premise) |
| ------------------ | -------------------------------------------- |
| `human-principal`  | `sig != ""`, `exp > 0`                       |
| `service-principal`| `secret != ""`                               |

## What the demo does

1. **Before** — counts the disjuncts in each tier:
   - `policies/cedar/policies.cedar`: `4` `permit` blocks (2 tenant + 2 resource).
   - `policies/rego/authz.rego`: `2` `tenant_access if` + `2` `resource_access if` bodies.
2. **Add a third disjunct** — appends a `delegated-principal` variant of
   `authenticated-principal` to `specs/core.shen`, carrying a flat, lowerable
   string premise `(not (= Delegator ""))` (the same shape `service-credential`
   uses for `secret != ""`):

   ```
   (datatype delegated-credential
     Delegator : string;
     Token : string;
     (not (= Delegator "")) : verified;
     ====================================
     [Delegator Token] : delegated-credential;)

   (datatype delegated-principal
     Cred : delegated-credential;
     ============================
     Cred : authenticated-principal;)
   ```
3. **Regenerate** — runs `sb policy --regen`, which re-parses `specs/core.shen`
   **directly** through the `shen-cedar` / `shen-rego` emitters. No Go-guard
   regeneration is involved; the emitters consume the `.shen` source, not the
   generated guards.
4. **After** — re-counts and proves the new variant lowered into both tiers.

## How to run

From the example root (or anywhere — it `cd`s itself):

```bash
bash demo/demo2_sum_type_fanout.sh
# or
demo/demo2_sum_type_fanout.sh
```

Step through it interactively:

```bash
DEMO_PAUSE=1 bash demo/demo2_sum_type_fanout.sh
```

The demo snapshots every file `--regen` rewrites (`specs/core.shen`,
`policies/cedar/policies.{cedar,json}`, `policies/cedar/schema.{json,cedarschema}`,
`policies/rego/authz.rego`, and the decidable sidecars
`specs/decidable-fragment.cert` + `specs/decidable_fragment_eval_stub.go`) and
restores them on EXIT — including ctrl-c / error — so the git tree is left
exactly as found. It is idempotent.

> Note: it invokes `sb` via the prebuilt binary at `cmd/sb/sb` (building a temp
> copy if that is missing). `go run ../../cmd/sb` does **not** work here because
> the example is a separate Go module.

## What to look for

- **Cedar permit blocks: `4 -> 6`** (3 tenant + 3 resource).
- **Rego bodies: `tenant_access` `2 -> 3`, `resource_access` `2 -> 3`.**
- The **new distinct condition** appears in both tiers, proving the third
  variant's premise was lowered (not just a duplicated body):
  - Cedar: `context.delegator != ""` (x2)
  - Rego: `input.delegator != ""` (x2)
- At the end, `git status --short` of the touched files shows **no
  demo-induced changes** — the tree is restored.

## Takeaway

The disjunction is **structural**. One spec variant fans out into every policy
tier simultaneously through the shared DNF lowering in `policyspec`
(`BuildRuleVariants` → `CollectClauses` → per-backend emit). The tiers stay in
lockstep with the spec by construction.
