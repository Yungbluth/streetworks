import type { Permits } from "./data";
import { distanceMeters, type LatLng } from "./geo";

/** what the person has chosen to see */
export interface Filter {
  from: number;
  to: number;
  cats: Set<number>;
  includeNoWork: boolean;
}

function matcher(p: Permits, f: Filter): (i: number) => boolean {
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
