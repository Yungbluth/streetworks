import type { Site } from "../data";
import { h } from "../dom";
import { num } from "../format";
import { matcher, type Filter } from "../filters";
import { paint } from "../paint";

function countByCategory(site: Site, f: Filter): number[] {
  const ok = matcher(site.permits, { ...f, cats: new Set(site.index.categories.map((_, i) => i)) });
  const out = new Array<number>(site.index.categories.length).fill(0);
  for (let i = 0; i < site.permits.cat.length; i++) if (ok(i)) out[site.permits.cat[i]!]!++;
  return out;
}

export interface FilterEvents {
  setCategory(c: number, on: boolean): void;
  setNoWork(on: boolean): void;
  setOverlay(name: string, on: boolean): Promise<void>;
}

export function filtersPanel(site: Site, f: Filter, overlaysOn: Set<string>, ev: FilterEvents): HTMLElement {
  const counts = countByCategory(site, f);
  const check = (c: number) => (e: Event) => ev.setCategory(c, (e.target as HTMLInputElement).checked);
  return h(
    "section",
    { class: "filters" },
    h("h3", {}, "Kinds of work"),
    h(
      "div",
      { class: "cats" },
      ...site.index.categories.map((name, c) =>
        h(
          "label",
          { class: "cat", style: `--paint: ${paint(name)}` },
          h("input", { type: "checkbox", checked: f.cats.has(c), onchange: check(c) }),
          h("span", { class: "name" }, name),
          h("span", { class: "n" }, num(counts[c]!)),
        ),
      ),
    ),
    h(
      "label",
      { class: "toggle" },
      h("input", { type: "checkbox", checked: f.includeNoWork, onchange: (e) => ev.setNoWork((e.target as HTMLInputElement).checked) }),
      "Include permits that never led to work: voided, withdrawn, denied, or still in review",
    ),
    site.index.overlays.length
      ? h(
          "div",
          { class: "overlays" },
          h("h3", {}, "Reference layers"),
          ...site.index.overlays.map((o) => {
            const status = h("span", { class: "error", role: "status" });
            return h(
              "label",
              { class: "toggle" },
              h("input", {
                type: "checkbox",
                checked: overlaysOn.has(o.name),
                disabled: o.rows === 0,
                onchange: (e) => {
                  status.textContent = "";
                  ev.setOverlay(o.name, (e.target as HTMLInputElement).checked).catch((err: unknown) => {
                    status.textContent = ` Couldn't load it: ${err instanceof Error ? err.message : String(err)}`;
                  });
                },
              }),
              o.label,
              h("span", { class: "n" }, o.rows ? ` ${num(o.rows)} segments` : " no rows downloaded"),
              status,
            );
          }),
        )
      : null,
  );
}
