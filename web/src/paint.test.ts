// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from "vitest";
import { edgeFor } from "./paint";

const root = document.documentElement.style;
afterEach(() => root.cssText = "");

describe("edgeFor", () => {
  it("outlines white paint with the shared edge colour by default", () => {
    root.setProperty("--paint-edge", "#55585d");
    expect(edgeFor("Excavation")).toBe("#55585d");
    expect(edgeFor("Combo")).toBeUndefined();
  });

  it("lets a stylesheet outline any kind", () => {
    root.setProperty("--paint-edge", "#55585d");
    root.setProperty("--paint-combo-edge", "#27358a");
    root.setProperty("--paint-excavation-edge", "none");
    expect(edgeFor("Combo")).toBe("#27358a");
    expect(edgeFor("Excavation")).toBeUndefined();
    expect(edgeFor("Small wireless")).toBeUndefined();
  });
});
