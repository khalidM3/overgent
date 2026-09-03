import { expect, test } from "@playwright/test";

test("Project Workroom shows people, Codex, Claude, and session drill-down", async ({ page }) => {
  await page.goto("/?fixtures=1&state=ready");
  await expect(page.getByRole("heading", { name: "Atlas launch" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Needs you" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Sessions" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Open Codex session for Khalid" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Open Claude Code session for Mina" })).toBeVisible();
  await expect(page.getByLabel("Details inspector")).toContainText("Codex");
  await expect(page.getByLabel("Details inspector")).toContainText("feature/session-rotation");
  await expect(page.getByLabel("Details inspector")).toContainText("Coordination routed");
  await page.getByRole("button", { name: "Open Claude Code session for Mina" }).click();
  await expect(page.getByLabel("Details inspector")).toContainText("Claude Code");
  await expect(page.getByLabel("Details inspector")).toContainText("Waiting for input");
  await expect(page.getByLabel("Details inspector").getByRole("heading", { name: /Subagents/ })).toHaveCount(0);
  await page.getByRole("button", { name: "Open Ravi's work" }).click();
  await page.getByRole("button", { name: "Open session details" }).click();
  await expect(page.getByLabel("Details inspector")).toContainText("1,000 paths");
  await expect(page.getByLabel("Semantic processing status")).toContainText("degraded");
});

test("activity updates and Project switching remain isolated", async ({ page }) => {
  await page.goto("/?fixtures=1&state=ready");
  await page.getByRole("button", { name: "Simulate activity" }).click();
  await expect(page.getByText("rev 185")).toBeVisible();
  await page.getByRole("button", { name: "History" }).click();
  await page.getByText(/recorded events/).click();
  await expect(page.getByText("Published one new path-only manifest revision.")).toBeVisible();
  await page.getByRole("button", { name: /Orchard mobile/ }).click();
  await expect(page.getByRole("heading", { name: "Orchard mobile" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Collision detected/ })).toHaveCount(0);
  await expect(page.getByLabel("Semantic processing status")).toContainText("disabled");
  await expect(page.getByText("Your sharing is paused in this Project", { exact: true })).toBeVisible();
});

test("collision lifecycle and settings are accessible", async ({ page }) => {
  await page.goto("/?fixtures=1&state=ready");
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByText("Your sharing is paused in this Project", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Resume" }).click();
  await expect(page.getByText("Your sharing is paused in this Project", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Collision detected Khalid and Mina" }).click();
  const detail = page.getByLabel("Selected finding detail");
  await expect(detail).toContainText("Codex");
  await expect(detail).toContainText("Claude Code");
  // Three exits, not five: Acknowledge and the standalone feedback row are
  // gone; dismissing carries the reason, and the reason is the feedback.
  await expect(page.getByRole("button", { name: "Acknowledge", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Useful" })).toHaveCount(0);

  // Branch is read straight off the sessions the finding names: divergent
  // branches are the case nothing outside this Project reports.
  await expect(detail).toContainText("until those branches meet at merge");

  // Resolving is recording a decision, and the decision says where it goes
  // before it is written rather than after it has been sent.
  await expect(page.getByRole("button", { name: "Mark resolved" })).toHaveCount(0);
  await expect(detail).toContainText("Goes to Khalid's Codex session and Mina's Claude Code session.");
  await detail.getByLabel(/^Decision for /).fill("Khalid owns the rotation boundary.");
  await detail.getByRole("button", { name: "Send to both sessions" }).click();
  // The loop closes where the decision was made: each affected session with
  // its own delivery state.
  await expect(detail).toContainText("queued for its next turn");
  await expect(detail).toContainText("resolved");

  await page.getByRole("button", { name: "Open Project settings" }).click();
  const settings = page.getByRole("main", { name: "Settings" });
  await expect(settings).toBeVisible();
  await expect(settings).toContainText("Local-first analysis, bounded Project sharing");
  // Settings is a screen, so it replaces the main and inspector columns rather
  // than floating over them. Escape still goes back, as the dialog did.
  await expect(page.getByRole("complementary", { name: "Details inspector" })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(settings).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Atlas launch" })).toBeVisible();
});

test("command palette switches Projects and theme changes", async ({ page }) => {
  await page.goto("/?fixtures=1&state=ready");
  await page.getByRole("button", { name: "Search Projects and commands" }).click();
  const command = page.getByRole("dialog", { name: "Search Projects and commands" });
  await expect(command).toBeVisible();
  await command.getByRole("textbox").fill("Orchard");
  await command.getByRole("button", { name: /Orchard mobile/ }).click();
  await expect(page.getByRole("heading", { name: "Orchard mobile" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Switch to (dark|light) theme/ })).toHaveCount(0);
  await page.getByRole("button", { name: "Open Project settings" }).click();
  await page.getByRole("button", { name: /^Theme/ }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});

test("explicit shell states neither leak nor overstate data", async ({ page }) => {
  await page.goto("/?fixtures=1&state=loading");
  // The fixture banner is also role=status, so target the state card itself.
  await expect(page.getByRole("status").filter({ hasText: "Connecting" })).toContainText("Loading Project coordination");
  await page.goto("/?fixtures=1&state=unauthorized");
  await expect(page.getByRole("alert")).toContainText("not authorized");
  await expect(page.getByText("overgent/atlas")).toHaveCount(0);
  await page.goto("/?fixtures=1&state=offline");
  await expect(page.getByText(/Showing revision 184/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Pause" })).toBeDisabled();
  await page.goto("/?fixtures=1&state=version_mismatch");
  await expect(page.getByRole("alert")).toContainText("Upgrade Overgent");
  await expect(page.getByText("overgent/atlas")).toHaveCount(0);
  await page.goto("/?fixtures=1&state=empty");
  await expect(page.getByRole("heading", { name: /No Projects are available/ })).toBeVisible();
});

test("activation discloses metadata without exposing a ticket input", async ({ page }) => {
  await page.goto("/?fixtures=1&state=activation");
  await expect(page.getByRole("textbox")).toHaveCount(0);
  await expect(page.getByText(/Never source, diffs, prompts, transcripts/)).toBeVisible();
  await page.getByRole("button", { name: "Check for a session" }).click();
  await expect(page.getByRole("heading", { name: "Atlas launch" })).toBeVisible();
});
