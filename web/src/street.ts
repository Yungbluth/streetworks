// Street names, normalized exactly the way the Go build does it (internal/permits/normalize.go)

const suffixes: Record<string, string> = {
  BOULEVARD: "BLVD", BLVD: "BLVD", BL: "BLVD", AVENUE: "AVE", AV: "AVE", AVE: "AVE",
  ROAD: "RD", RD: "RD", STREET: "ST", ST: "ST", DRIVE: "DR", DR: "DR",
  LANE: "LN", LN: "LN", PARKWAY: "PKWY", PKWY: "PKWY", HIGHWAY: "HWY", HWY: "HWY",
  COURT: "CT", CT: "CT", PLACE: "PL", PL: "PL", CIRCLE: "CIR", CIR: "CIR", CI: "CIR",
  TRAIL: "TRL", TRL: "TRL", LP: "LOOP", LOOP: "LOOP", WY: "WAY", WAY: "WAY", TERRACE: "TER", TER: "TER",
};

const directions: Record<string, string> = {
  N: "N", NORTH: "N", S: "S", SOUTH: "S", E: "E", EAST: "E", W: "W", WEST: "W",
  NE: "NE", NW: "NW", SE: "SE", SW: "SW",
};

const shortDirections = new Set(Object.values(directions));
const shortSuffixes = new Set(Object.values(suffixes));
const ordinalTail = new Set(["ST", "ND", "RD", "TH"]);

const tokens = (s: string) =>
  s
    .replace(/[^\p{L}\p{Nd}]/gu, " ")
    .toUpperCase()
    .split(/\s+/)
    .filter(Boolean);

export function canonicalStreet(dir: string, name: string): string {
  let toks = tokens(name);
  if (toks.length === 0) return "";
  let d = directions[dir.replace(/^[.\s]+|[.\s]+$/g, "").toUpperCase()] ?? "";
  if (toks.length >= 2) {
    const td = directions[toks[0]!];
    if (td && !(toks[1]! in suffixes)) {
      if (!d) d = td;
      if (td === d) toks = toks.slice(1);
    }
  }
  // "22 ND ST" -> "22ND ST", but never swallow the final suffix
  const merged: string[] = [];
  for (let i = 0; i < toks.length; i++) {
    if (/^\d+$/.test(toks[i]!) && i + 2 < toks.length && ordinalTail.has(toks[i + 1]!)) {
      merged.push(toks[i]! + toks[i + 1]!);
      i++;
    } else {
      merged.push(toks[i]!);
    }
  }
  const last = merged.length - 1;
  if (last > 0 && merged[last]! in suffixes) merged[last] = suffixes[merged[last]!]!;
  const out = merged.join(" ");
  return d ? `${d} ${out}` : out;
}

export interface StreetParts {
  dir: string;
  core: string;
  suffix: string;
}

export function splitStreet(canon: string): StreetParts {
  let t = canon.split(" ").filter(Boolean);
  let dir = "";
  let suffix = "";
  if (t.length > 1 && shortDirections.has(t[0]!)) {
    dir = t[0]!;
    t = t.slice(1);
  }
  if (t.length > 1 && shortSuffixes.has(t.at(-1)!)) {
    suffix = t.at(-1)!;
    t = t.slice(0, -1);
  }
  return { dir, core: t.join(" "), suffix };
}

export function withoutDirection(canon: string): string {
  const [first, ...rest] = canon.split(" ");
  return rest.length > 0 && shortDirections.has(first!) ? rest.join(" ") : canon;
}
