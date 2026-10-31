import { expect, test, type Page } from "@playwright/test";

const PLACE = "700 block of E Speedway Blvd";

const open = async (page: Page, hash = "") => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto(`/${hash}`);
  await expect(page.locator(".years h3")).toBeVisible();
};

const pickedRow = (page: Page) => page.locator('tr[aria-current="true"]');

test("a word from a permit's description finds it", async ({ page }) => {
  await open(page);
  await page.getByRole("combobox", { name: /Search/ }).fill("sidewalk");
  const group = page.getByRole("group", { name: "Permits" });
  await expect(group).toBeVisible();
  const hit = group.getByRole("option").first();
  await expect(hit).toContainText("T06EX01005, 2006");
  await expect(hit.locator("mark")).toHaveText("SIDEWALK");
  await hit.click();

  await expect(page.getByRole("heading", { level: 2, name: PLACE })).toBeVisible();
  await expect(pickedRow(page)).toHaveCount(1);
  await expect(pickedRow(page)).toContainText("SIDEWALK");
  await expect(pickedRow(page)).toBeInViewport();
  await expect(page.locator(".picked .permit-open")).toBeFocused();
  expect(page.url()).toContain("permit=T06EX01005");
});

test("the arrow keys reach the permit hits", async ({ page }) => {
  await open(page);
  const search = page.getByRole("combobox", { name: /Search/ });
  await search.fill("sidewalk");
  await expect(page.getByRole("group", { name: "Permits" })).toBeVisible();
  await search.press("ArrowDown");
  await expect(page.getByRole("listbox").getByRole("option", { selected: true })).toContainText("T06EX01005");
  await search.press("Enter");
  await expect(page.getByRole("heading", { level: 2, name: PLACE })).toBeVisible();
  await expect(pickedRow(page)).toHaveCount(1);
});

for (const [how, hash] of [["lower case", "#permit=t06ex01005"], ["spaces around it", "#permit=%20T06EX01005%20"]]) {
  test(`a link to a permit with ${how} opens its place with the permit picked out`, async ({ page }) => {
    await open(page, hash);
    await expect(page.getByRole("heading", { level: 2, name: PLACE })).toBeVisible();
    await expect(pickedRow(page)).toHaveCount(1);
  });
}

test("a link to a permit opens its place with the permit picked out", async ({ page }) => {
  await open(page, "#permit=T06EX01005");
  await expect(page.getByRole("heading", { level: 2, name: PLACE })).toBeVisible();
  await expect(pickedRow(page)).toHaveCount(1);
  await expect(pickedRow(page)).toBeInViewport();
  await expect(page.locator("#status")).toHaveText(`Showing permit T06EX01005, ${PLACE}.`);
  await expect(page).toHaveURL(/permit=T06EX01005/);

  await page.reload();
  await expect(pickedRow(page)).toHaveCount(1);
});

test("a permit outside the chosen years widens them", async ({ page }) => {
  await open(page, "#from=2005&to=2005&permit=T06EX01005");
  await expect(pickedRow(page)).toHaveCount(1);
  await expect(page.locator(".years h3")).toHaveText(/2005 to 2006$/);
});

test("a link to a permit that isn't on record says so", async ({ page }) => {
  await open(page, "#permit=T00XX000000");
  await expect(page.locator("#status")).toHaveText("No permit T00XX000000 is on record.");
  await expect(page).not.toHaveURL(/permit=/);
  await expect(page.locator(".note")).toHaveText("No permit T00XX000000 is on record.");
  await expect(page.getByRole("heading", { level: 2, name: /places in view/ })).toBeVisible();
});

test("with the permit server down", async ({ page }) => {
  await page.route("**/v1/**", (r) => r.abort());
  await open(page, "#permit=T06EX01005");
  const alert = page.getByRole("alert");
  await expect(alert).toContainText("Couldn't load permit T06EX01005. Couldn't reach the permit server.");
  await expect(page).toHaveURL(/permit=T06EX01005/);
  await page.unroute("**/v1/**");
  await alert.getByRole("button", { name: "Try again" }).click();
  await expect(page.getByRole("heading", { level: 2, name: PLACE })).toBeVisible();
  await expect(pickedRow(page)).toHaveCount(1);
  // The keyboard follows: the pressed button has gone, so it lands on the permit
  await expect(page.locator(".picked .permit-open")).toBeFocused();
  await expect(alert).toBeHidden();
});

test("with the permit server down, places are still found and permit search says it's unavailable", async ({ page }) => {
  await page.route("**/v1/search**", (r) => r.abort());
  await open(page);
  await page.getByRole("combobox", { name: /Search/ }).fill("speedway");
  await expect(page.getByText("Permit search is unavailable right now.")).toBeVisible();
  await expect(page.getByRole("option").first()).toContainText("Speedway");
});
