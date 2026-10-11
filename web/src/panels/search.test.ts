// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { PlaceKind, type Places } from "../data";
import type { Found } from "../search";
import { apartMessage, describe as describeFound, searchBox } from "./search";

const places: Places = {
  streets: ["E SPEEDWAY BLVD", "E GRANT RD"],
  kind: [PlaceKind.Block],
  street: [0],
  cross: [-1],
  block: [1200],
  lat: [3223000],
  lng: [-11095000],
  label: {},
};
const bounds: [number, number, number, number] = [32.2, -111, 32.3, -110.9];

describe("search results", () => {
  it("say how many permits a whole street has", () => {
    const r = describeFound(places, { kind: "street", street: 0, bounds, permits: 1234 }, "speedway");
    expect(r.detail).toBe("Whole street, 1,234 permits");
  });

  it("say once, above the list", () => {
    const apart = { sides: [0, 1] as [number, number], gapM: 1700 };
    const r = describeFound(places, { kind: "street", street: 0, bounds, permits: 1, apart }, "speedway & grant");
    expect(r.detail).toBe("Whole street, 1 permit");
    expect(apartMessage(places, apart)).toBe("Speedway Blvd and Grant Rd don't meet. At their closest they're 1.1 mi apart.");
  });
});

it("counts a whole street under the current filters", () => {
  const r = describeFound(places, { kind: "street", street: 0, bounds, permits: 1234 }, "speedway", (street) => (street === 0 ? 99 : 0));
  expect(r.detail).toBe("Whole street, 99 permits");
});

describe("the search box while typing", () => {
  afterEach(() => vi.useRealTimers());
  const box = (run: (q: string) => Found[]) => {
    const el = searchBox(places, run, () => {});
    document.body.append(el);
    return { el, input: el.querySelector("input")!, empty: el.querySelector<HTMLElement>(".results-empty")! };
  };
  const type = (input: HTMLInputElement, text: string) => {
    input.value = text;
    input.dispatchEvent(new Event("input"));
  };

  it("after a number, asks for the street instead of calling it a mistake", () => {
    vi.useFakeTimers();
    const { el, input, empty } = box(() => []);
    type(input, "1200 e");
    vi.advanceTimersByTime(500);
    expect(empty.hidden).toBe(false);
    expect(empty.textContent).toBe("Now the street, like 1200 E Speedway.");
    // Five digits on their own read as a ZIP code, which search can't use
    type(input, "85719");
    vi.advanceTimersByTime(500);
    expect(empty.textContent).toContain("ZIP codes");
    el.remove();
  });
});
