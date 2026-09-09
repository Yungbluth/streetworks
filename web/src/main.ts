import "@fontsource/public-sans/400.css";
import "@fontsource/public-sans/600.css";
import "@fontsource/public-sans/800.css";
import "leaflet/dist/leaflet.css";
import "./styles.css";

import { loadSite, type Site } from "./data";
import { fill, h } from "./dom";
import { summarizePlaces, type Filter } from "./filters";
import { placeName, plural } from "./format";
import { PermitMap } from "./map";
import { placeAt, type Ctx, type Focus } from "./model";
import { aboutPanel } from "./panels/about";
import { busiestPanel } from "./panels/busiest";
import { filtersPanel } from "./panels/filters";
import { placePanel, spotPanel } from "./panels/place";
import { describe, searchBox } from "./panels/search";
import { streetBlocks, streetPanel } from "./panels/street";
import { yearStrip } from "./panels/years";
import { buildSearch } from "./search";
import { DEFAULT_RADIUS, fromHash, toHash } from "./url";

const $ = (id: string) => document.getElementById(id)!;
const dark = window.matchMedia("(prefers-color-scheme: dark)");

// these start switched off
const OFF_AT_START = new Set(["Civic event", "Small wireless"]);

// skip the thin early years when there are later ones to show
function initialFilter(site: Site): Filter {
  const first = Number(site.index.dateRange[0].slice(0, 4));
  const last = Number(site.index.dateRange[1].slice(0, 4));
  const sparse = site.index.sparseBefore;
  return {
    from: first < sparse && last >= sparse ? sparse : first,
    to: last,
    cats: new Set(site.index.categories.flatMap((c, i) => (OFF_AT_START.has(c) ? [] : [i]))),
    includeNoWork: false,
  };
}

async function start() {
  const base = new URL("data/", document.baseURI).href;
  let site: Site;
  try {
    site = await loadSite(base);
    // console.log(site.index);
  } catch (e) {
    fill(
      $("map"),
      h(
        "div",
        { class: "load-error" },
        h("h2", {}, "The permit data didn't load"),
        h("p", {}, e instanceof Error ? e.message : String(e)),
        h("p", {}, "Reload the page to try again."),
      ),
    );
    return;
  }

  const placeCount = site.places.kind.length;
  const search = buildSearch(site.places, summarizePlaces(site.permits, { from: 0, to: 0, cats: new Set(site.index.categories.map((_, i) => i)), includeNoWork: true }, placeCount).count);
  let filter = initialFilter(site);
  let radiusM = DEFAULT_RADIUS;
  let focus: Focus | null = null;
  const overlaysOn = new Set<string>();
  let summary = summarizePlaces(site.permits, filter, placeCount);

  const tip = $("tip");
  const map = new PermitMap($("map"), site, {
    pick: (place) => go({ kind: "place", place }),
    hover: (place, at) => {
      tip.hidden = place === undefined;
      if (place === undefined) return;
      tip.textContent = `${placeName(site.places, place)}: ${plural(summary.count[place]!, "permit")}`;
      tip.style.transform = `translate(${at.x + 14}px, ${at.y + 14}px)`;
    },
    moved: () => {
      if (!focus) renderFocus(false);
    },
  });
  map.setBasemap(dark.matches);
  dark.addEventListener("change", () => {
    map.setBasemap(dark.matches);
    render({ filters: true });
  });

  const ctx = (): Ctx => ({ site, filter, radiusM, summary, go, setRadius });

  function go(f: Focus | null, opts: { move?: boolean } = { move: true }) {
    focus = f;
    renderFocus(opts.move ?? true);
    saveHash();
    $("focus").querySelector<HTMLElement>("h2")?.focus({ preventScroll: true });
  }

  function setRadius(m: number) {
    radiusM = m;
    renderFocus(true);
    saveHash();
  }

  function renderFocus(move: boolean) {
    const c = ctx();
    const box = $("focus");
    if (!focus) {
      map.clearFocus();
      fill(box, busiestPanel(c, map.visible()));
      return;
    }
    switch (focus.kind) {
      case "place":
        map.circle(placeAt(site, focus.place), radiusM, move);
        fill(box, placePanel(c, focus.place, focus.note));
        break;
      case "street":
        map.outline(streetBlocks(c, focus.street), move);
        fill(box, streetPanel(c, focus.street));
        break;
      case "spot":
        map.circle(focus.at, radiusM, move);
        fill(box, spotPanel(c, focus.title, focus.at, focus.note));
        break;
    }
  }

  const years = yearStrip(site, {
    setRange(from, to, live) {
      filter = { ...filter, from, to };
      render({ filters: !live, focus: !live });
    },
  });
  $("years").replaceWith(years.el);

  function renderFilters() {
    fill(
      $("filters"),
      filtersPanel(site, filter, overlaysOn, {
        setCategory(c, on) {
          const cats = new Set(filter.cats);
          if (on) cats.add(c);
          else cats.delete(c);
          filter = { ...filter, cats };
          render({ focus: true });
        },
        setNoWork(on) {
          filter = { ...filter, includeNoWork: on };
          render({ focus: true, filters: true });
        },
        async setOverlay(name, on) {
          const o = site.index.overlays.find((x) => x.name === name)!;
          if (on) overlaysOn.add(name);
          else overlaysOn.delete(name);
          await map.toggleOverlay(name, o.file, on, async (file) => {
            const res = await fetch(new URL(file, base));
            if (!res.ok) throw new Error(`the server said ${res.status}`);
            return res.json();
          });
        },
      }),
    );
  }

  function render(what: { focus?: boolean; filters?: boolean } = {}) {
    summary = summarizePlaces(site.permits, filter, placeCount);
    map.show(summary, filter.to);
    years.update(filter);
    if (what.filters) renderFilters();
    if (what.focus) renderFocus(false);
    saveHash();
  }

  function saveHash() {
    const keepQuery = focus && focus.kind !== "place" ? focus.q : undefined;
    const hash = toHash({
      place: focus?.kind === "place" ? focus.place : undefined,
      q: keepQuery,
      radiusM: radiusM === DEFAULT_RADIUS ? undefined : radiusM,
      from: filter.from,
      to: filter.to,
      cats: [...filter.cats].sort((a, b) => a - b),
      includeNoWork: filter.includeNoWork || undefined,
    });
    history.replaceState(null, "", hash || location.pathname + location.search);
  }

  function applyHash() {
    const s = fromHash(location.hash, site.index);
    const first = Number(site.index.dateRange[0].slice(0, 4));
    const last = Number(site.index.dateRange[1].slice(0, 4));
    const clamp = (y: number) => Math.min(last, Math.max(first, y));
    const start = initialFilter(site);
    filter = {
      from: clamp(s.from ?? start.from),
      to: clamp(s.to ?? start.to),
      cats: s.cats ? new Set(s.cats) : start.cats,
      includeNoWork: s.includeNoWork ?? false,
    };
    if (filter.from > filter.to) [filter.from, filter.to] = [filter.to, filter.from];
    radiusM = s.radiusM ?? DEFAULT_RADIUS;
    focus = null;
    if (s.place !== undefined && s.place < placeCount) focus = { kind: "place", place: s.place };
    else if (s.q) {
      const hit = search(s.q, 1)[0];
      if (hit) focus = describe(site.places, hit, s.q).focus;
    }
  }

  $("lede").textContent = `Every permit to dig up, close or work in a Tucson street, back to ${site.index.dateRange[0].slice(0, 4)}.`;
  fill($("search"), searchBox(site.places, (q) => search(q), (f) => go(f)));
  fill($("about"), aboutPanel(site.index));
  applyHash();
  render({ filters: true });
  renderFocus(true);
  window.addEventListener("hashchange", () => {
    applyHash();
    render({ filters: true });
    renderFocus(true);
  });
}

void start();
