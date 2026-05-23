// runtime/runtime_checkers.test.ts — Verify the :runtime-via plumbing.
//
// These tests exercise the in-process path (mocked fetcher) so they
// run in CI without booting the SBCL backend. There is also a documented
// manual smoke procedure in docs/RUNTIME-VIA.md for the live-backend
// path — see "End-to-end smoke" in that document.
//
// Wave-2 separation convention (mirrors `internal/verified/` in
// multi-tenant-api): runtime checkers and their tests live in their
// own files; the generated guards import them.

import { test } from "node:test";
import assert from "node:assert/strict";

import { shenEval, setShenEvalFetcher, setShenEvalEndpoint } from "./runtime_checkers.ts";
import { QueryText } from "./guards_gen.ts";

// Reusable mock fetcher: capture the last request + drive a scripted
// response. Each test resets the state before configuring its scenario.
function newMockFetcher() {
  const calls: Array<{ url: string; body: any }> = [];
  let nextResponse: { ok: boolean; status: number; body: any } = {
    ok: true,
    status: 200,
    body: { ok: true },
  };
  const fetcher = async (
    url: string,
    init: { method: string; headers: Record<string, string>; body: string; signal?: AbortSignal }
  ) => {
    calls.push({ url, body: JSON.parse(init.body) });
    return {
      ok: nextResponse.ok,
      status: nextResponse.status,
      async json() {
        return nextResponse.body;
      },
      async text() {
        return typeof nextResponse.body === "string"
          ? nextResponse.body
          : JSON.stringify(nextResponse.body);
      },
    };
  };
  return {
    fetcher,
    calls,
    setResponse(r: { ok: boolean; status: number; body: any }) {
      nextResponse = r;
    },
  };
}

test("shenEval: POSTs predicate + args envelope and returns ok=true on backend success", async () => {
  const mock = newMockFetcher();
  setShenEvalFetcher(mock.fetcher);
  setShenEvalEndpoint(null);
  mock.setResponse({ ok: true, status: 200, body: { ok: true } });

  const result = await shenEval({}, "query-text", ["non-empty"]);

  assert.equal(result.ok, true);
  assert.equal(mock.calls.length, 1);
  assert.equal(mock.calls[0].url, "/api/eval-predicate");
  assert.deepEqual(mock.calls[0].body, { predicate: "query-text", args: ["non-empty"] });

  setShenEvalFetcher(null);
});

test("shenEval: propagates backend ok=false + error string", async () => {
  const mock = newMockFetcher();
  setShenEvalFetcher(mock.fetcher);
  mock.setResponse({ ok: true, status: 200, body: { ok: false, error: "empty query rejected" } });

  const result = await shenEval({}, "query-text", [""]);

  assert.equal(result.ok, false);
  assert.equal(result.error, "empty query rejected");

  setShenEvalFetcher(null);
});

test("shenEval: surfaces non-2xx HTTP responses as ok=false with status in message", async () => {
  const mock = newMockFetcher();
  setShenEvalFetcher(mock.fetcher);
  mock.setResponse({ ok: false, status: 503, body: "service unavailable" });

  const result = await shenEval({}, "query-text", ["x"]);

  assert.equal(result.ok, false);
  assert.match(result.error ?? "", /503/);

  setShenEvalFetcher(null);
});

test("shenEval: surfaces transport errors as ok=false with a transport: prefix", async () => {
  setShenEvalFetcher(async () => {
    throw new Error("connection refused");
  });

  const result = await shenEval({}, "query-text", ["x"]);

  assert.equal(result.ok, false);
  assert.match(result.error ?? "", /transport.*connection refused/);

  setShenEvalFetcher(null);
});

// ============================================================================
// End-to-end compile-time gate × runtime check composition
// ============================================================================

test("QueryText.createOrThrow consults shenEval via the constructor; non-empty input accepted", async () => {
  const mock = newMockFetcher();
  setShenEvalFetcher(mock.fetcher);
  mock.setResponse({ ok: true, status: 200, body: { ok: true } });

  const qt = await QueryText.createOrThrow({}, "medicare advantage plans");

  assert.equal(qt.val(), "medicare advantage plans");
  // The constructor MUST have called the backend; the runtime check is
  // non-skippable.
  assert.equal(mock.calls.length, 1);
  assert.equal(mock.calls[0].body.predicate, "query-text");
  assert.deepEqual(mock.calls[0].body.args, ["medicare advantage plans"]);

  setShenEvalFetcher(null);
});

test("QueryText.createOrThrow rejects when the backend returns ok=false", async () => {
  const mock = newMockFetcher();
  setShenEvalFetcher(mock.fetcher);
  mock.setResponse({ ok: true, status: 200, body: { ok: false, error: "query is empty" } });

  await assert.rejects(
    QueryText.createOrThrow({}, ""),
    (err: Error) => /shenEval rejected query-text/.test(err.message)
  );
  // The backend WAS consulted — that's the whole point. There is no
  // path through the constructor that skips this call.
  assert.equal(mock.calls.length, 1);

  setShenEvalFetcher(null);
});

test("QueryText.tryCreate returns Error on rejection rather than throwing", async () => {
  const mock = newMockFetcher();
  setShenEvalFetcher(mock.fetcher);
  mock.setResponse({ ok: true, status: 200, body: { ok: false, error: "bad input" } });

  const result = await QueryText.tryCreate({}, "");

  assert.ok(result instanceof Error);
  if (result instanceof Error) {
    assert.match(result.message, /shenEval rejected query-text/);
  }

  setShenEvalFetcher(null);
});
