---
name: help
description: Show all Shen Backpressure commands, what they do, and when to use each one.
---

# Shen Backpressure — Command Reference

You have four commands and one skill for adding formal verification to AI coding workflows.

## Quick Start

**New project?** Start here:
- `/sb:init` — if you want backpressure without a Ralph loop (CI, manual dev, any workflow)
- `/sb:ralph-scaffold` — if you want the full Ralph loop (autonomous coding with five-gate verification)

**Already set up?** Use these:
- `/sb:loop` — configure and launch a Ralph loop on an existing project
- `/sb:create-shengen` — build shengen for a new language (Rust, Python, Java, etc.)

## Commands

### `/sb:init` — Add Backpressure to Any Project
**When:** You have an existing project and want formal type verification.
**What it does:**
1. Asks about your domain (entities, invariants, operations)
2. Drafts `specs/core.shen` — Shen sequent-calculus type specifications
3. Shows you the specs for confirmation before writing anything
4. Installs shen-sbcl (Shen on SBCL) for type checking
5. Generates guard types (Go or TypeScript) with opaque constructors
6. Sets up shell scripts for all five verification gates
7. Installs Claude Code skills for ongoing LLM guidance

**Output:** `specs/core.shen`, generated guard types, `bin/` scripts, skills.
**Does NOT:** Assume Ralph, CI, or any specific workflow. You decide how to run the gates.

---

### `/sb:loop` — Configure and Launch a Ralph Loop
**When:** You already ran `/sb:init` and want autonomous coding with backpressure.
**Prerequisite:** `/sb:init` must be done first (specs and guard types exist).
**What it does:**
1. Verifies prerequisites (specs, guard types, shen-check works)
2. Asks which LLM harness to use (claude, cursor-agent, codex, rho-cli, custom)
3. Generates the prompt, plan, and Makefile
4. Launches a headless loop: gates → inject errors → call harness → repeat

**The five gates (in order):**
1. `shengen` — regenerate guard types from specs (catches spec drift)
2. `test` — run tests (catches logic errors)
3. `build` — compile against regenerated types (catches type mismatches)
4. `shen-check` — Shen `tc+` on specs (catches spec inconsistency)
5. `tcb-audit` — diff generated code, reject unexpected files (catches tampering)

**Configuration:** `sb.toml [loop]` or env vars: `RALPH_HARNESS`, `RALPH_MAX_ITER`, `RALPH_HARNESS_TIMEOUT`

**CLI shortcut:** `sb loop` runs the loop directly. `sb loop --dry-run` prints the bash script.

---

### `/sb:ralph-scaffold` — Full Setup in One Shot
**When:** Starting from scratch and want Ralph + backpressure together.
**What it does:** Combines `/sb:init` + `/sb:loop` into a single flow.
**Smart detection:** If `/sb:init` was already run, skips to the Ralph loop setup automatically.
**Goes from:** "I have a project" → "five-gate verification is running autonomously"
**Does NOT:** Run the loop or implement domain code. It scaffolds and verifies.

---

### `/sb:create-shengen` — Build Shengen for Any Language
**When:** You need guard types in a language other than Go or TypeScript, or you're extending shengen to handle new Shen patterns.
**What it does:**
1. Provides the complete shengen algorithm: grammar, parser, symbol table, accessor resolution
2. Explains the five datatype patterns (wrapper, constrained, composite, guarded, proof chain)
3. Shows enforcement strategies per language (Go unexported fields, Rust private fields, Python slots, etc.)
4. Guides you through building a working shengen for your target language

**Supported targets:** Go, Rust, Python, TypeScript, Java, C#, Swift, Kotlin, or any language with module-level visibility.

## CLI Tool

The `sb` CLI is a thin launcher — the intelligence lives in these skills. The CLI just runs things:

```
sb init      # Scaffold project (specs, scripts, skills)
sb gen       # Run shengen to generate guard types
sb gates     # Run all five verification gates
sb loop      # Launch Ralph loop (headless LLM + gates)
```

All gate commands are configurable via `sb.toml` or shell scripts in `bin/`.

## Shen Runtime

Gate 4 needs a Shen implementation to run `tc+`. Use **shen-sbcl** (Shen on SBCL/Common Lisp). shengen is a separate text-processing tool that does NOT need a Shen runtime.

Install: `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl`

Do NOT use shen-go — it has known memory allocation crash bugs.

## Concepts

**Shen specs** — Sequent-calculus type definitions in `specs/core.shen`. These are the source of truth. Shen's type checker (`tc+`) proves they're internally consistent.

**Guard types** — Generated code with **module-private fields** (Go: unexported, TS: private, Rust: non-pub). The ONLY way to create a value is through the constructor, which validates the spec's preconditions. The **compiler** enforces this, not the LLM.

**Backpressure** — When generated types change (because specs changed), code that uses them breaks at compile time. This forces the developer (or LLM) to update usage to match the new invariants.

**Ralph loop** — A headless LLM harness in a bash loop: run gates → inject failures → call LLM → repeat until all gates pass.
