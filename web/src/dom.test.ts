// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { h } from "./dom";

describe("h", () => {
  it("treats strings as text", () => {
    const p = h("p", {}, "<b>x</b>");
    expect(p.textContent).toBe("<b>x</b>");
    expect(p.querySelector("b")).toBeNull();
  });

  it("sets attributes, classes and listeners", () => {
    let clicked = 0;
    const b = h("button", { class: "linkish", type: "button", "aria-label": "Clear", onclick: () => clicked++ }, "Clear", null, false, h("span", {}, "!"));
    b.click();
    expect(clicked).toBe(1);
    expect(b.className).toBe("linkish");
    expect(b.getAttribute("aria-label")).toBe("Clear");
    expect(b.textContent).toBe("Clear!");
  });

});
