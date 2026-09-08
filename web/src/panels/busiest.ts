import type L from "leaflet";
import { h } from "../dom";
import { num, placeName } from "../format";
import type { Ctx } from "../model";

export function busiestPanel(ctx: Ctx, view: L.LatLngBounds): HTMLElement {
  const { places } = ctx.site;
  const { count } = ctx.summary;
  const top: number[] = [];
  for (let p = 0; p < places.kind.length; p++) {
    if (!count[p] || !view.contains([places.lat[p]! / 1e5, places.lng[p]! / 1e5])) continue;
    top.push(p);
    if (top.length > 64) {
      top.sort((a, b) => count[b]! - count[a]!);
      top.length = 8;
    }
  }
  top.sort((a, b) => count[b]! - count[a]!);
  return h(
    "section",
    { class: "focus" },
    h("h2", {}, "Busiest places in view"),
    top.length
      ? h(
          "ol",
          { class: "busiest" },
          ...top.slice(0, 8).map((p) =>
            h(
              "li",
              {},
              h("button", { type: "button", class: "linkish", onclick: () => ctx.go({ kind: "place", place: p }, { move: true }) }, placeName(places, p)),
              h("span", { class: "n" }, num(count[p]!)),
            ),
          ),
        )
      : h("p", { class: "sub" }, "No permits match the filters in this part of the map."),
    h("p", { class: "quiet" }, "Search above, or select a dot to see everything filed there."),
  );
}
