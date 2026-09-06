export interface LatLng {
  lat: number;
  lng: number;
}

const EARTH_RADIUS_M = 6_371_000;
const toRad = (deg: number) => (deg * Math.PI) / 180;

/** Great-circle distance in metres */
export function distanceMeters(a: LatLng, b: LatLng): number {
  const dLat = toRad(b.lat - a.lat);
  const dLng = toRad(b.lng - a.lng);
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(toRad(a.lat)) * Math.cos(toRad(b.lat)) * Math.sin(dLng / 2) ** 2;
  return 2 * EARTH_RADIUS_M * Math.asin(Math.min(1, Math.sqrt(h)));
}

const fixed = (deg: number) => Math.round(deg * 1e5);
const square = (deg: number, step: number) => Math.floor(fixed(deg) / Math.round(step * 1e5));

export function cellKey(lat: number, lng: number, step = 0.01): string {
  return `${square(lat, step)}_${square(lng, step)}`;
}

export function cellsInCircle(c: LatLng, radiusM: number, step = 0.01): string[] {
  const dLat = (radiusM / (Math.PI * EARTH_RADIUS_M)) * 180;
  const dLng = dLat / Math.cos(toRad(c.lat));
  const keys: string[] = [];
  for (let y = square(c.lat - dLat, step); y <= square(c.lat + dLat, step); y++) {
    for (let x = square(c.lng - dLng, step); x <= square(c.lng + dLng, step); x++) {
      keys.push(`${y}_${x}`);
    }
  }
  return keys;
}
