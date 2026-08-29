import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FixtureProjectSource } from "../src/fixture-source";
import { fixtureSession, snapshotForProject } from "../src/fixtures";
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
    expect(within(inspector).getByText("Working now")).toBeTruthy();
    expect(within(inspector).getAllByText("feature/session-rotation")).toHaveLength(1);
    expect(within(inspector).queryByRole("tab")).toBeNull();
    expect(await within(inspector).findByText(/Rotate the browser session on every permission change/)).toBeTruthy();
    // Lifecycle and Stickguy coordination live in the same chronology as chat.
    const started = within(inspector).getByText("Session started").closest("li") as HTMLElement;
    const routed = within(inspector).getByText("Coordination routed").closest("li") as HTMLElement;
    const considered = within(inspector).getByText("Agent considered coordination").closest("li") as HTMLElement;
    expect(started.compareDocumentPosition(routed) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(routed.compareDocumentPosition(considered) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(routed.classList.contains("converging")).toBe(true);
    expect(routed.textContent).toContain("Mina is reviewing the same session boundary");
    // Parallel work belongs to the session stream, not separate navigation.
    const parallel = inspector.querySelector(".thread-parallel") as HTMLElement;
    expect(parallel.textContent).toContain("Reviewer working in parallel");
    expect(parallel.textContent).toContain("Active now");
    expect(parallel.textContent).not.toContain("sub-a1b2c3");
    // Debug/provenance facts are opened from the header and files stay last.
    expect(within(inspector).queryByLabelText("Session details")).toBeNull();
    await user.click(within(inspector).getByRole("button", { name: "Open session details" }));
    const sessionInfo = within(inspector).getByLabelText("Session details");
    const coverage = within(inspector).getByRole("heading", { name: "How this session is connected" });
    const files = within(inspector).getByRole("heading", { name: "Files this session 1" });
    expect(within(sessionInfo).getByText("codex-a1b2c3")).toBeTruthy();
    expect(within(sessionInfo).getByText("sub-a1b2c3")).toBeTruthy();
    expect(coverage.compareDocumentPosition(files) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Open Claude Code session for Mina" }));
    expect(within(inspector).getByRole("heading", { name: "Audit session validity checks" })).toBeTruthy();
    expect(within(inspector).getAllByText(/Waiting for input/).length).toBeGreaterThan(0);
    expect(within(inspector).queryByText(/sub-a1b2c3/)).toBeNull();
    expect(within(inspector).queryByLabelText("Session details")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Open Shared task session for Ravi" }));
    await user.click(within(inspector).getByRole("button", { name: "Open session details" }));
    expect(within(inspector).getByText(/Git observed/)).toBeTruthy();
    expect(within(inspector).getAllByText(/1,000/).length).toBeGreaterThan(0);
  });

  it("ends completed sessions in the chronology instead of pinning a finished activity strip", async () => {
    const snapshot = snapshotForProject("prj_atlas");
    const session = snapshot.workstreams.find((stream) => stream.id === "wrk_agent_fixture_codex");
    if (!session?.agent) throw new Error("Codex fixture session is missing");
    session.presence = "offline";
    session.updatedLabel = "2 min";
    session.agent.status = "done";
    session.agent.endedAt = "2026-08-25T09:59:45Z";
    session.agent.activity = [{ id: "codex-end", at: "2 min", occurredAt: session.agent.endedAt, kind: "SessionEnd", status: "done", action: "Session ended", paths: [] }, ...(session.agent.activity ?? [])];
    render(<App initialState="ready" initialSession={fixtureSession} source={new FixtureProjectSource([snapshot])} />);

    const inspector = screen.getByLabelText("Details inspector");
    expect(within(inspector).getByText("Complete")).toBeTruthy();
    expect(within(inspector).getByText("Session ended")).toBeTruthy();
    expect(within(inspector).queryByLabelText("Current session activity")).toBeNull();
    expect(within(inspector).queryByText("Session finished")).toBeNull();
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
    // Activity is a record of what already happened, so it lives with Decisions
    // rather than competing with live work on the Workroom.
    await user.click(screen.getByRole("button", { name: /Decisions/ }));
    expect(screen.getByText("Published one new path-only manifest revision.")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /Workroom/ }));
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
    // Settings is a screen, not a modal: it takes over the main and inspector
    // columns and the sidebar stays put.
    const settings = screen.getByRole("main", { name: "Settings" });
    expect(within(settings).getByText("Local-first analysis, bounded Project sharing")).toBeTruthy();
    expect(await within(settings).findByText("Export retained Project data")).toBeTruthy();
    expect(screen.queryByRole("complementary", { name: "Details inspector" })).toBeNull();
    await user.click(within(settings).getByRole("button", { name: /Theme/ }));
    expect(document.documentElement.dataset.theme).toBe("dark");
    // Back returns to the Project it was opened from.
    await user.click(within(settings).getByRole("button", { name: "Back to Atlas launch" }));
    expect(screen.getByRole("heading", { name: "Atlas launch" })).toBeTruthy();
  });

  it("reaches members and invites from the workroom, not only from settings", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Invite people to this Project" }));
    const people = screen.getByRole("main", { name: "People" });
    expect(await within(people).findByRole("heading", { name: /Members/ })).toBeTruthy();
    expect(within(people).getByRole("button", { name: "Create one-use invite" })).toBeTruthy();

    // Settings still reaches the same surface rather than carrying a second copy.
    await user.click(within(people).getByRole("button", { name: "Back to Atlas launch" }));
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const settings = screen.getByRole("main", { name: "Settings" });
    expect(within(settings).queryByRole("button", { name: "Create one-use invite" })).toBeNull();
    await user.click(await within(settings).findByRole("button", { name: /Members & invites/ }));
    // Reached from Settings, back goes to Settings rather than always the workroom.
    const nested = screen.getByRole("main", { name: "People" });
    await user.click(within(nested).getByRole("button", { name: "Back to Settings" }));
    expect(screen.getByRole("main", { name: "Settings" })).toBeTruthy();
  });

  it("leaves a deleted Project instead of stranding the member inside it", async () => {
    const user = userEvent.setup();
    renderReady();
    expect(screen.getByRole("heading", { name: "Atlas launch" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const settings = screen.getByRole("main", { name: "Settings" });
    await user.type(await within(settings).findByLabelText("Type Atlas launch to confirm"), "Atlas launch");
    await user.click(within(settings).getByRole("button", { name: "Delete Project" }));
    // The shell moves to the remaining Project rather than rendering a Project
    // the member no longer belongs to.
    expect(await screen.findByRole("heading", { name: "Orchard mobile" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Atlas launch" })).toBeNull();
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

  it("never shows a create form it cannot submit while the native bridge is being probed", async () => {
    const user = userEvent.setup();
    let rejectState: (reason: Error) => void = () => undefined;
    const api: NativeOnboarding = {
      state: vi.fn(() => new Promise<never>((_resolve, reject) => { rejectState = reject; })),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(),
      openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(),
    };
    render(<App initialState="ready" source={new FixtureProjectSource()} nativeApi={api} navigate={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Add a new Project" }));
    // Probing. The form used to render here and then vanish under the member.
    const add = screen.getByRole("main", { name: "Add a Project" });
    expect(within(add).queryByPlaceholderText("Choose a local Git repository")).toBeNull();
    expect(within(add).getByRole("status").textContent).toContain("Checking this Mac");

    rejectState(new Error("The native Stickguy bridge is unavailable."));
    // Hosted origin: hand off to the app, phrased as the task continuing rather
    // than as reopening the app the member is already looking at.
    const handoff = await screen.findByRole("button", { name: "Continue in the Stickguy app" });
    expect(handoff).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Open Stickguy/ })).toBeNull();
    expect(screen.queryByPlaceholderText("Choose a local Git repository")).toBeNull();
  });

  it("creates a new Project on its own screen, without bouncing back to the app", async () => {
    const user = userEvent.setup();
    const navigate = vi.fn();
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ available: true, development: true, enrolled: true, projectId: "prj_atlas", repositoryRoot: "/tmp/atlas", repositoryLabel: "atlas", deviceLabel: "Khalid’s Mac", apiBaseUrl: "http://127.0.0.1:3211", adapters: [], limitation: "" })),
      chooseRepository: vi.fn(async () => "/tmp/orbit"),
      createProject: vi.fn(),
      createAdditionalProject: vi.fn(async () => ({ projectId: "prj_orbit", joinCode: "inv_orbit.secret", warnings: [] })),
      joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(),
      openLiveProject: vi.fn(async () => "http://127.0.0.1:49152/activate/orbit"), resetEnrollment: vi.fn(), sessionDetail: vi.fn(),
    };
    render(<App initialState="ready" source={new FixtureProjectSource()} nativeApi={api} navigate={navigate} />);
    await user.click(screen.getByRole("button", { name: "Add a new Project" }));
    const add = await screen.findByRole("main", { name: "Add a Project" });
    // With the native bridge reachable the form is the screen. Nothing offers to
    // open the app the member is already looking at.
    expect(within(add).queryByRole("button", { name: /Continue in the Stickguy app/ })).toBeNull();
    await user.click(within(add).getByRole("button", { name: "Choose…" }));
    expect((within(add).getByPlaceholderText("Choose a local Git repository") as HTMLInputElement).value).toBe("/tmp/orbit");
    await user.click(within(add).getByRole("button", { name: "Create Project" }));
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
    const dialog = screen.getByRole("main", { name: "Settings" });
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
    const dialog = screen.getByRole("main", { name: "Settings" });
    await user.type(within(dialog).getByLabelText("Display name"), "khalid@example.com");
    await user.click(within(dialog).getByRole("button", { name: "Save name" }));
    expect((await within(dialog).findByRole("alert")).textContent).toContain("email address cannot be your Project identity");
  });

  it("keeps device names under an explicit security heading rather than as identity", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const dialog = screen.getByRole("main", { name: "Settings" });
    expect(within(dialog).getByRole("heading", { name: "Devices & security" })).toBeTruthy();
    expect(within(dialog).getByText(/never shown as your live-work identity/)).toBeTruthy();
    expect(screen.queryByText("Choose how teammates see you")).toBeNull();
  });
});

describe("session content", () => {
  it("always shows your own session locally, before and without sharing it", async () => {
    const user = userEvent.setup();
    renderReady();
    const inspector = screen.getByLabelText("Details inspector");
    // The owner sees prompts, replies, and reasoning locally while the safe
    // projection is automatically available to Project members.
    await user.click(within(inspector).getByRole("button", { name: "Open session details" }));
    expect(await within(inspector).findByText(/classifier-passing messages are visible/i)).toBeTruthy();
    expect(within(inspector).getByText(/Rotate the browser session on every permission change/)).toBeTruthy();
    expect(within(inspector).getByText(/The rotation boundary lives in session.ts/)).toBeTruthy();
    expect(within(inspector).getAllByText("Thinking").length).toBeGreaterThan(0);
    expect(within(inspector).getAllByText("Codex").length).toBeGreaterThan(0);
    // Assistant Markdown is semantic UI, not raw punctuation.
    const utilityClasses = within(inspector).getByText("bg-black text-white");
    expect(utilityClasses.tagName).toBe("CODE");
    expect(within(inspector).getByText("Removed the conflicting body classes.").tagName).toBe("LI");
    // Consecutive tool steps coalesce without hiding their names or exposing
    // inputs and results.
    const conversation = inspector.querySelector(".session-thread") as HTMLElement;
    const toolRuns = conversation.querySelectorAll(".thread-tool");
    expect(toolRuns).toHaveLength(1);
    expect(toolRuns[0]!.textContent).toContain("Read → apply_patch");
    expect(toolRuns[0]!.textContent).toContain("2 tool actions");
    expect(conversation.textContent).not.toContain("file_path");
  });

  it("shows a teammate's Project-visible session without an extra ceremony", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Open Claude Code session for Mina" }));
    const inspector = screen.getByLabelText("Details inspector");
    expect(await within(inspector).findByText("Waiting for approval to continue")).toBeTruthy();
    expect(within(inspector).getByText(/Read · apps\/dashboard\/src\/session.ts/)).toBeTruthy();
  });
});

describe("browser activation recovery", () => {
  it("tells a browser with no session that only the Stickguy app can issue a ticket", async () => {
    const user = userEvent.setup();
    const unauthorized = Object.assign(new Error("dashboard API 401"), { status: 401 });
    const loadSession = vi.fn().mockRejectedValue(unauthorized);
    vi.doMock("../src/live-source", async () => ({
      ...(await vi.importActual<typeof import("../src/live-source")>("../src/live-source")),
      loadSession,
    }));
    vi.resetModules();
    const { LiveApp } = await import("../src/main");

    render(<LiveApp />);
    const first = await screen.findByRole("button", { name: "Activate secure session" });
    expect(screen.queryByRole("alert")).toBeNull();

    await user.click(first);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("still has no active session");
    expect(alert.textContent).toContain("Stickguy Dev.app");
    expect(await screen.findByRole("button", { name: "Check again" })).toBeTruthy();
    expect(loadSession.mock.calls.length).toBeGreaterThanOrEqual(2);
    vi.doUnmock("../src/live-source");
  });
});

// A Codex session's reads are not observed, so it can never receive a
// stale-assumption finding. The inspector has to say that, or an operator reads
// silence as safety (ADR-052).
describe("read coverage disclosure", () => {
  it("tells the operator when a session is never told about contract drift", async () => {
    const user = userEvent.setup();
    renderReady();
    const inspector = screen.getByLabelText("Details inspector");
    await user.click(within(inspector).getByRole("button", { name: "Open session details" }));
    const sessionInfo = within(inspector).getByLabelText("Session details");
    // The default selection is the Codex session, whose reads nothing observes.
    expect(within(sessionInfo).getByText("Contract drift")).toBeTruthy();
    expect(within(sessionInfo).getByText("Not observed")).toBeTruthy();
    expect(within(sessionInfo).getByText(/Silence here is missing evidence, not an all-clear/)).toBeTruthy();
  });

  it("says an observed session is told when a contract it read moves", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Open Claude Code session for Mina" }));
    const inspector = screen.getByLabelText("Details inspector");
    await user.click(within(inspector).getByRole("button", { name: "Open session details" }));
    const sessionInfo = within(inspector).getByLabelText("Session details");
    expect(within(sessionInfo).getByText("Observed")).toBeTruthy();
    expect(within(sessionInfo).getByText(/told when a contract it read changes underneath it/)).toBeTruthy();
  });
});
