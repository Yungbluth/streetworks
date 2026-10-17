import type L from "leaflet";
import { h } from "../dom";
import { num, placeName } from "../format";
import { ageMark, inkDot, swatch } from "../marks";
import { showAll } from "./common";
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
    h("h2", { tabindex: "-1" }, "Busiest places in view"),
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
      : h(
          "div",
          {},
          h("p", { class: "sub" }, "No permits match the filters in this part of the map."),
          ctx.filter.cats.size < ctx.site.index.categories.length ? showAll(ctx.showAllKinds) : null,
        ),
    h("p", { class: "quiet" }, "Search above, or select a dot to see everything filed there."),
    dotKey([...ctx.site.index.quality.categories].sort((a, b) => b.n - a.n).map((k) => k.key)),
  );
}

/** What one dot says, drawn: its paint, its size and its shape */
function dotKey(commonest: string[]): HTMLElement {
  const row = (term: string, marks: SVGElement[], text: string) =>
    [h("dt", {}, h("span", { class: "marks" }, ...marks), term), h("dd", {}, text)];
  return h(
    "section",
    { class: "dot-key" },
    h("h3", {}, "Reading a dot"),
    h(
      "dl",
      {},
      ...row("Color", commonest.slice(0, 3).map((c) => swatch(c, 12)), "The kind of work filed there most often."),
      ...row("Size", [inkDot(6), inkDot(10), inkDot(14)], "How many permits."),
      ...row("Shape", [ageMark("recent"), ageMark("back"), ageMark("old")], "How recent: solid for the last three years, a ring for the seven before, a speck for older."),
    ),
  );
}
