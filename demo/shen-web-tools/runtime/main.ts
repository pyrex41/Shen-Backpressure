/**
 * runtime/main.ts — Application entry point.
 *
 * Wires together:
 *   1. The Shen engine (loads src/*.shen files)
 *   2. The TypeScript bridge (provides web I/O)
 *   3. The Arrow UI renderer (reactive DOM)
 *
 * Flow:
 *   User types query → Arrow captures event → Shen engine evaluates
 *   on-search-submit → Shen calls bridge.webSearch, bridge.webFetch,
 *   bridge.aiGenerate → Shen calls bridge.render with UI panel tree
 *   → Arrow re-renders reactively
 */

import { configure, setRenderCallback } from "./bridge.js";
import { createEngine, ShenEngine } from "./shen-engine.js";
import { mount, onShenRender } from "./ui.js";

// ---------------------------------------------------------------------------
// Shen source files (loaded at startup)
// ---------------------------------------------------------------------------

const SHEN_SOURCES = [
  "/src/web-tools.shen",
  "/src/ai-gen.shen",
  "/src/ui-resolve.shen",
  "/src/app.shen",
];

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------

async function loadShenSource(path: string): Promise<string> {
  const resp = await fetch(path);
  if (!resp.ok) throw new Error(`Failed to load ${path}: ${resp.status}`);
  return resp.text();
}

async function main(): Promise<void> {
  const rootEl = document.getElementById("app");
  if (!rootEl) throw new Error("No #app element found");

  // Show loading state
  rootEl.innerHTML = '<div class="boot">Loading Shen engine...</div>';

  // Configure bridge
  // Check URL params for provider config
  const params = new URLSearchParams(window.location.search);
  configure({
    searchProvider: (params.get("search") as any) || "mock",
    fetchProvider: (params.get("fetch") as any) || "mock",
    aiProvider: (params.get("ai") as any) || "mock",
    anthropicApiKey: params.get("key") || undefined,
    anthropicModel: params.get("model") || undefined,
  });

  // Wire up render callback: Shen → Arrow UI
  setRenderCallback(onShenRender);

  // Load Shen sources
  const sources: string[] = [];
  for (const path of SHEN_SOURCES) {
    try {
      sources.push(await loadShenSource(path));
    } catch (e) {
      console.warn(`Could not load ${path}, using inline fallback`);
    }
  }

  // Create Shen engine
  let engine: ShenEngine;
  try {
    engine = await createEngine(sources);
  } catch (e) {
    console.error("Failed to create Shen engine:", e);
    rootEl.innerHTML = `<div class="error">Failed to initialize Shen engine: ${e}</div>`;
    return;
  }

  // Mount Arrow UI
  mount(rootEl, async (query: string) => {
    try {
      await engine.call("on-search-submit", query);
    } catch (e) {
      console.error("Shen pipeline error:", e);
      rootEl.querySelector(".loading")?.remove();
      const errEl = document.createElement("div");
      errEl.className = "error";
      errEl.textContent = `Pipeline error: ${e}`;
      rootEl.appendChild(errEl);
    }
  });

  // Initialize Shen app state
  try {
    await engine.call("init-app");
  } catch (e) {
    console.warn("init-app not available, starting in idle state");
  }

  console.log("Shen Web Tools ready — all logic runs in Shen");
}

// ---------------------------------------------------------------------------
// Start
// ---------------------------------------------------------------------------

if (typeof document !== "undefined") {
  document.addEventListener("DOMContentLoaded", main);
}
