---
name: shen-backpressure
description: Autonomous AI coding loops with Shen sequent-calculus type backpressure and Go codegen bridge. Activates when the user mentions formal verification, Shen types, guard types, backpressure loops, or Ralph loops. Provides commands for scaffolding, spec generation, and loop execution.
user-invocable: false
---

# Shen-Backpressure

This project uses Shen sequent-calculus types as formal specifications, with a codegen bridge (shengen) that generates Go guard types enforcing those specs at compile time.

## Available Commands

- `/sb:scaffold` — Full setup: domain description → specs → guard types → orchestrator → all four gates verified
- `/sb:setup` — Scaffold directory structure, orchestrator, and four-gate configuration
- `/sb:init` — Generate Shen specs from natural language, build shengen, produce initial guard types
- `/sb:loop` — Verify prerequisites, confirm configuration, launch the Ralph loop

## Architecture

```
specs/core.shen          Shen sequent-calculus type rules
       │
       ▼  (shengen)
internal/shenguard/      Generated Go guard types with opaque constructors
       │
       ▼  (import)
Application code         Uses guard types at domain boundaries
       │
       ▼  (four gates)
Ralph loop               shengen → go test → go build → shen tc+
```

The inner LLM harness (claude -p, cursor-agent, codex, etc.) does the coding work. Ralph orchestrates the loop. The four gates provide backpressure — if any gate fails, errors are fed back into the next iteration's prompt.

## Guard Type Discipline

See `AGENT_PROMPT.md` for the full reference on how the inner harness should use guard types: wrap at the boundary, trust internally, follow the proof chain, extract with `.Val()`.
