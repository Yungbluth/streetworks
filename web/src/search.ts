// Search works entirely from the permit data, so it needs no geocoder and no key.

import { type LatLng } from "./geo";
import { canonicalStreet, splitStreet } from "./street";

export type Bounds = [number, number, number, number];

export type Found =
  | { kind: "address"; place: number; wanted: number; exact: boolean }
  | { kind: "street"; street: number; bounds: Bounds }
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
const crossSplit = /\s*(?:&|@|\/|\band\b|\bat\b)\s*/i;
const controlChars = /[\u0000-\u001f\u007f]/;

// "e speedway blvd" -> peices we can compare, or nothing if there's no street name in it
function parseQuery(s: string): Query | undefined {
  const parts = splitStreet(canonicalStreet("", s));
  return parts.core ? parts : undefined;
}
