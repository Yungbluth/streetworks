import { describe, expect, it } from "vitest";
import { DETAIL_ZOOM, arriving, dotStyle, drawnAge, faded } from "./dots";

describe("dotStyle", () => {
  it("draws recent work solid", () => {
    expect(dotStyle("recent", 6, "#e8418c")).toEqual({ r: 6, fill: "#e8418c", alpha: 1 });
    expect(dotStyle("back", 6, "#e8418c")).toMatchObject({ stroke: "#e8418c", fill: undefined });
    const old = dotStyle("old", 6, "#e8418c");
    expect(old.r).toBeLessThan(3);
    expect(old.alpha).toBeLessThan(1);
    expect(old.stroke).toBeUndefined();
  });

  it("keeps white paint white at every age", () => {
    for (const age of ["recent", "back", "old"] as const) {
      const d = dotStyle(age, 6, "#ffffff", "#55585d");
      // The paint itself stays white: a dark ring would read as the "ring" age mark
      expect(d.fill ?? d.stroke).toBe("#ffffff");
      expect(d.outline).toBe("#55585d");
      expect(d.outlineWidth).toBeLessThanOrEqual(1);
    }
    expect(dotStyle("back", 6, "#ffffff", "#55585d")).toMatchObject({ stroke: "#ffffff", fill: undefined });
    expect(dotStyle("recent", 6, "#ffffff", "#55585d").stroke).toBeUndefined();
  });

});

describe("faded", () => {
  it("fades a dot that isn't part of the selection", () => {
    const style = dotStyle("back", 6, "#e8418c");
    const f = faded(style);
    expect(f.alpha).toBeCloseTo(style.alpha * 0.25);
    expect(f.stroke).toBe(style.stroke);
    expect(f.r).toBe(style.r);
  });
});

describe("drawnAge", () => {
  it("draws every dot solid at city zoom", () => {
    expect(drawnAge("back", 11)).toBe("recent");
    expect(drawnAge("old", 13)).toBe("recent");
    expect(drawnAge("recent", 11)).toBe("recent");
    expect(drawnAge("back", DETAIL_ZOOM)).toBe("back");
    expect(drawnAge("old", 17)).toBe("old");
  });
});

describe("arriving", () => {
  it("marks the year a replay has just added", () => {
    expect(arriving(2019, 2019, true)).toBe(true);
    expect(arriving(2018, 2019, true)).toBe(false);
    expect(arriving(2019, 2019, false)).toBe(false);
  });
});
