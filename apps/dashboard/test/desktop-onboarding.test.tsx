import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DesktopOnboarding } from "../src/desktop-onboarding";
import type { NativeOnboarding, OnboardingState } from "../src/native";

const adapters = [
  { name: "Codex", installed: true, configured: false, fidelity: "MCP intent + Git observation", detail: "Project scoped", binding: "not_configured" as const, currentProfile: "Overgent Shared Dev", runtimeVerified: false, restartRequired: false, reconnectAllowed: false, hooksNeedReview: false },
  { name: "Claude Code", installed: true, configured: false, fidelity: "MCP intent + Git observation", detail: "Project scoped", binding: "not_configured" as const, currentProfile: "Overgent Shared Dev", runtimeVerified: false, restartRequired: false, reconnectAllowed: false, hooksNeedReview: false },
];
const initial: OnboardingState = { available: true, development: true, enrolled: false, projectId: "", repositoryRoot: "", repositoryLabel: "", deviceLabel: "Khalid’s Mac", apiBaseUrl: "http://127.0.0.1:3211", adapters, limitation: "First Project only." };
const enrolled: OnboardingState = { ...initial, enrolled: true, projectId: "prj_test", repositoryRoot: "/tmp/atlas", repositoryLabel: "atlas", adapters: adapters.map((adapter) => ({ ...adapter, configured: true, binding: "current", runtimeVerified: true })) };

const needsReview: OnboardingState = {
  ...enrolled,
  adapters: [
    { ...adapters[0], configured: true, binding: "current" as const, runtimeVerified: false, hooksNeedReview: true,
      detail: "Connected, but Codex has not trusted the activity hooks yet, so no session activity can be observed. Open Codex → Settings → Hooks and choose Trust all, or run /hooks in the Codex CLI.",
      reviewGuidance: "Open Codex → Settings → Hooks and choose Trust all, or run /hooks in the Codex CLI." },
    { ...adapters[1], configured: true, binding: "current" as const, runtimeVerified: true },
  ],
};

/**
 * First run is three steps, so a test that wants the connect step has to walk
 * there. The walk is the assertion in one test and setup in the rest.
 */
async function reachAgentStep(user: ReturnType<typeof userEvent.setup>, mode: "create" | "join" = "create") {
  await user.click(await screen.findByRole("button", { name: mode === "create" ? "Create a Project" : "I have an invite code" }));
  await user.click(screen.getByRole("button", { name: "Choose…" }));
  if (mode === "join") await user.type(screen.getByLabelText("Invite code"), "inv_test.secret");
  await user.click(screen.getByRole("button", { name: "Continue" }));
}

describe("desktop onboarding", () => {
  it("tells the member a Codex binding is inert while its hooks await review", async () => {
    const api = {
      state: vi.fn(async () => needsReview),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    } as unknown as NativeOnboarding;
    render(<DesktopOnboarding api={api} />);
    const connections = await screen.findByLabelText("Agent connections");
    // An installed-but-untrusted binding must never read as working: that is
    // the exact failure this state exists to expose.
    expect(within(connections).getByText(/has not trusted the activity hooks/)).toBeTruthy();
    expect(within(connections).getByText(/Settings . Hooks/)).toBeTruthy();
  });

  it("opens on what Overgent is and what it already found on this Mac", async () => {
    const api = {
      state: vi.fn(async () => initial),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    } as unknown as NativeOnboarding;
    render(<DesktopOnboarding api={api} />);
    await screen.findByRole("heading", { name: /Welcome to Overgent/ });
    // Three lines, and nothing asked. What this Mac already has is said on the
    // step where it is actionable, next to the checkbox it applies to, rather
    // than twice.
    const points = screen.getByRole("list").querySelectorAll("li");
    expect(points.length).toBe(3);
    expect(screen.queryByLabelText("Your name")).toBeNull();
    expect(screen.queryByText(/Step 1 of 3/)).toBeNull();
    expect(screen.getByRole("button", { name: "Create a Project" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "I have an invite code" })).toBeTruthy();
  });

  it("states plainly when it found no coding agents, without dressing it as a fault", async () => {
    const api = {
      state: vi.fn(async () => ({ ...initial, adapters: initial.adapters.map((adapter) => ({ ...adapter, installed: false })) })),
      chooseRepository: vi.fn(async () => "/tmp/atlas"), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    } as unknown as NativeOnboarding;
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} />);
    await reachAgentStep(user);
    // Finding nothing is an answer, not work converging on the member. It is
    // said once, on the row it applies to, and it is not styled as a fault.
    const line = screen.getByText(/Not found . connect anyway and sessions appear once Codex/);
    expect(line).toBeTruthy();
    expect(line.closest(".connection-line")).toBeNull();
    // And it must not be a dead end - the Project can still be created.
    expect((screen.getByRole("button", { name: "Create and connect" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("puts the sharing boundary on the step that connects agents, one disclosure from exact", async () => {
    const user = userEvent.setup();
    const api = {
      state: vi.fn(async () => initial),
      chooseRepository: vi.fn(async () => "/tmp/atlas"), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    } as unknown as NativeOnboarding;
    render(<DesktopOnboarding api={api} />);
    await reachAgentStep(user);
    // The summary line is what a member reads at the moment of highest
    // friction. The full boundary must stay reachable and stay exact.
    expect(screen.getByText(/never your source, diffs, prompts, or credentials/)).toBeTruthy();
    const disclosure = screen.getByText("Exactly what is and is not shared").closest("details");
    expect(disclosure).toBeTruthy();
    expect(within(disclosure as HTMLElement).getByText(/never cross the wire/)).toBeTruthy();
    // The boundary is stated once, immediately above the button that acts on
    // it. It used to be stated three times on this screen, which is how a
    // member learns to skip all three.
    expect(screen.getAllByText(/never your source, diffs, prompts, or credentials/).length).toBe(1);
  });

  it("names the field a disabled Continue is waiting on", async () => {
    const user = userEvent.setup();
    const api = {
      state: vi.fn(async () => initial),
      chooseRepository: vi.fn(async () => ""), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    } as unknown as NativeOnboarding;
    render(<DesktopOnboarding api={api} />);
    await user.click(await screen.findByRole("button", { name: "Create a Project" }));
    expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(true);
    // A control that cannot be pressed explains itself or it is a dead end.
    expect(screen.getByText("Choose a repository to continue.")).toBeTruthy();
  });

  it("creates a Project, opts both detected agents in, and exposes the one-use invite", async () => {
    const user = userEvent.setup();
    let calls = 0;
    const api: NativeOnboarding = {
      state: vi.fn(async () => calls++ === 0 ? initial : enrolled),
      chooseRepository: vi.fn(async () => "/tmp/atlas"),
      createProject: vi.fn(async () => ({ projectId: "prj_test", joinCode: "inv_test.secret", warnings: null as unknown as string[] })),
      createAdditionalProject: vi.fn(),
      joinProject: vi.fn(), joinAdditionalProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    await reachAgentStep(user);
    // Detected agents arrive already ticked. Enrolling without connecting one
    // leaves the Project observing Git alone, which reads as a broken install,
    // so the detected default is what the member should have to opt out of.
    expect((screen.getByRole("checkbox", { name: /Codex/ }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole("checkbox", { name: /Claude Code/ }) as HTMLInputElement).checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Create and connect" }));
    // Enrollment ends by saying it worked and offering the one thing worth
    // doing next, not by handing over a status list.
    expect(await screen.findByRole("heading", { name: "atlas is connected." })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open Project" })).toBeTruthy();
    // A Project with one member already does its whole job, so the invite is
    // an option rather than the next step - but it must still be reachable.
    expect(screen.queryByText("inv_test.secret")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Invite a teammate" }));
    expect(screen.getByText("inv_test.secret")).toBeTruthy();
    expect(api.createProject).toHaveBeenCalledWith(expect.objectContaining({ repositoryRoot: "/tmp/atlas", projectLabel: "atlas", enableCodex: true, enableClaude: true }));
  });

  it("allows explicit adapter configuration when process-level detection is inconclusive", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ ...initial, adapters: initial.adapters.map((adapter) => ({ ...adapter, installed: false })) })),
      chooseRepository: vi.fn(async () => "/tmp/atlas"), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} />);
    await reachAgentStep(user);
    const codex = screen.getByRole("checkbox", { name: /Codex/ });
    const claude = screen.getByRole("checkbox", { name: /Claude Code/ });
    expect((codex as HTMLInputElement).disabled).toBe(false);
    expect((claude as HTMLInputElement).disabled).toBe(false);
    await user.click(codex);
    await user.click(claude);
    expect((codex as HTMLInputElement).checked).toBe(true);
    expect((claude as HTMLInputElement).checked).toBe(true);
  });

  it("opens the authenticated live Project through a native one-time handoff", async () => {
    const navigate = vi.fn();
    const api: NativeOnboarding = {
      state: vi.fn(async () => enrolled), chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(),
      openLiveProject: vi.fn(async () => "http://127.0.0.1:49152/activate/nonce"), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} navigate={navigate} />);
    await user.click(await screen.findByRole("button", { name: "Open Project" }));
    expect(navigate).toHaveBeenCalledWith("http://127.0.0.1:49152/activate/nonce");
  });

  it("explains automatic repo-scoped session observation without requiring worktrees", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => enrolled), chooseRepository: vi.fn(async () => "/tmp/atlas-claude"), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(),
      connectAgentWorktree: vi.fn(async () => enrolled.adapters[1]), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    // Observation is repository-scoped and automatic, so the home screen has no
    // worktree to assign and never asks about one.
    expect(await screen.findByLabelText("Agent connections")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Assign .* worktree/ })).toBeNull();
    expect(api.connectAgentWorktree).not.toHaveBeenCalled();
    // Home is a place to leave, not a wall. Both exits are ordinary buttons.
    expect(screen.getByRole("button", { name: "Open Project" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add a Project" })).toBeTruthy();
  });

  it("previews and explicitly confirms a safe profile reconnect", async () => {
    const otherProfile: OnboardingState = { ...enrolled, adapters: enrolled.adapters.map((adapter) => adapter.name === "Codex" ? { ...adapter, configured: false, binding: "other_profile", previousProfile: "Overgent", runtimeVerified: false, restartRequired: false, reconnectAllowed: true, detail: "Connected to a different Overgent profile." } : adapter) };
    const api: NativeOnboarding = {
      state: vi.fn(async () => otherProfile), chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(), configureAdapters: vi.fn(),
      reconnectAdapter: vi.fn(async () => ({ ...otherProfile.adapters[0], configured: true, binding: "current" as const, reconnectAllowed: false, restartRequired: true })),
      connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} />);
    await user.click(await screen.findByRole("button", { name: "Reconnect to this Project" }));
    const dialog = screen.getByRole("dialog", { name: "Reconnect Codex" });
    expect(dialog).toBeTruthy();
    expect(screen.getByText("Overgent")).toBeTruthy();
    expect(screen.getByText("Overgent Shared Dev")).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Reconnect to this Project" }));
    expect(api.reconnectAdapter).toHaveBeenCalledWith("/tmp/atlas", "codex");
  });

  it("keeps a configured adapter pending until a live event verifies it", async () => {
    const pending: OnboardingState = { ...enrolled, adapters: enrolled.adapters.map((adapter) => adapter.name === "Codex" ? { ...adapter, runtimeVerified: false, restartRequired: true, detail: "Configured for this Project. Restart the agent, then start a new task in this repository to verify the connection." } : adapter) };
    const api: NativeOnboarding = {
      state: vi.fn(async () => pending), chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByText(/Restart the agent, then start a new task/)).toBeTruthy();
    // The row already says what to do. Raising an attention banner as well
    // fired after every successful setup, which taught members to ignore the
    // one case that is genuinely converging on them.
    expect(screen.queryByText(/needs attention/)).toBeNull();
  });
});

describe("first-run identity", () => {
  it("asks for a member name and keeps the device name as a security detail", async () => {
    const user = userEvent.setup();
    const api: NativeOnboarding = {
      state: vi.fn(async () => initial),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} navigate={() => undefined} />);
    await user.click(await screen.findByRole("button", { name: "Create a Project" }));

    const name = screen.getByLabelText("Your name") as HTMLInputElement;
    expect(name.value).toBe("");
    await user.type(name, "Khalid M");
    expect((screen.getByLabelText("Your name") as HTMLInputElement).value).toBe("Khalid M");

    // The device name is an audit label with a correct default, so first run
    // no longer asks for it. A field whose answer is already right is a step
    // the member pays for and nobody reads.
    expect(screen.queryByLabelText("Device name")).toBeNull();
    expect(screen.queryByText("Device name & security")).toBeNull();
  });

  it("offers an in-app reconnect when an owner revoked this Mac", async () => {
    const user = userEvent.setup();
    const reset = vi.fn(async () => initial);
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ ...enrolled, credential: "revoked" as const })),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: reset, sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByRole("heading", { name: /access was revoked/ })).toBeTruthy();
    // The member must be told their code is safe before being asked to confirm.
    expect(screen.getByText(/repositories and code were not touched/)).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Reconnect this Mac" }));
    expect(screen.getByRole("heading", { name: "Reconnect this Mac?" })).toBeTruthy();
    await user.click(within(screen.getByRole("group", { name: "Confirm reconnect" })).getByRole("button", { name: "Reconnect this Mac" }));
    expect(reset).toHaveBeenCalled();
    // Clearing the dead credential returns the member to first-run enrollment.
    expect(await screen.findByRole("heading", { name: /Welcome to Overgent/ })).toBeTruthy();
  });

  it("explains an unknown credential differently from a revoked one", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ ...enrolled, credential: "unknown" as const })),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByRole("heading", { name: /no longer recognised/ })).toBeTruthy();
    expect(screen.queryByText(/access was revoked/)).toBeNull();
  });

  it("never offers to erase an enrollment it could not verify", async () => {
    const reset = vi.fn();
    const api: NativeOnboarding = {
      // Offline, timing out, or a server fault - none of which mean locked out.
      state: vi.fn(async () => ({ ...enrolled, credential: "uncertain" as const })),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: reset, sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByRole("heading", { name: /could not confirm this Mac/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Reconnect this Mac" })).toBeNull();
    expect(reset).not.toHaveBeenCalled();
  });

  it("leaves a healthy enrollment alone", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ ...enrolled, credential: "ok" as const })),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByRole("heading", { name: "atlas" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Reconnect this Mac" })).toBeNull();
  });
});

describe("joining a second Project", () => {
  const nativeDouble = (overrides: Record<string, unknown>) => ({
    state: vi.fn(async () => enrolled), chooseRepository: vi.fn(async () => "/tmp/beacon"),
    createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), joinAdditionalProject: vi.fn(),
    configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
    resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    ...overrides,
  } as unknown as NativeOnboarding);

  it("accepts an invite without minting a second device identity", async () => {
    const joinAdditionalProject = vi.fn(async () => ({ projectId: "prj_invited", joinCode: "", warnings: [] }));
    const api = nativeDouble({ joinAdditionalProject });
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} navigate={() => undefined} />);

    await user.click(await screen.findByRole("button", { name: "Join a Project" }));
    await user.type(await screen.findByLabelText("Invite code"), "inv_test.secret");
    await user.click(screen.getByRole("button", { name: "Choose…" }));
    await user.click(screen.getByRole("button", { name: "Join Project" }));

    expect(await screen.findByRole("heading", { name: "Project joined" })).toBeTruthy();
    expect(joinAdditionalProject).toHaveBeenCalledWith(expect.objectContaining({ joinCode: "inv_test.secret", repositoryRoot: "/tmp/beacon" }));
    // joinProject enrolls a new device and is only correct on a Mac that has
    // none. Calling it here burned the invite and joined nothing.
    expect(api.joinProject).not.toHaveBeenCalled();
    expect(api.createAdditionalProject).not.toHaveBeenCalled();
    // A member who just accepted an invite has not been handed one to pass on.
    expect(screen.queryByText("One-use invite code")).toBeNull();
  });

  it("comes back to the home screen instead of stranding the member", async () => {
    const user = userEvent.setup();
    render(<DesktopOnboarding api={nativeDouble({})} navigate={() => undefined} />);
    await user.click(await screen.findByRole("button", { name: "Join a Project" }));
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("button", { name: "Open Project" })).toBeTruthy();
  });

  it("says why the invite was refused rather than failing silently", async () => {
    const api = nativeDouble({ joinAdditionalProject: vi.fn(async () => { throw new Error("join Project: That invite has expired. Ask for a new one."); }) });
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} navigate={() => undefined} />);
    await user.click(await screen.findByRole("button", { name: "Join a Project" }));
    await user.type(await screen.findByLabelText("Invite code"), "inv_test.secret");
    await user.click(screen.getByRole("button", { name: "Choose…" }));
    await user.click(screen.getByRole("button", { name: "Join Project" }));
    expect(await screen.findByText(/That invite has expired/)).toBeTruthy();
  });
});
