import { expect, test } from "@playwright/test";

// the main path through the page
test("search an address", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  // Map tiles come from OpenStreetMap; the test doesn't need them
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Busiest places in view" })).toBeVisible();

  const search = page.getByRole("combobox", { name: /Search/ });
  await search.fill("749 E Speedway");
  await expect(page.getByRole("option").first()).toContainText("700 block of E Speedway Blvd");
  await search.press("Enter");

  const heading = page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" });
  await expect(heading).toBeVisible();
  await expect(page.locator(".permit-row").first()).toContainText("Excavation");
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
  await expect(page.locator(".permit-row")).not.toHaveCount(0);

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

test("the zoom note shows only", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto("/#place=b%3AE+SPEEDWAY+BLVD%3A700");
  await expect(page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" })).toBeVisible();
  const note = page.getByText("Zoom in to see older work");
  await expect(note).toBeHidden();

  const out = page.locator(".leaflet-control-zoom-out");
  // clicks during a zoom animation are dropped, so keep zooming out until it shows
  await expect(async () => {
    await out.click();
    await expect(note).toBeVisible({ timeout: 500 });
  }).toPass({ timeout: 10_000 });
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
    await page.getByRole("combobox", { name: /Search/ }).fill("749 E Speedway");
    await page.getByRole("combobox", { name: /Search/ }).press("Enter");
    const heading = page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" });
    await expect(heading).toBeInViewport();
    await expect(page.locator(".lead")).toBeInViewport();
  });

  test("search suggestions sit above the map's zoom buttons", async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
    await page.goto("/");
    await page.getByRole("combobox", { name: /Search/ }).fill("749 E Speedway");
    const option = page.getByRole("option").first();
    await expect(option).toBeVisible();
    // Probe the left edge of the suggestion, where the zoom buttons sit
    const box = (await option.boundingBox())!;
    const onTop = await page.evaluate(
      ({ x, y }) => !!document.elementFromPoint(x, y)?.closest("[role=option]"),
      { x: box.x + 30, y: box.y + box.height / 2 },
    );
    expect(onTop).toBe(true);
  });

  test("the page never scrolls sideways", async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
    await page.goto("/");
    await expect(page.locator(".years h3")).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });

});

test.describe("getting back", () => {
  test.beforeEach(async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  });

  test("Back returns to the previous place", async ({ page }) => {
    await page.goto("/");
    const search = page.getByRole("combobox", { name: /Search/ });
    await search.fill("749 E Speedway");
    await search.press("Enter");
    await expect(page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" })).toBeVisible();
    await search.fill("250 S Williams");
    await search.press("Enter");
    await expect(page.getByRole("heading", { level: 2, name: "200 block of S Williams Blvd" })).toBeVisible();
    await page.goBack();
    await expect(page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" })).toBeVisible();
    await page.goBack();
    await expect(page.getByRole("heading", { level: 2, name: "Busiest places in view" })).toBeVisible();
  });

  test("Reset filters puts the years and kinds back", async ({ page }) => {
    await page.goto("/#from=2006&to=2006&cats=Other");
    const reset = page.getByRole("button", { name: "Reset filters" });
    await reset.click();
    await expect(page.locator(".years h3")).toHaveText(/permits, 2005 to 2006$/);
    await expect(reset).toBeHidden();
  });

  test("an empty map offers to show every kind of work", async ({ page }) => {
    await page.goto("/#cats=");
    await page.getByRole("button", { name: "Show all kinds of work" }).click();
    await expect(page.locator(".busiest button").first()).toBeVisible();
  });
});

test.describe("keyboard and screen reader", () => {
  test.beforeEach(async ({ page }) => {
    await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  });

  test("search is the first thing Tab reaches", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator(".years h3")).toHaveText(/permits/);
    await page.keyboard.press("Tab");
    await expect(page.getByRole("combobox", { name: /Search/ })).toBeFocused();
  });

  test("Esc closes the open place", async ({ page }) => {
    await page.goto("/#place=b%3AE+SPEEDWAY+BLVD%3A700");
    await expect(page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("heading", { name: "Busiest places in view" })).toBeVisible();
    expect(page.url()).not.toContain("place=");
  });

});

test("the legend sits beside the map", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.setViewportSize({ width: 1400, height: 900 });
  await page.goto("/");
  const kind = page.getByRole("button", { name: /Excavation/ });
  await expect(kind).toBeInViewport();
  await expect(kind).toHaveAttribute("aria-pressed", "true");
  await kind.click();
  await expect(page.getByRole("button", { name: /Excavation/ })).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByRole("button", { name: /Excavation/ })).toBeFocused();
  await page.getByRole("button", { name: "What the kinds mean" }).click();
  await expect(page.getByRole("heading", { name: "What the kinds mean" })).toBeInViewport();
});

test("changing the filters doesn't resize the map", async ({ page }) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.setViewportSize({ width: 1400, height: 900 });
  await page.goto("/");
  await expect(page.locator(".years h3")).toBeVisible();
  const height = () => page.locator("#map").evaluate((el) => el.getBoundingClientRect().height);
  const before = await height();
  await page.locator('.kind[aria-pressed="true"]').first().click();
  await expect(page.getByRole("button", { name: "Reset filters" })).toBeVisible();
  expect(await height()).toBe(before);
});
