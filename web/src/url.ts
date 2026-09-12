import type { Index } from "./data";

export interface ViewState {
  place?: number;
  // a search to run again, for views that aren't a single place (a street, a crossing)
  q?: string;
  radiusM?: number;
  from?: number;
  to?: number;
  cats?: number[];
  includeNoWork?: boolean;
}

export const RADII = [
  { m: 76, label: "250 ft" },
  { m: 152, label: "500 ft" },
  { m: 402, label: "a quarter mile" },
  { m: 805, label: "half a mile" },
  { m: 1609, label: "a mile" },
] as const;

// 500 ft
export const DEFAULT_RADIUS = 152;

export function toHash(s: ViewState): string {
  const p = new URLSearchParams();
  if (s.place !== undefined) p.set("place", String(s.place));
  if (s.q) p.set("q", s.q);
  if (s.radiusM !== undefined) p.set("r", String(s.radiusM));
  if (s.from !== undefined) p.set("from", String(s.from));
  if (s.to !== undefined) p.set("to", String(s.to));
  if (s.cats) p.set("cats", s.cats.join(","));
  if (s.includeNoWork !== undefined) p.set("nowork", s.includeNoWork ? "1" : "0");
  const out = p.toString();
  return out ? `#${out}` : "";
}

const wholeNumber = (v: string | null) => (v !== null && /^\d{1,9}$/.test(v) ? Number(v) : undefined);
const year = (v: string | null) => {
  const n = wholeNumber(v);
  return n !== undefined && n >= 1900 && n <= 2200 ? n : undefined;
};

/** reads a link back */
export function fromHash(hash: string, idx: Index): ViewState {
  let p: URLSearchParams;
  try {
    p = new URLSearchParams(hash.replace(/^#/, ""));
  } catch {
    return {};
  }
  const s: ViewState = {};
  const place = wholeNumber(p.get("place"));
  if (place !== undefined) s.place = place;
  const q = p.get("q");
  if (q && q.length <= 120) s.q = q;
  const r = wholeNumber(p.get("r"));
  if (r !== undefined && RADII.some((x) => x.m === r)) s.radiusM = r;
  const from = year(p.get("from"));
  if (from !== undefined) s.from = from;
  const to = year(p.get("to"));
  if (to !== undefined) s.to = to;
  const cats = p.get("cats");
  if (cats !== null) {
    s.cats = cats
      .split(",")
      .map(wholeNumber)
      .filter((c): c is number => c !== undefined && c < idx.categories.length);
  }
  const nowork = p.get("nowork");
  if (nowork === "1" || nowork === "0") s.includeNoWork = nowork === "1";
  return s;
}
