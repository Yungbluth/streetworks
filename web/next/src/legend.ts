import type { Site } from "../../src/data";
import { h } from "../../src/dom";
import { matcher, type Filter } from "../../src/filters";
import { num } from "../../src/format";
import { swatch } from "./marks";

export interface LegendEvents {
  setCategory(c: number, on: boolean): void;
  setNoWork(on: boolean): void;
  showNotes(): void;
}

export function countByCategory(site: Site, f: Filter): number[] {
  const ok = matcher(site.permits, { ...f, cats: new Set(site.index.categories.map((_, i) => i)) });
  const out = new Array<number>(site.index.categories.length).fill(0);
  for (let i = 0; i < site.permits.cat.length; i++) if (ok(i)) out[site.permits.cat[i]!]!++;
  return out;
}

// the legend: each kind of work in its paint, with how many permits it has in the chosen years
export function legend(site: Site, f: Filter, ev: LegendEvents): HTMLElement {
  const counts = countByCategory(site, f);
  const on = f.cats.size;
  const all = site.index.categories.length;
  // Busiest kinds first, so the colours that fill the map lead the key
  const order = site.index.categories.map((_, c) => c).sort((a, b) => counts[b]! - counts[a]!);
  return h(
    "div",
    { class: "cell legend" },
    h("h2", { class: "cell-label" }, "Legend: kind of work", h("span", { class: "cell-count" }, on === all ? `all ${all} shown` : `${on} of ${all} shown`)),
    h(
      "ul",
      { class: "kinds" },
      ...order.map((c) => {
        const name = site.index.categories[c]!;
        const pressed = f.cats.has(c);
        return h(
          "li",
          {},
          h(
            "button",
            {
              type: "button", class: "kind", "aria-pressed": String(pressed),
              title: pressed ? `Hide ${name}` : `Show ${name}`,
              onclick: () => ev.setCategory(c, !pressed),
            },
            swatch(name),
            h("span", { class: "kind-name" }, name),
            h("span", { class: "kind-n" }, num(counts[c]!)),
          ),
        );
      }),
    ),
    h(
      "div",
      { class: "legend-foot" },
      h(
        "label",
        { class: "check" },
        h("input", { type: "checkbox", checked: f.includeNoWork, onchange: (e) => ev.setNoWork((e.target as HTMLInputElement).checked) }),
        "Include permits that never led to work",
      ),
      h("button", { type: "button", class: "link", onclick: ev.showNotes }, "What the kinds mean"),
    ),
  );
}
