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

  it("carries the goals a session moved on from, oldest first, without rewording them", () => {
    const snapshot = deriveScopeSnapshot({
      revision: 6,
      workstreamStatus: "active",
      declared: { intendedOutcome: "Rotate credentials after a permission change." },
      priorGoals: [
        { title: "Read how sessions are validated", endedAt: "2026-08-25T09:41:00Z" },
        { title: "Add a rotation helper", intendedOutcome: "Add a rotation helper to the store.", endedAt: "2026-08-25T09:52:00Z" },
      ],
    });

    expect(snapshot.priorGoals.map((goal) => goal.title)).toEqual(["Read how sessions are validated", "Add a rotation helper"]);
    // Passed through, never re-described: these are what the session said at
    // the time, and finished work is not ours to restate.
    expect(snapshot.priorGoals[1]!.intendedOutcome).toBe("Add a rotation helper to the store.");
    expect(snapshot.priorGoalsDropped).toBe(0);
    // The current goal is unaffected by having a history behind it.
    expect(snapshot.goal.text).toBe("Rotate credentials after a permission change.");
    expect(snapshot.goal.provenance).toBe("declared");
  });

  it("reports goals dropped from the bounded history rather than implying the list is whole", () => {
    const snapshot = deriveScopeSnapshot({
      revision: 40,
      workstreamStatus: "active",
      priorGoals: [{ title: "The oldest goal still kept", endedAt: "2026-08-25T10:00:00Z" }],
      priorGoalsDropped: 4,
    });

    expect(snapshot.priorGoals).toHaveLength(1);
    expect(snapshot.priorGoalsDropped).toBe(4);
  });

  it("discards a history entry that names no goal or no end, and never invents one", () => {
    const snapshot = deriveScopeSnapshot({
      revision: 3,
      workstreamStatus: "active",
      priorGoals: [
        { title: "   ", endedAt: "2026-08-25T09:41:00Z" },
        { title: "Kept", endedAt: "  " },
        { title: "Also kept", endedAt: "2026-08-25T09:52:00Z" },
      ],
    });

    expect(snapshot.priorGoals.map((goal) => goal.title)).toEqual(["Also kept"]);
  });

  it("has no history for a session that has only ever had one goal", () => {
    const snapshot = deriveScopeSnapshot({ revision: 1, workstreamStatus: "active" });

    expect(snapshot.priorGoals).toEqual([]);
    expect(snapshot.priorGoalsDropped).toBe(0);
  });
});
