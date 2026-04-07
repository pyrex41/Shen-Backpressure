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
