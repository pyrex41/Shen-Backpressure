/**
 * runtime/ui.ts — Arrow.js generative UI renderer for the Shen web tools demo.
 *
 * Shen resolves WHAT to render (via ui-resolve.shen). This module handles
 * the HOW — reactive DOM rendering using Arrow.js patterns.
 *
 * Architecture:
 *   Shen emits a UI panel tree: [id, kind, [children...]]
 *   This renderer maps each panel to a reactive Arrow component.
 *   State transitions from Shen trigger re-renders automatically.
 *
 * Arrow.js is a ~5kb reactive library. We implement a minimal reactive
 * core here to keep deps zero — same reactive signal/effect pattern.
 */

// ---------------------------------------------------------------------------
// Minimal Reactive Core (Arrow.js-compatible pattern)
// ---------------------------------------------------------------------------

type Subscriber = () => void;

class Signal<T> {
  private _value: T;
  private _subs: Set<Subscriber> = new Set();

  constructor(initial: T) {
    this._value = initial;
  }

  get value(): T {
    // Track dependency if inside an effect
    if (currentEffect) this._subs.add(currentEffect);
    return this._value;
  }

  set value(v: T) {
    if (this._value === v) return;
    this._value = v;
    // Notify subscribers
    for (const sub of this._subs) sub();
  }
}

let currentEffect: Subscriber | null = null;

function effect(fn: () => void): void {
  currentEffect = fn;
  fn();
  currentEffect = null;
}

function signal<T>(initial: T): Signal<T> {
  return new Signal(initial);
}

// ---------------------------------------------------------------------------
// UI State (reactive signals driven by Shen)
// ---------------------------------------------------------------------------

interface AppState {
  phase: Signal<string>;
  query: Signal<string>;
  searchHits: Signal<any[]>;
  sources: Signal<any[]>;
  summary: Signal<string>;
  summaryMeta: Signal<string>;
  panels: Signal<any>;
  error: Signal<string>;
  isLoading: Signal<boolean>;
}

const state: AppState = {
  phase: signal("idle"),
  query: signal(""),
  searchHits: signal([]),
  sources: signal([]),
  summary: signal(""),
  summaryMeta: signal(""),
  panels: signal(null),
  error: signal(""),
  isLoading: signal(false),
};

// ---------------------------------------------------------------------------
// Render callback (called by Shen via bridge.render)
// ---------------------------------------------------------------------------

export type RenderState = [string, any, any];

export function onShenRender(renderState: RenderState): void {
  const [phase, panel, data] = renderState;
  state.phase.value = phase;
  state.panels.value = panel;
  state.isLoading.value = phase === "searching" || phase === "fetching" || phase === "generating";

  switch (phase) {
    case "idle":
      state.query.value = "";
      state.searchHits.value = [];
      state.sources.value = [];
      state.summary.value = "";
      break;

    case "searching":
      // data is the search query
      if (Array.isArray(data)) state.query.value = String(data[0] || data);
      break;

    case "fetching":
      // data is search result [query, hits, ts]
      if (Array.isArray(data) && Array.isArray(data[1])) {
        state.searchHits.value = data[1];
      }
      break;

    case "generating":
      // data is [sources, query]
      if (Array.isArray(data) && Array.isArray(data[0])) {
        state.sources.value = data[0];
      }
      break;

    case "complete":
      // data is research-summary [query, sources, response]
      if (Array.isArray(data)) {
        if (Array.isArray(data[1])) state.sources.value = data[1];
        if (Array.isArray(data[2])) {
          const [prompt, text, ts] = data[2];
          state.summary.value = text || "";
          state.summaryMeta.value = `Generated at ${new Date(ts).toLocaleTimeString()} from ${state.sources.value.length} sources`;
        }
      }
      state.isLoading.value = false;
      break;
  }
}

// ---------------------------------------------------------------------------
// DOM Renderer
// ---------------------------------------------------------------------------

function h(tag: string, attrs: Record<string, any>, ...children: (string | HTMLElement | null)[]): HTMLElement {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "className") el.className = v;
    else if (k.startsWith("on")) el.addEventListener(k.slice(2).toLowerCase(), v);
    else el.setAttribute(k, String(v));
  }
  for (const child of children) {
    if (child === null) continue;
    if (typeof child === "string") el.appendChild(document.createTextNode(child));
    else el.appendChild(child);
  }
  return el;
}

// ---------------------------------------------------------------------------
// Component Renderers
// ---------------------------------------------------------------------------

function renderSearchBar(onSubmit: (q: string) => void): HTMLElement {
  const container = h("div", { className: "search-bar" });

  const input = h("input", {
    className: "search-input",
    type: "text",
    placeholder: "Search any topic — Shen resolves what to research...",
  }) as HTMLInputElement;

  const btn = h("button", {
    className: "search-btn",
    onClick: () => {
      const q = input.value.trim();
      if (q) onSubmit(q);
    },
  }, "Research");

  input.addEventListener("keydown", (e: KeyboardEvent) => {
    if (e.key === "Enter") {
      const q = input.value.trim();
      if (q) onSubmit(q);
    }
  });

  container.appendChild(input);
  container.appendChild(btn);
  return container;
}

function renderLoading(phase: string): HTMLElement {
  const messages: Record<string, string> = {
    searching: "Shen is dispatching web search via bridge...",
    fetching: "Shen is fetching and grounding sources...",
    generating: "Shen is generating AI summary from grounded sources...",
  };
  const container = h("div", { className: "loading" });
  const spinner = h("div", { className: "spinner" });
  const text = h("p", { className: "loading-text" }, messages[phase] || "Processing...");
  const detail = h("p", { className: "loading-detail" },
    `Pipeline stage: ${phase} — all logic runs in Shen, TypeScript is only the I/O bridge`);
  container.appendChild(spinner);
  container.appendChild(text);
  container.appendChild(detail);
  return container;
}

function renderSearchHits(hits: any[]): HTMLElement {
  const container = h("div", { className: "search-hits" });
  const header = h("h3", {}, `Found ${hits.length} results`);
  container.appendChild(header);

  for (const hit of hits.slice(0, 10)) {
    const [title, url, snippet] = hit;
    const card = h("div", { className: "hit-card" },
      h("a", { className: "hit-title", href: url, target: "_blank" }, title),
      h("span", { className: "hit-url" }, url),
      h("p", { className: "hit-snippet" }, snippet),
    );
    container.appendChild(card);
  }
  return container;
}

function renderSources(sources: any[]): HTMLElement {
  const container = h("div", { className: "sources-panel" });
  const header = h("h3", {}, `Grounded Sources (${sources.length})`);
  const subtitle = h("p", { className: "sources-subtitle" },
    "Each source is paired with its search hit — Shen enforces URL matching");
  container.appendChild(header);
  container.appendChild(subtitle);

  for (let i = 0; i < sources.length; i++) {
    const [page, hit] = sources[i];
    const [pageUrl, content, ts] = page;
    const [title, hitUrl, snippet] = hit;
    const card = h("div", { className: "source-card" },
      h("div", { className: "source-num" }, `${i + 1}`),
      h("div", { className: "source-body" },
        h("a", { className: "source-title", href: hitUrl, target: "_blank" }, title),
        h("p", { className: "source-snippet" }, snippet),
        h("p", { className: "source-grounded" }, `✓ Grounded: page URL matches hit URL`),
      ),
    );
    container.appendChild(card);
  }
  return container;
}

function renderSummary(text: string, meta: string): HTMLElement {
  const container = h("div", { className: "summary-panel" });
  const header = h("h2", {}, "Research Summary");
  const metaEl = h("p", { className: "summary-meta" }, meta);
  container.appendChild(header);
  container.appendChild(metaEl);

  // Simple markdown-ish rendering
  const sections = text.split("\n\n");
  for (const section of sections) {
    if (section.startsWith("## ")) {
      container.appendChild(h("h3", {}, section.slice(3)));
    } else if (section.startsWith("**") && section.endsWith("**")) {
      container.appendChild(h("h4", {}, section.slice(2, -2)));
    } else if (section.startsWith("- ") || section.startsWith("1. ")) {
      const ul = h("ul", { className: "summary-list" });
      for (const line of section.split("\n")) {
        const text = line.replace(/^[-\d.]\s*/, "");
        if (text.trim()) ul.appendChild(h("li", {}, text));
      }
      container.appendChild(ul);
    } else {
      container.appendChild(h("p", {}, section));
    }
  }
  return container;
}

function renderWelcome(): HTMLElement {
  return h("div", { className: "welcome" },
    h("h1", {}, "Shen Web Tools"),
    h("p", { className: "welcome-sub" },
      "A research assistant where all logic is written in Shen"),
    h("div", { className: "welcome-arch" },
      h("div", { className: "arch-box" },
        h("strong", {}, "Shen Logic"),
        h("span", {}, "Query refinement, source grounding, UI resolution, prompt construction"),
      ),
      h("div", { className: "arch-arrow" }, "→"),
      h("div", { className: "arch-box" },
        h("strong", {}, "TypeScript Bridge"),
        h("span", {}, "WebSearch, WebFetch, AI Generate (I/O only)"),
      ),
      h("div", { className: "arch-arrow" }, "→"),
      h("div", { className: "arch-box" },
        h("strong", {}, "Arrow UI"),
        h("span", {}, "Reactive rendering of Shen's UI panel tree"),
      ),
    ),
    h("p", { className: "welcome-hint" },
      "Try: \"quantum computing\", \"climate change solutions\", \"functional programming\""),
  );
}

function renderActions(onNewSearch: () => void, onCompare: () => void, showCompare: boolean): HTMLElement {
  const container = h("div", { className: "actions-panel" });
  container.appendChild(
    h("button", { className: "action-btn", onClick: onNewSearch }, "New Search")
  );
  if (showCompare) {
    container.appendChild(
      h("button", { className: "action-btn action-compare", onClick: onCompare }, "Compare Sources")
    );
  }
  return container;
}

function renderPipelineIndicator(phase: string): HTMLElement {
  const stages = ["idle", "searching", "fetching", "generating", "complete"];
  const labels = ["Ready", "Searching", "Fetching", "Generating", "Done"];
  const currentIdx = stages.indexOf(phase);

  const container = h("div", { className: "pipeline" });
  for (let i = 0; i < stages.length; i++) {
    const cls = i < currentIdx ? "stage done" : i === currentIdx ? "stage active" : "stage";
    container.appendChild(h("div", { className: cls }, labels[i]));
    if (i < stages.length - 1) {
      container.appendChild(h("div", { className: "stage-connector" }, "→"));
    }
  }
  return container;
}

// ---------------------------------------------------------------------------
// Main Render Loop (reactive via signals)
// ---------------------------------------------------------------------------

export function mount(root: HTMLElement, onSearch: (q: string) => void): void {
  effect(() => {
    const phase = state.phase.value;
    const loading = state.isLoading.value;
    const hits = state.searchHits.value;
    const sources = state.sources.value;
    const summaryText = state.summary.value;
    const summaryMeta = state.summaryMeta.value;

    // Clear and rebuild
    root.innerHTML = "";

    // Pipeline indicator
    root.appendChild(renderPipelineIndicator(phase));

    // Search bar (always visible)
    root.appendChild(renderSearchBar(onSearch));

    // Phase-specific content
    if (phase === "idle") {
      root.appendChild(renderWelcome());
    }

    if (loading) {
      root.appendChild(renderLoading(phase));
    }

    if (phase === "fetching" && hits.length > 0) {
      root.appendChild(renderSearchHits(hits));
    }

    if (phase === "complete") {
      root.appendChild(renderSummary(summaryText, summaryMeta));
      if (sources.length > 0) {
        root.appendChild(renderSources(sources));
      }
      root.appendChild(renderActions(
        () => {
          state.phase.value = "idle";
          state.summary.value = "";
          state.sources.value = [];
          state.searchHits.value = [];
        },
        () => { /* compare callback - would call Shen's on-compare-click */ },
        sources.length > 1,
      ));
    }
  });
}
