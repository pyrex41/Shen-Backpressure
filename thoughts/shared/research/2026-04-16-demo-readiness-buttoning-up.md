---
date: 2026-04-16T21:37:11Z
researcher: reuben
git_commit: a6d24d67e648926bb1eb608e77a2dd35d7c2bb19
branch: main
repository: pyrex41/Shen-Backpressure
topic: "Demo readiness — what it takes to button up the scatter into a focused compelling demo"
tags: [research, demo-readiness, examples, cleanup, skill-bundle, sb-engine, shen-derive, blog, site]
status: complete
last_updated: 2026-04-16
last_updated_by: reuben
---

# Research: Demo Readiness — Buttoning Up Scatter Into a Focused Compelling Demo

**Date**: 2026-04-16T21:37:11Z
**Researcher**: reuben
**Git Commit**: a6d24d67e648926bb1eb608e77a2dd35d7c2bb19
**Branch**: main
**Repository**: pyrex41/Shen-Backpressure

## Research Question

Help me figure out what all we've got to do to get this up and running. We have too many examples and too much scatter; I want to get it focused. Clean up markdown files so they live in `thoughts/` in the right places. Get demos built out where they aren't, archive ones that aren't focused enough, and get the project into a shape that is a really compelling demo.

## Summary

The codebase has three layers that are in different states of completion:

1. **Core engine (`cmd/sb/` + `shen-derive/` + shengen variants)** — substantively complete. `sb` v0.3.0 has working `init`, `gen`, `gates`, `derive`, `context`, `loop` subcommands. Wave 1 (manifest-driven gates), Wave 2 (`sb context`), and Wave 3 (prompt hydration) are all wired through end-to-end in Go and TypeScript. Four shengen variants exist (Go, TS, Python, Rust-via-Python); Go + TS are production-wired, Python/Rust are standalone scripts not dispatched by `sb gen`.

2. **Examples (`examples/`, 34 directories)** — scattered across **four tiers of completeness**. Only 2 are full end-to-end demos (`payment/`, `multi-tenant-api/`). 1 polyglot app is complete (`shen-web-tools/`). 13 are spec+guards library stubs (same pattern repeated). 4 are framework scaffolds (no app code). 9 are pure PROMPT-only stubs. Heavy overlap in four clusters: K8s infra (3 near-duplicates), anti-hallucination (3), state machines (4), polyglot-comparison (3+ duplicates of `payment/reference/`).

3. **Public-facing (blog + Hugo site + README)** — mostly drafted but **nothing is deployed**. Seven Hugo posts are `draft: false` with full content; `baseURL` is still `localhost:1313`; no `.github/workflows`, `netlify.toml`, `vercel.json`, or CNAME anywhere. README does not link to the site. The `blog/post-3-impossible-by-construction/` directory mirrors the Hugo post plus sidecar review artifacts; posts 1 and 2 were numbered but never written.

**Key scatter problems the user flagged:**
- **Two diverging skill copies**: `sb/` (canonical, 6 commands, Wave-3 aware) vs. `cmd/sb/skilldata/` (embedded in the binary, 5 commands, pre-Wave). `sb init` installs the older snapshot while SKM/manual installs the current one. Users see two different skills depending on install path.
- **Scattered root markdown**: `EXPLORATION.md` (224 lines of example directions), `heavy_analysis.md` (71 lines), `heavy_analysis_2.md` (48 lines), `temp_prompt.md` (1 line), plus `research/shen-bend-feasibility-prompt.md` — all of which are synthesis/research material that belongs in `thoughts/shared/research/`.
- **Blog working-directory sidecars**: `blog/post-3-impossible-by-construction/scud-review.md` + `scud-review-clean.md` are review artifacts duplicated against Hugo content.
- **Wave 1/2/3 engine work is undocumented in `thoughts/`** — the three most recent waves have no research memo in `thoughts/shared/research/`.
- **The main repo does not dogfood its own `sb.toml`** — only two examples have manifests, both use the legacy (non-`[[gates]]`) format.

## Detailed Findings

### 1. The `sb` Engine and Skill Bundle

#### Engine source (`cmd/sb/`, Go 1.24.7, `github.com/BurntSushi/toml` only)

Seven files, 2,355 LOC. All six subcommands are fully implemented:

- `cmd/sb/main.go:23` — `const version = "0.3.0"`; dispatch at lines 31–52
- `cmd/sb/config.go:166` — two-pass TOML parser (new `[[gates]]` array first, falls back to legacy `[gates]` table + `[commands]`)
- `cmd/sb/gates.go:107` — `buildGateList` synthesises the gate list from either format; `runGateList` at line 198 runs contiguous same-group gates in parallel via WaitGroup; `derive` gate auto-appended at line 147 when `[[derive.specs]]` is non-empty
- `cmd/sb/context.go:272` — `cmdContext` command; `ProjectContext` struct at line 15; `RenderMarkdown` at line 183; embedded Shen spec parser at line 332 classifies types into wrapper/constrained/composite/guarded/alias/sumtype and computes a linear proof chain
- `cmd/sb/derive.go:28` — two-pass gate (drift diff, then `go test` / `node --test`); branches on `spec.Lang` for Go vs TS dispatch
- `cmd/sb/loop.go:58` — `runLoop` calls `BuildContext(cfg)` every iteration (line 81); `buildHarnessPrompt` at line 120 composes `prompt + "## Live Project Context" + ctx.RenderMarkdown() + plan + backpressure errors`
- `cmd/sb/gen.go:11` — dispatches `runShengenGo` or `runShengenTS` based on `cfg.Lang`

**Wave status in the engine:** Wave 1 (manifest gates), Wave 2 (`sb context`), Wave 3 (prompt hydration) are all wired end-to-end — `HasManifestGates()` is the toggle; all three callers (gates, loop, context) flow through the same `buildGateList`/`buildGateInfos` code path.

#### Skill bundle — the canonical/embedded divergence

There are **two parallel copies** of the skill files that have drifted across the last few commits:

| File | `sb/` (canonical, delivered by SKM/manual) | `cmd/sb/skilldata/` (embedded, delivered by `sb init`) |
|---|---|---|
| `commands/init.md` | 276 lines, includes derive config step, shen-sbcl vs shen-scheme table | 175 lines, pre-derive |
| `commands/loop.md` | 126 lines, has "Live Context Injection" section, Gate 6 | 77 lines, five gates only |
| `commands/derive.md` | **present** (122 lines) | **ABSENT** |
| `commands/ralph-scaffold.md` | Gate 6 awareness | older |
| `commands/create-shengen.md` | 1,073 lines, includes hardened-mode for Go/Rust/TS/Python | older |
| `commands/help.md` | 128 lines, lists all 6 commands | lists 5 |
| `skills/shen-backpressure/SKILL.md` | 176 lines, Gate 6, shen-scheme, `multi_tool_use.parallel` | 113 lines, no Gate 6 |
| `AGENT_PROMPT.md` | 105 lines, "Live Project Context" section, Gate 6 in table | 221 lines, static "Backpressure Errors" section, five gates |

The canonical side has the current Wave-3 content; the embedded side is the pre-Wave snapshot. `skm.toml` points SKM at `sb/` (canonical); `cmd/sb/init.go:118` `installSkills` walks the embedded `cmd/sb/skilldata/` (older). Users see different skills depending on install path.

#### Main repo does not dogfood its own manifest

The project root has **no `sb.toml`**. Only two examples have manifests:
- `examples/payment/sb.toml` (28 lines) — legacy format (no `[[gates]]`), uses `[derive]` + `[[derive.specs]]` for Go
- `examples/shen-web-tools/sb.toml` (23 lines) — legacy format, `lang = "ts"` + one TS derive spec

Neither exercises the new `[[gates]]` array format. The new format is implemented in the engine and documented in the README + `sb.toml.tmpl`, but no example uses it.

#### Shengen variants

| Binary | File | Lines | Status |
|---|---|---|---|
| Go | `cmd/shengen/main.go` | 1,713 | Production — wired by `FindShengen`, used by `examples/payment`, has `main_test.go` |
| TS | `cmd/shengen-ts/shengen.ts` | 883 | Production — wired by `FindShengenTS`, used by `examples/shen-web-tools` |
| Python | `cmd/shengen-py/shengen.py` | 651 | Standalone script, **not dispatched by `sb gen`**, no tests, no example project |
| Rust (as Python) | `cmd/shengen-rs/shengen.py` | 565 | Standalone script, **not dispatched by `sb gen`**, no tests, no example project |

Only Go and TS are production-wired through `sb gen`. Python and Rust exist as reference emitters implementing the `/sb:create-shengen` spec but nothing in the engine invokes them.

### 2. `shen-derive` — Status

`shen-derive/` is a separate Go module (`github.com/pyrex41/Shen-Backpressure/shen-derive`) that is fully functional at v0.3.0:

- `shen-derive/main.go:38` — four subcommands: `repl`, `eval`, `parse`, `verify`
- `shen-derive/core/` — S-expr parser, evaluator (`+`, `-`, `*`, `/`, `%`, comparisons, `and`/`or`/`not`, `cons`, `concat`, `fst`/`snd`, `map`, `foldr`, `foldl`, `scanl`, `filter`, `unfoldr`, `compose`), pattern matcher; substantive unit tests
- `shen-derive/specfile/` — parses `(datatype ...)` and `(define ...)` blocks, handles `where` guards, classifies into six categories
- `shen-derive/verify/` — sample generation (deterministic boundary pool `{0, 1, -1, 5, 2.5, 100}` + string pool `{"", "alice", "bob"}`, constraint-filtered for `verified` predicates, optional seeded random draws), cartesian-product harness, Go test emitter. 460-line test suite (`harness_test.go`) covers payment-domain integration, constraint filtering, domain-typed returns, multi-clause `where`-guarded recursion, and seeded reproducibility
- `shen-derive/archive/` — v1 rewrite-engine code as `.go.bak` files; inert, not compiled
- `shen-derive/DESIGN.md` — records the v1→v2 pivot

**TS counterpart (`cmd/shen-derive-ts/`)** is at feature parity for `verify` — same core/specfile/verify split, same sample pools, same `envHolder` trick for mutual recursion, same cartesian product. Emits `node:test` individual `test()` calls instead of Go's table-driven `t.Run` loop. Supports composite/guarded return types (Go version rejects these with an error at `harness.go:440`). Every module has a `.test.ts`. Does not implement `repl`/`eval`/`parse`.

**Examples using shen-derive (only 2):**
- `examples/payment/internal/derived/processable_spec_test.go` — 35 cases, committed
- `examples/shen-web-tools/runtime/sum_nonneg.shen-derive.test.ts` — 8 cases, committed

No other example has `[[derive.specs]]` or a committed `_spec_test.{go,ts}` file.

### 3. Examples Inventory — 30+ directories across 4 tiers

**Tier A — Full end-to-end demos (runnable Go app + tests + demo narrative): 2**

| Example | Highlights |
|---|---|
| `examples/payment/` | `specs/core.shen`, generated `internal/shenguard/guards_gen.go`, reference outputs in Go/TS/Rust/Python (both standard and hardened), full `demo-shen-derive/DEMO.md` + `run.sh` + three `.go.bak` bug files, `sb.toml` with derive gate wired to `processable_spec_test.go`. **The flagship demo.** |
| `examples/multi-tenant-api/` | Full Go HTTP service with JWT → AuthenticatedUser → TenantAccess → ResourceAccess proof chain; `demo.md` is a live curl transcript with real JWT tokens and real `go test -v` output; `transcript/` has 5 subagent JSONL sessions showing Ralph generating it. No `sb.toml`. |

**Tier B — Full polyglot app: 1**

| Example | Highlights |
|---|---|
| `examples/shen-web-tools/` | Shen/SBCL backend + Arrow.js frontend; three specs (`core.shen`, `medicare.shen`, `shen-derive-smoke.shen`); `sb.toml` with TS derive gate; CL server + bridge + medicare module. Only polyglot end-to-end demo in the tree. |

**Tier C — Spec + guards library stubs (13, all using the same pattern): `specs/core.shen` + `shenguard/guards_gen.go` committed, README.md ≡ PROMPT.md (byte-identical), no `go.mod`, no `Makefile`, no app code**

- `workflow-saga/`, `shenplane/`, `shenguard-bolt-on/`, `k8s-infra/`, `rbac-capabilities/`, `feature-flags/`, `defi-invariants/`, `data-pipeline/`, `crispr-pipeline/`, `consensus-quorum/`, `circuit-breaker/`, `audit-trail/`, `ai-grounding/`

**Tier D — Framework scaffolds (4, `specs/core.shen` + `PROMPT.md` + detailed sub-prompt in `prompts/`, no guards_gen, no app):**

- `shen-hono/`, `shen-fastapi/`, `shen-go-api/`, `shen-rust-axum/` — plus `shen-go-advanced/` as a variant

**Tier E — Pure PROMPT-only stubs (9, some with `specs/.gitkeep` only):**

- `order-state-machine/`, `llm-hallucination-guard/`, `category-showcase/`, `sum-type-showcase/`, `polyglot-comparison/`, `relational-constraints/`, `pipeline-state-machine/`, `shen-prolog-ui/`, plus `dosage-calculator/` (scaffolded with Makefile + `cmd/server/main.go` but `internal/shenguard/` never generated and no tests). `email-crud/` has full Go app + tests but the guard types sit in `reference/` and the build does not regenerate into `internal/shenguard/` — schengen is not in the hot path.

**Overlap clusters (high-level):**

| Cluster | Members | Nature |
|---|---|---|
| **K8s infra** | `k8s-infra/`, `shenguard-bolt-on/`, `shenplane/` | `k8s-infra` and `shenguard-bolt-on` have **near-identical** README text and both ship the same four scanner Go files; `shenplane` is the clean-sheet counterpart. `INFRA_COMPARISON.md` serves as the shared intro. |
| **Anti-hallucination / AI output** | `ai-grounding/`, `llm-hallucination-guard/`, `shen-web-tools/` | All enforce "AI output must be grounded"; `ai-grounding` explicitly says it extends the `shen-web-tools` pattern; `llm-hallucination-guard` is a simpler closed-enum variant. |
| **State machine / flow control** | `order-state-machine/`, `workflow-saga/`, `circuit-breaker/`, `pipeline-state-machine/` | Same "invalid transition = type error" thesis at different scales. |
| **Polyglot / language comparison** | `polyglot-comparison/`, `sum-type-showcase/`, `payment/reference/` | `payment/reference/` already contains guards_gen in 5 languages; `polyglot-comparison/` would duplicate exactly that. |
| **shengen category teaching** | `category-showcase/`, `sum-type-showcase/` | Both "teach the shengen categories" at different granularities. |
| **Go API + payment domain** | `payment/`, `shen-go-api/`, `order-state-machine/` | All are Go HTTP services with overlapping domain framing. |
| **Authorization proof chain** | `multi-tenant-api/`, `rbac-capabilities/` | Same multi-step authorization chain; `multi-tenant-api` is built, `rbac-capabilities` is spec+guards only. |

#### Example directory docs (top-level)

- `examples/FRAMEWORK_EXAMPLES.md` (28 lines) — meta-doc pointing at the 4 framework scaffolds
- `examples/INFRA_COMPARISON.md` (165 lines) — comparison table + architecture diagram contrasting `shenguard-bolt-on` vs `shenplane`

### 4. Markdown Scatter Map

#### Root-level loose docs (candidates for `thoughts/shared/research/`)

| File | Lines | Content |
|---|---|---|
| `/EXPLORATION.md` | 224 | "All The Ways" — maps every shengen direction with per-demo spec links and impossible-to-violate invariants |
| `/heavy_analysis.md` | 71 | Synthesis: Shen as verifiable interlingua, five-gate/Ralph loops, sequent-calculus datatypes |
| `/heavy_analysis_2.md` | 48 | Follow-on synthesis, reviews examples/ scaffolding and directions |
| `/temp_prompt.md` | 1 | Raw working prompt blob |
| `/research/shen-bend-feasibility-prompt.md` | — | Feasibility brief: Shen × Bend interaction nets across 3 codebases (4 investigations, cites Girard/Lafont/Tarau) |

#### Blog working directory (`blog/post-3-impossible-by-construction/`)

- `post.md` (287 lines) — complete prose; only placeholder is `Next:` link pointing to `#`
- `demo.md` — Showboat-format verifiable demo, dated 2026-03-28
- `scud-review.md` — raw multi-agent review output with ANSI escape codes
- `scud-review-clean.md` — review stripped of escape codes
- 4 PNG screenshots

`post.md` is substantially **mirrored** at `site/content/posts/impossible-by-construction/index.md` with Hugo frontmatter prepended. The review sidecars are review artifacts with no counterpart in Hugo.

#### Hugo site content (`site/content/posts/`) — all `draft: false`

| Slug | Date |
|---|---|
| `impossible-by-construction/index.md` | 2026-03-28 |
| `one-spec-every-language/index.md` | 2026-03-29 |
| `bypass-taxonomy/index.md` | 2026-03-29 |
| `go-hardening/index.md` | 2026-03-29 |
| `rust-linear-proofs/index.md` | 2026-03-29 |
| `typescript-branded-nominals/index.md` | 2026-03-29 |
| `python-closure-vaults/index.md` | 2026-03-29 |
| `how-shen-derive-works/index.md` | 2026-04-10 |
| `shen-derive-walkthrough/index.md` | 2026-04-10 |

(Note: the codebase-locator found 9 posts; the blog-audit sub-agent counted 7 — the two `shen-derive` posts dated 2026-04-10 are real and present.)

Hugo `baseURL = 'http://localhost:1313/'`. No deployment pipeline exists: no `.github/workflows/`, no `netlify.toml` at project level (the one in `site/themes/ananke/` is the theme's own), no `vercel.json`, no `CNAME`, no committed `public/`. README does not link to site.

#### Existing `thoughts/` inventory (17 documents)

**Research (10):**
- `2026-03-23-codebase-overview.md`
- `2026-03-24-shengen-codegen-bridge.md`
- `2026-03-28-blog-series-deterministic-backpressure.md`
- `2026-03-28-cli-vs-skill-framework.md`
- `2026-03-28-implementation-log.md`
- `2026-03-28-shengen-improvement-research.md`
- `2026-03-29-closures-opaque-types-shen-idea-space.md`
- `2026-03-29-cross-language-enforcement-spectrum.md`
- `2026-03-31-full-codebase-exploration.md`
- `2026-04-09-shen-derive-vision-gap-analysis.md`

**Reviews (3):**
- `2026-04-09_pr10-heavy-review-meta.md`
- `2026-04-09_pr10-shen-derive-heavy-review.md`
- `2026-04-09_shen-derive-v1-docs-synthesis.md`

**Handoffs (3):**
- `2026-04-09_18-13-49_shen-derive-v2-sexpr-pivot.md`
- `2026-04-10_12-39-23_shen-derive-verification-next-steps.md`
- `2026-04-10_17-04-28_shen-derive-ts-port-prompt.md`

**Gap:** Nothing in `thoughts/shared/research/` documents Waves 1/2/3 (the three most recent commits on main: manifest gates, `sb context`, prompt hydration) or the examples-overlap/demo-readiness work.

### 5. Public-Facing Asset Readiness

- **Blog/Hugo content**: substantively written; 7–9 posts, all `draft: false`. Nothing blocking publish besides infrastructure.
- **Deployment**: zero. `baseURL` is `localhost:1313`. No CI pipeline. No hosting wired up. README has no site link.
- **Posts 1 and 2**: the `post-3-` numbering implies a series; posts 1 and 2 do not exist anywhere in repo. The numbering currently dangles.
- **Site/README linkage**: README does not mention the site or published URL.

## Code References

### Engine (`cmd/sb/`)
- `cmd/sb/main.go:23` — `version = "0.3.0"`
- `cmd/sb/main.go:31` — subcommand dispatch
- `cmd/sb/config.go:22-72` — `Config` + `GateDef` + `DeriveSpec` types
- `cmd/sb/config.go:60` — `HasManifestGates()` — the Wave-1 toggle
- `cmd/sb/config.go:166-284` — two-pass parser
- `cmd/sb/gates.go:107` — `buildGateList`
- `cmd/sb/gates.go:147-158` — auto-appends `shen-derive` gate
- `cmd/sb/gates.go:198` — `runGateList` with parallel-group handling
- `cmd/sb/context.go:78` — `BuildContext`
- `cmd/sb/context.go:183` — `RenderMarkdown`
- `cmd/sb/context.go:243` — `proofChain`
- `cmd/sb/context.go:332` — `parseSpecTypes`
- `cmd/sb/derive.go:28` — `cmdDerive`
- `cmd/sb/derive.go:96` — drift-diff pass
- `cmd/sb/derive.go:113` — `spec.Lang` Go/TS branch
- `cmd/sb/derive.go:211` — `go test` / `node --test` pass
- `cmd/sb/loop.go:58` — `runLoop`
- `cmd/sb/loop.go:81` — per-iteration `BuildContext(cfg)`
- `cmd/sb/loop.go:120-135` — `buildHarnessPrompt` composes context + plan + backpressure
- `cmd/sb/gen.go:11` — dispatch
- `cmd/sb/init.go:118` — `installSkills` walks embedded `skilldata/` (older snapshot)

### Skill bundles
- `sb/AGENT_PROMPT.md` (canonical, 105 lines, Wave-3)
- `sb/commands/*.md` (6 files, all Wave-3)
- `sb/skills/shen-backpressure/SKILL.md` (176 lines)
- `cmd/sb/skilldata/AGENT_PROMPT.md` (embedded, 221 lines, pre-Wave)
- `cmd/sb/skilldata/commands/*.md` (5 files — no `derive.md`)
- `cmd/sb/skilldata/skills/shen-backpressure/SKILL.md` (113 lines, pre-Wave)

### shen-derive
- `shen-derive/main.go:30` — v0.3.0
- `shen-derive/main.go:38-54` — four subcommands
- `shen-derive/verify/harness.go:292-329` — `envHolder` trick for mutual recursion
- `shen-derive/verify/harness.go:392-503` — `Emit()`
- `shen-derive/verify/harness.go:440-443` — composite/guarded return rejection
- `shen-derive/DESIGN.md` — v1→v2 pivot record
- `cmd/shen-derive-ts/` — full TS port

### Examples that work end-to-end
- `examples/payment/sb.toml` — derive gate wired
- `examples/payment/internal/derived/processable_spec_test.go` — 35 committed cases
- `examples/payment/demo-shen-derive/DEMO.md` + `run.sh` + `bug{1,2,3}_*.go.bak`
- `examples/payment/reference/guards_gen.{go,ts,rs,py}` — polyglot outputs already here
- `examples/multi-tenant-api/demo.md` — live curl transcript
- `examples/multi-tenant-api/transcript/` — 5 subagent JSONL sessions
- `examples/shen-web-tools/sb.toml` — TS derive gate
- `examples/shen-web-tools/runtime/sum_nonneg.shen-derive.test.ts`

### Scatter to consolidate
- `/EXPLORATION.md` (224 lines)
- `/heavy_analysis.md`, `/heavy_analysis_2.md`, `/temp_prompt.md`
- `/research/shen-bend-feasibility-prompt.md`
- `/blog/post-3-impossible-by-construction/scud-review.md`, `scud-review-clean.md`

## Architecture Documentation

**Install-path divergence.** Three advertised install paths resolve to two different skill bundles:

| Path | Reads | Result |
|---|---|---|
| `skm sb` | `skm.toml` → `path = "sb"` | Canonical `sb/` bundle (6 commands, Wave-3) |
| Manual `cp sb/commands/*.md ...` | `sb/` | Canonical bundle |
| `sb init` | `cmd/sb/skilldata/` (embedded) | Pre-Wave bundle (5 commands, no derive) |

The embedded `skilldata/` was frozen at an earlier commit and has not been re-synced after Waves 1/2/3.

**Gate topology.** The engine supports two manifest formats simultaneously. The new format (`[[gates]]` array with optional `group` for parallel execution) is implemented and documented in the README; the `sb.toml.tmpl` emits both formats; the two existing example manifests use the legacy format exclusively. The `shen-derive` gate is auto-appended regardless of format whenever `[[derive.specs]]` exists.

**Engine/agentic boundary.** The engine explicitly knows nothing about LLMs or prompts. Everything an agent needs flows through `sb context --format markdown`, which hydrates into the `## Live Project Context` section of the harness prompt per iteration. The README calls this "the manifest is the contract" (README line 43).

## Historical Context (from thoughts/)

- `thoughts/shared/research/2026-03-23-codebase-overview.md` — first-principles orientation
- `thoughts/shared/research/2026-03-24-shengen-codegen-bridge.md` — shengen bridge mechanics
- `thoughts/shared/research/2026-03-28-cli-vs-skill-framework.md` — CLI-vs-skill decision
- `thoughts/shared/research/2026-03-28-implementation-log.md` — running log from scud-reviewed session
- `thoughts/shared/research/2026-03-28-shengen-improvement-research.md` — phantom types, TCB reduction
- `thoughts/shared/research/2026-03-29-closures-opaque-types-shen-idea-space.md`
- `thoughts/shared/research/2026-03-29-cross-language-enforcement-spectrum.md`
- `thoughts/shared/research/2026-03-31-full-codebase-exploration.md` — last broad snapshot before Waves 1/2/3
- `thoughts/shared/research/2026-04-09-shen-derive-vision-gap-analysis.md`
- `thoughts/shared/reviews/2026-04-09_pr10-shen-derive-heavy-review.md` + `_pr10-heavy-review-meta.md` — PR #10 shen-derive review
- `thoughts/shared/reviews/2026-04-09_shen-derive-v1-docs-synthesis.md` — review of V1 docs
- `thoughts/shared/handoffs/general/2026-04-09_18-13-49_shen-derive-v2-sexpr-pivot.md` — v2 pivot
- `thoughts/shared/handoffs/general/2026-04-10_12-39-23_shen-derive-verification-next-steps.md` — next steps
- `thoughts/shared/handoffs/general/2026-04-10_17-04-28_shen-derive-ts-port-prompt.md` — TS port prompt

## Open Questions (aka "Path to Button Up")

The user explicitly asked for guidance on what to do, so this section is recommendation-shaped. Each item is a lever the user can pull; none are prescriptive.

### A. Cleanup (low-cost, unblocks everything else)

1. **Move root-level scatter into `thoughts/shared/research/`**:
   - `EXPLORATION.md` → `thoughts/shared/research/2026-04-16-exploration-all-the-ways.md` (frontmatter: research, tags: examples, shengen-directions)
   - `heavy_analysis.md` + `heavy_analysis_2.md` → merge or keep as two dated research docs
   - `temp_prompt.md` → delete or move to handoffs if still useful
   - `research/shen-bend-feasibility-prompt.md` → `thoughts/shared/research/2026-XX-XX-shen-bend-feasibility.md`, then delete `/research/`
2. **Blog sidecars** → `thoughts/shared/reviews/2026-03-28-impossible-by-construction-scud-review.md`. Keep only `post.md` + images in `blog/` (or delete `blog/` entirely now that Hugo has the authoritative copy).
3. **Resolve the skill bundle divergence**: either (a) make `sb init` install from `sb/` at build time (`go:embed` against `../../sb/`), or (b) run a pre-commit/CI script that rsyncs `sb/` → `cmd/sb/skilldata/`. Right now, updating `sb/` silently leaves `sb init` users on the old bundle.
4. **Add root-level `sb.toml`** that dogfoods the new `[[gates]]` format on the engine itself (gates: `go build ./cmd/...`, `go test ./...`, optionally `sb check` on a meta-spec). Proves the format works and gives users a reference manifest.

### B. Examples — focus the tree

Cut/archive/consolidate (specific proposals, user decides):

- **Pick one k8s example; archive the others.** `shenguard-bolt-on/` and `k8s-infra/` are near-duplicates. `shenplane/` is the clean-sheet story. Either keep `shenguard-bolt-on/` (with `INFRA_COMPARISON.md` as the intro) and archive the other two, or keep `shenplane/` as the ambitious vision and archive the bolt-on pair. Move archived dirs to `examples/.archive/` (git-tracked but clearly shelved).
- **Pick one anti-hallucination example.** `shen-web-tools/` is built. `ai-grounding/` and `llm-hallucination-guard/` are stubs covering the same thesis. Archive the two stubs.
- **Pick one state-machine example.** The four stubs (`order-state-machine`, `workflow-saga`, `circuit-breaker`, `pipeline-state-machine`) teach the same lesson. Build one out end-to-end (probably `order-state-machine` for relatable domain); archive the rest.
- **Drop `polyglot-comparison/`.** `examples/payment/reference/` already has all 5 languages. Write a README inside `payment/reference/` explaining the comparison and point to it from the top-level README.
- **Drop `sum-type-showcase/` and `category-showcase/`** (PROMPT-only stubs that duplicate learning content the README's "Guard Type Patterns" table already covers).
- **Decision point for framework scaffolds** (`shen-hono/`, `shen-fastapi/`, `shen-go-api/`, `shen-rust-axum/`): either commit to building them all out (big lift) or archive them behind one "polyglot framework starter" README that shows the one that's built.
- **`dosage-calculator/`**: scaffolded but never generated. Either run Ralph on it and make it a third Tier-A demo or demote to Tier-E/archive.
- **`email-crud/`**: full Go app but guards live in `reference/` (not wired to build). Decide whether to wire it through shengen → `internal/shenguard/` (making it a third Tier-A demo) or archive as "generated by Ralph but not part of the verification story."

**Focused example set (one proposal):**
- `payment/` — flagship (shengen + shen-derive + demo script + polyglot reference)
- `multi-tenant-api/` — proof-chain flagship
- `shen-web-tools/` — polyglot end-to-end flagship
- `order-state-machine/` — state-machine flagship (needs build-out)
- `shenplane/` OR `shenguard-bolt-on/` — infra flagship (pick one)
- `category-showcase/` as spec-library index (spec + guards only, serves as "Rosetta stone" referenced from README)

That's 6 examples instead of 34.

### C. Missing Tier-A demos to build (if aiming for wider coverage)

If the demo needs to show breadth, prioritize building one example per cluster to Tier-A (runnable app + tests + demo narrative):
1. Order state machine (Go, invalid transition = compile error) — strongest candidate
2. Feature flags (good domain, small spec, easy to demo)
3. One DeFi invariant (ambitious but headline-grabbing)

### D. Publish the site

1. Change `site/hugo.toml` `baseURL` to the intended domain
2. Add deployment (GitHub Pages via Actions is lightest: `.github/workflows/pages.yml`, build Hugo, publish `public/`)
3. Add a site link to the README ("Read the series" → `https://.../posts/impossible-by-construction/`)
4. Write posts 1 and 2 OR renumber the existing post series so there's no dangling `post-3-` implying a missing series
5. Fix the `Next:` `#` placeholder in `post.md`/`impossible-by-construction/index.md`

### E. Document the three Waves

Nothing in `thoughts/` covers the Wave 1/2/3 engine work. Worth a single research memo (`thoughts/shared/research/2026-04-16-sb-waves-1-3-architecture.md`) before this research rolls off fast-context.

### F. Order-of-operations suggestion

If you want minimum time to "buttoned up":

1. **First hour**: move the 5 root scatter files into `thoughts/`, delete `/research/`, resolve the skill-bundle divergence (pick one side to be canonical, delete or regenerate the other)
2. **Second hour**: trim examples to the 6-ish focused set; move the rest to `examples/.archive/`
3. **Third hour**: fill in one additional Tier-A demo (pick `order-state-machine` or `dosage-calculator`)
4. **Fourth hour**: wire Hugo deployment, set `baseURL`, add site link to README

A–D below are ordered by user leverage, not by logical dependency — you could do them in parallel with different agents if you want.

## Related Research

- `thoughts/shared/research/2026-03-31-full-codebase-exploration.md` — last comprehensive snapshot pre-Waves
- `thoughts/shared/research/2026-03-28-cli-vs-skill-framework.md` — grounds the engine/skill split
- `thoughts/shared/research/2026-03-28-blog-series-deterministic-backpressure.md` — blog-series context
- `thoughts/shared/reviews/2026-04-09_pr10-heavy-review-meta.md` — PR #10 review (shen-derive landing)
