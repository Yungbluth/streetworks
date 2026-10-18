// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { streetProfile, stretches } from "./street";

const blocks = [{ block: 100, n: 3 }, { block: 200, n: 0 }, { block: 300, n: 9 }, { block: 400, n: 1 }];

describe("streetProfile", () => {
  const opened: number[] = [];
  const el = () => streetProfile(blocks.map((b) => ({ ...b, open: () => opened.push(b.block) })), (b) => `${b}`);

  it("is one Tab stop", () => {
    const stations = [...el().querySelectorAll<HTMLButtonElement>(".station")];
    expect(stations.map((s) => s.tabIndex)).toEqual([-1, -1, 0, -1]);
    expect(stations[2]!.getAttribute("aria-label")).toBe("300 block, 9 permits");
  });

  it("moves between stations with the arrow keys", () => {
    const p = el();
    document.body.append(p);
    const stations = [...p.querySelectorAll<HTMLButtonElement>(".station")];
    stations[2]!.focus();
    stations[2]!.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    expect(document.activeElement).toBe(stations[3]);
    stations[3]!.dispatchEvent(new KeyboardEvent("keydown", { key: "Home", bubbles: true }));
    expect(document.activeElement).toBe(stations[0]);
    expect(stations[0]!.tabIndex).toBe(0);
    stations[0]!.click();
    expect(opened).toEqual([100]);
    p.remove();
  });

  it("labels the ends and evenly spaced stations between", () => {
    const labels = [...el().querySelectorAll(".profile-axis span")].map((s) => s.textContent);
    expect(labels[0]).toBe("100");
    expect(labels.at(-1)).toBe("400");
  });
});

describe("stretches", () => {
  const long = Array.from({ length: 110 }, (_, i) => ({ block: i * 100, n: i === 31 ? 50 : 1, open: () => {} }));

  it("leaves a short street a bar per block", () => {
    const short = long.slice(0, 30);
    expect(stretches(short)).toHaveLength(30);
    expect(stretches(short)[0]!.name).toBeUndefined();
  });

  it("groups a long street into equal stretches of addresses, forty bars at most", () => {
    const bars = stretches(long);
    expect(bars.length).toBeLessThanOrEqual(40);
    expect(bars[0]).toMatchObject({ block: 0, n: 3, name: "0–200 blocks" });
    expect(bars[10]).toMatchObject({ block: 3000, n: 52, name: "3000–3200 blocks", opens: 3100 });
  });

  it("opens the busiest block in a stretch", () => {
    const opened: number[] = [];
    const bars = stretches(long.map((b) => ({ ...b, open: () => opened.push(b.block) })));
    bars[10]!.open();
    expect(opened).toEqual([3100]);
  });
});
