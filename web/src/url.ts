import { PlaceKind, type Index, type Places } from "./data";

export interface ViewState {
  place?: string;
  // a search to run again, for views that aren't a single place (a street, a crossing)
  q?: string;
  /** A permit by its number, such as T23SY041246, opened at its place. */
  permit?: string;
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

export function placeKey(places: Places, p: number): string {
  const street = (i: number) => places.streets[i] ?? "";
  switch (places.kind[p]) {
    case PlaceKind.Block:
      return `b:${street(places.street[p]!)}:${places.block[p]}`;
    case PlaceKind.Intersection:
      return `x:${street(places.street[p]!)}:${street(places.cross[p]!)}`;
    default:
      return `p:${places.label[String(p)] ?? ""}:${Math.round(places.lat[p]! / 10)}:${Math.round(places.lng[p]! / 10)}`;
  }
}

export function placeLookup(places: Places): (key: string) => number | undefined {
  const byKey = new Map<string, number>();
  for (let p = 0; p < places.kind.length; p++) byKey.set(placeKey(places, p), p);
  return (key) => byKey.get(key);
}

export function toHash(s: ViewState, idx: Index): string {
  const p = new URLSearchParams();
  if (s.place !== undefined) p.set("place", s.place);
  if (s.q) p.set("q", s.q);
  if (s.permit) p.set("permit", s.permit);
  if (s.radiusM !== undefined) p.set("r", String(s.radiusM));
  if (s.from !== undefined) p.set("from", String(s.from));
  if (s.to !== undefined) p.set("to", String(s.to));
  // categories by name: their order in index.json can change between builds
  if (s.cats) p.set("cats", s.cats.map((c) => idx.categories[c]).join(","));
  if (s.includeNoWork !== undefined) p.set("nowork", s.includeNoWork ? "1" : "0");
  const out = p.toString();
  return out ? `#${out}` : "";
}

// Letters, digits and dashes
const permitNumber = /^[A-Za-z0-9-]{1,40}$/;

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
  const place = p.get("place");
  if (place && place.length <= 200) s.place = place;
  const q = p.get("q");
  if (q && q.length <= 120) s.q = q;
  const permit = p.get("permit")?.trim();
  if (permit && permitNumber.test(permit)) s.permit = permit;
  const r = wholeNumber(p.get("r"));
  if (r !== undefined && RADII.some((x) => x.m === r)) s.radiusM = r;
  const from = year(p.get("from"));
  if (from !== undefined) s.from = from;
  const to = year(p.get("to"));
  if (to !== undefined) s.to = to;
  const cats = p.get("cats");
  if (cats === "") s.cats = [];
  else if (cats !== null) {
    const known = cats.split(",").flatMap((name) => {
      const i = idx.categories.indexOf(name);
      return i >= 0 ? [i] : [];
    });
    if (known.length) s.cats = known;
  }
  const nowork = p.get("nowork");
  if (nowork === "1" || nowork === "0") s.includeNoWork = nowork === "1";
  return s;
}
