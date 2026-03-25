/**
 * runtime/medicare-bridge.ts — API client for Medicare plan lookup
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

export interface CacheStats {
  count: number;
  ttlSeconds: number;
  entries: { key: string; cachedAt: number; ageSeconds: number }[];
}

// ---------------------------------------------------------------------------
// API calls
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
    const err = await resp.json();
    throw new Error(err.error || `Medicare lookup failed: ${resp.status}`);
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
    const err = await resp.json();
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

export async function getCacheStats(): Promise<CacheStats> {
  const resp = await fetch(`${BASE}/api/medicare/cache`);
  if (!resp.ok) throw new Error(`Cache stats failed: ${resp.status}`);
  return resp.json();
}

export async function clearCache(): Promise<void> {
  await fetch(`${BASE}/api/medicare/cache`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action: "clear" }),
  });
}
