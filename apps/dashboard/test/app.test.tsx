import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FixtureProjectSource } from "../src/fixture-source";
import { fixtureSession, snapshotForProject } from "../src/fixtures";
import { App, DesktopPreviewBanner, LiveApp, groupByArea, sessionArea } from "../src/main";
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
    expect(screen.queryByText("overgent/atlas")).toBeNull();
  });

  it("groups every session by area, with drill-down details", async () => {
    const user = userEvent.setup();
    renderReady();
    // The workroom answers "what is reaching me" before "what is everyone doing".
    expect(screen.getByRole("heading", { name: "Needs you" })).toBeTruthy();
    // One Sessions block across everyone: splitting yours from teammates' made
    // "who else is in my lane" a job of reading two lists and matching their
    // area headings. Your rows stay rich; a teammate's row stays intent-first.
    expect(screen.getByRole("heading", { name: "Sessions" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Your sessions" })).toBeNull();
    expect(screen.queryByRole("heading", { name: "Nearby" })).toBeNull();
    expect(screen.getByRole("button", { name: "Open Codex session for Khalid" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open Claude Code session for Mina" })).toBeTruthy();

    const inspector = screen.getByLabelText("Details inspector");
    expect(within(inspector).getByRole("heading", { name: "Rotate the browser session boundary" })).toBeTruthy();
    expect(within(inspector).getByText("Working now")).toBeTruthy();
    expect(within(inspector).getAllByText("feature/session-rotation")).toHaveLength(1);
    expect(within(inspector).queryByRole("tab")).toBeNull();
    expect(await within(inspector).findByText(/Rotate the browser session on every permission change/)).toBeTruthy();
    // Lifecycle and Overgent coordination live in the same chronology as chat.
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

  it("speaks about evidence only where it is degraded, and renders no empty scope field", async () => {
    const user = userEvent.setup();
    renderReady();

    // The row is the scanning surface. The goal is its primary text, so it
    // carries no label and no grade - except that this Codex goal is a
    // fallback, which is the one case the reader has to know about.
    const codexRow = screen.getByRole("button", { name: "Open Codex session for Khalid" });
    expect(codexRow.textContent).toContain("Rotate the browser session boundary");
    expect(codexRow.textContent).toContain("Edited apps/dashboard/src/session.ts · 1 parallel agent active");
    expect(codexRow.textContent).toContain("from the opening message");
    expect(codexRow.textContent).not.toContain("evidence");
    expect(codexRow.textContent).not.toMatch(/\d+%/);

    const inspector = screen.getByLabelText("Details inspector");
    // "Goal" labelled the heading directly above it, and `scope r8` reads in
    // the snapshot header below. Neither is repeated here.
    expect(within(inspector).queryByText("Goal")).toBeNull();
    const codexInspectorScope = within(inspector).getByRole("group", { name: "Scope snapshot revision 8" });
    expect(inspector.textContent).toContain("no declared intent; taken from the opening message");
    expect(codexInspectorScope.textContent).toContain("scope r8");

    // This session reported completed work and a scope, and reported nothing
    // it is waiting on and no verification. The two it did not report are
    // absent rather than present and empty.
    for (const label of ["Done", "Scope"]) expect(within(codexInspectorScope).getByText(label)).toBeTruthy();
    for (const label of ["Waiting on", "Verification"]) expect(within(codexInspectorScope).queryByText(label)).toBeNull();
    expect(codexInspectorScope.textContent).not.toContain("Nothing reported.");
    expect(codexInspectorScope.textContent).not.toContain("No verification reported.");

    // A session carrying the best its vendor can give says nothing at all
    // about evidence, on the row or in the snapshot.
    await user.click(screen.getByRole("button", { name: "Open Claude Code session for Mina" }));
    const claudeInspectorScope = within(inspector).getByRole("group", { name: "Scope snapshot revision 11" });
    for (const label of ["Waiting on", "Scope"]) expect(within(claudeInspectorScope).getByText(label)).toBeTruthy();
    for (const label of ["Done", "Verification"]) expect(within(claudeInspectorScope).queryByText(label)).toBeNull();
    expect(claudeInspectorScope.textContent).not.toContain("evidence");
    expect(claudeInspectorScope.textContent).not.toContain("inferred from");
  });

  it("shows the goals a session moved on from, so completed work is not read against the wrong goal", async () => {
    renderReady();

    // The row is scanned, so it carries the count rather than the list.
    const codexRow = screen.getByRole("button", { name: "Open Codex session for Khalid" });
    expect(codexRow.textContent).toContain("2 earlier goals");

    const inspector = screen.getByLabelText("Details inspector");
    const scope = within(inspector).getByRole("group", { name: "Scope snapshot revision 8" });
    expect(within(scope).getByText("Earlier in this session")).toBeTruthy();
    // Oldest first: the order is the chronology, so no timestamp is repeated
    // from the thread above.
    const earlier = within(scope).getByText("Earlier in this session").parentElement!.querySelectorAll("ol li");
    expect([...earlier].map((item) => item.textContent)).toEqual([
      "Read how browser sessions are currently validated",
      "Add a rotation helper to the session store",
    ]);
    expect(scope.textContent).not.toContain("no longer kept");

    // A session that has only ever had one goal shows no history at all.
    const claudeRow = screen.getByRole("button", { name: "Open Claude Code session for Mina" });
    expect(claudeRow.textContent).not.toContain("earlier goal");
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
    expect(openOwningSession).toHaveBeenCalledWith(expect.stringMatching(/^wrk_agent_/), expect.stringContaining("Overgent found:"), "vendor");
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
    const detail = screen.getByLabelText("Selected finding detail");
    expect(detail.textContent).toContain("Advisory only");
    // Three exits and no more: the decision composer, settled-elsewhere (a
    // chip on it), and dismiss. Acknowledge said "read but unhandled", which
    // is the ambiguity the product exists to remove.
    expect(screen.queryByRole("button", { name: "Acknowledge" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Mark resolved" })).toBeNull();
    // Dismissing names its reason, and the reason is the feedback that trains
    // the engine - one gesture, no separate feedback row.
    await user.click(screen.getByRole("button", { name: "Dismiss" }));
    await user.click(screen.getByRole("button", { name: "Not related" }));
    expect(detail.textContent).toContain("dismissed");
  });

  it("leads the collision thread with the decision and names where it will go", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    const detail = screen.getByLabelText("Selected finding detail");

    // Divergent branches are the case nothing else reports, and the inspector
    // says so from data the snapshot already carried.
    expect(detail.textContent).toContain("Khalid on feature/session-rotation");
    expect(detail.textContent).toContain("Mina on main");
    expect(detail.textContent).toContain("until those branches meet at merge");

    // The consequence is stated before the decision is written, not after it
    // has already been sent.
    expect(detail.textContent).toContain("Goes to Khalid's Codex session and Mina's Claude Code session.");

    // A suggested outcome prefills the composer rather than acting on its
    // own: the member always sees and can edit the exact words that will be
    // injected before anything is sent.
    await user.click(screen.getByRole("button", { name: /Settled outside Overgent/ }));
    const composer = screen.getByLabelText(/^Decision for /) as HTMLTextAreaElement;
    expect(composer.value).toContain("Settled outside Overgent");
    await user.clear(composer);
    await user.type(composer, "Khalid owns the rotation boundary; Mina reviews after it lands.");
    // The send control names the delivery, because delivery is the effect.
    await user.click(screen.getByRole("button", { name: "Send to both sessions" }));
    expect(detail.textContent).toContain("Khalid owns the rotation boundary");
    // The loop is visible where the decision was made: each affected session
    // with its own delivery state, queued until its next turn boundary.
    expect(detail.textContent).toContain("queued for its next turn");
    expect(detail.textContent).toContain("Considered records that the agent read the decision");
    // Recording the decision is what resolves the finding.
    expect(detail.textContent).toContain("resolved");
  });

  it("exposes settings and theme controls without a standalone feedback row", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    // Feedback rides the dismiss reason now; a survey row asking "was this
    // useful?" beside real controls was input with no downstream effect.
    expect(screen.queryByRole("button", { name: "Useful" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    // Settings is a screen, not a modal: it takes over the main and inspector
    // columns and the sidebar stays put.
    const settings = screen.getByRole("main", { name: "Settings" });
    // The Project's own facts come first; what it does with data is a tab.
    expect(within(settings).getByText("Coordination lives")).toBeTruthy();
    await user.click(within(settings).getByRole("button", { name: "Data" }));
    expect(within(settings).getByText("Local-first analysis, bounded Project sharing")).toBeTruthy();
    expect(await within(settings).findByText("Export retained Project data")).toBeTruthy();
    expect(screen.queryByRole("complementary", { name: "Details inspector" })).toBeNull();
    await user.click(within(settings).getByRole("button", { name: "Project" }));
    await user.click(within(settings).getByRole("button", { name: /App settings/ }));
    await user.click(screen.getByRole("radio", { name: /Dark/ }));
    expect(document.documentElement.dataset.theme).toBe("dark");
    // Back returns to the Project it was opened from.
    await user.click(screen.getByRole("button", { name: "Back to Project settings" }));
    await user.click(screen.getByRole("button", { name: "Back to Atlas launch" }));
    expect(screen.getByRole("heading", { name: "Atlas launch" })).toBeTruthy();
  });

  it("reaches members and invites from the workroom, not only from settings", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Open People for this Project" }));
    const people = screen.getByRole("main", { name: "People" });
    expect(await within(people).findByRole("heading", { name: /Members/ })).toBeTruthy();
    expect(within(people).getByRole("button", { name: "Create invite link" })).toBeTruthy();

    // Settings carries the same sections rather than a row that leaves for
    // them: one implementation, two ways in. A member who opens the Project's
    // settings to add someone should not be sent somewhere else to do it.
    await user.click(within(people).getByRole("button", { name: "Back to Atlas launch" }));
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const settings = screen.getByRole("main", { name: "Settings" });
    await user.click(within(settings).getByRole("button", { name: "People" }));
    expect(await within(settings).findByRole("button", { name: "Create invite link" })).toBeTruthy();
    expect(within(settings).getByRole("heading", { name: /Members/ })).toBeTruthy();
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
      chooseRepository: vi.fn(), createProject: vi.fn(), createLocalProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(),
      openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<App initialState="ready" source={new FixtureProjectSource()} nativeApi={api} navigate={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Add a new Project" }));
    // Probing. The form used to render here and then vanish under the member.
    const add = screen.getByRole("main", { name: "Open a repository" });
    expect(within(add).queryByPlaceholderText("Choose a local Git repository")).toBeNull();
    expect(within(add).getByRole("status").textContent).toContain("Checking this Mac");

    rejectState(new Error("The native Overgent bridge is unavailable."));
    // A browser, which is what this test environment is: the hand-off is a real
    // app hand-off and says so. Inside the desktop window the same screen says
    // "continuing on this Mac" instead, because telling somebody to open the app
    // they are looking at is the dead end this replaced.
    expect(await screen.findByRole("button", { name: "Open the Overgent app" })).toBeTruthy();
    expect(screen.getByText(/it cannot reach that service/)).toBeTruthy();
    expect(screen.queryByPlaceholderText("Choose a local Git repository")).toBeNull();
  });

  it("creates a new Project on its own screen, without bouncing back to the app", async () => {
    const user = userEvent.setup();
    const navigate = vi.fn();
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ available: true, development: true, enrolled: true, projectId: "prj_atlas", repositoryRoot: "/tmp/atlas", repositoryLabel: "atlas", deviceLabel: "Khalid’s Mac", apiBaseUrl: "http://127.0.0.1:3211", adapters: [], limitation: "" })),
      chooseRepository: vi.fn(async () => "/tmp/orbit"),
      createProject: vi.fn(), createLocalProject: vi.fn(),
      createAdditionalProject: vi.fn(async () => ({ projectId: "prj_orbit", joinCode: "inv_orbit.secret", warnings: [] })),
      joinProject: vi.fn(), joinAdditionalProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(),
      openLiveProject: vi.fn(async () => "http://127.0.0.1:49152/activate/orbit"), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<App initialState="ready" source={new FixtureProjectSource()} nativeApi={api} navigate={navigate} />);
    await user.click(screen.getByRole("button", { name: "Add a new Project" }));
    const add = await screen.findByRole("main", { name: "Open a repository" });
    // With the native bridge reachable the form is the screen. Nothing offers to
    // open the app the member is already looking at.
    expect(within(add).queryByRole("button", { name: /Overgent app/ })).toBeNull();
    await user.click(within(add).getByRole("button", { name: "Choose…" }));
    expect((within(add).getByPlaceholderText("Choose a local Git repository") as HTMLInputElement).value).toBe("/tmp/orbit");
    await user.click(within(add).getByText("Collaborate remotely"));
    await user.click(within(add).getByRole("checkbox", { name: "Create a shared Project" }));
    await user.click(within(add).getByRole("button", { name: "Create shared Project" }));
    expect(await screen.findByRole("heading", { name: "orbit is ready." })).toBeTruthy();
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
    expect(within(prompt).getByText(/device name stays in Project settings/)).toBeTruthy();

    await user.click(within(prompt).getByRole("button", { name: "Choose a name" }));
    // The field is in App settings now: a person is not called something
    // different in each Project, so it is asked once and applied everywhere.
    const dialog = screen.getByRole("main", { name: "App settings" });
    const field = within(dialog).getByLabelText("Display name");
    expect((field as HTMLInputElement).value).toBe("");
    expect((field as HTMLInputElement).placeholder).toBe("khalids-macbook-pro.local");

    await user.type(field, "Khalid M");
    await user.click(within(dialog).getByRole("button", { name: "Save name" }));
    expect(await within(dialog).findByText(/Display name updated across/)).toBeTruthy();
  });

  it("rejects an email address as live-work identity", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" initialSession={deviceNamedSession} source={new FixtureProjectSource()} />);
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const settings = screen.getByRole("main", { name: "Settings" });
    await user.click(within(settings).getByRole("button", { name: /App settings/ }));
    const dialog = await screen.findByRole("main", { name: "App settings" });
    await user.type(within(dialog).getByLabelText("Display name"), "khalid@example.com");
    await user.click(within(dialog).getByRole("button", { name: "Save name" }));
    expect((await within(dialog).findByRole("alert")).textContent).toContain("email address cannot be your Project identity");
  });

  it("keeps device names under an explicit security heading rather than as identity", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    const dialog = screen.getByRole("main", { name: "Settings" });
    await user.click(within(dialog).getByRole("button", { name: "People" }));
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
  it("tells a browser with no session that only the Overgent app can issue a ticket", async () => {
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
    expect(await screen.findByText(/Open the Project from the Overgent app/)).toBeTruthy();
    expect(screen.getByText(/overgent dashboard --project/)).toBeTruthy();
    const first = await screen.findByRole("button", { name: "Check for a session" });
    expect(screen.queryByRole("alert")).toBeNull();

    await user.click(first);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("still has no active session");
    expect(alert.textContent).toContain("Overgent Dev.app");
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

  it("tells each finding's whole story as one case", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" source={new FixtureProjectSource()} />);
    await user.click(screen.getByRole("button", { name: "History" }));

    // One entry per finding, newest movement first, under day dividers. The
    // three independently ordered lists stored the lifecycle as a puzzle. The
    // view names itself once, on the tab that opened it, and not again as a
    // heading directly beneath.
    expect(screen.getByRole("button", { name: /^History/ }).getAttribute("aria-current")).toBe("page");
    expect(screen.queryByRole("heading", { name: "History" })).toBeNull();
    expect(screen.getAllByText("Today").length).toBeGreaterThan(0);
    const collision = screen.getByRole("button", { name: "Open Collision detected" });
    expect(collision.textContent).toContain("collision detected");
    expect(collision.textContent).toContain("still open");
    expect(screen.queryByRole("heading", { name: "Raised" })).toBeNull();
    expect(screen.queryByRole("heading", { name: "Delivered into a turn" })).toBeNull();
    // Filters narrow by how the case ended.
    await user.click(screen.getByRole("button", { name: /^Dismissed/ }));
    expect(screen.getByText("Nothing here under this filter.")).toBeTruthy();
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

describe("the Project is the only top level", () => {
  const sidebar = () => screen.getByRole("navigation", { name: "Projects" });

  it("lists Projects in the sidebar and nothing above them", () => {
    renderReady();
    // Workroom and History used to sit above this list as root items, which
    // put the open Project on screen three ways at once: as Workroom, as its
    // History, and as the current row here. Neither was a sibling of the list
    // - both are views of whichever row is selected - so neither is here.
    const side = sidebar();
    expect(within(side).queryByRole("button", { name: "Workroom" })).toBeNull();
    expect(within(side).queryByRole("button", { name: "History" })).toBeNull();
    expect(within(side).getByRole("button", { name: /Atlas launch/ }).getAttribute("aria-current")).toBe("page");
    expect(within(side).getByRole("button", { name: /Orchard mobile/ })).toBeTruthy();
  });

  it("switches views under the Project's name, which stays on screen", async () => {
    const user = userEvent.setup();
    renderReady();
    const views = screen.getByRole("navigation", { name: "Project views" });
    expect(screen.getByRole("heading", { name: "Atlas launch" })).toBeTruthy();
    expect(within(views).getByRole("button", { name: /^Workroom/ }).getAttribute("aria-current")).toBe("page");

    await user.click(within(views).getByRole("button", { name: /^History/ }));
    // Reading order is Project, then view: History is a different answer about
    // the same Project, so the Project's own header does not go anywhere.
    expect(screen.getByRole("heading", { name: "Atlas launch" })).toBeTruthy();
    expect(screen.getByText(/Everything this Project has caught/)).toBeTruthy();
    expect(within(views).getByRole("button", { name: /^Workroom/ }).getAttribute("aria-current")).toBeNull();
  });

  it("starts a Project you switch to on its own workroom, not the last Project's view", async () => {
    const user = userEvent.setup();
    renderReady();
    const views = screen.getByRole("navigation", { name: "Project views" });
    await user.click(within(views).getByRole("button", { name: /^History/ }));
    await user.click(within(sidebar()).getByRole("button", { name: /Orchard mobile/ }));

    expect(screen.getByRole("heading", { name: "Orchard mobile" })).toBeTruthy();
    expect(screen.getByRole("navigation", { name: "Project views" }).querySelector('[aria-current="page"]')!.textContent).toContain("Workroom");
  });

  it("names the Project, never the view, as the way back out of a screen", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: /^History/ }));
    await user.click(screen.getByRole("button", { name: "Open Project settings" }));
    // A view is not a place you came from. Back from a screen returned to
    // "History" whenever that tab happened to be open, naming a view as if it
    // were one.
    expect(screen.getByRole("button", { name: "Back to Atlas launch" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Back to History" })).toBeNull();
  });

  it("names the People button for the screen it opens, in every kind of Project", async () => {
    const user = userEvent.setup();
    renderReady();
    // It read "Sharing" on a local Project and "Invite" on a shared one while
    // announcing "Invite" to a screen reader either way, and "Sharing" named
    // the wrong control besides: Pause is what starts and stops sharing.
    expect(screen.queryByRole("button", { name: /Sharing/ })).toBeNull();
    const people = screen.getByRole("button", { name: "Open People for this Project" });
    expect(people.textContent).toContain("People");

    await user.click(people);
    expect(screen.getByRole("heading", { name: "People" })).toBeTruthy();
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
    expect(container.querySelector(".main-bar")!.textContent).not.toContain("overgent pause");
  });

  it("names the recovery only where a paused member needs it", async () => {
    const unreachable = new FixtureProjectSource();
    Object.defineProperty(unreachable, "live", { value: true });
    unreachable.localControl = async () => false;
    unreachable.togglePause("prj_atlas");
    render(<App initialState="ready" source={unreachable} />);
    // The instruction appears at the one moment it is actionable, beside the
    // state it resolves, rather than permanently in the header.
    expect(await screen.findByText(/Resume it from the Overgent app or menu bar/)).toBeTruthy();
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
    const detail = screen.getByLabelText("Selected finding detail");
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

describe("finding detail navigation", () => {
  it("returns to the collision that sent you into a session", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: /Collision detected Khalid and Mina/ }));
    expect(screen.getByLabelText("Selected finding detail")).toBeTruthy();

    // Drilling into one side of a collision used to be a one-way trip: the
    // session replaced the finding and nothing led back to it.
    await user.click(screen.getByRole("button", { name: /Open Mina's session detail/ }));
    expect(screen.queryByLabelText("Selected finding detail")).toBeNull();

    const back = screen.getByRole("button", { name: /Two of Khalid.s sessions are both changing/ });
    await user.click(back);
    expect(screen.getByLabelText("Selected finding detail")).toBeTruthy();
    // Back is only offered when something actually sent you here.
    expect(screen.queryByRole("button", { name: /Two of Khalid.s sessions are both changing/ })).toBeNull();
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
    expect(screen.getByLabelText("Selected finding detail")).toBeTruthy();
    expect(opener.getAttribute("aria-current")).toBe("true");
  });
});

describe("grouping sessions by area", () => {
  // Same shape the branch-grouping tests use: grouping reads a handful of
  // fields, so the fixture states exactly those.
  const session = (id: string, extra: Record<string, unknown> = {}): Workstream =>
    ({ id, paths: [], ...extra }) as unknown as Workstream;

  it("prefers a declared contract, because a contract is what two sessions collide over", () => {
    expect(sessionArea(session("a", { contracts: ["BrowserSession rotation"], components: ["auth"], paths: ["src/a.ts"] })))
      .toBe("BrowserSession rotation");
  });

  it("falls back to a declared component, then to the directory the paths share", () => {
    expect(sessionArea(session("b", { components: ["protocol generation"], paths: ["src/a.ts"] }))).toBe("protocol generation");
    expect(sessionArea(session("c", { paths: ["protocol/schemas/a.json", "protocol/schemas/b.json"] }))).toBe("protocol/schemas");
    // Nothing in common yields nothing, rather than a parent nobody declared.
    expect(sessionArea(session("d", { paths: ["apps/a.ts", "internal/b.go"] }))).toBeNull();
    expect(sessionArea(session("e", { paths: [] }))).toBeNull();
  });

  it("does not group when grouping would add one heading to a list that was already legible", () => {
    const one = [session("a", { contracts: ["Refresh"] })];
    expect(groupByArea(one)).toEqual([{ area: null, sessions: one }]);
  });

  it("puts areas holding more than one session first, and the unplaced last", () => {
    const sessions = [
      session("solo", { contracts: ["Zebra"] }),
      session("unplaced", { paths: [] }),
      session("pair-1", { contracts: ["Alpha"] }),
      session("pair-2", { contracts: ["Alpha"] }),
    ];

    expect(groupByArea(sessions).map((group) => [group.area, group.sessions.length]))
      .toEqual([["Alpha", 2], ["Zebra", 1], [null, 1]]);
  });
});

// The landing an invite link opens on. Its one job is that a shared link never
// dead-ends: it renders before authentication, keeps the invite in the URL
// fragment (which never reaches server logs), and hands the recipient the one
// command that redeems it.
describe("invite join landing", () => {
  it("turns a valid fragment into the join command without transmitting it", async () => {
    const { JoinLanding } = await import("../src/main");
    render(<JoinLanding fragment="inv_49b778cd.sec_ret-42" />);
    expect(screen.getByRole("heading", { name: /invited to an Overgent Project/i })).toBeTruthy();
    const command = screen.getByText(/^overgent join /);
    expect(command.textContent).toContain("/join#inv_49b778cd.sec_ret-42");
    expect(screen.getByText(/sends it nowhere/i)).toBeTruthy();
  });

  it("states plainly when the fragment is missing or damaged", async () => {
    const { JoinLanding } = await import("../src/main");
    render(<JoinLanding fragment="" />);
    expect(screen.getByRole("alert").textContent).toContain("incomplete");
    expect(screen.queryByText(/^overgent join /)).toBeNull();
  });
});

describe("switching between Projects", () => {
  it("moves the whole workroom to the Project picked in the sidebar", async () => {
    const user = userEvent.setup();
    render(<App initialState="ready" source={new FixtureProjectSource()} />);

    // The session carries two Projects, and the sidebar offers both.
    const orchard = screen.getByRole("button", { name: /Orchard mobile/ });
    expect(screen.getByRole("button", { name: /Atlas launch/ }).getAttribute("aria-current")).toBe("page");
    expect(orchard.getAttribute("aria-current")).toBeNull();
    // Atlas's work is on screen and Orchard's is not.
    expect(screen.queryByText("Reduce checkout to two explicit steps.")).toBeNull();

    await user.click(orchard);

    // Current-ness follows the click, and so does what the workroom is reading.
    expect(screen.getByRole("button", { name: /Orchard mobile/ }).getAttribute("aria-current")).toBe("page");
    expect(screen.getByRole("button", { name: /Atlas launch/ }).getAttribute("aria-current")).toBeNull();
    expect(screen.getAllByText("Reduce checkout to two explicit steps.").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Open Codex session for Khalid" })).toBeNull();
  });
});

/**
 * The live dashboard polls one Project at a time, and which one it polls used
 * to be fixed at page load. Switching Projects therefore left the second one
 * frozen at whatever it looked like when the tab opened: no new finding, no
 * session moving, nothing until the member reopened it from the app.
 */
describe("live polling follows the Project on screen", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("stops polling the Project it left and starts polling the one it opened", async () => {
    const session = {
      memberId: "mem_live", memberName: "Khalid", memberNameSource: "member",
      projects: [
        { id: "prj_one", name: "Project One", repositoryLabel: "one", semanticStatus: "enabled", semanticMode: "offline_fallback" },
        { id: "prj_two", name: "Project Two", repositoryLabel: "two", semanticStatus: "enabled", semanticMode: "offline_fallback" },
      ],
      selectedProjectId: "prj_one",
    };
    const snapshotFor = (id: string) => ({
      project: session.projects.find((project) => project.id === id),
      contextRevision: 1, synchronizedAt: "now", workspacePaused: false,
      workstreams: [], findings: [], activity: [], devices: [],
    });
    const polled: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/dashboard/session")) return json(session);
      const snapshot = url.match(/\/dashboard\/projects\/([^/?]+)$/);
      if (snapshot) {
        polled.push(snapshot[1]!);
        return json(snapshotFor(snapshot[1]!));
      }
      const collaboration = url.match(/\/projects\/([^/?]+)\/collaboration$/);
      if (collaboration) return json({ projectId: collaboration[1], syncCards: [], resolutions: [], cursor: "time:0" });
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal("fetch", fetchMock);

    vi.useFakeTimers({ shouldAdvanceTime: true });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<LiveApp />);
    await screen.findByRole("button", { name: /Project Two/ });

    // Both Projects were read once at startup; only the selected one polls.
    polled.length = 0;
    await advance(2_500);
    expect(polled).toContain("prj_one");
    expect(polled).not.toContain("prj_two");

    await user.click(screen.getByRole("button", { name: /Project Two/ }));
    polled.length = 0;
    await advance(2_500);
    expect(polled).toContain("prj_two");
    expect(polled).not.toContain("prj_one");
  });
});

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

async function advance(milliseconds: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(milliseconds);
  });
}

/**
 * Pausing writes to the local service and then re-reads the snapshot, so there
 * is a real gap between the click and the new state. The control used to spend
 * that gap greyed out with the same icon and the same word, which reads as a
 * button that did nothing rather than one that is working.
 */
describe("pausing sharing", () => {
  it("says it is working while the change is in flight", async () => {
    const source = new FixtureProjectSource();
    let settle: () => void = () => undefined;
    vi.spyOn(source, "localControl").mockResolvedValue(true);
    vi.spyOn(source, "setProjectPaused").mockImplementation(() => new Promise<void>((resolve) => { settle = resolve; }));

    const user = userEvent.setup();
    render(<App initialState="ready" source={source} />);

    const pause = await screen.findByRole("button", { name: /^Pause$/ });
    await user.click(pause);

    // The label is the state: greying out alone said nothing.
    const working = await screen.findByRole("button", { name: /Pausing…/ });
    expect(working.getAttribute("aria-busy")).toBe("true");
    expect((working as HTMLButtonElement).disabled).toBe(true);

    await act(async () => { settle(); });
    expect(await screen.findByRole("button", { name: /^Pause$|^Resume$/ })).toBeTruthy();
  });
});
