import { afterEach, describe, expect, it, vi } from "vitest";
import { loadSession } from "../src/live-source";

afterEach(() => vi.unstubAllGlobals());

describe("live dashboard transport", () => {
  it("loads the authorized session with the HTTP-only cookie boundary", async () => {
    const session = { memberName: "Synthetic member", projects: [], selectedProjectId: "" };
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(async () => new Response(JSON.stringify(session), { status: 200, headers: { "content-type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(loadSession()).resolves.toEqual(session);
    expect(fetchMock.mock.calls[0]?.[1]?.credentials).toBe("include");
  });
});
