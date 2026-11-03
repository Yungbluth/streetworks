// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from "vitest";
import { cloudPath, station, stationTicks, swatch } from "./marks";

describe("cloudPath", () => {
  it("draws a closed loop of outward scallops", () => {
    const d = cloudPath(30, 30, 20, 8);
    expect(d.startsWith("M30.00 10.00")).toBe(true);
    expect(d.match(/ A/g)).toHaveLength(8);
    expect(d.endsWith("Z")).toBe(true);
  });
});

describe("stations", () => {
  it("writes a block number as a drafting station", () => {
    expect(station(1900)).toBe("19+00");
    expect(station(0)).toBe("0+00");
    expect(station(10000)).toBe("100+00");
  });

});

describe("swatch", () => {
  afterEach(() => (document.documentElement.style.cssText = ""));
  it("gets the page's swatch hairline", () => {
    document.documentElement.style.setProperty("--paint-excavation", "#e7e8e3");
    document.documentElement.style.setProperty("--paint-excavation-edge", "none");
    expect(swatch("Excavation").querySelector("circle")!.getAttribute("stroke")).toBeNull();
    document.documentElement.style.setProperty("--swatch-edge", "#23252a66");
    expect(swatch("Excavation").querySelector("circle")!.getAttribute("stroke")).toBe("#23252a66");
  });
});
