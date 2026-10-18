import { PlaceKind } from "../data";
import { h } from "../dom";
import { num, plural, prettyStreet } from "../format";
import { stationTicks } from "../marks";
import type { Ctx } from "../model";
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
  const top = [...busy].sort((a, b) => summary.count[b]! - summary.count[a]!).slice(0, 8);
  const bars = stretches(blocks.map((p) => ({ block: site.places.block[p]!, n: summary.count[p]!, open: open(p) })));
  const peak = bars.reduce((best, b) => (b.n > best.n ? b : best), bars[0]!);
  return h(
    "section",
    { class: "focus" },
    panelHead(prettyStreet(site.places.streets[street]!), () => ctx.go(null)),
    h("p", { class: "sub" }, total ? `${plural(total, "permit")} on ${plural(busy.length, "block")}.` : "No permits on this street match the filters."),
    h("h3", {}, "Blocks in address order"),
    total ? h("p", { class: "quiet" }, `Busiest: the ${peak.name ?? `${peak.block} block`}, ${plural(peak.n, "permit")} of ${num(total)}.`) : null,
    streetProfile(bars, String),
    ...(top.length
      ? [
          h("h3", {}, "Busiest blocks"),
          h(
            "ol",
            { class: "busiest" },
            ...top.map((p) =>
              h(
                "li",
                {},
                h("button", { type: "button", class: "linkish", onclick: open(p) }, `${site.places.block[p]} block`),
                h("span", { class: "n" }, num(summary.count[p]!)),
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
}

// more bars than this and each is too thin to see or point at
const MAX_BARS = 40;

export function stretches(blocks: Station[]): Station[] {
  if (blocks.length <= MAX_BARS) return blocks;
  const first = Math.min(...blocks.map((b) => b.block));
  const last = Math.max(...blocks.map((b) => b.block));
  const span = (last - first) / 100 + 1;
  const width = [2, 3, 5, 10, 20, 50, 100].find((w) => Math.ceil(span / w) <= MAX_BARS) ?? 100;
  const groups = new Map<number, Station[]>();
  for (const b of blocks) {
    const at = first + Math.floor((b.block - first) / (width * 100)) * width * 100;
    groups.set(at, [...(groups.get(at) ?? []), b]);
  }
  return [...groups]
    .sort((a, b) => a[0] - b[0])
    .map(([at, members]) => {
      const busiest = members.reduce((best, b) => (b.n > best.n ? b : best), members[0]!);
      return {
        block: at,
        n: members.reduce((sum, b) => sum + b.n, 0),
        open: busiest.open,
        name: `${at}–${at + (width - 1) * 100} blocks`,
      };
    });
}

// a street as a profile: its blocks in address order, each a bar as tall as its permit count
export function streetProfile(stations: Station[], label: (block: number) => string): HTMLElement {
  const max = Math.max(1, ...stations.map((s) => s.n));
  const busiest = stations.reduce((best, s, i) => (s.n > stations[best]!.n ? i : best), 0);
  const ticks = stationTicks(stations.length);
  const buttons = stations.map((s, i) =>
    h(
      "button",
      {
        type: "button", class: "station", tabindex: i === busiest ? 0 : -1, onclick: s.open,
        style: `--h: ${(s.n / max) * 100}%`,
        "aria-label": `${s.name ?? `${s.block} block`}, ${plural(s.n, "permit")}`,
        title: `${s.name ?? `${s.block} block`}: ${plural(s.n, "permit")}`,
      },
      h("span", { class: "station-bar", "aria-hidden": "true" }),
    ),
  );
  const bars = h("div", { class: "profile-bars", role: "group", "aria-label": "Blocks in address order" }, ...buttons);
  bars.addEventListener("keydown", (e) => {
    const at = buttons.indexOf(e.target as HTMLButtonElement);
    if (at < 0) return;
    const to = { ArrowLeft: at - 1, ArrowRight: at + 1, Home: 0, End: buttons.length - 1 }[e.key];
    if (to === undefined || to < 0 || to >= buttons.length) return;
    e.preventDefault();
    buttons[at]!.tabIndex = -1;
    buttons[to]!.tabIndex = 0;
    buttons[to]!.focus();
  });
  const readout = h("p", { class: "profile-readout", "aria-hidden": "true" }, "\u00a0");
  const at = (clientX: number) => {
    const r = bars.getBoundingClientRect();
    return Math.min(stations.length - 1, Math.max(0, Math.floor(((clientX - r.left) / r.width) * stations.length)));
  };
  const say = (i: number) => `${stations[i]!.name ?? `${stations[i]!.block} block`} · ${plural(stations[i]!.n, "permit")}`;
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
    buttons[near]?.classList.remove("near");
    near = -1;
    readout.textContent = "\u00a0";
  });
  // the keyboard gets the same readout for the block it's on
  bars.addEventListener("focusin", (e) => {
    const i = buttons.indexOf(e.target as HTMLButtonElement);
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
    h("div", { class: "profile-axis", "aria-hidden": "true" }, ...stations.map((s, i) => h("span", {}, ticks[i] ? label(s.block) : ""))),
  );
}
