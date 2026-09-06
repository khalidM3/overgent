import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MacSettings } from "../src/mac-settings";
import type { AdapterState, NativeOnboarding, OnboardingState } from "../src/native";

const adapter = (over: Partial<AdapterState> = {}): AdapterState => ({
  name: "Codex", installed: true, configured: false, fidelity: "Git", detail: "Detected on this Mac",
  binding: "not_configured", currentProfile: "test", runtimeVerified: false, restartRequired: false,
  reconnectAllowed: false, hooksNeedReview: false, ...over,
});

const state = (adapters: AdapterState[]): OnboardingState => ({
  available: true, development: false, enrolled: true, projectId: "atlas", repositoryRoot: "/repo",
  repositoryLabel: "overgent/atlas", deviceLabel: "Test Mac", apiBaseUrl: "http://127.0.0.1:4319",
  adapters, limitation: "", backend: { present: true, running: true, port: 4319, version: "0.4.1" },
});

const bridge = (over: Partial<NativeOnboarding> = {}) => ({
  configureAdapters: vi.fn(async () => undefined),
  disconnectAgent: vi.fn(async () => undefined),
  reconnectAdapter: vi.fn(async () => undefined),
  state: vi.fn(async () => state([])),
  ...over,
}) as unknown as NativeOnboarding;

const open = (api: NativeOnboarding, value: OnboardingState, theme: "light" | "dark" | "system" = "system", onTheme = vi.fn()) =>
  render(<MacSettings api={api} state={value} projectId="atlas" onBack={vi.fn()} refresh={async () => value} theme={theme} onTheme={onTheme} />);

describe("appearance", () => {
  it("offers light, dark and following the Mac, and marks which is on", async () => {
    const onTheme = vi.fn();
    const user = userEvent.setup();
    open(bridge(), state([]), "dark", onTheme);
    await user.click(screen.getByRole("button", { name: "Appearance" }));

    const group = screen.getByRole("radiogroup", { name: "Appearance" });
    expect(within(group).getAllByRole("radio")).toHaveLength(3);
    // "System" is the option the old boolean could not express, which is why a
    // Mac that switches at sunset used to leave Overgent behind.
    expect((within(group).getByRole("radio", { name: /Dark/ }) as HTMLElement).getAttribute("aria-checked")).toBe("true");
    await user.click(within(group).getByRole("radio", { name: /System/ }));
    expect(onTheme).toHaveBeenCalledWith("system");
  });
});

describe("coding agents", () => {
  it("puts one action per agent on its own row, and none on an agent that is not there", async () => {
    const api = bridge();
    const user = userEvent.setup();
    open(api, state([
      adapter({ name: "Claude Code", configured: true, runtimeVerified: true }),
      adapter({ name: "Codex" }),
      adapter({ name: "Cursor", installed: false, detail: "Not detected" }),
    ]));

    await user.click(screen.getByRole("button", { name: "Agents" }));
    const rows = screen.getAllByRole("checkbox").length;
    // The two installed agents carry the "connect in new Projects" preference;
    // an agent that is not on this Mac carries no controls at all.
    expect(rows).toBe(2);
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(api.configureAdapters).toHaveBeenCalledWith("/repo", true, false, false);
  });

  it("never claims observation from an agent whose hooks have not run", async () => {
    const user = userEvent.setup();
    open(bridge(), state([adapter({ configured: true, hooksNeedReview: true, reviewGuidance: "Review hooks in Codex before sessions can be observed." })]));
    await user.click(screen.getByRole("button", { name: "Agents" }));
    expect(screen.getByText(/Review hooks in Codex/)).toBeTruthy();
    expect(screen.queryByText("Observing session activity")).toBeNull();
  });
});

describe("the local service", () => {
  it("reports what it is doing as facts rather than a sentence", async () => {
    const user = userEvent.setup();
    open(bridge(), state([]));
    await user.click(screen.getByRole("button", { name: "Advanced" }));
    expect(screen.getByText("Running now")).toBeTruthy();
    expect(screen.getByText("127.0.0.1:4319")).toBeTruthy();
  });
});
