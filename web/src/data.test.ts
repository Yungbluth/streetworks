import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { DataError, loadSite } from "./data";

const fixtures = join(__dirname, "fixtures", "data");
const base = "https://example.test/data/";

type Override = (file: string) => Response | undefined;

// fakeFetch serves the fixture files, with overrides for the cases under test
function fakeFetch(override: Override = () => undefined) {
  const calls: string[] = [];
  const fn = async (input: RequestInfo | URL): Promise<Response> => {
    const file = String(input).slice(base.length).split("?")[0]!;
    calls.push(file);
    const o = override(file);
    if (o) return o;
    try {
      return new Response(readFileSync(join(fixtures, file)), { status: 200 });
    } catch {
      return new Response("not found", { status: 404, statusText: "Not Found" });
    }
  };
  return { fn: fn as typeof fetch, calls };
}

describe("loadSite", () => {
  it("loads the fixture site", async () => {
    const site = await loadSite(base, fakeFetch().fn);
    expect(site.index.version).toBe(1);
    expect(site.index.sparseBefore).toBe(2005);
    expect(site.index.quality.unique_permits).toBe(10);
    expect(site.places.kind.length).toBe(10);
    expect(site.permits.place.length).toBe(10);
    expect(site.places.streets).toContain("E SPEEDWAY BLVD");
  });

  it("rejects a data format it doesn't understand", async () => {
    const { fn } = fakeFetch((f) =>
      f === "index.json" ? new Response(JSON.stringify({ version: 2 }), { status: 200 }) : undefined,
    );
    const err = await loadSite(base, fn).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DataError);
    expect((err as Error).message).toContain("data format 2, this page expects 1");
  });

  it("names the file that failed to load", async () => {
    const { fn } = fakeFetch((f) => (f === "places.json" ? new Response("", { status: 404, statusText: "Not Found" }) : undefined));
    const err = await loadSite(base, fn).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DataError);
    expect((err as Error).message).toContain("places.json");
    expect((err as Error).message).toContain("404");
  });

  it("turns a network failure into a DataError", async () => {
    const fn = (async () => {
      throw new TypeError("Failed to fetch");
    }) as typeof fetch;
    await expect(loadSite(base, fn)).rejects.toBeInstanceOf(DataError);
  });
});

describe("detail squares", () => {
  const key = "3223_-11097"; // the square with 749 E Speedway in it

  it("fetches a square once however often it's asked for", async () => {
    const { fn, calls } = fakeFetch();
    const site = await loadSite(base, fn);
    const [a, b] = await Promise.all([site.detail(key), site.detail(key)]);
    await site.detail(key);
    expect(a).toBe(b);
    expect(calls.filter((c) => c === `details/${key}.json`).length).toBe(1);
    expect(Object.values(a).some((d) => d.filed === "749 E SPEEDWAY BL")).toBe(true);
  });

  it("tries a failed square again next time", async () => {
    let fail = true;
    const { fn, calls } = fakeFetch((f) => (f.startsWith("details/") && fail ? new Response("", { status: 503 }) : undefined));
    const site = await loadSite(base, fn);
    await expect(site.detail(key)).rejects.toBeInstanceOf(DataError);
    fail = false;
    await expect(site.detail(key)).resolves.toBeTypeOf("object");
    expect(calls.filter((c) => c === `details/${key}.json`).length).toBe(2);
  });

  it("refuses a square from a newer build than the page loaded", async () => {
    const { fn } = fakeFetch((f) =>
      f.startsWith("details/") ? new Response(JSON.stringify({ built: "2099-01-01T00:00:00Z", permits: {} }), { status: 200 }) : undefined,
    );
    const site = await loadSite(base, fn);
    const err = await site.detail(key).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DataError);
    expect((err as Error).message).toContain("Reload the page");
  });

  it("asks for a square by build", async () => {
    const urls: string[] = [];
    const { fn } = fakeFetch();
    const site = await loadSite(base, ((u: RequestInfo | URL) => (urls.push(String(u)), fn(u))) as typeof fetch);
    await site.detail(key);
    const asked = new URL(urls.find((u) => u.includes("details/"))!);
    expect(asked.searchParams.get("v")).toBe(site.index.built);
  });

  it("treats a square with no file as empty", async () => {
    const site = await loadSite(base, fakeFetch().fn);
    await expect(site.detail("0_0")).resolves.toEqual({});
  });
});
