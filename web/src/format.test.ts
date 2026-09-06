import { describe, expect, it } from "vitest";
import { PlaceKind, type Places } from "./data";
import { feet, placeName, yearsText } from "./format";

describe("placeName", () => {
  const places: Places = {
    streets: ["E SPEEDWAY BLVD", "CATHY DR", "GOLF LINKS RD"],
    kind: [PlaceKind.Block, PlaceKind.Intersection, PlaceKind.Point],
    street: [0, 1, -1], cross: [-1, 2, -1], block: [700, -1, -1],
    lat: [0, 0, 0], lng: [0, 0, 0], label: { "2": "4700 S" },
  };
  it("names each kind of place", () => {
    expect(placeName(places, 0)).toBe("700 block of E Speedway Blvd");
    expect(placeName(places, 1)).toBe("Cathy Dr and Golf Links Rd");
    expect(placeName(places, 2)).toBe("4700 S");
  });
  it("calls the zero block by its street", () => {
    expect(placeName({ ...places, block: [0, -1, -1] }, 0)).toBe("0 block of E Speedway Blvd");
  });
});

describe("feet and years", () => {
  it("rounds distances the way people say them", () => {
    expect(feet(10)).toBe("30 ft");
    expect(feet(152)).toBe("500 ft");
    expect(feet(805)).toBe("0.5 mi");
  });
  it("says a span of years plainly", () => {
    expect(yearsText(2005, 2026)).toBe("2005 to 2026");
    expect(yearsText(2012, 2012)).toBe("2012");
  });
});
