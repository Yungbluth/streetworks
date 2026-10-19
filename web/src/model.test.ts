import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import type { Permits, Places, Site } from "./data";
import { digsByPlace, permitsOf } from "./model";

const read = <T>(f: string) => JSON.parse(readFileSync(join(__dirname, "fixtures/data", f), "utf8")) as T;

describe("permitsOf", () => {
  const permits = read<Permits>("permits.json");
  const places = read<Places>("places.json");
  const site = { permits, places } as Site;
  const all = { from: 0, to: 0, cats: new Set([0, 1, 2]), includeNoWork: true };

  it("finds exactly the permits stored under each place", () => {
    for (let p = 0; p < places.kind.length; p++) {
      const want = permits.place.flatMap((q, i) => (q === p ? [i] : []));
      expect(permitsOf(site, p, all)).toEqual(want);
    }
  });

  it("applies the filter", () => {
    expect(permitsOf(site, 0, { ...all, cats: new Set() })).toEqual([]);
  });
});

describe("digsByPlace", () => {
  it("counts the Excavation and Combo permits at each place", () => {
    const site = {
      index: { categories: ["Barricade", "Combo", "Excavation"] },
      places: { kind: [0, 0] },
      permits: { place: [0, 0, 0, 1], year: [2010, 2011, 2012, 2011], cat: [0, 1, 2, 2], noWork: [0, 0, 0, 1] },
    } as unknown as Site;
    const filter = { from: 2005, to: 2026, cats: new Set([0]), includeNoWork: false };
    expect([...digsByPlace(site, filter)]).toEqual([2, 0]);
    expect([...digsByPlace(site, { ...filter, from: 2012 })]).toEqual([1, 0]);
  });
});
