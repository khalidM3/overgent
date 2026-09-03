import { describe, expect, it } from "vitest";
import { DeterministicCandidateRouter } from "@overgent/coordination";
import { activationFailureResponse } from "../src/activation.js";
import { semanticFidelity } from "../src/boundary.js";

describe("hosted dependency boundary", () => {
  it("reports structural fidelity when semantic services are absent", () => {
    expect(semanticFidelity({ router: new DeterministicCandidateRouter() })).toBe("structural");
  });
});

describe("dashboard activation failure boundary", () => {
  it("renders an actionable development recovery without echoing sensitive material", async () => {
    const response = activationFailureResponse("ticket_invalid", 409, true);
    const body = await response.text();
    expect(response.status).toBe(409);
    expect(response.headers.get("content-type")).toContain("text/html");
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(body).toContain("This dashboard link is no longer valid.");
    expect(body).toContain("overgent-dev://open");
    expect(body).not.toContain("ticket_invalid");
    expect(body).not.toContain("requestId");
    expect(body).not.toContain("secretHash");
  });

  it("uses the production scheme and safe generic copy for unknown failures", async () => {
    const body = await activationFailureResponse("unexpected_provider_detail", 500, false).text();
    expect(body).toContain("Overgent could not open the live Project.");
    expect(body).toContain("overgent://open");
    expect(body).not.toContain("unexpected_provider_detail");
  });
});
