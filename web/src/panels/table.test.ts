// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import type { Detail } from "../data";
import { permitTable, type TableStyle } from "./table";

const style: TableStyle = { first: null, button: "more", link: "linkish" };
const historyList = (entries: Parameters<typeof permitTable>[0], st = style) => permitTable(entries, st);

const base: Detail = {
  permit: "T23EX02214", type: "EXCAVATION", status: "FINAL", noWork: false,
  date: "2023-08-02", dateBasis: "DATE_ISSUED", description: "New water service tap", filed: "749 E SPEEDWAY BL",
};

const render = (d: Partial<Detail>) => historyList([{ index: 0, category: "Excavation", detail: { ...base, ...d } }]).textContent ?? "";

describe("a permit in the history", () => {
  it("shows the address as the city filed it", () => {
    expect(render({})).toContain("Filed as");
    expect(render({})).toContain("749 E SPEEDWAY BL");
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
    expect(text).toContain("Finished");
    expect(text).toContain("Sep 14, 2023");
  });

  it("falls back to the sub-type", () => {
    expect(render({ description: "", subType: "CURB" })).toContain("CURB");
    expect(render({ description: "", subType: undefined })).toContain("EXCAVATION");
  });

  it("names the kind of work in the closed row, not only as a color", () => {
    const el = historyList([{ index: 0, category: "Excavation", detail: base }]);
    expect(el.querySelector(".permit-open")!.textContent).toContain("Excavation");
  });

});

describe("paging a long history", () => {
  const many = (n: number) => Array.from({ length: n }, (_, i) => ({ index: i, category: "Excavation", detail: base }));
  const button = (n: number) => historyList(many(n)).querySelector("button.more")?.textContent;

  it("says how many are left", () => {
    expect(button(60)).toBe("Show 25 more of 35");
  });

});

describe("the permit table", () => {
  const many = (n: number) => Array.from({ length: n }, (_, i) => ({ index: i, category: "Excavation", detail: base }));

  it("numbers revisions or gives distances in its first column", () => {
    const delta = historyList([{ index: 0, category: "Excavation", detail: base, delta: 7 }], { ...style, first: "Δ" });
    expect(delta.querySelector("th")!.textContent).toBe("Δ");
    expect(delta.querySelector(".permit-first:not(th)")!.textContent).toBe("Δ7");
    const away = historyList([{ index: 0, category: "Excavation", detail: base, distanceM: 100 }], { ...style, first: "Away" });
    expect(away.querySelector("td.permit-first")!.textContent).toBe("330 ft");
    expect(historyList(many(1)).querySelectorAll("th")).toHaveLength(2);
  });

  it("opens a line to its record and says so", () => {
    const el = historyList(many(1));
    const open = el.querySelector<HTMLButtonElement>(".permit-open")!;
    open.click();
    expect(open.getAttribute("aria-expanded")).toBe("true");
    expect(el.querySelector<HTMLElement>(".permit-more")!.hidden).toBe(false);
    expect(el.querySelector(".permit-more")!.textContent).toContain("Filed as");
  });
});

describe("nearby permits", () => {
  it("say where each one is, beside its kind", () => {
    const el = permitTable(
      [{ index: 0, category: "Excavation", detail: base, distanceM: 50, placeName: "1300 block of E Speedway Blvd", onPlace: () => {} }],
      { ...style, first: "Away" },
    );
    expect(el.querySelector(".permit-kind")!.textContent).toBe("Excavation · 1300 block of E Speedway Blvd");
  });
});

describe("a chosen permit", () => {
  const many = (n: number, picked: number) => Array.from({ length: n }, (_, i) => ({ index: i, category: "Excavation", detail: base, picked: i === picked }));

  it("is marked as the current row", () => {
    const el = historyList(many(3, 1));
    const rows = el.querySelectorAll(".permit-row");
    expect(rows[1]!.getAttribute("aria-current")).toBe("true");
    expect(rows[1]!.classList.contains("picked")).toBe(true);
    expect(el.querySelectorAll("[aria-current], .picked")).toHaveLength(1);
  });

});
