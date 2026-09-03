import { act, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

// The whole point of this file is the branch that only exists inside the
// desktop window, which is decided at module load from the webview's user
// agent. Mocking the flag is the only way to reach it from jsdom.
vi.mock("../src/native", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../src/native")>()),
  isDesktopShell: true,
  desktopHandoffURL: () => "/?desktop=onboarding&add=project",
}));

const { NewProjectScreen } = await import("../src/new-project");
const bridgeless = {
  state: vi.fn(async () => { throw new Error("The native Overgent bridge is unavailable."); }),
  chooseRepository: vi.fn(), createProject: vi.fn(), createAdditionalProject: vi.fn(), joinProject: vi.fn(),
  configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(),
  resetEnrollment: vi.fn(), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn(),
} as unknown as import("../src/native").NativeOnboarding;

describe("adding a Project from inside the desktop window", () => {
  it("continues on this Mac instead of offering to open the app the member is in", async () => {
    const navigate = vi.fn();
    render(<NewProjectScreen api={bridgeless} displayName="Khalid" navigate={navigate} backLabel="atlas" onBack={vi.fn()} />);

    // The live Project view is hosted, so it has no bridge - but it is running
    // inside the app, and the previous screen answered that by telling the
    // member to open the app. Nothing here says that.
    expect(await screen.findByText("Continuing on this Mac…")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Open the Overgent app/ })).toBeNull();

    // And it continues on its own: the member already asked for this screen, so
    // an interstitial with one button is a second ask for the same decision.
    expect(navigate).toHaveBeenCalledWith("/?desktop=onboarding&add=project");
    // The shell's own origin, never the registered scheme: WKWebView never
    // hands a custom scheme to the system, so an `overgent://` link here is the
    // dead end this replaced.
    expect(navigate.mock.calls[0][0].startsWith("overgent")).toBe(false);
  });

  it("offers a route that does not depend on the hand-off when the window has not moved", async () => {
    vi.useFakeTimers();
    try {
      render(<NewProjectScreen api={bridgeless} displayName="" navigate={vi.fn()} backLabel="atlas" onBack={vi.fn()} />);
      await vi.waitFor(() => expect(screen.getByText("Continuing on this Mac…")).toBeTruthy());
      await act(async () => { await vi.advanceTimersByTimeAsync(3_000); });
      // A hand-off that silently does nothing is the failure this screen was
      // built around, so the fallback names a route through the menu bar that
      // cannot fail the same way.
      expect(screen.getByRole("button", { name: "Continue on this Mac" })).toBeTruthy();
      expect(screen.getByText(/from the Overgent icon in the menu bar/)).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });
});
