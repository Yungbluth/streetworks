import type { Age } from "./filters";

export interface DotStyle {
  r: number;
  fill?: string;
  stroke?: string;
  lineWidth?: number;
  alpha: number;
}

export function dotStyle(age: Age, full: number, color: string, edge?: string): DotStyle {
  switch (age) {
    case "recent":
      return edge ? { r: full, fill: color, stroke: edge, lineWidth: 1, alpha: 1 } : { r: full, fill: color, alpha: 1 };
    case "back": {
      const r = full * 0.9;
      return { r, fill: undefined, stroke: edge ?? color, lineWidth: Math.max(1.4, r * 0.4), alpha: 1 };
    }
    case "old": {
      const r = Math.max(1.1, full * 0.42);
      return edge ? { r: r + 0.4, fill: color, stroke: edge, lineWidth: 0.8, alpha: 0.85 } : { r, fill: color, alpha: 0.7 };
    }
  }
}

/** the zoom at which older work joins the map */
export const DETAIL_ZOOM = 14;

export const visibleAt = (age: Age, zoom: number) => age === "recent" || zoom >= DETAIL_ZOOM;

export const faded = (s: DotStyle): DotStyle => ({ ...s, alpha: s.alpha * 0.25 });
