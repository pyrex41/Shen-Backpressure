/**
 * runtime/medicare-main.ts — Entry point for Medicare Plan Finder
 *
 * Wires the Medicare bridge (API client) to the Medicare UI (Arrow.js).
 * Polls pipeline state during lookups for progress updates.
 */

import { medicareLookup, medicareCompare } from "./medicare-bridge.js";
import {
  mountMedicare,
  setMedicarePhase,
  setMedicareResult,
  setMedicareComparisons,
  setMedicareError,
} from "./medicare-ui.js";

// ---------------------------------------------------------------------------
// Pipeline state polling
// ---------------------------------------------------------------------------

let pollTimer: ReturnType<typeof setInterval> | null = null;

async function pollPipelineState(): Promise<void> {
  if (pollTimer) return; // already polling
  pollTimer = setInterval(async () => {
    try {
      const resp = await fetch("/api/state");
      if (!resp.ok) return;
      const data = await resp.json();
      const phase = data.phase || "idle";

      if (["searching", "fetching", "generating"].includes(phase)) {
        setMedicarePhase(phase);
      }

      if (phase === "complete" || phase === "idle") {
        stopPolling();
      }
    } catch {
      // ignore polling errors
    }
  }, 600);
}

function stopPolling(): void {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

// ---------------------------------------------------------------------------
// Lookup handler
// ---------------------------------------------------------------------------

async function onLookup(planType: string, zip: string, filter: string): Promise<void> {
  setMedicarePhase("searching");
  pollPipelineState();

  try {
    const result = await medicareLookup(planType, zip, filter);
    stopPolling();
    setMedicareResult(result);
  } catch (e) {
    stopPolling();
    setMedicareError(e instanceof Error ? e.message : String(e));
  }
}

// ---------------------------------------------------------------------------
// Compare handler
// ---------------------------------------------------------------------------

async function onCompare(zip: string): Promise<void> {
  setMedicarePhase("searching");
  pollPipelineState();

  try {
    const result = await medicareCompare(zip, ["advantage", "part-d", "supplement"]);
    stopPolling();
    setMedicareComparisons(result.comparisons);
  } catch (e) {
    stopPolling();
    setMedicareError(e instanceof Error ? e.message : String(e));
  }
}

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------

function init(): void {
  const root = document.getElementById("app");
  if (!root) {
    console.error("No #app element found");
    return;
  }
  mountMedicare(root, onLookup, onCompare);
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
