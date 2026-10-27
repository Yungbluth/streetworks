import type { Detail, Site } from "./data";
import { matcher, type Filter, type PlaceSummary } from "./filters";
import type { LatLng } from "./geo";

export type Focus =
  | { kind: "place"; place: number; note?: string; q?: string; /** Where it was opened from, such as its street. */ from?: Focus }
  | { kind: "street"; street: number; q: string }
  | { kind: "spot"; at: LatLng; title: string; note?: string; q: string };

export interface Ctx {
  site: Site;
  filter: Filter;
  radiusM: number;
  summary: PlaceSummary;
  go(focus: Focus | null, opts?: { move?: boolean }): void;
  setRadius(m: number): void;
  showAllKinds(): void;
  // Draws the panel again, for a "Try again" after a failed load
  retry(): void;
}

export const placeAt = (site: Site, p: number): LatLng => ({ lat: site.places.lat[p]! / 1e5, lng: site.places.lng[p]! / 1e5 });
export const permitAt = (site: Site, i: number): LatLng => ({ lat: site.permits.lat[i]! / 1e5, lng: site.permits.lng[i]! / 1e5 });

const starts = new WeakMap<Site, Uint32Array>();

// Indexes of a place's permits that pass the filter.
export function permitsOf(site: Site, place: number, f: Filter): number[] {
  let s = starts.get(site);
  if (!s) {
    s = new Uint32Array(site.places.kind.length + 1);
    for (const p of site.permits.place) s[p + 1]!++;
    for (let i = 1; i < s.length; i++) s[i]! += s[i - 1]!;
    starts.set(site, s);
  }
  const ok = matcher(site.permits, f);
  const out: number[] = [];
  for (let i = s[place]!; i < s[place + 1]!; i++) if (ok(i)) out.push(i);
  return out;
}

// full records for some permits, from the permit server
export function detailsFor(site: Site, permits: number[]): Promise<Map<number, Detail>> {
  return site.api.records(permits);
}

// newest first; permits with no date go last
export const byDateDesc = (a?: Detail, b?: Detail) => (b?.date ?? "").localeCompare(a?.date ?? "");

export const DIG_KINDS = ["Excavation", "Combo"];

const digCache = new WeakMap<Site, { key: string; out: Uint32Array }>();

/** The digging kinds that are switched on: digs follow the same filters as the map */
export function digKinds(site: Site, f: Filter): Set<number> {
  return new Set(DIG_KINDS.map((k) => site.index.categories.indexOf(k)).filter((c) => c >= 0 && f.cats.has(c)));
}

export function digsByPlace(site: Site, f: Filter): Uint32Array {
  const cats = digKinds(site, f);
  const key = `${f.from} ${f.to} ${f.includeNoWork} ${[...cats].sort((a, b) => a - b)}`;
  const hit = digCache.get(site);
  if (hit?.key === key) return hit.out;
  const ok = matcher(site.permits, { ...f, cats });
  const out = new Uint32Array(site.places.kind.length);
  for (let i = 0; i < site.permits.place.length; i++) if (ok(i)) out[site.permits.place[i]!]!++;
  digCache.set(site, { key, out });
  return out;
}
