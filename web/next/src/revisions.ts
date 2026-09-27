import { yearSpan, type Site } from "../../src/data";
import { fill, h } from "../../src/dom";
import { ageOf, ageRanges, countByYear, type Filter } from "../../src/filters";
import { num, yearsText } from "../../src/format";
import { ageMark } from "./marks";

export interface RevisionEvents {
  setRange(from: number, to: number, live: boolean): void;
}

const SLOP_PX = 8;

export function revisions(site: Site, ev: RevisionEvents) {
  const [first, last] = yearSpan(site.index);
  const span = last - first + 1;
  const sparse = site.index.sparseBefore;
  let from = first;
  let to = last;
  let timer = 0;
  let before: [number, number] | null = null;
  let dragged = false;

  const bars = h("div", { class: "rev-bars", "aria-hidden": "true" });
  const yearBars = Array.from({ length: span }, () => bars.appendChild(h("span", {})));
  const handle = (which: "from" | "to") =>
    h("button", {
      type: "button", class: `rev-handle ${which}`, role: "slider",
      "aria-label": which === "from" ? "First year" : "Last year",
      "aria-valuemin": first, "aria-valuemax": last,
    });
  const hFrom = handle("from");
  const hTo = handle("to");
  const band = h("span", { class: "rev-band", "aria-hidden": "true" });
  const chart = h("div", { class: "rev-chart" }, bars, band, hFrom, hTo);
  const axis = h("div", { class: "rev-axis", "aria-hidden": "true" });
  for (let y = first; y <= last; y++) {
    if (y === first || y === last || (y % 5 === 0 && y - first > 2 && last - y > 2)) {
      axis.append(h("span", { style: `left: ${((y - first + 0.5) / span) * 100}%` }, String(y)));
    }
  }

  const play = h("button", { type: "button", class: "btn play" }, "Replay");
  const total = h("p", { class: "rev-total" });
  const reset = h("button", { type: "button", class: "link reset", hidden: true }, "Reset filters");
  const key = h("p", { class: "rev-key" });
  // always present, so its text can change without moving anything
  const zoomNote = h("span", { class: "rev-zoom" }, "Rings and specks appear when you zoom in");
  const hint = h("p", { class: "rev-hint" });

  const el = h(
    "div",
    { class: "cell revisions" },
    h("h2", { class: "cell-label" }, "Revisions by year"),
    h("div", { class: "rev-head" }, play, total, reset),
    chart,
    axis,
    h("div", { class: "rev-foot" }, key, zoomNote),
    hint,
  );

  const yearAt = (clientX: number) => {
    const r = bars.getBoundingClientRect();
    return Math.min(last, Math.max(first, first + Math.floor(((clientX - r.left) / r.width) * span)));
  };
  const set = (f: number, t: number, live: boolean) => {
    from = Math.min(f, t);
    to = Math.max(f, t);
    ev.setRange(from, to, live);
  };

  // Mouse and pen: press anywhere on the chart to move the nearer handle
  let dragging: "from" | "to" | null = null;
  let pending: { id: number; x: number; y: number } | null = null;
  const nearer = (y: number) => (Math.abs(y - from) <= Math.abs(y - to) ? "from" : "to");
  const dragTo = (y: number) => {
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
    if (e && pending?.id === e.pointerId && e.type === "pointerup") {
      // A tap with no travel: move the nearer handle there
      const y = yearAt(e.clientX);
      dragging = nearer(y);
      stop();
      dragTo(y);
    }
    pending = null;
    if (!dragging) return;
    dragging = null;
    dragged = true;
    set(from, to, false);
  };
  chart.addEventListener("pointerup", release);
  chart.addEventListener("pointercancel", () => ((pending = null), release()));
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
      dragged = true;
      if (which === "from") set(Math.max(first, Math.min(y, to)), to, false);
      else set(from, Math.min(last, Math.max(y, from)), false);
    });
  }

  function stop() {
    if (!timer) return;
    clearInterval(timer);
    timer = 0;
    play.textContent = "Replay";
    const [f, t] = before ?? [from, to];
    before = null;
    set(f, t, false);
  }
  play.addEventListener("click", () => {
    if (timer) return stop();
    before = [from, to];
    const end = to;
    timer = window.setInterval(() => {
      if (to >= end) return stop();
      set(from, to + 1, true);
    }, 650);
    play.textContent = "Stop";
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
      bar.style.height = `${Math.max(n ? 3 : 0, (n / max) * 100)}%`;
      bar.className = y < from || y > to ? "out" : ageOf(y, to);
      if (y < sparse) bar.classList.add("thin");
    });
    const pos = (y: number) => `${((y - first) / span) * 100}%`;
    hFrom.style.left = pos(from);
    hTo.style.left = pos(to + 1);
    band.style.left = pos(from);
    band.style.width = `${((to - from + 1) / span) * 100}%`;
    for (const [hd, y] of [[hFrom, from], [hTo, to]] as const) {
      hd.setAttribute("aria-valuenow", String(y));
      hd.setAttribute("aria-valuetext", String(y));
    }
    fill(total, h("strong", {}, num(inRange)), ` permits, ${yearsText(from, to)}`);
    if (!timer) play.textContent = "Replay";

    const r = ageRanges(to);
    const clampText = (a: number, b: number) => yearsText(Math.max(a, from), b);
    fill(
      key,
      h("span", {}, ageMark("recent"), clampText(r.recent[0], r.recent[1])),
      r.back[1] >= from ? h("span", { class: "older" }, ageMark("back"), clampText(r.back[0], r.back[1])) : null,
      r.oldBefore > from ? h("span", { class: "older" }, ageMark("old"), `${from} to ${r.oldBefore - 1}`) : null,
    );
    hint.textContent =
      from < sparse
        ? `Before ${sparse} the city's records are thin (hatched), so those years look quieter than they were.`
        : dragged
          ? ""
          : "Drag either end of the chart to rewind the city.";
  }

  function showZoomNote(zoomedOut: boolean) {
    zoomNote.classList.toggle("off", !zoomedOut);
    key.classList.toggle("zoomed-out", zoomedOut);
  }

  function showReset(visible: boolean, onReset: () => void) {
    reset.hidden = !visible;
    reset.onclick = () => {
      stop();
      onReset();
    };
  }

  const describe = () => total.textContent ?? "";

  return { el, update, showReset, showZoomNote, describe };
}
