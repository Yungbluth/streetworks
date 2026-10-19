import type L from "leaflet";
import { h } from "../dom";
import { num, placeName } from "../format";
import { showAll } from "./common";
import { digKinds, digsByPlace, type Ctx } from "../model";

// For when nothing is selected
export function busiestPanel(ctx: Ctx, view: L.LatLngBounds): HTMLElement {
  const { places } = ctx.site;
  const { count } = ctx.summary;
  // With no digging kind shown, rank by permits instead, and say so
  const digging = digKinds(ctx.site, ctx.filter).size > 0;
  const digs = digging ? digsByPlace(ctx.site, ctx.filter) : count;
  // Among the dots on the map now, so the list matches what's drawn.
  const shown: number[] = [];
  for (let p = 0; p < places.kind.length; p++) {
    if (count[p] && view.contains([places.lat[p]! / 1e5, places.lng[p]! / 1e5])) shown.push(p);
  }
  const top = shown
    .filter((p) => digs[p])
    .sort((a, b) => digs[b]! - digs[a]! || count[b]! - count[a]!)
    .slice(0, 8);
  return h(
    "section",
    { class: "focus" },
    h("h2", { tabindex: "-1" }, digging ? "Most dug-up places in view" : "Busiest places in view"),
    ...(top.length
      ? [
          h(
            "ol",
            { class: "busiest" },
            ...top.map((p) =>
              h(
                "li",
                {},
                h("button", { type: "button", class: "linkish", onclick: () => ctx.go({ kind: "place", place: p }, { move: true }) }, placeName(places, p)),
                h("span", { class: "n" }, num(digs[p]!)),
              ),
            ),
          ),
          h("p", { class: "quiet" }, digging ? "Counts are times dug up: Excavation and Combo permits." : "Counts are permits; no digging kind is shown."),
        ]
      : shown.length
        ? [h("p", { class: "sub" }, "Nothing dug up on the map in this view. Select a dot to see what was filed there.")]
        : [
            h(
              "div",
              {},
              h("p", { class: "sub" }, "No permits match the filters in this part of the map."),
              ctx.filter.cats.size < ctx.site.index.categories.length ? showAll(ctx.showAllKinds) : null,
            ),
          ]),
    h("p", { class: "quiet" }, "Search above, or select a dot to see everything filed there."),
  );
}
