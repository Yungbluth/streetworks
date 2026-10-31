import type { PermitHit } from "../api";
import type { Places } from "../data";
import { fill, h } from "../dom";
import { feet, num, placeName, plural, prettyStreet } from "../format";
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

const PERMIT_PAUSE_MS = 250;

const permitSearchable = (q: string) => (q.match(/[\p{L}\p{N}]/gu)?.length ?? 0) >= 2 && [...q].length <= 100;

/** a permit hit's snippet is escaped text with <mark> around the matches */
function snippet(html: string): Node[] {
  const parsed = new DOMParser().parseFromString(html, "text/html").body.childNodes;
  return [...parsed].flatMap((n) => {
    if (n.nodeType === Node.TEXT_NODE) return [document.createTextNode(n.textContent ?? "")];
    if (n.nodeType !== Node.ELEMENT_NODE) return [];
    const text = n.textContent ?? "";
    return [n.nodeName === "MARK" ? h("mark", {}, text) : document.createTextNode(text)];
  });
}

export function searchBox(
  places: Places,
  run: (q: string) => Found[],
  choose: (focus: Focus) => void,
  countOf?: (street: number) => number,
  permits?: (q: string, signal: AbortSignal) => Promise<PermitHit[]>,
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
  // permits arrive after the places, so a screen reader is told when they do
  const permitsFound = h("p", { class: "visually-hidden", role: "status" });
  const permitsDown = h("p", { class: "results-note", hidden: true, role: "status" });
  let results: ReturnType<typeof describe>[] = [];
  let hits: PermitHit[] = [];
  // The query the permit hits are for, or are being asked for
  let hitsFor: string | undefined;
  let asking: AbortController | undefined;
  let permitTimer: number | undefined;
  let choices: Focus[] = [];
  let emptyTimer: number | undefined;
  let active = -1;

  const close = () => {
    clearTimeout(emptyTimer);
    // an answer still to come would open the list again, so it's dropped and asked for again later
    clearTimeout(permitTimer);
    if (asking) {
      asking.abort();
      asking = undefined;
      hitsFor = undefined;
    }
    list.hidden = true;
    empty.hidden = true;
    apart.hidden = true;
    permitsDown.hidden = true;
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
    active = -1;
  };
  const options = () => [...list.querySelectorAll<HTMLElement>("[role=option]")];
  const highlight = (i: number) => {
    active = i;
    options().forEach((li, j) => li.setAttribute("aria-selected", String(j === i)));
    if (i < 0) return input.removeAttribute("aria-activedescendant");
    input.setAttribute("aria-activedescendant", `result-${i}`);
    options()[i]?.scrollIntoView({ block: "nearest" });
  };
  const pick = (i: number) => {
    const f = choices[i];
    if (!f) return;
    close();
    input.value = "";
    choose(f);
  };
  const option = (i: number, label: string, ...detail: (Node | string)[]) =>
    h(
      "li",
      { id: `result-${i}`, role: "option", "aria-selected": "false", onmousedown: (e) => (e.preventDefault(), pick(i)) },
      h("span", { class: "label" }, label),
      h("span", { class: "detail" }, ...detail),
    );
  const draw = () => {
    const keep = active < results.length ? active : -1;
    fill(
      list,
      ...results.map((r, i) => option(i, r.label, r.detail)),
      hits.length
        ? h(
            "li",
            { role: "group", class: "permit-hits", "aria-labelledby": "permit-hits-label" },
            h("h4", { id: "permit-hits-label", "aria-hidden": "true" }, "Permits"),
            h(
              "ul",
              { role: "none" },
              ...hits.map((hit, j) => option(results.length + j, hit.year ? `${hit.number}, ${hit.year}` : hit.number, ...snippet(hit.snippet))),
            ),
          )
        : null,
    );
    choices = [...results.map((r) => r.focus), ...hits.map((hit): Focus => ({ kind: "place", place: hit.place, permit: hit.id }))];
    list.hidden = !choices.length;
    input.setAttribute("aria-expanded", String(!!choices.length));
    if (hits.length) empty.hidden = true;
    highlight(keep);
  };
  const askPermits = (q: string) => {
    clearTimeout(permitTimer);
    asking?.abort();
    asking = undefined;
    hits = [];
    hitsFor = q;
    permitsFound.textContent = "";
    permitsDown.hidden = true;
    permitsDown.textContent = "";
    if (!permits || !permitSearchable(q)) return;
    const ctrl = (asking = new AbortController());
    permitTimer = window.setTimeout(() => {
      permits(q, ctrl.signal).then(
        (found) => {
          if (ctrl.signal.aborted) return;
          asking = undefined;
          hits = found;
          draw();
          permitsFound.textContent = found.length ? `${plural(found.length, "permit")} found.` : "";
        },
        () => {
          if (ctrl.signal.aborted) return;
          asking = undefined;
          // Asked again the next time the list opens
          hitsFor = undefined;
          // Set as it happens, so a screen reader hears it
          permitsDown.textContent = "Permit search is unavailable right now.";
          permitsDown.hidden = false;
        },
      );
    }, PERMIT_PAUSE_MS);
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
    if (q !== hitsFor) askPermits(q);
    active = -1;
    draw();
    // Most queries are mid-word, so "nothing matches" waits for a pause
    clearTimeout(emptyTimer);
    empty.hidden = true;
    if (choices.length) return;
    emptyTimer = window.setTimeout(() => {
      if (choices.length) return;
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
      if (!choices.length) return;
      e.preventDefault();
      const step = e.key === "ArrowDown" ? 1 : -1;
      highlight((active + step + choices.length) % choices.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (list.hidden) update();
      pick(active >= 0 ? active : 0);
    } else if (e.key === "Escape") {
      close();
    }
  });

  // the notes, the list and the empty message float together under the field
  return h(
    "div",
    { class: "search", role: "search" },
    input,
    h("div", { class: "dropdown" }, apart, list, permitsDown, empty, permitsFound),
  );
}
