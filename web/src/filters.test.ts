import { describe, expect, it } from "vitest";
import type { Permits } from "./data";
import { countByPlace, permitsMatching, type Filter } from "./filters";

const cats = { Excavation: 0, Barricade: 1, Combo: 2, "Civic event": 3 } as const;

// six permits at place 0, from the old Go filter tests: one voided, one undated
function permits(): Permits {
  const rows: [keyof typeof cats, number, 0 | 1][] = [
    ["Excavation", 2019, 0],
    ["Excavation", 2020, 1], // VOID
    ["Barricade", 2021, 0],
    ["Combo", 2022, 0],
    ["Civic event", 2023, 0],
    ["Excavation", 0, 0], // undated
  ];
  return {
    place: rows.map(() => 0),
    year: rows.map((r) => r[1]),
    cat: rows.map((r) => cats[r[0]]),
    noWork: rows.map((r) => r[2]),
    lat: rows.map(() => 3_222_000),
    lng: rows.map(() => -11_097_000),
  };
}

const every = new Set(Object.values(cats));
const only = (...names: (keyof typeof cats)[]) => new Set(names.map((n) => cats[n]));
const filter = (f: Partial<Filter>): Filter => ({ from: 0, to: 0, cats: every, includeNoWork: false, ...f });

describe("countByPlace", () => {
  it.each<[string, Partial<Filter>, number]>([
    ["no filter: voided hidden, undated included", {}, 5],
    ["include void", { includeNoWork: true }, 6],
    ["from only", { from: 2021 }, 3],
    ["to only", { to: 2020 }, 1],
    ["year range inclusive both ends", { from: 2020, to: 2022, includeNoWork: true }, 3],
    ["single year", { from: 2022, to: 2022 }, 1],
    ["range with no data", { from: 2030, to: 2031 }, 0],
    ["inverted range matches nothing", { from: 2023, to: 2019 }, 0],
    ["one category", { cats: only("Barricade") }, 1],
    ["two categories", { cats: only("Barricade", "Combo") }, 2],
    ["category + year", { cats: only("Excavation"), from: 2019, to: 2019 }, 1],
    ["category + void + year", { cats: only("Excavation"), from: 2020, to: 2020, includeNoWork: true }, 1],
    ["any year range leaves out undated permits", { from: 1900, to: 2100 }, 4],
    ["an empty category set shows nothing", { cats: new Set() }, 0],
  ])("%s", (_, f, want) => {
    const counts = countByPlace(permits(), filter(f), 1);
    expect(counts[0]).toBe(want);
  });

  it("counts each place separately", () => {
    const p = permits();
    p.place = [0, 0, 1, 1, 1, 2];
    expect(Array.from(countByPlace(p, filter({ includeNoWork: true }), 3))).toEqual([2, 3, 1]);
  });

});

describe("permitsMatching", () => {
  it("lists the permits that pass the filter", () => {
    expect(permitsMatching(permits(), filter({ from: 2021 }))).toEqual([2, 3, 4]);
  });

  it("keeps only permits inside the circle when given one", () => {
    const p = permits();
    p.lat[2] = 3_232_000; // about 11 km north of the rest
    const near = { center: { lat: 32.22, lng: -110.97 }, radiusM: 500 };
    expect(permitsMatching(p, filter({ includeNoWork: true }), near)).toEqual([0, 1, 3, 4, 5]);
    expect(permitsMatching(p, filter({ includeNoWork: true }), { ...near, radiusM: 20_000 })).toHaveLength(6);
  });
});
