---
date: 2026-03-28T21:56:00-07:00
researcher: reuben
git_commit: f00bb15
branch: claude/add-web-tools-integration-eu9L4
repository: Shen-Backpressure
topic: "CLI tool vs skill framework: distribution and delivery strategy for Shen-Backpressure"
tags: [research, architecture, cli, skills, distribution, agentskills, mcp, shengen]
status: complete
last_updated: 2026-03-28
last_updated_by: reuben
---

# Research: CLI Tool vs Skill Framework for Shen-Backpressure

**Date**: 2026-03-28T21:56:00-07:00
**Researcher**: reuben
**Git Commit**: f00bb15
**Branch**: claude/add-web-tools-integration-eu9L4
**Repository**: Shen-Backpressure

## Research Question

Should Shen-Backpressure be delivered primarily as a CLI tool (likely Go) or continue as a Claude Code skill framework? Could a CLI tool provide the same context to a model that skills currently provide? What are the tradeoffs?

## Summary

**The answer is: both, but they serve different functions.** The current architecture already reflects this split — `cmd/shengen` is a Go CLI tool (1885 lines) that does the deterministic work (parsing, codegen, verification), while `sb/` contains skills/commands that teach the LLM *how to use* shengen and *how to think about* guard types. These are fundamentally different concerns, and conflating them would lose the key advantage of each.

The skill framework's value isn't in running commands — it's in **concept injection**. The `create-shengen.md` command alone is 875 lines of algorithm specification that teaches a model how to build a shengen compiler for any language. That knowledge can't be a CLI flag. A CLI tool that "provides this context to a model" would effectively become a prompt generator — which is exactly what skills already are, but with the additional benefit of being lazily loaded, selectively activated, and distributable via SKM and the emerging agentskills.io open standard.

However, there's a legitimate gap: the *orchestration* layer (setting up gates, running the loop, auditing TCB) is currently described in skill markdown but executed by hand or by Ralph. A Go CLI tool that wraps these operations would be valuable — not replacing the skills, but complementing them with deterministic tooling.

## Detailed Findings

### What Exists Today

**CLI tools (deterministic, already in Go):**
- `cmd/shengen/main.go` — 1885-line Go codegen tool. Parses `.shen` specs, builds symbol table, resolves accessor chains, emits guard types. This is the core engine.
- `bin/shengen` — compiled binary (2.7MB)
- `bin/shengen-codegen.sh` — wrapper script for codegen invocation
- `bin/shenguard-audit.sh` — Gate 5 TCB audit script

**Skill/command framework (conceptual, in `sb/`):**
- `sb/AGENT_PROMPT.md` — reference manual for inner LLM harness (guard type discipline, iteration rules, gate failure diagnosis)
- `sb/skills/shen-backpressure/SKILL.md` — auto-activated skill with conceptual overview, pipeline description, spec patterns
- `sb/commands/init.md` — 8-step interactive setup flow (gather requirements → draft specs → confirm → install tooling → generate → verify → report)
- `sb/commands/loop.md` — Ralph loop configuration
- `sb/commands/ralph-scaffold.md` — all-in-one init + loop
- `sb/commands/create-shengen.md` — 875-line algorithm spec for building shengen in any language (grammar, classification, symbol table, s-expression parser, accessor resolution, code generation templates, testing strategy)

**Distribution:**
- `skm.toml` — SKM bundle configuration
- README.md documents both `skm sb` install and manual `.claude/` copy

### The Two Layers: What Can vs Can't Be a CLI Tool

#### Layer 1: Deterministic operations (CLI-appropriate)

These are things a Go CLI tool does well and skills do poorly:

| Operation | Currently | CLI tool would... |
|-----------|-----------|-------------------|
| Parse `.shen` files | `cmd/shengen` | Already done |
| Generate guard types | `bin/shengen` | Already done |
| Run Gate 4 (shen tc+) | `bin/shen-check.sh` | Wrap with better error formatting |
| Run Gate 5 (TCB audit) | `bin/shenguard-audit.sh` | Integrate into single binary |
| Run all 5 gates in sequence | Ralph orchestrator or manual | `sb gates run` |
| Scaffold directory structure | Described in init.md, executed by LLM | `sb init --lang go --pkg shenguard` |
| Validate spec syntax | Implicit in shengen | `sb check specs/core.shen` |

A unified `sb` CLI that wraps shengen, gate-running, scaffolding, and audit into a single binary would be valuable. It would eliminate the shell script layer and provide structured output that either a human or an LLM can consume.

#### Layer 2: Concept teaching (skill-appropriate, NOT CLI-appropriate)

These are things skills do well and a CLI tool fundamentally cannot:

| Concern | Why it needs a skill |
|---------|---------------------|
| Guard type discipline | Teaching "wrap at boundary, trust internally, follow proof chain" — this is a design philosophy, not a command |
| Spec authoring from domain description | "Translate the user's domain into Shen sequent-calculus datatypes" — requires understanding natural language + code simultaneously |
| Create-shengen for new languages | 875 lines of algorithm spec that teaches a model to build a compiler — this IS the knowledge transfer |
| Gate failure diagnosis | "If Gate 3 fails, the generated types changed and Go code uses old signatures" — contextual reasoning |
| Iteration rules | "Pick the FIRST unchecked item, fix backpressure errors first" — behavioral programming for the LLM |
| Anti-pattern detection | "Never construct struct literals, never duplicate validation" — pattern recognition that requires code understanding |

The `create-shengen.md` command is the strongest argument for skills: it's an 875-line algorithm document that teaches any model how to build a shengen compiler from scratch for any target language. No CLI tool can replace that. The skill IS the deliverable.

### What the Landscape Looks Like (Web Research)

#### Agent Skills Open Standard (agentskills.io)

Anthropic has published Agent Skills as an open standard. Key facts:
- **Cross-tool compatibility**: Skills now work with Claude Code, JetBrains Junie, Gemini CLI, Autohand Code CLI, OpenCode, and more
- **Same SKILL.md format**: The format Shen-Backpressure already uses
- **Distribution**: Via SKM (skill-manager, Rust CLI) or manual copy
- Skills are **lazy-loaded** — only metadata enters context until invoked, then full SKILL.md is injected

This means the skill format isn't Claude Code-specific anymore. The `sb/` bundle can work across multiple AI coding tools. A Go CLI that replaces skills would narrow the audience.

- https://agentskills.io/home
- https://the-decoder.com/anthropic-publishes-agent-skills-as-an-open-standard-for-ai-platforms/
- https://serenitiesai.com/articles/agent-skills-guide-2026

#### SKM (Skill Manager)

SKM is a Rust CLI that installs skill bundles to Claude Code, OpenCode, and Cursor:
- `skm sources add https://github.com/pyrex41/Shen-Backpressure`
- `skm sb` — installs the bundle
- Supports fuzzy search, bundle management, multi-tool distribution

- https://lib.rs/crates/skill-manager
- https://github.com/landn172/skill-manager

#### MCP as a Third Option

MCP (Model Context Protocol) is relevant but serves a different niche:
- **MCP excels at**: External integrations, real-time data, multi-model compatibility
- **Skills excel at**: Local workflows, code-adjacent tasks, single-team scope
- **Hybrid pattern**: Skills as UX layer, MCP for shared backend integrations

For Shen-Backpressure, MCP could expose shengen as a tool server that any MCP-compatible model can call. But this adds operational complexity (running a server) without clear benefit over a CLI binary.

#### Comparable Projects: Formal Verification + AI

**Leanstral** (Mistral, 2026): Open-source proof agent for Lean 4. Uses a 6B-parameter model fine-tuned to generate formal proofs. Notably, it's a **model** not a CLI tool — but it uses Lean's CLI (`lean`) as the verification backend. The pattern: model writes proofs, CLI tool checks them. Same pattern as Shen-Backpressure.

- https://mistral.ai/news/leanstral

**dafny-annotator** (Microsoft, 2025): Uses LLM-guided greedy search to add annotations to Dafny programs. The loop: LLM proposes annotations → Dafny CLI validates → accepted if verification progresses. Again: model does the creative work, CLI tool does the deterministic checking.

- https://dafny.org/blog/2025/06/21/dafny-annotator/

**AutoVerus** (2025): Automated proof generation for Rust using Verus. LLM generates proof annotations, Verus verifier checks them.

**Repomix**: CLI tool that packs codebases into AI-friendly formats. Pure context generation — no verification. This is the "CLI provides context to model" pattern, but for generic codebase understanding, not domain-specific verification.

- https://repomix.com/

### The Hybrid Architecture: What Would Actually Work

```
sb (Go CLI)                          sb/ (skill bundle)
├── sb init [--lang go|ts|rs]        ├── SKILL.md (auto-activated concept)
│   scaffold dirs, deps, scripts     ├── commands/
├── sb gen specs/core.shen           │   ├── init.md (interactive setup)
│   runs shengen + formats output    │   ├── create-shengen.md (algorithm)
├── sb gates                         │   ├── loop.md (Ralph config)
│   runs all 5 gates, structured     │   └── ralph-scaffold.md (all-in-one)
│   JSON output for LLM consumption  └── AGENT_PROMPT.md (harness reference)
├── sb audit
│   Gate 5 TCB audit
├── sb check
│   Gate 4 shen tc+
└── sb context
    emit structured context for
    any LLM (prompt fragment)
```

The `sb context` subcommand is the bridge: it outputs a structured prompt fragment that any LLM harness can consume, containing:
- Current spec summary
- Guard type inventory
- Recent gate failures
- Proof chain documentation

This gives you the "CLI provides context to model" capability without replacing the skills.

### Why Skills Can't Be Fully Replaced

The user's intuition ("part of the thing that makes the skill framework attractive is that it's able to be flexible and it knows how to adapt to different kinds of code because it's really a concept") is exactly right. Consider what `create-shengen.md` actually is:

1. A **grammar specification** (§2) that a model reads and implements
2. A **classification algorithm** (§3) described in pseudocode
3. A **symbol table schema** (§4) with construction rules
4. An **s-expression parser spec** (§5) with AST definition
5. An **accessor resolution algorithm** (§6) with chain walking logic
6. **Verified premise translation rules** (§7) with pattern → code mappings
7. **Per-language code generation templates** (§8) for any target
8. A **testing strategy** (§11) and implementation checklist (§12)

This is a 875-line compiler specification delivered as a skill. The model reads it and can then build shengen for Rust, Python, Java, or any language. A CLI tool can't do this — the skill IS the knowledge transfer mechanism.

Similarly, `AGENT_PROMPT.md` teaches the inner harness how to think about guard types, when to wrap/trust/extract, and how to diagnose gate failures. This is behavioral programming for an LLM, not a command to execute.

### Decision Framework

| If your goal is... | Use... |
|---------------------|--------|
| Running gates deterministically | Go CLI (`sb gates`) |
| Scaffolding project structure | Go CLI (`sb init`) |
| Teaching a model guard type discipline | Skill (SKILL.md, AGENT_PROMPT.md) |
| Enabling model to build shengen for new language | Skill (create-shengen.md) |
| Providing gate failure context to any harness | Go CLI (`sb context`) |
| Interactive setup with domain conversation | Skill (init.md command) |
| Distributing to non-Claude-Code tools | Both (agentskills.io for skills, binary for CLI) |
| Auditing TCB boundary | Go CLI (`sb audit`) |

## Architecture Documentation

The current architecture is already a two-layer system:
1. **Deterministic layer**: `cmd/shengen/` Go binary + shell scripts in `bin/`
2. **Conceptual layer**: `sb/` skill bundle with markdown commands

The gap is in the middle: gate orchestration and structured context output live partially in shell scripts and partially in the skill descriptions. A Go CLI binary would close this gap.

## Historical Context (from thoughts/)

- `thoughts/shared/research/2026-03-23-codebase-overview.md` — Documents the original 3-gate architecture and Ralph orchestrator
- `thoughts/shared/research/2026-03-24-shengen-codegen-bridge.md` — Detailed analysis of shengen's parsing and codegen pipeline
- `thoughts/shared/research/2026-03-28-shengen-improvement-research.md` — Recent improvements (Gate 5, sum types, element?, scoped DB wrappers)
- `thoughts/shared/research/2026-03-28-implementation-log.md` — Implementation of improvements from 16-agent review

## Related Research

- [agentskills.io](https://agentskills.io/home) — Open standard for agent skills
- [Leanstral](https://mistral.ai/news/leanstral) — Formal verification + AI model pattern
- [dafny-annotator](https://dafny.org/blog/2025/06/21/dafny-annotator/) — LLM + verifier CLI loop
- [Repomix](https://repomix.com/) — CLI context generation for AI
- [SKM](https://lib.rs/crates/skill-manager) — Skill distribution CLI

## Open Questions

1. **Should `sb` CLI be a single binary or stay as shengen + scripts?** Consolidating into one binary simplifies distribution but increases build complexity.
2. **Should `sb context` output structured JSON or markdown?** JSON is machine-parseable; markdown is readable by both models and humans.
3. **Should the CLI embed the skill content for `sb context`, or reference the skill files?** Embedding makes the CLI self-contained but duplicates knowledge.
4. **Is MCP worth exploring?** A shengen MCP server could expose verification as tools to any compatible model, but adds server management overhead.
5. **Should create-shengen be split into "spec" (CLI-readable algorithm) and "teach" (skill for model)?** Currently it's one 875-line document serving both purposes.
