import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FixtureProjectSource } from "../src/fixture-source";
import { snapshotForProject } from "../src/fixtures";
import { IntelligenceIndicator } from "../src/intelligence-indicator";

function sourceWithoutEnrichment() {
  const snapshot = structuredClone(snapshotForProject("prj_orchard"));
  delete snapshot.project.intelligence;
  return new FixtureProjectSource([snapshot]);
}

describe("IntelligenceIndicator", () => {
  it("loads through one animated bar without adding checking copy", () => {
    const source = sourceWithoutEnrichment();
    vi.spyOn(source, "getProjectIntelligence").mockImplementation(() => new Promise(() => undefined));
    render(<IntelligenceIndicator projectId="prj_orchard" source={source} onConfigure={() => undefined} />);

    expect(screen.getByRole("status", { name: "Loading intelligence layers" })).toBeTruthy();
    expect(screen.queryByText(/checking/i)).toBeNull();
    expect(document.querySelectorAll(".intelligence-meter-loading i")).toHaveLength(1);
  });

  it("falls back to the known built-in layer if configuration cannot be read", async () => {
    const source = sourceWithoutEnrichment();
    vi.spyOn(source, "getProjectIntelligence").mockRejectedValue(new Error("older shell"));
    render(<IntelligenceIndicator projectId="prj_orchard" source={source} onConfigure={() => undefined} />);

    expect(await screen.findByRole("button", { name: "Intelligence layer 2 of 3" })).toBeTruthy();
    expect(screen.queryByText(/unavailable|unknown/i)).toBeNull();
  });

  it("names built-in embeddings and an unconfigured model honestly", async () => {
    const user = userEvent.setup();
    const source = new FixtureProjectSource();
    render(<IntelligenceIndicator projectId="prj_orchard" source={source} onConfigure={() => undefined} />);

    await user.click(screen.getByRole("button", { name: "Intelligence layer 2 of 3" }));
    const detail = screen.getByLabelText("Intelligence layer details");
    expect(detail.textContent).toContain("Built in");
    expect(detail.textContent).toContain("Not set");
    expect(detail.textContent).not.toMatch(/unknown/i);
  });
});
