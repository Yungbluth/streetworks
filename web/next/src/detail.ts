import type L from "leaflet";
import { errorMessage, type Detail } from "../../src/data";
import { h } from "../../src/dom";
import { permitsMatching } from "../../src/filters";
import { dateText, feet, num, placeName, plural } from "../../src/format";
import { distanceMeters, type LatLng } from "../../src/geo";
import { byDateDesc, detailsFor, permitAt, permitsOf, placeAt, type Ctx } from "../../src/model";
import { placeSummary } from "../../src/panels/place";
import { RADII } from "../../src/url";
import { swatch } from "./marks";

export interface SheetCtx extends Ctx {
  showNotes(): void;
}

const PAGE = 25;

const dateBases: Record<string, string> = {
  DATE_APPROVE: "Dated by approval",
  DATE_START: "Dated by the start of work",
  DATE_ENTERED: "Dated by when it was entered",
};

function head(title: string, onClear?: () => void): HTMLElement {
  return h(
    "div",
    { class: "detail-head" },
    h("h2", { tabindex: "-1" }, title),
    h(
      "div",
      { class: "detail-actions" },
      // Only shown on a phone, where the map is above and out of sight.
      h("button", { type: "button", class: "link map-jump", "aria-label": "Show the map", onclick: showMap }, "Map"),
      onClear ? h("button", { type: "button", class: "link", onclick: onClear }, "Clear") : null,
    ),
  );
}

function showMap() {
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  document.getElementById("plan")?.scrollIntoView({ block: "start", behavior: reduce ? "auto" : "smooth" });
}

const loading = () => h("p", { class: "quiet" }, "Loading permits…");

const failed = (what: string, e: unknown, retry: () => void) =>
  h(
    "div",
    { class: "error", role: "alert" },
    h("p", {}, `Couldn't load ${what}. ${errorMessage(e)}`),
    h("button", { type: "button", class: "btn", onclick: retry }, "Try again"),
  );

const showAll = (ctx: SheetCtx) =>
  ctx.filter.cats.size < ctx.site.index.categories.length
    ? h("button", { type: "button", class: "btn", onclick: ctx.showAllKinds }, "Show all kinds of work")
    : null;

function kindSplit(counts: Map<number, number>, categories: string[]): HTMLElement {
  const rows = [...counts].sort((a, b) => b[1] - a[1]);
  const total = rows.reduce((a, [, n]) => a + n, 0) || 1;
  return h(
    "div",
    { class: "split" },
    h(
      "div",
      { class: "split-bar", "aria-hidden": "true" },
      ...rows.map(([c, n]) =>
        h("span", { class: `seg seg-${slug(categories[c]!)}`, style: `flex-grow: ${n / total}; --seg: var(--paint-${slug(categories[c]!)})` }),
      ),
    ),
    h(
      "ul",
      { class: "split-key" },
      ...rows.map(([c, n]) => h("li", {}, swatch(categories[c]!, 10), `${categories[c]} `, h("span", { class: "n" }, num(n)))),
    ),
  );
}

const slug = (name: string) => name.toLowerCase().replace(/[^a-z]+/g, "-").replace(/^-|-$/g, "");

export interface Entry {
  index: number;
  category: string;
  detail?: Detail;
  // its revision number at its place, oldest first
  delta?: number;
  distanceM?: number;
  placeName?: string;
  onPlace?: () => void;
}

let rowId = 0;

function revisionRows(e: Entry): HTMLTableRowElement[] {
  const d = e.detail;
  const what = d ? d.description || d.subType || d.type : "Details for this permit didn't load.";
  const who = d ? [d.applicant, d.utility].filter(Boolean).join(", ") : "";
  const id = `rev-${++rowId}`;
  const more = h(
    "tr",
    { class: "rev-more", id, hidden: true },
    h(
      "td",
      { colspan: 3 },
      h(
        "dl",
        {},
        h("dt", {}, "Kind"), h("dd", {}, d ? `${e.category}, ${d.status.toLowerCase()}` : e.category),
        ...(who ? [h("dt", {}, "Applicant"), h("dd", {}, who)] : []),
        ...(d?.filed ? [h("dt", {}, "Filed as"), h("dd", {}, d.filed)] : []),
        ...(d?.final ? [h("dt", {}, "Finished"), h("dd", {}, dateText(d.final))] : []),
        ...(e.onPlace && e.placeName ? [h("dt", {}, "At"), h("dd", {}, h("button", { type: "button", class: "link", onclick: e.onPlace }, e.placeName))] : []),
      ),
      d && dateBases[d.dateBasis] ? h("p", { class: "quiet" }, dateBases[d.dateBasis]) : null,
      ...(d?.flags ?? []).map((f) => h("p", { class: "flag" }, f)),
      d
        ? h(
            "p",
            { class: "ids" },
            h("span", { class: "permit-no" }, d.permit),
            d.link ? h("a", { href: d.link, target: "_blank", rel: "noopener", "aria-label": `City record for ${d.permit}` }, "City record") : null,
          )
        : null,
    ),
  );
  const toggle = h(
    "button",
    {
      type: "button", class: "rev-open", "aria-expanded": "false", "aria-controls": id,
      onclick: () => {
        const open = toggle.getAttribute("aria-expanded") !== "true";
        toggle.setAttribute("aria-expanded", String(open));
        more.hidden = !open;
      },
    },
    swatch(e.category, 10),
    h("span", { class: "rev-what" }, what),
    h("span", { class: "rev-kind" }, e.category, d?.noWork ? ", no work done" : ""),
  );
  const row = h(
    "tr",
    { class: d?.noWork ? "rev-row no-work" : "rev-row" },
    h("td", { class: "rev-delta" }, e.distanceM !== undefined ? feet(e.distanceM) : e.delta ? `Δ${e.delta}` : ""),
    h("td", { class: "rev-date" }, d ? dateText(d.date) : ""),
    h("td", { class: "rev-work" }, toggle),
  );
  return [row, more];
}

export function revisionTable(entries: Entry[], firstColumn: "Δ" | "Away"): HTMLElement {
  const body = h("tbody", {});
  const table = h(
    "table",
    { class: "rev-table" },
    h("thead", {}, h("tr", {}, h("th", { scope: "col" }, firstColumn), h("th", { scope: "col" }, "Date"), h("th", { scope: "col" }, "Work"))),
    body,
  );
  const wrap = h("div", { class: "rev-wrap" }, table);
  let shown = 0;
  const more = h("button", { type: "button", class: "btn" });
  const page = () => {
    for (const e of entries.slice(shown, shown + PAGE)) body.append(...revisionRows(e));
    shown = Math.min(entries.length, shown + PAGE);
    const left = entries.length - shown;
    more.textContent = left === 1 ? "Show the last one" : left <= PAGE ? `Show the last ${num(left)}` : `Show ${PAGE} more of ${num(left)}`;
    if (shown >= entries.length) more.remove();
  };
  more.addEventListener("click", page);
  wrap.append(more);
  page();
  return wrap;
}

/** Busiest places in view, as a schedule. */
export function busiestDetail(ctx: SheetCtx, view: L.LatLngBounds): HTMLElement {
  const { places } = ctx.site;
  const { count } = ctx.summary;
  const top: number[] = [];
  for (let p = 0; p < places.kind.length; p++) {
    if (!count[p] || !view.contains([places.lat[p]! / 1e5, places.lng[p]! / 1e5])) continue;
    top.push(p);
    if (top.length > 64) {
      top.sort((a, b) => count[b]! - count[a]!);
      top.length = 10;
    }
  }
  top.sort((a, b) => count[b]! - count[a]!);
  return h(
    "section",
    { class: "detail-body" },
    head("Busiest places in view"),
    h("p", { class: "lead" }, "Open one to read its record, or search for an address above."),
    top.length
      ? h(
          "table",
          { class: "schedule" },
          h("thead", {}, h("tr", {}, h("th", { scope: "col" }, "No."), h("th", { scope: "col" }, "Place"), h("th", { scope: "col", class: "num" }, "Permits"))),
          h(
            "tbody",
            {},
            ...top.slice(0, 10).map((p, i) =>
              h(
                "tr",
                {},
                h("td", { class: "no" }, String(i + 1)),
                h("td", {}, h("button", { type: "button", class: "link", onclick: () => ctx.go({ kind: "place", place: p }, { move: true }) }, placeName(places, p))),
                h("td", { class: "num" }, num(count[p]!)),
              ),
            ),
          ),
        )
      : h("div", { class: "empty" }, h("p", {}, "No permits match the filters in this part of the map."), showAll(ctx)),
  );
}

function nearby(ctx: SheetCtx, center: LatLng, opts: { listNow?: boolean; exclude?: number } = {}): HTMLElement {
  const { site } = ctx;
  const found = permitsMatching(site.permits, ctx.filter, { center, radiusM: ctx.radiusM }).filter((i) => site.permits.place[i] !== opts.exclude);
  const placesHit = new Set(found.map((i) => site.permits.place[i]!));
  const select = h(
    "select",
    { "aria-label": "Distance", onchange: (e) => ctx.setRadius(Number((e.target as HTMLSelectElement).value)) },
    ...RADII.map((r) => h("option", { value: r.m, selected: r.m === ctx.radiusM }, r.label)),
  );
  const other = opts.exclude !== undefined;
  const out = h(
    "section",
    { class: "nearby" },
    h("h3", {}, h("span", {}, other ? "Nearby, within" : "Within"), select),
    h(
      "p",
      { class: "sub" },
      !found.length
        ? `No ${other ? "other " : ""}permits match the filters inside this circle.`
        : `${plural(found.length, other ? "more permit" : "permit")} at ${plural(placesHit.size, other ? "other place" : "place")}.`,
    ),
  );
  if (!found.length) return out;
  if (!other) {
    const cats = new Map<number, number>();
    for (const i of found) cats.set(site.permits.cat[i]!, (cats.get(site.permits.cat[i]!) ?? 0) + 1);
    out.append(kindSplit(cats, site.index.categories));
  }
  const list = h("div", {});
  const show = async () => {
    list.replaceChildren(loading());
    try {
      const details = await detailsFor(site, found);
      const entries = found
        .map((i) => {
          const place = site.permits.place[i]!;
          return {
            index: i,
            category: site.index.categories[site.permits.cat[i]!]!,
            detail: details.get(i),
            distanceM: distanceMeters(center, permitAt(site, i)),
            placeName: placeName(site.places, place),
            onPlace: () => ctx.go({ kind: "place", place }, { move: true }),
          };
        })
        .sort((a, b) => byDateDesc(a.detail, b.detail) || a.distanceM - b.distanceM);
      list.replaceChildren(revisionTable(entries, "Away"));
    } catch (e) {
      list.replaceChildren(failed("the permits here", e, () => void show()));
    }
  };
  if (opts.listNow) void show();
  else list.append(h("button", { type: "button", class: "btn", onclick: () => void show() }, "List them, newest first"));
  out.append(list);
  return out;
}

/** one place: the answer in a sentence, its kinds, what's around it, then its revisions */
export function placeDetail(ctx: SheetCtx, place: number, extraNote?: string): HTMLElement {
  const { site, filter } = ctx;
  const mine = permitsOf(site, place, filter);
  const byCategory = new Map<number, number>();
  for (const i of mine) byCategory.set(site.permits.cat[i]!, (byCategory.get(site.permits.cat[i]!) ?? 0) + 1);

  const history = h("div", { class: "history" }, mine.length ? loading() : null);
  const out = h(
    "section",
    { class: "detail-body" },
    head(placeName(site.places, place), () => ctx.go(null)),
    extraNote ? h("p", { class: "note" }, extraNote) : null,
    h("p", { class: "lead" }, placeSummary(byCategory, mine.map((i) => site.permits.year[i]!), site.index.categories)),
    byCategory.size > 1 ? kindSplit(byCategory, site.index.categories) : null,
    !mine.length ? showAll(ctx) : null,
    mine.length ? h("h3", {}, "Revisions here, newest first") : null,
    history,
    nearby(ctx, placeAt(site, place), { exclude: place }),
  );
  if (mine.length)
    detailsFor(site, mine)
      .then((details) => {
        const oldestFirst = mine
          .map((i) => ({ index: i, category: site.index.categories[site.permits.cat[i]!]!, detail: details.get(i) }))
          .sort((a, b) => byDateDesc(b.detail, a.detail));
        const entries = oldestFirst.map((e, n) => ({ ...e, delta: n + 1 })).reverse();
        history.replaceWith(revisionTable(entries, "Δ"));
      })
      .catch((e: unknown) => history.replaceChildren(failed("this place's permits", e, ctx.retry)));
  return out;
}

/** A point with no place of its own, such as where two streets come closest */
export function spotDetail(ctx: SheetCtx, title: string, at: LatLng, extraNote?: string): HTMLElement {
  return h(
    "section",
    { class: "detail-body" },
    head(title, () => ctx.go(null)),
    extraNote ? h("p", { class: "note" }, extraNote) : null,
    nearby(ctx, at, { listNow: true }),
  );
}
