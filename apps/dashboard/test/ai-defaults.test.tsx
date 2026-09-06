import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AIDefaultsSettings } from "../src/ai-defaults";
import type { AIDefaults, AIDefaultsWrite } from "../src/native";

const stored: AIDefaults = {
  judgment: { provider: "none", model: "", baseUrl: "", keyStored: false },
  embeddings: { provider: "deterministic", model: "overgent-concepts/v1", dimensions: 1024, baseUrl: "", keyStored: false },
};

const api = (overrides: Partial<{ aiDefaults: () => Promise<AIDefaults>; putAIDefaults: (write: AIDefaultsWrite) => Promise<AIDefaults> }> = {}) => ({
  aiDefaults: vi.fn(async () => stored),
  putAIDefaults: vi.fn(async (write: AIDefaultsWrite) => ({
    judgment: { provider: write.judgment.provider, model: write.judgment.model, baseUrl: write.judgment.baseUrl ?? "", keyStored: Boolean(write.judgment.apiKey) },
    embeddings: { provider: write.embeddings.provider, model: write.embeddings.model, baseUrl: write.embeddings.baseUrl ?? "", dimensions: 1024 as const, keyStored: Boolean(write.embeddings.apiKey) },
  })),
  ...overrides,
});

describe("defaults for new Projects", () => {
  it("says where these apply automatically and where they do not", async () => {
    render(<AIDefaultsSettings api={api()} />);
    // The split is the entire safety property, so it is on screen rather than
    // left to be discovered when a key turns up on somebody else's server.
    const note = await screen.findByText(/A Project on this Mac takes these automatically/);
    expect(note.textContent).toMatch(/shared Project does not/);
    expect(note.textContent).toMatch(/uploads it to that Project’s server/);
  });

  it("saves a provider and its key, and reports the key as stored without returning it", async () => {
    const bridge = api();
    const user = userEvent.setup();
    render(<AIDefaultsSettings api={bridge} />);

    await user.selectOptions(await screen.findByLabelText("Model provider"), "anthropic");
    await user.selectOptions(screen.getByLabelText("Model"), "claude-opus-5");
    await user.type(screen.getByLabelText("API key"), "sk-test-key");
    await user.click(screen.getByRole("button", { name: "Save defaults" }));

    expect(bridge.putAIDefaults).toHaveBeenCalledWith(expect.objectContaining({
      judgment: expect.objectContaining({ provider: "anthropic", model: "claude-opus-5", apiKey: "sk-test-key" }),
    }));
    // Re-read from what came back: a stored key is a flag, never a value, so
    // the field goes empty and says the key is kept.
    const key = await screen.findByLabelText("API key") as HTMLInputElement;
    expect(key.value).toBe("");
    expect(key.placeholder).toMatch(/Key saved/);
  });

  it("will not save a provider with no model, which would configure nothing", async () => {
    const user = userEvent.setup();
    render(<AIDefaultsSettings api={api()} />);
    await user.selectOptions(await screen.findByLabelText("Model provider"), "anthropic");
    expect((screen.getByRole("button", { name: "Save defaults" }) as HTMLButtonElement).disabled).toBe(true);
    await user.selectOptions(screen.getByLabelText("Model"), "claude-opus-5");
    expect((screen.getByRole("button", { name: "Save defaults" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("offers a retry rather than an empty panel when the bridge is unavailable", async () => {
    const failing = api({ aiDefaults: vi.fn(async () => { throw new Error("bridge unavailable"); }) });
    render(<AIDefaultsSettings api={failing} />);
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
  });

  it("is explicit that Projects already made are untouched", async () => {
    render(<AIDefaultsSettings api={api()} />);
    expect(await screen.findByText(/Projects you already have keep the settings they have/)).toBeTruthy();
  });
});
