import L from "leaflet";
import type { Index, Places, TileSource } from "./data";
import { arriving, dotStyle, drawnAge, faded } from "./dots";
import { overlayPopup } from "./overlay";
import { ageOf, type PlaceSummary } from "./filters";
import type { LatLng } from "./geo";
import { cssVar, edgeFor, paint } from "./paint";

interface Drawn {
  place: Int32Array;
  x: Float32Array;
  y: Float32Array;
  r: Float32Array;
  n: number;
}

class DotLayer extends L.Layer {
  private canvas = document.createElement("canvas");
  private drawn: Drawn = { place: new Int32Array(0), x: new Float32Array(0), y: new Float32Array(0), r: new Float32Array(0), n: 0 };
  summary?: PlaceSummary;
  end = 0;
  highlighted = new Set<number>();
  /** true while a replay runs, and after one ends: older work stays on as specks at any zoom */
  filling = false;
  /** true only while a replay runs: the year just added gets a halo */
  arrivals = false;

  constructor(
    private places: Places,
    private categories: string[],
  ) {
    super();
    this.canvas.className = "dots leaflet-zoom-hide";
  }

  // leaflet calls this when the layer goes on the map
  override onAdd(map: L.Map) {
    map.getPane("dots")!.append(this.canvas);
    map.on("moveend zoomend resize viewreset", this.redraw, this);
    this.redraw();
    return this;
  }

  override onRemove(map: L.Map) {
    this.canvas.remove();
    map.off("moveend zoomend resize viewreset", this.redraw, this);
    return this;
  }

  // a hundred-block is about 100 m long
  private radius(count: number, metersPerPx: number) {
    const base = Math.min(9, Math.max(1.4, (30 / metersPerPx) * 0.42));
    return Math.min(16, base * (1 + 0.28 * Math.log(Math.max(1, count))));
  }

  redraw() {
    const map = this._map;
    if (!map || !this.summary) return;
    const size = map.getSize();
    const dpr = window.devicePixelRatio || 1;
    const c = this.canvas;
    c.width = size.x * dpr;
    c.height = size.y * dpr;
    c.style.width = `${size.x}px`;
    c.style.height = `${size.y}px`;
    L.DomUtil.setPosition(c, map.containerPointToLayerPoint([0, 0]));
    const ctx = c.getContext("2d")!;
    ctx.scale(dpr, dpr);

    const { places, summary } = this;
    const zoom = map.getZoom();
    const metersPerPx = (156543.03 * Math.cos((map.getCenter().lat * Math.PI) / 180)) / 2 ** map.getZoom();
    const colors = this.categories.map(paint);
    const edges = this.categories.map(edgeFor);
    const bounds = map.getBounds().pad(0.05);

    const order: number[] = [];
    for (let p = 0; p < places.kind.length; p++) {
      if (!summary.count[p]) continue;
      if (!drawnAge(ageOf(summary.newest[p]!, this.end), zoom, this.filling) && !this.highlighted.has(p)) continue;
      const lat = places.lat[p]! / 1e5;
      const lng = places.lng[p]! / 1e5;
      if (lat < bounds.getSouth() || lat > bounds.getNorth() || lng < bounds.getWest() || lng > bounds.getEast()) continue;
      order.push(p);
    }
    order.sort((a, b) => summary.newest[a]! - summary.newest[b]!);

    const d: Drawn = {
      place: new Int32Array(order.length),
      x: new Float32Array(order.length),
      y: new Float32Array(order.length),
      r: new Float32Array(order.length),
      n: order.length,
    };
    order.forEach((p, i) => {
      const pt = map.latLngToContainerPoint([places.lat[p]! / 1e5, places.lng[p]! / 1e5]);
      const cat = summary.cat[p]!;
      const real = ageOf(summary.newest[p]!, this.end);
      const age = drawnAge(real, zoom, this.filling) ?? real;
      const own = dotStyle(age, this.radius(summary.count[p]!, metersPerPx), colors[cat]!, edges[cat]);
      // While a street is open, the rest of the map steps back
      const style = this.highlighted.size && !this.highlighted.has(p) ? faded(own) : own;
      d.place[i] = p;
      d.x[i] = pt.x;
      d.y[i] = pt.y;
      d.r[i] = Math.max(style.r, 4); // a comfortable target to click, whatever is drawn

      if (arriving(summary.newest[p]!, this.end, this.arrivals)) {
        ctx.globalAlpha = 0.35;
        ctx.beginPath();
        ctx.arc(pt.x, pt.y, style.r + 3, 0, Math.PI * 2);
        ctx.lineWidth = 3;
        ctx.strokeStyle = colors[cat]!;
        ctx.stroke();
      }
      ctx.globalAlpha = style.alpha;
      ctx.beginPath();
      ctx.arc(pt.x, pt.y, style.r, 0, Math.PI * 2);
      if (style.outline) {
        // Wider than the mark by the hairline on each side; the paint goes on top.
        const w = style.outlineWidth ?? 1;
        ctx.lineWidth = (style.stroke ? (style.lineWidth ?? 1) : 0) + 2 * w;
        ctx.strokeStyle = style.outline;
        ctx.stroke();
      }
      if (style.fill) {
        ctx.fillStyle = style.fill;
        ctx.fill();
      }
      if (style.stroke) {
        ctx.lineWidth = style.lineWidth ?? 1;
        ctx.strokeStyle = style.stroke;
        ctx.stroke();
      }
      ctx.globalAlpha = 1;
    });

    this.drawn = d;
  }

  nearest(pt: L.Point, maxPx: number): number | undefined {
    const d = this.drawn;
    let best: number | undefined;
    let bestD = maxPx * maxPx;
    for (let i = 0; i < d.n; i++) {
      const dx = d.x[i]! - pt.x;
      const dy = d.y[i]! - pt.y;
      if (dx * dx + dy * dy <= bestD) {
        bestD = dx * dx + dy * dy;
        best = d.place[i];
      }
    }
    return best;
  }

  hit(pt: L.Point): number | undefined {
    const d = this.drawn;
    for (let i = d.n - 1; i >= 0; i--) {
      const dx = d.x[i]! - pt.x;
      const dy = d.y[i]! - pt.y;
      const r = d.r[i]! + 3;
      if (dx * dx + dy * dy <= r * r) return d.place[i];
    }
    return undefined;
  }
}

export interface MapEvents {
  pick(place: number): void;
  hover(place: number | undefined, at: L.Point): void;
  moved(): void;
  /** The zoom the map is heading to, as soon as it's known */
  zoomed(zoom: number): void;
  // While the map has keyboard focus: the place Enter would open (undefined if none)
  aimed(place: number | undefined, active: boolean): void;
}

export class PermitMap {
  readonly map: L.Map;
  private dots: DotLayer;
  private tiles?: L.TileLayer;
  private focus = L.layerGroup();
  private lines = L.canvas({ pane: "lines" });
  private overlays = new Map<string, L.GeoJSON>();

  constructor(
    el: HTMLElement,
    private site: { index: Index; places: Places },
    events: MapEvents,
  ) {
    const [s, w, n, e] = site.index.bounds;
    this.map = L.map(el, { minZoom: 9, maxZoom: 19, zoomControl: false, attributionControl: true });
    L.control.zoom({ position: "topleft" }).addTo(this.map);
    this.map.attributionControl.setPrefix('<a href="https://leafletjs.com">Leaflet</a>');
    this.map.createPane("lines").style.zIndex = "410";
    this.map.createPane("dots").style.zIndex = "420";
    this.map.createPane("marks").style.zIndex = "430";
    if (s || n) this.map.fitBounds([[s, w], [n, e]]);
    else this.map.setView([32.2226, -110.9747], 11);

    // The map's box changes size after start-up (the year strip arrives, the window resizes)
    new ResizeObserver(() => this.map.invalidateSize()).observe(el);

    if (matchMedia("(pointer: coarse)").matches) {
      this.map.dragging.disable();
      const hint = L.DomUtil.create("p", "map-hint", el);
      hint.textContent = "Use two fingers to move the map";
      hint.hidden = true;
      let timer: number | undefined;
      el.addEventListener(
        "touchmove",
        (e) => {
          if (e.touches.length !== 1) return void (hint.hidden = true);
          hint.hidden = false;
          clearTimeout(timer);
          timer = window.setTimeout(() => (hint.hidden = true), 1500);
        },
        { passive: true },
      );
    }

    this.dots = new DotLayer(site.places, site.index.categories).addTo(this.map) as DotLayer;
    this.focus.addTo(this.map);

    this.map.on("click", (ev: L.LeafletMouseEvent) => {
      const p = this.dots.hit(ev.containerPoint);
      if (p !== undefined) events.pick(p);
    });
    this.map.on("mousemove", (ev: L.LeafletMouseEvent) => {
      const p = this.dots.hit(ev.containerPoint);
      el.classList.toggle("over-dot", p !== undefined);
      events.hover(p, ev.containerPoint);
    });
    this.map.on("mouseout", () => events.hover(undefined, L.point(0, 0)));
    this.map.on("moveend", () => events.moved());
    this.map.on("zoomanim", (e: L.ZoomAnimEvent) => events.zoomed(e.zoom));
    this.map.on("zoomend", () => events.zoomed(this.map.getZoom()));
    const aimed = () => this.dots.nearest(this.map.getSize().divideBy(2), 48);
    el.addEventListener("keydown", (e) => {
      if (e.key !== "Enter" || e.target !== el) return;
      const p = aimed();
      if (p !== undefined) events.pick(p);
    });
    let byPointer = false;
    document.addEventListener("pointerdown", () => (byPointer = true), true);
    document.addEventListener("keydown", () => (byPointer = false), true);
    const tellAim = () => {
      if (document.activeElement === el && !byPointer) events.aimed(aimed(), true);
    };
    el.addEventListener("focus", tellAim);
    el.addEventListener("blur", () => events.aimed(undefined, false));
    this.map.on("moveend", tellAim);
  }

  setBasemap(dark: boolean) {
    const src: TileSource = dark ? this.site.index.basemap.dark : this.site.index.basemap.light;
    this.tiles?.remove();
    this.tiles = L.tileLayer(src.url, { maxZoom: src.max_zoom || 19, attribution: src.attribution }).addTo(this.map);
    const box = this.map.getContainer();
    box.classList.remove("tiles-mute", "tiles-invert");
    if (src.filter) box.classList.add(`tiles-${src.filter}`);
    this.dots.redraw();
  }

  /** Keeps older work on the map while a replay fills the city in */
  /** Keeps older work on the map at any zoom; arrivals also rings the newest year. */
  setFilling(on: boolean, arrivals = false) {
    this.dots.filling = on;
    this.dots.arrivals = arrivals;
    this.dots.redraw();
  }

  get filling() {
    return this.dots.filling;
  }

  show(summary: PlaceSummary, end: number) {
    this.dots.summary = summary;
    this.dots.end = end;
    this.dots.redraw();
  }

  /** clears the circle, marker and outlines */
  clearFocus() {
    this.focus.clearLayers();
    this.dots.highlighted = new Set();
    this.dots.redraw();
  }

  circle(at: LatLng, radiusM: number, move: boolean) {
    this.clearFocus();
    const ink = cssVar("--ink");
    const c = L.circle([at.lat, at.lng], {
      radius: radiusM, pane: "lines", renderer: this.lines, interactive: false,
      color: ink, weight: 1.5, dashArray: "5 4", fillColor: ink, fillOpacity: 0.06,
    }).addTo(this.focus);
    L.circleMarker([at.lat, at.lng], {
      pane: "marks", interactive: false, radius: 5, color: cssVar("--panel"), weight: 2, fillColor: ink, fillOpacity: 1,
    }).addTo(this.focus);
    if (move) this.map.fitBounds(c.getBounds(), { padding: [40, 40], maxZoom: 17 });
  }

  outline(places: number[], move: boolean) {
    this.clearFocus();
    this.dots.highlighted = new Set(places);
    this.dots.redraw();
    if (move && places.length) {
      const pts = places.map((p) => L.latLng(this.site.places.lat[p]! / 1e5, this.site.places.lng[p]! / 1e5));
      this.map.fitBounds(L.latLngBounds(pts), { padding: [40, 40], maxZoom: 17 });
    }
  }

  visible(): L.LatLngBounds {
    return this.map.getBounds();
  }

  async toggleOverlay(name: string, file: string, on: boolean, load: (file: string) => Promise<GeoJSON.GeoJsonObject>) {
    this.overlays.get(name)?.remove();
    this.overlays.delete(name);
    if (!on) return;
    const road = cssVar("--overlay");
    const layer = L.geoJSON(await load(file), {
      pane: "lines",
      onEachFeature: (f, l) => l.bindPopup(() => overlayPopup(f.properties)),
      style: (f) =>
        /Polygon/.test(f?.geometry.type ?? "")
          ? { color: road, weight: 1.5, fillColor: road, fillOpacity: 0.12, renderer: this.lines }
          : { color: road, weight: 3, opacity: 0.85, renderer: this.lines },
    });
    this.overlays.set(name, layer.addTo(this.map));
  }
}
