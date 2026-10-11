import type { Age } from "./filters";
import { cssVar, edgeFor, paint } from "./paint";

const NS = "http://www.w3.org/2000/svg";

function svg(size: number, ...children: SVGElement[]): SVGSVGElement {
  const s = document.createElementNS(NS, "svg");
  s.setAttribute("width", String(size));
  s.setAttribute("height", String(size));
  s.setAttribute("viewBox", `0 0 ${size} ${size}`);
  s.setAttribute("aria-hidden", "true");
  s.append(...children);
  return s;
}

function circle(attrs: Record<string, string | number>): SVGCircleElement {
  const c = document.createElementNS(NS, "circle");
  for (const [k, v] of Object.entries(attrs)) c.setAttribute(k, String(v));
  return c;
}

/** The three age marks, drawn in ink, as the key and chart use them */
export function ageMark(age: Age): SVGSVGElement {
  switch (age) {
    case "recent":
      return svg(14, circle({ cx: 7, cy: 7, r: 5.5, fill: "currentColor" }));
    case "back":
      return svg(14, circle({ cx: 7, cy: 7, r: 4.6, fill: "none", stroke: "currentColor", "stroke-width": 2 }));
    case "old":
      return svg(14, circle({ cx: 7, cy: 7, r: 2, fill: "currentColor", opacity: 0.75 }));
  }
}

export function swatch(category: string, size = 12): SVGSVGElement {
  // a page may give every swatch a hairline (--swatch-edge)
  const edge = edgeFor(category) || cssVar("--swatch-edge") || undefined;
  const r = size / 2 - (edge ? 1 : 0.5);
  return svg(size, circle({ cx: size / 2, cy: size / 2, r, fill: paint(category), ...(edge ? { stroke: edge, "stroke-width": 1 } : {}) }));
}

/** A revision cloud: the scalloped loop a drafter draws around a change, here around the open place */
export function cloudPath(cx: number, cy: number, radius: number, scallops = 11): string {
  const step = (Math.PI * 2) / scallops;
  const point = (a: number) => [cx + radius * Math.cos(a), cy + radius * Math.sin(a)] as const;
  const chord = 2 * radius * Math.sin(step / 2);
  const arc = (chord / 2) * 1.12;
  let d = "";
  for (let i = 0; i < scallops; i++) {
    const [x0, y0] = point(i * step - Math.PI / 2);
    const [x1, y1] = point((i + 1) * step - Math.PI / 2);
    if (i === 0) d += `M${x0.toFixed(2)} ${y0.toFixed(2)}`;
    d += ` A${arc.toFixed(2)} ${arc.toFixed(2)} 0 0 1 ${x1.toFixed(2)} ${y1.toFixed(2)}`;
  }
  return `${d} Z`;
}

export const station = (block: number) => `${Math.floor(block / 100)}+00`;

export function stationTicks(n: number): boolean[] {
  const step = Math.max(1, Math.ceil(n / 6));
  return Array.from({ length: n }, (_, i) => i === n - 1 || (i % step === 0 && n - 1 - i >= step));
}

export function northArrow(): SVGSVGElement {
  const s = svg(28);
  s.setAttribute("viewBox", "0 0 28 28");
  const path = (d: string, fill: string) => {
    const p = document.createElementNS(NS, "path");
    p.setAttribute("d", d);
    p.setAttribute("fill", fill);
    p.setAttribute("stroke", "currentColor");
    p.setAttribute("stroke-width", "1.2");
    p.setAttribute("stroke-linejoin", "round");
    return p;
  };
  s.append(path("M14 3 L20 23 L14 19 Z", "currentColor"), path("M14 3 L8 23 L14 19 Z", "none"));
  return s;
}

/** Plus, minus and clear, drawn as strokes at the sheet's line weight */
export function strokeIcon(kind: "plus" | "minus" | "clear"): SVGSVGElement {
  const s = svg(14);
  const p = document.createElementNS(NS, "path");
  p.setAttribute("d", { plus: "M7 2v10M2 7h10", minus: "M2 7h10", clear: "M3 3l8 8M11 3l-8 8" }[kind]);
  p.setAttribute("stroke", "currentColor");
  p.setAttribute("stroke-width", "1.6");
  p.setAttribute("fill", "none");
  s.append(p);
  return s;
}

/** Play and stop, filled, for the Replay button */
export function controlGlyph(kind: "play" | "stop"): SVGSVGElement {
  const s = svg(12);
  const p = document.createElementNS(NS, "path");
  p.setAttribute("d", kind === "play" ? "M2.5 1.5 L10.5 6 L2.5 10.5 Z" : "M2.5 2.5 H9.5 V9.5 H2.5 Z");
  p.setAttribute("fill", "currentColor");
  s.append(p);
  return s;
}
