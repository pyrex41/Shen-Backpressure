# Captured Output from shen-rust Cedar Examples (Post-2 Artifacts)

This file contains real output from the three integration patterns in `shen-rust/examples/shen-cedar-authz`.

All examples were run with:
```
cargo run -p shen-cedar-authz --example <name> --release
```

---

## 1. generate — Cedar generated FROM a Shen spec (the main "Way 2" story)

**Command:** `cargo run -p shen-cedar-authz --example generate --release`

**Key output:**

```
booting served Shen VM… ready (spec: spec/authz.shen).

=== Cedar generated from spec/authz.shen ===
permit(principal in Role::"Analyst", action == Action::"Eval", resource == ShenCap::"pure");
permit(principal in Role::"Auditor", action == Action::"Eval", resource == ShenCap::"logs");
permit(principal in Role::"Admin", action == Action::"Eval", resource == ShenCap::"pure");
permit(principal in Role::"Admin", action == Action::"Eval", resource == ShenCap::"logs");
permit(principal in Role::"Lead", action == Action::"Eval", resource == ShenCap::"pure");

Generated 5 permits — Cedar-parsed + strict-validated ✓
Artifact written: /var/folders/.../shen-cedar-authz.generated.cedar

Enforcing the generated policy on live requests:
  dana(Admin) · logs   => ALLOW ✓
  dana(Admin) · pure   => ALLOW ✓
  erin(Lead) · pure    => ALLOW ✓
  erin(Lead) · logs    => DENY  ✓

spec/authz.shen → Cedar (validated artifact) → enforced. Closure computed by the Shen engine.
```

This is the core "shengen (or Shen logic) generating Cedar" pattern for post 2.

---

## 2. verify — Shen reasons ABOUT Cedar policies (hierarchy-aware analysis)

**Command:** `cargo run -p shen-cedar-authz --example verify --release`

**Key output:**

```
policies: strict-validated against schema ✓
booting served Shen VM… ready.

policy0 forbid  principal=in Role::"Staff"           resource=any
policy1 permit  principal=in Role::"Analyst"         resource=== ShenCap::"pure"
policy2 forbid  principal=any                        resource=== ShenCap::"io"
policy3 permit  principal=in Role::"Admin"           resource=any
policy4 permit  principal=in Role::"Manager"         resource=== ShenCap::"pure"

Shen-computed interactions (hierarchy-aware: `in` resolved over the role DAG):
  ⚠ policy1 is DEAD — shadowed by forbid policy0  (via Analyst in Staff — string-equality would miss this)
  ⚠ policy3 OVERLAPS forbid policy2 — forbid wins on the intersection

Cross-check (live Cedar): alice(Analyst∈Staff) · pure => DENY ✓ confirms p1 is dead

Shen reasoned over 5 policies, flagged 2 interaction(s).
```

This shows the complementary direction: using Shen's logic engine + DAG reasoning to analyze policies in ways Cedar's per-request evaluator cannot (dead policies due to role hierarchy, overlaps, etc.), then cross-checking against live Cedar.

---

## 3. gate (mentioned for completeness)

Cedar gates Shen evaluation (each request is authorized by Cedar before the served VM runs the Shen source). Not yet captured in detail here.

---

## How these map to post-2

- **generate** → Primary "shengen / Shen logic → Cedar policy" story (Way 2)
- **verify** → Shen as a powerful static analysis / reasoning layer over policies
- **gate** → Cedar as a runtime enforcement front for Shen execution

Combined with the `:runtime-via` compile-time guards from the main Shen-Backpressure examples, these give multiple surfaces (compile-time structural + runtime policy + analysis) from Shen artifacts.

## Real Captured Run (2026-06-02)

Command:
```
cargo run -p shen-cedar-authz --example generate --release
```

Output:

```
booting served Shen VM… ready (spec: spec/authz.shen).

=== Cedar generated from spec/authz.shen ===
permit(principal in Role::"Analyst", action == Action::"Eval", resource == ShenCap::"pure");
permit(principal in Role::"Auditor", action == Action::"Eval", resource == ShenCap::"logs");
permit(principal in Role::"Admin", action == Action::"Eval", resource == ShenCap::"pure");
permit(principal in Role::"Admin", action == Action::"Eval", resource == ShenCap::"logs");
permit(principal in Role::"Lead", action == Action::"Eval", resource == ShenCap::"pure");

Generated 5 permits — Cedar-parsed + strict-validated ✓
Artifact written: /var/folders/.../shen-cedar-authz.generated.cedar

Enforcing the generated policy on live requests:
  dana(Admin) · logs   => ALLOW ✓
  dana(Admin) · pure   => ALLOW ✓
  erin(Lead) · pure    => ALLOW ✓
  erin(Lead) · logs    => DENY  ✓

spec/authz.shen → Cedar (validated artifact) → enforced. Closure computed by the Shen engine.
```

Key observations for the post:
- Shen (in served/VM mode) computed the full transitive role closure.
- Output was parsed + **strict schema validated** by Cedar (build artifact would fail on bad policy).
- Runtime enforcement correctly reflected the inheritance (Lead does not get logs).
- This is the "generate" direction: Shen as the source of truth for policy authoring + analysis.