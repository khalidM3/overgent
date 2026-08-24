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
});
