import { PlaceKind } from "../data";
import { h } from "../dom";
import { num, plural, prettyStreet } from "../format";
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
