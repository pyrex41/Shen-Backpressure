Generative UI system where Shen's built-in Prolog engine is the reasoning layer that decides WHAT to render, Pretext handles text measurement without DOM reflow, and Arrow.js is the reactive rendering layer that handles the DOM. The LLM describes intent, Shen resolves constraints to produce a declarative UI description, shengen-ts generates TypeScript guard types, Pretext pre-calculates all text dimensions, and an Arrow sandbox renders the result.

This is Option C from the Shen-frontend analysis: Shen reasons, Pretext measures, Arrow renders.

## Architecture

```
User intent (natural language)
       |
       v
LLM generates Shen query
       |
       v
Shen Prolog engine resolves constraints:
  - Which components fit the intent?
  - What data do they need?
  - What states are valid?
  - What transitions are allowed?
       |
       v
Declarative UI description (JSON)
  { components: [...], bindings: [...], constraints: [...] }
       |
       v
shengen-ts generates guard types from specs/core.shen
       |
       v
Pretext measurement pass (@chenglou/pretext)
  - Pre-calculates text dimensions from layout JSON
  - Zero DOM reflow — pure arithmetic on cached font metrics
  - Chart labels, axis text, table cells, filter pills all sized before render
  - 0.05ms vs 30ms per measurement (vs getBoundingClientRect)
       |
       v
Arrow.js sandbox payload (main.ts)
  - Imports guard types for state validation
  - Receives pre-computed text dimensions from Pretext
  - Renders reactive components with known layout sizes
  - State transitions go through guard constructors
  - Invalid states are construction errors, not runtime bugs
  - Zero layout thrash — no reflow on drill-down or filter changes
```

## Stack

- **Reasoning**: Shen-Go with Prolog queries for constraint resolution
- **Codegen**: shengen-ts for TypeScript guard types
- **Measurement**: Pretext (`@chenglou/pretext`) for reflow-free text layout
- **Frontend**: Arrow.js (reactive, ~5kb, zero deps) running in sandbox
- **Backend**: Go stdlib net/http serving the Shen engine as an API
- **No frameworks** on either side

## Why Pretext (by Cheng Lou)

The dashboard builder generates layouts dynamically — every drill-down, filter change, and data refresh produces new text (labels, values, axis titles). Normally the browser must reflow to measure each piece of text. Pretext eliminates this entirely:

- **`prepare(text, font)`** — one-time analysis, segments text, caches font metrics (~19ms for 500 texts)
- **`layout(prepared, maxWidth, lineHeight)`** — pure arithmetic, returns height and line count (~0.05ms)
- **Zero reflow** — no `getBoundingClientRect`, no `offsetHeight`, no layout passes
- **All languages** — handles emojis, bidirectional text, CJK, browser quirks

This matters for generative UI because text dimensions are unpredictable at build time. When Shen emits a new layout, Pretext pre-computes all text sizes from the JSON description before Arrow touches the DOM. Arrow renders with known dimensions — no measurement, no jank, no layout shift.

The three-layer pipeline: Shen proves the layout is **valid**, Pretext proves the text **fits**, Arrow renders it **reactively**.

## Domain: Configurable Dashboard Builder

Users describe what they want to see ("show me sales by region with drill-down"). The system:

1. Parses intent into a Shen Prolog query
2. Resolves which visualization components match (bar chart, table, map, etc.)
3. Checks data source compatibility (does this dataset have a "region" field?)
4. Validates component composition (can these two components share a filter?)
5. Emits a declarative layout that Arrow renders

## Shen Spec (specs/core.shen)

Domain entities:
- Data sources with typed schemas (fields, types, cardinality)
- Visualization components with required field types (bar needs numeric Y + categorical X)
- Layout containers (row, column, tabs, grid) with child constraints
- Filters that bind across components sharing a data source
- Drill-down paths that require parent-child field relationships

Invariants:
- A visualization can only bind to a data source that has the required field types
- A bar chart requires at least one numeric field (Y axis) and one categorical field (X axis)
- A drill-down requires a hierarchical relationship between the parent and child fields
- Shared filters require all bound components to use the same data source
- A layout is valid only if all children are valid (recursive proof)
- Component composition: two components in the same row must have compatible height constraints

Prolog rules (embedded in Shen):
- `(compatible? DataSource Component)` — resolves whether a data source satisfies a component's field requirements
- `(valid-drilldown? Parent Child DataSource)` — checks hierarchical field relationship
- `(resolve-layout Intent DataSources)` — given user intent and available data, produces a valid layout tree
- `(suggest-filters Layout)` — finds fields that appear in multiple components and can be shared filters

## Guard Types (via shengen-ts)

The generated TypeScript guards ensure the Arrow sandbox can only render valid configurations:
- `DataBinding` requires `ValidSource` + `CompatibleComponent` proofs
- `DrillDownPath` requires `HierarchicalRelationship` proof
- `SharedFilter` requires `CommonDataSource` proof across bound components
- `ValidLayout` is a recursive proof-carrying type — you can't construct it with invalid children

## The Demo Flow

1. User types: "show me monthly revenue by product category, with drill-down to individual products"
2. Backend sends this to Shen Prolog engine
3. Shen resolves: revenue is numeric (Y axis OK), product_category is categorical (X axis OK), product is child of product_category (drill-down OK) → bar chart with drill-down
4. Shen emits JSON layout description
5. Frontend receives layout, runs Pretext measurement pass (all labels, axis text, cell values → dimensions in ~0.05ms each, zero reflow)
6. Constructs guard types (DataBinding.create throws if source doesn't match component), renders Arrow components with pre-computed sizes
7. User clicks a bar → drill-down fires → new Shen query for the child level → new layout → Pretext re-measures → Arrow re-renders (no jank)

## What This Demonstrates

- Shen Prolog as a **constraint solver** for UI generation — not just type checking, but actively finding valid configurations
- Guard types at the **frontend boundary** — the Arrow sandbox can't render an invalid dashboard
- Pretext as the **measurement layer** — text dimensions are known before rendering, eliminating reflow entirely
- The full loop: natural language → Prolog resolution → type-safe codegen → reflow-free measurement → reactive rendering
- Backpressure works at THREE levels: Shen rejects invalid specs (gate 4), TypeScript rejects invalid UI code (gate 3 via shengen-ts), Pretext prevents layout thrash (gate 0 — the render pipeline itself)

Use /sb:ralph-scaffold to set up the Ralph loop, then run the loop to build it out. The Shen Prolog integration is the novel part — start with the reasoning engine and work outward to the UI.
