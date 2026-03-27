---
name: help
description: Show all Shen Backpressure commands, what they do, and when to use each one.
---

# Shen Backpressure — Command Reference

You have four commands and one skill for adding formal verification to AI coding workflows.

## Quick Start

**New project?** Start here:
- `/sb:init` — if you want backpressure without a Ralph loop (CI, manual dev, any workflow)
- `/sb:ralph-scaffold` — if you want the full Ralph loop (autonomous coding with four-gate verification)

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
6. Verifies all gates pass

**Output:** `specs/core.shen`, generated guard types, verification gates you wire into any workflow.
**Does NOT:** Assume Ralph, CI, or any specific workflow. You decide how to run the gates.

---

### `/sb:loop` — Configure a Ralph Loop
**When:** You already ran `/sb:init` and want autonomous coding with backpressure.
**Prerequisite:** `/sb:init` must be done first (specs and guard types exist).
**What it does:**
1. Verifies prerequisites (specs, guard types, shen-check works)
2. Asks which LLM harness to use (claude, cursor-agent, codex, rho-cli, custom)
3. Generates the orchestrator (`cmd/ralph/main.go`), prompt, plan, and Makefile
4. Verifies all four gates pass clean
5. Ready to `make run`

**The four gates (in order):**
1. `shengen` — regenerate guard types from specs (catches spec drift)
2. `test` — run tests (catches logic errors)
3. `build` — compile against regenerated types (catches type mismatches)
4. `shen-check` — Shen `tc+` on specs (catches spec inconsistency)

**Environment variables:** `RALPH_HARNESS`, `RALPH_MAX_ITER`, `RALPH_HARNESS_TIMEOUT`

---

### `/sb:ralph-scaffold` — Full Setup in One Shot
**When:** Starting from scratch and want Ralph + backpressure together.
**What it does:** Combines `/sb:init` + `/sb:loop` into a single flow.
**Smart detection:** If `/sb:init` was already run, skips to the Ralph loop setup automatically.
**Goes from:** "I have a project" → "four-gate verification is running autonomously"
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

## Shen Runtime

Gate 4 needs a Shen implementation to run `tc+`. Use **shen-sbcl** (Shen on SBCL/Common Lisp). shengen is a separate text-processing tool that does NOT need a Shen runtime.

Install: `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl`

If SBCL is already installed: shen-sbcl can be added on top. Do NOT use shen-go (known crash bugs).

## Concepts

**Shen specs** — Sequent-calculus type definitions in `specs/core.shen`. These are the source of truth. Shen's type checker (`tc+`) proves they're internally consistent.

**Guard types** — Generated code with opaque constructors. The ONLY way to create a value is through the constructor, which validates the spec's preconditions. Wrap at system boundaries, trust internally.

**Backpressure** — When generated types change (because specs changed), code that uses them breaks at compile time. This forces the developer (or LLM) to update usage to match the new invariants.

**Ralph loop** — An autonomous outer loop: call LLM harness → run four gates → inject failures into next prompt → repeat until all gates pass.

## Typical Workflow

```
/sb:init                     # Set up specs + guard types
# ... write domain code using guard types ...
/sb:loop                     # (optional) Add autonomous Ralph loop
make run                     # Launch the loop
```

Or all at once:
```
/sb:ralph-scaffold           # Everything from zero to running
make run                     # Launch
```
