import { errorMessage, type Site } from "../data";
import { h } from "../dom";
import { num } from "../format";

// plain descriptions of the city's permit types, as the map groups them
export const kindMeanings: [string, string][] = [
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
  setOverlay(name: string, on: boolean): Promise<void>;
}

export function filtersPanel(site: Site, overlaysOn: Set<string>, ev: FilterEvents): HTMLElement | null {
  if (!site.index.overlays.length) return null;
  return h(
    "section",
    { class: "filters overlays" },
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
  );
}
