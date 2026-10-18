// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { PlaceKind, type Site } from "../data";
import { summarizePlaces } from "../filters";
import type { Ctx } from "../model";
import { aboutPanel } from "./about";
import { streetPanel } from "./street";

const site = {
  index: { categories: ["Civic event", "Excavation", "Small wireless"], overlays: [] } as unknown as Site["index"],
  places: { streets: ["E A ST"], kind: [PlaceKind.Block, PlaceKind.Block], street: [0, 0], cross: [-1, -1], block: [0, 100], lat: [3222000, 3222000], lng: [-11097000, -11096900], label: {} },
  permits: { place: [0, 1], year: [2020, 2021], cat: [1, 1], noWork: [0, 0], lat: [3222000, 3222000], lng: [-11097000, -11096900] },
  detail: async () => ({}),
} as Site;
const filter = { from: 2005, to: 2026, cats: new Set([1]), includeNoWork: false };
const quality = { unique_permits: 2, no_block: 0, unplaced: 0, no_date: 0, no_work: 0, date_min: "2020-01-01", date_max: "2021-01-01" };

describe("kinds of work, in About", () => {
  const text = () => aboutPanel({ ...site.index, sparseBefore: 2005, sources: [], quality } as unknown as Site["index"]).textContent ?? "";
  it("says what a dot's color and size mean", () => {
    expect(text()).toContain("A dot's color is the kind of work filed there most often");
  });
  it("explains each kind, and why two start off", () => {
    expect(text()).toContain("What the kinds mean");
    expect(text()).toContain("Civic event and Small wireless start off");
  });
});

describe("a whole street", () => {
  it("is a profile of its blocks", () => {
    const ctx: Ctx = {
      site, filter, radiusM: 152, summary: summarizePlaces(site.permits, filter, 2),
      go: () => {}, setRadius: () => {}, showAllKinds: () => {}, retry: () => {},
    };
    const labels = [...streetPanel(ctx, 0).querySelectorAll(".station")].map((l) => l.getAttribute("aria-label"));
    expect(labels).toEqual(["0 block, 1 permit", "100 block, 1 permit"]);
    expect(streetPanel(ctx, 0).textContent).toContain("2 permits on 2 blocks.");
    expect(streetPanel(ctx, 0).textContent).toContain("Busiest: the 0 block, 1 permit of 2.");
  });
});
