import { describe, it, beforeEach, mock } from "node:test";
import assert from "node:assert/strict";

// Mock fetch before importing the module
const mockFetch = mock.fn<typeof globalThis.fetch>();
globalThis.fetch = mockFetch as unknown as typeof globalThis.fetch;

// Dynamic import after mock is in place
const { research, search, fetchUrl, generate, getPipelineState } = await import("./bridge.js");

function mockResponse(data: unknown, ok = true, status = 200): Response {
  return {
    ok,
    status,
    json: async () => data,
    headers: new Headers(),
    redirected: false,
    statusText: ok ? "OK" : "Error",
    type: "basic" as ResponseType,
    url: "",
    clone: () => mockResponse(data, ok, status),
    body: null,
    bodyUsed: false,
    arrayBuffer: async () => new ArrayBuffer(0),
    blob: async () => new Blob(),
    formData: async () => new FormData(),
    text: async () => JSON.stringify(data),
  } as Response;
}

describe("bridge API client", () => {
  beforeEach(() => {
    mockFetch.mock.resetCalls();
  });

  it("research() sends POST with query", async () => {
    const result = { query: "test", summary: "result", sources: [], timestamp: 1 };
    mockFetch.mock.mockImplementation(async () => mockResponse(result));

    const res = await research("test query");
    assert.equal(res.summary, "result");

    const call = mockFetch.mock.calls[0];
    assert.equal(call.arguments[0], "/api/research");
    const opts = call.arguments[1] as RequestInit;
    assert.equal(opts.method, "POST");
    assert.deepEqual(JSON.parse(opts.body as string), { query: "test query" });
  });

  it("search() sends POST with query and maxResults", async () => {
    mockFetch.mock.mockImplementation(async () =>
      mockResponse({ query: "q", hits: [], timestamp: 1 })
    );

    await search("q", 5);

    const call = mockFetch.mock.calls[0];
    assert.equal(call.arguments[0], "/api/search");
    const body = JSON.parse((call.arguments[1] as RequestInit).body as string);
    assert.equal(body.query, "q");
    assert.equal(body.maxResults, 5);
  });

  it("fetchUrl() sends POST with url", async () => {
    mockFetch.mock.mockImplementation(async () =>
      mockResponse({ url: "http://example.com", content: "text", timestamp: 1 })
    );

    await fetchUrl("http://example.com");

    const body = JSON.parse((mockFetch.mock.calls[0].arguments[1] as RequestInit).body as string);
    assert.equal(body.url, "http://example.com");
  });

  it("generate() sends POST with system and user", async () => {
    mockFetch.mock.mockImplementation(async () =>
      mockResponse({ text: "response", timestamp: 1 })
    );

    const res = await generate("sys", "usr");
    assert.equal(res.text, "response");

    const body = JSON.parse((mockFetch.mock.calls[0].arguments[1] as RequestInit).body as string);
    assert.equal(body.system, "sys");
    assert.equal(body.user, "usr");
  });

  it("getPipelineState() sends GET", async () => {
    mockFetch.mock.mockImplementation(async () =>
      mockResponse({ phase: "idle", data: null })
    );

    const state = await getPipelineState();
    assert.equal(state.phase, "idle");
    assert.equal(mockFetch.mock.calls[0].arguments[0], "/api/state");
  });

  it("throws on non-ok response", async () => {
    mockFetch.mock.mockImplementation(async () => mockResponse({}, false, 500));

    await assert.rejects(() => research("fail"), /Research failed: 500/);
  });
});
