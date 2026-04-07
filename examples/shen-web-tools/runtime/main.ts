/**
 * runtime/main.ts — Frontend entry point.
 *
 * Wires together:
 *   1. The CL backend API client (runtime/bridge.ts)
 *   2. The Arrow.js UI renderer (runtime/ui.ts)
 *
 * Flow:
 *   User types query → Arrow captures event → POST /api/research
 *   → CL backend runs Shen pipeline (search → fetch → ground → generate)
 *   → JSON response → Arrow re-renders reactively
 *
 * The frontend polls /api/state during long operations to show
 * pipeline progress (searching → fetching → generating → complete).
 */

import { research, getPipelineState, type ResearchResult } from "./bridge.js";
import { mount, onShenRender } from "./ui.js";

// ---------------------------------------------------------------------------
// Pipeline state polling
// ---------------------------------------------------------------------------

let polling = false;

async function pollPipelineState(): Promise<void> {
  polling = true;
  while (polling) {
    try {
      const state = await getPipelineState();
      onShenRender([state.phase, null, state.data]);
      if (state.phase === "complete" || state.phase === "idle") {
        polling = false;
        break;
      }
    } catch {
      // Server not responding, stop polling
      polling = false;
      break;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
}

// ---------------------------------------------------------------------------
// Search handler
// ---------------------------------------------------------------------------

async function onSearch(query: string): Promise<void> {
  // Start polling for pipeline state updates
  onShenRender(["searching", null, { query }]);
  const pollPromise = pollPipelineState();

  try {
    // POST to CL backend — Shen orchestrates the full pipeline
    const result: ResearchResult = await research(query);

    // Stop polling and render final state
    polling = false;
    await pollPromise;

    // Convert to the format the UI expects
    const sources = result.sources.map((s) => [
      [s.pageUrl, "(content on server)", result.timestamp],
      [s.title, s.url, s.snippet],
    ]);

    onShenRender(["complete", null, [
      result.query,
      sources,
      [null, result.summary, result.timestamp],
    ]]);
  } catch (e) {
    polling = false;
    console.error("Research pipeline error:", e);
    onShenRender(["idle", null, null]);
  }
}

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------

function main(): void {
  const rootEl = document.getElementById("app");
  if (!rootEl) throw new Error("No #app element found");

  mount(rootEl, onSearch);
  onShenRender(["idle", null, null]);

  console.log("Shen Web Tools ready — CL backend at same origin, Arrow.js frontend");
}

if (typeof document !== "undefined") {
  document.addEventListener("DOMContentLoaded", main);
}
