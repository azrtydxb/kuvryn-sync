import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
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
      await expect(
        page.getByRole("button", { name: /Sign in with/ }),
      ).toBeVisible();
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
