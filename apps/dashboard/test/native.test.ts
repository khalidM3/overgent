import { afterEach, describe, expect, it, vi } from "vitest";
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
