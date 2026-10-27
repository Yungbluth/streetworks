
import { API_BASE, apiClient, type Api } from "./api";

export const DATA_VERSION = 2;

export interface KV {
  key: string;
  n: number;
}

export interface Quality {
  rows: number;
  unique_permits: number;
  duplicate_rows: number;
  status_differed: number;
  no_work: number;
  no_block: number;
  unplaced: number;
  no_date: number;
  bad_dates: number;
  street_from_text: number;
  date_min: string;
  date_max: string;
  per_year: KV[];
  statuses: KV[];
  categories: KV[];
  layers: { layer: string; rows: number; kept: number; dupes: number }[];
}

export interface TileSource {
  url: string;
  attribution: string;
  max_zoom: number;
  filter: "" | "mute" | "invert";
}

export interface Index {
  version: number;
  built: string;
  sources: { name: string; url: string; fetched: string; rows: number }[];
  categories: string[];
  dateRange: [string, string];
  sparseBefore: number;
  bounds: [number, number, number, number];
  quality: Quality;
  basemap: { light: TileSource; dark: TileSource };
  overlays: { name: string; label: string; file: string; rows: number }[];
}

export function yearSpan(index: Index): [first: number, last: number] {
  return [Number(index.dateRange[0].slice(0, 4)), Number(index.dateRange[1].slice(0, 4))];
}

export const enum PlaceKind {
  Block = 0,
  Intersection = 1,
  Point = 2,
}

export interface Places {
  streets: string[];
  kind: PlaceKind[];
  street: number[];
  cross: number[];
  block: number[];
  lat: number[];
  lng: number[];
  label: Record<string, string>;
}

export interface Permits {
  place: number[];
  year: number[]; // 0 when the permit has no usable date
  cat: number[];
  noWork: (0 | 1)[];
  lat: number[];
  lng: number[];
}

export interface Detail {
  permit: string;
  type: string;
  subType?: string;
  status: string;
  noWork: boolean;
  date: string;
  dateBasis: string;
  final?: string;
  description: string;
  applicant?: string;
  utility?: string;
  filed: string;
  link?: string;
  located?: string;
  flags?: string[];
}

export interface Site {
  index: Index;
  places: Places;
  permits: Permits;
  /** The permit server, for full records and search */
  api: Api;
}

export class DataError extends Error {
  override name = "DataError";
}

export const errorMessage = (e: unknown) => (e instanceof Error ? e.message : String(e));

async function getJSON<T>(base: string, file: string, fetchFn: typeof fetch): Promise<T> {
  let res: Response;
  try {
    res = await fetchFn(new URL(file, base));
  } catch (e) {
    throw new DataError(`Couldn't load ${file}: ${errorMessage(e)}`);
  }
  if (!res.ok) {
    throw new DataError(`Couldn't load ${file}: the server said ${res.status} ${res.statusText}`.trimEnd());
  }
  try {
    return (await res.json()) as T;
  } catch {
    throw new DataError(`${file} isn't valid JSON. Rebuild the site data with streetworks build.`);
  }
}

export async function loadSite(base: string, fetchFn: typeof fetch = fetch, apiBase = API_BASE): Promise<Site> {
  const index = await getJSON<Index>(base, "index.json", fetchFn);
  if (index.version !== DATA_VERSION) {
    throw new DataError(
      `The site's data is data format ${index.version}, this page expects ${DATA_VERSION}. Rebuild the data and the page together.`,
    );
  }
  const [places, permits] = await Promise.all([
    getJSON<Places>(base, "places.json", fetchFn),
    getJSON<Permits>(base, "permits.json", fetchFn),
  ]);
  return { index, places, permits, api: apiClient(apiBase, index.built, fetchFn) };
}
