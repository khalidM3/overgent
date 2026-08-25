import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FixtureProjectSource } from "../src/fixture-source";
import { App, DesktopPreviewBanner } from "../src/main";

const renderReady = () => render(<App initialState="ready" source={new FixtureProjectSource()} />);

describe("Project Workroom behavior", () => {
  it("labels the native desktop fixture boundary", () => {
    render(<DesktopPreviewBanner />);
    expect(screen.getByRole("status").textContent).toContain("Fixture data");
    expect(screen.getByRole("status").textContent).toContain("menu bar");
  });

  it("omits all Project data in the unauthorized state", () => {
    render(<App initialState="unauthorized" source={new FixtureProjectSource()} />);
    expect(screen.getByRole("alert").textContent).toContain("not authorized");
    expect(screen.queryByText("Atlas launch")).toBeNull();
    expect(screen.queryByText("stickguy/atlas")).toBeNull();
  });

  it("centers people and live agent sessions with drill-down details", async () => {
    const user = userEvent.setup();
    renderReady();
    expect(screen.getByRole("heading", { name: "Now" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open Codex session for Khalid" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open Claude Code session for Mina" })).toBeTruthy();
    const inspector = screen.getByLabelText("Details inspector");
    expect(within(inspector).getByRole("heading", { name: "Codex" })).toBeTruthy();
    expect(within(inspector).getByText("Live agent")).toBeTruthy();
    expect(within(inspector).getByRole("heading", { name: "Subagents 1" })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Open Claude Code session for Mina" }));
    expect(within(inspector).getByRole("heading", { name: "Claude Code" })).toBeTruthy();
    expect(within(inspector).getByText("Waiting for input")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Open Shared task session for Ravi" }));
    expect(within(inspector).getByText("Git observed")).toBeTruthy();
    expect(within(inspector).getByText("1,000 paths")).toBeTruthy();
  });

  it("keeps Project data isolated when switching", async () => {
    const user = userEvent.setup();
    renderReady();
    expect(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /Orchard mobile/ }));
    expect(screen.getByRole("heading", { name: "Orchard mobile" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Collision detected/ })).toBeNull();
    expect(screen.getByLabelText("Semantic processing status").textContent).toContain("disabled");
    expect(screen.getByText("Workspace sharing is paused")).toBeTruthy();
  });

  it("applies pause, activity, and collision lifecycle changes immediately", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Pause" }));
    expect(screen.getByText("Workspace sharing is paused")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Resume" }));
    expect(screen.queryByText("Workspace sharing is paused")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Simulate activity" }));
    expect(screen.getByText("Published one new path-only manifest revision.")).toBeTruthy();
    expect(screen.getByText("revision 185")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    const detail = screen.getByLabelText("Selected collision detail");
    expect(detail.textContent).toContain("Advisory only");
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));
    expect(detail.textContent).toContain("acknowledged");
    await user.click(screen.getByRole("button", { name: "Mark resolved" }));
    expect(detail.textContent).toContain("resolved");
  });

  it("records collision feedback and exposes settings and theme controls", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    await user.click(screen.getByRole("button", { name: "Useful" }));
    expect(await screen.findByText("Feedback recorded")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const dialog = screen.getByRole("dialog", { name: "Settings" });
    expect(within(dialog).getByText("Coordination metadata only")).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: /Theme/ }));
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("switches Projects through the command palette", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Search Projects and commands" }));
    const dialog = screen.getByRole("dialog", { name: "Search Projects and commands" });
    await user.type(within(dialog).getByRole("textbox"), "Orchard");
    await user.click(within(dialog).getByRole("button", { name: /Orchard mobile/ }));
    expect(screen.getByRole("heading", { name: "Orchard mobile" })).toBeTruthy();
  });

  it("activates a browser session without asking for a ticket value", async () => {
    const user = userEvent.setup();
    render(<App initialState="activation" source={new FixtureProjectSource()} />);
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.getByText(/never stored in this page/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Activate secure session" }));
    expect(screen.getByRole("heading", { name: "Atlas launch" })).toBeTruthy();
  });
});
