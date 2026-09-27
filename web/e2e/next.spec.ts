import { expect, test, type Page } from "@playwright/test";

// The ground-up rebuild at next/: the same features as the first page, checked the same way

const open = async (page: Page, hash = "") => {
  await page.route(/tile\.openstreetmap\.org/, (r) => r.abort());
  await page.goto(`/next/${hash}`);
  await expect(page.locator(".rev-total")).toBeVisible();
};

test("search an address and read its revision table down to the city record", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await open(page);
  const search = page.getByRole("combobox", { name: /Search/ });
  await search.fill("749 E Speedway");
  await expect(page.getByRole("option").first()).toContainText("700 block of E Speedway Blvd");
  await search.press("Enter");

  const heading = page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" });
  await expect(heading).toBeFocused();
  await expect(page.locator(".lead")).toContainText(/permits? (in|from)/);
  const first = page.locator(".rev-open").first();
  await expect(first).toContainText("Excavation");
  await first.click();
  await expect(first).toHaveAttribute("aria-expanded", "true");
  await expect(page.locator(".rev-more").first()).toContainText("Filed as");
  await expect(page.locator(".redline-mark .delta")).toHaveText(/^Δ\d+$/);
  expect(page.url()).toContain("place=b%3AE+SPEEDWAY+BLVD%3A700");
  expect(errors).toEqual([]);
});

test("a link restores the place and years; Back returns to the busiest list", async ({ page }) => {
  await open(page, "#from=2006&to=2006");
  await expect(page.locator(".rev-total")).toHaveText(/permits, 2006$/);
  await page.getByRole("button", { name: /block of E Speedway Blvd/ }).first().click();
  await expect(page.getByRole("heading", { level: 2, name: /block of E Speedway Blvd/ })).toBeVisible();
  await page.goBack();
  await expect(page.getByRole("heading", { name: "Busiest places in view" })).toBeVisible();
  expect(page.url()).toContain("from=2006");
});

test("the year handles move with the keyboard", async ({ page }) => {
  await open(page, "#from=2006&to=2006");
  const firstYear = page.getByRole("slider", { name: "First year" });
  await firstYear.focus();
  await page.keyboard.press("ArrowLeft");
  await expect(firstYear).toHaveAttribute("aria-valuenow", "2005");
  await expect(page.locator(".rev-total")).toHaveText(/2005 to 2006$/);

  await page.getByRole("button", { name: "Replay" }).click();
  await expect(page.getByRole("button", { name: "Stop" })).toBeVisible();
  await expect(page.locator(".rev-total")).toHaveText(/permits, 2005$/);
  await expect(page.getByRole("button", { name: "Replay" })).toBeVisible({ timeout: 4000 });
  await expect(page.locator(".rev-total")).toHaveText(/2005 to 2006$/);
});

test("the legend switches a kind off and on", async ({ page }) => {
  await open(page);
  const kind = page.getByRole("button", { name: /Excavation/ });
  await expect(kind).toHaveAttribute("aria-pressed", "true");
  await kind.click();
  await expect(page.getByRole("button", { name: /Excavation/ })).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByRole("button", { name: /Excavation/ })).toBeFocused();
  await expect(page.getByRole("button", { name: "Show all kinds of work" })).toBeVisible();
  await page.getByRole("button", { name: "Reset filters" }).click();
  await expect(page.getByRole("button", { name: /Excavation/ })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "Reset filters" })).toBeHidden();
});

test("a whole street opens as a profile whose stations open their blocks", async ({ page }) => {
  await open(page);
  const search = page.getByRole("combobox", { name: /Search/ });
  await search.fill("e speedway blvd");
  await expect(page.getByRole("option").first()).toContainText("Whole street");
  await search.press("Enter");
  await expect(page.getByRole("heading", { level: 2, name: "E Speedway Blvd" })).toBeVisible();
  await page.getByRole("button", { name: /^700 block, \d+ permits?$/ }).click();
  await expect(page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" })).toBeVisible();
});

test("general notes say what the data can't", async ({ page }) => {
  await open(page);
  await page.getByRole("button", { name: "General notes" }).click();
  await expect(page.getByRole("heading", { level: 2, name: "General notes" })).toBeFocused();
  await expect(page.locator(".general-notes")).toContainText("thin");
  await expect(page.locator(".kind-meanings")).toContainText("Excavation");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("heading", { name: "Busiest places in view" })).toBeVisible();
});

test("from the keyboard, the map says what Enter opens", async ({ page }) => {
  await open(page, "#place=b%3AE+SPEEDWAY+BLVD%3A700");
  await page.keyboard.press("Escape");
  const map = page.locator("#map");
  await map.focus();
  await expect(page.locator(".aim-line")).toHaveText(/^Enter opens 700 block of E Speedway Blvd/);
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { level: 2, name: "700 block of E Speedway Blvd" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(map).toBeFocused();
});
