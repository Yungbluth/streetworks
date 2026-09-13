// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { PlaceKind, type Detail, type Site } from "../data";
import { summarizePlaces } from "../filters";
import type { Ctx } from "../model";
import { placePanel, placeSummary } from "./place";

describe("placeSummary", () => {
  const cats = ["Barricade", "Combo", "Excavation"];
  it("says how many, over", () => {
    const counts = new Map([[0, 14], [1, 11], [2, 3]]);
    expect(placeSummary(counts, [2006, 2026, 2019], cats)).toBe("28 permits from 2006 to 2026, mostly Barricade (14) and Combo (11).");
  });
  it("keeps it short for one kind or one permit", () => {
    expect(placeSummary(new Map([[2, 3]]), [2010, 2012], cats)).toBe("3 permits from 2010 to 2012, all Excavation.");
    expect(placeSummary(new Map([[1, 1]]), [2015], cats)).toBe("1 permit in 2015, Combo.");
  });
  it("says so when nothing matches", () => {
    expect(placeSummary(new Map(), [], cats)).toBe("No permits here match the filters.");
  });
});

function tinySite(): Site {
  const detail = (permit: string, date: string): Detail => ({
    permit, type: "EXCAVATION", status: "CLOSED", noWork: false, date, dateBasis: "DATE_ISSUED", description: `Work ${permit}`, filed: "100 E A ST",
  });
  const details: Record<string, Detail> = { "0": detail("P0", "2020-01-02"), "1": detail("P1", "2024-05-06"), "2": detail("P2", "2022-03-04") };
  return {
    index: { categories: ["Excavation"], built: "b" } as Site["index"],
    places: { streets: ["E A ST"], kind: [PlaceKind.Block, PlaceKind.Block], street: [0, 0], cross: [-1, -1], block: [100, 200], lat: [3222000, 3222000], lng: [-11097000, -11096970], label: {} },
    permits: { place: [0, 0, 1], year: [2020, 2024, 2022], cat: [0, 0, 0], noWork: [0, 0, 0], lat: [3222000, 3222000, 3222000], lng: [-11097000, -11097000, -11096970] },
    detail: async () => details,
  };
}

describe("placePanel", () => {
  const site = tinySite();
  const filter = { from: 2005, to: 2026, cats: new Set([0]), includeNoWork: false };
  const ctx: Ctx = {
    site, filter, radiusM: 152, summary: summarizePlaces(site.permits, filter, 2),
    go: () => {}, setRadius: () => {}, showAllKinds: () => {}, retry: () => {},
  };

  it("leads with the summary, then what's nearby", async () => {
    const el = placePanel(ctx, 0);
    await new Promise((r) => setTimeout(r, 0));
    const order = [...el.querySelectorAll(".lead, .nearby, .history")].map((n) => n.className.split(" ")[0]);
    expect(order).toEqual(["lead", "nearby", "history"]);
    expect(el.querySelector(".lead")!.textContent).toBe("2 permits from 2020 to 2024, all Excavation.");
  });

  it("shows each permit as a one-line row that opens to the details", async () => {
    const el = placePanel(ctx, 0);
    await new Promise((r) => setTimeout(r, 0));
    const rows = [...el.querySelectorAll(".history details")];
    expect(rows).toHaveLength(2);
    const first = rows[0]!;
    expect(first.querySelector("summary")!.textContent).toContain("Work P1");
    expect(first.querySelector("summary")!.textContent).toContain("May 6, 2024");
    expect(first.textContent).toContain("Filed as 100 E A ST");
  });
});

describe("when something goes wrong", () => {
  const filter = { from: 2005, to: 2026, cats: new Set([0]), includeNoWork: false };

  it("offers every kind of work", () => {
    const site = tinySite();
    let shown = 0;
    const none = { ...filter, cats: new Set<number>() };
    const ctx: Ctx = {
      site, filter: none, radiusM: 152, summary: summarizePlaces(site.permits, none, 2),
      go: () => {}, setRadius: () => {}, showAllKinds: () => shown++, retry: () => {},
    };
    const button = [...placePanel(ctx, 0).querySelectorAll("button")].find((b) => b.textContent === "Show all kinds of work");
    button!.click();
    expect(shown).toBe(1);
  });

  it("offers to try again when the permits don't load", async () => {
    const site = { ...tinySite(), detail: async () => { throw new Error("offline"); } };
    let retried = 0;
    const ctx: Ctx = {
      site, filter, radiusM: 152, summary: summarizePlaces(site.permits, filter, 2),
      go: () => {}, setRadius: () => {}, showAllKinds: () => {}, retry: () => retried++,
    };
    const el = placePanel(ctx, 0);
    await new Promise((r) => setTimeout(r, 0));
    expect(el.textContent).toContain("offline");
    [...el.querySelectorAll("button")].find((b) => b.textContent === "Try again")!.click();
    expect(retried).toBe(1);
  });
});
