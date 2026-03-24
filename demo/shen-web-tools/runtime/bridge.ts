/**
 * runtime/bridge.ts — Frontend API client for the CL backend.
 *
 * The Shen logic runs on SBCL (Common Lisp). This module is a thin
 * HTTP client that calls the CL backend's JSON API endpoints.
 *
 * All intelligence lives in Shen on the server. This is just fetch().
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
