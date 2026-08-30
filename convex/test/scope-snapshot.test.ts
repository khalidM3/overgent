import { describe, expect, it } from "vitest";
import { deriveScopeSnapshot, type ScopeVerificationFact } from "../src/scope-snapshot.js";

const passed: ScopeVerificationFact = {
  state: "passed",
  checkKind: "test",
  label: "Dashboard suite",
  summary: "All assertions passed",
  source: "mcp",
  observedAt: "2026-08-30T18:00:00Z",
};

describe("ScopeSnapshot derivation", () => {
  it("applies declared, observed, then fallback precedence without transcript input", () => {
    const snapshot = deriveScopeSnapshot({
      revision: 7,
      workstreamStatus: "active",
      agentStatus: "active",
      vendor: "claude",
      declared: {
        intendedOutcome: "Add revisioned workstream scope.",
        approachSummary: "Project canonical facts into six fields.",
        components: ["dashboard", "hosted projection"],
        contracts: ["ScopeSnapshot"],
        waitingOn: [],
      },
      observed: {
        currentAction: "Editing the dashboard row",
        writes: ["apps/dashboard/src/main.tsx"],
        contractPaths: ["protocol/schemas/scope-snapshot.schema.json"],
        subagents: [{ agentType: "reviewer", status: "done" }],
        verification: [passed],
      },
      fallbackDerivedTitle: "This fallback must not replace a declared goal",
    });

    expect(snapshot).toMatchObject({
      revision: 7,
      state: "implementing",
      goal: { text: "Add revisioned workstream scope.", provenance: "declared", evidenceQuality: "high", facts: ["intent.intendedOutcome"] },
      now: { text: "Project canonical facts into six fields.", provenance: "declared", facts: ["intent.approachSummary"] },
      waitingOn: { text: "Nothing declared.", provenance: "declared", facts: ["intent.waitingOn"] },
      verification: { provenance: "observed", evidenceQuality: "high", facts: ["checkpoint.verification"] },
      scope: { provenance: "declared", facts: ["intent.components", "intent.contracts"] },
    });
    expect(snapshot.done.text).toContain("Writes observed in apps/dashboard/src/main.tsx");
    expect(snapshot.done.text).toContain("Contract fingerprints reported");
    expect(snapshot.goal.text).not.toContain("fallback");
  });

  it("keeps Codex observations visibly thinner and preserves the derived title verbatim", () => {
    const title = "Implement ScopeSnapshot without reading agent history";
    const snapshot = deriveScopeSnapshot({
      revision: 3,
      workstreamStatus: "active",
      agentStatus: "active",
      vendor: "codex",
      observed: {
        currentAction: "Editing protocol schemas",
        writes: ["protocol/schemas/dashboard.schema.json"],
        verification: [passed],
      },
      fallbackDerivedTitle: title,
    });

    expect(snapshot.goal).toEqual({ text: title, provenance: "fallback", evidenceQuality: "low", facts: ["session.derivedTitle"] });
    expect(snapshot.now.evidenceQuality).toBe("medium");
    expect(snapshot.done.evidenceQuality).toBe("medium");
    expect(snapshot.verification.evidenceQuality).toBe("medium");
    expect(snapshot.waitingOn.evidenceQuality).toBe("none");
    expect(snapshot.scope.evidenceQuality).toBe("medium");
  });

  it("uses only honest named states and never emits a percentage", () => {
    const verifying = deriveScopeSnapshot({
      revision: 2,
      workstreamStatus: "active",
      observed: { verification: [{ ...passed, state: "running" }] },
    });
    const waiting = deriveScopeSnapshot({ revision: 3, workstreamStatus: "blocked", agentStatus: "waiting" });
    const complete = deriveScopeSnapshot({ revision: 4, workstreamStatus: "done" });

    expect(verifying.state).toBe("verifying");
    expect(waiting.state).toBe("waiting");
    expect(complete.state).toBe("complete");
    expect(JSON.stringify([verifying, waiting, complete])).not.toMatch(/\d+%/);
  });
});
