import { expect, test } from "@playwright/test";

test("live board updates and isolates Projects", async ({ page }) => {
  await page.goto("/?state=ready");
  await expect(page.getByRole("heading", { name: "Atlas launch" })).toBeVisible();
  await expect(page.getByText("Large change · 1,000 paths")).toBeVisible();
  await expect(page.getByLabel("Semantic processing status")).toContainText("degraded");
  await page.getByRole("button", { name: "Publish fixture update" }).click();
  await expect(page.getByText("Published one new path-only manifest revision.")).toBeVisible();
  await expect(page.getByText("revision 185")).toBeVisible();
  await page.getByRole("button", { name: /Orchard mobile/ }).click();
  await expect(page.getByRole("heading", { name: "Orchard mobile" })).toBeVisible();
  await expect(page.getByText("No findings for this Project")).toBeVisible();
  await expect(page.getByText("Session contract is changing in two workstreams")).toHaveCount(0);
  await expect(page.getByLabel("Semantic processing status")).toContainText("disabled");
});

test("pause, finding lifecycle, and devices are accessible entry points", async ({ page }) => {
  await page.goto("/?state=ready");
  await page.getByRole("button", { name: "Pause sharing" }).click();
  await expect(page.getByText("Workspace sharing is paused", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Resume sharing" }).click();
  await expect(page.getByText("Workspace sharing is paused", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Useful" }).click();
  await expect(page.getByText("Feedback recorded")).toBeVisible();
  await page.getByRole("button", { name: "Acknowledge", exact: true }).click();
  await expect(page.getByLabel("Selected finding detail")).toContainText("acknowledged");
  await page.getByRole("button", { name: "Mark resolved" }).click();
  await expect(page.getByLabel("Selected finding detail")).toContainText("resolved");
  await page.getByRole("button", { name: "Open devices and privacy" }).click();
  const dialog = page.getByRole("dialog", { name: "Devices & privacy" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Not collected in V1");
  await expect(page.getByRole("button", { name: "Close devices and privacy" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("explicit shell states neither leak nor overstate data", async ({ page }) => {
  await page.goto("/?state=loading");
  await expect(page.getByRole("status")).toContainText("Loading Project coordination");
  await page.goto("/?state=unauthorized");
  await expect(page.getByRole("alert")).toContainText("not authorized");
  await expect(page.getByText("stickguy/atlas")).toHaveCount(0);
  await page.goto("/?state=offline");
  await expect(page.getByText(/Showing revision 184/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Pause sharing" })).toBeDisabled();
  await page.goto("/?state=version_mismatch");
  await expect(page.getByRole("alert")).toContainText("Upgrade Stickguy");
  await expect(page.getByText("stickguy/atlas")).toHaveCount(0);
  await page.goto("/?state=empty");
  await expect(page.getByRole("heading", { name: /No Projects are available/ })).toBeVisible();
});

test("activation discloses metadata without exposing a ticket input", async ({ page }) => {
  await page.goto("/?state=activation");
  await expect(page.getByRole("textbox")).toHaveCount(0);
  await expect(page.getByText(/Never source, diffs, prompts, transcripts/)).toBeVisible();
  await page.getByRole("button", { name: "Activate secure session" }).click();
  await expect(page.getByRole("heading", { name: "Atlas launch" })).toBeVisible();
});
