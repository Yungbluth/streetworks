import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./api";
import { DataError, type Detail } from "./data";

const base = "https://api.example.test";
const built = "2026-10-23T13:17:00Z";

const detail = (permit: string): Detail => ({
  permit, type: "Excavation", status: "Issued", noWork: false, date: "2023-05-01", dateBasis: "issued", description: "Water tap", filed: "100 Main St",
});

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

interface Call {
  url: string;
  init?: RequestInit;
}

function fakeFetch(answer?: (c: Call) => Promise<Response> | Response | undefined) {
  const calls: Call[] = [];
  const fn = async (input: RequestInfo | URL, init?: RequestInit) => {
    const c = { url: String(input), init };
    calls.push(c);
    const a = answer?.(c);
    if (a) return a;
    const ids = (JSON.parse(String(init?.body)) as { ids: number[] }).ids;
    return json({ built, permits: Object.fromEntries(ids.map((id) => [id, detail(`T${id}`)])) });
  };
  return { fn: fn as typeof fetch, calls };
}

const rejection = (p: Promise<unknown>) => p.then(() => { throw new Error("expected a rejection"); }, (e: unknown) => e as Error);

describe("records", () => {
  it("asks in chunks of 2000 ids", async () => {
    const { fn, calls } = fakeFetch();
    const ids = Array.from({ length: 4500 }, (_, i) => i);
    const got = await apiClient(base, built, fn).records(ids);
    expect(calls.length).toBe(3);
    expect(calls.every((c) => c.url === `${base}/v1/permits/lookup` && c.init?.method === "POST")).toBe(true);
    expect(calls.map((c) => (JSON.parse(String(c.init?.body)) as { ids: number[] }).ids.length)).toEqual([2000, 2000, 500]);
    expect(got.size).toBe(4500);
    expect(got.get(4499)?.permit).toBe("T4499");
  });

  it("asks only for ids it doesn't have yet", async () => {
    const { fn, calls } = fakeFetch();
    const api = apiClient(base, built, fn);
    await api.records([1, 2, 3]);
    const again = await api.records([3, 2]);
    expect(calls.length).toBe(1);
    expect([...again.keys()].sort()).toEqual([2, 3]);
    await api.records([3, 4]);
    expect(calls.length).toBe(2);
    expect(JSON.parse(String(calls[1]!.init?.body))).toEqual({ ids: [4] });
  });

  it("refuses records from a different build than the page", async () => {
    const { fn } = fakeFetch(() => json({ built: "2099-01-01T00:00:00Z", permits: {} }));
    const err = await rejection(apiClient(base, built, fn).records([1]));
    expect(err).toBeInstanceOf(DataError);
    expect(err.message).toBe("The permit data was updated while this page was open. Reload the page to see it.");
  });

  it("says so when the server can't be reached", async () => {
    const fn = (async () => {
      throw new TypeError("Failed to fetch");
    }) as typeof fetch;
    const err = await rejection(apiClient(base, built, fn).records([1]));
    expect(err).toBeInstanceOf(DataError);
    expect(err.message).toBe("Couldn't reach the permit server.");
  });

  it.each([400, 429, 500, 503])("passes on the server's message with a %i", async (status) => {
    const { fn } = fakeFetch(() => json({ message: "Too many requests; wait a moment and try again." }, status));
    const err = await rejection(apiClient(base, built, fn).records([1]));
    expect(err).toBeInstanceOf(DataError);
    expect(err.message).toBe("Too many requests; wait a moment and try again.");
  });

  it("asks again after a failure", async () => {
    let fail = true;
    const { fn, calls } = fakeFetch(() => (fail ? json({ message: "Something went wrong." }, 500) : undefined));
    const api = apiClient(base, built, fn);
    await expect(api.records([7])).rejects.toBeInstanceOf(DataError);
    fail = false;
    expect((await api.records([7])).get(7)?.permit).toBe("T7");
    expect(calls.length).toBe(2);
  });
});

describe("byNumber", () => {
  it("encodes the permit number into the path", async () => {
    const { fn, calls } = fakeFetch(() => json({ built, id: 4, place: 2, permit: detail("T23/1") }));
    const got = await apiClient(base, built, fn).byNumber("T23/1");
    expect(calls[0]!.url).toBe(`${base}/v1/permits/T23%2F1`);
    expect(got).toEqual({ id: 4, place: 2, permit: detail("T23/1") });
  });
});

describe("byNumber when there's no such permit", () => {
  it("keeps the status, so the page can tell a missing permit from a failure", async () => {
    const { fn } = fakeFetch(() => json({ message: "No permit T00XX000000 is on record." }, 404));
    const err = await rejection(apiClient(base, built, fn).byNumber("t00xx000000"));
    expect(err).toBeInstanceOf(DataError);
    expect((err as DataError).status).toBe(404);
    expect(err.message).toBe("No permit T00XX000000 is on record.");
  });
});

afterEach(() => {
  vi.useRealTimers();
});
