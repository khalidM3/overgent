import { describe, expect, it } from "vitest";
import { stateMessage } from "../src/state";

describe("dashboard states", () => {
  it("does not present offline data as current", () => {
    expect(stateMessage("offline")).toContain("last synchronized revision");
  });
});
