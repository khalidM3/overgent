import { afterEach, describe, expect, it, vi } from "vitest";
import { LiveProjectSource, intelligenceFromSettings, loadSession } from "../src/live-source";
import type { AISettings } from "../src/native";

afterEach(() => vi.unstubAllGlobals());

describe("live dashboard transport", () => {
  it("derives intelligence depth from effective providers without exposing keys", () => {
    const settings: AISettings = {
      judgment: { provider: "anthropic", model: "claude", baseUrl: null, keyConfigured: true, keyHint: "1234" },
      embeddings: { provider: "openai", model: "embed", dimensions: 1024, baseUrl: null, keyConfigured: true, keyHint: "5678" },
      effective: { judgment: "operator", embeddings: "project" }, revision: 1, updatedAt: "2026-09-08T00:00:00Z",
    };
    expect(intelligenceFromSettings(settings, true)).toEqual({ structural: "active", embeddings: "provider", judgment: "active", degraded: true });
  });

  it("counts the deterministic concept provider as built-in embedding depth", () => {
    const settings: AISettings = {
      judgment: { provider: "none", model: "none", baseUrl: null, keyConfigured: false, keyHint: null },
      embeddings: { provider: "deterministic", model: "overgent-concepts/v1", dimensions: 1024, baseUrl: null, keyConfigured: false, keyHint: null },
      effective: { judgment: "none", embeddings: "deterministic" }, revision: 1, updatedAt: "2026-09-08T00:00:00Z",
    };
    expect(intelligenceFromSettings(settings)).toEqual({ structural: "active", embeddings: "built_in", judgment: "off", degraded: false });
  });

  it("loads the authorized session with the HTTP-only cookie boundary", async () => {
    const session = { memberName: "Synthetic member", projects: [], selectedProjectId: "" };
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(async () => new Response(JSON.stringify(session), { status: 200, headers: { "content-type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(loadSession()).resolves.toEqual(session);
    expect(fetchMock.mock.calls[0]?.[1]?.credentials).toBe("include");
  });

  it("posts bounded radar feedback with the browser session cookie", async () => {
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(async () => new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await new LiveProjectSource([]).recordFindingFeedback("fnd_synthetic", "not_related");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/findings/fnd_synthetic/feedback");
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: "POST", credentials: "include", body: JSON.stringify({ value: "not_related" }) });
  });

  it("persists a finding state change and re-reads the snapshot the poll would overwrite", async () => {
    const snapshot = { project: { id: "prj_fixture" } };
    const responses = [
      new Response(null, { status: 204 }),
      new Response(JSON.stringify(snapshot), { status: 200, headers: { "content-type": "application/json" } }),
      new Response(JSON.stringify({ syncCards: [], resolutions: [] }), { status: 200, headers: { "content-type": "application/json" } }),
    ];
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(async () => responses.shift()!);
    vi.stubGlobal("fetch", fetchMock);
    await new LiveProjectSource([]).setFindingState("prj_fixture", "fnd_synthetic", "dismissed");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/findings/fnd_synthetic/state");
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: "POST", credentials: "include", body: JSON.stringify({ state: "dismissed" }) });
    // A local-only patch would be wiped by the two-second snapshot poll.
    expect(fetchMock.mock.calls.slice(1).map(([url]) => url)).toEqual([
      "/api/v1/dashboard/projects/prj_fixture",
      "/api/v1/projects/prj_fixture/collaboration",
    ]);
  });

  it("uses cookie-authorized Project administration and data-rights routes", async () => {
    const responses = [
      new Response(JSON.stringify({ role: "owner", members: [], devices: [], invites: [] }), { status: 200, headers: { "content-type": "application/json" } }),
      new Response(null, { status: 204 }),
      new Response(null, { status: 202 }),
      new Response(null, { status: 202 }),
    ];
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(async () => responses.shift()!);
    vi.stubGlobal("fetch", fetchMock);
    const source = new LiveProjectSource([]);
    await expect(source.getProjectAccess("prj_fixture")).resolves.toMatchObject({ role: "owner" });
    await source.revokeDevice("prj_fixture", "dev_fixture");
    await source.deleteOwnProjectData("prj_fixture");
    await source.deleteProject("prj_fixture");
    expect(source.exportURL("prj_fixture")).toBe("/api/v1/projects/prj_fixture/export");
    expect(fetchMock.mock.calls.map(([url, init]) => [url, init?.method ?? "GET"])).toEqual([
      ["/api/v1/projects/prj_fixture/access", "GET"],
      ["/api/v1/devices/dev_fixture/revoke", "POST"],
      ["/api/v1/projects/prj_fixture/member", "DELETE"],
      ["/api/v1/projects/prj_fixture", "DELETE"],
    ]);
  });
});
