import { test, expect } from "@playwright/test";

// A filter bar's fields are bottom-aligned, so their labels only share a
// line when every control has the same height.
for (const theme of ["dark", "light"]) {
  test(`TestFilterBarFieldsLineUp ${theme}`, async ({ page }) => {
    await page.addInitScript(
      (t) => localStorage.setItem("ksync.theme", t),
      theme,
    );
    await page.goto("/apps");
    const bar = page.getByRole("search", { name: "Filter applications" });
    await expect(bar).toBeVisible();

    const sync = await bar.getByText("Sync", { exact: true }).boundingBox();
    const health = await bar.getByText("Health", { exact: true }).boundingBox();
    expect(sync && health).toBeTruthy();
    expect(Math.abs(sync!.y - health!.y)).toBeLessThanOrEqual(1);
    expect(Math.abs(sync!.height - health!.height)).toBeLessThanOrEqual(1);

    const controls = [
      bar.getByLabel("Search applications"),
      bar.getByLabel("Sync", { exact: true }),
      bar.getByRole("radiogroup", { name: "Health" }),
    ];
    const boxes = [];
    for (const c of controls) boxes.push(await c.boundingBox());
    const [search, select, seg] = boxes;
    for (const box of [select, seg]) {
      expect(Math.abs(box!.height - search!.height)).toBeLessThanOrEqual(1);
      expect(Math.abs(box!.y - search!.y)).toBeLessThanOrEqual(1);
    }
  });
}

test("TestResourceFilterSwitchLinesUp", async ({ page }) => {
  await page.goto("/apps/default/payments/resources");
  const bar = page.getByRole("search", { name: "Filter resources" });
  await expect(bar).toBeVisible();
  const search = await bar.getByLabel("Search resources").boundingBox();
  const kind = await bar.getByLabel("Kind", { exact: true }).boundingBox();
  const toggle = await bar
    .locator("label", { hasText: "Only not synced or unhealthy" })
    .boundingBox();
  for (const box of [kind, toggle]) {
    expect(Math.abs(box!.height - search!.height)).toBeLessThanOrEqual(1);
    expect(Math.abs(box!.y - search!.y)).toBeLessThanOrEqual(1);
  }
});
