import { describe, expect, it } from "vitest";
import type { Index } from "./data";
import { fromHash, toHash, type ViewState } from "./url";

const idx = { categories: ["Barricade", "Combo", "Excavation", "Other"] } as unknown as Index;

describe("url state", () => {
  it("round-trips a full state", () => {
    const s: ViewState = { place: 812, q: "749 E Speedway & 4th", radiusM: 402, from: 2005, to: 2012, cats: [0, 2], includeNoWork: true };
    expect(fromHash(toHash(s), idx)).toEqual(s);
  });

  it("leaves out what isn't set", () => {
    expect(toHash({})).toBe("");
    expect(fromHash("", idx)).toEqual({});
    expect(fromHash("#", idx)).toEqual({});
  });

  it("drops what it doesn't recognize and keeps the rest", () => {
    expect(fromHash("#place=abc&r=999&cats=0,9,x,2&from=2005&to=soon&nowork=maybe&zz=1", idx)).toEqual({ from: 2005, cats: [0, 2] });
    expect(fromHash("#place=-3&r=152", idx)).toEqual({ radiusM: 152 });
    expect(fromHash("#place=1.5", idx)).toEqual({});
    expect(fromHash("#%E0%A4%A&&&==", idx)).toEqual({});
    expect(fromHash("#from=1200&to=2026", idx)).toEqual({ to: 2026 });
  });

});
