import { describe, expect, it } from "vitest";
import { DeterministicCandidateRouter } from "@stickguy/coordination";
import { semanticFidelity } from "../src/boundary.js";

describe("hosted dependency boundary", () => {
  it("reports structural fidelity when semantic services are absent", () => {
    expect(semanticFidelity({ router: new DeterministicCandidateRouter() })).toBe("structural");
  });
});
