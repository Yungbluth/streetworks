import { PlaceKind, type Places } from "./data";

export function prettyStreet(canon: string): string {
  return canon
    .split(" ")
    .filter(Boolean)
    .map((w) => (/^[NSEW]{1,2}$/.test(w) ? w : w[0] + w.slice(1).toLowerCase()))
    .join(" ");
}

export function placeName(places: Places, p: number): string {
  const street = (i: number) => prettyStreet(places.streets[i] ?? "");
  switch (places.kind[p]) {
    case PlaceKind.Block:
      return `${places.block[p]} block of ${street(places.street[p]!)}`;
    case PlaceKind.Intersection:
      return `${street(places.street[p]!)} and ${street(places.cross[p]!)}`;
    default:
      return places.label[String(p)] ?? "A spot with no address";
  }
}

const nf = new Intl.NumberFormat("en-US");
export const num = (n: number) => nf.format(n);
export const plural = (n: number, one: string, many = `${one}s`) => `${num(n)} ${n === 1 ? one : many}`;

export function feet(m: number): string {
  const ft = m * 3.28084;
  if (ft >= 1320) return `${(ft / 5280).toFixed(1).replace(/\.0$/, "")} mi`;
  return `${Math.round(ft / 10) * 10} ft`;
}

export const yearsText = (from: number, to: number) => (from === to ? String(from) : `${from} to ${to}`);

const df = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric", timeZone: "UTC" });
export const dateText = (iso: string) => (iso ? df.format(new Date(`${iso}T00:00:00Z`)) : "No date");
