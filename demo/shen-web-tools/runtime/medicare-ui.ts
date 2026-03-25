/**
 * runtime/medicare-ui.ts — Arrow.js UI for Medicare plan lookup
 *
 * Consumer-facing interface for searching Medicare insurance plans
 * by zip code and plan type. Displays pricing info from medicare.gov
 * via the Shen/CL backend with caching.
 */

import { reactive, html } from "@arrow-js/core";
import type { MedicareResult, MedicareSource } from "./medicare-bridge.js";

// ---------------------------------------------------------------------------
// Reactive State
// ---------------------------------------------------------------------------

const state = reactive({
  phase: "idle" as string,
  zip: "" as string,
  planType: "advantage" as string,
  filter: "" as string,
  result: null as MedicareResult | null,
  comparing: false as boolean,
  comparisons: [] as MedicareResult[],
  error: "" as string,
  isLoading: false as boolean,
});

// ---------------------------------------------------------------------------
// Plan type options
// ---------------------------------------------------------------------------

const PLAN_TYPES = [
  { id: "advantage", label: "Medicare Advantage (Part C)", desc: "Bundled coverage from private insurers" },
  { id: "part-d", label: "Part D (Prescription Drugs)", desc: "Prescription drug coverage" },
  { id: "supplement", label: "Medigap (Supplement)", desc: "Fills gaps in Original Medicare" },
  { id: "original", label: "Original Medicare (A & B)", desc: "Government-provided hospital + medical" },
  { id: "part-a", label: "Part A (Hospital)", desc: "Hospital and inpatient care" },
  { id: "part-b", label: "Part B (Medical)", desc: "Doctor visits and outpatient care" },
];

// ---------------------------------------------------------------------------
// State management (called by main)
// ---------------------------------------------------------------------------

export function setMedicarePhase(phase: string): void {
  state.phase = phase;
  state.isLoading = ["searching", "fetching", "generating"].includes(phase);
}

export function setMedicareResult(result: MedicareResult): void {
  state.result = result;
  state.phase = "complete";
  state.isLoading = false;
  state.error = "";
}

export function setMedicareComparisons(results: MedicareResult[]): void {
  state.comparisons = results;
  state.comparing = true;
  state.phase = "complete";
  state.isLoading = false;
}

export function setMedicareError(msg: string): void {
  state.error = msg;
  state.isLoading = false;
  state.phase = "idle";
}

// ---------------------------------------------------------------------------
// Components
// ---------------------------------------------------------------------------

function header() {
  return html`
    <div class="medicare-header">
      <h1>Medicare Plan Finder</h1>
      <p class="medicare-subtitle">
        Compare Medicare insurance plans and estimated costs in your area
      </p>
      <p class="medicare-disclaimer">
        Powered by web search of medicare.gov — for official information, visit
        <a href="https://www.medicare.gov/plan-compare" target="_blank">medicare.gov/plan-compare</a>
        or call <strong>1-800-MEDICARE</strong> (1-800-633-4227)
      </p>
    </div>
  `;
}

function searchForm(
  onLookup: (planType: string, zip: string, filter: string) => void,
  onCompare: (zip: string) => void,
) {
  const doLookup = () => {
    const zip = state.zip.trim();
    if (zip.length !== 5 || !/^\d{5}$/.test(zip)) {
      state.error = "Please enter a valid 5-digit zip code";
      return;
    }
    state.error = "";
    onLookup(state.planType, zip, state.filter);
  };

  const doCompare = () => {
    const zip = state.zip.trim();
    if (zip.length !== 5 || !/^\d{5}$/.test(zip)) {
      state.error = "Please enter a valid 5-digit zip code";
      return;
    }
    state.error = "";
    onCompare(zip);
  };

  return html`
    <div class="medicare-form">
      <div class="form-row">
        <div class="form-group">
          <label for="zip-input">Zip Code</label>
          <input
            id="zip-input"
            class="form-input zip-input"
            type="text"
            maxlength="5"
            placeholder="e.g. 33101"
            value="${() => state.zip}"
            @input="${(e: any) => { state.zip = e.target.value.replace(/\D/g, '').slice(0, 5); }}"
            @keydown="${(e: any) => { if (e.key === 'Enter') doLookup(); }}"
          />
        </div>
        <div class="form-group">
          <label for="plan-select">Plan Type</label>
          <select
            id="plan-select"
            class="form-input plan-select"
            @change="${(e: any) => { state.planType = e.target.value; }}"
          >
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
          <label for="filter-input">Specific coverage? <span class="optional">(optional)</span></label>
          <input
            id="filter-input"
            class="form-input"
            type="text"
            placeholder="e.g. insulin, dental, vision, hearing aids..."
            value="${() => state.filter}"
            @input="${(e: any) => { state.filter = e.target.value; }}"
            @keydown="${(e: any) => { if (e.key === 'Enter') doLookup(); }}"
          />
        </div>
      </div>

      <div class="form-actions">
        <button class="btn btn-primary" @click="${doLookup}"
          disabled="${() => state.isLoading ? 'disabled' : undefined}">
          ${() => state.isLoading ? 'Searching...' : 'Find Plans'}
        </button>
        <button class="btn btn-secondary" @click="${doCompare}"
          disabled="${() => state.isLoading ? 'disabled' : undefined}">
          Compare All Plan Types
        </button>
      </div>

      <div class="form-error" style="${() => state.error ? '' : 'display:none'}">
        ${() => state.error}
      </div>
    </div>
  `;
}

function pipelineStatus() {
  const stages = [
    { key: "searching", label: "Searching medicare.gov" },
    { key: "fetching", label: "Reading plan details" },
    { key: "generating", label: "Preparing your summary" },
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

function resultPanel() {
  return html`
    <div class="medicare-result" style="${() => state.phase === 'complete' && state.result && !state.comparing ? '' : 'display:none'}">
      <div class="result-header">
        <h2>${() => state.result?.planLabel || 'Medicare Plans'}</h2>
        <div class="result-meta">
          <span class="result-zip">Zip: ${() => state.result?.zip}</span>
          ${() => state.result?.filter ? html`<span class="result-filter">Filter: ${state.result.filter}</span>` : ''}
          ${() => state.result?.cached ? html`<span class="result-cached" title="From cache">Cached</span>` : ''}
        </div>
      </div>

      <div class="result-summary">
        ${() => renderMarkdown(state.result?.summary || '')}
      </div>

      <div class="result-sources">
        <h3>Sources</h3>
        ${() => (state.result?.sources || []).map((src: MedicareSource) => html`
          <div class="source-item ${src.isMedicareGov ? 'source-official' : ''}">
            <a href="${src.url}" target="_blank" class="source-link">
              ${src.title || src.url}
            </a>
            ${src.isMedicareGov ? html`<span class="official-badge">medicare.gov</span>` : ''}
            <p class="source-snippet-text">${src.snippet}</p>
          </div>
        `)}
      </div>

      <div class="result-footer">
        <p class="result-disclaimer">
          Prices shown are estimates based on web search results. Actual costs depend on your
          specific situation, income, and chosen plan. For official pricing, visit
          <a href="https://www.medicare.gov/plan-compare" target="_blank">medicare.gov/plan-compare</a>.
        </p>
      </div>
    </div>
  `;
}

function comparisonPanel() {
  return html`
    <div class="medicare-comparison" style="${() => state.phase === 'complete' && state.comparing ? '' : 'display:none'}">
      <h2>Plan Comparison — Zip ${() => state.zip}</h2>
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

function welcomePanel() {
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
          <li>Enter your zip code and select a plan type</li>
          <li>We search medicare.gov and trusted sources for current pricing</li>
          <li>AI summarizes the results in plain language</li>
          <li>Results are cached so repeat lookups are instant</li>
        </ol>
        <p class="welcome-note">
          <strong>Note:</strong> This tool searches publicly available information from medicare.gov
          and presents it using AI. It is not affiliated with Medicare or CMS.
          Always verify information at <a href="https://www.medicare.gov" target="_blank">medicare.gov</a>.
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
      // Lists
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
// Mount
// ---------------------------------------------------------------------------

export function mountMedicare(
  root: HTMLElement,
  onLookup: (planType: string, zip: string, filter: string) => void,
  onCompare: (zip: string) => void,
): void {
  const app = html`
    <div class="medicare-app">
      ${header()}
      ${searchForm(onLookup, onCompare)}
      ${pipelineStatus()}
      ${welcomePanel()}
      ${resultPanel()}
      ${comparisonPanel()}
    </div>
  `;

  app(root);
}
