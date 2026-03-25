/**
 * runtime/medicare-main.ts — Entry point for Medicare Plan Finder
 *
 * Wires three interaction modes:
 *   1. Form-based: user fills in zip + plan type → structured lookup
 *   2. Conversational: user types natural language → LLM interprets → generative UI
 *   3. Follow-up: user asks follow-up → LLM generates new layout on the fly
 *
 * The key insight: the LLM is in the loop TWICE:
 *   a) Interpreting the user's intent
 *   b) Deciding what UI panels to show
 * Shen validates the layout against grounded data (backpressure).
 * Arrow.js renders whatever panels Shen approves.
 */

import {
  medicareChat,
  medicareLookup,
  medicareCompare,
} from "./medicare-bridge.js";
import type { ChatResponse } from "./medicare-bridge.js";
import {
  mountMedicare,
  setPhase,
  setResult,
  setComparisons,
  setNeedsInput,
  setError,
  addConversationTurn,
} from "./medicare-ui.js";

// ---------------------------------------------------------------------------
// Session state
// ---------------------------------------------------------------------------

let sessionId = `s-${Date.now()}`;
let pollTimer: ReturnType<typeof setInterval> | null = null;

// ---------------------------------------------------------------------------
// Pipeline state polling
// ---------------------------------------------------------------------------

function startPolling(): void {
  if (pollTimer) return;
  pollTimer = setInterval(async () => {
    try {
      const resp = await fetch("/api/state");
      if (!resp.ok) return;
      const data = await resp.json();
      const phase = data.phase || "idle";
      if (["searching", "fetching", "generating"].includes(phase)) {
        setPhase(phase);
      }
      if (phase === "complete" || phase === "idle") {
        stopPolling();
      }
    } catch {
      // ignore
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
// Handle chat response — route to appropriate UI state
// ---------------------------------------------------------------------------

function handleChatResponse(response: ChatResponse): void {
  stopPolling();

  if (response.session) {
    sessionId = response.session;
  }

  switch (response.type) {
    case "result":
      if (response.data?.comparisons) {
        // Comparison result
        setComparisons(
          response.data.comparisons,
          response.layout || { panels: ["comparison", "disclaimer"], emphasis: "", reasoning: "" },
        );
      } else if (response.data) {
        // Single result with layout
        setResult(
          response.data,
          response.layout || { panels: ["summary", "source-list", "disclaimer"], emphasis: "", reasoning: "" },
          sessionId,
        );
      }
      break;

    case "needs-input":
      setNeedsInput(response.field || "zip", response.message || "Please provide more information.");
      break;

    case "error":
      setError(response.error || "Something went wrong");
      break;

    default:
      setError("Unexpected response type");
  }
}

// ---------------------------------------------------------------------------
// Chat handler — natural language → LLM interprets → generative UI
// ---------------------------------------------------------------------------

async function onChat(message: string): Promise<void> {
  addConversationTurn("user", message);
  setPhase("searching");
  startPolling();

  try {
    const response = await medicareChat(message, sessionId);
    handleChatResponse(response);

    // Add assistant response to conversation
    if (response.data?.summary) {
      addConversationTurn(
        "assistant",
        response.data.summary.slice(0, 200) + (response.data.summary.length > 200 ? "..." : ""),
        response.layout,
      );
    }
  } catch (e) {
    stopPolling();
    setError(e instanceof Error ? e.message : String(e));
  }
}

// ---------------------------------------------------------------------------
// Structured lookup handler — form-based
// ---------------------------------------------------------------------------

async function onLookup(planType: string, zip: string, filter: string): Promise<void> {
  setPhase("searching");
  startPolling();

  try {
    // Use chat endpoint so we get layout intent
    const message = filter
      ? `Show me ${planType} plans covering ${filter} in ${zip}`
      : `Show me ${planType} plans in ${zip}`;

    addConversationTurn("user", message);

    const response = await medicareChat(message, sessionId, zip, planType);
    handleChatResponse(response);

    if (response.data?.summary) {
      addConversationTurn("assistant",
        response.data.summary.slice(0, 200) + "...",
        response.layout,
      );
    }
  } catch (e) {
    stopPolling();
    setError(e instanceof Error ? e.message : String(e));
  }
}

// ---------------------------------------------------------------------------
// Compare handler
// ---------------------------------------------------------------------------

async function onCompare(zip: string): Promise<void> {
  setPhase("searching");
  startPolling();

  try {
    addConversationTurn("user", `Compare all plan types for zip ${zip}`);

    const response = await medicareChat(
      `Compare Medicare Advantage, Part D, and Medigap plans in ${zip}`,
      sessionId,
      zip,
    );
    handleChatResponse(response);
  } catch (e) {
    stopPolling();
    setError(e instanceof Error ? e.message : String(e));
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
  mountMedicare(root, onLookup, onCompare, onChat);
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
