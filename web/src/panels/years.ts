import { yearSpan, type Site } from "../data";
import { fill, h } from "../dom";
import { ageOf, ageRanges, countByYear, type Filter } from "../filters";
import { num, yearsText } from "../format";
import { controlGlyph } from "../marks";

export interface YearEvents {
  setRange(from: number, to: number, live: boolean): void;
  // Replay started (true) or stopped; finished is true when it ran to the end
  replaying?(on: boolean, finished?: boolean): void;
}

const markRadius = { solid: "5", ring: "4.2", speck: "1.8" };

const hidden = (text: string) => h("span", { class: "visually-hidden" }, text);

const STEP_MS = 400;

// A finger has to travel this far before it counts as a drag rather than a tap
const SLOP_PX = 8;

const svg = (shape: "solid" | "ring" | "speck") => {
  const ns = "http://www.w3.org/2000/svg";
  const s = document.createElementNS(ns, "svg");
  s.setAttribute("width", "12");
  s.setAttribute("height", "12");
  s.setAttribute("aria-hidden", "true");
  const c = document.createElementNS(ns, "circle");
  c.setAttribute("cx", "6");
  c.setAttribute("cy", "6");
  c.setAttribute("r", markRadius[shape]);
  c.setAttribute("fill", shape === "ring" ? "none" : "currentColor");
  if (shape === "ring") {
    c.setAttribute("stroke", "currentColor");
    c.setAttribute("stroke-width", "1.6");
  }
  if (shape === "speck") c.setAttribute("opacity", "0.7");
  s.append(c);
  return s;
};

export function yearStrip(site: Site, ev: YearEvents) {
  const [first, last] = yearSpan(site.index);
  const span = last - first + 1;
  const sparse = site.index.sparseBefore;
  let from = first;
  let to = last;
  let timer = 0;
  let before: [number, number] | null = null;
  // once someone has moved the years, they don't need telling how
  let moved = false;

  const bars = h("div", { class: "bars" });
  const yearBars = Array.from({ length: span }, (_, i) => bars.appendChild(h("div", { title: String(first + i) })));
  const handle = (which: "from" | "to") =>
    h("button", {
      type: "button", class: `handle ${which}`, role: "slider",
      "aria-label": which === "from" ? "First year" : "Last year",
      "aria-valuemin": first, "aria-valuemax": last,
    });
  const hFrom = handle("from");
  const hTo = handle("to");
  const chart = h("div", { class: "chart" }, bars, hFrom, hTo);
  // Not a live region: it changes on every step of a drag or replay
  const total = h("h3", {});
  const key = h("p", { class: "key" });
  const play = h("button", { type: "button", class: "play primary" });
  const sparseNote = h("span", { class: "sparse-note" });
  const zoomNote = h("span", { class: "zoom-note", hidden: true }, "Zoom in to see rings and specks");
  const axis = h("div", { class: "axis", "aria-hidden": "true" });
  for (let y = first; y <= last; y++) {
    // every fifth year and the ends, unless an end would crowd its neighbour
    if (y === first || y === last || (y % 5 === 0 && y - first > 2 && last - y > 2)) {
      axis.append(h("span", { style: `left: ${((y - first + 0.5) / span) * 100}%` }, String(y)));
    }
  }
  // The key and notes sit under the chart
  const el = h(
    "section",
    { class: "years", "aria-label": "Years" },
    h("div", { class: "head" }, play, total),
    chart,
    axis,
    h("div", { class: "foot" }, key, sparseNote),
  );
  const label = (playing: boolean) => play.replaceChildren(controlGlyph(playing ? "stop" : "play"), playing ? "Stop" : "Replay");

  const yearAt = (clientX: number) => {
    const r = bars.getBoundingClientRect();
    return Math.min(last, Math.max(first, first + Math.floor(((clientX - r.left) / r.width) * span)));
  };
  const set = (f: number, t: number, live: boolean) => {
    from = Math.min(f, t);
    to = Math.max(f, t);
    ev.setRange(from, to, live);
  };

  let dragging: "from" | "to" | null = null;
  let pending: { id: number; x: number; y: number } | null = null;
  const nearer = (y: number) => (Math.abs(y - from) <= Math.abs(y - to) ? "from" : "to");
  const dragTo = (y: number) => {
    moved = true;
    if (dragging === "from") set(Math.min(y, to), to, true);
    else set(from, Math.max(y, from), true);
  };
  const begin = (e: PointerEvent, which: "from" | "to") => {
    dragging = which;
    chart.setPointerCapture(e.pointerId);
    stop();
    dragTo(yearAt(e.clientX));
  };
  chart.addEventListener("pointerdown", (e) => {
    const onHandle = e.target === hFrom ? "from" : e.target === hTo ? "to" : null;
    if (onHandle) return begin(e, onHandle);
    if (e.pointerType === "touch") pending = { id: e.pointerId, x: e.clientX, y: e.clientY };
    else begin(e, nearer(yearAt(e.clientX)));
  });
  chart.addEventListener("pointermove", (e) => {
    if (pending?.id === e.pointerId) {
      const dx = Math.abs(e.clientX - pending.x);
      const dy = Math.abs(e.clientY - pending.y);
      if (dy > SLOP_PX && dy >= dx) pending = null;
      else if (dx > SLOP_PX) {
        pending = null;
        begin(e, nearer(yearAt(e.clientX)));
      }
      return;
    }
    if (dragging) dragTo(yearAt(e.clientX));
  });
  const release = (e?: PointerEvent) => {
    if (e?.type === "pointerup" && pending?.id === e.pointerId) {
      // A tap with no travel: move the nearer handle there
      const y = yearAt(e.clientX);
      dragging = nearer(y);
      stop();
      dragTo(y);
    }
    pending = null;
    if (!dragging) return;
    dragging = null;
    set(from, to, false);
  };
  chart.addEventListener("pointerup", release);
  chart.addEventListener("pointercancel", () => release());
  chart.addEventListener("lostpointercapture", () => release());
  window.addEventListener("blur", () => release());

  for (const [hd, which] of [[hFrom, "from"], [hTo, "to"]] as const) {
    hd.addEventListener("keydown", (e) => {
      const step = { ArrowLeft: -1, ArrowDown: -1, ArrowRight: 1, ArrowUp: 1, PageDown: -5, PageUp: 5 }[e.key];
      const cur = which === "from" ? from : to;
      let y: number | undefined = step === undefined ? undefined : cur + step;
      if (e.key === "Home") y = which === "from" ? first : from;
      if (e.key === "End") y = which === "from" ? to : last;
      if (y === undefined) return;
      e.preventDefault();
      stop();
      moved = true;
      if (which === "from") set(Math.max(first, Math.min(y, to)), to, false);
      else set(from, Math.min(last, Math.max(y, from)), false);
    });
  }

  function stop(finished = false) {
    if (!timer) return;
    clearInterval(timer);
    timer = 0;
    ev.replaying?.(false, finished);
    label(false);
    const [f, t] = before ?? [from, to];
    before = null;
    set(f, t, false);
  }
  play.addEventListener("click", () => {
    if (timer) return stop();
    before = [from, to];
    const end = to;
    ev.replaying?.(true);
    // start the timer first: setting the range redraws this strip
    timer = window.setInterval(() => {
      if (to >= end) return stop(true);
      set(from, to + 1, true);
    }, STEP_MS);
    label(true);
    set(from, from, true);
  });

  function update(f: Filter, counts = countByYear(site.permits, f)) {
    from = f.from;
    to = f.to;
    const max = Math.max(1, ...counts.values());
    let inRange = 0;
    yearBars.forEach((bar, i) => {
      const y = first + i;
      const n = counts.get(y) ?? 0;
      if (y >= from && y <= to) inRange += n;
      bar.style.height = `${(n / max) * 100}%`;
      bar.className = y < from || y > to ? "out" : ageOf(y, to);
      if (y < sparse) bar.classList.add("thin");
      bar.title = `${y}: ${num(n)} permits`;
    });
    const pos = (y: number) => `${((y - first) / span) * 100}%`;
    hFrom.style.left = pos(from);
    hTo.style.left = pos(to + 1);
    hFrom.setAttribute("aria-valuenow", String(from));
    hTo.setAttribute("aria-valuenow", String(to));
    hFrom.setAttribute("aria-valuetext", String(from));
    hTo.setAttribute("aria-valuetext", String(to));
    total.textContent = `${num(inRange)} permits, ${yearsText(from, to)}`;
    if (!timer) label(false);

    const r = ageRanges(to);
    const clampText = (a: number, b: number) => yearsText(Math.max(a, from), b);
    fill(
      key,
      h("span", {}, svg("solid"), hidden("Solid dots: "), clampText(r.recent[0], r.recent[1])),
      r.back[1] >= from ? h("span", { class: "older" }, svg("ring"), hidden("Rings: "), clampText(r.back[0], r.back[1])) : null,
      r.oldBefore > from ? h("span", { class: "older" }, svg("speck"), hidden("Specks: "), yearsText(from, r.oldBefore - 1)) : null,
      zoomNote,
    );
    sparseNote.textContent =
      from < sparse
        ? `Before ${sparse} the city's records are thin, so those years look quieter than they were.`
        : moved
          ? "\u00a0"
          : "Drag either end to choose the years.";
  }

  function showZoomNote(zoomedOut: boolean) {
    zoomNote.hidden = !zoomedOut;
    key.classList.toggle("zoomed-out", zoomedOut);
  }

  return { el, update, showZoomNote };
}
