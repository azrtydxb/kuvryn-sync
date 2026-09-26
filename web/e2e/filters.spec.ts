import { test, expect, type Page } from "@playwright/test";
import { execSync } from "node:child_process";

// hack/console-dev seeds eight Applications; the refresh test may have left
// catalog Degraded on a reused server.
test.beforeAll(() => {
  execSync("go run ./hack/console-dev set-health catalog Healthy", {
    cwd: "..",
    stdio: "inherit",
  });
});

const rows = (page: Page) => page.locator("main table tbody tr");

test("TestFiltersNarrowRowsAndPersistInTheURL", async ({ page }) => {
  await page.goto("/apps");
  await expect(rows(page)).toHaveCount(8);
  await expect(page.getByText("8 of 8")).toBeVisible();

  await page.getByLabel("Search applications").fill("platform");
  await expect(rows(page)).toHaveCount(5);
  await page
    .getByRole("radiogroup", { name: "Health" })
    .getByRole("radio", { name: "Healthy" })
    .click();
  await expect(rows(page)).toHaveCount(3);
  await page.getByLabel("Sync", { exact: true }).selectOption("Drifted");
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first()).toContainText("search");
  await expect(page.getByText("1 of 8")).toBeVisible();
  await expect(page).toHaveURL(/[?&]q=platform(&|$)/);
  await expect(page).toHaveURL(/[?&]sync=Drifted(&|$)/);
  await expect(page).toHaveURL(/[?&]health=Healthy(&|$)/);

  // Reload and Back both restore the filters and their controls.
  await page.reload();
  await expect(rows(page)).toHaveCount(1);
  await expect(page.getByLabel("Search applications")).toHaveValue("platform");
  await expect(page.getByLabel("Sync", { exact: true })).toHaveValue("Drifted");
  await expect(
    page.getByRole("radio", { name: "Healthy", checked: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Repositories" }).click();
  await expect(page).toHaveURL(/\/repositories$/);
  await page.goBack();
  await expect(rows(page)).toHaveCount(1);

  await page.getByRole("button", { name: "Clear filters" }).click();
  await expect(rows(page)).toHaveCount(8);
  await expect(page).toHaveURL(/\/apps$/);
});

test("TestStatCardsSetFilters", async ({ page }) => {
  await page.goto("/apps");
  await expect(rows(page)).toHaveCount(8);
  await page.getByRole("button", { name: /^Awaiting approval/ }).click();
  await expect(page).toHaveURL(/[?&]sync=AwaitingApproval(&|$)/);
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first()).toContainText("checkout");

  // Keyboard: Enter on "Not synced" filters to diverged and converging apps.
  await page.getByRole("button", { name: /^Not synced/ }).focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/[?&]sync=NotSynced(&|$)/);
  await expect(rows(page)).toHaveCount(3);
  for (const name of ["payments", "search", "notifications"]) {
    await expect(page.getByRole("link", { name, exact: true })).toBeVisible();
  }
  await expect(
    page.getByRole("button", { name: /^Not synced/ }),
  ).toHaveAttribute("aria-pressed", "true");

  await page.getByRole("button", { name: /^Healthy/ }).focus();
  await page.keyboard.press(" ");
  await expect(page).toHaveURL(/[?&]health=Healthy(&|$)/);
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first()).toContainText("search");
});

test("TestFiltersShowAnEmptyState", async ({ page }) => {
  await page.goto("/apps?q=no-such-application");
  await expect(
    page.getByRole("heading", { name: "No applications match these filters" }),
  ).toBeVisible();
  await expect(page.getByText("0 of 8")).toBeVisible();
  await expect(page.locator("main table")).toHaveCount(0);
  await page
    .getByRole("main")
    .getByRole("button", { name: "Clear filters" })
    .last()
    .click();
  await expect(rows(page)).toHaveCount(8);
  await expect(page).toHaveURL(/\/apps$/);
});

test("TestEveryTableFilters", async ({ page }) => {
  await page.goto("/revisions?app=payments&phase=Failed");
  await expect(rows(page)).toHaveCount(1);
  await expect(page.getByLabel("Application", { exact: true })).toHaveValue(
    "payments",
  );
  await page.getByLabel("Search revisions").fill("catalog");
  await expect(
    page.getByRole("heading", { name: "No revisions match these filters" }),
  ).toBeVisible();

  await page.goto("/repositories");
  await expect(rows(page)).toHaveCount(3);
  await page
    .getByRole("radiogroup", { name: "State" })
    .getByRole("radio", { name: "Failed" })
    .click();
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first()).toContainText("finance-gitops");
  await expect(page).toHaveURL(/\?state=Failed$/);

  await page.goto("/imagepolicies");
  await page.getByLabel("Search image policies").fill("acme/check");
  await expect(rows(page)).toHaveCount(1);
  await expect(page).toHaveURL(/\?q=acme%2Fcheck$/);

  await page.goto("/apps/default/payments/resources");
  await expect(rows(page).first()).toBeVisible();
  const total = await rows(page).count();
  await page.getByLabel("Kind", { exact: true }).selectOption("Deployment");
  await expect(rows(page)).toHaveCount(1);
  await page.getByLabel("Kind", { exact: true }).selectOption("");
  await expect(rows(page)).toHaveCount(total);
  await page.getByText("Only not synced or unhealthy").click();
  await expect(page).toHaveURL(/[?&]attention=1(&|$)/);
  await expect(rows(page)).not.toHaveCount(total);
  const narrowed = await rows(page).count();
  expect(narrowed).toBeGreaterThan(0);
  expect(narrowed).toBeLessThan(total);
  await expect(
    rows(page).filter({ hasText: "Synced" }).filter({ hasText: "Healthy" }),
  ).toHaveCount(0);
  await expect(page.getByText(narrowed + " of " + total)).toBeVisible();

  await page.goto("/apps/default/payments/history?phase=Failed");
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first()).toContainText("Failed");
});
