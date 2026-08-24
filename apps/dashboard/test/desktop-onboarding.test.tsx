import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DesktopOnboarding } from "../src/desktop-onboarding";
import type { NativeOnboarding, OnboardingState } from "../src/native";

const adapters = [
  { name: "Codex", installed: true, configured: false, fidelity: "MCP intent + Git observation", detail: "Project scoped" },
  { name: "Claude Code", installed: true, configured: false, fidelity: "MCP intent + Git observation", detail: "Project scoped" },
];
const initial: OnboardingState = { available: true, enrolled: false, projectId: "", repositoryRoot: "", repositoryLabel: "", deviceLabel: "Khalid’s Mac", apiBaseUrl: "http://127.0.0.1:3211", adapters, limitation: "First Project only." };
const enrolled: OnboardingState = { ...initial, enrolled: true, projectId: "prj_test", repositoryRoot: "/tmp/atlas", repositoryLabel: "atlas", adapters: adapters.map((adapter) => ({ ...adapter, configured: true })) };

describe("desktop onboarding", () => {
  it("creates a Project, opts both detected agents in, and exposes the one-use invite", async () => {
    const user = userEvent.setup();
    let calls = 0;
    const api: NativeOnboarding = {
      state: vi.fn(async () => calls++ === 0 ? initial : enrolled),
      chooseRepository: vi.fn(async () => "/tmp/atlas"),
      createProject: vi.fn(async () => ({ projectId: "prj_test", joinCode: "inv_test.secret", warnings: null as unknown as string[] })),
      joinProject: vi.fn(), configureAdapters: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
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
      chooseRepository: vi.fn(), createProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
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
      state: vi.fn(async () => enrolled), chooseRepository: vi.fn(), createProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(), connectAgentWorktree: vi.fn(),
      openLiveProject: vi.fn(async () => "http://127.0.0.1:49152/activate/nonce"),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} navigate={navigate} />);
    await user.click(await screen.findByRole("button", { name: "Open live Project" }));
    expect(navigate).toHaveBeenCalledWith("http://127.0.0.1:49152/activate/nonce");
  });

  it("assigns an existing linked worktree without asking Stickguy to mutate Git", async () => {
    const api: NativeOnboarding = {
      state: vi.fn(async () => enrolled), chooseRepository: vi.fn(async () => "/tmp/atlas-claude"), createProject: vi.fn(), joinProject: vi.fn(), configureAdapters: vi.fn(),
      connectAgentWorktree: vi.fn(async () => enrolled.adapters[1]), openLiveProject: vi.fn(),
    };
    const user = userEvent.setup();
    render(<DesktopOnboarding api={api} />);
    await user.click(await screen.findByRole("button", { name: "Assign Claude worktree…" }));
    expect(api.connectAgentWorktree).toHaveBeenCalledWith("/tmp/atlas-claude", "claude");
    expect(await screen.findByText(/Restart sessions that were already open/)).toBeTruthy();
  });
});
