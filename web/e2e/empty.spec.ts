import { test, expect, type Page } from "@playwright/test";

// hack/console-dev seeds every Kuvryn Sync object in "default"; the
// "finance" namespace exists but holds none of them.
async function inNamespace(page: Page, ns: string) {
  await page.addInitScript(
    (ns) => localStorage.setItem("ksync.namespace", ns),
    ns,
  );
}

const pages: [string, string, string][] = [
  ["/apps", "No applications in this namespace", "Search applications"],
  ["/repositories", "No repositories in this namespace", "Search repositories"],
  ["/revisions", "No revisions in this namespace", "Search revisions"],
  [
    "/imagepolicies",
    "No image policies in this namespace",
    "Search image policies",
  ],
];

for (const [path, title, search] of pages) {
  test(`TestEmptyTableExplainsItself ${path}`, async ({ page }) => {
    await inNamespace(page, "finance");
    await page.goto(path);
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { name: title })).toBeVisible();
    await expect(main.getByRole("link", { name: "the docs" })).toHaveAttribute(
      "href",
      "https://github.com/azrtydxb/kuvryn-sync/tree/main/docs",
    );
    // Nothing to filter: no filter bar, no "0 of 0", no table.
    await expect(main.getByText("0 of 0")).toHaveCount(0);
    await expect(main.getByLabel(search)).toHaveCount(0);
    await expect(main.getByRole("search")).toHaveCount(0);
    await expect(main.locator("table")).toHaveCount(0);
    await expect(main.getByText(/match these filters/)).toHaveCount(0);
  });
}

test("TestEmptyImagePoliciesDescribeImagePolicies", async ({ page }) => {
  await inNamespace(page, "finance");
  await page.goto("/imagepolicies");
  await expect(
    page
      .getByRole("main")
      .getByText(
        "Image policies scan a registry and commit new image tags back to Git.",
      ),
  ).toBeVisible();
});

test("TestFilteredTableStillSaysNoMatch", async ({ page }) => {
  await page.goto("/imagepolicies?q=no-such-policy");
  await expect(
    page.getByRole("heading", {
      name: "No image policies match these filters",
    }),
  ).toBeVisible();
  await expect(page.getByText("0 of 3")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: /in this namespace/ }),
  ).toHaveCount(0);
});

test("TestEmptyResourcesTabExplainsItself", async ({ page }) => {
  await page.route("**/api/applications/default/payments/resources", (r) =>
    r.fulfill({ json: [] }),
  );
  await page.goto("/apps/default/payments/resources");
  const main = page.getByRole("main");
  await expect(
    main.getByRole("heading", { name: "No managed resources" }),
  ).toBeVisible();
  await expect(main.getByText("0 of 0")).toHaveCount(0);
  await expect(main.getByLabel("Search resources")).toHaveCount(0);
  await expect(main.locator("table")).toHaveCount(0);
});
