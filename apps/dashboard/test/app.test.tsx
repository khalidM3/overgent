import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FixtureProjectSource } from "../src/fixture-source";
import { fixtureSession, snapshotForProject } from "../src/fixtures";
import { App, DesktopPreviewBanner, groupByBranch } from "../src/main";
import type { NativeOnboarding } from "../src/native";
import type { Workstream } from "../src/model";

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
    expect(screen.getByRole("heading", { name: "Needs you" })).toBeTruthy();
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
    expect(within(inspector).getByRole("heading", { name: "Audit session validity checks before changing the rotation boundary." })).toBeTruthy();
    expect(within(inspector).getAllByText(/Waiting for input/).length).toBeGreaterThan(0);
    expect(within(inspector).queryByText(/sub-a1b2c3/)).toBeNull();
    expect(within(inspector).queryByLabelText("Session details")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Open Ravi's work" }));
    await user.click(within(inspector).getByRole("button", { name: "Open session details" }));
    expect(within(inspector).getByText(/Git observed/)).toBeTruthy();
    expect(within(inspector).getAllByText(/1,000/).length).toBeGreaterThan(0);
  });

  it("keeps rows chronological and renders the six revisioned facts with honest Codex quality", async () => {
    const user = userEvent.setup();
    renderReady();

    const codexRow = screen.getByRole("button", { name: "Open Codex session for Khalid" });
    expect(codexRow.textContent).toContain("Rotate the browser session boundary");
    expect(codexRow.textContent).toContain("Edited apps/dashboard/src/session.ts · 1 parallel agent active");
    expect(codexRow.textContent).toContain("Goal low evidence");
    expect(codexRow.textContent).toContain("Now medium evidence");
    expect(codexRow.textContent).not.toContain("Verification");
    expect(codexRow.textContent).not.toMatch(/\d+%/);

    const inspector = screen.getByLabelText("Details inspector");
    expect(within(inspector).getByText("Goal")).toBeTruthy();
    expect(within(inspector).getByText("Now")).toBeTruthy();
    const codexInspectorScope = within(inspector).getByRole("group", { name: "Scope snapshot revision 8" });
    for (const label of ["Done", "Waiting on", "Scope", "Verification"]) expect(within(codexInspectorScope).getByText(label)).toBeTruthy();
    expect(inspector.textContent).toContain("fallback · low evidence · derived title");
    expect(inspector.textContent).toContain("scope r8");
    expect(codexInspectorScope.textContent).toContain("unavailable · none evidence");

    await user.click(screen.getByRole("button", { name: "Open Claude Code session for Mina" }));
    const claudeInspectorScope = within(inspector).getByRole("group", { name: "Scope snapshot revision 11" });
    expect(inspector.textContent).toContain("declared · high evidence · intended outcome");
    expect(claudeInspectorScope.textContent).toContain("observed · high evidence · current action");
    expect(claudeInspectorScope.textContent).not.toContain("observed · medium evidence");
  });

  it("does not silently continue an active Codex session from the labelled inspector action", async () => {
    const user = userEvent.setup();
    const openOwningSession = vi.fn(async () => ({ vendor: "codex" as const, opened: false, detail: "Codex could not be started. Copy the exact continuation command instead.", fallbackCommand: "codex continue fixture-id" }));
    const api = { openOwningSession } as unknown as NativeOnboarding;
    render(<App initialState="ready" source={new FixtureProjectSource()} nativeApi={api} />);

    const inspector = screen.getByLabelText("Details inspector");
    await user.click(await within(inspector).findByRole("button", { name: "Continue in Codex" }));
    expect(within(inspector).getByText(/still reported active/)).toBeTruthy();
    expect(openOwningSession).not.toHaveBeenCalled();

    await user.click(within(inspector).getByRole("button", { name: "Continue exact session" }));
    expect(openOwningSession).toHaveBeenCalledWith(expect.stringMatching(/^wrk_agent_/), expect.stringContaining("Stickguy found:"), "vendor");
    expect(await within(inspector).findByText(/Copy the exact continuation command/)).toBeTruthy();
    expect(within(inspector).getByRole("button", { name: "Copy command" })).toBeTruthy();
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
    const user = userEvent.setup();
    render(<App initialState="ready" initialSession={fixtureSession} source={new FixtureProjectSource([snapshot])} />);
    // A finished session is never the default selection while another of the
    // member's own sessions is still running, so open the one under test.
    await user.click(screen.getByRole("button", { name: "Open Codex session for Khalid" }));

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
    expect(screen.getByText("Your sharing is paused in this Project")).toBeTruthy();
  });

  it("applies pause, activity, and collision lifecycle changes immediately", async () => {
    const user = userEvent.setup();
    renderReady();
    // The control appears once the page knows it can actually reach the local
    // service, so it is awaited rather than assumed present on first paint.
    await user.click(await screen.findByRole("button", { name: "Pause" }));
    // Pausing only ever stops this device publishing, so the notice says whose
    // sharing stopped rather than implying the Project went quiet.
    expect(screen.getByText("Your sharing is paused in this Project")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Resume" }));
    expect(screen.queryByText("Your sharing is paused in this Project")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Simulate activity" }));
    // Activity is a record of what already happened, so it lives in History
    // rather than competing with live work on the Workroom, and it is folded
    // away there because it is the least scannable thing on that screen.
    await user.click(screen.getByRole("button", { name: /History/ }));
    await user.click(screen.getByText(/recorded events/));
    expect(screen.getByText("Published one new path-only manifest revision.")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /Workroom/ }));
    expect(screen.getByText("rev 185")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    const detail = screen.getByLabelText("Selected collision detail");
    expect(detail.textContent).toContain("Advisory only");
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));
    expect(detail.textContent).toContain("acknowledged");
    // Resolving is recording a decision, so there is no second control that
    // claims the word without routing anything. What is left beside
    // Acknowledge is the way to stop reading without deciding.
    expect(screen.queryByRole("button", { name: "Mark resolved" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(detail.textContent).toContain("dismissed");
  });

  it("leads the collision thread with the decision and names where it will go", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    const detail = screen.getByLabelText("Selected collision detail");

    // Divergent branches are the case nothing else reports, and the inspector
    // says so from data the snapshot already carried.
    expect(detail.textContent).toContain("Khalid on feature/session-rotation");
    expect(detail.textContent).toContain("Mina on main");
    expect(detail.textContent).toContain("until those branches meet at merge");

    // The consequence is stated before the decision is written, not after it
    // has already been sent. The Atlas fixture already carries an open card for
    // this finding, so the decision form is the control on screen.
    expect(detail.textContent).toContain("Goes to Khalid's Codex session and Mina's Claude Code session.");

    await user.type(screen.getByLabelText(/^Decision for /), "Khalid owns the rotation boundary; Mina reviews after it lands.");
    await user.click(screen.getByRole("button", { name: "Record decision" }));
    expect(detail.textContent).toContain("Khalid owns the rotation boundary");
    expect(detail.textContent).toContain("Delivered to 2 sessions");
    // Recording the decision is what resolves the finding.
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
      openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
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
      openLiveProject: vi.fn(async () => "http://127.0.0.1:49152/activate/orbit"), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
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
    await user.click(screen.getByRole("button", { name: "Check for a session" }));
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
    expect((await within(inspector).findAllByText("Waiting for approval to continue")).length).toBeGreaterThan(0);
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
    // The recovery is on the page before the control, not behind a press that
    // cannot succeed: this page can never mint a ticket itself.
    expect(await screen.findByText(/Open the Project from the Stickguy app/)).toBeTruthy();
    expect(screen.getByText(/stickguy dashboard --project/)).toBeTruthy();
    const first = await screen.findByRole("button", { name: "Check for a session" });
    expect(screen.queryByRole("alert")).toBeNull();

    await user.click(first);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("still has no active session");
    expect(alert.textContent).toContain("Stickguy Dev.app");
    // The control keeps its name: pressing it confirmed the state rather than
    // revealing an instruction that should have been visible all along.
    expect(await screen.findByRole("button", { name: "Check for a session" })).toBeTruthy();
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

describe("agent health and the coordination ledger", () => {
  it("puts a stopped session of your own in the same block as a collision", () => {
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    const lead = screen.getByRole("heading", { name: "Needs you" }).parentElement as HTMLElement;
    // Two things need the member: the session collision and their own quiet agent.
    expect(within(lead).getByText("2")).toBeTruthy();
    expect(screen.getByRole("heading", { name: /Regenerate protocol types has gone quiet/ })).toBeTruthy();
    expect(screen.getByText(/still open but has reported nothing/)).toBeTruthy();
  });

  it("states silence as a measurement and refuses to call it a fault", () => {
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    const block = screen.getByLabelText(/Regenerate protocol types has gone quiet/);
    const meta = block.querySelector(".converge-meta") as HTMLElement;
    // One line carries the measurement and its limit; "last reported" used to
    // repeat the fact already shown on the session line above it.
    expect(meta.textContent).toContain("silent for 21m 00s");
    expect(meta.textContent).toContain("observed silence, not a diagnosis");
  });

  it("records what was raised, where it was delivered, and what was settled", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    await user.click(screen.getByRole("button", { name: "History" }));

    expect(screen.getByRole("heading", { name: "Raised" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Settled" })).toBeTruthy();
    expect(screen.getByText(/Acknowledgement records that the agent read the correction, not that it followed it/)).toBeTruthy();
    const deliveries = screen.getByRole("heading", { name: "Delivered into a turn" }).parentElement!.nextElementSibling as HTMLElement;
    expect(within(deliveries).getByText(/Mina is reviewing the same session boundary/)).toBeTruthy();
    // Two of the three fixture deliveries are acknowledged, so this asserts the
    // acknowledged state is rendered rather than that exactly one row carries it.
    expect(within(deliveries).getAllByText("acknowledged").length).toBe(2);
    expect(within(deliveries).getByText("not yet acknowledged")).toBeTruthy();
    expect(screen.getByText(/2 of 3 delivered briefs were acknowledged/)).toBeTruthy();
  });

  it("opens a finding from History without leaving the screen", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    await user.click(screen.getByRole("button", { name: "History" }));
    await user.click(screen.getByRole("button", { name: /Collision detected/ }));

    const inspector = screen.getByLabelText("Details inspector");
    expect(within(inspector).getByText(/Two live agent sessions report the same dashboard session path/)).toBeTruthy();
  });
});

describe("a Project of one", () => {
  it("describes a solo Project as finished rather than as missing teammates", () => {
    render(<App initialState="ready" source={new FixtureProjectSource([soloSnapshot()])} />);
    expect(screen.getByText(/You are the only member/)).toBeTruthy();
    expect(screen.getByText(/coordinates your own parallel sessions the same way it coordinates a team/)).toBeTruthy();
    expect(screen.queryByText(/No teammates are registered/)).toBeNull();
  });

  it("still coordinates two of the member's own sessions with no teammate present", () => {
    render(<App initialState="ready" source={new FixtureProjectSource([soloSnapshot()])} />);
    expect(screen.getByRole("heading", { name: "Needs you" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: /has gone quiet/ })).toBeTruthy();
  });
});

function soloSnapshot() {
  const snapshot = snapshotForProject("prj_atlas");
  snapshot.workstreams = snapshot.workstreams.filter((stream) => stream.memberName === "Khalid");
  const mine = new Set(snapshot.workstreams.map((stream) => stream.id));
  snapshot.findings = snapshot.findings.filter((finding) => finding.workstreamIds.every((id) => mine.has(id)));
  return snapshot;
}

describe("reading a session", () => {
  it("identifies people by a stable mark, not by one grey letter", () => {
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    // A mononym gave a single character, which told the reader almost nothing.
    const chips = [...document.querySelectorAll(".avatar")].map((node) => node.textContent);
    expect(chips).toContain("KH");
    expect(chips).toContain("MI");
    // The same person is the same hue everywhere they appear.
    const khalid = [...document.querySelectorAll(".avatar")].filter((node) => node.textContent === "KH");
    const hues = new Set(khalid.map((node) => (node as HTMLElement).style.getPropertyValue("--member-hue")));
    expect(hues.size).toBe(1);
    expect([...hues][0]).toBeTruthy();
  });

  it("keeps what the session is in the header and what it did in the thread", () => {
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    const inspector = screen.getByLabelText("Details inspector");
    // Status is an attribute of the session and appears once, in the header.
    const live = within(inspector).getByLabelText("Current session activity");
    expect(live.textContent).toContain("Working now");
    expect(live.closest(".inspector-bar")).toBeTruthy();
    // At the tail there is nothing to return to, so no control offers it.
    expect(inspector.querySelector(".jump-to-now")).toBeNull();
  });

  it("names a workstream with no agent by its person rather than by an absent vendor", () => {
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    expect(screen.getByRole("button", { name: "Open Ravi's work" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /No agent connected session for/ })).toBeNull();
  });

  it("offers no control it cannot honour, and names the one that works", async () => {
    // A browser tab with no native bridge genuinely cannot reach the local
    // service, so it offers nothing there at all: a toolbar is for controls,
    // and a standing paragraph of instructions in the header is not one.
    const unreachable = new FixtureProjectSource();
    Object.defineProperty(unreachable, "live", { value: true });
    unreachable.localControl = async () => false;
    const { container } = render(<App initialState="ready" source={unreachable} />);
    expect(screen.queryByRole("button", { name: "Menu bar" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
    expect(container.querySelector(".main-bar")!.textContent).not.toContain("stickguy pause");
  });

  it("names the recovery only where a paused member needs it", async () => {
    const unreachable = new FixtureProjectSource();
    Object.defineProperty(unreachable, "live", { value: true });
    unreachable.localControl = async () => false;
    unreachable.togglePause("prj_atlas");
    render(<App initialState="ready" source={unreachable} />);
    // The instruction appears at the one moment it is actionable, beside the
    // state it resolves, rather than permanently in the header.
    expect(await screen.findByText(/Resume it from the Stickguy app or menu bar/)).toBeTruthy();
  });

  it("pauses the Project being read rather than every Project on the machine", async () => {
    const user = userEvent.setup();
    const live = new FixtureProjectSource();
    Object.defineProperty(live, "live", { value: true });
    const scoped: string[] = [];
    live.setProjectPaused = async (projectId, paused) => { scoped.push(`${projectId}:${paused}`); };
    render(<App initialState="ready" source={live} />);
    await user.click(await screen.findByRole("button", { name: "Pause" }));
    expect(scoped).toEqual(["prj_atlas:true"]);
  });

  it("keeps theme out of the toolbar, where only Project actions belong", () => {
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    expect(screen.queryByRole("button", { name: /Switch to (dark|light) theme/ })).toBeNull();
  });
});

describe("one grid for the whole column", () => {
  it("keeps machine facts on one line instead of stacking three", () => {
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    // The section and its heading button share a label, so select structurally.
    const converge = document.querySelector("section.converge") as HTMLElement;
    // Confidence and age qualify the finding rather than evidencing it, so they
    // ride the action row rather than taking one of their own.
    expect(converge.querySelector(".converge-actions .converge-meta")).toBeTruthy();
    expect(converge.querySelectorAll(".converge-meta")).toHaveLength(1);
  });
});

describe("the workroom summarises, the inspector explains", () => {
  it("keeps a finding's reasoning reachable without printing it twice", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    const card = document.querySelector("section.converge") as HTMLElement;

    // The summary must never be an unexplained alarm, so the plain-language
    // reason stays on it even though the detail moves.
    expect(card.textContent).toContain("Two live agent sessions report the same dashboard session path");
    expect(card.querySelector(".evidence")).toBeNull();
    expect(card.textContent).not.toContain("deterministic");

    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    const detail = screen.getByLabelText("Selected collision detail");
    // Everything the spec requires of a finding is on the surface that is the
    // finding: severity was always only here, and now evidence is too.
    expect(detail.textContent).toContain("deterministic confidence");
    expect(detail.textContent).toContain("high");
    expect(within(detail).getByText("Evidence")).toBeTruthy();
    expect(detail.textContent).toContain("apps/dashboard/src/session.ts");
    expect(detail.textContent).toContain("git");
  });

  it("keeps a debugging identifier off the row and in the details popover", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    const row = screen.getByRole("button", { name: "Open Codex session for Khalid" });
    expect(row.textContent).not.toContain("codex-a1b2c3");
    expect(row.textContent).toContain("feature/session-rotation");

    const inspector = screen.getByLabelText("Details inspector");
    await user.click(within(inspector).getByRole("button", { name: "Open session details" }));
    expect(within(within(inspector).getByLabelText("Session details")).getByText("codex-a1b2c3")).toBeTruthy();
  });
});

describe("focus, the inbound half of the pair", () => {
  it("mutes one of your own running sessions without hiding it from the Project", async () => {
    const user = userEvent.setup();
    const source = new FixtureProjectSource();
    render(<App initialState="ready" source={source} />);

    // The viewer's own active session is the default selection.
    const mute = await screen.findByRole("button", { name: /Mute for an hour/ });
    // Focus is asymmetric on purpose: quieting yourself must not make teammates
    // less able to avoid your work, so it never touches what is published.
    expect(mute.getAttribute("title")).toContain("Your work stays visible to the Project");

    await user.click(mute);
    const muted = await screen.findByRole("button", { name: /Muted until/ });
    expect(muted.getAttribute("title")).toContain("not being injected into its turns");
    // Nothing about sharing changed.
    expect(screen.queryByText("Workspace sharing is paused")).toBeNull();
    expect(await source.getSessionFocus("wrk_agent_fixture_codex")).toMatchObject({ focused: true });

    await user.click(muted);
    expect(await screen.findByRole("button", { name: /Mute for an hour/ })).toBeTruthy();
    expect(await source.getSessionFocus("wrk_agent_fixture_codex")).toMatchObject({ focused: false });
  });

  it("offers no mute for a teammate's session", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    await user.click(screen.getByRole("button", { name: /Open Claude Code session for Mina/ }));
    // A teammate's session is not the viewer's to quiet, and muting it here
    // would silence corrections meant for someone else.
    expect(screen.queryByRole("button", { name: /Mute for an hour/ })).toBeNull();
  });
});

describe("branch grouping", () => {
  it("groups rows by branch only once a list spans more than one", () => {
    const single = [
      { agent: { branch: "main" } }, { agent: { branch: "main" } },
    ] as unknown as Workstream[];
    expect(groupByBranch(single)).toEqual([{ branch: null, sessions: single }]);

    const mixed = [
      { id: "a", agent: { branch: "main" } },
      { id: "b", agent: { branch: "feat/x" } },
      { id: "c" },
    ] as unknown as Workstream[];
    const groups = groupByBranch(mixed);
    expect(groups.map((group) => group.branch)).toEqual(["main", "feat/x", null]);
    // A session that reported no branch keeps its own group rather than being
    // given a branch it never claimed.
    expect(groups[2]!.sessions.map((session) => session.id)).toEqual(["c"]);
  });
});

describe("finding detail navigation", () => {
  it("returns to the collision that sent you into a session", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    expect(screen.getByLabelText("Selected collision detail")).toBeTruthy();

    // Drilling into one side of a collision used to be a one-way trip: the
    // session replaced the finding and nothing led back to it.
    await user.click(screen.getByRole("button", { name: /Open Mina's session detail/ }));
    expect(screen.queryByLabelText("Selected collision detail")).toBeNull();

    const back = screen.getByRole("button", { name: /Codex and Claude are touching the session boundary/ });
    await user.click(back);
    expect(screen.getByLabelText("Selected collision detail")).toBeTruthy();
    // Back is only offered when something actually sent you here.
    expect(screen.queryByRole("button", { name: /Codex and Claude are touching the session boundary/ })).toBeNull();
  });

  it("opens a Needs you card from anywhere on the card, and shows that it can be", async () => {
    const user = userEvent.setup();
    renderReady();
    const card = document.querySelector("section.converge") as HTMLElement;
    // The whole block is the target: the headline button is stretched across it
    // rather than being one clickable line of text inside a paragraph.
    const opener = within(card).getByRole("button", { name: /Collision detected Khalid and Mina/ });
    expect(opener.classList.contains("converge-open")).toBe(true);
    // And it says so, with the same chevron every other openable row uses.
    expect(card.querySelector(".converge-chev")).toBeTruthy();

    await user.click(opener);
    expect(screen.getByLabelText("Selected collision detail")).toBeTruthy();
    expect(opener.getAttribute("aria-current")).toBe("true");
  });
});
