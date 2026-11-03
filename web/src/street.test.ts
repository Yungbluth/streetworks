import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { canonicalStreet, splitStreet, withoutDirection } from "./street";

// The same table the Go build is tested with, so both sides name streets alike
const cases = JSON.parse(
  readFileSync(join(__dirname, "../../internal/permits/testdata/street_cases.json"), "utf8"),
) as [string, string, string][];

describe("canonicalStreet", () => {
  it("loads the shared table", () => expect(cases.length).toBeGreaterThanOrEqual(40));
  it.each(cases)("(%j, %j) is %j", (dir, name, want) => {
    expect(canonicalStreet(dir, name)).toBe(want);
  });
});

describe("withoutDirection", () => {
  it("drops a leading direction the way intersections are stored", () => {
    expect(withoutDirection("E SPEEDWAY BLVD")).toBe("SPEEDWAY BLVD");
    expect(withoutDirection("WEST ST")).toBe("WEST ST");
    expect(withoutDirection("KOLB RD")).toBe("KOLB RD");
  });
});
