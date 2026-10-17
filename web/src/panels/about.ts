import type { Index } from "../data";
import { h } from "../dom";
import { dateText, num } from "../format";
import { swatch } from "../marks";
import { kindMeanings } from "./filters";

export function aboutPanel(idx: Index): HTMLElement {
  const q = idx.quality;
  const pct = (n: number) => (q.unique_permits ? `${((100 * n) / q.unique_permits).toFixed(1)}%` : "0%");
  const fetched = idx.sources.map((s) => s.fetched.slice(0, 10)).sort().at(-1);
  return h(
    "details",
    { class: "about" },
    h("summary", {}, "About this data"),
    h(
      "p",
      {},
      `${num(q.unique_permits)} permits from the City of Tucson's right-of-way records, ${q.date_min.slice(0, 4)} to ${q.date_max.slice(0, 4)}. `,
      fetched ? `Downloaded ${dateText(fetched)}.` : "",
    ),
    h(
      "ul",
      {},
      h("li", {}, `Before ${idx.sparseBefore} the city's records are thin, so those years look quieter than they were.`),
      h("li", {}, `${num(q.no_block)} permits (${pct(q.no_block)}) have no street number. They're placed at the intersection or spot the city recorded instead of on a block.`),
      q.unplaced ? h("li", {}, `${num(q.unplaced)} permits have no location at all, so they aren't on the map.`) : null,
      q.no_date ? h("li", {}, `${num(q.no_date)} permits have no usable date, so they never appear on the map.`) : null,
      h("li", {}, `${num(q.no_work)} permits were voided, withdrawn or denied, or are still in review. They're hidden unless you include them.`),
      h("li", {}, "A permit near a corner may be filed under either street. Work done without a permit never appears."),
    ),
    h("h3", { id: "reading-the-map", tabindex: "-1" }, "Reading the map"),
    h(
      "ul",
      {},
      h("li", {}, "A dot's color is the kind of work filed there most often. Bigger dots mean more permits."),
      h("li", {}, "Its shape is how recent the newest permit is, within the years you've chosen: solid for the last three years, a ring for the seven before that, a faint speck for older."),
      h("li", {}, "Zoomed out over the whole city, only solid dots show. Rings and specks appear when you zoom in."),
    ),
    h("h3", { id: "kinds-meaning", tabindex: "-1" }, "What the kinds mean"),
    h(
      "dl",
      { class: "kind-meanings" },
      ...kindMeanings
        .filter(([name]) => idx.categories.includes(name))
        .flatMap(([name, meaning]) => [h("dt", {}, swatch(name, 10), name), h("dd", {}, meaning)]),
    ),
    h("p", { class: "quiet" }, "Civic event and Small wireless start off: they're mostly street closures and equipment on poles, not digging."),
    h("h3", { id: "how-it-works", tabindex: "-1" }, "How it works"),
    h(
      "p",
      {},
      "A Go program downloads the City of Tucson's open right-of-way records every week and writes them as a few compressed files. Everything else happens in your browser: searching, filtering and counting the permits takes milliseconds. There's no server, no geocoder and no API key.",
    ),
    h(
      "p",
      { class: "quiet" },
      "Data: City of Tucson. Map: ",
      h("a", { href: "https://www.openstreetmap.org/copyright" }, "OpenStreetMap"),
      " contributors, drawn with ",
      h("a", { href: "https://leafletjs.com" }, "Leaflet"),
      ".",
    ),
  );
}
