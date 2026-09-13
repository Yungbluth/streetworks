import { describe, expect, it } from "vitest";
import { dotStyle } from "./dots";

describe("dotStyle", () => {
  it("draws recent work solid", () => {
    expect(dotStyle("recent", 6, "#e8418c")).toEqual({ r: 6, fill: "#e8418c", alpha: 1 });
    expect(dotStyle("back", 6, "#e8418c")).toMatchObject({ stroke: "#e8418c", fill: undefined });
    const old = dotStyle("old", 6, "#e8418c");
    expect(old.r).toBeLessThan(3);
    expect(old.alpha).toBeLessThan(1);
    expect(old.stroke).toBeUndefined();
  });

  it("outlines white paint at every age", () => {
    for (const age of ["recent", "back", "old"] as const) {
      expect(dotStyle(age, 6, "#ffffff", "#6d7077").stroke).toBe("#6d7077");
    }
    expect(dotStyle("old", 6, "#ffffff", "#6d7077").fill).toBe("#ffffff");
  });

});
