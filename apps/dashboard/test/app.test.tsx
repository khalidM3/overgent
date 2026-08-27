import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FixtureProjectSource } from "../src/fixture-source";
import { fixtureSession } from "../src/fixtures";
import { App, DesktopPreviewBanner } from "../src/main";
import type { NativeOnboarding } from "../src/native";

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

  it("separates your own sessions from nearby teammates, with drill-down details", async () => {
    const user = userEvent.setup();
    renderReady();
    // The workroom answers "what is reaching me" before "what is everyone doing".
    expect(screen.getByRole("heading", { name: "Converging on you" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Your sessions" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Nearby" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open Codex session for Khalid" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open Claude Code session for Mina" })).toBeTruthy();

    const inspector = screen.getByLabelText("Details inspector");
    expect(within(inspector).getByRole("heading", { name: "Rotate the browser session boundary" })).toBeTruthy();
    expect(within(inspector).getByText(/Live agent/)).toBeTruthy();
    expect(within(inspector).getByText("feature/session-rotation")).toBeTruthy();
    expect(within(inspector).getByRole("heading", { name: "Activity 3" })).toBeTruthy();
    expect(within(inspector).getByRole("heading", { name: "Subagents 1" })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Open Claude Code session for Mina" }));
    expect(within(inspector).getByRole("heading", { name: "Audit session validity checks" })).toBeTruthy();
    expect(within(inspector).getByText(/Waiting for input/)).toBeTruthy();
    expect(within(inspector).queryByRole("heading", { name: /Subagents/ })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Open Shared task session for Ravi" }));
    expect(within(inspector).getByText(/Git observed/)).toBeTruthy();
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
    expect(screen.getByText("rev 185")).toBeTruthy();

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
    expect(within(dialog).getByText("Local-first analysis, bounded Project sharing")).toBeTruthy();
    expect(await within(dialog).findByRole("heading", { name: "Members" })).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Create one-use invite" })).toBeTruthy();
    expect(within(dialog).getByText("Export retained Project data")).toBeTruthy();
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

  it("closes the command palette from its esc control and backdrop", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Search Projects and commands" }));
    await user.click(screen.getByRole("button", { name: "Close command palette" }));
    expect(screen.queryByRole("dialog", { name: "Search Projects and commands" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Search Projects and commands" }));
    await user.click(screen.getByRole("dialog", { name: "Search Projects and commands" }));
    expect(screen.queryByRole("dialog", { name: "Search Projects and commands" })).toBeNull();
  });

  it("restores the Projects sidebar after it is collapsed", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Collapse Projects sidebar" }));
    const restore = screen.getByRole("button", { name: "Expand Projects sidebar" });
    expect(restore).toBeTruthy();
    await user.click(restore);
    expect(screen.getByRole("button", { name: "Collapse Projects sidebar" })).toBeTruthy();
  });

  it("creates a new Project through the dedicated native dialog", async () => {
    const user = userEvent.setup();
    const navigate = vi.fn();
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ available: true, development: true, enrolled: true, projectId: "prj_atlas", repositoryRoot: "/tmp/atlas", repositoryLabel: "atlas", deviceLabel: "Khalid’s Mac", apiBaseUrl: "http://127.0.0.1:3211", adapters: [], limitation: "" })),
      chooseRepository: vi.fn(async () => "/tmp/orbit"),
      createProject: vi.fn(),
      createAdditionalProject: vi.fn(async () => ({ projectId: "prj_orbit", joinCode: "inv_orbit.secret", warnings: [] })),
      joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(),
      openLiveProject: vi.fn(async () => "http://127.0.0.1:49152/activate/orbit"), sessionDetail: vi.fn(),
    };
    render(<App initialState="ready" source={new FixtureProjectSource()} nativeApi={api} navigate={navigate} />);
    await user.click(screen.getByRole("button", { name: "Add a new Project" }));
    const dialog = screen.getByRole("dialog", { name: "Add a new Project" });
    await user.click(within(dialog).getByRole("button", { name: "Choose…" }));
    expect((within(dialog).getByPlaceholderText("Choose a local Git repository") as HTMLInputElement).value).toBe("/tmp/orbit");
    await user.click(within(dialog).getByRole("button", { name: "Create Project" }));
    expect(await screen.findByRole("heading", { name: "Project created" })).toBeTruthy();
    expect(screen.getByText("inv_orbit.secret")).toBeTruthy();
    expect(api.createAdditionalProject).toHaveBeenCalledWith(expect.objectContaining({ repositoryRoot: "/tmp/orbit", projectLabel: "orbit", displayName: "Khalid" }));
    await user.click(screen.getByRole("button", { name: "Open Project" }));
    expect(navigate).toHaveBeenCalledWith("http://127.0.0.1:49152/activate/orbit");
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

describe("member identity", () => {
  const deviceNamedSession = { ...fixtureSession, memberName: "khalids-macbook-pro.local", memberNameSource: "device" as const };

  it("asks a Project still using the device name to choose one, and never renames silently", async () => {
    const user = userEvent.setup();
    const source = new FixtureProjectSource();
    render(<App initialState="ready" initialSession={deviceNamedSession} source={source} />);

    const prompt = screen.getByText("Choose how teammates see you").closest(".notice") as HTMLElement;
    expect(prompt).toBeTruthy();
    // The legacy name is shown as-is; migration is a prompt, never a rewrite.
    expect(within(prompt).getByText(/khalids-macbook-pro\.local/)).toBeTruthy();
    expect(within(prompt).getByText(/device name stays in Settings/)).toBeTruthy();

    await user.click(within(prompt).getByRole("button", { name: "Choose a name" }));
    const dialog = screen.getByRole("dialog", { name: "Settings" });
    const field = within(dialog).getByLabelText("Display name");
    expect((field as HTMLInputElement).value).toBe("");
    expect((field as HTMLInputElement).placeholder).toBe("khalids-macbook-pro.local");

    await user.type(field, "Khalid M");
    await user.click(within(dialog).getByRole("button", { name: "Save name" }));
    expect(await within(dialog).findByText("Display name updated across this Project.")).toBeTruthy();
  });

  it("rejects an email address as live-work identity", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" initialSession={deviceNamedSession} source={new FixtureProjectSource()} />);
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const dialog = screen.getByRole("dialog", { name: "Settings" });
    await user.type(within(dialog).getByLabelText("Display name"), "khalid@example.com");
    await user.click(within(dialog).getByRole("button", { name: "Save name" }));
    expect((await within(dialog).findByRole("alert")).textContent).toContain("email address cannot be your Project identity");
  });

  it("keeps device names under an explicit security heading rather than as identity", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const dialog = screen.getByRole("dialog", { name: "Settings" });
    expect(within(dialog).getByRole("heading", { name: "Devices & security" })).toBeTruthy();
    expect(within(dialog).getByText(/never shown as your live-work identity/)).toBeTruthy();
    expect(screen.queryByText("Choose how teammates see you")).toBeNull();
  });
});

describe("session content", () => {
  it("always shows your own session locally, before and without sharing it", async () => {
    renderReady();
    const inspector = screen.getByLabelText("Details inspector");
    // The owner sees prompts, replies, and reasoning locally while the safe
    // projection is automatically available to Project members.
    expect(await within(inspector).findByText(/classifier-passing messages are visible/)).toBeTruthy();
    expect(within(inspector).getByText(/Rotate the browser session on every permission change/)).toBeTruthy();
    expect(within(inspector).getByText(/The rotation boundary lives in session.ts/)).toBeTruthy();
    expect(within(inspector).getAllByText("Thinking").length).toBeGreaterThan(0);
    expect(within(inspector).getAllByText("Assistant").length).toBeGreaterThan(0);
    // Tool steps appear as names only, never as inputs or results.
    const conversation = inspector.querySelector(".conversation-list") as HTMLElement;
    expect(within(conversation).getAllByText("apply_patch").length).toBe(1);
    expect(conversation.textContent).not.toContain("file_path");
  });

  it("shows a teammate's Project-visible session without an extra ceremony", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Open Claude Code session for Mina" }));
    const inspector = screen.getByLabelText("Details inspector");
    expect(await within(inspector).findByText(/Waiting for the first classifier-passing message/)).toBeTruthy();
  });
});
