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
  await user.click(await screen.findByRole("button", { name: mode === "create" ? "Create your first Project" : "I have an invite code" }));
  await user.click(screen.getByRole("button", { name: "Choose…" }));
  if (mode === "join") await user.type(screen.getByLabelText("Invite code"), "inv_test.secret");
  await user.click(screen.getByRole("button", { name: "Continue" }));
}

describe("desktop onboarding", () => {
  it("tells the member a Codex binding is inert while its hooks await review", async () => {
    const api = {
      state: vi.fn(async () => needsReview),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
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
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    } as unknown as NativeOnboarding;
    render(<DesktopOnboarding api={api} />);
    await screen.findByRole("heading", { name: /Welcome to Overgent/ });
    // Opening on an empty repository picker made the member guess whether the
    // app had found anything at all. It has, and it says so first.
    expect(screen.getByText("Found on this Mac: Codex and Claude Code")).toBeTruthy();
    expect(screen.getByText("Step 1 of 3")).toBeTruthy();
    // Nothing on the first step touches the machine, and it says that too.
    expect(screen.getByText(/Nothing is configured until you confirm on the last step/)).toBeTruthy();
  });

  it("states plainly when it found no coding agents, without dressing it as a fault", async () => {
    const api = {
      state: vi.fn(async () => ({ ...initial, adapters: initial.adapters.map((adapter) => ({ ...adapter, installed: false })) })),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    } as unknown as NativeOnboarding;
    render(<DesktopOnboarding api={api} />);
    const line = await screen.findByText("No coding agents found on this Mac");
    expect(line).toBeTruthy();
    expect(screen.getByText(/install an agent later and its sessions appear/)).toBeTruthy();
    // Finding nothing is an answer, not work converging on the member: the
    // alert treatment is reserved and must not leak onto an ordinary fact.
    expect(line.closest(".connection-line")?.className).not.toContain("needs-attention");
    // And it must not be a dead end - the Project can still be created.
    expect(screen.getByRole("button", { name: "Create your first Project" })).toBeTruthy();
  });

  it("puts the sharing boundary on the step that connects agents, one disclosure from exact", async () => {
    const user = userEvent.setup();
    const api = {
      state: vi.fn(async () => initial),
      chooseRepository: vi.fn(async () => "/tmp/atlas"), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
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
    // Connecting installs a background service and writes agent configuration.
    // Saying so before the button is the difference between consent and a
    // surprise.
    expect(screen.getByText(/starts Overgent’s background service on this Mac/)).toBeTruthy();
  });

  it("names the field a disabled Continue is waiting on", async () => {
    const user = userEvent.setup();
    const api = {
      state: vi.fn(async () => initial),
      chooseRepository: vi.fn(async () => ""), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    } as unknown as NativeOnboarding;
    render(<DesktopOnboarding api={api} />);
    await user.click(await screen.findByRole("button", { name: "Create your first Project" }));
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
      joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    await reachAgentStep(user);
    // Detected agents arrive already ticked. Enrolling without connecting one
    // leaves the Project observing Git alone, which reads as a broken install,
    // so the detected default is what the member should have to opt out of.
    expect((screen.getByRole("checkbox", { name: /Codex/ }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole("checkbox", { name: /Claude Code/ }) as HTMLInputElement).checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Create and connect" }));
    expect(await screen.findByRole("heading", { name: "atlas" })).toBeTruthy();
    expect(screen.getByText("inv_test.secret")).toBeTruthy();
    expect(api.createProject).toHaveBeenCalledWith(expect.objectContaining({ repositoryRoot: "/tmp/atlas", projectLabel: "atlas", enableCodex: true, enableClaude: true }));
  });

  it("allows explicit adapter configuration when process-level detection is inconclusive", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ ...initial, adapters: initial.adapters.map((adapter) => ({ ...adapter, installed: false })) })),
      chooseRepository: vi.fn(async () => "/tmp/atlas"), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
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
      state: vi.fn(async () => enrolled), chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(),
      openLiveProject: vi.fn(async () => "http://127.0.0.1:49152/activate/nonce"), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} navigate={navigate} />);
    await user.click(await screen.findByRole("button", { name: "Open live Project" }));
    expect(navigate).toHaveBeenCalledWith("http://127.0.0.1:49152/activate/nonce");
  });

  it("explains automatic repo-scoped session observation without requiring worktrees", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => enrolled), chooseRepository: vi.fn(async () => "/tmp/atlas-claude"), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(),
      connectAgentWorktree: vi.fn(async () => enrolled.adapters[1]), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByText(/New Codex, Claude Code, and Cursor sessions opened in this repository appear automatically/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Assign .* worktree/ })).toBeNull();
    expect(api.connectAgentWorktree).not.toHaveBeenCalled();
  });

  it("previews and explicitly confirms a safe profile reconnect", async () => {
    const otherProfile: OnboardingState = { ...enrolled, adapters: enrolled.adapters.map((adapter) => adapter.name === "Codex" ? { ...adapter, configured: false, binding: "other_profile", previousProfile: "Overgent", runtimeVerified: false, restartRequired: false, reconnectAllowed: true, detail: "Connected to a different Overgent profile." } : adapter) };
    const api: NativeOnboarding = {
      state: vi.fn(async () => otherProfile), chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(),
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
      state: vi.fn(async () => pending), chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByText(/Restart the agent, then start a new task/)).toBeTruthy();
    expect(screen.getByText(/agent setup needs attention/)).toBeTruthy();
  });
});

describe("first-run identity", () => {
  it("asks for a member name and keeps the device name as a security detail", async () => {
    const user = userEvent.setup();
    const api: NativeOnboarding = {
      state: vi.fn(async () => initial),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} navigate={() => undefined} />);
    await user.click(await screen.findByRole("button", { name: "Create your first Project" }));

    const name = screen.getByLabelText("Your name") as HTMLInputElement;
    expect(name.value).toBe("");
    expect(screen.getByText(/Not your email address/)).toBeTruthy();
    // The device name is still available, but behind an explicit security disclosure.
    expect(screen.getByText("Device name & security")).toBeTruthy();
    expect(screen.getByText(/never shown as your identity/)).toBeTruthy();

    await user.type(name, "Khalid M");
    expect((screen.getByLabelText("Your name") as HTMLInputElement).value).toBe("Khalid M");
  });

  it("offers an in-app reconnect when an owner revoked this Mac", async () => {
    const user = userEvent.setup();
    const reset = vi.fn(async () => initial);
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ ...enrolled, credential: "revoked" as const })),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
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
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
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
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
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
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
      resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByRole("heading", { name: "atlas" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Reconnect this Mac" })).toBeNull();
  });
});
