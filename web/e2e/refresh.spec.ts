import { test, expect } from "@playwright/test";
import { execSync } from "node:child_process";
test("TestLiveRefresh", async ({ page }) => {
  // A reused dev server may still hold an earlier run's change.
  execSync("go run ./hack/console-dev set-health catalog Healthy", {
    cwd: "..",
    stdio: "inherit",
  });
  await page.goto("/apps");
  await expect(
    page.getByRole("row", { name: /catalog.*Healthy/ }),
  ).toBeVisible();
  execSync("go run ./hack/console-dev set-health catalog Degraded", {
    cwd: "..",
    stdio: "inherit",
  });
  await expect(
    page.getByRole("row", { name: /catalog.*Degraded/ }),
  ).toBeVisible({ timeout: 11000 });
});
