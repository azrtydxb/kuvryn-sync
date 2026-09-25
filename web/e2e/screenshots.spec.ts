import { test } from "@playwright/test";

// Writes the documentation screenshots. Run it with the real emblems in
// place: KSYNC_SCREENSHOTS=1 make test-ui
test.skip(
  !process.env.KSYNC_SCREENSHOTS,
  "set KSYNC_SCREENSHOTS=1 to write docs/images",
);

for (const [name, path] of [
  ["console-login", "/login"],
  ["console-applications", "/apps"],
  ["console-diagnosis", "/apps/default/payments/diagnosis"],
]) {
  test(`screenshot ${name}`, async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.addInitScript(() => localStorage.setItem("ksync.theme", "dark"));
    await page.goto(path);
    await page.waitForLoadState("networkidle");
    await page.screenshot({ path: `../docs/images/${name}.png` });
  });
}
