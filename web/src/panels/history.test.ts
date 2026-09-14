// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import type { Detail } from "../data";
import { historyList } from "./history";

const base: Detail = {
  permit: "T23EX02214", type: "EXCAVATION", status: "FINAL", noWork: false,
  date: "2023-08-02", dateBasis: "DATE_ISSUED", description: "New water service tap", filed: "749 E SPEEDWAY BL",
};

const render = (d: Partial<Detail>) => historyList([{ index: 0, category: "Excavation", detail: { ...base, ...d } }]).textContent ?? "";

describe("a permit in the history", () => {
  it("shows the address as the city filed it", () => {
    expect(render({})).toContain("Filed as 749 E SPEEDWAY BL");
  });

  it("says which date it's listed under", () => {
    expect(render({})).not.toContain("Dated by");
    expect(render({ dateBasis: "DATE_APPROVE" })).toContain("Dated by approval");
    expect(render({ dateBasis: "DATE_ENTERED" })).toContain("Dated by when it was entered");
  });

  it("names who applied, the utility and", () => {
    const text = render({ applicant: "BROWNE CONSTRUCTION", utility: "SOUTHWEST GAS", final: "2023-09-14" });
    expect(text).toContain("BROWNE CONSTRUCTION");
    expect(text).toContain("SOUTHWEST GAS");
    expect(text).toContain("Finished Sep 14, 2023");
  });

  it("falls back to the sub-type", () => {
    expect(render({ description: "", subType: "CURB" })).toContain("CURB");
    expect(render({ description: "", subType: undefined })).toContain("EXCAVATION");
  });

  it("names the kind of work in the closed row, not only as a color", () => {
    const el = historyList([{ index: 0, category: "Excavation", detail: base }]);
    expect(el.querySelector("summary")!.textContent).toContain("Excavation");
  });

});

describe("paging a long history", () => {
  const many = (n: number) => Array.from({ length: n }, (_, i) => ({ index: i, category: "Excavation", detail: base }));
  const button = (n: number) => historyList(many(n)).querySelector("button")?.textContent;

  it("says how many are left", () => {
    expect(button(60)).toBe("Show 25 more of 35");
  });

  it("says 'the last' when the rest fit on one page", () => {
    expect(button(43)).toBe("Show the last 18");
    expect(button(26)).toBe("Show the last one");
  });
});
