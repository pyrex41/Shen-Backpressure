/**
 * runtime/medicare-bridge.ts — API client for Medicare plan lookup
 *
 * Two modes:
 *   1. Structured: medicareLookup(planType, zip, filter) — form-based
 *   2. Conversational: medicareChat(message, sessionId) — natural language
 *
 * The chat endpoint returns both data AND layout intent (generative UI).
 */

const BASE = "";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface MedicareSource {
  url: string;
  title: string;
  snippet: string;
  isMedicareGov: boolean;
}

export interface MedicareResult {
  planType: string;
  planLabel: string;
  zip: string;
  filter: string;
  summary: string;
  sources: MedicareSource[];
  timestamp: number;
  cached: boolean;
  comparisons?: MedicareResult[];
  isFollowup?: boolean;
}

export interface LayoutIntent {
  panels: string[];
  emphasis: string;
  reasoning: string;
}

export interface QueryIntent {
  planType: string;
  zip: string;
  filter: string;
  action: string;
  needsZip: boolean;
  rawQuery: string;
}

export interface ChatResponse {
  type: "result" | "needs-input" | "error";
  data?: MedicareResult;
  layout?: LayoutIntent;
  intent?: QueryIntent;
  session?: string;
  field?: string;
  message?: string;
  error?: string;
}

export interface MedicareComparison {
  zip: string;
  comparisons: MedicareResult[];
  timestamp: number;
}

export interface PlanTypeInfo {
  id: string;
  label: string;
}

// ---------------------------------------------------------------------------
// Conversational endpoint (primary — generative UI)
// ---------------------------------------------------------------------------

export async function medicareChat(
  message: string,
  sessionId?: string,
  zip?: string,
  planType?: string,
): Promise<ChatResponse> {
  const body: Record<string, string> = { message };
  if (sessionId) body.sessionId = sessionId;
  if (zip) body.zip = zip;
  if (planType) body.planType = planType;

  const resp = await fetch(`${BASE}/api/medicare/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: `HTTP ${resp.status}` }));
    throw new Error(err.error || `Chat failed: ${resp.status}`);
  }
  return resp.json();
}

// ---------------------------------------------------------------------------
// Structured endpoints (form-based fallback)
// ---------------------------------------------------------------------------

export async function medicareLookup(
  planType: string,
  zip: string,
  filter = "",
): Promise<MedicareResult> {
  const resp = await fetch(`${BASE}/api/medicare`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ planType, zip, filter }),
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: `HTTP ${resp.status}` }));
    throw new Error(err.error || `Lookup failed: ${resp.status}`);
  }
  return resp.json();
}

export async function medicareCompare(
  zip: string,
  planTypes: string[] = ["advantage", "part-d", "supplement"],
): Promise<MedicareComparison> {
  const resp = await fetch(`${BASE}/api/medicare/compare`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ zip, planTypes }),
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: `HTTP ${resp.status}` }));
    throw new Error(err.error || `Comparison failed: ${resp.status}`);
  }
  return resp.json();
}

export async function getPlanTypes(): Promise<PlanTypeInfo[]> {
  const resp = await fetch(`${BASE}/api/medicare/plans`);
  if (!resp.ok) throw new Error(`Failed to load plan types: ${resp.status}`);
  const data = await resp.json();
  return data.planTypes;
}

export async function clearCache(): Promise<void> {
  await fetch(`${BASE}/api/medicare/cache`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action: "clear" }),
  });
}
