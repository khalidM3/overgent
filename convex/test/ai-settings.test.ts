import { describe, expect, it } from "vitest";

const intelligenceModulePath = "../functions/intelligence.js";
const httpModulePath = "../functions/http.js";

describe("Project AI settings boundary", () => {
  it("resolves Project keys before explicitly enabled operator keys", async () => {
    const { selectProviderSource } = await import(intelligenceModulePath) as {
      selectProviderSource(options: { disabled: boolean; projectKeyUsable: boolean; operatorEnabled: boolean; operatorKeyConfigured: boolean; fallback: "none" | "deterministic" }): string;
    };
    expect(selectProviderSource({ disabled: false, projectKeyUsable: true, operatorEnabled: true, operatorKeyConfigured: true, fallback: "none" })).toBe("project");
    expect(selectProviderSource({ disabled: false, projectKeyUsable: false, operatorEnabled: true, operatorKeyConfigured: true, fallback: "none" })).toBe("operator");
    expect(selectProviderSource({ disabled: false, projectKeyUsable: false, operatorEnabled: false, operatorKeyConfigured: true, fallback: "none" })).toBe("none");
    expect(selectProviderSource({ disabled: true, projectKeyUsable: true, operatorEnabled: true, operatorKeyConfigured: true, fallback: "deterministic" })).toBe("deterministic");
  });

  it("accepts credentials only on the dedicated parser and enforces provider origins", async () => {
    const { parseAISettingsWrite } = await import(httpModulePath) as { parseAISettingsWrite(value: unknown): unknown };
    const key = "api_key=synthetic-value";
    const parsed = parseAISettingsWrite({
      judgment: { provider: "openai-compatible", model: "local-model", baseUrl: "http://127.0.0.1:11434", apiKey: key },
      embeddings: { provider: "deterministic", model: "overgent-concepts/v1", dimensions: 1024 },
    });
    expect(JSON.stringify(parsed)).toContain(key);
    expect(() => parseAISettingsWrite({
      judgment: { provider: "anthropic", model: "m", baseUrl: "http://example.com", apiKey: "12345678" },
      embeddings: { provider: "openai", model: "m", dimensions: 1024 },
    })).toThrow();
    expect(() => parseAISettingsWrite({
      judgment: { provider: "anthropic", model: "m" },
      embeddings: { provider: "openai", model: "m", dimensions: 1536 },
    })).toThrow("unsupported_dimensions");
  });
});
