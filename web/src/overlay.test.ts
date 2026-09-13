// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { overlayPopup } from "./overlay";

describe("overlayPopup", () => {
  it("lists a segment's properties", () => {
    const el = overlayPopup({ OBJECTID: 4, STREET: "BROADWAY BLVD", YEAR: 2024, NOTE: "", GlobalID: "x", Shape__Length: 12.3, PHASE: null });
    const rows = [...el.querySelectorAll("tr")].map((tr) => tr.textContent);
    expect(rows).toEqual(["STREETBROADWAY BLVD", "YEAR2024"]);
  });

  it("keeps markup in a property as text", () => {
    expect(overlayPopup({ NAME: "<b>x</b>" }).querySelector("b")).toBeNull();
  });

});
