import { expect, test } from "@playwright/test";

// the main path through the page
test("search an address", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  // Map tiles come from OpenStreetMap; the test doesn't need them
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Busiest places in view" })).toBeVisible();

  const search = page.getByRole("combobox");
  await search.fill("749 E Speedway");
  await expect(page.getByRole("option").first()).toContainText("700 block of E Speedway Blvd");
  await search.press("Enter");

  const heading = page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" });
  await expect(heading).toBeVisible();
  await expect(page.locator(".history .permit").first()).toContainText("Excavation");
  expect(page.url()).toContain("place=b%3AE+SPEEDWAY+BLVD%3A700");

  const link = page.url();
  await page.goto("about:blank");
  await page.goto(link);
  await expect(heading).toBeVisible();

  // the map is now centred on that block: clear the panel and click its dot
  await page.getByRole("button", { name: "Clear" }).click();
  await expect(heading).toBeHidden();
  const box = (await page.locator("#map").boundingBox())!;
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
  await expect(heading).toBeVisible();
  await expect(page.locator(".history .permit")).not.toHaveCount(0);

  expect(errors).toEqual([]);
});

test("a link restores the years", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/#from=2006&to=2006");
  const total = page.locator(".years h3");
  await expect(total).toHaveText(/permits, 2006$/);

  const first = page.getByRole("slider", { name: "First year" });
  await first.focus();
  await page.keyboard.press("ArrowLeft");
  await expect(first).toHaveAttribute("aria-valuenow", "2005");
  await expect(total).toHaveText(/permits, 2005 to 2006$/);
  expect(page.url()).toContain("from=2005");
});

test("replay starts at the first year and steps to the last", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/#from=2005&to=2006");
  const total = page.locator(".years h3");
  const play = page.getByRole("button", { name: "Replay" });
  await play.click();
  await expect(page.getByRole("button", { name: "Stop" })).toBeVisible();
  await expect(total).toHaveText(/permits, 2005$/);
  await expect(total).toHaveText(/permits, 2005 to 2006$/, { timeout: 3000 });
  await expect(play).toBeVisible({ timeout: 3000 });
});

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test("picking a place brings its summary into view", async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
    await page.goto("/");
    await page.getByRole("combobox").fill("749 E Speedway");
    await page.getByRole("combobox").press("Enter");
    const heading = page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" });
    await expect(heading).toBeInViewport();
    await expect(page.locator(".lead")).toBeInViewport();
  });

});
