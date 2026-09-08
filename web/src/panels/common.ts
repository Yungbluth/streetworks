import { h } from "../dom";
import { num } from "../format";
import { paint } from "../paint";

export function panelHead(title: string, onClear: () => void): HTMLElement {
  return h(
    "div",
    { class: "panel-head" },
    h("h2", { tabindex: "-1" }, title),
    h("button", { type: "button", class: "linkish", onclick: onClear }, "Clear"),
  );
}

export const note = (text?: string) => (text ? h("p", { class: "note" }, text) : null);

export const loading = () => h("p", { class: "quiet" }, "Loading permits…");

export const failed = (what: string, e: unknown) =>
  h("p", { class: "error" }, `Couldn't load ${what}. ${e instanceof Error ? e.message : String(e)}`);

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
