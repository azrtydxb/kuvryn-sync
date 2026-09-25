import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { execSync } from "node:child_process";

const HINT = "kubectl create token <serviceaccount> -n <namespace>";

/** Routes /api/me to answer as a console with OIDC configured. */
async function withOIDC(page: Page) {
  await page.route("**/api/me", async (route) => {
    const res = await route.fetch();
    const me = await res.json();
    await route.fulfill({
      response: res,
      json: {
        ...me,
        authenticated: false,
        oidc: true,
        ssoName: "Dex",
        connectors: ["github", "gitlab", "local"],
      },
    });
  });
}

for (const theme of ["dark", "light"]) {
  for (const width of [1400, 700]) {
    test(`TestLoginPage ${theme} ${width}px`, async ({ page }) => {
      // Content-Security-Policy violations surface as console errors.
      const errors: string[] = [];
      page.on("console", (m) => {
        if (m.type() === "error") errors.push(m.text());
      });
      await page.setViewportSize({ width, height: 900 });
      await page.addInitScript(
        (t) => localStorage.setItem("ksync.theme", t),
        theme,
      );
      await page.goto("/login");
      await expect(
        page.getByRole("heading", { name: "Welcome back" }),
      ).toBeVisible();
      // hack/console-dev runs without OIDC: the token form and nothing else.
      const token = page.getByLabel("Kubernetes token");
      await expect(token).toBeVisible();
      await expect(token).toHaveAttribute("type", "password");
      await expect(page.getByText(HINT)).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Sign in with a Kubernetes token" }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: /Dex|GitHub/ }),
      ).toHaveCount(0);
      await expect(page.getByText("GitOps that sticks.")).toBeVisible({
        visible: width >= 900,
      });
      // The emblem must be the real 640x640 artwork, not a truncated file.
      const emblem = page.locator(
        `.az-plogo__em--${theme === "dark" ? "dark" : "light"}`,
      );
      await expect(emblem.first()).toBeVisible();
      const widths = await emblem.evaluateAll((imgs) =>
        imgs.map((img) => (img as HTMLImageElement).naturalWidth),
      );
      for (const w of widths) expect(w).toBeGreaterThanOrEqual(512);
      const results = await new AxeBuilder({ page }).analyze();
      expect(
        results.violations.filter(
          (v) => v.impact === "serious" || v.impact === "critical",
        ),
      ).toEqual([]);
      expect(errors).toEqual([]);
    });
  }
}

test("TestLoginPage shows OIDC buttons only when configured", async ({
  page,
}) => {
  await withOIDC(page);
  await page.goto("/login");
  await expect(
    page.getByRole("button", { name: "Sign in with Dex" }),
  ).toBeVisible();
  for (const name of ["GitHub", "GitLab", "Sign in with email"]) {
    await expect(page.getByRole("button", { name })).toBeVisible();
  }
  // The token form stays next to single sign-on.
  await expect(page.getByLabel("Kubernetes token")).toBeVisible();
  const results = await new AxeBuilder({ page }).analyze();
  expect(
    results.violations.filter(
      (v) => v.impact === "serious" || v.impact === "critical",
    ),
  ).toEqual([]);
});

test("TestLoginPage signs in with a ServiceAccount token", async ({ page }) => {
  const token = execSync("go run ./hack/console-dev token", {
    cwd: "..",
  })
    .toString()
    .trim();
  expect(token.split(".")).toHaveLength(3);
  await page.goto("/login");
  await page.getByLabel("Kubernetes token").fill(token);
  await page
    .getByRole("button", { name: "Sign in with a Kubernetes token" })
    .click();
  await expect(page).toHaveURL(/\/apps$/);
  await expect(page.locator(".ks-shell__username")).toHaveText(
    "system:serviceaccount:default:console-viewer",
  );
  await expect(
    page.getByRole("link", { name: "catalog", exact: true }),
  ).toBeVisible();
  // The token never lands in the URL or the page.
  expect(page.url()).not.toContain(token.slice(-20));
  expect(await page.content()).not.toContain(token.slice(-20));
  // Signing out returns to the stub viewer that console-dev falls back to.
  await page.context().clearCookies();
});

test("TestLoginPage refuses a bad token", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Kubernetes token").fill("not-a-kubernetes-token");
  await page
    .getByRole("button", { name: "Sign in with a Kubernetes token" })
    .click();
  await expect(page).toHaveURL(/\/login\?error=token$/);
  await expect(page.getByText("Sign-in failed")).toBeVisible();
  await expect(page.getByText(/token was not accepted/)).toBeVisible();
  await page.goto("/login?error=cluster");
  await expect(
    page.getByText(/could not reach the Kubernetes API server/),
  ).toBeVisible();
});
