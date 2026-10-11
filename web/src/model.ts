import type { Detail, Site } from "./data";
import { matcher, type Filter, type PlaceSummary } from "./filters";
import { cellKey, type LatLng } from "./geo";

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

/** Full records for some permits, fetching each detail square once */
export async function detailsFor(site: Site, permits: number[]): Promise<Map<number, Detail>> {
  const byCell = new Map<string, number[]>();
  for (const i of permits) {
    const at = permitAt(site, i);
    const key = cellKey(at.lat, at.lng);
    byCell.set(key, [...(byCell.get(key) ?? []), i]);
  }
  const out = new Map<number, Detail>();
  await Promise.all(
    [...byCell].map(async ([key, list]) => {
      const cell = await site.detail(key);
      for (const i of list) {
        const d = cell[String(i)];
        if (d) out.set(i, d);
      }
    }),
  );
  return out;
}

// newest first; permits with no date go last
export const byDateDesc = (a?: Detail, b?: Detail) => (b?.date ?? "").localeCompare(a?.date ?? "");
