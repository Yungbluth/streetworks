// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { PlaceKind, type Site } from "../data";
import { summarizePlaces } from "../filters";
import type { Ctx } from "../model";
import { filtersPanel } from "./filters";
import { streetPanel } from "./street";

const site = {
  index: { categories: ["Civic event", "Excavation", "Small wireless"], overlays: [] } as unknown as Site["index"],
  places: { streets: ["E A ST"], kind: [PlaceKind.Block, PlaceKind.Block], street: [0, 0], cross: [-1, -1], block: [0, 100], lat: [3222000, 3222000], lng: [-11097000, -11096900], label: {} },
  permits: { place: [0, 1], year: [2020, 2021], cat: [1, 1], noWork: [0, 0], lat: [3222000, 3222000], lng: [-11097000, -11096900] },
  detail: async () => ({}),
} as Site;
const filter = { from: 2005, to: 2026, cats: new Set([1]), includeNoWork: false };
const noop = { setCategory() {}, setNoWork() {}, async setOverlay() {} };

describe("kinds of work", () => {
  const text = () => filtersPanel(site, filter, new Set(), noop).textContent ?? "";
  it("says what a dot's color and size mean", () => {
    expect(text()).toContain("A dot's color is the kind of work filed there most often");
  });
  it("explains each kind, and why two start off", () => {
    expect(text()).toContain("What the kinds mean");
    expect(text()).toContain("Civic event and Small wireless start off");
  });
});

describe("street rows", () => {
  it("name blocks as blocks", () => {
    const ctx: Ctx = {
      site, filter, radiusM: 152, summary: summarizePlaces(site.permits, filter, 2),
      go: () => {}, setRadius: () => {}, showAllKinds: () => {}, retry: () => {},
    };
    const labels = [...streetPanel(ctx, 0).querySelectorAll(".label")].map((l) => l.textContent);
    expect(labels).toEqual(["0 block", "100 block"]);
    expect(streetPanel(ctx, 0).textContent).toContain("2 permits on 2 blocks, in address order.");
  });
});
