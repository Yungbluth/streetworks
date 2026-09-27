import L from "leaflet";
import { h } from "../../src/dom";
import { feet } from "../../src/format";
import type { LatLng } from "../../src/geo";
import { cloudPath } from "./marks";

const NS = "http://www.w3.org/2000/svg";
const CLOUD_PX = 26;

export class Redline {
  private layer = L.layerGroup();

  constructor(private map: L.Map) {
    map.createPane("redline").style.zIndex = "640";
    this.layer.addTo(map);
  }

  clear() {
    this.layer.clearLayers();
  }

  // a cloud around a place, tagged with its latest revision number
  cloud(at: LatLng, tag: string | undefined) {
    const size = CLOUD_PX * 2 + 8;
    const s = document.createElementNS(NS, "svg");
    s.setAttribute("width", String(size));
    s.setAttribute("height", String(size));
    s.setAttribute("class", "cloud");
    const p = document.createElementNS(NS, "path");
    p.setAttribute("d", cloudPath(size / 2, size / 2, CLOUD_PX));
    s.append(p);
    const icon = L.divIcon({
      className: "redline-mark",
      html: h("div", { "aria-hidden": "true" }, s, tag ? h("span", { class: "delta" }, tag) : null),
      iconSize: [size, size],
      iconAnchor: [size / 2, size / 2],
    });
    L.marker([at.lat, at.lng], { icon, pane: "redline", interactive: false, keyboard: false }).addTo(this.layer);
  }

  dimension(at: LatLng, radiusM: number) {
    const east = at.lng + radiusM / (111_320 * Math.cos((at.lat * Math.PI) / 180));
    const icon = L.divIcon({
      className: "dimension",
      html: h("span", { "aria-hidden": "true" }, `R ${feet(radiusM).toUpperCase()}`),
      iconSize: [0, 0],
    });
    L.marker([at.lat, east], { icon, pane: "redline", interactive: false, keyboard: false }).addTo(this.layer);
  }
}
