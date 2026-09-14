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
  await expect(page.locator(".history .permit").first()).toBeVisible();
  await expect(page.locator(".history .error")).toHaveCount(0);

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
  await page.locator(".cat input:checked").first().uncheck();
  await expect(page.getByRole("button", { name: "Reset filters" })).toBeVisible();
  expect(await height()).toBe(before);

  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  await expect(page.locator(".years h3")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});
