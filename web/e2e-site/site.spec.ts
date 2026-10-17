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

test("zoomed out, the older entries in the key stay readable", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/");
  await expect(page.getByText("Zoom in to see older work")).toBeVisible();
  // Only the marks fade; the year labels keep their full contrast
  const older = page.locator(".years .key .older").first();
  await expect(older).toHaveCSS("opacity", "1");
  await expect(older.locator("svg")).not.toHaveCSS("opacity", "1");
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
  await expect(readout).toHaveText(/^\d+ block · \d+ permits?$/);
  const first = await readout.textContent();
  await page.keyboard.press("Home");
  await expect(readout).not.toHaveText(first!);
  await expect(readout).toHaveText(/^\d+ block · \d+ permits?$/);
  const mark = await page.locator(".station:focus").evaluate((b) => parseFloat(getComputedStyle(b, "::before").width));
  expect(mark).toBeGreaterThanOrEqual(8);
});

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

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
