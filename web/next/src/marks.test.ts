import { describe, expect, it } from "vitest";
import { cloudPath } from "./marks";

describe("cloudPath", () => {
  it("draws a closed loop of outward scallops", () => {
    const d = cloudPath(30, 30, 20, 8);
    expect(d.startsWith("M30.00 10.00")).toBe(true);
    expect(d.match(/ A/g)).toHaveLength(8);
    expect(d.endsWith("Z")).toBe(true);
  });
});
