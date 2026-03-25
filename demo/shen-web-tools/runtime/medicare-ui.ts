/**
 * runtime/medicare-ui.ts — Generic panel renderer for Medicare generative UI
 *
 * This file has ZERO domain knowledge. It renders whatever panels
 * the backend (Shen + LLM) tells it to render. Each panel kind maps
 * to a render function that takes props and returns Arrow.js HTML.
 *
 * The backend sends: { layout: { panels: [...], emphasis, reasoning }, data: {...} }
 * This renderer: iterates panels, looks up renderer by kind, passes props.
 *
 * Adding a new panel type = adding one render function here. No logic changes.
 */

import { reactive, html } from "@arrow-js/core";
import type { MedicareResult, MedicareSource } from "./medicare-bridge.js";

// ---------------------------------------------------------------------------
// Types — what the backend sends
// ---------------------------------------------------------------------------

export interface LayoutIntent {
  panels: string[];
  emphasis: string;
  reasoning: string;
}

export interface PanelSpec {
  kind: string;
  props: Record<string, string>;
}

export interface ConversationTurn {
  role: "user" | "assistant";
  content: string;
  layout?: LayoutIntent;
}

// ---------------------------------------------------------------------------
// Reactive state — driven entirely by backend responses
// ---------------------------------------------------------------------------

const state = reactive({
  phase: "idle" as string,
  panels: [] as PanelSpec[],
  data: null as MedicareResult | null,
  comparisons: [] as MedicareResult[],
  layout: null as LayoutIntent | null,
  conversation: [] as ConversationTurn[],
  sessionId: "" as string,
  error: "" as string,
  isLoading: false as boolean,
  // Current form values (local state, not from backend)
  zip: "" as string,
  planType: "advantage" as string,
  filter: "" as string,
});

// ---------------------------------------------------------------------------
// State management (called by main.ts)
// ---------------------------------------------------------------------------

export function setPhase(phase: string): void {
  state.phase = phase;
  state.isLoading = ["searching", "fetching", "generating"].includes(phase);
}

export function setResult(data: MedicareResult, layout: LayoutIntent, sessionId: string): void {
  state.data = data;
  state.layout = layout;
  state.phase = "complete";
  state.isLoading = false;
  state.error = "";
  state.sessionId = sessionId;
  // Build panel specs from layout
  state.panels = (layout?.panels || ["summary", "source-list", "disclaimer"]).map(kind => ({
    kind,
    props: {},
  }));
}

export function setComparisons(results: MedicareResult[], layout: LayoutIntent): void {
  state.comparisons = results;
  state.layout = layout;
  state.phase = "complete";
  state.isLoading = false;
  state.panels = [{ kind: "comparison", props: {} }];
}

export function setNeedsInput(field: string, message: string): void {
  state.error = "";
  state.phase = "needs-input";
  addConversationTurn("assistant", message);
}

export function setError(msg: string): void {
  state.error = msg;
  state.isLoading = false;
}

export function addConversationTurn(role: "user" | "assistant", content: string, layout?: LayoutIntent): void {
  state.conversation = [...state.conversation, { role, content, layout }];
}

export function resetState(): void {
  state.phase = "idle";
  state.panels = [];
  state.data = null;
  state.comparisons = [];
  state.layout = null;
  state.conversation = [];
  state.error = "";
  state.isLoading = false;
}

// ---------------------------------------------------------------------------
// Panel registry — maps panel kind → render function
// Each renderer takes the full state and returns Arrow.js html
// ---------------------------------------------------------------------------

type PanelRenderer = () => ReturnType<typeof html>;

const PANEL_RENDERERS: Record<string, PanelRenderer> = {
  "header": renderHeader,
  "search-form": renderSearchForm_internal,
  "chat-input": renderChatInput_internal,
  "progress": renderProgress,
  "summary": renderSummary,
  "cost-table": renderCostTable,
  "plan-cards": renderPlanCards,
  "comparison": renderComparison,
  "source-list": renderSourceList,
  "disclaimer": renderDisclaimer,
  "detail": renderDetail,
  "chart": renderChart,
  "followup": renderFollowup,
  "filter-pills": renderFilterPills,
  "error": renderError,
};

// Store callbacks for wiring
let _onChat: ((msg: string) => void) | null = null;
let _onLookup: ((pt: string, zip: string, filter: string) => void) | null = null;
let _onCompare: ((zip: string) => void) | null = null;

// ---------------------------------------------------------------------------
// Panel renderers — each one is dumb, just renders props
// ---------------------------------------------------------------------------

function renderHeader(): ReturnType<typeof html> {
  return html`
    <div class="medicare-header">
      <h1>Medicare Plan Finder</h1>
      <p class="medicare-subtitle">
        ${() => state.layout?.emphasis || "Compare Medicare insurance plans and estimated costs in your area"}
      </p>
      <p class="medicare-disclaimer">
        Powered by web search — for official info visit
        <a href="https://www.medicare.gov/plan-compare" target="_blank">medicare.gov</a>
        or call <strong>1-800-MEDICARE</strong>
      </p>
    </div>
  `;
}

function renderSearchForm_internal(): ReturnType<typeof html> {
  const PLAN_TYPES = [
    { id: "advantage", label: "Medicare Advantage (Part C)" },
    { id: "part-d", label: "Part D (Prescription Drugs)" },
    { id: "supplement", label: "Medigap (Supplement)" },
    { id: "original", label: "Original Medicare (A & B)" },
    { id: "part-a", label: "Part A (Hospital)" },
    { id: "part-b", label: "Part B (Medical)" },
  ];

  const doLookup = () => {
    const zip = state.zip.trim();
    if (zip.length !== 5 || !/^\d{5}$/.test(zip)) {
      state.error = "Please enter a valid 5-digit zip code";
      return;
    }
    state.error = "";
    _onLookup?.(state.planType, zip, state.filter);
  };

  return html`
    <div class="medicare-form">
      <div class="form-row">
        <div class="form-group">
          <label>Zip Code</label>
          <input class="form-input zip-input" type="text" maxlength="5"
            placeholder="e.g. 33101"
            value="${() => state.zip}"
            @input="${(e: any) => { state.zip = e.target.value.replace(/\D/g, '').slice(0, 5); }}"
            @keydown="${(e: any) => { if (e.key === 'Enter') doLookup(); }}" />
        </div>
        <div class="form-group">
          <label>Plan Type</label>
          <select class="form-input plan-select"
            @change="${(e: any) => { state.planType = e.target.value; }}">
            ${PLAN_TYPES.map(pt => html`
              <option value="${pt.id}" selected="${() => state.planType === pt.id ? 'selected' : undefined}">
                ${pt.label}
              </option>
            `)}
          </select>
        </div>
      </div>
      <div class="form-row">
        <div class="form-group form-group-wide">
          <label>Specific coverage? <span class="optional">(optional)</span></label>
          <input class="form-input" type="text"
            placeholder="e.g. insulin, dental, vision..."
            value="${() => state.filter}"
            @input="${(e: any) => { state.filter = e.target.value; }}"
            @keydown="${(e: any) => { if (e.key === 'Enter') doLookup(); }}" />
        </div>
      </div>
      <div class="form-actions">
        <button class="btn btn-primary" @click="${doLookup}"
          disabled="${() => state.isLoading ? 'disabled' : undefined}">
          ${() => state.isLoading ? "Searching..." : "Find Plans"}
        </button>
        <button class="btn btn-secondary"
          @click="${() => { const z = state.zip.trim(); if (z.length === 5) _onCompare?.(z); }}"
          disabled="${() => state.isLoading ? 'disabled' : undefined}">
          Compare All Types
        </button>
      </div>
      <div class="form-error" style="${() => state.error ? '' : 'display:none'}">
        ${() => state.error}
      </div>
    </div>
  `;
}

function renderChatInput_internal(): ReturnType<typeof html> {
  const local = reactive({ value: "" });

  const doSend = () => {
    const msg = local.value.trim();
    if (!msg) return;
    local.value = "";
    _onChat?.(msg);
  };

  return html`
    <div class="chat-input-panel">
      <div class="chat-history" style="${() => state.conversation.length > 0 ? '' : 'display:none'}">
        ${() => state.conversation.map(turn => html`
          <div class="chat-turn chat-${turn.role}">
            <span class="chat-role">${turn.role === 'user' ? 'You' : 'Medicare Advisor'}</span>
            <div class="chat-content">${turn.content}</div>
          </div>
        `)}
      </div>
      <div class="chat-input-row">
        <input class="form-input chat-text-input" type="text"
          placeholder="${() => state.phase === 'idle'
            ? 'Ask a question: \"What Part D plans cover insulin in 33101?\"'
            : 'Ask a follow-up question...'}"
          value="${() => local.value}"
          @input="${(e: any) => { local.value = e.target.value; }}"
          @keydown="${(e: any) => { if (e.key === 'Enter') doSend(); }}"
          disabled="${() => state.isLoading ? 'disabled' : undefined}" />
        <button class="btn btn-primary chat-send-btn" @click="${doSend}"
          disabled="${() => state.isLoading ? 'disabled' : undefined}">
          ${() => state.isLoading ? '...' : 'Ask'}
        </button>
      </div>
    </div>
  `;
}

function renderProgress(): ReturnType<typeof html> {
  const stages = [
    { key: "searching", label: "Searching medicare.gov" },
    { key: "fetching", label: "Reading plan details" },
    { key: "generating", label: "Preparing summary" },
  ];

  return html`
    <div class="medicare-pipeline" style="${() => state.isLoading ? '' : 'display:none'}">
      ${stages.map(s => html`
        <div class="${() => {
          const order = ["searching", "fetching", "generating"];
          const cur = order.indexOf(state.phase);
          const idx = order.indexOf(s.key);
          return idx < cur ? "pipe-stage done" : idx === cur ? "pipe-stage active" : "pipe-stage";
        }}">
          <div class="pipe-dot"></div>
          <span>${s.label}</span>
        </div>
      `)}
    </div>
  `;
}

function renderSummary(): ReturnType<typeof html> {
  return html`
    <div class="result-summary" style="${() => state.data?.summary ? '' : 'display:none'}">
      ${() => renderMarkdown(state.data?.summary || '')}
    </div>
  `;
}

function renderCostTable(): ReturnType<typeof html> {
  // Same content as summary but styled as a table-focused view
  return html`
    <div class="cost-table-panel" style="${() => state.data?.summary ? '' : 'display:none'}">
      <h3>Cost Breakdown</h3>
      ${() => renderMarkdown(state.data?.summary || '')}
    </div>
  `;
}

function renderPlanCards(): ReturnType<typeof html> {
  return html`
    <div class="plan-cards-panel" style="${() => state.data?.summary ? '' : 'display:none'}">
      <h3>Plan Options</h3>
      ${() => renderMarkdown(state.data?.summary || '')}
    </div>
  `;
}

function renderComparison(): ReturnType<typeof html> {
  return html`
    <div class="medicare-comparison" style="${() => state.comparisons.length > 0 ? '' : 'display:none'}">
      <h2>Plan Comparison — Zip ${() => state.zip || state.data?.zip}</h2>
      ${() => state.comparisons.map((result: MedicareResult) => html`
        <div class="comparison-card">
          <h3>${result.planLabel}</h3>
          <div class="comparison-summary">
            ${renderMarkdown(result.summary || '')}
          </div>
          <div class="comparison-sources">
            ${(result.sources || []).slice(0, 3).map((src: MedicareSource) => html`
              <a href="${src.url}" target="_blank" class="comparison-source-link">
                ${src.title || src.url}
                ${src.isMedicareGov ? ' (official)' : ''}
              </a>
            `)}
          </div>
        </div>
      `)}
    </div>
  `;
}

function renderSourceList(): ReturnType<typeof html> {
  return html`
    <div class="result-sources" style="${() =>
      (state.data?.sources?.length || 0) > 0 ? '' : 'display:none'
    }">
      <h3>Sources</h3>
      ${() => (state.data?.sources || []).map((src: MedicareSource) => html`
        <div class="source-item ${src.isMedicareGov ? 'source-official' : ''}">
          <a href="${src.url}" target="_blank" class="source-link">
            ${src.title || src.url}
          </a>
          ${src.isMedicareGov ? html`<span class="official-badge">medicare.gov</span>` : ''}
          <p class="source-snippet-text">${src.snippet}</p>
        </div>
      `)}
    </div>
  `;
}

function renderDisclaimer(): ReturnType<typeof html> {
  return html`
    <div class="result-footer" style="${() => state.phase === 'complete' ? '' : 'display:none'}">
      <p class="result-disclaimer">
        Prices shown are estimates based on web search results. Actual costs depend on your
        specific situation. Visit
        <a href="https://www.medicare.gov/plan-compare" target="_blank">medicare.gov/plan-compare</a>
        or call <strong>1-800-MEDICARE</strong> (1-800-633-4227) for exact pricing.
      </p>
    </div>
  `;
}

function renderDetail(): ReturnType<typeof html> {
  return html`
    <div class="detail-panel" style="${() => state.data?.summary ? '' : 'display:none'}">
      <h3>${() => state.layout?.emphasis || 'Details'}</h3>
      ${() => renderMarkdown(state.data?.summary || '')}
    </div>
  `;
}

function renderChart(): ReturnType<typeof html> {
  // Simple text-based cost range visualization
  return html`
    <div class="chart-panel" style="${() => state.data?.summary ? '' : 'display:none'}">
      <h3>Cost Overview</h3>
      <p class="chart-note">Based on available data for zip ${() => state.data?.zip}</p>
      ${() => renderMarkdown(state.data?.summary || '')}
    </div>
  `;
}

function renderFollowup(): ReturnType<typeof html> {
  // Generate follow-up suggestions based on current data
  const suggestions = () => {
    if (!state.data) return [];
    const pt = state.data.planType || "";
    const base = [
      `What's the out-of-pocket maximum?`,
      `Compare with other plan types`,
      `What's covered under this plan?`,
      `Show me the cheapest options`,
    ];
    if (pt === "part-d") {
      return [`What drugs are covered?`, `What's the donut hole?`, ...base.slice(2)];
    }
    if (pt === "advantage") {
      return [`Does this include dental?`, ...base];
    }
    return base;
  };

  return html`
    <div class="followup-panel" style="${() => state.phase === 'complete' ? '' : 'display:none'}">
      <h4>Ask more about your plans</h4>
      <div class="followup-chips">
        ${() => suggestions().map(q => html`
          <button class="followup-chip" @click="${() => _onChat?.(q)}">${q}</button>
        `)}
      </div>
    </div>
  `;
}

function renderFilterPills(): ReturnType<typeof html> {
  return html`
    <div class="filter-pills" style="${() => state.data ? '' : 'display:none'}">
      ${() => state.data?.planType ? html`
        <span class="pill pill-plan">${state.data.planLabel || state.data.planType}</span>
      ` : ''}
      ${() => state.data?.zip ? html`
        <span class="pill pill-zip">Zip: ${state.data.zip}</span>
      ` : ''}
      ${() => state.data?.filter ? html`
        <span class="pill pill-filter">${state.data.filter}</span>
      ` : ''}
      ${() => state.data?.cached ? html`
        <span class="pill pill-cached">Cached</span>
      ` : ''}
    </div>
  `;
}

function renderError(): ReturnType<typeof html> {
  return html`
    <div class="form-error" style="${() => state.error ? '' : 'display:none'}">
      ${() => state.error}
    </div>
  `;
}

// Welcome panel (shown only on idle — not part of dynamic panel list)
function renderWelcome(): ReturnType<typeof html> {
  const PLAN_TYPES = [
    { id: "advantage", label: "Medicare Advantage (Part C)", desc: "Bundled coverage from private insurers" },
    { id: "part-d", label: "Part D (Prescription Drugs)", desc: "Prescription drug coverage" },
    { id: "supplement", label: "Medigap (Supplement)", desc: "Fills gaps in Original Medicare" },
    { id: "original", label: "Original Medicare (A & B)", desc: "Government-provided hospital + medical" },
  ];

  return html`
    <div class="medicare-welcome" style="${() => state.phase === 'idle' && !state.isLoading ? '' : 'display:none'}">
      <div class="welcome-cards">
        ${PLAN_TYPES.map(pt => html`
          <div class="welcome-card" @click="${() => { state.planType = pt.id; }}">
            <strong>${pt.label}</strong>
            <p>${pt.desc}</p>
          </div>
        `)}
      </div>
      <div class="welcome-info">
        <h3>How it works</h3>
        <ol>
          <li>Enter your zip code, or just ask a question in plain English</li>
          <li>We search medicare.gov and trusted sources for current pricing</li>
          <li>AI summarizes the results and decides how to present them</li>
          <li>Ask follow-up questions — the AI adapts the view on the fly</li>
        </ol>
        <p class="welcome-note">
          <strong>Generative UI:</strong> The layout you see is decided by AI + validated by
          Shen's type system. The frontend has zero domain knowledge — it renders
          whatever panels the backend says to show.
        </p>
      </div>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Markdown renderer (simple)
// ---------------------------------------------------------------------------

function renderMarkdown(text: string): string {
  if (!text) return "";
  return text
    .split("\n\n")
    .map((block) => {
      const trimmed = block.trim();
      if (!trimmed) return "";
      if (trimmed.startsWith("### ")) return `<h4>${esc(trimmed.slice(4))}</h4>`;
      if (trimmed.startsWith("## ")) return `<h3>${esc(trimmed.slice(3))}</h3>`;
      if (trimmed.startsWith("# ")) return `<h2>${esc(trimmed.slice(2))}</h2>`;
      if (trimmed.startsWith("**") && trimmed.endsWith("**"))
        return `<h4>${esc(trimmed.slice(2, -2))}</h4>`;
      if (/^[-*]\s/.test(trimmed) || /^\d+\.\s/.test(trimmed)) {
        const items = trimmed
          .split("\n")
          .map((l) => l.replace(/^[-*\d.]\s*/, "").trim())
          .filter(Boolean)
          .map((l) => `<li>${inlineMarkdown(l)}</li>`)
          .join("");
        return `<ul>${items}</ul>`;
      }
      return `<p>${inlineMarkdown(trimmed)}</p>`;
    })
    .join("");
}

function inlineMarkdown(text: string): string {
  return esc(text)
    .replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>")
    .replace(/\*(.+?)\*/g, "<em>$1</em>")
    .replace(/`(.+?)`/g, "<code>$1</code>");
}

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// ---------------------------------------------------------------------------
// Dynamic panel renderer — the core of generative UI
// ---------------------------------------------------------------------------

function renderDynamicPanels(): ReturnType<typeof html> {
  return html`
    <div class="dynamic-panels" style="${() => state.phase === 'complete' ? '' : 'display:none'}">
      ${() => {
        const panelKinds = state.layout?.panels || ["summary", "source-list", "disclaimer"];
        return panelKinds
          .filter((kind: string) => PANEL_RENDERERS[kind])
          .map((kind: string) => {
            const renderer = PANEL_RENDERERS[kind];
            return html`<div class="panel panel-${kind}">${renderer()}</div>`;
          });
      }}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Mount — compose the shell + dynamic panel area
// ---------------------------------------------------------------------------

export function mountMedicare(
  root: HTMLElement,
  onLookup: (planType: string, zip: string, filter: string) => void,
  onCompare: (zip: string) => void,
  onChat: (message: string) => void,
): void {
  // Store callbacks for panel renderers
  _onLookup = onLookup;
  _onCompare = onCompare;
  _onChat = onChat;

  const app = html`
    <div class="medicare-app">
      ${renderHeader()}
      ${renderSearchForm_internal()}
      ${renderChatInput_internal()}
      ${renderProgress()}
      ${renderWelcome()}
      ${renderDynamicPanels()}
    </div>
  `;

  app(root);
}
