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
    expect(site.index.version).toBe(2);
    expect(site.index.sparseBefore).toBe(2005);
    expect(site.index.quality.unique_permits).toBe(10);
    expect(site.places.kind.length).toBe(10);
    expect(site.permits.place.length).toBe(10);
    expect(site.places.streets).toContain("E SPEEDWAY BLVD");
  });

  it("rejects a data format it doesn't understand", async () => {
    const { fn } = fakeFetch((f) =>
      f === "index.json" ? new Response(JSON.stringify({ version: 1 }), { status: 200 }) : undefined,
    );
    const err = await loadSite(base, fn).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DataError);
    expect((err as Error).message).toContain("data format 1, this page expects 2");
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

describe("the permit server", () => {
  it("is asked for records, and checks they're from the same build as the data", async () => {
    const { built } = JSON.parse(readFileSync(join(fixtures, "index.json"), "utf8")) as { built: string };
    const asked: string[] = [];
    const { fn } = fakeFetch();
    const both = (async (u: RequestInfo | URL, init?: RequestInit) => {
      if (String(u).startsWith(base)) return fn(u, init);
      asked.push(String(u));
      return new Response(JSON.stringify({ built, permits: { "0": { permit: "T05EX000001" } } }), { status: 200 });
    }) as typeof fetch;
    const site = await loadSite(base, both, "https://api.example.test/");
    const got = await site.api.records([0]);
    expect(asked).toEqual(["https://api.example.test/v1/permits/lookup"]);
    expect(got.get(0)?.permit).toBe("T05EX000001");
  });
});
