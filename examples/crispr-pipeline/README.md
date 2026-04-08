# CRISPR Gene Editing Pipeline

Makes invalid edits, dangerous off-target hits, and protocol violations into **type errors**.

The difference between catching a bad edit in silico vs. in a patient.

## Where Computation Actually Plays In

CRISPR is conceptually simple (a guided molecular scissors) but the **computational pipeline is where most design decisions happen** — and where most preventable errors originate.

### The Real Workflow (What A Researcher Does On A Computer)

```
1. LOOK UP TARGET
   Ensembl / UCSC Genome Browser → find the exon to target
   (early constitutive exons for knockouts)

2. FIND CANDIDATE GUIDES
   Paste exon sequence into CRISPOR / Benchling / CHOPCHOP
   Tool finds all 20-mers adjacent to a PAM site (NGG for SpCas9)
   Typically 10-50 candidates per exon

3. SCORE FOR EFFICIENCY
   ML models predict cutting efficiency from sequence features:
   - Rule Set 2 (Doench 2016): gradient-boosted regression, R² ~0.35
   - CRISPRscan: linear regression on positional nucleotide features
   - TIGER (2023): transformer model, state-of-the-art, R² ~0.45
   These scores are imperfect — researchers order 2-3 top guides to hedge

4. SEARCH FOR OFF-TARGETS
   For each guide, search the 3-billion-base genome for every locus
   within 0-4 mismatches:
   - Cas-OFFinder: GPU-accelerated exact enumeration (minutes)
   - FlashFry: pre-indexed database of all PAM-adjacent sites (seconds)
   - CPU-only with 4+ mismatches: can take hours
   This is THE computational bottleneck

5. SCORE OFF-TARGET HITS
   Each hit scored by position-weighted mismatch penalty:
   - CFD score (Doench 2016): accounts for specific nucleotide substitution
   - MIT specificity score (Hsu 2013): position-weighted, aggregate =
     100 / (100 + sum of hit scores)
   CFD generally outperforms MIT

6. SELECT & ORDER
   Pick guides with high efficiency + high specificity + no off-targets
   in coding regions / essential genes. Order as synthetic oligos.

         ——— WET LAB BEGINS ———

7. POST-EDITING ANALYSIS
   After editing, sequence the target locus:
   - CRISPResso2: FASTQ reads + reference amplicon → Needleman-Wunsch
     alignment → quantify indel frequencies → classify (NHEJ, HDR, unmod)
   - GUIDE-seq: genome-wide off-target detection via dsODN integration
   - CIRCLE-seq / DISCOVER-seq: similar empirical off-target mapping
```

### Where Computational Errors Lead To Wet Lab Failures

| Failure Mode | What Went Wrong | Cost |
|-------------|----------------|------|
| Guide doesn't cut | Efficiency score was misleading (R² only ~0.35) | Wasted synthesis + transfection ($200-500) |
| Off-target in essential gene | Search used too few mismatches, or missed bulges | Potentially catastrophic (cell death, oncogenesis) |
| Guide doesn't match target | Cell line has SNPs vs. reference genome (hg38) | Complete experiment failure |
| Post-edit analysis wrong | Insufficient read depth, bad alignment parameters | False confidence in edit quality |
| Off-target false negative | Computational prediction missed chromatin effects | Real off-target cutting discovered too late |

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
                    efficiency-sufficient (ML model score ≥ threshold)
                         │                    ┌─ search-complete (genome-wide, ≥3 mismatches)
                    ot-aggregate-score ◄──────┤
                         │                    └─ CFD/MIT scoring method
                    reference-verified (cell line matches reference genome)
                         │
                    guide-computationally-validated ◄── READY TO ORDER
                         │
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

## What The Computational Proofs Catch

The spec (Layer 4a) adds proof types for every computational quality gate:

| Proof Type | What It Validates | Tool It Maps To |
|-----------|------------------|----------------|
| `efficiency-sufficient` | ML model score ≥ threshold | Rule Set 2, CRISPRscan, TIGER |
| `search-complete` | Genome-wide search with ≥3 mismatches | Cas-OFFinder, FlashFry |
| `ot-aggregate-score` | CFD/MIT specificity score computed | CRISPOR, Benchling |
| `reference-verified` | Cell line target sequenced + matches reference | Sanger sequencing |
| `read-depth-sufficient` | Post-edit reads ≥ minimum depth | CRISPResso2 |
| `guide-computationally-validated` | ALL computational gates passed | Composite proof |

The key insight: `guide-computationally-validated` requires ALL four sub-proofs. You cannot proceed to the wet lab with:
- An unscored guide (no `efficiency-sufficient`)
- An incomplete off-target search (no `search-complete` — and it enforces ≥3 mismatches)
- An unverified reference genome (no `reference-verified` — catches the SNP problem)
- A guide that wasn't scored for off-targets (no `ot-aggregate-score`)

The `edit-spec` type now takes `guide-computationally-validated` instead of raw `guide-rna` — the computational pipeline is a mandatory proof obligation before any wet lab work begins.

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
