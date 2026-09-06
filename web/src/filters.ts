import type { Permits } from "./data";
import { distanceMeters, type LatLng } from "./geo";

/** what the person has chosen to see */
export interface Filter {
  from: number;
  to: number;
  cats: Set<number>;
  includeNoWork: boolean;
}

/** a test for one permit index against the filter */
export function matcher(p: Permits, f: Filter): (i: number) => boolean {
  const ranged = f.from !== 0 || f.to !== 0;
  const from = f.from || -Infinity;
  const to = f.to || Infinity;
  const catOK = new Uint8Array(Math.max(0, ...f.cats) + 1);
  for (const c of f.cats) catOK[c] = 1;
  return (i) => {
    if (!f.includeNoWork && p.noWork[i]) return false;
    if (!catOK[p.cat[i]!]) return false;
    const y = p.year[i]!;
    // todo: undated permits just drop out once a range is set
    if (y === 0) return !ranged;
    return y >= from && y <= to;
  };
}

export function countByPlace(p: Permits, f: Filter, placeCount = (p.place.at(-1) ?? -1) + 1): Uint32Array {
  const counts = new Uint32Array(placeCount);
  const ok = matcher(p, f);
  for (let i = 0; i < p.place.length; i++) {
    if (ok(i)) counts[p.place[i]!]!++;
  }
  return counts;
}

/** Indexes of the permits that pass the filter, optionally within a circle */
export function permitsMatching(p: Permits, f: Filter, within?: { center: LatLng; radiusM: number }): number[] {
  const ok = matcher(p, f);
  const out: number[] = [];
  for (let i = 0; i < p.place.length; i++) {
    if (!ok(i)) continue;
    if (within && distanceMeters(within.center, { lat: p.lat[i]! / 1e5, lng: p.lng[i]! / 1e5 }) > within.radiusM) continue;
    out.push(i);
  }
  return out;
}

export interface PlaceSummary {
  count: Uint32Array;
  newest: Uint16Array;
  /** the category with the most matching permits */
  cat: Uint8Array;
}

/** what the map needs for each place in one pass */
export function summarizePlaces(p: Permits, f: Filter, placeCount = (p.place.at(-1) ?? -1) + 1): PlaceSummary {
  const out: PlaceSummary = { count: new Uint32Array(placeCount), newest: new Uint16Array(placeCount), cat: new Uint8Array(placeCount) };
  const ok = matcher(p, f);
  const tally = new Uint32Array(256);
  let current = -1;
  let touched: number[] = [];
  const finish = () => {
    if (current < 0) return;
    let best = -1;
    for (const c of touched) if (best < 0 || tally[c]! > tally[best]!) best = c;
    if (best >= 0) out.cat[current] = best;
    for (const c of touched) tally[c] = 0;
    touched = [];
  };
  for (let i = 0; i < p.place.length; i++) {
    const place = p.place[i]!;
    if (place !== current) {
      finish();
      current = place;
    }
    if (!ok(i)) continue;
    out.count[place]!++;
    if (p.year[i]! > out.newest[place]!) out.newest[place] = p.year[i]!;
    const c = p.cat[i]!;
    if (tally[c] === 0) touched.push(c);
    tally[c]!++;
  }
  finish();
  return out;
}

/** Permits per year under the category and no-work filters, ignoring the year range */
export function countByYear(p: Permits, f: Filter): Map<number, number> {
  const ok = matcher(p, { ...f, from: 0, to: 0 });
  const out = new Map<number, number>();
  for (let i = 0; i < p.year.length; i++) {
    const y = p.year[i]!;
    if (y !== 0 && ok(i)) out.set(y, (out.get(y) ?? 0) + 1);
  }
  return out;
}

export type Age = "recent" | "back" | "old";

export function ageRanges(end: number) {
  return { recent: [end - 2, end], back: [end - 10, end - 3], oldBefore: end - 10 } as const;
}

export function ageOf(newest: number, end: number): Age {
  const r = ageRanges(end);
  if (newest >= r.recent[0]) return "recent";
  if (newest >= r.back[0]) return "back";
  return "old";
}
