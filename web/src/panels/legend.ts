import type { Site } from "../data";
import { h } from "../dom";
import { matcher, type Filter } from "../filters";
import { num, plural } from "../format";
import { strokeIcon, swatch } from "../marks";

export interface LegendEvents {
  setCategory(c: number, on: boolean): void;
  setNoWork(on: boolean): void;
  /** Where "What the kinds mean" leads. */
  showMeanings(): void;
  /** If given, a picker that shows one kind alone. */
  showOnly?(c: number): void;
  /** if given, Reset sits in the legend's head; it's hidden while there's nothing to reset */
  reset?: { visible: boolean; go(): void };
  moreOpen?: boolean;
}

// permits of each kind under the years and no-work choice, ignoring which kinds are on
export function countByCategory(site: Site, f: Filter): number[] {
  const ok = matcher(site.permits, { ...f, cats: new Set(site.index.categories.map((_, i) => i)) });
  const out = new Array<number>(site.index.categories.length).fill(0);
  for (let i = 0; i < site.permits.cat.length; i++) if (ok(i)) out[site.permits.cat[i]!]!++;
  return out;
}

/** The legend: each kind of work in its paint, with how many permits it has in the chosen years */
export function legend(site: Site, f: Filter, ev: LegendEvents, linkClass = "link"): HTMLElement {
  const counts = countByCategory(site, f);
  const on = f.cats.size;
  const all = site.index.categories.length;
  // Busiest kinds first, so the colours that fill the map lead the key.
  const order = site.index.categories.map((_, c) => c).sort((a, b) => counts[b]! - counts[a]!);
  const shown = on === all ? `all ${all} shown` : `${on} of ${all} shown`;
  const label = h("h2", { class: "cell-label", "aria-label": `Legend: kind of work, ${shown}` }, "Legend: kind of work", h("span", { class: "cell-count" }, shown));
  const reset = ev.reset
    ? h("button", {
        type: "button", class: `${linkClass} reset${ev.reset.visible ? "" : " off"}`,
        "aria-hidden": ev.reset.visible ? undefined : "true", tabindex: ev.reset.visible ? undefined : "-1",
        onclick: ev.reset.go,
      }, "Reset filters")
    : null;
  const only = ev.showOnly
    ? h(
        "select",
        {
          class: "only", "aria-label": "Show only one kind",
          onchange: (e: Event) => {
            const c = site.index.categories.indexOf((e.target as HTMLSelectElement).value);
            if (c >= 0) ev.showOnly!(c);
          },
        },
        h("option", { value: "" }, "Show only…"),
        ...order.map((c) => {
          const name = site.index.categories[c]!;
          return h("option", { value: name, selected: on === 1 && f.cats.has(c) }, name);
        }),
      )
    : null;
  const noWork = h(
    "label",
    { class: "check" },
    h("input", { type: "checkbox", checked: f.includeNoWork, onchange: (e) => ev.setNoWork((e.target as HTMLInputElement).checked) }),
    "Include permits that never led to work",
  );
  return h(
    "div",
    { class: "cell legend" },
    reset ? h("div", { class: "legend-head" }, label, reset) : label,
    h("p", { class: "legend-hint" }, "Select a kind to show or hide it. Bigger dots, more permits."),
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
              "aria-label": `${name}, ${plural(counts[c]!, "permit")}`,
              title: pressed ? `Hide ${name}` : `Show ${name}`,
              onclick: () => ev.setCategory(c, !pressed),
            },
            swatch(name),
            h("span", { class: "kind-name" }, name),
            h("span", { class: "kind-n" }, num(counts[c]!)),
            pressed ? strokeIcon("check") : h("span", { class: "check-slot", "aria-hidden": "true" }),
          ),
        );
      }),
    ),
    h(
      "div",
      { class: "legend-foot" },
      only
        ? h(
            "details",
            { class: "legend-more", open: !!ev.moreOpen },
            h("summary", {}, "More options"),
            h("div", { class: "legend-more-body" }, only, noWork),
          )
        : noWork,
      h("button", { type: "button", class: linkClass, onclick: ev.showMeanings }, "What the kinds mean"),
    ),
  );
}
