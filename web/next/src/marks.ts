import type { Age } from "../../src/filters";
import { edgeFor, paint } from "../../src/paint";

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

/** a kind of work's paint, drawn the way its dots are: a solid with its own outline, if any */
export function swatch(category: string, size = 12): SVGSVGElement {
  const edge = edgeFor(category);
  const r = size / 2 - (edge ? 1 : 0.5);
  return svg(size, circle({ cx: size / 2, cy: size / 2, r, fill: paint(category), ...(edge ? { stroke: edge, "stroke-width": 1 } : {}) }));
}

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
