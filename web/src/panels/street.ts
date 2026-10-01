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
  const total = blocks.reduce((a, p) => a + summary.count[p]!, 0);
  const busy = blocks.filter((p) => summary.count[p]! > 0).length;
  const max = Math.max(1, ...blocks.map((p) => summary.count[p]!));
  return h(
    "section",
    { class: "focus" },
    panelHead(prettyStreet(site.places.streets[street]!), () => ctx.go(null)),
    h("p", { class: "sub" }, `${plural(total, "permit")} on ${plural(busy, "block")}, in address order.`),
    h(
      "ul",
      { class: "bars street" },
      ...blocks.map((p) =>
        h(
          "li",
          {},
          h(
            "button",
            { type: "button", onclick: () => ctx.go({ kind: "place", place: p }, { move: true }) },
            h("span", { class: "label" }, `${site.places.block[p]} block`),
            h("span", { class: "track" }, h("i", { style: `width: ${Math.round((100 * summary.count[p]!) / max)}%` })),
            h("span", { class: "n" }, num(summary.count[p]!)),
          ),
        ),
      ),
    ),
  );
}

export interface Station {
  block: number;
  n: number;
  open: () => void;
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
        "aria-label": `${s.block} block, ${plural(s.n, "permit")}`,
        title: `${s.block} block: ${plural(s.n, "permit")}`,
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
  return h(
    "div",
    { class: "profile" },
    bars,
    h("div", { class: "profile-axis", "aria-hidden": "true" }, ...stations.map((s, i) => h("span", {}, ticks[i] ? label(s.block) : ""))),
  );
}
