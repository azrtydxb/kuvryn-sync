import { test, expect } from "@playwright/test";
test("TestConsolePages", async ({ page }) => {
  await page.goto("/apps");
  for (const label of [
    "Applications",
    "Healthy",
    "Not synced",
    "Awaiting approval",
  ]) {
    await expect(page.locator(".az-stat", { hasText: label })).toBeVisible();
  }
  await expect(page.getByText("payments is Degraded")).toBeVisible();
  await page.getByRole("button", { name: "View diagnosis" }).click();
  await expect(page.getByText("MissingSecret")).toBeVisible();
  await expect(page.getByText("Chain to root cause")).toBeVisible();
  await expect(page.getByText("ksync diagnose payments")).toBeVisible();
  for (const tab of ["Overview", "Plan", "History", "Resources"]) {
    await page.getByRole("tab", { name: new RegExp(tab) }).click();
    await expect(page.locator("main")).not.toContainText("Error");
  }
  for (const [nav, heading] of [
    ["Repositories", "Repositories"],
    ["Revisions", "Revisions"],
    ["Image policies", "Image policies"],
  ]) {
    await page.getByRole("button", { name: nav }).click();
    await expect(page.getByRole("heading", { name: heading })).toBeVisible();
  }
});
