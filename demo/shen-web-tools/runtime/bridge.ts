/**
 * runtime/bridge.ts — TypeScript I/O bridge for the Shen web tools engine.
 *
 * This module provides the actual web capabilities that Shen's logic layer
 * calls into via `js.call`. Shen decides WHAT to do; this bridge does the I/O.
 *
 * The bridge implements:
 *   - webSearch: search the web via provider tools or fallback API
 *   - webFetch: fetch a URL and extract text content
 *   - aiGenerate: send a prompt to an AI model and return the response
 *   - render: push UI state to the Arrow.js renderer
 *
 * All functions return Shen-compatible data structures (nested arrays).
 */

// ---------------------------------------------------------------------------
// Types (mirrors the Shen spec for documentation; actual enforcement is
// in the generated guard types from shengen-ts)
// ---------------------------------------------------------------------------

type ShenList<T> = T[];
type SearchHit = [string, string, string];          // [title, url, snippet]
type SearchQuery = [string, number];                 // [queryText, maxResults]
type SearchResult = [SearchQuery, ShenList<SearchHit>, number]; // [query, hits, ts]
type FetchedPage = [string, string, number];         // [url, content, ts]
type AiPrompt = [string, string];                    // [system, user]
type AiResponse = [AiPrompt, string, number];        // [prompt, text, ts]
type GroundedSource = [FetchedPage, SearchHit];
type UiPanel = [string, string, ShenList<string>];   // [id, kind, children]
type RenderState = [string, UiPanel, unknown];        // [phase, panel, data]

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

interface BridgeConfig {
  searchProvider: "websearch" | "mock";
  fetchProvider: "webfetch" | "mock";
  aiProvider: "anthropic" | "mock";
  anthropicApiKey?: string;
  anthropicModel?: string;
  onRender?: (state: RenderState) => void;
}

const DEFAULT_CONFIG: BridgeConfig = {
  searchProvider: "mock",
  fetchProvider: "mock",
  aiProvider: "mock",
};

let config: BridgeConfig = { ...DEFAULT_CONFIG };

export function configure(cfg: Partial<BridgeConfig>): void {
  config = { ...config, ...cfg };
}

// ---------------------------------------------------------------------------
// Web Search
// ---------------------------------------------------------------------------

async function webSearchReal(query: SearchQuery): Promise<ShenList<SearchHit>> {
  const [text, maxResults] = query;
  // Uses the harness WebSearch tool if available, otherwise falls back
  if (typeof (globalThis as any).__webSearch === "function") {
    const raw = await (globalThis as any).__webSearch(text, maxResults);
    return raw.map((r: any) => [r.title, r.url, r.snippet || r.description || ""]);
  }
  // Fallback: use a simple fetch to a search API endpoint
  const resp = await fetch(`/api/search?q=${encodeURIComponent(text)}&n=${maxResults}`);
  const data = await resp.json();
  return data.results.map((r: any) => [r.title, r.url, r.snippet]);
}

function webSearchMock(query: SearchQuery): ShenList<SearchHit> {
  const [text, maxResults] = query;
  const hits: SearchHit[] = [];
  for (let i = 1; i <= Math.min(maxResults, 5); i++) {
    hits.push([
      `${text} - Result ${i}`,
      `https://example.com/${encodeURIComponent(text)}/${i}`,
      `This is a snippet about ${text} from source ${i}. It contains relevant information about the topic.`,
    ]);
  }
  return hits;
}

export async function webSearch(query: SearchQuery): Promise<ShenList<SearchHit>> {
  if (config.searchProvider === "mock") return webSearchMock(query);
  return webSearchReal(query);
}

// ---------------------------------------------------------------------------
// Web Fetch
// ---------------------------------------------------------------------------

async function webFetchReal(url: string): Promise<FetchedPage> {
  // Uses the harness WebFetch tool if available
  if (typeof (globalThis as any).__webFetch === "function") {
    const raw = await (globalThis as any).__webFetch(url);
    return [url, typeof raw === "string" ? raw : raw.text || raw.content || "", Date.now()];
  }
  const resp = await fetch(`/api/fetch?url=${encodeURIComponent(url)}`);
  const data = await resp.json();
  return [url, data.content, Date.now()];
}

function webFetchMock(url: string): FetchedPage {
  return [
    url,
    `Mock content fetched from ${url}. This simulates the text content ` +
    `that would be extracted from the web page. In production, the bridge ` +
    `calls the harness WebFetch tool or a proxy endpoint to retrieve and ` +
    `parse the actual page content.`,
    Date.now(),
  ];
}

export async function webFetch(url: string): Promise<FetchedPage> {
  if (config.fetchProvider === "mock") return webFetchMock(url);
  return webFetchReal(url);
}

// ---------------------------------------------------------------------------
// AI Generation
// ---------------------------------------------------------------------------

async function aiGenerateReal(prompt: AiPrompt): Promise<AiResponse> {
  const [system, user] = prompt;

  // Use Anthropic API if configured
  if (config.aiProvider === "anthropic" && config.anthropicApiKey) {
    const resp = await fetch("https://api.anthropic.com/v1/messages", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "x-api-key": config.anthropicApiKey,
        "anthropic-version": "2023-06-01",
      },
      body: JSON.stringify({
        model: config.anthropicModel || "claude-sonnet-4-6",
        max_tokens: 1024,
        system,
        messages: [{ role: "user", content: user }],
      }),
    });
    const data = await resp.json();
    const text = data.content?.[0]?.text || "";
    return [prompt, text, Date.now()];
  }

  // Fallback: use a local endpoint
  if (typeof (globalThis as any).__aiGenerate === "function") {
    const text = await (globalThis as any).__aiGenerate(system, user);
    return [prompt, text, Date.now()];
  }

  const resp = await fetch("/api/generate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ system, user }),
  });
  const data = await resp.json();
  return [prompt, data.text, Date.now()];
}

function aiGenerateMock(prompt: AiPrompt): AiResponse {
  const [system, user] = prompt;
  const mockText =
    `## Research Summary\n\n` +
    `Based on the provided sources, here is a comprehensive overview:\n\n` +
    `**Key Findings:**\n` +
    `1. The topic has been extensively covered across multiple sources\n` +
    `2. There is general consensus on the core concepts\n` +
    `3. Several sources provide unique perspectives worth exploring\n\n` +
    `**Details:**\n` +
    `The research covers the query "${user.slice(0, 80)}..." ` +
    `drawing from multiple web sources to provide a grounded analysis. ` +
    `Each source was verified and cross-referenced to ensure accuracy.\n\n` +
    `**Open Questions:**\n` +
    `- Further research could explore emerging developments\n` +
    `- Some sources suggest alternative interpretations worth investigating`;
  return [prompt, mockText, Date.now()];
}

export async function aiGenerate(prompt: AiPrompt): Promise<AiResponse> {
  if (config.aiProvider === "mock") return aiGenerateMock(prompt);
  return aiGenerateReal(prompt);
}

// ---------------------------------------------------------------------------
// Utility bridge functions (called by Shen via js.call)
// ---------------------------------------------------------------------------

export function now(): number {
  return Date.now();
}

export function toString(x: unknown): string {
  if (typeof x === "string") return x;
  if (typeof x === "number") return String(x);
  if (Array.isArray(x)) return JSON.stringify(x);
  return String(x);
}

export function substr(args: [string, number, number]): string {
  const [s, start, end] = args;
  return s.substring(start, end);
}

export function extractTerms(query: string): string[] {
  // Simple term extraction: split on spaces, filter short words
  return query
    .toLowerCase()
    .split(/\s+/)
    .filter((w) => w.length > 3)
    .filter((w) => !["the", "and", "for", "with", "about", "from", "that", "this"].includes(w));
}

// ---------------------------------------------------------------------------
// Render bridge — pushes UI state to Arrow.js
// ---------------------------------------------------------------------------

let renderCallback: ((state: RenderState) => void) | null = null;

export function setRenderCallback(cb: (state: RenderState) => void): void {
  renderCallback = cb;
}

export function render(state: RenderState): void {
  if (config.onRender) config.onRender(state);
  if (renderCallback) renderCallback(state);
}

// ---------------------------------------------------------------------------
// Shen Engine Interface
// ---------------------------------------------------------------------------
// This is the dispatch table that the Shen runtime uses for js.call.
// Each Shen `(js.call "bridge.X" args)` maps to bridge.X(args).

export const bridge = {
  webSearch,
  webFetch,
  aiGenerate,
  now,
  toString,
  substr,
  extractTerms,
  render,
};

// Make bridge available globally for the Shen runtime
if (typeof globalThis !== "undefined") {
  (globalThis as any).bridge = bridge;
}
