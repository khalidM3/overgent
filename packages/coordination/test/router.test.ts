import { describe, expect, it } from "vitest";
import { DeterministicCandidateRouter } from "../src/index.js";

describe("DeterministicCandidateRouter", () => {
  it("deduplicates and prioritizes structural evidence deterministically", () => {
    const result = new DeterministicCandidateRouter().route({
      projectId: "prj_fixture",
      repositoryId: "repo_fixture",
      limit: 2,
      structural: [{ objectId: "obj_b", revision: 1, evidence: "structural", score: 0.4 }],
      lexical: [{ objectId: "obj_a", revision: 2, evidence: "lexical", score: 0.9 }],
      semantic: [{ objectId: "obj_b", revision: 1, evidence: "semantic", score: 0.99 }],
    });
    expect(result.map(({ objectId, evidence }) => ({ objectId, evidence }))).toEqual([
      { objectId: "obj_b", evidence: "structural" },
      { objectId: "obj_a", evidence: "lexical" },
    ]);
  });
});
