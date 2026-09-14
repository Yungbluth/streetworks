// Search works entirely from the permit data, so it needs no geocoder and no key.

import { PlaceKind, type Places } from "./data";
import { distanceMeters, type LatLng } from "./geo";
import { canonicalStreet, splitStreet, withoutDirection } from "./street";

export type Bounds = [number, number, number, number];

export type Found =
  | { kind: "address"; place: number; wanted: number; exact: boolean }
  | {
      kind: "street";
      street: number;
      bounds: Bounds;
      permits: number;
      /** set when this came from two cross streets that never meet */
      apart?: { sides: [number, number]; gapM: number };
    }
  | {
      kind: "intersection";
      place?: number;
      at: LatLng;
      streets: [number, number];
      exact: boolean;
      // How far apart the two streets' nearest blocks are
      gapM: number;
    };

interface Block {
  num: number;
  place: number;
  lat: number;
  lng: number;
}

interface Street {
  id: number;
  name: string;
  dir: string;
  core: string;
  suffix: string;
  blocks: Block[]; // in block order
  permits: number;
  bounds: Bounds;
}

interface Query {
  dir: string;
  core: string;
  suffix: string;
}

// Hundred-block points sit mid-block
const MAX_CROSSING_GAP_M = 350;
// TODO is 120 too many?
const MAX_QUERY = 120;

const leadingNumber = /^\s*(\d{1,6})(?:\s*[-–]\s*\d{1,6})?\s+(.*)$/;
// "Speedway 1200", "E Speedway Blvd, 1234": the number typed after the street
const trailingNumber = /^(.*\D)[\s,]+(\d{1,6})\s*$/;
const crossSplit = /\s*(?:&|@|\/|\band\b|\bat\b)\s*/i;
const controlChars = /[\u0000-\u001f\u007f]/;

// "e speedway blvd" -> pieces we can compare, or nothing if there's no street name in it
function parseQuery(s: string): Query | undefined {
  const parts = splitStreet(canonicalStreet("", s));
  return parts.core ? parts : undefined;
}

export function buildSearch(places: Places, perPlace: Uint32Array) {
  const streets = new Map<number, Street>();
  const crossings = new Map<string, number>();
  for (let p = 0; p < places.kind.length; p++) {
    const lat = places.lat[p]! / 1e5;
    const lng = places.lng[p]! / 1e5;
    if (places.kind[p] === PlaceKind.Intersection) {
      crossings.set(crossingKey(places.streets[places.street[p]!]!, places.streets[places.cross[p]!]!), p);
      continue;
    }
    if (places.kind[p] !== PlaceKind.Block) continue;
    const id = places.street[p]!;
    let s = streets.get(id);
    if (!s) {
      const name = places.streets[id]!;
      s = { id, name, ...splitStreet(name), blocks: [], permits: 0, bounds: [90, 180, -90, -180] };
      streets.set(id, s);
    }
    s.blocks.push({ num: places.block[p]!, place: p, lat, lng });
    s.permits += perPlace[p] ?? 0;
    s.bounds = [Math.min(s.bounds[0], lat), Math.min(s.bounds[1], lng), Math.max(s.bounds[2], lat), Math.max(s.bounds[3], lng)];
  }

  const byCore = new Map<string, Street[]>();
  for (const s of streets.values()) {
    s.blocks.sort((a, b) => a.num - b.num);
    const list = byCore.get(s.core) ?? [];
    list.push(s);
    byCore.set(s.core, list);
  }
  for (const list of byCore.values()) list.sort((a, b) => b.permits - a.permits || a.name.localeCompare(b.name));
  const cores = [...byCore.keys()].sort();

  const busiestFirst = (l: Street[]) => l.sort((a, b) => b.permits - a.permits);

  // the streets a query could mean
  function matchStreets(q: Query, limit: number): Street[] {
    const exact = byCore.get(q.core) ?? [];
    const prefix: Street[] = [];
    const contains: Street[] = [];
    if (q.core.length >= 2) {
      let i = lowerBound(cores, q.core);
      for (; i < cores.length && cores[i]!.startsWith(q.core); i++) {
        if (cores[i] !== q.core) prefix.push(...byCore.get(cores[i]!)!);
      }
      if (exact.length + prefix.length === 0 && q.core.length >= 3) {
        for (const c of cores) if (c.includes(q.core)) contains.push(...byCore.get(c)!);
      }
      if (exact.length + prefix.length + contains.length === 0 && q.core.length >= 4) {
        const allowed = q.core.length >= 8 ? 2 : 1;
        for (const c of cores) if (typoDistance(q.core, c, allowed) <= allowed) contains.push(...byCore.get(c)!);
      }
    }
    let all = [...exact, ...busiestFirst(prefix), ...busiestFirst(contains)];
    const narrow = (keep: (s: Street) => boolean) => {
      const kept = all.filter(keep);
      if (kept.length > 0) all = kept;
    };
    if (q.dir) narrow((s) => s.dir === q.dir);
    if (q.suffix) narrow((s) => s.suffix === q.suffix);
    return all.slice(0, limit);
  }

  function searchIntersection(a: string, b: string, limit: number): Found[] {
    const qa = parseQuery(a);
    const qb = parseQuery(b);
    if (!qa || !qb) return [];

    type Pair = { found: Extract<Found, { kind: "intersection" }>; d: number };
    const pairs: Pair[] = [];
    const direct = crossings.get(crossingKey(canonicalStreet("", a), canonicalStreet("", b)));
    if (direct !== undefined) pairs.push({ found: atPlace(direct), d: 0 });

    const sidesA = matchStreets(qa, 4);
    const sidesB = matchStreets(qb, 4);
    for (const sa of sidesA) {
      for (const sb of sidesB) {
        if (sa === sb) continue;
        const place = crossings.get(crossingKey(sa.name, sb.name));
        if (place !== undefined) {
          pairs.push({ found: { ...atPlace(place), streets: [sa.id, sb.id] }, d: 0 });
          continue;
        }
        const near = nearestBlocks(sa, sb);
        if (!near) continue;
        pairs.push({
          d: near.d,
          found: {
            kind: "intersection",
            at: { lat: (near.x.lat + near.y.lat) / 2, lng: (near.x.lng + near.y.lng) / 2 },
            streets: [sa.id, sb.id],
            exact: near.d <= MAX_CROSSING_GAP_M,
            gapM: Math.round(near.d),
          },
        });
      }
    }
    pairs.sort((x, y) => x.d - y.d);

    const best = pairs[0];
    if (best && best.d > MAX_CROSSING_GAP_M) {
      const busiest = (l: Street[]) => [...l].sort((x, y) => y.permits - x.permits)[0]!;
      const apart = { sides: [busiest(sidesA).id, busiest(sidesB).id] as [number, number], gapM: Math.round(best.d) };
      const all = [...new Set([...sidesA, ...sidesB])].sort((x, y) => y.permits - x.permits);
      return all.slice(0, limit).map((st) => streetFound(st, apart));
    }

    // When the streets really do cross somewhere, pairings that don't
    let kept = pairs.filter((p) => p.d <= MAX_CROSSING_GAP_M);
    const seen = new Set<number>();
    kept = kept.filter((p) => {
      if (p.found.place === undefined) return true;
      if (seen.has(p.found.place)) return false;
      seen.add(p.found.place);
      return true;
    });
    return kept.slice(0, limit).map((p) => p.found);
  }

  function streetFound(s: Street, apart?: { sides: [number, number]; gapM: number }): Found {
    return { kind: "street", street: s.id, bounds: s.bounds, permits: s.permits, ...(apart === undefined ? {} : { apart }) };
  }

  function atPlace(p: number): Extract<Found, { kind: "intersection" }> {
    return {
      kind: "intersection",
      place: p,
      at: { lat: places.lat[p]! / 1e5, lng: places.lng[p]! / 1e5 },
      streets: [places.street[p]!, places.cross[p]!],
      exact: true,
      gapM: 0,
    };
  }

  return function search(query: string, limit = 8): Found[] {
    limit = Math.min(Math.max(1, Math.floor(limit) || 8), 20);
    const q = query.trim();
    if (!q || q.length > MAX_QUERY || controlChars.test(q)) return [];

    const sides = q.split(crossSplit);
    if (sides.length === 2 && sides[0]!.trim() && sides[1]!.trim()) {
      return searchIntersection(sides[0]!, sides[1]!, limit);
    }

    let want = -1;
    let rest = q;
    const m = leadingNumber.exec(q);
    const t = m ? null : trailingNumber.exec(q);
    if (m && Number(m[1]) > 0) {
      want = Math.floor(Number(m[1]) / 100) * 100;
      rest = m[2]!;
    } else if (t && Number(t[2]) > 0 && !byCore.has(parseQuery(q)?.core ?? "")) {
      // Unless the number is part of a street's name, like Highway 77
      want = Math.floor(Number(t[2]) / 100) * 100;
      rest = t[1]!;
    }
    const sq = parseQuery(rest);
    if (!sq) return [];
    const matches = matchStreets(sq, limit);
    if (want < 0) return matches.map((s) => streetFound(s));

    const found = matches.map((s) => {
      const b = nearestBlock(s.blocks, want);
      return { f: { kind: "address", place: b.place, wanted: want, exact: b.num === want } as Found, off: Math.abs(b.num - want) };
    });
    // exact blocks first, then the closest stand-ins; otherwise keep street order
    return found.sort((x, y) => x.off - y.off).map((x) => x.f);
  };
}

function crossingKey(a: string, b: string): string {
  const [x, y] = [withoutDirection(a), withoutDirection(b)].sort();
  return `${x}|${y}`;
}

export function typoDistance(a: string, b: string, max: number): number {
  if (Math.abs(a.length - b.length) > max) return max + 1;
  let prev2: number[] = [];
  let prev = Array.from({ length: b.length + 1 }, (_, j) => j);
  for (let i = 1; i <= a.length; i++) {
    const cur = [i];
    let rowMin = i;
    for (let j = 1; j <= b.length; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      let d = Math.min(prev[j]! + 1, cur[j - 1]! + 1, prev[j - 1]! + cost);
      if (i > 1 && j > 1 && a[i - 1] === b[j - 2] && a[i - 2] === b[j - 1]) d = Math.min(d, prev2[j - 2]! + 1);
      cur.push(d);
      rowMin = Math.min(rowMin, d);
    }
    if (rowMin > max) return max + 1;
    prev2 = prev;
    prev = cur;
  }
  return prev[b.length]!;
}

function lowerBound(sorted: string[], s: string): number {
  let lo = 0;
  let hi = sorted.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (sorted[mid]! < s) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}

function nearestBlock(blocks: Block[], want: number): Block {
  let i = 0;
  while (i < blocks.length && blocks[i]!.num < want) i++;
  if (i === blocks.length) return blocks[i - 1]!;
  if (i === 0 || blocks[i]!.num === want) return blocks[i]!;
  return want - blocks[i - 1]!.num <= blocks[i]!.num - want ? blocks[i - 1]! : blocks[i]!;
}

function nearestBlocks(a: Street, b: Street): { x: Block; y: Block; d: number } | undefined {
  let best: { x: Block; y: Block; d: number } | undefined;
  for (const x of a.blocks) {
    for (const y of b.blocks) {
      const d = distanceMeters(x, y);
      if (!best || d < best.d) best = { x, y, d };
    }
  }
  return best;
}
