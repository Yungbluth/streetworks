import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { Permits } from "./data";
import { cellKey, cellsInCircle, distanceMeters } from "./geo";

describe("distanceMeters", () => {
  it("measures a degree of latitude", () => {
    expect(distanceMeters({ lat: 32, lng: -111 }, { lat: 33, lng: -111 })).toBeCloseTo(111_195, -2);
  });

  it("shrinks east-west distances away from the equator", () => {
    expect(distanceMeters({ lat: 32.2, lng: -111 }, { lat: 32.2, lng: -110 })).toBeCloseTo(94_100, -3);
  });
});

describe("cellsInCircle", () => {
  const fixture = JSON.parse(readFileSync(join(__dirname, "fixtures/data/permits.json"), "utf8")) as Permits;

  it.each([
    [{ lat: 32.2362, lng: -110.9612 }, 152],
    [{ lat: 32.2362, lng: -110.9612 }, 1609],
    [{ lat: 32.2, lng: -110.88 }, 8000],
    [{ lat: 32.22, lng: -110.9 }, 20000],
  ])("covers every fixture permit within the circle around %o, %i m", (center, radius) => {
    const cells = new Set(cellsInCircle(center, radius));
    let inside = 0;
    for (let i = 0; i < fixture.lat.length; i++) {
      const p = { lat: fixture.lat[i]! / 1e5, lng: fixture.lng[i]! / 1e5 };
      if (distanceMeters(center, p) <= radius) {
        inside++;
        expect(cells.has(cellKey(p.lat, p.lng))).toBe(true);
      }
    }
    expect(inside).toBeGreaterThan(0);
  });

  it("covers a circle straddling square edges with every square it touches", () => {
    // Right on a corner: the four squares around it
    const cells = cellsInCircle({ lat: 32.23, lng: -110.97 }, 50);
    expect(cells.sort()).toEqual(["3222_-11097", "3222_-11098", "3223_-11097", "3223_-11098"].sort());
  });

  it("returns just the one square for a small circle in the middle of it", () => {
    expect(cellsInCircle({ lat: 32.2255, lng: -110.9655 }, 50)).toEqual(["3222_-11097"]);
  });
});
