/**
 * runtime/ui.ts — Arrow.js generative UI renderer for the Shen web tools demo.
 *
 * Shen resolves WHAT to render (via ui-resolve.shen). This module handles
 * the HOW — reactive DOM rendering using Arrow.js.
 *
 * Arrow.js API:
 *   reactive({...})  → creates reactive data (auto-tracks dependencies)
 *   html`...`         → tagged template that produces reactive DOM
 *   ${() => expr}     → reactive expression (re-evaluates when deps change)
 *   @click="${fn}"    → event binding
 *
 * Architecture:
 *   Shen emits a UI panel tree: [id, kind, [children...]]
 *   Shen calls bridge.render → onShenRender updates reactive state
 *   Arrow.js automatically re-renders only the parts that changed
 */

import { reactive, html } from "@arrow-js/core";

// ---------------------------------------------------------------------------
// Reactive State (driven by Shen via bridge.render)
// ---------------------------------------------------------------------------

const state = reactive({
  phase: "idle" as string,
  query: "" as string,
  searchHits: [] as any[],
  sources: [] as any[],
  summary: "" as string,
  summaryMeta: "" as string,
  panels: null as any,
  error: "" as string,
  isLoading: false as boolean,
});

// ---------------------------------------------------------------------------
// Render callback (called by Shen via bridge.render)
// ---------------------------------------------------------------------------

export type RenderState = [string, any, any];

export function onShenRender(renderState: RenderState): void {
  const [phase, panel, data] = renderState;
  state.phase = phase;
  state.panels = panel;
  state.isLoading = phase === "searching" || phase === "fetching" || phase === "generating";

  switch (phase) {
    case "idle":
      state.query = "";
      state.searchHits = [];
      state.sources = [];
      state.summary = "";
      state.error = "";
      break;

    case "searching":
      if (Array.isArray(data)) state.query = String(data[0] || data);
      break;

    case "fetching":
      if (Array.isArray(data) && Array.isArray(data[1])) {
        state.searchHits = data[1];
      }
      break;

    case "generating":
      if (Array.isArray(data) && Array.isArray(data[0])) {
        state.sources = data[0];
      }
      break;

    case "complete":
      if (Array.isArray(data)) {
        if (Array.isArray(data[1])) state.sources = data[1];
        if (Array.isArray(data[2])) {
          const [_prompt, text, ts] = data[2];
          state.summary = text || "";
          state.summaryMeta = `Generated at ${new Date(ts).toLocaleTimeString()} from ${state.sources.length} sources`;
        }
      }
      state.isLoading = false;
      break;
  }
}

// ---------------------------------------------------------------------------
// Arrow.js Components
// ---------------------------------------------------------------------------

function pipelineIndicator() {
  const stages = ["idle", "searching", "fetching", "generating", "complete"];
  const labels = ["Ready", "Searching", "Fetching", "Generating", "Done"];

  return html`
    <div class="pipeline">
      ${stages.map((s, i) => html`
        <div class="${() => {
          const idx = stages.indexOf(state.phase);
          return i < idx ? "stage done" : i === idx ? "stage active" : "stage";
        }}">${labels[i]}</div>
        ${i < stages.length - 1 ? html`<div class="stage-connector">→</div>` : ""}
      `)}
    </div>
  `;
}

function searchBar(onSubmit: (q: string) => void) {
  const local = reactive({ value: "" });

  const doSubmit = () => {
    const q = local.value.trim();
    if (q) onSubmit(q);
  };

  return html`
    <div class="search-bar">
      <input
        class="search-input"
        type="text"
        placeholder="Search any topic — Shen resolves what to research..."
        value="${() => local.value}"
        @input="${(e: Event) => { local.value = (e.target as HTMLInputElement).value; }}"
        @keydown="${(e: KeyboardEvent) => { if (e.key === "Enter") doSubmit(); }}"
      />
      <button class="search-btn" @click="${doSubmit}">Research</button>
    </div>
  `;
}

function loadingIndicator() {
  const messages: Record<string, string> = {
    searching: "Shen is dispatching web search via bridge...",
    fetching: "Shen is fetching and grounding sources...",
    generating: "Shen is generating AI summary from grounded sources...",
  };

  return html`
    <div class="loading" style="${() => state.isLoading ? "" : "display:none"}">
      <div class="spinner"></div>
      <p class="loading-text">${() => messages[state.phase] || "Processing..."}</p>
      <p class="loading-detail">
        Pipeline stage: ${() => state.phase} — all logic runs in Shen, TypeScript is only the I/O bridge
      </p>
    </div>
  `;
}

function welcome() {
  return html`
    <div class="welcome" style="${() => state.phase === "idle" ? "" : "display:none"}">
      <h1>Shen Web Tools</h1>
      <p class="welcome-sub">A research assistant where all logic is written in Shen</p>
      <div class="welcome-arch">
        <div class="arch-box">
          <strong>Shen Logic</strong>
          <span>Query refinement, source grounding, UI resolution, prompt construction</span>
        </div>
        <div class="arch-arrow">→</div>
        <div class="arch-box">
          <strong>TypeScript Bridge</strong>
          <span>WebSearch, WebFetch, AI Generate (I/O only)</span>
        </div>
        <div class="arch-arrow">→</div>
        <div class="arch-box">
          <strong>Arrow.js UI</strong>
          <span>Reactive rendering of Shen's UI panel tree</span>
        </div>
      </div>
      <p class="welcome-hint">
        Try: "quantum computing", "climate change solutions", "functional programming"
      </p>
    </div>
  `;
}

function searchHitsList() {
  return html`
    <div class="search-hits" style="${() =>
      state.phase === "fetching" && state.searchHits.length > 0 ? "" : "display:none"
    }">
      <h3>${() => `Found ${state.searchHits.length} results`}</h3>
      ${() => state.searchHits.slice(0, 10).map(
        (hit: any) => html`
          <div class="hit-card">
            <a class="hit-title" href="${hit[1]}" target="_blank">${hit[0]}</a>
            <span class="hit-url">${hit[1]}</span>
            <p class="hit-snippet">${hit[2]}</p>
          </div>
        `
      )}
    </div>
  `;
}

function summaryPanel() {
  return html`
    <div class="summary-panel" style="${() => state.phase === "complete" ? "" : "display:none"}">
      <h2>Research Summary</h2>
      <p class="summary-meta">${() => state.summaryMeta}</p>
      <div class="summary-content">${() => renderMarkdown(state.summary)}</div>
    </div>
  `;
}

/** Simple markdown-ish rendering into HTML string */
function renderMarkdown(text: string): string {
  if (!text) return "";
  return text
    .split("\n\n")
    .map((block) => {
      if (block.startsWith("## ")) return `<h3>${esc(block.slice(3))}</h3>`;
      if (block.startsWith("**") && block.endsWith("**")) return `<h4>${esc(block.slice(2, -2))}</h4>`;
      if (block.startsWith("- ") || block.startsWith("1. ")) {
        const items = block
          .split("\n")
          .map((l) => l.replace(/^[-\d.]\s*/, "").trim())
          .filter(Boolean)
          .map((l) => `<li>${esc(l)}</li>`)
          .join("");
        return `<ul class="summary-list">${items}</ul>`;
      }
      return `<p>${esc(block)}</p>`;
    })
    .join("");
}

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function sourcesPanel() {
  return html`
    <div class="sources-panel" style="${() =>
      state.phase === "complete" && state.sources.length > 0 ? "" : "display:none"
    }">
      <h3>${() => `Grounded Sources (${state.sources.length})`}</h3>
      <p class="sources-subtitle">
        Each source is paired with its search hit — Shen enforces URL matching
      </p>
      ${() => state.sources.map(
        (src: any, i: number) => {
          const [page, hit] = src;
          const [_pageUrl, _content, _ts] = page;
          const [title, hitUrl, snippet] = hit;
          return html`
            <div class="source-card">
              <div class="source-num">${i + 1}</div>
              <div class="source-body">
                <a class="source-title" href="${hitUrl}" target="_blank">${title}</a>
                <p class="source-snippet">${snippet}</p>
                <p class="source-grounded">Grounded: page URL matches hit URL</p>
              </div>
            </div>
          `;
        }
      )}
    </div>
  `;
}

function actionsPanel(onNewSearch: () => void) {
  return html`
    <div class="actions-panel" style="${() => state.phase === "complete" ? "" : "display:none"}">
      <button class="action-btn" @click="${onNewSearch}">New Search</button>
      <button
        class="action-btn action-compare"
        style="${() => state.sources.length > 1 ? "" : "display:none"}"
        @click="${() => { /* would call Shen's on-compare-click */ }}"
      >Compare Sources</button>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Mount — compose all Arrow.js components into the root
// ---------------------------------------------------------------------------

export function mount(root: HTMLElement, onSearch: (q: string) => void): void {
  const onNewSearch = () => {
    state.phase = "idle";
    state.summary = "";
    state.sources = [];
    state.searchHits = [];
  };

  const app = html`
    <div>
      ${pipelineIndicator()}
      ${searchBar(onSearch)}
      ${welcome()}
      ${loadingIndicator()}
      ${searchHitsList()}
      ${summaryPanel()}
      ${sourcesPanel()}
      ${actionsPanel(onNewSearch)}
    </div>
  `;

  app(root);
}
