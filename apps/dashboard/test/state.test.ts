import { describe, expect, it } from "vitest";
import { parseShellState, snapshotForProject } from "../src/fixtures";
import { fidelityLabel, semanticMessage, stateMessage } from "../src/state";

describe("dashboard state contracts", () => {
  it("does not present offline data as current", () => {
    expect(stateMessage("offline")).toContain("last synchronized revision");
  });

  it("defaults unknown URL state to ready", () => {
    expect(parseShellState("?state=made_up")).toBe("ready");
    expect(parseShellState("?state=unauthorized")).toBe("unauthorized");
  });

  it("labels lower-fidelity sources and semantic degradation honestly", () => {
    expect(fidelityLabel("manual")).toBe("Manual intent");
    expect(fidelityLabel("hook_unverified")).toBe("Hook unverified");
    expect(semanticMessage("degraded")).toContain("Structural findings remain live");
    expect(semanticMessage("disabled")).toContain("Structural findings remain available");
  });

  it("rejects projects outside the authorized fixture session", () => {
    expect(() => snapshotForProject("prj_foreign")).toThrow(/not authorized/);
  });
});
