import type L from "leaflet";
import { errorMessage, type Index } from "../../src/data";
import { h } from "../../src/dom";
import { permitsMatching } from "../../src/filters";
import { dateText, num, placeName, plural, prettyStreet } from "../../src/format";
import { distanceMeters, type LatLng } from "../../src/geo";
import { byDateDesc, detailsFor, permitAt, permitsOf, placeAt, type Ctx } from "../../src/model";
import { kindMeanings } from "../../src/panels/filters";
import { placeSummary } from "../../src/panels/place";
import { streetBlocks, streetProfile } from "../../src/panels/street";
import { RADII } from "../../src/url";
import { station, swatch } from "../../src/marks";
import { kindSplit } from "../../src/panels/common";
import { permitTable, type TableStyle } from "../../src/panels/table";

export interface SheetCtx extends Ctx {
  showNotes(): void;
}

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

const tableStyle = (first: "Δ" | "Away"): TableStyle => ({ first, button: "btn", link: "link" });

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
      list.replaceChildren(permitTable(entries, tableStyle("Away")));
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
        history.replaceWith(permitTable(entries, tableStyle("Δ")));
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

export function streetDetail(ctx: SheetCtx, street: number): HTMLElement {
  const { site, summary } = ctx;
  const blocks = streetBlocks(ctx, street);
  const busy = blocks.filter((p) => summary.count[p]! > 0);
  const total = busy.reduce((a, p) => a + summary.count[p]!, 0);
  const name = prettyStreet(site.places.streets[street]!);
  const open = (p: number) => () => ctx.go({ kind: "place", place: p }, { move: true });
  const top = [...busy].sort((a, b) => summary.count[b]! - summary.count[a]!).slice(0, 8);
  return h(
    "section",
    { class: "detail-body" },
    head(name, () => ctx.go(null)),
    h("p", { class: "lead" }, total ? `${plural(total, "permit")} on ${plural(busy.length, "block")} of ${name}.` : `No permits on ${name} match the filters.`),
    total ? null : showAll(ctx),
    h("h3", {}, "Profile, by station"),
    h("p", { class: "quiet" }, "Each station is a hundred-block: 19+00 is the 1900 block. Select one to open it."),
    streetProfile(blocks.map((p) => ({ block: site.places.block[p]!, n: summary.count[p]!, open: open(p) })), station),
    ...(top.length
      ? [
          h("h3", {}, "Busiest blocks"),
          h(
            "table",
            { class: "schedule" },
            h("thead", {}, h("tr", {}, h("th", { scope: "col" }, "Block"), h("th", { scope: "col", class: "num" }, "Permits"))),
            h(
              "tbody",
              {},
              ...top.map((p) =>
                h(
                  "tr",
                  {},
                  h("td", {}, h("button", { type: "button", class: "link", onclick: open(p) }, `${site.places.block[p]} block`)),
                  h("td", { class: "num" }, num(summary.count[p]!)),
                ),
              ),
            ),
          ),
        ]
      : []),
  );
}

export function notesDetail(ctx: SheetCtx, idx: Index): HTMLElement {
  const q = idx.quality;
  const pct = (n: number) => (q.unique_permits ? `${((100 * n) / q.unique_permits).toFixed(1)}%` : "0%");
  const fetched = idx.sources.map((s) => s.fetched.slice(0, 10)).sort().at(-1);
  return h(
    "section",
    { class: "detail-body notes" },
    head("General notes", () => ctx.go(null)),
    h(
      "p",
      { class: "lead" },
      `${num(q.unique_permits)} permits from the City of Tucson's right-of-way records, ${q.date_min.slice(0, 4)} to ${q.date_max.slice(0, 4)}.`,
      fetched ? ` Downloaded ${dateText(fetched)}.` : "",
    ),
    h(
      "ol",
      { class: "general-notes" },
      h("li", {}, `Before ${idx.sparseBefore} the city's records are thin, so those years look quieter than they were.`),
      h("li", {}, `${num(q.no_block)} permits (${pct(q.no_block)}) have no street number. They're placed at the intersection or spot the city recorded.`),
      q.unplaced ? h("li", {}, `${plural(q.unplaced, "permit has", "permits have")} no location at all, so they aren't on the map.`) : null,
      q.no_date ? h("li", {}, `${plural(q.no_date, "permit has", "permits have")} no usable date, so they never appear.`) : null,
      h("li", {}, `${num(q.no_work)} permits were voided, withdrawn or denied, or are still in review. They're hidden unless you include them.`),
      h("li", {}, "A permit near a corner may be filed under either street. Work done without a permit never appears."),
      h("li", {}, "Colours follow the paint utility crews spray before digging. Civic event and Small wireless start hidden: they're mostly closures and equipment on poles, not digging."),
    ),
    h("h3", {}, "What the kinds mean"),
    h(
      "dl",
      { class: "kind-meanings" },
      ...kindMeanings
        .filter(([name]) => idx.categories.includes(name))
        .flatMap(([name, meaning]) => [h("dt", {}, swatch(name, 10), name), h("dd", {}, meaning)]),
    ),
    h(
      "p",
      { class: "quiet" },
      "Data: City of Tucson. Map: ",
      h("a", { href: "https://www.openstreetmap.org/copyright" }, "OpenStreetMap"),
      " contributors, drawn with ",
      h("a", { href: "https://leafletjs.com" }, "Leaflet"),
      ". ",
      h("a", { href: "../" }, "The first version of this page"),
      " is still up for comparison.",
    ),
  );
}
