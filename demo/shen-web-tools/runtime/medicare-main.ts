/**
 * runtime/medicare-main.ts — Entry point for Medicare Plan Finder
 *
 * Wires three interaction modes:
 *   1. Form-based: user fills in zip + plan type → structured lookup
 *   2. Conversational: user types natural language → LLM interprets → generative UI
 *   3. Follow-up: user asks follow-up → LLM generates new layout on the fly
 *
 * Pipeline state updates arrive via SSE (server-sent events), not polling.
 * The LLM generates follow-up suggestions as part of the layout intent.
 */

import {
  medicareChat,
  connectSSE,
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
let closeSSE: (() => void) | null = null;

// ---------------------------------------------------------------------------
// SSE streaming — replaces polling
// ---------------------------------------------------------------------------
// Open an SSE connection before firing the chat request.
// The backend pushes phase transitions as they happen:
//   searching → fetching → generating → complete
// This gives the user instant feedback with zero polling overhead.

function startStreaming(): void {
  // Close any existing connection
  stopStreaming();

  closeSSE = connectSSE({
    onPhase(phase: string) {
      // Update UI immediately on each phase transition
      if (["searching", "fetching", "generating"].includes(phase)) {
        setPhase(phase);
      }
    },
    onResult(_data: any) {
      // Result arrives via the chat response, not SSE.
      // SSE result is a bonus — we could use it for cache-hit fast path.
    },
    onError(msg: string) {
      // SSE errors are non-fatal — the chat response is the source of truth
      console.warn("SSE error:", msg);
    },
    onDone() {
      closeSSE = null;
    },
  });
}

function stopStreaming(): void {
  if (closeSSE) {
    closeSSE();
    closeSSE = null;
  }
}

// ---------------------------------------------------------------------------
// Handle chat response — route to appropriate UI state
// ---------------------------------------------------------------------------

function handleChatResponse(response: ChatResponse): void {
  stopStreaming();

  if (response.session) {
    sessionId = response.session;
  }

  // Default layout with empty followups
  const defaultLayout = {
    panels: ["summary", "source-list", "disclaimer"],
    emphasis: "",
    reasoning: "",
    followups: [],
  };

  switch (response.type) {
    case "result":
      if (response.data?.comparisons) {
        setComparisons(
          response.data.comparisons,
          response.layout || { ...defaultLayout, panels: ["comparison", "disclaimer"] },
        );
      } else if (response.data) {
        setResult(
          response.data,
          response.layout || defaultLayout,
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
  startStreaming(); // SSE for real-time phase updates

  try {
    const response = await medicareChat(message, sessionId);
    handleChatResponse(response);

    if (response.data?.summary) {
      addConversationTurn(
        "assistant",
        response.data.summary.slice(0, 200) + (response.data.summary.length > 200 ? "..." : ""),
        response.layout,
      );
    }
  } catch (e) {
    stopStreaming();
    setError(e instanceof Error ? e.message : String(e));
  }
}

// ---------------------------------------------------------------------------
// Structured lookup handler — form-based, but uses chat for layout intent
// ---------------------------------------------------------------------------

async function onLookup(planType: string, zip: string, filter: string): Promise<void> {
  setPhase("searching");
  startStreaming();

  try {
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
    stopStreaming();
    setError(e instanceof Error ? e.message : String(e));
  }
}

// ---------------------------------------------------------------------------
// Compare handler
// ---------------------------------------------------------------------------

async function onCompare(zip: string): Promise<void> {
  setPhase("searching");
  startStreaming();

  try {
    addConversationTurn("user", `Compare all plan types for zip ${zip}`);

    const response = await medicareChat(
      `Compare Medicare Advantage, Part D, and Medigap plans in ${zip}`,
      sessionId,
      zip,
    );
    handleChatResponse(response);
  } catch (e) {
    stopStreaming();
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
