import { describe, expect, it } from "vitest";
import { distanceMeters } from "./geo";

describe("distanceMeters", () => {
  it("measures a degree of latitude", () => {
    expect(distanceMeters({ lat: 32, lng: -111 }, { lat: 33, lng: -111 })).toBeCloseTo(111_195, -2);
  });

  it("shrinks east-west distances away from the equator", () => {
    expect(distanceMeters({ lat: 32.2, lng: -111 }, { lat: 32.2, lng: -110 })).toBeCloseTo(94_100, -3);
  });
});
