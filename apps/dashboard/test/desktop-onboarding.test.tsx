import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DesktopOnboarding } from "../src/desktop-onboarding";
import type { NativeOnboarding, OnboardingState } from "../src/native";

const adapters = [
  { name: "Codex", installed: true, configured: false, fidelity: "MCP intent + Git observation", detail: "Project scoped", binding: "not_configured" as const, currentProfile: "Stickguy Shared Dev", runtimeVerified: false, restartRequired: false, reconnectAllowed: false },
  { name: "Claude Code", installed: true, configured: false, fidelity: "MCP intent + Git observation", detail: "Project scoped", binding: "not_configured" as const, currentProfile: "Stickguy Shared Dev", runtimeVerified: false, restartRequired: false, reconnectAllowed: false },
];
const initial: OnboardingState = { available: true, development: true, enrolled: false, projectId: "", repositoryRoot: "", repositoryLabel: "", deviceLabel: "Khalid’s Mac", apiBaseUrl: "http://127.0.0.1:3211", adapters, limitation: "First Project only." };
const enrolled: OnboardingState = { ...initial, enrolled: true, projectId: "prj_test", repositoryRoot: "/tmp/atlas", repositoryLabel: "atlas", adapters: adapters.map((adapter) => ({ ...adapter, configured: true, binding: "current", runtimeVerified: true })) };

describe("desktop onboarding", () => {
  it("creates a Project, opts both detected agents in, and exposes the one-use invite", async () => {
    const user = userEvent.setup();
    let calls = 0;
    const api: NativeOnboarding = {
      state: vi.fn(async () => calls++ === 0 ? initial : enrolled),
      chooseRepository: vi.fn(async () => "/tmp/atlas"),
      createProject: vi.fn(async () => ({ projectId: "prj_test", joinCode: "inv_test.secret", warnings: null as unknown as string[] })),
      createAdditionalProject: vi.fn(),
      joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), sessionDetail: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    await screen.findByRole("heading", { name: /Connect the Project/ });
    await user.click(screen.getByRole("button", { name: "Choose…" }));
    await user.click(screen.getByRole("checkbox", { name: /Codex/ }));
    await user.click(screen.getByRole("checkbox", { name: /Claude Code/ }));
    await user.click(screen.getByRole("button", { name: "Create and connect" }));
    expect(await screen.findByRole("heading", { name: "atlas" })).toBeTruthy();
    expect(screen.getByText("inv_test.secret")).toBeTruthy();
    expect(api.createProject).toHaveBeenCalledWith(expect.objectContaining({ repositoryRoot: "/tmp/atlas", projectLabel: "atlas", enableCodex: true, enableClaude: true }));
  });

  it("allows explicit adapter configuration when process-level detection is inconclusive", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => ({ ...initial, adapters: initial.adapters.map((adapter) => ({ ...adapter, installed: false })) })),
      chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), sessionDetail: vi.fn(),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} />);
    const codex = await screen.findByRole("checkbox", { name: /Codex/ });
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
      openLiveProject: vi.fn(async () => "http://127.0.0.1:49152/activate/nonce"), sessionDetail: vi.fn(),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} navigate={navigate} />);
    await user.click(await screen.findByRole("button", { name: "Open live Project" }));
    expect(navigate).toHaveBeenCalledWith("http://127.0.0.1:49152/activate/nonce");
  });

  it("explains automatic repo-scoped session observation without requiring worktrees", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => enrolled), chooseRepository: vi.fn(async () => "/tmp/atlas-claude"), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(),
      connectAgentWorktree: vi.fn(async () => enrolled.adapters[1]), openLiveProject: vi.fn(), sessionDetail: vi.fn(),
    };
    render(<DesktopOnboarding api={api} />);
    expect(await screen.findByText(/New Codex and Claude Code sessions opened in this repository appear automatically/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Assign .* worktree/ })).toBeNull();
    expect(api.connectAgentWorktree).not.toHaveBeenCalled();
  });

  it("previews and explicitly confirms a safe profile reconnect", async () => {
    const otherProfile: OnboardingState = { ...enrolled, adapters: enrolled.adapters.map((adapter) => adapter.name === "Codex" ? { ...adapter, configured: false, binding: "other_profile", previousProfile: "Stickguy", runtimeVerified: false, restartRequired: false, reconnectAllowed: true, detail: "Connected to a different Stickguy profile." } : adapter) };
    const api: NativeOnboarding = {
      state: vi.fn(async () => otherProfile), chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(),
      reconnectAdapter: vi.fn(async () => ({ ...otherProfile.adapters[0], configured: true, binding: "current" as const, reconnectAllowed: false, restartRequired: true })),
      connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), sessionDetail: vi.fn(),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} />);
    await user.click(await screen.findByRole("button", { name: "Reconnect to this Project" }));
    const dialog = screen.getByRole("dialog", { name: "Reconnect Codex" });
    expect(dialog).toBeTruthy();
    expect(screen.getByText("Stickguy")).toBeTruthy();
    expect(screen.getByText("Stickguy Shared Dev")).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Reconnect to this Project" }));
    expect(api.reconnectAdapter).toHaveBeenCalledWith("/tmp/atlas", "codex");
  });

  it("keeps a configured adapter pending until a live event verifies it", async () => {
    const pending: OnboardingState = { ...enrolled, adapters: enrolled.adapters.map((adapter) => adapter.name === "Codex" ? { ...adapter, runtimeVerified: false, restartRequired: true, detail: "Configured for this Project. Restart the agent, then start a new task in this repository to verify the connection." } : adapter) };
    const api: NativeOnboarding = {
      state: vi.fn(async () => pending), chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), sessionDetail: vi.fn(),
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
      configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(), sessionDetail: vi.fn(),
    };
    render(<DesktopOnboarding api={api} navigate={() => undefined} />);
    await screen.findByLabelText("Your name");

    const name = screen.getByLabelText("Your name") as HTMLInputElement;
    expect(name.value).toBe("");
    expect(screen.getByText(/Not your email address/)).toBeTruthy();
    // The device name is still available, but behind an explicit security disclosure.
    expect(screen.getByText("Device name & security")).toBeTruthy();
    expect(screen.getByText(/never shown as your identity/)).toBeTruthy();

    await user.type(name, "Khalid M");
    expect((screen.getByLabelText("Your name") as HTMLInputElement).value).toBe("Khalid M");
  });
});
