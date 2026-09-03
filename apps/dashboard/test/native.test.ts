import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nativeOnboarding, type OnboardingState } from "../src/native";

afterEach(() => {
  delete globalThis.wails;
});

describe("native bridge", () => {
  it("calls the registered Wails onboarding service by its full name", async () => {
    const state = { available: true, enrolled: false, adapters: [] } as unknown as OnboardingState;
    const byName = vi.fn();
    globalThis.wails = { Call: { ByName: async <T>(name: string, ...args: unknown[]) => {
      byName(name, ...args);
      return state as T;
    } } };

    await expect(nativeOnboarding.state()).resolves.toBe(state);
    expect(byName).toHaveBeenCalledWith("main.OnboardingService.State");
  });

  it("keeps owning-session identity resolution behind the local Wails method", async () => {
    const result = { vendor: "claude" as const, opened: false, detail: "handler unavailable", fallbackCommand: "claude 'review'" };
    const byName = vi.fn();
    globalThis.wails = { Call: { ByName: async <T>(name: string, ...args: unknown[]) => {
      byName(name, ...args);
      return result as T;
    } } };

    await expect(nativeOnboarding.openOwningSession("wrk_agent_0123456789abcdef0123456789abcdef", "Review this finding.", "vendor")).resolves.toBe(result);
    expect(byName).toHaveBeenCalledWith("main.OnboardingService.OpenOwningSession", "wrk_agent_0123456789abcdef0123456789abcdef", "Review this finding.", "vendor");
  });
});

/**
 * The desktop window stamps its own name into the webview's user agent
 * (`desktopUserAgentName` in apps/desktop/deeplink.go) so a page served from
 * the hosted origin can tell it is running inside the app. Nothing is granted
 * by that claim - it decides only what a screen says and where it hands off -
 * but both halves of the string are easy to change independently, and the
 * failure is silent: a hand-off control that does nothing at all.
 */
describe("recognising the desktop window from a hosted page", () => {
  const realAgent = navigator.userAgent;
  const asAgent = (value: string) => Object.defineProperty(navigator, "userAgent", { value, configurable: true });
  beforeEach(() => vi.resetModules());
  afterEach(() => asAgent(realAgent));

  it("reads the shell's user agent name and hands off to the shell's own origin", async () => {
    asAgent("Mozilla/5.0 (Macintosh) AppleWebKit/605.1.15 Safari/605.1.15 OvergentDesktop/1.0");
    const native = await import("../src/native");
    expect(native.isDesktopShell).toBe(true);
    // A registered scheme is inert here: this webview never hands one to the
    // system, so the hand-off addresses the shell's assets directly.
    expect(native.desktopHandoffURL()).toMatch(/^wails:\/\/localhost(:\d+)?\/\?desktop=onboarding&add=project$/);
  });

  it("treats an ordinary browser as an ordinary browser", async () => {
    asAgent("Mozilla/5.0 (Macintosh) AppleWebKit/605.1.15 Safari/605.1.15");
    const native = await import("../src/native");
    expect(native.isDesktopShell).toBe(false);
    // Where the scheme is the right instrument, it is the one used.
    expect(native.desktopHandoffURL()).toBe(`${native.desktopScheme}://new-project`);
  });
});
