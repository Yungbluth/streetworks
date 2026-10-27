import { expect, test, type Page } from "@playwright/test";

const PLACE = "#place=b%3AE+SPEEDWAY+BLVD%3A700";

// The map, the years and the filters run on the page's own data
const open = async (page: Page, hash: string) => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto(`/${hash}`);
  await expect(page.locator(".years h3")).toBeVisible();
};

test("with the permit server down, the page keeps working and a retry recovers", async ({ page }) => {
  await page.route("**/v1/**", (r) => r.abort());
  await open(page, `${PLACE}&from=2006&to=2006`);

  await expect(page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" })).toBeVisible();
  const alert = page.getByRole("alert");
  await expect(alert).toContainText("Couldn't load this place's permits. Couldn't reach the permit server.");
  await expect(alert.getByRole("button", { name: "Try again" })).toBeVisible();

  // The years strip still answers
  const stamp = page.locator("#year-stamp");
  const before = await stamp.textContent();
  await page.getByRole("slider", { name: "First year" }).press("ArrowLeft");
  await expect(stamp).not.toHaveText(before!);

  await page.unroute("**/v1/**");
  await alert.getByRole("button", { name: "Try again" }).click();
  await expect(page.locator(".permit-row").first()).toContainText("Excavation");
  await expect(page.getByRole("alert")).toBeHidden();
});

test("a 503 from the permit server shows in the radius panel and Try again recovers", async ({ page }) => {
  // the fixture's blocks are miles apart
  await page.route(/\/data\/permits\.json$/, async (r) => {
    const res = await r.fetch();
    const permits = await res.json();
    permits.lat[4] = permits.lat[3] + 30;
    permits.lng[4] = permits.lng[3];
    await r.fulfill({ response: res, json: permits });
  });
  await page.route("**/v1/**", (r) => r.fulfill({ status: 503, body: "" }));
  await open(page, PLACE);
  await expect(page.getByText("1 more permit at 1 other place")).toBeVisible();
  await page.getByRole("button", { name: "List them, newest first" }).click();

  const alert = page.getByRole("alert").filter({ hasText: "the permits here" });
  await expect(alert).toContainText("Couldn't load the permits here. The permit server said 503.");

  await page.unroute("**/v1/**");
  await alert.getByRole("button", { name: "Try again" }).click();
  await expect(alert).toBeHidden();
  // The circle's table has the Away column; the place's own history, which wasn't retried, doesn't.
  await expect(page.getByRole("columnheader", { name: "Away" })).toBeVisible();
});
