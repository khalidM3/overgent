import { afterEach, describe, expect, it, vi } from "vitest";
import { LiveProjectSource, loadSession } from "../src/live-source";

afterEach(() => vi.unstubAllGlobals());

describe("live dashboard transport", () => {
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
});
