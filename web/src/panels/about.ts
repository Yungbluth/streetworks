import type { Index } from "../data";
import { h } from "../dom";
import { dateText, num } from "../format";

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
      h("li", {}, `${num(q.unplaced)} permits have no location at all, so they aren't on the map.`),
      h("li", {}, `${num(q.no_date)} permits have no usable date, so they never appear on the map.`),
      h("li", {}, `${num(q.no_work)} permits were voided, withdrawn or denied, or are still in review. They're hidden unless you include them.`),
      h("li", {}, "A permit near a corner may be filed under either street. Work done without a permit never appears."),
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
