import { describe, expect, it } from "vitest";
import { PlaceKind, type Places } from "./data";
import { distanceMeters } from "./geo";
import { buildSearch, type Found } from "./search";

// A small city laid out like Tucson's grid.
const cLat = 32.22;
const cLng = -110.97;
const step = 0.001;
const speedwayLat = cLat + 10 * step;
const campbellLng = cLng + 18 * step;

class City {
  places: Places = { streets: [], kind: [], street: [], cross: [], block: [], lat: [], lng: [], label: {} };
  perPlace: number[] = [];

  private id(name: string) {
    let i = this.places.streets.indexOf(name);
    if (i < 0) i = this.places.streets.push(name) - 1;
    return i;
  }

  private add(kind: PlaceKind, street: number, cross: number, block: number, lat: number, lng: number, n: number) {
    const p = this.places;
    p.kind.push(kind);
    p.street.push(street);
    p.cross.push(cross);
    p.block.push(block);
    p.lat.push(Math.round(lat * 1e5));
    p.lng.push(Math.round(lng * 1e5));
    this.perPlace.push(n);
    return p.kind.length - 1;
  }

  eastWest(name: string, lat: number, blocks: number[], n = 1) {
    const west = name.startsWith("W ");
    for (const b of blocks) this.add(PlaceKind.Block, this.id(name), -1, b, lat, cLng + ((west ? -b : b) / 100) * step, n);
    return this;
  }

  northSouth(name: string, lng: number, blocks: number[], n = 1) {
    const south = name.startsWith("S ");
    for (const b of blocks) this.add(PlaceKind.Block, this.id(name), -1, b, cLat + ((south ? -b : b) / 100) * step, lng, n);
    return this;
  }

  crossing(a: string, b: string, lat: number, lng: number, n = 1) {
    return this.add(PlaceKind.Intersection, this.id(a), this.id(b), -1, lat, lng, n);
  }

  search() {
    return buildSearch(this.places, Uint32Array.from(this.perPlace));
  }
}

function testCity() {
  return new City()
    .eastWest("E SPEEDWAY BLVD", speedwayLat, [100, 900, 1200, 1300, 1800, 1900, 2500], 3)
    .eastWest("W SPEEDWAY BLVD", speedwayLat, [100, 400])
    .northSouth("N CAMPBELL AVE", campbellLng, [300, 900, 1000, 1100, 1700])
    .northSouth("N 4TH AVE", cLng + 4 * step, [500, 600])
    .eastWest("E 6TH ST", cLat + 6 * step, [300, 400])
    .eastWest("E SPEEDWAY ACCESS RD", cLat + 40 * step, [100]) // far away, shares a prefix
    .northSouth("N SPEED AVE", cLng + 50 * step, [100]) // a shorter name sharing a prefix
    .eastWest("WEST ST", cLat - 20 * step, [300]); // a street named West
}

function labels(city: City, found: Found[]): string {
  const { streets, street, block } = city.places;
  return found
    .map((f) => {
      switch (f.kind) {
        case "street":
          return `street:${streets[f.street]}`;
        case "address":
          return `address:${block[f.place]} ${streets[street[f.place]!]} exact=${f.exact}`;
        case "intersection":
          return `x:${streets[f.streets[0]]} & ${streets[f.streets[1]]} exact=${f.exact}`;
      }
    })
    .join(" | ");
}

describe("addresses", () => {
  const city = testCity();
  const search = city.search();

  it.each([
    ["1234 E Speedway Blvd", "address:1200 E SPEEDWAY BLVD exact=true"],
    ["1234 east speedway boulevard", "address:1200 E SPEEDWAY BLVD exact=true"],
    ["1234 E. SPEEDWAY BL.", "address:1200 E SPEEDWAY BLVD exact=true"],
    ["1234 speedway", "address:1200 E SPEEDWAY BLVD exact=true"], // no direction: the street with that block
    ["1299 E Speedway", "address:1200 E SPEEDWAY BLVD exact=true"],
    ["1300 e speedway", "address:1300 E SPEEDWAY BLVD exact=true"],
    ["1600 E Speedway", "address:1800 E SPEEDWAY BLVD exact=false"], // 1300 is 3 blocks off, 1800 is 2
    ["5000 E Speedway", "address:2500 E SPEEDWAY BLVD exact=false"], // past the end of the data
    ["40 E Speedway", "address:100 E SPEEDWAY BLVD exact=false"],
    ["400 W Speedway", "address:400 W SPEEDWAY BLVD exact=true"],
    ["400 Speedway", "address:400 W SPEEDWAY BLVD exact=true"], // only W has a 400 block
    ["1000 N Campbell Ave", "address:1000 N CAMPBELL AVE exact=true"],
    ["1000 N Campbell Avenue", "address:1000 N CAMPBELL AVE exact=true"],
    ["512 N 4th Ave", "address:500 N 4TH AVE exact=true"],
    ["512 N 4 th Av", "address:500 N 4TH AVE exact=true"],
    ["512 north fourth", ""], // spelled-out numbers aren't understood; no guessing
    ["1234-1240 E Speedway", "address:1200 E SPEEDWAY BLVD exact=true"], // a number range
    ["300 West St", "address:300 WEST ST exact=true"],
  ])("%s", (q, wantFirst) => {
    expect(labels(city, search(q).slice(0, 1))).toBe(wantFirst);
  });

  it("offers both directions, the exact block first", () => {
    expect(labels(city, search("1234 speedway"))).toContain("W SPEEDWAY BLVD");
  });

  it("reports the block that was asked for", () => {
    const f = search("1600 E Speedway", 1)[0];
    expect(f).toMatchObject({ kind: "address", wanted: 1600, exact: false });
    expect(city.places.block[(f as Extract<Found, { kind: "address" }>).place]).toBe(1800);
  });
});

describe("streets", () => {
  const city = testCity();
  const search = city.search();

  it.each([
    ["Speedway Blvd", "street:E SPEEDWAY BLVD | street:W SPEEDWAY BLVD"], // busiest first, suffix narrows
    ["e speedway", "street:E SPEEDWAY BLVD | street:E SPEEDWAY ACCESS RD"], // direction narrows
    ["W Speedway", "street:W SPEEDWAY BLVD"],
    ["N Campbell", "street:N CAMPBELL AVE"],
    ["campbell avenue", "street:N CAMPBELL AVE"],
    ["S Campbell", "street:N CAMPBELL AVE"], // no S Campbell: the direction is ignored rather than finding nothing
    ["4th ave", "street:N 4TH AVE"],
    ["west st", "street:WEST ST"],
    ["camp", "street:N CAMPBELL AVE"], // still typing
    ["pbell", "street:N CAMPBELL AVE"], // contained, when nothing starts with it
  ])("%s", (q, want) => {
    expect(labels(city, search(q))).toBe(want);
  });

  it("puts an exact name before longer ones that start the same", () => {
    const got = labels(city, search("speed"));
    expect(got.startsWith("street:N SPEED AVE")).toBe(true);
    expect(got).toContain("E SPEEDWAY BLVD");
  });

  it("gives a whole street bounds that cover its blocks", () => {
    const f = search("E Speedway Blvd", 1)[0] as Extract<Found, { kind: "street" }>;
    const [south, west, north, east] = f.bounds;
    expect(west).toBeLessThanOrEqual(cLng + step + 1e-9);
    expect(east).toBeGreaterThanOrEqual(cLng + 25 * step - 1e-9);
    expect(south).toBeCloseTo(speedwayLat, 5);
    expect(north).toBeCloseTo(speedwayLat, 5);
  });
});

describe("cross streets", () => {
  it.each(["Speedway & Campbell", "speedway and campbell", "Campbell / Speedway", "E Speedway Blvd at N Campbell Ave", "speedway@campbell"])(
    "%s",
    (q) => {
      const city = testCity();
      const got = city.search()(q);
      const f = got[0];
      expect(f?.kind).toBe("intersection");
      if (f?.kind !== "intersection") return;
      const names = f.streets.map((s) => city.places.streets[s]).join("+");
      expect(names).toContain("E SPEEDWAY BLVD");
      expect(names).toContain("N CAMPBELL AVE");
      expect(f.exact).toBe(true);
      expect(f.place).toBeUndefined();
      // Between the nearest blocks: close to the real crossing
      expect(distanceMeters(f.at, { lat: speedwayLat, lng: campbellLng })).toBeLessThan(150);
    },
  );

  it("offers each street on its own", () => {
    const city = testCity();
    const got = city.search()("4th Ave & West St");
    expect(labels(city, got)).toBe("street:N 4TH AVE | street:WEST ST");
    for (const f of got as Extract<Found, { kind: "street" }>[]) {
      expect(f.apart!.gapM).toBeGreaterThan(500);
      expect(f.apart!.sides.map((i) => city.places.streets[i])).toEqual(["N 4TH AVE", "WEST ST"]);
    }
  });

  it("puts the busier streets first", () => {
    const grantLat = speedwayLat + 30 * step;
    const city = new City()
      .eastWest("E SPEEDWAY BLVD", speedwayLat, [100, 200, 300], 9)
      .eastWest("W SPEEDWAY BLVD", speedwayLat, [100])
      .eastWest("E GRANT RD", grantLat, [100, 200, 300], 8)
      .eastWest("W GRANT RD", grantLat - 2 * step, [100]);
    const got = labels(city, city.search()("speedway & grant")).split(" | ");
    expect(got.slice(0, 2)).toEqual(["street:E SPEEDWAY BLVD", "street:E GRANT RD"]);
    expect(got).toContain("street:W SPEEDWAY BLVD");
  });

  it("finds nothing for a half-typed or unknown pair", () => {
    const search = testCity().search();
    for (const q of ["nowhere & campbell", " & "]) {
      expect(search(q).filter((f) => f.kind === "intersection")).toEqual([]);
    }
  });

  it("drops pairings that don't cross", () => {
    const city = new City()
      .eastWest("E SPEEDWAY BLVD", speedwayLat, [1700, 1800, 1900])
      .northSouth("N CAMPBELL AVE", campbellLng, [900, 1000, 1100])
      .northSouth("S CAMPBELL AVE", campbellLng, [500, 600]); // south of center: never meets Speedway
    const got = city.search()("speedway & campbell");
    expect(labels(city, got)).toBe("x:E SPEEDWAY BLVD & N CAMPBELL AVE exact=true");
  });

  it("uses the intersection place", () => {
    const city = testCity();
    const at = city.crossing("CAMPBELL AVE", "SPEEDWAY BLVD", speedwayLat + 0.0001, campbellLng - 0.0001, 4);
    const f = city.search()("speedway & campbell")[0];
    expect(f).toMatchObject({ kind: "intersection", place: at, exact: true, gapM: 0 });
    if (f?.kind === "intersection") expect(f.at).toEqual({ lat: city.places.lat[at]! / 1e5, lng: city.places.lng[at]! / 1e5 });
  });

  it("finds an intersection place whose streets have no blocks of their own", () => {
    const city = testCity();
    const at = city.crossing("HEMISPHERE LOOP", "HEMISPHERE PL", 32.13, -110.85);
    for (const q of ["hemisphere pl & hemisphere lp", "Hemisphere Loop and Hemisphere Place"]) {
      expect(city.search()(q)[0]).toMatchObject({ kind: "intersection", place: at, exact: true });
    }
  });
});

describe("forgiving search", () => {
  const city = testCity();
  const search = city.search();

  it.each([
    ["speedwy", "E SPEEDWAY BLVD"], // a letter missing
    ["speedwya blvd", "E SPEEDWAY BLVD"], // two letters swapped
    ["cambpell", "N CAMPBELL AVE"],
  ])("%s finds %s despite the typo", (q, want) => {
    expect(labels(city, search(q))).toContain(`street:${want}`);
  });

  it("puts a typed address on its block despite a typo", () => {
    expect(labels(city, search("1234 e speedwya")).split(" | ")[0]).toBe("address:1200 E SPEEDWAY BLVD exact=true");
  });

  it.each(["e speedway blvd 1234", "Speedway 1200", "E Speedway, 1250"])("takes the number after the street: %s", (q) => {
    expect(labels(city, search(q)).split(" | ")[0]).toBe("address:1200 E SPEEDWAY BLVD exact=true");
  });

  it("says how many permits each street has", () => {
    const f = search("E Speedway Blvd", 1)[0] as Extract<Found, { kind: "street" }>;
    expect(f.permits).toBe(21);
  });
});

describe("the real sample", () => {
  it("finds 749 E Speedway and the two crossings in it", async () => {
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const read = (f: string) => JSON.parse(readFileSync(join(__dirname, "fixtures/data", f), "utf8"));
    const places = read("places.json") as Places;
    const search = buildSearch(places, new Uint32Array(places.kind.length).fill(1));

    const f = search("749 E Speedway")[0];
    expect(f).toMatchObject({ kind: "address", wanted: 700, exact: true });
    expect(search("golf links rd & cathy dr")[0]).toMatchObject({ kind: "intersection", exact: true, gapM: 0 });
    expect(search("Hemisphere Loop & Hemisphere Pl")[0]).toMatchObject({ kind: "intersection", exact: true });
  });
});
