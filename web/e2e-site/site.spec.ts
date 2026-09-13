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
