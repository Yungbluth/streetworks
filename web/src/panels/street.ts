import { PlaceKind } from "../data";
import { h } from "../dom";
import { num, plural, prettyStreet } from "../format";
import { stationTicks } from "../marks";
import { digKinds, digsByPlace, type Ctx } from "../model";
import { panelHead } from "./common";

export function streetBlocks(ctx: Ctx, street: number): number[] {
  const { places } = ctx.site;
  const out: number[] = [];
  for (let p = 0; p < places.kind.length; p++) {
    if (places.kind[p] === PlaceKind.Block && places.street[p] === street) out.push(p);
  }
  return out.sort((a, b) => places.block[a]! - places.block[b]!);
}

export function streetPanel(ctx: Ctx, street: number): HTMLElement {
  const { site, summary } = ctx;
  const blocks = streetBlocks(ctx, street);
  const busy = blocks.filter((p) => summary.count[p]! > 0);
  const total = busy.reduce((a, p) => a + summary.count[p]!, 0);
  const open = (p: number) => () => ctx.go({ kind: "place", place: p }, { move: true });
  // The busiest blocks rank by digs, as the first view's list does, while a digging kind is shown
  const digging = digKinds(site, ctx.filter).size > 0;
  const rank = digging ? digsByPlace(site, ctx.filter) : summary.count;
  const dug = digging ? busy.reduce((a, p) => a + rank[p]!, 0) : 0;
  const top = busy.filter((p) => rank[p]).sort((a, b) => rank[b]! - rank[a]! || summary.count[b]! - summary.count[a]!).slice(0, 8);
  // the profile, like the list, counts digs while a digging kind is shown
  const bars = stretches(blocks.map((p) => ({ block: site.places.block[p]!, n: rank[p]!, open: open(p) })));
  const unit = digging ? "dig" : "permit";
  const barTotal = digging ? dug : total;
  const peak = bars.reduce((best, b) => (b.n > best.n ? b : best), bars[0]!);
  return h(
    "section",
    { class: "focus" },
    panelHead(prettyStreet(site.places.streets[street]!), () => ctx.go(null)),
    h(
      "p",
      { class: "sub" },
      total
        ? `${plural(total, "permit")} on ${plural(busy.length, "block")}${digging ? `; dug up ${dug === 1 ? "once" : dug === 2 ? "twice" : `${num(dug)} times`}` : ""}.`
        : "No permits on this street match the filters.",
    ),
    h("h3", {}, "Blocks in address order"),
    barTotal ? h("p", { class: "quiet" }, `Busiest ${peak.name ? "stretch" : "block"}: the ${peak.name ?? `${peak.block} block`}, ${plural(peak.n, unit)} of ${num(barTotal)}.`) : null,
    streetProfile(bars, String, "Block", unit),
    ...(top.length
      ? [
          h("h3", {}, bars.length < blocks.length ? "Busiest single blocks" : "Busiest blocks"),
          h(
            "ol",
            { class: "busiest" },
            ...top.map((p) =>
              h(
                "li",
                {},
                h("button", { type: "button", class: "linkish", onclick: open(p) }, `${site.places.block[p]} block`),
                h("span", { class: "n" }, plural(rank[p]!, digging ? "dig" : "permit")),
              ),
            ),
          ),
        ]
      : []),
  );
}

export interface Station {
  block: number;
  n: number;
  open: () => void;
  // for a stretch of several blocks, such as "3000–3200 blocks"
  name?: string;
  opens?: number;
  /** A stretch of addresses with no blocks on record: an empty slot, not a control */
  gap?: boolean;
}

// more bars than this and each is too thin to see or point at
const MAX_BARS = 40;

export function stretches(blocks: Station[]): Station[] {
  if (!blocks.length) return blocks;
  const first = Math.min(...blocks.map((b) => b.block));
  const last = Math.max(...blocks.map((b) => b.block));
  const span = (last - first) / 100 + 1;
  const width = [1, 2, 3, 5, 10, 20, 50, 100].find((w) => Math.ceil(span / w) <= MAX_BARS) ?? 100;
  const step = width * 100;
  const groups = new Map<number, Station[]>();
  for (const b of blocks) {
    const at = first + Math.floor((b.block - first) / step) * step;
    groups.set(at, [...(groups.get(at) ?? []), b]);
  }
  const out: Station[] = [];
  // Every stretch from first to last, empty ones included
  for (let at = first; at <= last; at += step) {
    const members = groups.get(at);
    const name = width > 1 ? `${at}–${at + step - 100} blocks` : undefined;
    if (!members) {
      out.push({ block: at, n: 0, open: () => {}, gap: true, name });
    } else if (width === 1) {
      out.push(members[0]!);
    } else {
      const busiest = members.reduce((best, b) => (b.n > best.n ? b : best), members[0]!);
      out.push({ block: at, n: members.reduce((sum, b) => sum + b.n, 0), open: busiest.open, opens: busiest.block, name });
    }
  }
  return out;
}

// a street as a profile: its blocks in address order, each a bar as tall as its permit count
export function streetProfile(stations: Station[], label: (block: number) => string, unit?: string, counts = "permit"): HTMLElement {
  const max = Math.max(1, ...stations.map((s) => s.n));
  const busiest = stations.reduce((best, s, i) => (s.n > stations[best]!.n ? i : best), 0);
  const ticks = stationTicks(stations.length);
  // One slot per station; a gap is an empty slot, not a control
  const buttons: HTMLElement[] = stations.map((s, i) =>
    s.gap ? h("span", { class: "station-gap", "aria-hidden": "true" }) : h(
      "button",
      {
        type: "button", class: "station", tabindex: i === busiest ? 0 : -1, onclick: () => press(i),
        style: `--h: ${(s.n / max) * 100}%`,
        "aria-label": `${s.name ?? `${s.block} block`}, ${plural(s.n, counts)}${s.opens === undefined ? "" : `, opens the ${s.opens} block`}`,
        title: `${s.name ?? `${s.block} block`}: ${plural(s.n, counts)}${s.opens === undefined ? "" : `, opens the ${s.opens} block`}`,
      },
      h("span", { class: "station-bar", "aria-hidden": "true" }),
    ),
  );
  const bars = h("div", { class: "profile-bars", role: "group", "aria-label": "Blocks in address order" }, ...buttons);
  // a finger can't hover, and a bar is a few pixels wide
  let touching = false;
  let armed = -1;
  bars.addEventListener("pointerdown", (e) => (touching = e.pointerType === "touch"));
  function press(i: number) {
    if (touching && armed !== i) {
      armed = i;
      buttons[near]?.classList.remove("near");
      near = i;
      buttons[i]!.classList.add("near");
      readout.textContent = say(i);
      return;
    }
    armed = -1;
    stations[i]!.open();
  }
  const real = buttons.filter((b) => b.tagName === "BUTTON");
  bars.addEventListener("keydown", (e) => {
    const at = real.indexOf(e.target as HTMLElement);
    if (at < 0) return;
    const to = { ArrowLeft: at - 1, ArrowRight: at + 1, Home: 0, End: real.length - 1 }[e.key];
    if (to === undefined || to < 0 || to >= real.length) return;
    e.preventDefault();
    real[at]!.tabIndex = -1;
    real[to]!.tabIndex = 0;
    real[to]!.focus();
  });
  const readout = h("p", { class: "profile-readout", "aria-hidden": "true" }, "\u00a0");
  const at = (clientX: number) => {
    const r = bars.getBoundingClientRect();
    return Math.min(stations.length - 1, Math.max(0, Math.floor(((clientX - r.left) / r.width) * stations.length)));
  };
  const say = (i: number) => {
    const s = stations[i]!;
    if (s.gap) return `${s.name ?? `${s.block} block`} · no blocks on record`;
    return `${s.name ?? `${s.block} block`} · ${plural(s.n, counts)}${s.opens === undefined ? "" : ` · opens the ${s.opens} block`}`;
  };
  let near = -1;
  bars.addEventListener("pointermove", (e) => {
    const i = at(e.clientX);
    if (i === near) return;
    buttons[near]?.classList.remove("near");
    near = i;
    buttons[i]!.classList.add("near");
    readout.textContent = say(i);
  });
  bars.addEventListener("pointerleave", () => {
    if (touching) return;
    buttons[near]?.classList.remove("near");
    near = -1;
    readout.textContent = "\u00a0";
  });
  // the keyboard gets the same readout for the block it's on
  bars.addEventListener("focusin", (e) => {
    const i = buttons.indexOf(e.target as HTMLElement);
    if (i >= 0) readout.textContent = say(i);
  });
  bars.addEventListener("focusout", () => {
    if (near < 0) readout.textContent = "\u00a0";
  });
  bars.addEventListener("click", (e) => {
    if (e.target === bars) stations[at(e.clientX)]!.open();
  });
  return h(
    "div",
    { class: "profile" },
    readout,
    // the tallest bar's count, so the heights have a scale
    h("div", { class: "profile-plot" }, h("span", { class: "profile-max", "aria-hidden": "true" }, num(max)), bars),
    // The first label carries the unit ("Block 0"), where there's room for it
    h("div", { class: "profile-axis", "aria-hidden": "true" }, ...stations.map((s, i) => h("span", {}, ticks[i] ? `${unit && i === 0 ? `${unit} ` : ""}${label(s.block)}` : ""))),
  );
}
