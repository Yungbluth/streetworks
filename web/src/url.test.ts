import { describe, expect, it } from "vitest";
import { PlaceKind, type Index, type Places } from "./data";
import { fromHash, placeKey, toHash, type ViewState } from "./url";

const idx = { categories: ["Barricade", "Combo", "Excavation", "Other"] } as unknown as Index;

describe("url state", () => {
  it("round-trips a full state", () => {
    const s: ViewState = { place: "b:E SPEEDWAY BLVD:700", q: "749 E Speedway & 4th", radiusM: 402, from: 2005, to: 2012, cats: [0, 2], includeNoWork: true };
    expect(fromHash(toHash(s, idx), idx)).toEqual(s);
  });

  it("opens a permit by its number", () => {
    expect(fromHash("#permit=T23SY041246", idx)).toEqual({ permit: "T23SY041246" });
    // spaces from a copied link, as %20 or +, are trimmed off
    expect(fromHash("#permit=%20T23SY041246%20", idx)).toEqual({ permit: "T23SY041246" });
    expect(fromHash("#permit=+t23sy041246", idx)).toEqual({ permit: "t23sy041246" });
    const s: ViewState = { permit: "T23SY041246", from: 2005, to: 2012 };
    expect(toHash(s, idx)).toContain("permit=T23SY041246");
    expect(fromHash(toHash(s, idx), idx)).toEqual(s);
  });

  it("names categories in the link", () => {
    expect(toHash({ cats: [0, 2] }, idx)).toBe("#cats=Barricade%2CExcavation");
    const later = { categories: ["Annual", "Barricade", "Combo", "Excavation", "Other"] } as unknown as Index;
    expect(fromHash("#cats=Barricade,Excavation", later)).toEqual({ cats: [1, 3] });
  });

  it("leaves out what isn't set", () => {
    expect(toHash({}, idx)).toBe("");
    expect(fromHash("", idx)).toEqual({});
    expect(fromHash("#", idx)).toEqual({});
  });

  it("falls back to the default categories", () => {
    expect(fromHash("#cats=99", idx)).toEqual({});
    expect(fromHash("#cats=Nope,x", idx)).toEqual({});
  });

  it("drops what it doesn't recognize and keeps the rest", () => {
    expect(fromHash("#place=&r=999&cats=Barricade,Nope,Excavation&from=2005&to=soon&nowork=maybe&zz=1", idx)).toEqual({ from: 2005, cats: [0, 2] });
    expect(fromHash("#r=152", idx)).toEqual({ radiusM: 152 });
    expect(fromHash("#%E0%A4%A&&&==", idx)).toEqual({});
    expect(fromHash("#from=1200&to=2026", idx)).toEqual({ to: 2026 });
  });

});

describe("place keys", () => {
  const places: Places = {
    streets: ["E SPEEDWAY BLVD", "CATHY DR", "GOLF LINKS RD"],
    kind: [PlaceKind.Block, PlaceKind.Intersection, PlaceKind.Point],
    street: [0, 1, -1], cross: [-1, 2, -1], block: [700, -1, -1],
    lat: [3223620, 3218580, 3215900], lng: [-11096120, -11084050, -11091270], label: { "2": "4700 S" },
  };

  it("names a place by what it is", () => {
    expect([0, 1, 2].map((p) => placeKey(places, p))).toEqual(["b:E SPEEDWAY BLVD:700", "x:CATHY DR:GOLF LINKS RD", "p:4700 S:321590:-1109127"]);
  });

});
