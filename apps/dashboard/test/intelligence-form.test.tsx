import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AIDefaultsSettings } from "../src/ai-defaults";
import { DesktopAISettings } from "../src/desktop-ai-settings";
import type { AIDefaults, AIDefaultsWrite, AISettings, AISettingsWrite } from "../src/native";

/**
 * The shared intelligence form.
 *
 * What is tested here is what a preset is allowed to do to a key. A preset
 * fills in an address, and an address is where a credential gets spent — so
 * every one of these is a case where carrying the key over, or quietly falling
 * back to somebody else's endpoint, would send a live key somewhere the member
 * did not name.
 */
const off: AIDefaults = {
  judgment: { provider: "none", model: "", baseUrl: "", keyStored: false },
  embeddings: { provider: "deterministic", model: "overgent-concepts/v1", dimensions: 1024, baseUrl: "", keyStored: false },
};
const withAnthropicKey: AIDefaults = { ...off, judgment: { provider: "anthropic", model: "claude-sonnet-5", baseUrl: "", keyStored: true } };

const defaultsBridge = (stored: AIDefaults = off) => ({
  aiDefaults: vi.fn(async () => stored),
  putAIDefaults: vi.fn(async (write: AIDefaultsWrite) => ({
    judgment: { provider: write.judgment.provider, model: write.judgment.model, baseUrl: write.judgment.baseUrl ?? "", keyStored: Boolean(write.judgment.apiKey) },
    embeddings: { provider: write.embeddings.provider, model: write.embeddings.model, baseUrl: write.embeddings.baseUrl ?? "", dimensions: 1024 as const, keyStored: Boolean(write.embeddings.apiKey) },
  })),
});

const projectSettings = (over: Partial<AISettings> = {}): AISettings => ({
  judgment: { provider: "none", model: "", baseUrl: null, keyConfigured: false, keyHint: null },
  embeddings: { provider: "deterministic", model: "overgent-concepts/v1", dimensions: 1024, baseUrl: null, keyConfigured: false, keyHint: null },
  effective: { judgment: "none", embeddings: "deterministic" },
  revision: 1, updatedAt: "2026-09-05T00:00:00.000Z",
  ...over,
});
const projectBridge = (stored: AISettings = projectSettings(), defaults?: AIDefaults) => ({
  aiSettings: vi.fn(async () => stored),
  putAISettings: vi.fn(async (_projectId: string, write: AISettingsWrite) => ({
    judgment: { provider: write.judgment.provider, model: write.judgment.model, baseUrl: write.judgment.baseUrl ?? null, keyConfigured: Boolean(write.judgment.apiKey), keyHint: null },
    embeddings: { provider: write.embeddings.provider, model: write.embeddings.model, dimensions: 1024 as const, baseUrl: write.embeddings.baseUrl ?? null, keyConfigured: Boolean(write.embeddings.apiKey), keyHint: null },
    effective: { judgment: "project" as const, embeddings: "deterministic" as const },
    revision: 2, updatedAt: "2026-09-05T00:00:00.000Z",
  })),
  ...(defaults ? { aiDefaults: vi.fn(async () => defaults) } : {}),
});

describe("choosing a provider", () => {
  it("shows the whole endpoint a key is about to be spent at", async () => {
    const bridge = defaultsBridge();
    const user = userEvent.setup();
    render(<AIDefaultsSettings api={bridge} />);

    await user.selectOptions(await screen.findByLabelText("Model provider"), "openai");
    // The one fact that settles whether the member meant this provider.
    expect(screen.getByText("https://api.openai.com/v1/chat/completions")).toBeTruthy();
    // No model is chosen for them: a stale ID written into the field is a
    // request the provider rejects, blamed on the key.
    expect((screen.getByLabelText("Model") as HTMLSelectElement).value).toBe("");

    await user.selectOptions(screen.getByLabelText("Model"), "gpt-5.6-terra");
    await user.type(screen.getByLabelText("API key"), "sk-test-key");
    await user.click(screen.getByRole("button", { name: "Save defaults" }));
    expect(bridge.putAIDefaults).toHaveBeenCalledWith(expect.objectContaining({
      judgment: expect.objectContaining({ provider: "openai-compatible", model: "gpt-5.6-terra", apiKey: "sk-test-key" }),
    }));
  });

  it("takes a model the list has never heard of, without losing the list", async () => {
    const bridge = defaultsBridge();
    const user = userEvent.setup();
    render(<AIDefaultsSettings api={bridge} />);

    await user.selectOptions(await screen.findByLabelText("Model provider"), "anthropic");
    // Every list of model IDs goes stale, so typing one is a first-class
    // choice rather than an escape hatch.
    await user.selectOptions(screen.getByLabelText("Model"), "__other");
    await user.type(screen.getByLabelText("Model"), "claude-opus-6");
    await user.type(screen.getByLabelText("API key"), "sk-test-key");
    await user.click(screen.getByRole("button", { name: "Save defaults" }));
    expect(bridge.putAIDefaults).toHaveBeenCalledWith(expect.objectContaining({
      judgment: expect.objectContaining({ model: "claude-opus-6" }),
    }));
  });

  it("drops the saved key when the destination changes", async () => {
    const bridge = defaultsBridge(withAnthropicKey);
    const user = userEvent.setup();
    render(<AIDefaultsSettings api={bridge} />);

    await user.selectOptions(await screen.findByLabelText("Model provider"), "openai");
    await user.selectOptions(screen.getByLabelText("Model"), "gpt-5.6-terra");
    await user.click(screen.getByRole("button", { name: "Save defaults" }));
    // A key for Anthropic is not a key for OpenAI. Carrying it would send a
    // live credential to an address the member never saved it against.
    expect(bridge.putAIDefaults).toHaveBeenCalledWith(expect.objectContaining({
      judgment: expect.objectContaining({ removeKey: true }),
    }));
    expect(bridge.putAIDefaults.mock.calls[0]![0].judgment.apiKey).toBeUndefined();
  });

  it("lets a member take back a removal before saving", async () => {
    const bridge = defaultsBridge(withAnthropicKey);
    const user = userEvent.setup();
    render(<AIDefaultsSettings api={bridge} />);

    await user.click(await screen.findByRole("button", { name: "Remove it" }));
    expect(screen.getByText(/This key is deleted when you save/)).toBeTruthy();
    // The control stays a control rather than turning into a label of its own
    // state, so a mis-click costs one click rather than a reload.
    await user.click(screen.getByRole("button", { name: "Keep it" }));
    await user.click(screen.getByRole("button", { name: "Save defaults" }));
    expect(bridge.putAIDefaults.mock.calls[0]![0].judgment.removeKey).toBeFalsy();
  });

  it("does not offer to keep a key once the destination has moved", async () => {
    const bridge = defaultsBridge(withAnthropicKey);
    const user = userEvent.setup();
    render(<AIDefaultsSettings api={bridge} />);

    await user.selectOptions(await screen.findByLabelText("Model provider"), "openai");
    // Keeping it would mean sending a key issued for Anthropic to OpenAI, so
    // the removal is stated as the consequence it is and has no undo.
    expect(screen.getByText(/The key saved for api\.anthropic\.com is deleted when you save/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Keep it" })).toBeNull();
  });

  it("will not save a custom server with no address, which would spend the key at OpenAI", async () => {
    const bridge = defaultsBridge();
    const user = userEvent.setup();
    render(<AIDefaultsSettings api={bridge} />);

    await user.selectOptions(await screen.findByLabelText("Model provider"), "custom");
    await user.type(screen.getByLabelText("Model"), "local-model");
    const save = screen.getByRole("button", { name: "Save defaults" }) as HTMLButtonElement;
    // An empty base URL is not "no opinion": the client falls back to
    // api.openai.com, so an unsaid address is a different provider.
    expect(save.disabled).toBe(true);
    expect(screen.getByText("Judgment needs the server’s address before it can be saved.")).toBeTruthy();

    await user.type(screen.getByLabelText("Server address"), "http://localhost:1234");
    expect(save.disabled).toBe(false);
  });

  it("states what runs before any provider is chosen", async () => {
    render(<AIDefaultsSettings api={defaultsBridge()} />);
    // Level one is what makes the other two optional rather than load-bearing,
    // so it is stated first and says it needs nothing.
    const level = await screen.findByRole("heading", { name: "Overlapping code" });
    expect(level.parentElement?.textContent).toMatch(/Always on/);
    expect(screen.getByText(/no source code or diffs are ever part of it/)).toBeTruthy();
  });
});

describe("a Project's own providers", () => {
  it("copies this Mac's defaults without copying the key", async () => {
    const bridge = projectBridge(projectSettings(), withAnthropicKey);
    const user = userEvent.setup();
    render(<DesktopAISettings api={bridge} projectId="atlas" local={false} />);

    // The offer says what it will and will not carry before it is taken.
    expect(await screen.findByText(/The key is not copied/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Use this Mac’s defaults" }));
    expect((screen.getByLabelText("Model") as HTMLSelectElement).value).toBe("claude-sonnet-5");
    // The key is in the login Keychain and this page has no route to it, which
    // is what makes offering the button on a shared Project safe at all. The
    // field is empty and says so, rather than looking configured.
    const key = screen.getByLabelText("API key") as HTMLInputElement;
    expect(key.value).toBe("");
    expect(key.placeholder).toBe("Enter an API key");
    // Nothing is left to copy, so the offer stops offering.
    expect(screen.queryByRole("button", { name: "Use this Mac’s defaults" })).toBeNull();
  });

  it("holds the save until uploading a key to a shared Project is agreed to", async () => {
    const bridge = projectBridge();
    const user = userEvent.setup();
    render(<DesktopAISettings api={bridge} projectId="atlas" local={false} />);

    await user.selectOptions(await screen.findByLabelText("Model provider"), "anthropic");
    await user.selectOptions(screen.getByLabelText("Model"), "claude-sonnet-5");
    await user.type(screen.getByLabelText("API key"), "sk-test-key");
    const save = screen.getByRole("button", { name: "Save intelligence settings" }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    expect(screen.getByText("Confirm that these keys may be stored on this Project’s server.")).toBeTruthy();

    await user.click(screen.getByLabelText(/Store these keys on this Project’s server/));
    expect(save.disabled).toBe(false);
    await user.click(save);
    expect(bridge.putAISettings).toHaveBeenCalledWith("atlas", expect.objectContaining({
      judgment: expect.objectContaining({ apiKey: "sk-test-key" }),
    }));
  });

  it("reports a key the deployment owns rather than implying the form is empty by mistake", async () => {
    render(<DesktopAISettings api={projectBridge(projectSettings({ effective: { judgment: "operator", embeddings: "deterministic" } }))} projectId="atlas" local={false} />);
    expect(await screen.findByText(/Judgment runs on the deployment’s own key/)).toBeTruthy();
  });
});
