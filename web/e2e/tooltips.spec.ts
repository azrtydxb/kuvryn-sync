import { test, expect, type Locator, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { execSync } from "node:child_process";

/** The one tooltip on the page that is showing. */
const shown = (page: Page) =>
  page.getByRole("tooltip").filter({ visible: true });

/**
 * Presses Tab until target, an element inside it, or its focusable wrapper
 * has focus.
 */
async function tabTo(page: Page, target: Locator) {
  await page.locator("body").focus();
  for (let i = 0; i < 120; i++) {
    await page.keyboard.press("Tab");
    const inside = await target.evaluate((el) => {
      const a = document.activeElement;
      if (!a || a === document.body) return false;
      return el === a || el.contains(a) || a.contains(el);
    });
    if (inside) return;
  }
  throw new Error("Tab never reached the target");
}

/** A local time as the console's tooltips show it: 2026-09-26 14:03:12. */
function local(iso: string): string {
  const d = new Date(iso);
  const p = (n: number) => String(n).padStart(2, "0");
  return (
    d.getFullYear() +
    "-" +
    p(d.getMonth() + 1) +
    "-" +
    p(d.getDate()) +
    " " +
    p(d.getHours()) +
    ":" +
    p(d.getMinutes()) +
    ":" +
    p(d.getSeconds())
  );
}

test("TestSyncBadgeTooltip", async ({ page }) => {
  await page.goto("/apps");
  const row = page.getByRole("row", { name: /payments/ });
  const badge = row.locator(".ks-tip__target", { hasText: "OutOfSync" });
  const text = "OutOfSync: Git differs from the cluster";
  await expect(shown(page)).toHaveCount(0);
  await badge.hover();
  await expect(shown(page)).toHaveText(text);
  await expect(badge).toHaveAccessibleDescription(text);

  await page.mouse.move(0, 0);
  await expect(shown(page)).toHaveCount(0);
  await tabTo(page, badge);
  await expect(shown(page)).toHaveText(text);
});

test("TestRelativeTimeTooltip", async ({ page, request }) => {
  const apps = (await (await request.get("/api/applications")).json()) as {
    name: string;
    lastChange: string;
  }[];
  const catalog = apps.find((a) => a.name === "catalog");
  expect(catalog?.lastChange).toMatch(/^\d{4}-/);
  const want = local(catalog!.lastChange);

  await page.goto("/apps");
  const when = page
    .getByRole("row", { name: /catalog/ })
    .locator("td")
    .last()
    .locator(".ks-tip__target");
  await expect(when).toHaveText(/ago|just now/);
  await when.hover();
  await expect(shown(page)).toContainText(want);
  await page.mouse.move(0, 0);
  await tabTo(page, when);
  await expect(shown(page)).toContainText(want);
  await expect(
    page.getByRole("columnheader", { name: /Last change/ }),
  ).toBeVisible();
});

test("TestTruncatedUsernameTooltip", async ({ page }) => {
  const token = execSync("go run ./hack/console-dev token", { cwd: ".." })
    .toString()
    .trim();
  await page.goto("/login");
  await page.getByLabel("Kubernetes token").fill(token);
  await page
    .getByRole("button", { name: "Sign in with a Kubernetes token" })
    .click();
  await expect(page).toHaveURL(/\/apps$/);
  const name = "system:serviceaccount:default:console-viewer";
  const user = page.locator(".ks-shell__username");
  await expect(user).toHaveText(name);
  await user.hover();
  await expect(shown(page)).toHaveText(name);
  await page.mouse.move(700, 450);
  await expect(shown(page)).toHaveCount(0);
  await tabTo(page, user);
  await expect(shown(page)).toHaveText(name);
  await page.context().clearCookies();
});

test("TestTopBarAndHeaderTooltips", async ({ page }) => {
  await page.goto("/apps");
  for (const [target, text] of [
    [
      page.locator(".az-topbar .ks-tip__target", { hasText: "LIVE" }),
      "Refreshes every 10 seconds",
    ],
    [
      page.locator(".az-topbar .ks-tip__target", { hasText: "Read-only" }),
      "The console never changes the cluster",
    ],
    [
      page.getByRole("button", { name: /Use the (light|dark) theme/ }),
      "Use the",
    ],
    [
      page
        .getByRole("columnheader", { name: "Sync" })
        .locator(".ks-tip__target"),
      "Git",
    ],
    [
      page
        .getByRole("columnheader", { name: "Health" })
        .locator(".ks-tip__target"),
      "running",
    ],
  ] as const) {
    await target.hover();
    await expect(shown(page)).toContainText(text);
    await page.mouse.move(0, 0);
  }
});

for (const theme of ["dark", "light"]) {
  test(`TestConsoleAccessibility ${theme}`, async ({ page }) => {
    await page.addInitScript(
      (t) => localStorage.setItem("ksync.theme", t),
      theme,
    );
    await page.goto("/apps?health=Healthy");
    await expect(page.locator("main table tbody tr").first()).toBeVisible();
    // Show one tooltip so its colours are checked too.
    await page
      .getByRole("row", { name: /catalog/ })
      .locator(".ks-tip__target", { hasText: "Synced" })
      .hover();
    await expect(shown(page)).toBeVisible();
    // The LIVE dot pulses forever; wait out the tooltip's fade instead.
    await page.waitForTimeout(300);
    const results = await new AxeBuilder({ page }).analyze();
    expect(
      results.violations.filter(
        (v) => v.impact === "serious" || v.impact === "critical",
      ),
    ).toEqual([]);
  });
}

test("TestPlanHeaderTooltip", async ({ page }) => {
  await page.goto("/revisions");
  await page
    .getByRole("columnheader", { name: "Plan" })
    .locator(".ks-tip__target")
    .hover();
  await expect(shown(page)).toContainText("+N created");
});

test("TestCopyButtonTooltip", async ({ page }) => {
  await page.goto("/apps/default/payments/diagnosis");
  const copy = page.getByRole("button", { name: "Copy command" });
  await copy.hover();
  await expect(shown(page)).toHaveText("Copy command");
  await expect(copy).toHaveAccessibleDescription("Copy command");
  await expect(copy).not.toHaveAttribute("title");
});
