import { test, type Page } from "@playwright/test";
import { execSync } from "node:child_process";

// Writes the documentation screenshots. Run it with the real emblems in
// place: KSYNC_SCREENSHOTS=1 make test-ui
test.skip(
  !process.env.KSYNC_SCREENSHOTS,
  "set KSYNC_SCREENSHOTS=1 to write docs/images",
);

test.beforeAll(() => {
  // The refresh test may have left catalog Degraded on a reused server.
  execSync("go run ./hack/console-dev set-health catalog Healthy", {
    cwd: "..",
    stdio: "inherit",
  });
});

const shots: [string, string, ((page: Page) => Promise<void>)?][] = [
  ["console-login", "/login"],
  ["console-applications", "/apps"],
  ["console-diagnosis", "/apps/default/payments/diagnosis"],
  ["console-filters", "/apps?q=platform&health=Healthy"],
  [
    "console-tooltip",
    "/apps?sync=NotSynced",
    async (page) => {
      await page
        .getByRole("row", { name: /payments/ })
        .locator(".ks-tip__target", { hasText: "OutOfSync" })
        .hover();
      await page.getByRole("tooltip").filter({ visible: true }).waitFor();
    },
  ],
  ["console-no-match", "/revisions?q=no-such-revision"],
];

for (const [name, path, act] of shots) {
  test(`screenshot ${name}`, async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.addInitScript(() => localStorage.setItem("ksync.theme", "dark"));
    await page.goto(path);
    await page.waitForLoadState("networkidle");
    if (act) await act(page);
    await page.screenshot({ path: `../docs/images/${name}.png` });
  });
}
