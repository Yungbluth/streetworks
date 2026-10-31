// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PermitHit } from "../api";
import { PlaceKind, type Places } from "../data";
import type { Focus } from "../model";
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

describe("permit search in the box", () => {
  afterEach(() => vi.useRealTimers());
  const hit = (id: number, snippet = "a <mark>curb</mark> ramp"): PermitHit => ({ id, place: 0, year: 2023, cat: 0, number: `T23SY0${id}`, snippet });
  const speedway = (): Found[] => [{ kind: "street", street: 0, bounds, permits: 3 }];
  const setup = (permits: (q: string, signal: AbortSignal) => Promise<PermitHit[]>, run: (q: string) => Found[] = () => []) => {
    vi.useFakeTimers();
    const chosen: Focus[] = [];
    const el = searchBox(places, run, (f) => chosen.push(f), undefined, permits);
    document.body.append(el);
    const input = el.querySelector("input")!;
    const type = (text: string) => {
      input.value = text;
      input.dispatchEvent(new Event("input"));
    };
    const key = (k: string) => input.dispatchEvent(new KeyboardEvent("keydown", { key: k }));
    const options = () => [...el.querySelectorAll<HTMLElement>("[role=option]")];
    return { el, input, type, key, options, chosen };
  };

  it("asks after a 250 ms pause once two characters are typed", async () => {
    const permits = vi.fn(async (_q: string, _signal: AbortSignal) => [] as PermitHit[]);
    const { el, type } = setup(permits);
    type("c");
    await vi.advanceTimersByTimeAsync(1000);
    expect(permits).not.toHaveBeenCalled();
    type("cu");
    await vi.advanceTimersByTimeAsync(249);
    expect(permits).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(permits).toHaveBeenCalledTimes(1);
    expect(permits.mock.calls[0]![0]).toBe("cu");
    el.remove();
  });

  it("asks once for the last of several quick keystrokes", async () => {
    const permits = vi.fn(async (_q: string, _signal: AbortSignal) => [] as PermitHit[]);
    const { el, type } = setup(permits);
    type("cur");
    await vi.advanceTimersByTimeAsync(100);
    type("curb");
    await vi.advanceTimersByTimeAsync(250);
    expect(permits).toHaveBeenCalledTimes(1);
    expect(permits.mock.calls[0]![0]).toBe("curb");
    el.remove();
  });

  it("lists hits under a Permits heading, after the places", async () => {
    const { el, type, options } = setup(async () => [hit(7, "dig &amp; <mark>curb</mark> <b>x</b><img src=x onerror=alert(1)><!--c-->")], speedway);
    type("speedway");
    expect(options()).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(250);
    const group = el.querySelector("[role=listbox] [role=group]")!;
    const heading = group.querySelector("h4")!;
    expect(heading.textContent).toBe("Permits");
    expect(group.getAttribute("aria-labelledby")).toBe(heading.id);
    expect(options()).toHaveLength(2);
    const option = group.querySelector("[role=option]")!;
    expect(option.textContent).toContain("T23SY07");
    expect(option.textContent).toContain("2023");
    const snippet = option.querySelector(".detail")!;
    expect(snippet.textContent).toBe("dig & curb x");
    expect(snippet.querySelector("mark")!.textContent).toBe("curb");
    expect(snippet.querySelector("b, img")).toBeNull();
    expect([...snippet.querySelectorAll("*")].every((n) => n.localName === "mark")).toBe(true);
    el.remove();
  });

  it("moves with the arrow keys from the places into the permits", async () => {
    const { el, input, type, key, options, chosen } = setup(async () => [hit(7)], speedway);
    type("speedway");
    await vi.advanceTimersByTimeAsync(250);
    key("ArrowDown");
    expect(input.getAttribute("aria-activedescendant")).toBe(options()[0]!.id);
    key("ArrowDown");
    expect(input.getAttribute("aria-activedescendant")).toBe(options()[1]!.id);
    expect(options()[1]!.getAttribute("aria-selected")).toBe("true");
    key("Enter");
    expect(chosen).toEqual([{ kind: "place", place: 0, permit: 7 }]);
    el.remove();
  });

  it("says when permit search fails", async () => {
    const { el, type, options } = setup(() => Promise.reject(new Error("down")), speedway);
    type("speedway");
    await vi.advanceTimersByTimeAsync(250);
    expect(el.textContent).toContain("Permit search is unavailable right now.");
    expect(options()).toHaveLength(1);
    expect(options()[0]!.textContent).toContain("Speedway");
    // A new query clears it until that one fails too
    type("speedwa");
    const note = [...el.querySelectorAll<HTMLElement>(".results-note")].find((n) => !n.hidden);
    expect(note).toBeUndefined();
    expect(el.textContent).not.toContain("Permit search is unavailable");
    el.remove();
  });
});
