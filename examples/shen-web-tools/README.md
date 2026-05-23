# Shen Web Tools — Research Assistant

A research assistant where **all application logic is written in Shen**, running natively on **Common Lisp (SBCL)**. Arrow.js handles the frontend rendering.

## Architecture

```
Arrow.js Frontend (browser)
  ├── pi-ai (optional: client-side LLM streaming)
  ↕ HTTP JSON API
Common Lisp Backend (SBCL)
  ├── Shen runtime (loaded at boot)
  │   ├── specs/core.shen     ← formal type specs (sequent calculus)
  │   ├── src/web-tools.shen  ← web tool definitions + combinators
  │   ├── src/ai-gen.shen     ← prompt construction + response processing
  │   ├── src/ui-resolve.shen ← UI layout resolution (Prolog-style)
  │   └── src/app.shen        ← pipeline orchestration
  ├── CL bridge (bridge.lisp) — pluggable providers:
  │   ├── web-search  → DuckDuckGo (built-in, no API key) | rho-cli | Brave
  │   ├── web-fetch   → dexador + HTML→text | rho-cli
  │   └── ai-generate → Anthropic API | rho-cli
  └── HTTP server (server.lisp)
      └── hunchentoot serving JSON API + static files
```

**Shen decides WHAT to do. CL does the I/O. Arrow renders the result.**

## Tag Resolver Finish Line

This example also carries the current TypeScript proof gate for product
tag/ref-table rendering. `specs/core.shen` defines `tag-block`,
`ref-table`, `tag-provenance`, and the `tag-resolve-outcome` sum type.
The resolver contract in `specs/tag-block-resolver.shen` resolves child
refs into:

- `signed-complete`: every child ref resolves AND the root's signature
  passes a deterministic stub-HMAC predicate against the body and
  child-ref count. The outcome carries a structured `tag-provenance`
  composite (signer-id + stamp + signature), not a raw signature
  string.
- `unsigned-complete`: every child ref resolves, but the root's
  signature is empty OR fails the stub-HMAC predicate.
- `partial`: at least one child ref is missing, while available
  children remain renderable.

The stub-HMAC predicate is documented in
`specs/tag-block-resolver.shen`. It is intentionally a structural
stand-in for a real cryptographic HMAC: a signature is valid iff
`(length Signature) mod 256` equals
`((length Body) * 31 + (length ChildRefs) * 17 + 24) mod 256`. This
demonstrates how a real HMAC slots into the spec — by replacing the
two helper defines with calls to a future `:runtime-via` cryptographic
primitive — without claiming the demo is cryptographic.

The hand-written implementation in `runtime/tag_resolver.ts` is checked
by `runtime/tag_resolver.shen-derive.test.ts`, generated from the Shen
spec. `runtime/tag_render_contract.ts` is the renderer boundary: UI
code consumes `TagRenderState`, not raw ref-table data. The renderer
state now exposes the typed `provenance` composite (signer, stamp,
signature) so consumers don't have to re-derive structure from the
raw signature string.

See `thoughts/shared/research/2026-05-22-tag-resolver-closed.md` for
the decisions on each open question
(`2026-05-05-tag-resolver-open-questions.md`).

## Providers

The CL backend supports pluggable providers for each I/O operation:

| Operation | Provider | Description | API Key? |
|-----------|----------|-------------|----------|
| Search | `:duckduckgo` | DuckDuckGo HTML scraping (same as rho-cli) | No |
| Search | `:rho` | Shell out to rho-cli binary | No |
| Search | `:live` | Brave Search API | `BRAVE_API_KEY` |
| Search | `:mock` | Fake results for dev | No |
| Fetch | `:duckduckgo` | Direct HTTP + HTML→text via dexador | No |
| Fetch | `:rho` | Shell out to rho-cli binary | No |
| Fetch | `:mock` | Fake content for dev | No |
| AI | `:anthropic` | Anthropic Messages API (direct HTTP) | `ANTHROPIC_API_KEY` |
| AI | `:rho` | Shell out to rho-cli (uses its configured model) | Via rho config |
| AI | `:mock` | Fake summary for dev | No |

**Default**: DuckDuckGo search + dexador fetch (no API keys needed). AI defaults to mock unless `ANTHROPIC_API_KEY` is set.

### rho-cli integration

[rho](https://github.com/pyrex41/rho) is a Rust AI coding agent with built-in web search (DuckDuckGo) and fetch tools. When installed, you can use it as a provider:

```bash
# Install rho-cli
git clone https://github.com/pyrex41/rho.git && cd rho && cargo install --path .

# Use rho for everything
./backend/start.sh --search rho --fetch rho --ai rho
```

### pi-ai integration (frontend)

[pi-ai](https://github.com/badlogic/pi-mono/tree/main/packages/ai) enables client-side LLM streaming directly in the browser. This is optional — by default, AI generation goes through the CL backend. To enable:

```bash
npm install @mariozechner/pi-ai
```

Then use `streamGenerate()` in the frontend for token-by-token streaming with 18+ LLM providers.

## `:runtime-via` prototype (W3.2)

One verified premise on `query-text` is discharged at runtime by the
live Shen runtime hosted in the SBCL backend rather than by an
inlined predicate. The annotation:

```shen
(datatype query-text
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : query-text;)
```

Shen treats the trailing `\* ... *\` block as a comment so `tc+`
ignores it. shengen-ts reads the marker and emits a constructor that
imports `shenEval` from `runtime/runtime_checkers.ts` and calls it
inside an async `createOrThrow(ctx, x)`. The constructor cannot
compile without the named function (compile-time witness), and there
is no path to a `QueryText` value that skips the runtime call.

The checker `POST`s to the backend's `/api/eval-predicate`, which
dispatches via an allowlist (`*runtime-predicates*` in
`backend/shen-interop.lisp`). The default `query-text` predicate
delegates to the live Shen evaluator when available and falls back
to CL otherwise.

See `docs/RUNTIME-VIA.md` for the full design, trust-model
implications, and a manual smoke procedure.

## Key Invariant

The Shen spec enforces that AI summaries must be grounded in real sources:

```shen
(datatype grounded-source
  Page : fetched-page;
  Hit : search-hit;
  (= (head Page) (head (tail Hit))) : verified;
  =============================================
  [Page Hit] : grounded-source;)
```

You cannot construct a `research-summary` without `grounded-source` values — and those require matching URLs between fetched pages and search hits.

## Prerequisites

1. **SBCL** (Steel Bank Common Lisp):
   ```bash
   # macOS
   brew install sbcl
   # Ubuntu
   sudo apt install sbcl
   ```

2. **Quicklisp** (CL package manager):
   ```bash
   curl -O https://beta.quicklisp.org/quicklisp.lisp
   sbcl --load quicklisp.lisp \
        --eval '(quicklisp-quickstart:install)' \
        --eval '(ql:add-to-init-file)' \
        --quit
   ```

3. **Shen on SBCL**:
   ```bash
   git clone https://github.com/Shen-Language/shen-sbcl.git
   cd shen-sbcl && make && sudo make install
   ```

4. **Node.js** (for frontend build only):
   ```bash
   npm install
   ```

5. **Optional — rho-cli** (for web tools via Rust):
   ```bash
   git clone https://github.com/pyrex41/rho.git
   cd rho && cargo install --path .
   ```

## Running

```bash
# Build frontend + start CL backend (auto-detects providers)
make serve

# Or step by step:
make frontend          # compile Arrow.js TypeScript
./backend/start.sh     # boot SBCL + Shen + hunchentoot

# With real AI:
ANTHROPIC_API_KEY=sk-... ./backend/start.sh

# DuckDuckGo search + Anthropic AI (no rho needed):
ANTHROPIC_API_KEY=sk-... ./backend/start.sh --search duckduckgo

# Use rho-cli for everything:
./backend/start.sh --search rho --fetch rho --ai rho

# Custom port:
./backend/start.sh --port 8080
```

Then visit `http://localhost:3000`.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/research` | Full pipeline (Shen orchestrates all steps) |
| POST | `/api/search` | Web search only |
| POST | `/api/fetch` | Fetch URL only |
| POST | `/api/generate` | AI generation only |
| GET | `/api/state` | Current pipeline state (for polling) |

## File Layout

```
specs/core.shen          Shen sequent-calculus type specs
src/web-tools.shen       Web tool definitions (calls CL bridge)
src/ai-gen.shen          AI prompt logic
src/ui-resolve.shen      Generative UI resolution
src/app.shen             Main pipeline orchestration
backend/
  packages.lisp          CL package + Quicklisp deps + provider config
  bridge.lisp            Pluggable providers: DuckDuckGo, rho-cli, Brave, Anthropic
  server.lisp            Hunchentoot HTTP server + JSON API
  shen-interop.lisp      Load Shen, register bridge functions
  load.lisp              Bootstrap (load all, auto-detect, start server)
  start.sh               Shell launcher for SBCL
runtime/
  bridge.ts              API client + optional pi-ai streaming
  ui.ts                  Arrow.js reactive UI renderer
  tag_resolver.ts        Tag/ref-table resolver implementation
  tag_render_contract.ts Renderer-facing tri-state tag outcome adapter
  tag_resolver.shen-derive.test.ts
                         Generated spec-equivalence test for the resolver
  main.ts                Frontend bootstrap
index.html               Entry point (loads Arrow.js via importmap)
static/style.css         Styles
```

## Pipeline

Shen enforces a strict pipeline order via types:

1. **query** → validate and refine (`refine-query`)
2. **search** → web search via CL bridge (`search-and-collect`)
3. **fetch** → retrieve top pages via CL bridge (`fetch-top-n`)
4. **ground** → pair pages with hits, enforce URL match (`ground-sources`)
5. **generate** → AI summary from grounded sources (`summarize-with-sources`)
6. **render** → assemble UI panel tree (`assemble-research-view`)

The type system prevents skipping steps — you can't build a `research-summary` without `grounded-source` values.

## Verification Gate

The active `shen-derive-ts` gate in `sb.toml` targets
`resolve-tag-block-children`:

```bash
npm run shengen      # regenerate runtime/guards_gen.ts from specs/core.shen
npm run shen-derive  # regenerate runtime/tag_resolver.shen-derive.test.ts
npm run check        # shengen drift check + build + runtime tests
```

The derive sampler prepends three correlated fixture rows for the resolver:
signed complete, unsigned complete, and partial. That keeps the phase-critical
cases covered even when generic cartesian sampling changes.

## Auditor Workflow

If you're reviewing this demo for security or compliance purposes,
start with [`AUDIT.md`](AUDIT.md) — a one-page reviewer workflow
that walks through verifying the spec hash, reading the committed
audit report, re-running gates at the recorded commit, and reading
the TCB (Shen runtime boot, CL bridge providers, the
`grounded-source` URL-match constructor, the hand-written tag
resolver pinned by `shen-derive`). The project-level trust model
lives at [`../../docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md).
