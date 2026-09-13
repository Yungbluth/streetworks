// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { PlaceKind, type Places } from "../data";
import { describe as describeFound } from "./search";

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

  it("say when two cross streets never meet", () => {
    const r = describeFound(places, { kind: "street", street: 0, bounds, permits: 1, apart: 1 }, "speedway & grant");
    expect(r.detail).toBe("Whole street, 1 permit. It doesn't cross E Grant Rd.");
  });
});
