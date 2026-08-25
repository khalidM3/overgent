import { expect, test } from "@playwright/test";

test("Project Workroom shows people, Codex, Claude, and session drill-down", async ({ page }) => {
  await page.goto("/?state=ready");
  await expect(page.getByRole("heading", { name: "Atlas launch" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Now" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Open Codex session for Khalid" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Open Claude Code session for Mina" })).toBeVisible();
  await expect(page.getByLabel("Details inspector")).toContainText("Codex");
  await page.getByRole("button", { name: "Open Claude Code session for Mina" }).click();
  await expect(page.getByLabel("Details inspector")).toContainText("Claude Code");
  await expect(page.getByLabel("Details inspector")).toContainText("Waiting for input");
  await page.getByRole("button", { name: "Open Shared task session for Ravi" }).click();
  await expect(page.getByLabel("Details inspector")).toContainText("1,000 paths");
  await expect(page.getByLabel("Semantic processing status")).toContainText("degraded");
});

test("activity updates and Project switching remain isolated", async ({ page }) => {
  await page.goto("/?state=ready");
  await page.getByRole("button", { name: "Simulate activity" }).click();
  await expect(page.getByText("Published one new path-only manifest revision.")).toBeVisible();
  await expect(page.getByText("revision 185")).toBeVisible();
  await page.getByRole("button", { name: /Orchard mobile/ }).click();
  await expect(page.getByRole("heading", { name: "Orchard mobile" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Collision detected/ })).toHaveCount(0);
  await expect(page.getByLabel("Semantic processing status")).toContainText("disabled");
  await expect(page.getByText("Workspace sharing is paused", { exact: true })).toBeVisible();
});

test("collision lifecycle and settings are accessible", async ({ page }) => {
  await page.goto("/?state=ready");
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByText("Workspace sharing is paused", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Resume" }).click();
  await expect(page.getByText("Workspace sharing is paused", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: /Collision detected Khalid and Mina/ }).click();
  const detail = page.getByLabel("Selected collision detail");
  await expect(detail).toContainText("Codex");
  await expect(detail).toContainText("Claude Code");
  await page.getByRole("button", { name: "Useful" }).click();
  await expect(page.getByText("Feedback recorded")).toBeVisible();
  await page.getByRole("button", { name: "Acknowledge", exact: true }).click();
  await expect(detail).toContainText("acknowledged");
  await page.getByRole("button", { name: "Mark resolved" }).click();
  await expect(detail).toContainText("resolved");

  await page.getByRole("button", { name: "Open Project settings" }).click();
  const dialog = page.getByRole("dialog", { name: "Settings" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Coordination metadata only");
  await expect(page.getByRole("button", { name: "Close settings" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("command palette switches Projects and theme changes", async ({ page }) => {
  await page.goto("/?state=ready");
  await page.getByRole("button", { name: "Search Projects and commands" }).click();
  const command = page.getByRole("dialog", { name: "Search Projects and commands" });
  await expect(command).toBeVisible();
  await command.getByRole("textbox").fill("Orchard");
  await command.getByRole("button", { name: /Orchard mobile/ }).click();
  await expect(page.getByRole("heading", { name: "Orchard mobile" })).toBeVisible();
  await page.getByRole("button", { name: "Switch to dark theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});

test("explicit shell states neither leak nor overstate data", async ({ page }) => {
  await page.goto("/?state=loading");
  await expect(page.getByRole("status")).toContainText("Loading Project coordination");
  await page.goto("/?state=unauthorized");
  await expect(page.getByRole("alert")).toContainText("not authorized");
  await expect(page.getByText("stickguy/atlas")).toHaveCount(0);
  await page.goto("/?state=offline");
  await expect(page.getByText(/Showing revision 184/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Pause" })).toBeDisabled();
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
