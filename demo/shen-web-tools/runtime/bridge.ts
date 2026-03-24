/**
 * runtime/bridge.ts — Frontend API client for the CL backend.
 *
 * The Shen logic runs on SBCL (Common Lisp). This module is a thin
 * HTTP client that calls the CL backend's JSON API endpoints.
 *
 * All intelligence lives in Shen on the server. This is just fetch().
 *
 * Optional: pi-ai integration for client-side LLM streaming.
 * When pi-ai is loaded (via importmap or bundler), the `streamGenerate`
 * function streams tokens directly from the LLM to the browser,
 * bypassing the CL backend for the generation step only.
 * Install: npm install @mariozechner/pi-ai
 */

// ---------------------------------------------------------------------------
// Types (match the JSON shapes returned by the CL server)
// ---------------------------------------------------------------------------

export interface SearchHit {
  title: string;
  url: string;
  snippet: string;
}

export interface GroundedSource {
  pageUrl: string;
  title: string;
  url: string;
  snippet: string;
  grounded: boolean;
}

export interface ResearchResult {
  query: string;
  summary: string;
  sources: GroundedSource[];
  timestamp: number;
}

export interface PipelineState {
  phase: string;
  data: any;
}

// ---------------------------------------------------------------------------
// API client
// ---------------------------------------------------------------------------

const BASE = "";  // Same origin as the CL server

export async function research(query: string): Promise<ResearchResult> {
  const resp = await fetch(`${BASE}/api/research`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query }),
  });
  if (!resp.ok) throw new Error(`Research failed: ${resp.status}`);
  return resp.json();
}

export async function search(query: string, maxResults = 10): Promise<{ query: string; hits: SearchHit[]; timestamp: number }> {
  const resp = await fetch(`${BASE}/api/search`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, maxResults }),
  });
  if (!resp.ok) throw new Error(`Search failed: ${resp.status}`);
  return resp.json();
}

export async function fetchUrl(url: string): Promise<{ url: string; content: string; timestamp: number }> {
  const resp = await fetch(`${BASE}/api/fetch`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
  if (!resp.ok) throw new Error(`Fetch failed: ${resp.status}`);
  return resp.json();
}

export async function generate(system: string, user: string): Promise<{ text: string; timestamp: number }> {
  const resp = await fetch(`${BASE}/api/generate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ system, user }),
  });
  if (!resp.ok) throw new Error(`Generate failed: ${resp.status}`);
  return resp.json();
}

export async function getPipelineState(): Promise<PipelineState> {
  const resp = await fetch(`${BASE}/api/state`);
  if (!resp.ok) throw new Error(`State fetch failed: ${resp.status}`);
  return resp.json();
}

// ---------------------------------------------------------------------------
// Optional: pi-ai client-side streaming
// ---------------------------------------------------------------------------
// Uses @mariozechner/pi-ai for direct browser→LLM streaming.
// This is optional — the CL backend handles AI generation by default.
// To enable: load pi-ai via importmap and call streamGenerate() instead
// of generate().
//
// Example importmap entry:
//   "@mariozechner/pi-ai": "https://esm.sh/@mariozechner/pi-ai"

export interface StreamCallbacks {
  onToken: (text: string) => void;
  onThinking?: (text: string) => void;
  onDone: (fullText: string) => void;
  onError?: (error: Error) => void;
}

/**
 * Stream AI generation directly from browser using pi-ai.
 * Falls back to CL backend's /api/generate if pi-ai is not available.
 *
 * @param system - System prompt
 * @param user - User message
 * @param callbacks - Streaming callbacks
 * @param provider - pi-ai provider name (default: "anthropic")
 * @param model - Model ID (default: "claude-sonnet-4-6")
 */
export async function streamGenerate(
  system: string,
  user: string,
  callbacks: StreamCallbacks,
  provider = "anthropic",
  model = "claude-sonnet-4-6",
): Promise<void> {
  // Try to load pi-ai dynamically (optional dependency)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let piAi: any;
  try {
    // Dynamic import — only resolves if pi-ai is installed
    const piAiModule = "@mariozechner/pi-ai";
    piAi = await import(piAiModule);
  } catch {
    // pi-ai not available — fall back to CL backend
    try {
      const result = await generate(system, user);
      callbacks.onDone(result.text);
    } catch (e) {
      callbacks.onError?.(e instanceof Error ? e : new Error(String(e)));
    }
    return;
  }

  // pi-ai is available — stream directly
  try {
    const piModel = piAi.getModel(provider, model);
    const context = {
      systemPrompt: system,
      messages: [
        { role: "user" as const, content: [{ type: "text" as const, text: user }], timestamp: Date.now() },
      ],
    };

    let fullText = "";
    const response = piAi.stream(piModel, context);

    for await (const event of response) {
      switch (event.type) {
        case "text_delta":
          fullText += event.text;
          callbacks.onToken(event.text);
          break;
        case "thinking_delta":
          callbacks.onThinking?.(event.text);
          break;
        case "error":
          callbacks.onError?.(new Error(event.error));
          return;
      }
    }
    callbacks.onDone(fullText);
  } catch (e) {
    callbacks.onError?.(e instanceof Error ? e : new Error(String(e)));
  }
}
