import { h } from "./dom";

// ArcGIS bookkeeping fields that mean nothing to a reader
const hidden = /^(objectid|globalid|shape|datasource|fid)/i;

export function overlayPopup(props: Record<string, unknown> | null): HTMLElement {
  const rows = Object.entries(props ?? {})
    .filter(([k, v]) => v !== null && v !== undefined && v !== "" && !hidden.test(k))
    .slice(0, 12);
  if (!rows.length) return h("p", {}, "No details for this segment.");
  return h("table", { class: "popup" }, ...rows.map(([k, v]) => h("tr", {}, h("th", {}, k), h("td", {}, String(v)))));
}
