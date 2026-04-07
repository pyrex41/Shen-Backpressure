# Implementation Plan: All Stubbed Examples

This plan covers the 13 stubbed examples in `examples/`. Each follows the proven pattern established by the completed examples (`payment/`, `email-crud/`, `multi-tenant-api/`).

## Reference Architecture (from completed examples)

Every example follows this structure:

```
examples/<name>/
├── PROMPT.md              # Already exists (vision + requirements)
├── README.md              # Project overview + how to run
├── Makefile               # all, build, test, shen-check, run, clean
├── specs/
│   └── core.shen          # Shen sequent-calculus type specs
├── bin/
│   └── shen-check.sh      # Gate 4: Shen tc+ validation script
├── cmd/
│   └── ralph/
│       └── main.go        # Five-gate Ralph loop runner
├── internal/
│   └── shenguard/
│       └── guards_gen.go  # Generated guard types (from shengen)
├── reference/             # Reference guard outputs for other languages
├── prompts/               # Exploration/implementation prompts
└── src/ or domain code    # Application code using guard types
```

**Implementation recipe per example:**
1. Write `specs/core.shen` (the formal invariants)
2. Run `shengen` to produce `internal/shenguard/guards_gen.go` (or equivalent per language)
3. Scaffold project (`go.mod`, `Makefile`, `bin/shen-check.sh`, `cmd/ralph/main.go`)
4. Write application code that imports and uses the guard types
5. Write tests demonstrating both valid construction and invariant violation
6. Generate reference outputs for other target languages where applicable
7. Write `README.md`
8. Run all five gates to verify

---

## Wave 1: Simple Go Showcases (Foundation)

These are the simplest examples. They establish core patterns with minimal application code. No external dependencies beyond Go stdlib.

---

### 1.1 pipeline-state-machine
**Complexity:** Simple | **Language:** Go | **~LOC:** 150

**What it demonstrates:** Linear stage ordering enforced at compile time. You cannot skip stages.

**specs/core.shen:**
```shen
(datatype query
  X : string;
  (not (= X "")) : verified;
  ====================
  X : valid-query;)

(datatype search-result
  Q : valid-query;
  Results : string;
  ========================
  [Q Results] : search-result;)

(datatype fetched-content
  SR : search-result;
  Content : string;
  ===========================
  [SR Content] : fetched-content;)

(datatype summary
  FC : fetched-content;
  Text : string;
  (not (= Text "")) : verified;
  ==============================
  [FC Text] : summary;)
```

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — 4 guard types with linear dependency chain
- `cmd/demo/main.go` — Demo showing valid pipeline succeeds, skipped stage fails to compile
- `main_test.go` — Test valid path + test that invalid construction returns error

**Success criteria:**
- `NewSummary()` requires a `FetchedContent`, which requires a `SearchResult`, which requires a `ValidQuery`
- Attempting to create a `Summary` without going through all stages is a compile-time or construction-time error
- Clean `go build`, `go test`, and `shen-check` passes

---

### 1.2 sum-type-showcase
**Complexity:** Simple | **Languages:** Go, TypeScript | **~LOC:** 200

**What it demonstrates:** Closed variant enforcement — no unauthorized shape types can be created outside the guard package.

**specs/core.shen:**
```shen
(datatype radius
  X : number;
  (> X 0) : verified;
  ====================
  X : radius;)

(datatype width
  X : number;
  (> X 0) : verified;
  ====================
  X : width;)

(datatype height
  X : number;
  (> X 0) : verified;
  ====================
  X : height;)

(datatype shape
  R : radius;
  =============
  R : shape;

  W : width;
  H : height;
  ===============
  [W H] : shape;)
```

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — Shape interface with private marker, Circle + Rectangle variants
- `ts/guards_gen.ts` — TypeScript discriminated union with `never`-type exhaustiveness
- `cmd/demo/main.go` — Pattern matching, area calculation, exhaustiveness demo
- `ts/demo.ts` — Same demo in TypeScript

**Success criteria:**
- Go: `Shape` interface has unexported method preventing external implementation
- TS: `Shape` union type with `never` default catches missing variants
- External packages cannot create new shape variants

---

### 1.3 category-showcase
**Complexity:** Medium | **Language:** Go | **~LOC:** 250

**What it demonstrates:** All six shengen categories in one spec (the "Rosetta Stone").

**specs/core.shen:** Must include one example of each category:
1. **Wrapper** — `DocumentId` (string, no validation)
2. **Constrained** — `PageCount` (number, > 0)
3. **Alias** — `DraftDocument` = `Document`
4. **Composite** — `Document` with `DocumentId`, `Title`, `PageCount`
5. **Guarded** — `PublishedDocument` requiring approval verification
6. **Sum** — `AccessLevel` = `ReadOnly | ReadWrite`

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — All six patterns in generated code
- `cmd/demo/main.go` — Construct each, show which return errors vs infallible
- `TAXONOMY.md` — Short table mapping Shen pattern → Go construct → error behavior

**Success criteria:**
- Each category is clearly labeled in generated code comments
- Demo shows: wrapper (infallible), constrained (fallible), alias (infallible), composite (infallible if children valid), guarded (fallible), sum (closed)

---

## Wave 2: Medium Go Showcases (Depth)

These build on Wave 1 patterns with more interesting invariant enforcement.

---

### 2.1 llm-hallucination-guard
**Complexity:** Medium | **Language:** Go | **~LOC:** 250

**What it demonstrates:** Closed enumerations that reject LLM-hallucinated values.

**specs/core.shen:**
```shen
(datatype panel-kind
  X : string;
  (element? X ["bar-chart" "line-chart" "pie-chart" "table"
               "metric-card" "scatter-plot" "heatmap"
               "timeline" "map" "text-block"]) : verified;
  =====================================================
  X : panel-kind;)

(datatype title
  X : string;
  (not (= X "")) : verified;
  ==========================
  X : title;)

(datatype data-source
  X : string;
  (not (= X "")) : verified;
  ==========================
  X : data-source;)

(datatype panel
  Kind : panel-kind;
  T : title;
  DS : data-source;
  ==========================
  [Kind T DS] : panel;)

(datatype dashboard-layout
  T : title;
  Panels : (list panel);
  ==========================
  [T Panels] : dashboard-layout;)
```

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — `PanelKind` with closed enum, `Panel`, `DashboardLayout`
- `cmd/demo/main.go` — Parse mock LLM JSON output, show valid panels accepted, hallucinated kinds (`"gauge"`, `"sparkline"`) rejected with clear error messages
- `main_test.go` — Table-driven tests: all 10 valid kinds pass, 5+ invalid kinds fail

**Success criteria:**
- `NewPanelKind("gauge")` → error: `"gauge" is not a valid panel-kind`
- `NewPanelKind("bar-chart")` → success
- Demonstrates backpressure on LLM output

---

### 2.2 relational-constraints
**Complexity:** Medium | **Language:** Go | **~LOC:** 200

**What it demonstrates:** Cross-type field equality invariants (order currency must match region currency).

**specs/core.shen:**
```shen
(datatype currency-code
  X : string;
  (element? X ["USD" "EUR" "GBP" "JPY" "CAD"]) : verified;
  =========================================================
  X : currency-code;)

(datatype tax-rate
  X : number;
  (>= X 0) : verified;
  (<= X 1) : verified;
  ====================
  X : tax-rate;)

(datatype region-config
  Name : string;
  Currency : currency-code;
  Tax : tax-rate;
  ==============================
  [Name Currency Tax] : region-config;)

(datatype price
  X : number;
  (>= X 0) : verified;
  ====================
  X : price;)

(datatype order-pricing
  P : price;
  Currency : currency-code;
  Config : region-config;
  (= Currency (head (tail Config))) : verified;
  =============================================
  [P Currency Config] : order-pricing;)
```

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — Cross-field validation in `NewOrderPricing`
- `cmd/demo/main.go` — Matching currencies succeed, mismatched currencies fail
- `main_test.go` — Test matrix of currency combinations

**Success criteria:**
- `NewOrderPricing(price, usd, usRegion)` → success (both USD)
- `NewOrderPricing(price, eur, usRegion)` → error (EUR ≠ USD)
- Generated Go correctly translates `(head (tail Config))` into field access

---

### 2.3 polyglot-comparison
**Complexity:** Medium | **Languages:** Go, TypeScript, Rust, Python, Python-hardened | **~LOC:** 400

**What it demonstrates:** Same payment spec compiled to 5 targets, showing enforcement spectrum.

**specs/core.shen:** Reuses `examples/payment/specs/core.shen` exactly.

**Key implementation files:**
- `go/demo.go` — Go: unexported fields, constructor errors
- `ts/demo.ts` — TypeScript: private constructor, branded types
- `rs/demo.rs` — Rust: `Result<T, E>`, module privacy
- `py/demo.py` — Python standard: frozen dataclass
- `py-hardened/demo.py` — Python hardened: HMAC provenance
- `comparison-table.md` — Side-by-side matrix of enforcement mechanisms

**Success criteria:**
- Each language demo runs the same 4 scenarios: valid amount, negative amount (fails), bypass attempt (fails), full proof chain
- Table clearly shows enforcement spectrum from Rust (strongest) to Python-standard (weakest)
- All demos driven from the single `specs/core.shen`

---

## Wave 3: Complex Go Applications

Full applications with HTTP servers, persistence, and UIs.

---

### 3.1 order-state-machine
**Complexity:** Complex | **Language:** Go | **~LOC:** 600

**What it demonstrates:** E-commerce order lifecycle with provably valid state transitions, no deadlocks.

**specs/core.shen:** Encodes:
- `order-state` sum type: `created | paid | processing | shipped | delivered | cancelled | refund-requested | refunded`
- `allowed-transition?` define with valid pairs
- `state-transition` guarded type requiring `(allowed-transition? From To)`
- `order` composite: `Id`, `State`, `History`

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — State enum, transition guard
- `internal/db/db.go` — SQLite persistence for orders
- `internal/handlers/handlers.go` — HTTP handlers: create order, transition state, view history
- `cmd/server/main.go` — HTTP server with htmx dashboard
- `static/` + `templates/` — Dashboard showing orders by state
- `cmd/ralph/main.go` — Ralph loop runner

**Success criteria:**
- `created→paid` transition succeeds
- `created→shipped` transition fails (not allowed)
- Terminal states (`delivered`, `cancelled`, `refunded`) reject all outward transitions
- Every non-terminal state has ≥1 valid outward transition (no deadlocks)
- Dashboard shows orders grouped by state with valid transition buttons

---

### 3.2 dosage-calculator
**Complexity:** Complex | **Language:** Go | **~LOC:** 700

**What it demonstrates:** Clinical dosage safety AND drug interaction clearance — both required for administration.

**specs/core.shen:** Encodes:
- `patient-weight`: 0 < weight ≤ 500
- `dose-range`: max ≥ min, both non-negative
- `safe-dose`: within weight-based range
- `interaction-clear`: no contraindicated drug pairs (recursive check against medication list)
- `safe-administration`: requires BOTH `safe-dose` AND `interaction-clear` (no shortcuts)

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — Full proof chain types
- `internal/db/db.go` — SQLite for patients, drugs, dosage tables, interactions
- `internal/handlers/handlers.go` — CRUD + dosage calculator + administration endpoint
- `cmd/server/main.go` — HTTP server
- `static/` + `templates/` — Clinical dashboard with forms
- `cmd/ralph/main.go` — Ralph loop runner

**Success criteria:**
- Safe dose within range + no interactions → administration succeeds
- Dose out of range → error (even if no interactions)
- Interactions present → error (even if dose is safe)
- Both must be proven simultaneously — no shortcut construction

---

### 3.3 shen-go-api
**Complexity:** Medium | **Language:** Go | **~LOC:** 400

**What it demonstrates:** High-performance Go API with Shen-enforced invariants as the "middle ground" framework example.

**specs/core.shen:** Builds on payment domain with additional:
- Request/response validation types
- API error types (closed enum)
- Pagination with bounds checking

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — Guard types for API contracts
- `internal/api/` — HTTP handlers using guard types for all input validation
- `cmd/server/main.go` — stdlib `net/http` server
- `cmd/ralph/main.go` — Ralph loop runner
- Benchmarks comparing guard-validated vs raw validation

**Success criteria:**
- All API inputs validated through guard constructors
- Invalid requests fail at guard boundary, not in business logic
- Benchmark shows guard overhead is negligible

---

## Wave 4: Advanced Go

---

### 4.1 shen-go-advanced
**Complexity:** Complex | **Language:** Go | **~LOC:** 500

**What it demonstrates:** Backpressure and provable concurrency via graded modalities/linear logic mapped to goroutines and channels.

**specs/core.shen:** Encodes:
- `bounded-buffer` with capacity invariant
- `demand-signal` for consumer-driven flow control
- `session-protocol` for stream processing stages
- Liveness property: consumer always eventually reads
- Safety property: producer never overflows buffer

**Key implementation files:**
- `internal/shenguard/guards_gen.go` — Channel wrapper types with capacity guards
- `internal/pipeline/` — Stream processing pipeline with backpressure
- `cmd/demo/main.go` — Demo under varying consumer speeds
- Benchmarks showing resilience under load vs unguarded channels

**Success criteria:**
- Producer blocks (backpressure) when buffer full — no data loss
- Consumer at various speeds (fast, slow, bursty) never causes deadlock
- Shen spec proves properties Go's type system alone cannot express
- Benchmark under 10k messages shows bounded memory usage

---

## Wave 5: Polyglot Frameworks

These extend beyond Go, requiring `shengen-ts`, `shengen-py`, and `shengen-rs` respectively.

---

### 5.1 shen-hono
**Complexity:** Complex | **Language:** TypeScript (Bun/Node) | **~LOC:** 500

**What it demonstrates:** Shen as control plane for Hono web framework with TypeScript branded types.

**specs/core.shen → specs/api-specs.shen:** Encodes:
- Route contracts (valid endpoints, methods)
- Input validation types (non-empty strings, length limits, valid queries)
- State machine for workflows (no invalid transitions)
- Pipeline ordering (cannot summarize without search+fetch)
- Response grounding (all outputs traced to sources)

**Key implementation files:**
- `src/guards/guards_gen.ts` — shengen-ts output: branded types + runtime validators
- `src/app.ts` — Hono app with routes importing guards
- `src/middleware/validate.ts` — Zod-like middleware powered by guard types
- `src/routes/` — Route handlers: `/api/research`, `/api/search`, `/api/tasks`, `/api/state`
- `package.json`, `tsconfig.json`, `bun.lockb`
- `Makefile` — build, test, typecheck, shen-check

**Success criteria:**
- Invalid API calls rejected at guard boundary before handler executes
- TypeScript compiler catches type mismatches at build time
- Benchmark vs plain Hono shows minimal overhead
- Demonstrates Shen provides stronger guarantees than Zod alone

---

### 5.2 shen-fastapi
**Complexity:** Complex | **Language:** Python 3.11+ | **~LOC:** 500

**What it demonstrates:** Shen-generated Pydantic models replacing hand-written validators.

**specs/core.shen:** Encodes:
- Complex nested validators beyond Pydantic built-ins
- Grounded data flows
- Resource protocols (DB connection lifecycle)
- Research pipeline stages

**Key implementation files:**
- `src/guards/guards_gen.py` — shengen-py output: Pydantic BaseModels with validator methods
- `src/app.py` — FastAPI app with dependency injection
- `src/routes/` — Async endpoints using guard types
- `src/models/` — Domain models extending generated guards
- `requirements.txt`, `pyproject.toml`
- `Makefile` — run, test, typecheck (mypy), shen-check

**Success criteria:**
- Generated Pydantic models enforce invariants Pydantic alone cannot
- Async endpoints use guard constructors for all input validation
- Benchmark vs plain FastAPI shows overhead profile
- mypy passes with full type coverage

---

### 5.3 shen-rust-axum
**Complexity:** Complex | **Language:** Rust 2024 | **~LOC:** 600

**What it demonstrates:** Maximum performance + safety — Shen complements Rust's borrow checker with logical invariants.

**specs/core.shen:** Encodes:
- State transitions with provable absence of data races
- Resource protocols (connection pools, rate limiters)
- Complex business rules spanning multiple entities
- Linear logic for resource management

**Key implementation files:**
- `src/guards/guards_gen.rs` — shengen-rs output: `Result<T, E>` constructors, module privacy
- `src/main.rs` — Axum app with Tower middleware
- `src/routes/` — Handlers using guard types
- `src/state.rs` — App state with type-state pattern
- `Cargo.toml`, `Cargo.lock`
- `Makefile` — build, test, clippy, shen-check

**Success criteria:**
- `cargo build` with zero warnings
- `cargo clippy` clean
- Guard types complement (not duplicate) Rust's ownership guarantees
- Benchmark shows Shen guards add zero runtime overhead beyond the validation logic itself
- Demonstrates invariants Rust's type system alone cannot express

---

## Wave 6: Frontier

---

### 6.1 shen-prolog-ui
**Complexity:** Complex | **Languages:** TypeScript + Go | **~LOC:** 800

**What it demonstrates:** Three-layer generative UI: Shen Prolog proves layout valid → Pretext proves text fits → Arrow.js renders reactively.

**specs/core.shen:** Encodes:
- UI component binding validity (data source has required field types)
- Chart constraints (bar chart: numeric Y + categorical X)
- Drill-down requires hierarchical parent-child relationship
- Shared filter: all bound components use same data source
- Layout validity (all children recursively valid)
- Horizontal alignment: compatible height constraints

**Key implementation files:**
- Backend (Go):
  - `backend/internal/shenguard/guards_gen.go` — Layout constraint guard types
  - `backend/internal/prolog/` — Shen-Go Prolog integration for constraint resolution
  - `backend/cmd/server/main.go` — HTTP server serving layout specs
- Frontend (TypeScript):
  - `frontend/src/guards/guards_gen.ts` — TS guard types for UI components
  - `frontend/src/layout/` — Pretext measurement integration
  - `frontend/src/render/` — Arrow.js reactive rendering
  - `frontend/package.json`
- `Makefile` — build-backend, build-frontend, test, shen-check

**Success criteria:**
- Shen Prolog rejects invalid layout specs (e.g., bar chart on non-numeric field)
- Pretext measures text in ~0.05ms (no DOM reflow)
- Arrow.js renders valid layouts reactively
- Three-level backpressure: Shen rejects invalid specs → TS rejects invalid code → Pretext prevents layout thrash

---

## Implementation Order & Dependencies

```
Wave 1 (no deps, Go stdlib only)
  ├── 1.1 pipeline-state-machine
  ├── 1.2 sum-type-showcase
  └── 1.3 category-showcase

Wave 2 (builds on Wave 1 patterns)
  ├── 2.1 llm-hallucination-guard
  ├── 2.2 relational-constraints
  └── 2.3 polyglot-comparison ← needs all shengen-* targets working

Wave 3 (full apps, needs HTTP + SQLite patterns from email-crud/multi-tenant-api)
  ├── 3.1 order-state-machine
  ├── 3.2 dosage-calculator
  └── 3.3 shen-go-api

Wave 4 (advanced Go concurrency)
  └── 4.1 shen-go-advanced

Wave 5 (polyglot, needs respective shengen-* codegen tools)
  ├── 5.1 shen-hono ← needs shengen-ts
  ├── 5.2 shen-fastapi ← needs shengen-py
  └── 5.3 shen-rust-axum ← needs shengen-rs

Wave 6 (frontier, needs Shen-Go Prolog + Pretext + Arrow.js)
  └── 6.1 shen-prolog-ui ← needs shen-go, shengen-ts, Pretext, Arrow.js
```

## Per-Example Checklist Template

For each example, the implementer (human or Ralph loop) follows this checklist:

- [ ] Write/refine `specs/core.shen`
- [ ] Run `shengen` → generate `internal/shenguard/guards_gen.go` (or target equivalent)
- [ ] Scaffold: `go.mod` / `package.json` / `Cargo.toml`, `Makefile`, `bin/shen-check.sh`
- [ ] Create `cmd/ralph/main.go` (Ralph loop runner)
- [ ] Implement application code using guard types
- [ ] Write tests (valid + invalid construction, edge cases)
- [ ] Generate reference outputs for other languages (where applicable)
- [ ] Write `README.md` with usage instructions + what it demonstrates
- [ ] Pass all five gates: shengen → test → build → shen tc+ → tcb audit
- [ ] Verify demo scenario works end-to-end

## Estimated Effort

| Wave | Examples | Effort per | Total |
|------|----------|-----------|-------|
| 1 | 3 simple/medium | Small | Small |
| 2 | 3 medium | Medium | Medium |
| 3 | 3 complex | Large | Large |
| 4 | 1 complex | Large | Large |
| 5 | 3 complex | Large | Very Large |
| 6 | 1 complex | Very Large | Very Large |

**Recommendation:** Implement Waves 1-2 first. They provide the most pedagogical value per effort — covering all shengen categories, the enforcement spectrum, and key patterns (state machines, closed enums, cross-type constraints). Waves 3+ build on these patterns for full-stack applications.
