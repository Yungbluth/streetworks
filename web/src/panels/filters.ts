import { errorMessage, type Site } from "../data";
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

// plain descriptions of the city's permit types, as the map groups them
const kindMeanings: [string, string][] = [
  ["Excavation", "Digging into the street or sidewalk, such as a trench for a pipe or cable."],
  ["Barricade", "Closing part of a street or sidewalk with barricades, without digging."],
  ["Combo", "Digging and a closure under one permit."],
  ["Utility", "Work in the street by a gas, water, power or telecom company."],
  ["Right-of-way", "Other work in the public right-of-way, such as a driveway or sidewalk."],
  ["Annual", "One permit covering a year of routine work, usually held by a utility or contractor."],
  ["Small wireless", "Small cell antennas and their equipment on poles."],
  ["Civic event", "Closing a street for an event, such as a parade or a race."],
  ["Other", "Everything else, such as private improvement agreements."],
];

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
    h("p", { class: "quiet" }, "A dot's color is the kind of work filed there most often. Bigger dots mean more permits."),
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
    h(
      "details",
      { class: "kinds" },
      h("summary", {}, "What the kinds mean"),
      h(
        "dl",
        {},
        ...kindMeanings
          .filter(([name]) => site.index.categories.includes(name))
          .flatMap(([name, meaning]) => [h("dt", {}, name), h("dd", {}, meaning)]),
      ),
      h("p", { class: "quiet" }, "Civic event and Small wireless start off: they're mostly street closures and equipment on poles, not digging."),
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
                    status.textContent = ` Couldn't load it: ${errorMessage(err)}`;
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
