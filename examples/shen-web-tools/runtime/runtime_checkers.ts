// runtime/runtime_checkers.ts — Discharge `:runtime-via` premises.
//
// This module is the SOLE bridge between a generated guard
// constructor and a live runtime check. The annotation
// `(predicate) : verified via shenEval;` on a Shen `verified` premise
// rewires the corresponding TS constructor to call `shenEval(ctx,
// <predicate-name>, [args…])` instead of inlining the predicate. The
// generated module imports `shenEval` from THIS file and pins it to a
// `const _: RuntimeChecker = shenEval` compile-time witness — so if
// you delete this file or rename `shenEval`, `tsc` will fail the
// build. That is what makes the runtime call non-skippable.
//
// Trust model:
//
//   - This file is part of the TCB. Inspect it whenever you inspect
//     the generated guards.
//   - The wire format below is the contract between the TS side and
//     the SBCL backend. Changing either side without the other is a
//     security-relevant change.
//   - The backend MUST validate the predicate name against an
//     allowlist; arbitrary code execution is not on the menu.
//
// Wave-2 convention (mirrors `internal/verified/` in multi-tenant-api):
//   - Checkers live in this file, not sprinkled across the runtime/.
//   - The generated guards module imports them by name.
//   - Anything else that wants to call a Shen-evaluated predicate
//     goes through this file.

import type { RuntimeChecker, RuntimeCheckCtx } from "./guards_gen.js";

// Endpoint override knob for tests + non-default deployments.
const DEFAULT_ENDPOINT = "/api/eval-predicate";
let endpointOverride: string | null = null;

/**
 * setShenEvalEndpoint changes the URL `shenEval` POSTs to. Used by
 * tests; production wiring leaves the default.
 */
export function setShenEvalEndpoint(url: string | null): void {
  endpointOverride = url;
}

// fetcher abstraction — defaults to global fetch, swappable for tests.
type Fetcher = (
  input: string,
  init: { method: string; headers: Record<string, string>; body: string; signal?: AbortSignal }
) => Promise<{ ok: boolean; status: number; json(): Promise<unknown>; text(): Promise<string> }>;

let fetcherOverride: Fetcher | null = null;

/**
 * setShenEvalFetcher injects a custom fetcher for tests / in-process
 * mocks. Pass null to restore the global fetch. The mock receives the
 * same shape as a real Response (just `.ok`, `.status`, `.json()`,
 * `.text()`).
 */
export function setShenEvalFetcher(fn: Fetcher | null): void {
  fetcherOverride = fn;
}

function currentFetcher(): Fetcher {
  if (fetcherOverride) return fetcherOverride;
  if (typeof fetch !== "function") {
    throw new Error(
      "shenEval: no fetch available. In tests, call setShenEvalFetcher() with a mock; " +
        "in Node < 18, polyfill globalThis.fetch."
    );
  }
  return fetch as unknown as Fetcher;
}

/**
 * shenEval — the discharge function for any `: verified via shenEval`
 * premise in specs/core.shen.
 *
 * Wire shape:
 *   POST <endpoint>
 *   Content-Type: application/json
 *   Body: { "predicate": "<spec-side-name>", "args": [<positional…>] }
 *   Response: { "ok": true | false, "error"?: string }
 *
 * The function is intentionally schema-light: every checker shares
 * the same JSON envelope. The backend dispatches on `predicate` to a
 * named Shen function registered in shen-interop.lisp; see
 * `examples/shen-web-tools/backend/shen-interop.lisp` for the dispatch
 * table.
 *
 * Errors:
 *   - HTTP / transport failure → returns { ok: false, error: "<msg>" }.
 *     The generated constructor will throw with the error attached.
 *   - JSON shape mismatch → ditto.
 *   - The backend explicitly returning ok=false → propagated through.
 *
 * No retries: a rejected predicate is a hard failure, not a transient
 * one. Callers that want backpressure-style retries wrap the
 * constructor, not this function.
 */
export const shenEval: RuntimeChecker = async (
  ctx: RuntimeCheckCtx,
  predicate: string,
  args: unknown[]
): Promise<{ ok: boolean; error?: string }> => {
  const endpoint = endpointOverride ?? DEFAULT_ENDPOINT;
  const fetcher = currentFetcher();
  let response;
  try {
    response = await fetcher(endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ predicate, args }),
      signal: ctx.signal,
    });
  } catch (err) {
    return { ok: false, error: `transport: ${err instanceof Error ? err.message : String(err)}` };
  }
  if (!response.ok) {
    const body = await response.text().catch(() => "");
    return { ok: false, error: `backend ${response.status}: ${body || "(no body)"}` };
  }
  let payload: unknown;
  try {
    payload = await response.json();
  } catch (err) {
    return { ok: false, error: `bad json: ${err instanceof Error ? err.message : String(err)}` };
  }
  if (typeof payload !== "object" || payload === null) {
    return { ok: false, error: "backend returned non-object" };
  }
  const p = payload as { ok?: unknown; error?: unknown };
  if (typeof p.ok !== "boolean") {
    return { ok: false, error: "backend response missing boolean 'ok' field" };
  }
  if (p.ok) {
    return { ok: true };
  }
  return {
    ok: false,
    error: typeof p.error === "string" ? p.error : "predicate rejected by backend",
  };
};
