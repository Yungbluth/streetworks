// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { PlaceKind, type Places } from "../data";
import { apartMessage, describe as describeFound } from "./search";

const places: Places = {
  streets: ["E SPEEDWAY BLVD", "E GRANT RD"],
  kind: [PlaceKind.Block],
  street: [0],
  cross: [-1],
  block: [1200],
  lat: [3223000],
  lng: [-11095000],
  label: {},
};
const bounds: [number, number, number, number] = [32.2, -111, 32.3, -110.9];

describe("search results", () => {
  it("say how many permits a whole street has", () => {
    const r = describeFound(places, { kind: "street", street: 0, bounds, permits: 1234 }, "speedway");
    expect(r.detail).toBe("Whole street, 1,234 permits");
  });

  it("say once, above the list", () => {
    const apart = { sides: [0, 1] as [number, number], gapM: 1700 };
    const r = describeFound(places, { kind: "street", street: 0, bounds, permits: 1, apart }, "speedway & grant");
    expect(r.detail).toBe("Whole street, 1 permit");
    expect(apartMessage(places, apart)).toBe("Speedway Blvd and Grant Rd don't meet. At their closest they're 1.1 mi apart.");
  });
});
