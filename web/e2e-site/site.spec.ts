import { expect, test } from "@playwright/test";

test("the built site loads its data and opens a place", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());

  await page.goto("/");
  await expect(page.locator(".load-error")).toHaveCount(0);
  const busiest = page.locator(".busiest button");
  await expect(busiest.first()).toBeVisible();

  const name = await busiest.first().textContent();
  await busiest.first().click();
  await expect(page.getByRole("heading", { level: 2, name: name! })).toBeVisible();
  await expect(page.locator(".permit-row").first()).toBeVisible();
  await expect(page.locator(".focus .error")).toHaveCount(0);

  expect(errors).toEqual([]);
});

test("search forgives the way people really type", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/");
  const search = page.getByRole("combobox", { name: /Search/ });
  const first = page.getByRole("option").first();

  await search.fill("speedwy");
  await expect(first).toContainText("Speedway");
  await search.fill("e speedway blvd 1200");
  await expect(first).toContainText("1200 block of E Speedway Blvd");
  await search.fill("speedway & grant");
  await expect(page.locator(".results-note")).toHaveText(/^Speedway Blvd and Grant Rd don't meet/);
  await expect(first).toContainText("E ");
  await search.fill("85719");
  await expect(page.locator(".results-empty")).toContainText("ZIP codes");
});

test("unchecking a kind doesn't resize the map", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.setViewportSize({ width: 1400, height: 900 });
  await page.goto("/");
  await expect(page.locator(".years h3")).toBeVisible();
  const height = () => page.locator("#map").evaluate((el) => el.getBoundingClientRect().height);
  const before = await height();
  await page.locator('.kind[aria-pressed="true"]').first().click();
  await expect(page.getByRole("button", { name: "Reset filters" })).toBeVisible();
  expect(await height()).toBe(before);

  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  await expect(page.locator(".years h3")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

test("a street's profile says which block the keyboard is on", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/#q=e+speedway+blvd");
  const readout = page.locator(".profile-readout");
  await page.locator('.station[tabindex="0"]').focus();
  await expect(readout).toHaveText(/^\d+(–\d+)? blocks? · \d+ digs?( · opens the \d+ block)?$/);
  const first = await readout.textContent();
  await page.keyboard.press("Home");
  await expect(readout).not.toHaveText(first!);
  await expect(readout).toHaveText(/^\d+(–\d+)? blocks? · \d+ digs?( · opens the \d+ block)?$/);
  const mark = await page.locator(".station:focus").evaluate((b) => parseFloat(getComputedStyle(b, "::before").width));
  expect(mark).toBeGreaterThanOrEqual(8);
});

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test("a place opened from a link scrolls to its answer", async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
    await page.goto("/#place=b%3AE+SPEEDWAY+BLVD%3A1200");
    await expect(page.locator(".digs")).toBeVisible();
    // As a place opened by hand does: its name at the top, the answer under it
    await expect.poll(async () => (await page.getByRole("heading", { level: 2, name: "1200 block of E Speedway Blvd" }).boundingBox())!.y).toBeLessThan(120);
  });

  test("the year total keeps inside the page's side gutter", async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
    await page.goto("/");
    const total = page.locator(".years .head h3");
    await expect(total).toHaveText(/permits/);
    const right = await total.evaluate((t) => {
      const range = document.createRange();
      range.selectNodeContents(t);
      return range.getBoundingClientRect().right;
    });
    expect(right).toBeLessThanOrEqual(390 - 16);
    // Wrapped, its lines are near even: no lone "2026".
    const lines = await total.evaluate((t) => {
      const r = document.createRange();
      r.selectNodeContents(t);
      return [...r.getClientRects()].map((x) => x.width);
    });
    if (lines.length > 1) expect(Math.min(...lines) / Math.max(...lines)).toBeGreaterThan(0.5);
  });

  test("a first tap on the street profile previews a bar; a second opens it", async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
    await page.goto("/#q=e+speedway+blvd");
    const bar = page.locator(".station").nth(5);
    await bar.scrollIntoViewIfNeeded();
    await bar.tap();
    await expect(page.locator(".profile-readout")).toHaveText(/ · opens the \d+ block$/);
    await expect(page.getByRole("heading", { level: 2, name: "E Speedway Blvd" })).toBeVisible();
    await bar.tap();
    await expect(page.getByRole("button", { name: "Back to E Speedway Blvd" })).toBeVisible();
  });

  test("the pinned place header is whole while its record scrolls", async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
    await page.goto("/#place=b%3AE+SPEEDWAY+BLVD%3A1200");
    const heading = page.getByRole("heading", { level: 2, name: "1200 block of E Speedway Blvd" });
    await expect(heading).toBeVisible();
    // Partway down the record, where the head has pinned itself
    await page.locator(".permit-row").nth(12).evaluate((r) => window.scrollTo(0, r.getBoundingClientRect().top + window.scrollY - 300));
    await expect(heading).toBeInViewport({ ratio: 1 });
    await expect(page.getByRole("button", { name: "Clear" })).toBeInViewport({ ratio: 1 });
  });

});

test("the legend's counts follow Replay as it runs", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/");
  const count = page.getByRole("button", { name: /Excavation/ }).locator(".kind-n");
  const full = await count.textContent();
  await page.getByRole("button", { name: "Replay" }).click();
  await expect(count).not.toHaveText(full!, { timeout: 2000 });
  await page.getByRole("button", { name: "Stop" }).click();
  await expect(count).not.toHaveText(full!);
  await page.getByRole("button", { name: "Reset filters" }).click();
  await expect(count).toHaveText(full!);
});

test("one kind can be shown on its own", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/");
  await page.getByRole("combobox", { name: "Show only one kind" }).selectOption("Utility");
  await expect(page.getByRole("button", { name: /^Utility/ })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: /^Excavation/ })).toHaveAttribute("aria-pressed", "false");
  await expect(page.locator(".legend .cell-count")).toHaveText("1 of 9 shown");
  await expect(page.locator("#status")).toHaveText(/^Only Utility shown\. /);
});

test("Esc from a block opened on the street profile lands back on its bar", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/#q=e+speedway+blvd");
  await expect(page.locator(".focus")).toContainText("Busiest stretch: the ");
  await expect(page.getByRole("heading", { name: "Busiest single blocks" })).toBeVisible();
  const bar = page.locator('.station[tabindex="0"]');
  await bar.focus();
  await page.keyboard.press("Home");
  const label = await page.locator(".station:focus").getAttribute("aria-label");
  expect(label).toMatch(/, opens the \d+ block$/);
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button", { name: "Back to E Speedway Blvd" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator(".station:focus")).toHaveAttribute("aria-label", label!);
});

test("with no digging kind shown, the list ranks by permits and says so", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/");
  await page.getByRole("combobox", { name: "Show only one kind" }).selectOption("Utility");
  await expect(page.getByRole("heading", { level: 2, name: "Busiest places in view" })).toBeVisible();
  await expect(page.locator(".focus")).toContainText("Counts are permits; no digging kind is shown.");
});

test("zoomed out, the chart draws its years in one tone, as the dots are", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/");
  await expect(page.locator(".years .key")).toContainText("Size: times dug up");
  const tone = (sel: string) => page.locator(sel).first().evaluate((b) => getComputedStyle(b).backgroundColor);
  expect(await tone(".chart .bars .back")).toBe(await tone(".chart .bars .recent"));
  expect(await tone(".chart .bars .old")).toBe(await tone(".chart .bars .recent"));
});

test("a street ranks its blocks by digs, as the home list does", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/#q=e+speedway+blvd");
  await expect(page.locator(".focus .busiest li .n").first()).toHaveText(/^\d+ digs?$/);
  await expect(page.locator(".focus .sub").first()).toContainText("dug up");
});

test("a finished replay picks out the blocks dug again and again", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/#from=2020&to=2026");
  await expect(page.locator(".years h3")).toHaveText(/permits/);
  const drawn = () => page.locator("canvas.dots").evaluate((c: HTMLCanvasElement) => {
    const d = c.getContext("2d")!.getImageData(0, 0, c.width, c.height).data;
    let h = 0;
    for (let i = 0; i < d.length; i += 97) h = (h * 31 + d[i]!) | 0;
    return h;
  });
  await page.waitForTimeout(500);
  const before = await drawn();
  await page.getByRole("button", { name: "Replay" }).click();
  await expect(page.locator(".replay-note")).toHaveText(/ of [\d,]+ blocks dug up 5\+ times, picked out on the map/, { timeout: 8000 });
  await page.waitForTimeout(300);
  expect(await drawn()).not.toBe(before);
  // Changing the years puts the map back as it was
  await page.getByRole("slider", { name: "First year" }).press("ArrowLeft");
  await page.getByRole("slider", { name: "First year" }).press("ArrowRight");
  await page.waitForTimeout(300);
  expect(await drawn()).toBe(before);
});

test("with a digging kind shown, a street's profile counts digs", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/#q=e+speedway+blvd");
  await expect(page.locator('.station[tabindex="0"]')).toHaveAttribute("aria-label", /, \d+ digs?, opens the \d+ block$/);
  await expect(page.locator(".focus")).toContainText(/Busiest stretch: the [\d–]+ blocks, [\d,]+ digs of [\d,]+\./);
});
