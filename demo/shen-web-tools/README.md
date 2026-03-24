# Shen Web Tools — Research Assistant

A research assistant where **all application logic is written in Shen**. The app searches the web, fetches pages, generates AI summaries, and renders a generative UI — with Shen controlling every decision.

## Architecture

```
User query (natural language)
       |
       v
Shen Engine (src/*.shen)
  - refine-query: expand/improve the search query
  - search-and-collect: dispatch web search
  - fetch-top-n: fetch relevant pages
  - ground-sources: pair pages with search hits (KEY INVARIANT)
  - summarize-with-sources: construct AI prompt, generate summary
  - resolve-ui: decide what UI components to show
  - assemble-research-view: build the final UI panel tree
       |
       v (js.call "bridge.X" ...)
TypeScript Bridge (runtime/bridge.ts)
  - webSearch → harness WebSearch tool or API
  - webFetch → harness WebFetch tool or API
  - aiGenerate → Anthropic API or harness tool
  - render → push UI state to Arrow
       |
       v
Arrow UI (runtime/ui.ts)
  - Reactive signals driven by Shen's render calls
  - Components: search bar, loading, search hits, sources, summary
  - Pipeline indicator shows current Shen execution stage
```

## Key Invariant

The Shen spec enforces that **AI-generated summaries must be grounded in real sources**:

```shen
(datatype grounded-source
  Page : fetched-page;
  Hit : search-hit;
  (= (head Page) (head (tail Hit))) : verified;
  =============================================
  [Page Hit] : grounded-source;)
```

You cannot construct a `research-summary` without `grounded-source` values — and those require matching URLs between fetched pages and search hits. This is backpressure: the type system prevents ungrounded AI generation.

## Running

```bash
# Install and serve (mock providers)
make dev

# With real web search and AI
# (requires API keys as URL params)
make dev
# then visit: http://localhost:3000?search=websearch&ai=anthropic&key=YOUR_KEY
```

## Files

| File | Role |
|------|------|
| `specs/core.shen` | Formal type specs (sequent calculus) |
| `src/web-tools.shen` | Web tool definitions + combinators |
| `src/ai-gen.shen` | AI prompt construction + response processing |
| `src/ui-resolve.shen` | UI layout resolution (Prolog-style) |
| `src/app.shen` | Main pipeline orchestration |
| `runtime/bridge.ts` | I/O bridge (the only TypeScript with logic) |
| `runtime/shen-engine.ts` | Minimal Shen evaluator for the browser |
| `runtime/ui.ts` | Arrow.js reactive UI renderer |
| `runtime/main.ts` | Bootstrap: wire Shen + bridge + UI |

## Pipeline States

Shen enforces a strict pipeline order via types:

1. **idle** → `pipeline-idle` — waiting for user input
2. **searching** → `pipeline-searching` — web search in progress
3. **fetching** → `pipeline-fetching` — fetching top pages
4. **generating** → `pipeline-generating` — AI summary from grounded sources
5. **complete** → `pipeline-complete` — rendering safe results
