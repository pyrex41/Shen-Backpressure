import { describe, it, beforeEach, mock } from "node:test";
import assert from "node:assert/strict";

const mockFetch = mock.fn<typeof globalThis.fetch>();
globalThis.fetch = mockFetch as unknown as typeof globalThis.fetch;

const { medicareChat, medicareLookup, medicareCompare, getPlanTypes } = await import("./medicare-bridge.js");

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

describe("medicare-bridge API client", () => {
  beforeEach(() => {
    mockFetch.mock.resetCalls();
  });

  it("medicareChat() sends message and optional params", async () => {
    const chatResp = { type: "result", data: { summary: "test" }, session: "s-1" };
    mockFetch.mock.mockImplementation(async () => mockResponse(chatResp));

    await medicareChat("what plans?", "sess-1", "33101", "advantage");

    const call = mockFetch.mock.calls[0];
    assert.equal(call.arguments[0], "/api/medicare/chat");
    const body = JSON.parse((call.arguments[1] as RequestInit).body as string);
    assert.equal(body.message, "what plans?");
    assert.equal(body.sessionId, "sess-1");
    assert.equal(body.zip, "33101");
    assert.equal(body.planType, "advantage");
  });

  it("medicareLookup() sends planType, zip, filter", async () => {
    mockFetch.mock.mockImplementation(async () =>
      mockResponse({ planType: "advantage", zip: "33101", summary: "plans" })
    );

    await medicareLookup("advantage", "33101", "insulin");

    const body = JSON.parse((mockFetch.mock.calls[0].arguments[1] as RequestInit).body as string);
    assert.equal(body.planType, "advantage");
    assert.equal(body.zip, "33101");
    assert.equal(body.filter, "insulin");
  });

  it("medicareCompare() sends zip and planTypes", async () => {
    mockFetch.mock.mockImplementation(async () =>
      mockResponse({ zip: "33101", comparisons: [], timestamp: 1 })
    );

    await medicareCompare("33101", ["advantage", "part-d"]);

    const body = JSON.parse((mockFetch.mock.calls[0].arguments[1] as RequestInit).body as string);
    assert.equal(body.zip, "33101");
    assert.deepEqual(body.planTypes, ["advantage", "part-d"]);
  });

  it("getPlanTypes() sends GET", async () => {
    mockFetch.mock.mockImplementation(async () =>
      mockResponse({ planTypes: [{ id: "advantage", label: "Medicare Advantage" }] })
    );

    const types = await getPlanTypes();
    assert.equal(types[0].id, "advantage");
    assert.equal(mockFetch.mock.calls[0].arguments[0], "/api/medicare/plans");
  });

  it("throws on error response with message", async () => {
    mockFetch.mock.mockImplementation(async () =>
      mockResponse({ error: "Invalid zip" }, false, 400)
    );

    await assert.rejects(() => medicareLookup("advantage", "bad", ""), /Invalid zip/);
  });
});
