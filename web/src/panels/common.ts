import { errorMessage } from "../data";
import { h } from "../dom";
import { num } from "../format";
import { swatch } from "../marks";
import { paint } from "../paint";
import type { TableStyle } from "./table";

export function panelHead(title: string, onClear: () => void): HTMLElement {
  return h(
    "div",
    { class: "panel-head" },
    h("h2", { tabindex: "-1" }, title),
    h("button", { type: "button", class: "linkish map-jump", "aria-label": "Show the map", onclick: showMap }, "Map"),
    h("button", { type: "button", class: "linkish", onclick: onClear }, "Clear"),
  );
}

function showMap() {
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  document.getElementById("map")?.scrollIntoView({ block: "start", behavior: reduce ? "auto" : "smooth" });
}

export const note = (text?: string) => (text ? h("p", { class: "note" }, text) : null);

export const loading = () => h("p", { class: "quiet" }, "Loading permits…");

export const failed = (what: string, e: unknown, retry?: () => void) =>
  h(
    "div",
    { class: "error", role: "alert" },
    h("p", {}, `Couldn't load ${what}. ${errorMessage(e)}`),
    retry ? h("button", { type: "button", class: "more", onclick: retry }, "Try again") : null,
  );

export const showAll = (onClick: () => void) =>
  h("button", { type: "button", class: "more", onclick: onClick }, "Show all kinds of work");

/** horizontal bars, such as permits per category */
export function bars(rows: { label: string; n: number; color?: string }[]): HTMLElement {
  const max = Math.max(1, ...rows.map((r) => r.n));
  return h(
    "ul",
    { class: "bars" },
    ...rows.map((r) =>
      h(
        "li",
        {},
        h("span", { class: "label" }, r.label),
        h("span", { class: "track" }, h("i", { style: `width: ${Math.round((100 * r.n) / max)}%; ${r.color ? `--bar: ${r.color}` : ""}` })),
        h("span", { class: "n" }, num(r.n)),
      ),
    ),
  );
}

export function categoryBars(counts: Map<number, number>, categories: string[]): HTMLElement {
  return bars(
    [...counts]
      .sort((a, b) => b[1] - a[1])
      .map(([c, n]) => ({ label: categories[c]!, n, color: paint(categories[c]!) })),
  );
}

const slug = (name: string) => name.toLowerCase().replace(/[^a-z]+/g, "-").replace(/^-|-$/g, "");

export function kindSplit(counts: Map<number, number>, categories: string[]): HTMLElement {
  const rows = [...counts].sort((a, b) => b[1] - a[1]);
  const total = rows.reduce((a, [, n]) => a + n, 0) || 1;
  return h(
    "div",
    { class: "split" },
    h(
      "div",
      { class: "split-bar", "aria-hidden": "true" },
      ...rows.map(([c, n]) => h("span", { class: "seg", style: `flex-grow: ${n / total}; --seg: var(--paint-${slug(categories[c]!)})` })),
    ),
    h(
      "ul",
      { class: "split-key" },
      ...rows.map(([c, n]) => h("li", {}, swatch(categories[c]!, 10), `${categories[c]} `, h("span", { class: "n" }, num(n)))),
    ),
  );
}

export const tableStyle = (first: TableStyle["first"]): TableStyle => ({ first, button: "more", link: "linkish" });
