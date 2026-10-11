import type { Places } from "../data";
import { h } from "../dom";
import { feet, num, placeName, prettyStreet } from "../format";
import type { Focus } from "../model";
import type { Found } from "../search";
import { withoutDirection } from "../street";

export function describe(
  places: Places,
  f: Found,
  q: string,
  /** A street's permits under the page's current filters, so the count matches what opens. */
  countOf?: (street: number) => number,
): { label: string; detail: string; focus: Focus } {
  const street = (i: number) => prettyStreet(places.streets[i] ?? "");
  switch (f.kind) {
    case "address":
      return {
        label: placeName(places, f.place),
        detail: f.exact ? "Address" : `Nearest block with permits to the ${f.wanted} block`,
        focus: {
          kind: "place",
          place: f.place,
          q,
          note: f.exact ? undefined : `No permits are on record for the ${f.wanted} block, so this is the nearest block that has some.`,
        },
      };
    case "street": {
      const n = countOf ? countOf(f.street) : f.permits;
      const detail = `Whole street, ${num(n)} ${n === 1 ? "permit" : "permits"}`;
      return { label: street(f.street), detail, focus: { kind: "street", street: f.street, q } };
    }
    case "intersection": {
      const title = `${street(f.streets[0])} and ${street(f.streets[1])}`;
      const detail = f.exact ? "Where they cross" : `These streets don't cross. Their closest blocks are ${feet(f.gapM)} apart.`;
      if (f.place !== undefined) return { label: title, detail, focus: { kind: "place", place: f.place, q } };
      return {
        label: title,
        detail,
        focus: { kind: "spot", at: f.at, title: f.exact ? `Near ${title}` : `Between ${title}`, note: f.exact ? undefined : detail, q },
      };
    }
  }
}

/** The line above the list when two cross streets never meet. */
export function apartMessage(places: Places, apart: { sides: [number, number]; gapM: number }): string {
  const [a, b] = apart.sides.map((i) => prettyStreet(withoutDirection(places.streets[i] ?? "")));
  return `${a} and ${b} don't meet. At their closest they're ${feet(apart.gapM)} apart.`;
}

const PAUSE_MS = 400;
// "1200", "1200 E", "1200-1250 n.": a number, maybe a direction, no street yet
const numberOnly = /^\d{1,6}(?:\s*[-–]\s*\d{1,6})?(?:\s+[nsew]\.?)?\s*$/i;

const zip = /^\s*\d{5}(?:-\d{4})?\s*$/;

export function emptyMessage(q: string): string {
  const forms = "Try an address like 1200 E Speedway, a street name, or two cross streets like Speedway & Campbell.";
  if (zip.test(q)) return `Search doesn't know ZIP codes. ${forms}`;
  return `No street with permits matches that. Search knows street names, not places or businesses. ${forms}`;
}

export function searchBox(
  places: Places,
  run: (q: string) => Found[],
  choose: (focus: Focus) => void,
  countOf?: (street: number) => number,
): HTMLElement {
  const listId = "search-results";
  const input = h("input", {
    type: "search", id: "q", autocomplete: "off", spellcheck: "false",
    placeholder: "Address, street or cross streets",
    role: "combobox", "aria-expanded": "false", "aria-controls": listId, "aria-autocomplete": "list",
    "aria-label": "Search for an address, a street, or two cross streets",
  });
  const list = h("ul", { id: listId, role: "listbox", class: "results", hidden: true });
  const empty = h("p", { class: "results-empty", hidden: true, role: "status" });
  // said once above the list, rather than under every result
  const apart = h("p", { class: "results-note", hidden: true, role: "status" });
  let results: ReturnType<typeof describe>[] = [];
  let emptyTimer: number | undefined;
  let active = -1;

  const close = () => {
    clearTimeout(emptyTimer);
    list.hidden = true;
    empty.hidden = true;
    apart.hidden = true;
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
    active = -1;
  };
  const highlight = (i: number) => {
    active = i;
    [...list.children].forEach((li, j) => li.setAttribute("aria-selected", String(j === i)));
    if (i >= 0) input.setAttribute("aria-activedescendant", `result-${i}`);
  };
  const pick = (i: number) => {
    const r = results[i];
    if (!r) return;
    close();
    input.value = "";
    choose(r.focus);
  };
  const update = () => {
    const q = input.value.trim();
    if (!q) return close();
    const found = run(q);
    results = found.map((f) => describe(places, f, q, countOf));
    const first = found[0];
    const gap = first?.kind === "street" ? first.apart : undefined;
    apart.hidden = !gap;
    apart.textContent = gap ? apartMessage(places, gap) : "";
    list.replaceChildren(
      ...results.map((r, i) =>
        h(
          "li",
          { id: `result-${i}`, role: "option", "aria-selected": "false", onmousedown: (e) => (e.preventDefault(), pick(i)) },
          h("span", { class: "label" }, r.label),
          h("span", { class: "detail" }, r.detail),
        ),
      ),
    );
    list.hidden = !results.length;
    input.setAttribute("aria-expanded", String(!!results.length));
    highlight(-1);
    // Most queries are mid-word, so "nothing matches" waits for a pause
    clearTimeout(emptyTimer);
    empty.hidden = true;
    if (results.length) return;
    emptyTimer = window.setTimeout(() => {
      const message = numberOnly.test(q) && !zip.test(q) ? "Now the street, like 1200 E Speedway." : q.length >= 3 ? emptyMessage(q) : "";
      empty.textContent = message;
      empty.hidden = !message;
    }, PAUSE_MS);
  };

  input.addEventListener("input", update);
  input.addEventListener("focus", () => input.value.trim() && update());
  input.addEventListener("blur", () => setTimeout(close, 100));
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      if (list.hidden) update();
      if (!results.length) return;
      e.preventDefault();
      const step = e.key === "ArrowDown" ? 1 : -1;
      highlight((active + step + results.length) % results.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (list.hidden) update();
      pick(active >= 0 ? active : 0);
    } else if (e.key === "Escape") {
      close();
    }
  });

  // The note, the list and the empty message float together under the field
  return h("div", { class: "search", role: "search" }, input, h("div", { class: "dropdown" }, apart, list, empty));
}
