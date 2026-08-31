import { describe, expect, it } from "vitest";

import { findingTitle } from "../src/finding-title.js";

describe("findingTitle", () => {
  it("says what happened to whom instead of naming the finding kind", () => {
    expect(findingTitle({
      kind: "stale_assumption",
      actors: ["Khalid"],
      counterpart: "Mina",
      subject: "Refresh",
    })).toBe("Khalid is building on a version of Refresh that Mina already changed");
  });

  it("never leaks Stickguy's own vocabulary into the sentence", () => {
    const vocabulary = /workstream|stale assumption|redundant work|direct collision|downstream impact|assumption conflict|shared dependency|dependency ready|fingerprint|manifest|provenance/i;
    const kinds = [
      "direct_collision", "likely_collision", "redundant_work", "shared_dependency",
      "assumption_conflict", "downstream_impact", "stale_assumption", "dependency_ready",
    ] as const;

    for (const kind of kinds) {
      const withFacts = findingTitle({ kind, actors: ["Khalid", "Mina"], counterpart: "Mina", subject: "SessionAPI" });
      const withoutFacts = findingTitle({ kind, actors: [] });
      expect(withFacts).not.toMatch(vocabulary);
      expect(withoutFacts).not.toMatch(vocabulary);
      expect(withFacts.length).toBeGreaterThan(0);
      expect(withoutFacts.length).toBeGreaterThan(0);
    }
  });

  it("names both parties when the roles differ, and the subject when it has one", () => {
    expect(findingTitle({ kind: "downstream_impact", actors: ["Khalid"], counterpart: "Andre", subject: "SessionAPI" }))
      .toBe("Khalid's change to SessionAPI affects Andre");
    expect(findingTitle({ kind: "direct_collision", actors: ["Khalid", "Mina"], subject: "backend/refresh.go" }))
      .toBe("Khalid and Mina are both changing backend/refresh.go");
  });

  it("reads as good news when the news is good", () => {
    expect(findingTitle({ kind: "dependency_ready", actors: ["Mina"], subject: "SessionAPI" }))
      .toBe("SessionAPI, which Mina was waiting on, is ready");
  });

  it("degrades to a role noun rather than inventing or repeating a name", () => {
    expect(findingTitle({ kind: "stale_assumption", actors: ["Khalid"], subject: "Refresh" }))
      .toBe("Khalid is building on a version of Refresh that another session already changed");
    // A counterpart equal to the only actor would otherwise read as someone
    // colliding with themselves.
    expect(findingTitle({ kind: "downstream_impact", actors: ["Khalid"], counterpart: "Khalid" }))
      .toBe("Khalid's change affects another session");
    expect(findingTitle({ kind: "direct_collision", actors: [] }))
      .toBe("Two sessions are changing the same file");
  });

  it("joins three or more names so the sentence still reads aloud", () => {
    // "Both" is only true of two; three sessions all share it.
    expect(findingTitle({ kind: "shared_dependency", actors: ["Khalid", "Mina", "Andre"], subject: "the auth schema" }))
      .toBe("Khalid, Mina and Andre all depend on the auth schema");
  });

  it("names one member's two agents as two sessions rather than as a collision with themselves", () => {
    // The product's first case: one person, two agents, one repository.
    expect(findingTitle({ kind: "direct_collision", actors: ["Khalid", "Khalid"], subject: "session.ts" }))
      .toBe("Two of Khalid's sessions are both changing session.ts");
  });

  it("still has two sides when the finding is routed to only one session", () => {
    // shared_dependency reaches only the session that has to act on it.
    expect(findingTitle({ kind: "shared_dependency", actors: ["Khalid"], subject: "the manifest schema" }))
      .toBe("Khalid and another session both depend on the manifest schema");
    expect(findingTitle({ kind: "redundant_work", actors: ["Mina"], counterpart: "Andre" }))
      .toBe("Mina and Andre may be building the same thing");
  });

  it("stays inside the dashboard contract's title bound", () => {
    const title = findingTitle({
      kind: "stale_assumption",
      actors: ["A".repeat(120)],
      counterpart: "B".repeat(120),
      subject: "C".repeat(120),
    });

    expect(title.length).toBeLessThanOrEqual(160);
    expect(title.endsWith("…")).toBe(true);
  });
});
