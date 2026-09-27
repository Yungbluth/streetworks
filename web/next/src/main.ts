import "@fontsource/sofia-sans-semi-condensed/400.css";
import "@fontsource/sofia-sans-semi-condensed/600.css";
import "@fontsource/sofia-sans-semi-condensed/700.css";
import "leaflet/dist/leaflet.css";
import "./styles.css";

import { errorMessage, loadSite, yearSpan, type Site } from "../../src/data";
import { fill, h } from "../../src/dom";
import { DETAIL_ZOOM } from "../../src/dots";
import { summarizePlaces, type Filter } from "../../src/filters";
import { dateText, num, placeName, plural } from "../../src/format";
import { PermitMap } from "../../src/map";
import { placeAt, type Focus } from "../../src/model";
import { describe, searchBox } from "../../src/panels/search";
import { streetBlocks } from "../../src/panels/street";
import { buildSearch } from "../../src/search";
import { DEFAULT_RADIUS, fromHash, placeKey, placeLookup, toHash } from "../../src/url";
import { busiestDetail, notesDetail, placeDetail, spotDetail, streetDetail, type SheetCtx } from "./detail";
import { legend } from "./legend";
import { Redline } from "./redline";
import { revisions } from "./revisions";

const $ = (id: string) => document.getElementById(id)!;
const dark = matchMedia("(prefers-color-scheme: dark)");
const narrow = matchMedia("(max-width: 860px)");
const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");

// event closures and equipment on poles aren't street work, so they start off
const OFF_AT_START = new Set(["Civic event", "Small wireless"]);

function initialFilter(site: Site): Filter {
  const [first, last] = yearSpan(site.index);
  const sparse = site.index.sparseBefore;
  return {
    from: first < sparse && last >= sparse ? sparse : first,
    to: last,
    cats: new Set(site.index.categories.flatMap((c, i) => (OFF_AT_START.has(c) ? [] : [i]))),
    includeNoWork: false,
  };
}

async function start() {
  // This page lives one folder below the first version and shares its data
  const base = new URL("../data/", document.baseURI).href;
  fill($("detail"), h("p", { class: "quiet loading" }, "Loading the permit record…"));
  let site: Site;
  try {
    site = await loadSite(base);
  } catch (e) {
    fill(
      $("detail"),
      h(
        "div",
        { class: "error", role: "alert" },
        h("h2", {}, "The permit data didn't load"),
        h("p", {}, errorMessage(e)),
        h("button", { type: "button", class: "btn", onclick: () => location.reload() }, "Try again"),
      ),
    );
    return;
  }

  const placeCount = site.places.kind.length;
  const search = buildSearch(
    site.places,
    summarizePlaces(site.permits, { from: 0, to: 0, cats: new Set(site.index.categories.map((_, i) => i)), includeNoWork: true }, placeCount).count,
  );
  let filter = initialFilter(site);
  let radiusM = DEFAULT_RADIUS;
  let focus: Focus | null = null;
  let notesOpen = false;
  // a place opened from the map hands focus back to the map when it closes
  let openedFromMap = false;
  let summary = summarizePlaces(site.permits, filter, placeCount);
  const findPlace = placeLookup(site.places);

  const tip = $("tip");
  const aim = $("aim");
  const aimLine = aim.querySelector<HTMLElement>(".aim-line")!;

  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape" || e.defaultPrevented || (!focus && !notesOpen)) return;
    if ((e.target as HTMLElement).closest("input, select, textarea")) return;
    const back = openedFromMap;
    go(null);
    if (back) $("map").focus();
  });

  const map = new PermitMap($("map"), site, {
    pick: (place) => {
      go({ kind: "place", place });
      openedFromMap = true;
    },
    hover: (place, at) => {
      tip.hidden = place === undefined;
      if (place === undefined) return;
      tip.textContent = `${placeName(site.places, place)}: ${plural(summary.count[place]!, "permit")}`;
      tip.style.transform = `translate(${at.x + 14}px, ${at.y + 14}px)`;
    },
    moved: () => {
      if (!focus && !notesOpen) renderDetail(false);
    },
    zoomed: (zoom) => rev.showZoomNote(zoom < DETAIL_ZOOM),
    aimed: (place, active) => {
      aim.hidden = !active;
      aimLine.textContent = !active
        ? ""
        : place === undefined
          ? "Nothing at the crosshair. Arrow keys move the map."
          : `Enter opens ${placeName(site.places, place)} (${plural(summary.count[place]!, "permit")})`;
    },
  });
  const redline = new Redline(map.map);
  map.setBasemap(dark.matches);
  dark.addEventListener("change", () => {
    map.setBasemap(dark.matches);
    render({ legend: true, detail: true });
  });

  const ctx = (): SheetCtx => ({
    site, filter, radiusM, summary, go, setRadius, showAllKinds, showNotes,
    retry: () => renderDetail(false),
  });

  function showAllKinds() {
    filter = { ...filter, cats: new Set(site.index.categories.map((_, i) => i)) };
    render({ detail: true, legend: true });
  }

  function showNotes() {
    focus = null;
    notesOpen = true;
    renderDetail(false);
    saveHash(true);
    settle();
  }

  function resetFilters() {
    filter = initialFilter(site);
    render({ detail: true, legend: true });
  }

  const sameSet = (a: Set<number>, b: Set<number>) => a.size === b.size && [...a].every((x) => b.has(x));
  function filtersChanged() {
    const s = initialFilter(site);
    return filter.from !== s.from || filter.to !== s.to || filter.includeNoWork !== s.includeNoWork || !sameSet(filter.cats, s.cats);
  }

  function go(f: Focus | null, opts: { move?: boolean } = { move: true }) {
    focus = f;
    notesOpen = false;
    openedFromMap = false;
    renderDetail(opts.move ?? true);
    // A new place is a step people expect Back to undo
    saveHash(true);
    settle();
  }

  function settle() {
    const box = $("detail");
    box.querySelector<HTMLElement>("h2")?.focus({ preventScroll: true });
    $("status").textContent =
      focus || notesOpen ? [box.querySelector("h2")?.textContent, box.querySelector(".lead")?.textContent].filter(Boolean).join(". ") : "";
    if ((focus || notesOpen) && narrow.matches) {
      box.scrollIntoView({ block: "start", behavior: reducedMotion.matches ? "auto" : "smooth" });
    }
  }

  function setRadius(m: number) {
    radiusM = m;
    renderDetail(true);
    saveHash();
  }

  function renderDetail(move: boolean) {
    $("sheet").classList.toggle("has-focus", focus !== null || notesOpen);
    map.map.invalidateSize();
    redline.clear();
    const c = ctx();
    const box = $("detail");
    if (notesOpen) {
      map.clearFocus();
      fill(box, notesDetail(c, site.index));
      return;
    }
    if (!focus) {
      map.clearFocus();
      fill(box, busiestDetail(c, map.visible()));
      return;
    }
    switch (focus.kind) {
      case "place": {
        const at = placeAt(site, focus.place);
        map.circle(at, radiusM, move);
        const n = summary.count[focus.place]!;
        redline.cloud(at, n ? `Δ${n}` : undefined);
        redline.dimension(at, radiusM);
        fill(box, placeDetail(c, focus.place, focus.note));
        break;
      }
      case "street":
        map.outline(streetBlocks(c, focus.street), move);
        fill(box, streetDetail(c, focus.street));
        break;
      case "spot":
        map.circle(focus.at, radiusM, move);
        redline.cloud(focus.at, undefined);
        redline.dimension(focus.at, radiusM);
        fill(box, spotDetail(c, focus.title, focus.at, focus.note));
        break;
    }
  }

  const rev = revisions(site, {
    setRange(from, to, live) {
      filter = { ...filter, from, to };
      render({ legend: !live, detail: !live });
      if (!live) $("status").textContent = rev.describe();
    },
  });
  rev.showZoomNote(map.map.getZoom() < DETAIL_ZOOM);

  const legendSlot = h("div", { class: "cell-slot" });
  const fetched = site.index.sources.map((s) => s.fetched.slice(0, 10)).sort().at(-1);
  fill(
    $("sheet-info"),
    h("span", {}, `${num(site.index.quality.unique_permits)} permits on record`),
    fetched ? h("span", {}, `updated ${dateText(fetched)}`) : null,
    h("button", { type: "button", class: "link", onclick: showNotes }, "General notes"),
  );
  fill($("title-block"), rev.el, legendSlot);

  function renderLegend() {
    fill(
      legendSlot,
      legend(site, filter, {
        setCategory(c, on) {
          const cats = new Set(filter.cats);
          if (on) cats.add(c);
          else cats.delete(c);
          filter = { ...filter, cats };
          render({ detail: true, legend: true });
          legendSlot.querySelectorAll<HTMLButtonElement>(".kind")
            .forEach((b) => b.querySelector(".kind-name")?.textContent === site.index.categories[c] && b.focus());
        },
        setNoWork(on) {
          filter = { ...filter, includeNoWork: on };
          render({ detail: true, legend: true });
          legendSlot.querySelector<HTMLInputElement>(".check input")?.focus();
        },
        showNotes,
      }),
    );
  }

  function render(what: { detail?: boolean; legend?: boolean } = {}) {
    summary = summarizePlaces(site.permits, filter, placeCount);
    map.show(summary, filter.to);
    rev.update(filter);
    rev.showReset(filtersChanged(), resetFilters);
    if (what.legend) renderLegend();
    if (what.detail) renderDetail(false);
    saveHash();
  }

  function saveHash(push = false) {
    const keepQuery = focus && focus.kind !== "place" ? focus.q : undefined;
    const hash = toHash(
      {
        place: focus?.kind === "place" ? placeKey(site.places, focus.place) : undefined,
        q: keepQuery,
        radiusM: radiusM === DEFAULT_RADIUS ? undefined : radiusM,
        from: filter.from,
        to: filter.to,
        cats: [...filter.cats].sort((a, b) => a - b),
        includeNoWork: filter.includeNoWork || undefined,
      },
      site.index,
    );
    const url = hash || location.pathname + location.search;
    if (push && hash !== location.hash) history.pushState(null, "", url);
    else history.replaceState(null, "", url);
  }

  function applyHash() {
    const s = fromHash(location.hash, site.index);
    const [first, last] = yearSpan(site.index);
    const clamp = (y: number) => Math.min(last, Math.max(first, y));
    const init = initialFilter(site);
    filter = {
      from: clamp(s.from ?? init.from),
      to: clamp(s.to ?? init.to),
      cats: s.cats ? new Set(s.cats) : init.cats,
      includeNoWork: s.includeNoWork ?? false,
    };
    if (filter.from > filter.to) [filter.from, filter.to] = [filter.to, filter.from];
    radiusM = s.radiusM ?? DEFAULT_RADIUS;
    focus = null;
    notesOpen = false;
    const linked = s.place === undefined ? undefined : findPlace(s.place);
    if (linked !== undefined) focus = { kind: "place", place: linked };
    else if (s.q) {
      const hit = search(s.q, 1)[0];
      if (hit) focus = describe(site.places, hit, s.q).focus;
    }
  }

  const [firstYear, lastYear] = yearSpan(site.index);
  $("sheet-title").textContent = `City of Tucson right-of-way permits, ${firstYear} to ${lastYear}`;
  // the search box is shared with the first page
  const box = searchBox(site.places, (q) => search(q), (f) => go(f));
  box.append(h("div", { class: "dropdown" }, ...box.querySelectorAll(".results-note, .results, .results-empty")));
  fill($("search"), box);
  const restore = () => {
    applyHash();
    render({ legend: true });
    renderDetail(true);
  };
  restore();
  window.addEventListener("popstate", restore);
}

void start();
