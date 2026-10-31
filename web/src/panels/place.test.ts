// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import type { Api } from "../api";
import { PlaceKind, type Detail, type Site } from "../data";
import { summarizePlaces } from "../filters";
import type { Ctx } from "../model";
import { digSummary, perYear, placePanel, placeSummary, whoFor } from "./place";

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

describe("digSummary", () => {
  const street = "E Speedway Blvd";
  it("says how often a place was dug up, when last", () => {
    expect(digSummary([2010, 2024, 2018, 2015, 2019, 2021, 2022, 2012, 2016, 2013, 2011], { typical: 5, street })).toBe(
      "Dug up 11 times, most recently in 2024, about twice the typical block on E Speedway Blvd (5 digs).",
    );
    expect(digSummary([2019, 2020], { typical: 2, street })).toBe("Dug up twice, most recently in 2020, about the same as the typical block on E Speedway Blvd (2 digs).");
    expect(digSummary([2019], { typical: 4, street })).toBe("Dug up once, in 2019, fewer than the typical block on E Speedway Blvd (4 digs).");
    expect(digSummary([2019, 2020, 2021], { typical: 0, street })).toBe("Dug up 3 times, most recently in 2021; most blocks on E Speedway Blvd haven't been dug up at all.");
  });
});

describe("whoFor", () => {
  const d = (utility?: string, applicant?: string) => ({ utility, applicant }) as Detail;
  it("names the two it was most often filed for", () => {
    const ds = [d("Southwest Gas"), d("Southwest Gas"), d(undefined, "Tucson Water"), d("Southwest Gas", "Acme Paving"), d("Tucson Water")];
    expect(whoFor(ds)).toBe("Filed mostly for Southwest Gas (3) and Tucson Water (2).");
  });
});

describe("perYear", () => {
  it("counts permits in each year of the range, zeros included", () => {
    expect(perYear([2020, 2022, 2022, 2030, 0], 2019, 2023)).toEqual([0, 1, 0, 2, 0]);
  });
});

// An API that answers records with the given function and nothing else
const fakeApi = (records: Api["records"]): Api => ({
  records,
  byNumber: () => Promise.reject(new Error("not used here")),
  search: async () => [],
});

const recordsFrom = (details: Detail[]) =>
  fakeApi(async (ids) => new Map(ids.flatMap((i) => (details[i] ? [[i, details[i]] as const] : []))));

function tinySite(details = tinyDetails()): Site {
  return {
    index: { categories: ["Excavation"], built: "b" } as Site["index"],
    places: { streets: ["E A ST"], kind: [PlaceKind.Block, PlaceKind.Block], street: [0, 0], cross: [-1, -1], block: [100, 200], lat: [3222000, 3222000], lng: [-11097000, -11096970], label: {} },
    permits: { place: [0, 0, 1], year: [2020, 2024, 2022], cat: [0, 0, 0], noWork: [0, 0, 0], lat: [3222000, 3222000, 3222000], lng: [-11097000, -11097000, -11096970] },
    api: recordsFrom(details),
  };
}

function tinyDetails(): Detail[] {
  const detail = (permit: string, date: string): Detail => ({
    permit, type: "EXCAVATION", status: "CLOSED", noWork: false, date, dateBasis: "DATE_ISSUED", description: `Work ${permit}`, filed: "100 E A ST",
  });
  return [detail("P0", "2020-01-02"), detail("P1", "2024-05-06"), detail("P2", "2022-03-04")];
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
    const order = [...el.querySelectorAll(".lead, .nearby, .permit-wrap")].map((n) => n.className.split(" ")[0]);
    // What's around it comes before the full history, which can run long
    expect(order).toEqual(["lead", "nearby", "permit-wrap"]);
    expect(el.querySelector(".lead")!.textContent).toBe("2 permits from 2020 to 2024, all Excavation.");
  });

  it("leads with how often it was dug up, against the rest of its street", () => {
    const el = placePanel(ctx, 0);
    expect(el.querySelector(".digs")!.textContent).toBe("Dug up twice, most recently in 2024, about the same as the typical block on E A St (2 digs).");
  });

  it("draws its permits per year across the chosen years", () => {
    const el = placePanel(ctx, 0);
    const bars = el.querySelectorAll(".place-years .bar");
    expect(bars).toHaveLength(22);
    expect(el.querySelector(".place-years")!.getAttribute("aria-label")).toBe("Permits per year, 2005 to 2026: 1 in 2020, 1 in 2024.");
  });

  it("says who the work was filed for, once the records load", async () => {
    const named = tinySite(tinyDetails().map((d) => ({ ...d, utility: "Tucson Water" })));
    const el = placePanel({ ...ctx, site: named }, 0);
    await new Promise((r) => setTimeout(r, 0));
    expect(el.querySelector(".who")!.textContent).toBe("All filed for Tucson Water.");
    const plain = placePanel(ctx, 0);
    await new Promise((r) => setTimeout(r, 0));
    expect(plain.querySelector(".permit-row")).not.toBeNull();
    expect(plain.querySelector(".who:not([hidden])")).toBeNull();
  });

  it("shows each permit as a table line", async () => {
    const el = placePanel(ctx, 0);
    await new Promise((r) => setTimeout(r, 0));
    const rows = [...el.querySelectorAll(".permit-row")];
    expect(rows).toHaveLength(2);
    const first = rows[0]!;
    expect(first.querySelector(".permit-date")!.textContent).toBe("May 6, 2024");
    expect(first.querySelector(".permit-open")!.textContent).toContain("Work P1");
    expect(first.nextElementSibling!.textContent).toContain("100 E A ST");
  });
});

describe("a place opened at one of its permits", () => {
  const site = tinySite();
  const filter = { from: 2005, to: 2026, cats: new Set([0]), includeNoWork: false };
  const ctx: Ctx = {
    site, filter, radiusM: 152, summary: summarizePlaces(site.permits, filter, 2),
    go: () => {}, setRadius: () => {}, showAllKinds: () => {}, retry: () => {},
  };

  it("picks out that permit's row", async () => {
    const el = placePanel(ctx, 0, undefined, undefined, { permit: 0, land: false });
    await new Promise((r) => setTimeout(r, 0));
    const picked = el.querySelectorAll('tr[aria-current="true"]');
    expect(picked).toHaveLength(1);
    expect(picked[0]!.textContent).toContain("Work P0");
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
    const site = { ...tinySite(), api: fakeApi(async () => { throw new Error("offline"); }) };
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
