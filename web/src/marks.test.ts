import { describe, expect, it } from "vitest";
import { cloudPath, station } from "./marks";

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
