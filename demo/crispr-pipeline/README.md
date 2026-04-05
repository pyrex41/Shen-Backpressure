# CRISPR Gene Editing Pipeline

Makes invalid edits, dangerous off-target hits, and protocol violations into **type errors**.

The difference between catching a bad edit in silico vs. in a patient.

## The 8 Layers

| Layer | What It Proves | What It Prevents |
|-------|---------------|-----------------|
| **1. Sequence Primitives** | Valid positions, strand, sequence, guide length (17-24 nt) | Malformed coordinates, wrong-length guides |
| **2. Guide RNA Design** | PAM valid + GC content 40-70% + no secondary structure | Guides that won't bind, self-folding guides, wrong PAM |
| **3. Off-Target Analysis** | Specificity above threshold + no essential gene hits | Dangerous off-target edits, oncogene disruption |
| **4. Edit Specification** | Guide + edit type + off-target safety | Proceeding with unsafe guides |
| **5. Wet Lab Pipeline** | Design → synthesize → transfect → select → sequence → validate | Skipping steps, using failed intermediates |
| **6. Validated Edit** | On-target confirmed + off-targets acceptable | Unverified edits reaching downstream use |
| **7. Biosafety & Ethics** | BSL level + IRB current + somatic-only | Expired IRB, germline edits, wrong containment |
| **8. Multiplexing** | Guide pairs non-interfering | Cross-reactive guides in multiplex experiments |

## Key Proof Chains

### Guide RNA → Validated Edit (the full pipeline)

```
dna-sequence ──┐
genome-position┤
cas-variant ───┤
pam-valid ─────┤
gc-in-range ───┼──► guide-rna (all design criteria met)
no-secondary───┘         │
                    off-target-safe (specificity + essential genes clear)
                         │
                    edit-spec (guide + edit type + safety)
                         │
                    design-approved (human sign-off)
                         │
                    guide-synthesized (purity ≥ 90%)
                         │
                    cells-transfected (delivery method + efficiency)
                         │
                    cells-selected (survival rate)
                         │
                    on-target-confirmed (sequencing validates edit)
                         │
                    off-target-validated (empirical, not just computational)
                         │
                    off-target-acceptable (hits ≤ threshold)
                         │
                    validated-edit ◄── THE PROOF: this edit is real and safe
                         │
                    clinical-grade-edit (+ biosafety + IRB + somatic-only)
```

### What Each Step Requires From The Previous

| Step | Requires Proof Of | Cannot Proceed Without |
|------|------------------|----------------------|
| Synthesize guide | `design-approved` | Approved edit spec with safety analysis |
| Transfect cells | `guide-synthesized` (purity ≥ 90%) | High-purity synthesized guide |
| Select cells | `cells-transfected` | Confirmed delivery |
| Sequence | `cells-selected` | Isolated edited population |
| Validate off-target | `on-target-confirmed` | Confirmed on-target edit |
| Clinical use | `validated-edit` + `irb-current` + `somatic-only` | Complete validation + ethics |

## Why This Matters

### Computational Off-Target Prediction Is Not Enough

The spec separates **Layer 3** (computational off-target analysis, done during design) from **Layer 5 Step 6** (empirical off-target validation, done after editing). Both proofs are required for a `validated-edit`. Computational prediction catches most problems cheaply; empirical validation catches what prediction misses.

### The Pipeline Is A State Machine

You cannot skip steps. Each step's constructor requires the previous step's proof object. There is no way to construct `on-target-confirmed` without first having `cells-selected`, which requires `cells-transfected`, which requires `guide-synthesized`, which requires `design-approved`.

This is the same pattern as the workflow-saga spec, but the stakes are higher: the invariants aren't about money — they're about whether an edit is safe to put in a human being.

### Ethics As Types

`clinical-grade-edit` requires three additional proofs:
- **`irb-current`**: IRB approval exists AND is not expired
- **`somatic-only`**: Target cells are NOT germline (ethical constraint)
- **`biosafety-level`**: Correct containment level

A germline edit is a type error. An expired IRB is a type error. Wrong biosafety level is a type error.

## Edit Types Supported

`knockout | knockin | correction | deletion | insertion | base-edit | prime-edit | epigenetic-mod`

## Delivery Methods

`electroporation | lipofection | viral-vector | rnp | microinjection`

## Cas Variants

`spCas9 | saCas9 | cas12a | cas13`

## Usage

```bash
# Generate guard types
shengen -spec specs/core.shen -out shenguard/guards_gen.go -pkg shenguard

# Use in:
# - CRISPR design software (guide selection with safety proofs)
# - Lab information management systems (LIMS pipeline tracking)
# - Clinical trial data management
# - Regulatory submission documentation
```

See `specs/core.shen` for the full 8-layer specification.
