import "@fontsource/public-sans/400.css";
import "@fontsource/public-sans/600.css";
import "@fontsource/public-sans/800.css";
import "leaflet/dist/leaflet.css";
import "./styles.css";

import { DataError, errorMessage, loadSite, PlaceKind, yearSpan, type Site } from "./data";
import { fill, h } from "./dom";
import { countByYear, matcher, summarizePlaces, type Filter } from "./filters";
import { feet, num, placeName, plural, prettyStreet } from "./format";
import { DETAIL_ZOOM } from "./dots";
import { Annotations, mapDevices } from "./annotations";
import { PermitMap } from "./map";
import { digKinds, digsByPlace, placeAt, type Ctx, type Focus } from "./model";
import { aboutPanel } from "./panels/about";
import { busiestPanel } from "./panels/busiest";
import { failed, note } from "./panels/common";
import { filtersPanel } from "./panels/filters";
import { legend } from "./panels/legend";
import { placePanel, spotPanel } from "./panels/place";
import { nearby } from "./panels/radius";
import { describe, searchBox } from "./panels/search";
import { streetBlocks, streetPanel } from "./panels/street";
import { yearStrip } from "./panels/years";
import { buildSearch } from "./search";
import { DEFAULT_RADIUS, RADII, fromHash, placeKey, placeLookup, toHash } from "./url";

const $ = (id: string) => document.getElementById(id)!;
const dark = window.matchMedia("(prefers-color-scheme: dark)");
const narrow = window.matchMedia("(max-width: 820px)");
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");

// these start switched off
const OFF_AT_START = new Set(["Civic event", "Small wireless"]);

// skip the thin early years when there are later ones to show
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
        { class: "load-error", role: "alert" },
        h("h2", {}, "The permit data didn't load"),
        h("p", {}, "Check your connection, then try again."),
        h("button", { type: "button", class: "more", onclick: () => location.reload() }, "Try again"),
        h("details", {}, h("summary", {}, "Technical details"), h("p", {}, errorMessage(e))),
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
  let replaying = false;
  const zoomNote = () => years.showZoomNote(map.map.getZoom() < DETAIL_ZOOM);
  const findPlace = placeLookup(site.places);
  // permit numbers by index, for links: from search hits and from links followed
  const permitNumbers = new Map<number, string>();
  // counts permit lookups, so only the latest one opens anything
  let permitAsk = 0;
  // a permit number from a link that hasn't opened yet
  let linkedPermit: string | undefined;
  // true from choosing a permit until its place is first drawn, which goes to its row
  let landOnPermit = false;
  // what went wrong following a permit link, shown above the panel until something is opened
  let linkProblem: HTMLElement | null = null;

  const tip = $("tip");
  const aim = $("aim");
  const aimLine = aim.querySelector<HTMLElement>(".aim-line")!;

  // esc closes an open place, unless a field or the search list wants it
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape" || e.defaultPrevented || !focus) return;
    if ((e.target as HTMLElement).closest("input, select, textarea")) return;
    go(focus.kind === "place" && focus.from ? focus.from : null);
  });
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
    zoomed: () => zoomNote(),
    aimed: (place, active) => {
      aim.hidden = !active;
      aimLine.textContent = !active
        ? ""
        : place === undefined
          ? "Nothing at the center. Arrow keys move the map."
          : `Enter opens ${placeName(site.places, place)} (${plural(summary.count[place]!, "permit")})`;
      $("aim-live").textContent = aimLine.textContent;
    },
  });
  const notes = new Annotations(map.map, "ring");
  mapDevices(map.map);
  map.setBasemap(dark.matches);
  dark.addEventListener("change", () => {
    map.setBasemap(dark.matches);
    render({ filters: true });
  });

  const ctx = (): Ctx => ({ site, filter, radiusM, summary, go, setRadius, showAllKinds, retry: () => renderFocus(false) });

  function showAllKinds() {
    filter = { ...filter, cats: new Set(site.index.categories.map((_, i) => i)) };
    render({ focus: true, filters: true });
  }

  function openAbout(section: string) {
    const about = $("about").querySelector("details");
    if (about) about.open = true;
    const heading = $(section);
    heading.scrollIntoView({ block: "start", behavior: reducedMotion.matches ? "auto" : "smooth" });
    heading.focus({ preventScroll: true });
  }
  const showMeanings = () => openAbout("kinds-meaning");
  $("how").addEventListener("click", () => openAbout("how-it-works"));

  // a replay that runs to the end closes on what only watching the years go by shows
  function endReplay() {
    const span = filter.to - filter.from + 1;
    const digs = digsByPlace(site, filter);
    const repeatedBlocks: number[] = [];
    for (let p = 0; p < digs.length; p++) if (digs[p]! >= 5 && site.places.kind[p] === PlaceKind.Block) repeatedBlocks.push(p);
    const repeated = repeatedBlocks.length;
    let blocksOnMap = 0;
    for (let p = 0; p < summary.count.length; p++) if (summary.count[p] && site.places.kind[p] === PlaceKind.Block) blocksOnMap++;
    picked = !focus && repeated > 0 ? repeatedBlocks : null;
    if (picked) map.outline(picked, false);
    let peak = filter.from;
    let peakN = 0;
    for (const [y, n] of countByYear(site.permits, filter)) {
      if (y >= filter.from && y <= filter.to && n > peakN) [peak, peakN] = [y, n];
    }
    const blocks = repeated ? `${num(repeated)} of ${plural(blocksOnMap, "block")}` : "no blocks";
    const note = $("replay-note");
    note.textContent = `${plural(span, "year")} · ${blocks} dug up 5+ times${picked ? ", picked out on the map" : ""} · peak year ${peak} (${plural(peakN, "permit")})`;
    note.hidden = false;
    noteKey = filterKey(filter);
    $("status").textContent =
      `Replay finished. ${plural(span, "year")}. ${repeated ? `${num(repeated)} of ${plural(blocksOnMap, "block")}` : "No blocks"} dug up five times or more. ` +
      `Peak year ${peak}, ${plural(peakN, "permit")}.`;
  }
  let picked: number[] | null = null;
  function hideReplayNote() {
    $("replay-note").hidden = true;
    if (!picked) return;
    picked = null;
    if (!focus) map.clearFocus();
  }
  const filterKey = (f: Filter) => `${f.from} ${f.to} ${[...f.cats].sort((a, b) => a - b)} ${f.includeNoWork}`;
  let noteKey = "";

  function resetFilters() {
    hideReplayNote();
    filter = initialFilter(site);
    render({ focus: true, filters: true });
    years.el.querySelector<HTMLElement>(".play")?.focus();
    $("status").textContent = `Filters reset. ${years.el.querySelector("h3")?.textContent ?? ""}`;
  }

  const sameSet = (a: Set<number>, b: Set<number>) => a.size === b.size && [...a].every((x) => b.has(x));
  function filtersChanged() {
    const start = initialFilter(site);
    return filter.from !== start.from || filter.to !== start.to || filter.includeNoWork !== start.includeNoWork || !sameSet(filter.cats, start.cats);
  }

  // what opened the current place, so closing it can put the keyboard back
  let opener: { el: HTMLElement; text: string } | null = null;
  let fromBar: string | null = null;

  function go(f: Focus | null, opts: { move?: boolean } = { move: true }) {
    if (f?.kind === "place" && !f.from && focus?.kind === "street") {
      f = { ...f, from: focus };
      fromBar = (document.activeElement as HTMLElement | null)?.closest(".station")?.getAttribute("aria-label") ?? null;
    }
    const upToStreet = f?.kind === "street" && focus?.kind === "place" && focus.from !== undefined;
    if (f && !focus && document.activeElement instanceof HTMLElement && document.activeElement !== document.body) {
      opener = { el: document.activeElement, text: document.activeElement.textContent ?? "" };
    }
    const closing = !f && focus !== null;
    showLinkProblem(null);
    // a permit link still being looked up mustn't open anything now
    permitAsk++;
    linkedPermit = undefined;
    landOnPermit = f?.kind === "place" && f.permit !== undefined;
    const widened = f?.kind === "place" && f.permit !== undefined && reveal(f.permit);
    focus = f;
    if (widened) render({ filters: true, save: false });
    renderFocus(opts.move ?? true);
    if (closing) {
      saveHash(true);
      // the list is redrawn, so find the same button again by its text
      const back =
        (opener && document.contains(opener.el) && opener.el) ||
        [...document.querySelectorAll<HTMLElement>("#focus button, #map")].find((b) => opener && b.textContent === opener.text && opener.text) ||
        (opener?.el.id === "map" ? $("map") : null) ||
        $("focus").querySelector<HTMLElement>("h2");
      opener = null;
      back?.focus({ preventScroll: true });
      $("status").textContent = `Closed. ${$("focus").querySelector("h2")?.textContent ?? ""}.`;
      return;
    }
    // a new place is a step people expect Back to undo
    saveHash(true);
    const bar = upToStreet && fromBar ? [...$("focus").querySelectorAll<HTMLElement>(".station")].find((b) => b.getAttribute("aria-label") === fromBar) : undefined;
    fromBar = upToStreet ? null : fromBar;
    if (bar) {
      $("focus").querySelectorAll<HTMLElement>(".station").forEach((b) => (b.tabIndex = b === bar ? 0 : -1));
      bar.focus({ preventScroll: true });
    } else $("focus").querySelector<HTMLElement>("h2")?.focus({ preventScroll: true });
    // Say what was chosen for screen readers: the heading and its summary
    const box = $("focus");
    $("status").textContent = f
      ? [box.querySelector("h2")?.textContent, box.querySelector(".digs, .lead, .sub")?.textContent, widened && "The filters now include the permit you chose."]
          .filter(Boolean)
          .join(". ")
      : "";
    // The panel sits below the map on a phone, so bring the answer into view
    if (f && narrow.matches) {
      $("focus").scrollIntoView({ block: "start", behavior: reducedMotion.matches ? "auto" : "smooth" });
    }
  }

  /** a new radius redraws only the circle and the nearby section */
  function setRadius(m: number) {
    radiusM = m;
    saveHash();
    if (!focus || focus.kind === "street") return renderFocus(true);
    const at = focus.kind === "place" ? placeAt(site, focus.place) : focus.at;
    map.circle(at, radiusM, true);
    markFocus(at);
    const old = $("focus").querySelector(".nearby");
    const listed = !!old?.querySelector(".permit-table");
    const fresh = nearby(ctx(), at, focus.kind === "place" ? { exclude: focus.place, listNow: listed } : { listNow: true });
    old?.replaceWith(fresh);
    fresh.querySelector("select")?.focus();
    $("status").textContent = `Nearby now means within ${RADII.find((r) => r.m === m)?.label ?? feet(m)}.`;
  }

  /** the ring and count on the open place or spot, and its radius */
  function markFocus(at: { lat: number; lng: number }) {
    notes.clear();
    const place = focus?.kind === "place" ? focus.place : undefined;
    const n = place === undefined ? 0 : digKinds(site, filter).size ? digsByPlace(site, filter)[place]! : summary.count[place]!;
    notes.mark(at, n ? (digKinds(site, filter).size ? plural(n, "dig") : plural(n, "permit")) : undefined);
    notes.dimension(at, radiusM);
  }

  function renderFocus(move: boolean) {
    // On a phone the map gives up room once something is chosen
    $("layout").classList.toggle("has-focus", focus !== null);
    map.map.invalidateSize();
    const c = ctx();
    const box = $("focus");
    notes.clear();
    if (!focus) {
      if (picked) map.outline(picked, false);
      else map.clearFocus();
      fill(box, busiestPanel(c, map.visible()));
      return;
    }
    switch (focus.kind) {
      case "place": {
        const at = placeAt(site, focus.place);
        map.circle(at, radiusM, move);
        markFocus(at);
        const from = focus.from;
        const back = from?.kind === "street" ? { label: prettyStreet(site.places.streets[from.street]!), go: () => go(from) } : undefined;
        const picked = focus.permit === undefined ? undefined : { permit: focus.permit, land: landOnPermit };
        landOnPermit = false;
        fill(box, placePanel(c, focus.place, focus.note, back, picked));
        break;
      }
      case "street":
        map.outline(streetBlocks(c, focus.street), move);
        fill(box, streetPanel(c, focus.street));
        break;
      case "spot":
        map.circle(focus.at, radiusM, move);
        markFocus(focus.at);
        fill(box, spotPanel(c, focus.title, focus.at, focus.note));
        break;
    }
  }

  const years = yearStrip(site, {
    setRange(from, to, live) {
      filter = { ...filter, from, to };
      render({ filters: true, focus: !live || !focus });
      // A finished replay has already said its own line.
      if (!live && $("replay-note").hidden) $("status").textContent = years.el.querySelector("h3")?.textContent ?? "";
    },
    replaying: (on, finished, resumed) => {
      replaying = on;
      map.setReplaying(on);
      if (finished) endReplay();
      else hideReplayNote();
      if (on) $("status").textContent = resumed === undefined ? `Replay started from ${filter.from}.` : `Replay resumed from ${resumed}.`;
      if (on && narrow.matches) $("map").scrollIntoView({ block: "start", behavior: reducedMotion.matches ? "auto" : "smooth" });
    },
  });
  // The year chart and the legend share the strip under the map
  const legendSlot = h("div", { class: "legend-slot" });
  $("years").replaceWith(h("div", { class: "strip", id: "strip" }, years.el, legendSlot));
  zoomNote();
  // the strip isn't a control itself, so the skip link lands on Replay
  document.querySelector<HTMLAnchorElement>(".skip")!.addEventListener("click", (e) => {
    e.preventDefault();
    years.el.querySelector<HTMLElement>(".play")?.focus();
  });

  function renderFilters() {
    fill(
      legendSlot,
      legend(
        site,
        filter,
        {
          setCategory(c, on) {
            const cats = new Set(filter.cats);
            if (on) cats.add(c);
            else cats.delete(c);
            filter = { ...filter, cats };
            render({ focus: true, filters: true });
            $("status").textContent = `${site.index.categories[c]} ${on ? "shown" : "hidden"}. ${years.el.querySelector("h3")?.textContent ?? ""}`;
            legendSlot.querySelectorAll<HTMLButtonElement>(".kind")
              .forEach((b) => b.querySelector(".kind-name")?.textContent === site.index.categories[c] && b.focus());
          },
          setNoWork(on) {
            filter = { ...filter, includeNoWork: on };
            render({ focus: true, filters: true });
            legendSlot.querySelector<HTMLInputElement>(".check input")?.focus();
            $("status").textContent = `Permits that never led to work ${on ? "included" : "left out"}. ${years.el.querySelector("h3")?.textContent ?? ""}`;
          },
          showMeanings,
          showOnly(c) {
            filter = { ...filter, cats: new Set([c]) };
            render({ focus: true, filters: true });
            legendSlot.querySelector<HTMLSelectElement>(".only")?.focus();
            $("status").textContent = `Only ${site.index.categories[c]} shown. ${years.el.querySelector("h3")?.textContent ?? ""}`;
          },
          reset: { visible: filtersChanged() && !replaying, go: resetFilters },
        },
        "linkish",
      ),
    );
    fill(
      $("filters"),
      filtersPanel(site, overlaysOn, {
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

  function render(what: { focus?: boolean; filters?: boolean; save?: boolean } = {}) {
    if (!replaying && filterKey(filter) !== noteKey) hideReplayNote();
    summary = summarizePlaces(site.permits, filter, placeCount);
    const digging = digKinds(site, filter).size > 0;
    map.show(summary, filter.to, digging ? digsByPlace(site, filter) : undefined);
    years.sizeKey(digging ? "times dug up" : "permits filed");
    years.update(filter);
    $("year-stamp").textContent = filter.from === filter.to ? String(filter.from) : `${filter.from}–${filter.to}`;
    if (what.filters) renderFilters();
    if (what.focus) renderFocus(false);
    if (what.save !== false) saveHash();
  }

  function saveHash(push = false) {
    const keepQuery = focus && focus.kind !== "place" ? focus.q : undefined;
    const permit = focus?.kind === "place" && focus.permit !== undefined ? permitNumbers.get(focus.permit) : focus ? undefined : linkedPermit;
    const hash = toHash({
      place: focus?.kind === "place" && !permit ? placeKey(site.places, focus.place) : undefined,
      q: keepQuery,
      permit,
      radiusM: radiusM === DEFAULT_RADIUS ? undefined : radiusM,
      from: filter.from,
      to: filter.to,
      cats: [...filter.cats].sort((a, b) => a - b),
      includeNoWork: filter.includeNoWork || undefined,
    }, site.index);
    const url = hash || location.pathname + location.search;
    if (push && hash !== location.hash) history.pushState(null, "", url);
    else history.replaceState(null, "", url);
  }

  /** Sets up the view a link describes, and returns the permit number it names, to look up */
  function applyHash(): string | undefined {
    const s = fromHash(location.hash, site.index);
    const [first, last] = yearSpan(site.index);
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
    showLinkProblem(null);
    // A link followed now wins over one still being looked up
    permitAsk++;
    linkedPermit = s.permit;
    if (s.permit) return s.permit;
    const linked = s.place === undefined ? undefined : findPlace(s.place);
    if (linked !== undefined) focus = { kind: "place", place: linked };
    else if (s.q) {
      const hit = search(s.q, 1)[0];
      if (hit) focus = describe(site.places, hit, s.q).focus;
    }
    return undefined;
  }

  function showLinkProblem(el: HTMLElement | null) {
    linkProblem?.remove();
    linkProblem = el;
    if (el) $("focus").before(el);
  }

  function reveal(i: number): boolean {
    const y = site.permits.year[i]!;
    if (!y || matcher(site.permits, filter)(i)) return false;
    filter = {
      from: Math.min(filter.from, y),
      to: Math.max(filter.to, y),
      cats: new Set([...filter.cats, site.permits.cat[i]!]),
      includeNoWork: filter.includeNoWork || site.permits.noWork[i] === 1,
    };
    return true;
  }

  /** Opens the place of a permit named in a link, with the permit picked out */
  function openPermit(n: string, retrying = false) {
    const ask = ++permitAsk;
    const number = n.toUpperCase();
    site.api.byNumber(n).then(
      ({ id, place, permit }) => {
        // something else was opened, or another link followed, while this was on its way
        if (ask !== permitAsk || focus) return;
        showLinkProblem(null);
        linkedPermit = undefined;
        permitNumbers.set(id, permit.permit);
        focus = { kind: "place", place, permit: id };
        landOnPermit = true;
        reveal(id);
        render({ filters: true });
        renderFocus(true);
        const heading = $("focus").querySelector<HTMLElement>("h2");
        if (retrying) heading?.focus({ preventScroll: true });
        $("status").textContent = `Showing permit ${permit.permit}, ${heading?.textContent ?? ""}.`;
        if (narrow.matches) $("focus").scrollIntoView({ block: "start" });
      },
      (e: unknown) => {
        if (ask !== permitAsk || focus) return;
        const missing = e instanceof DataError && e.status === 404;
        const message = `No permit ${number} is on record.`;
        showLinkProblem(missing ? note(message) : failed(`permit ${number}`, e, () => openPermit(n, true)));
        if (missing) {
          $("status").textContent = message;
          linkedPermit = undefined;
          saveHash();
        }
        // the Try again button that was pressed has gone; keep the keyboard on the new one
        if (retrying) (linkProblem?.querySelector("button") ?? $("focus").querySelector<HTMLElement>("h2"))?.focus();
      },
    );
  }

  $("lede").textContent = `Every permit to dig up, close or work in a Tucson street, back to ${site.index.dateRange[0].slice(0, 4)}. Drag the years, or press Replay to watch the city fill in.`;
  const blocksOf = new Map<number, number[]>();
  site.places.kind.forEach((k, p) => {
    if (k !== PlaceKind.Block) return;
    const s = site.places.street[p]!;
    const list = blocksOf.get(s);
    if (list) list.push(p);
    else blocksOf.set(s, [p]);
  });
  const streetCount = (s: number) => (blocksOf.get(s) ?? []).reduce((a, p) => a + summary.count[p]!, 0);
  const permitSearch = async (q: string, signal: AbortSignal) => {
    const hits = await site.api.search(q, signal);
    for (const hit of hits) permitNumbers.set(hit.id, hit.number);
    return hits;
  };
  fill($("search"), searchBox(site.places, (q) => search(q), (f) => go(f), streetCount, permitSearch));
  fill($("about"), aboutPanel(site.index));
  const restore = () => {
    const permit = applyHash();
    render({ filters: true });
    renderFocus(true);
    // A place or street arrived by a link is said aloud, as one opened by hand is
    const shown = focus && $("focus").querySelector("h2")?.textContent;
    if (shown) $("status").textContent = `Showing ${shown}.`;
    if (focus && narrow.matches) $("focus").scrollIntoView({ block: "start" });
    if (permit) openPermit(permit);
  };
  restore();
  window.addEventListener("popstate", restore);
}

void start();
