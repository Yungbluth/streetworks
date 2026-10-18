import type { Age } from "./filters";

export interface DotStyle {
  r: number;
  fill?: string;
  stroke?: string;
  lineWidth?: number;
  alpha: number;
  outline?: string;
  outlineWidth?: number;
  /** A soft shadow round the mark instead: a pale dot shows without a line that reads as a ring */
  halo?: string;
}

export function dotStyle(age: Age, full: number, color: string, edge?: string, halo?: string): DotStyle {
  const line = { ...(edge ? { outline: edge, outlineWidth: 1 } : {}), ...(halo ? { halo } : {}) };
  switch (age) {
    case "recent":
      return { r: full, fill: color, alpha: 1, ...line };
    case "back": {
      const r = full * 0.9;
      return { r, fill: undefined, stroke: color, lineWidth: Math.max(1.4, r * 0.4), alpha: 1, ...line };
    }
    case "old": {
      const r = Math.max(1.1, full * 0.42);
      return edge ? { r: r + 0.4, fill: color, alpha: 0.9, ...line } : { r, fill: color, alpha: 0.7 };
    }
  }
}

export const DETAIL_ZOOM = 14;

export function drawnAge(age: Age, zoom: number): Age {
  return zoom >= DETAIL_ZOOM ? age : "recent";
}

/** During a replay, whether a place's newest work is the year just added. */
export const arriving = (newest: number, end: number, replaying: boolean) => replaying && newest === end;

export const faded = (s: DotStyle): DotStyle => ({ ...s, alpha: s.alpha * 0.25 });
