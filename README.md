# Shen-Backpressure

Formal verification gates for AI coding loops, in the language you're already using.

## The Problem

AI coding loops use tests as the only gate. Tests are empirical: they
check the cases you remembered to write. Specs are deductive: they
hold for every case the type system can construct. An LLM that
produces code which slips past your tests but contradicts an
invariant has produced a regression that won't surface until
production.

Shen-Backpressure adds spec-level gates to the loop. The agent must
pass the structural check (your invariants compile) and the
behavioral check (your pure functions match a spec on sampled
inputs) on top of the regular tests and build. When a gate fails,
the failure feeds back into the next prompt as backpressure.

## What Shen-Backpressure Does

You write a Shen sequent-calculus spec describing your domain's
invariants and pure functions. The spec lives in your repo as
`specs/core.shen` and is the human-edited source of truth.

`shengen` lowers the spec into opaque guard types in your target
language. Today: Go and TypeScript are production-wired through
`sb gen`; Python and Rust exist as reference emitters. The guard
types have unexported fields and validated constructors, so the
language compiler enforces every invariant the spec declares.
`shen-derive` turns `(define …)` spec blocks into table-driven tests
that pin a hand-written implementation against the spec on sampled
inputs.

Production ships your normal target-language binary. Shen runs at
build time, not at runtime — but the two guarantees are different in
kind. **Structural** guarantees (shen-guard) are compile-time: the
target-language compiler rejects any `Amount` that wasn't built
through `NewAmount`. **Behavioral** evidence (shen-derive) is
sampled: a deterministic boundary pool plus optional seeded random
draws asserts pointwise equality between the spec and the impl. The
former is a proof-for-all-inputs; the latter is high-confidence
evidence on a designed sample.

## The Five Gates

Every iteration of the Ralph loop must pass all five gates (plus a
sixth when `shen-derive` is configured):

| Gate | Command | What it catches |
|------|---------|----------------|
| 1. shengen | `sb gen` | Regenerates guard types from spec. Catches stale types. |
| 2. test | `go test ./...` (or `npm test`) | Tests against regenerated types. Catches runtime invariant violations. |
| 3. build | `go build ./...` (or `npx tsc --noEmit`) | Compiles against regenerated types. Catches type signature mismatches. |
| 4. shen tc+ | `bin/shen-check.sh` | Verifies spec internal consistency. Catches contradictory rules. |
| 5. tcb audit | `bin/shenguard-audit.sh` | Re-runs shengen, diffs output, rejects unexpected files in `shenguard/`. |
| 6. shen-derive | `sb derive` | Regenerates committed spec-equivalence tests, fails on drift, then runs them. Active when `[[derive.specs]]` is configured. |

Gate topology is declared in `sb.toml`. The legacy fixed five-gate
shape still works; the new `[[gates]]` array of tables lets you
declare a custom gate list with optional parallel groups.

## Discharge Reports — Audit-Grade Verification Artifacts

Every successful gate run also writes `.sb/discharge_report.json`, a
structured artifact that distinguishes how each premise of each Shen
rule was discharged: statically by the guard types, by runtime
sampling against the Shen oracle, or unproven. Failed premises ship
with concrete counter-examples — case ID, spec output, impl output,
and a ready-to-paste `go test -run …` reproduction command.

The artifact is **dual-purpose**:

- **Agent-loop backpressure.** `sb context` injects a five-second
  Markdown summary into the harness prompt. Failed cases steer the
  next iteration without scraping log output.
- **Audit-grade verification artifact.** `sb audit-report` renders a
  long-form Markdown document a security reviewer or compliance
  auditor can read end-to-end without prior knowledge of
  Shen-Backpressure: spec hash, git commit, tool versions, per-rule
  premise tables, history, and a "How to read this report"
  appendix.

The schema is locked at `schema_version: 1`
([design memo](thoughts/shared/research/2026-05-05-discharge-report-schema.md))
and time-stamped copies accumulate under `.sb/history/` so claims
like "this invariant has been verified at every commit since X" have
evidence on disk, not memory.

What this **does not** mean: not signed, not third-party verified,
not SOC-2 certified. The artifact is the foundation that audit and
compliance workflows can build on — not certification itself. The
schema reserves room for signature fields; v0 always emits
`signature: null`.

## Quick Start

```bash
# In your project (sb binary on $PATH)
sb init        # scaffold specs/core.shen, sb.toml, prompts/, plans/
               # and install /sb:* commands into .claude/
sb loop        # run the Ralph loop
```

`sb init` does both the project scaffold and the Claude Code skill
install in one go; the skill bundle comes from the binary's embedded
copy of `sb/`. Install Option D below describes the same flow if you
already have a project and only want the skills. Run
`sb gates` between iterations if you'd rather drive the loop yourself.
`examples/payment/` is the canonical end-to-end walkthrough.

## The Canonical Demo: Payment Processor

`examples/payment/` is a full Tier-A demo. A payment processor with a
balance-invariant proof chain:
`Amount → Transaction → BalanceChecked → SafeTransfer`. The spec
lives in `specs/core.shen`; shengen emits Go guard types into
`internal/shenguard/guards_gen.go`; `shen-derive` is wired through
`sb.toml` and pins `Processable` against its spec; reference outputs
in TypeScript, Rust, and Python sit alongside the Go output to show
how the same spec compiles in four languages.

The `demo-shen-derive/` subdirectory carries a runnable script and
three deliberately-broken `.go.bak` implementations so you can watch
the gates catch each bug in turn. Read `examples/payment/README.md`
for the walkthrough, including which gates require `bin/shengen-codegen.sh`,
`bin/shenguard-audit.sh`, and a Shen runtime.

## What Else This Does

The same machinery covers two other stories that are not the
headline:

- **Polyglot guard generation.** One Shen spec compiles to Go,
  TypeScript, Python, and Rust. `examples/payment/reference/` shows
  the same five datatypes in all four languages; `examples/shen-web-tools/`
  is a polyglot end-to-end app (Shen/SBCL backend, Arrow.js
  frontend) whose TypeScript proof gate now demonstrates a product
  tag/ref-table resolver that classifies child refs into
  `signed-complete`, `unsigned-complete`, or `partial` outcomes.
- **Proof chains for service authorization.**
  `examples/multi-tenant-api/` ships a JWT → AuthenticatedUser →
  TenantAccess → ResourceAccess proof chain in a live Go HTTP service,
  with a real curl transcript and `go test -v` output captured under
  `demo.md` and `transcript/`.

The verification gates do not assume Ralph specifically. Any
orchestrator that can run a shell command between LLM calls can use
the gate exit codes; any CI pipeline can run `sb gates` as a single
step.

## Engine Architecture

Shen-Backpressure is layered so that each piece answers exactly one
question:

1. **Core engine (`sb`)** — deterministic gate runner. Reads the
   manifest, runs gates, diffs `shen-derive` output, emits structured
   project context. Zero opinions about LLMs, prompts, or loops. One
   static binary, stdlib only.
2. **Project manifest (`sb.toml`)** — declares the project's gate
   topology via `[[gates]]`, `[[derive.specs]]`, and `[engine]`. The
   engine reads this and does nothing more.
3. **Agentic surface (prompts, skills, loops)** — consumes the
   engine's CLI output (`sb context`, gate exit codes, drift reports).
   Agents never reach past the CLI boundary; the manifest is the
   contract.

The rule: `sb` is the canonical source of deterministic knowledge. If
a prompt wants to know what gates exist or why one failed, it asks
`sb` — it does not scrape the filesystem. See
`thoughts/shared/research/2026-05-05-wave-1-manifest-driven-gates.md`,
`-wave-2-sb-context.md`, and `-wave-3-prompt-hydration.md` for the
post-hoc record of how this layering came to be.

For deeper reference material — guard-type pattern catalog, Shen→Go
side-by-side, design-decision Q&A, ASCII pipeline — see
[`docs/REFERENCE.md`](docs/REFERENCE.md). For in-flight work, see
[`thoughts/shared/research/2026-05-05-tag-resolver-finish-line.md`](thoughts/shared/research/2026-05-05-tag-resolver-finish-line.md)
and the matching open questions, and the design prompts under
`thoughts/shared/research/2026-05-05-feature-*.md` for mixed-evidence
reports, differential verification, counterexample traces,
holographic mocks, and compliance audit trails.

## Install

### Option A: Claude Code plugin (recommended)

```
/plugin marketplace add pyrex41/Shen-Backpressure
/plugin install sb@shen-backpressure
```

Run those two slash commands inside Claude Code. This installs the
`/sb:*` commands and the `shen-backpressure` skill globally — no files
copied into your project, and `/plugin` keeps it updated.

### Option B: SKM

```bash
skm sources add https://github.com/pyrex41/Shen-Backpressure
cd your-project
skm sb
```

### Option C: Manual Claude Code install

```bash
mkdir -p .claude/commands/sb .claude/skills/shen-backpressure
cp Shen-Backpressure/sb/commands/*.md .claude/commands/sb/
cp Shen-Backpressure/sb/AGENT_PROMPT.md .claude/commands/sb/
cp Shen-Backpressure/sb/skills/shen-backpressure/SKILL.md .claude/skills/shen-backpressure/
```

### Option D: `sb init`

`sb init` is the same skill-install flow as Option C, plus a project
scaffold (specs/core.shen, sb.toml, prompts/, plans/). It reads the
binary's embedded copy of `sb/`; that copy is a build-time mirror,
and `make check-skilldata` enforces equality with the canonical
`sb/` tree. Use it when you also want the project files; use another
option when you only want the skills.

## Commands

| Command | What it does |
|---------|-------------|
| `/sb:init` | Add Shen backpressure to a project — specs, shengen, guard types, gates. |
| `/sb:loop` | Configure and launch a Ralph loop with gate-driven backpressure. Requires init. |
| `/sb:ralph-scaffold` | All-in-one: init + loop setup. |
| `/sb:create-shengen` | Build a new shengen codegen tool for an additional target language. |
| `/sb:derive` | Wire up or refresh `shen-derive` spec-equivalence tests. |
| `/sb:help` | List available commands. |

## Supported Harnesses

| Harness | Command |
|---------|---------|
| Claude Code | `claude -p` (default) |
| Cursor | `cursor-agent -p` |
| Codex | `codex -p` |
| Rho | `rho-cli run --prompt` |
| Custom | Set `RALPH_HARNESS` env var |

## Project Structure

```
cmd/sb/                  Engine CLI (gen, gates, derive, context, loop, init)
cmd/shengen/             Go codegen (production-wired)
cmd/shengen-ts/          TypeScript codegen (production-wired, while-loop emission)
cmd/shengen-py/          Python codegen (reference)
cmd/shengen-rs/          Rust codegen (reference)
cmd/shen-derive-ts/      Self-hosted TS port of shen-derive (async crypto, aliases, multi-spec)
shen-derive/             Go shen-derive module
sb/                      Canonical SKM bundle (commands, skill, AGENT_PROMPT)
cmd/sb/skilldata/        Build-time mirror of sb/, embedded into the binary
docs/REFERENCE.md        Pattern catalog, side-by-sides, design-decision Q&A
examples/                payment/, multi-tenant-api/, shen-web-tools/, .archive/
thoughts/                Research notes, reviews, handoffs (incl. tag-resolver
                         finish line + feature design prompts under
                         shared/research/2026-05-05-*)
```

## Shen Runtime (Gate 4)

Gate 4 (`shen tc+`) needs a Shen runtime. `bin/shen-check.sh`
auto-detects the supported backends:

| Backend | Startup | Compute | Install |
|---------|---------|---------|---------|
| **shen-sbcl** (default) | 0.06s | 1× | `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl` |
| **shen-scheme** | 0.44s | 1.6× faster | Build from [shen-scheme](https://github.com/Shen-Language/shen-scheme) (`brew install chezscheme`, then `make`, then `cp bin/shen-scheme /usr/local/bin/`) |

For gate loops and CI, `shen-sbcl` wins — startup dominates on small
specs. For large specs with heavy typechecking, `shen-scheme`'s faster
compute may matter. Override with `SHEN=/path/to/binary
bin/shen-check.sh`.

`shengen` itself does not need a Shen runtime; it parses `.shen`
files as text.

## Two Tools, One Spec File

The project ships two complementary tools that share the same
`.shen` spec format:

| | **shen-guard** | **shen-derive** |
|---|---|---|
| Best for | Domain values that cross a boundary (I/O, mutation, glue) | Pure functions where the obvious spec is clear and the efficient impl isn't |
| How it works | Shen spec → shengen → opaque guard types → constructor validation at compile time | `(define …)` block acts as the oracle; generated table-driven test asserts the impl matches on sampled inputs |
| Artifact | Generated guard types committed to the repo | Generated test file committed to the repo, drift checked by a gate |
| Proof method | Shen sequent calculus + target-language compiler — proves the rule for every well-typed value | Spec-vs-impl equivalence on a deterministic boundary pool plus optional seeded random draws; constrained types filter samples against their `verified` predicates — sampled, not for-all |

`shen-derive` plugs into `sb` as Gate 6. Configure it via
`[[derive.specs]]` in `sb.toml`; `sb gates` registers the gate
automatically.

```bash
cd shen-derive && go build -o shen-derive .
./shen-derive verify path/to/spec.shen \
  --func processable \
  --impl-pkg your-module/internal/derived \
  --impl-func Processable \
  --import your-module/internal/shenguard \
  --out your/internal/derived/processable_spec_test.go
```

The generated test is a regular `go test` file — commit it, then run
the `sb derive` gate to detect drift between the spec, the impl, and
the committed test. The TypeScript port at `cmd/shen-derive-ts/` is
self-hosted and supports async crypto derivation, aliases, externs,
and multi-spec verify.

## Further Reading

- **[Don't Waste Your Backpressure](https://banay.me/dont-waste-your-backpressure/)** — The principle behind this project. AI agents that work autonomously need automated feedback on quality and correctness. Without capturing backpressure metrics, you can't delegate longer-horizon tasks with confidence.

- **[Ralph](https://ghuntley.com/ralph/)** — The technique this project implements. Ralph is a bash loop that repeatedly calls an LLM harness (`while :; do cat PROMPT.md | claude-code; done`). Shen-Backpressure adds Shen type checking and codegen guards as backpressure within that loop.

- **[The Loop](https://ghuntley.com/loop/)** — Why loop-based development changes the economics of software. Watch the loop itself; failures become learning opportunities fed back as backpressure, not dead ends.
