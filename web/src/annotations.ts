import L from "leaflet";
import { h } from "./dom";
import { feet } from "./format";
import type { LatLng } from "./geo";
import { cloudPath, northArrow, strokeIcon } from "./marks";

const NS = "http://www.w3.org/2000/svg";
const MARK_PX = 26;

export class Annotations {
  private layer = L.layerGroup();

  constructor(
    map: L.Map,
    private shape: "cloud" | "ring",
  ) {
    map.createPane("annotations").style.zIndex = "640";
    this.layer.addTo(map);
  }

  clear() {
    this.layer.clearLayers();
  }

  mark(at: LatLng, tag: string | undefined) {
    const size = MARK_PX * 2 + 8;
    const c = size / 2;
    const s = document.createElementNS(NS, "svg");
    s.setAttribute("width", String(size));
    s.setAttribute("height", String(size));
    s.setAttribute("class", "mark-shape");
    if (this.shape === "cloud") {
      const p = document.createElementNS(NS, "path");
      p.setAttribute("d", cloudPath(c, c, MARK_PX));
      s.append(p);
    } else {
      for (const r of [MARK_PX - 6, MARK_PX - 10]) {
        const ring = document.createElementNS(NS, "circle");
        ring.setAttribute("cx", String(c));
        ring.setAttribute("cy", String(c));
        ring.setAttribute("r", String(r));
        s.append(ring);
      }
    }
    const icon = L.divIcon({
      className: `place-mark ${this.shape}`,
      html: h("div", { "aria-hidden": "true" }, s, tag ? h("span", { class: "tag" }, tag) : null),
      iconSize: [size, size],
      iconAnchor: [c, c],
    });
    L.marker([at.lat, at.lng], { icon, pane: "annotations", interactive: false, keyboard: false }).addTo(this.layer);
  }

  /** The nearby circle's radius, lettered at its east edge like a dimension */
  dimension(at: LatLng, radiusM: number) {
    const east = at.lng + radiusM / (111_320 * Math.cos((at.lat * Math.PI) / 180));
    const icon = L.divIcon({
      className: "dimension",
      html: h("span", { "aria-hidden": "true" }, `R ${feet(radiusM)}`),
      iconSize: [0, 0],
    });
    L.marker([at.lat, east], { icon, pane: "annotations", interactive: false, keyboard: false }).addTo(this.layer);
  }
}

// What any map you read should carry
export function mapDevices(map: L.Map) {
  L.control.scale({ position: "bottomleft", imperial: true, metric: false, maxWidth: 140 }).addTo(map);
  const North = L.Control.extend({
    onAdd: () => h("div", { class: "north", "aria-hidden": "true" }, northArrow(), h("span", {}, "N")),
  });
  new North({ position: "topright" }).addTo(map);
  const box = map.getContainer();
  for (const [sel, icon] of [[".leaflet-control-zoom-in", "plus"], [".leaflet-control-zoom-out", "minus"]] as const) {
    box.querySelector(sel)?.replaceChildren(strokeIcon(icon));
  }
}
