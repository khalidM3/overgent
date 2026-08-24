import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FixtureProjectSource } from "../src/fixture-source";
import { App } from "../src/main";

const renderReady = () => render(<App initialState="ready" source={new FixtureProjectSource()} />);

describe("dashboard component behavior", () => {
  it("omits all Project data in the unauthorized state", () => {
    render(<App initialState="unauthorized" source={new FixtureProjectSource()} />);
    expect(screen.getByRole("alert").textContent).toContain("not authorized");
    expect(screen.queryByText("Atlas launch")).toBeNull();
    expect(screen.queryByText("stickguy/atlas")).toBeNull();
  });

  it("renders fidelity, structural fallback, large-change, evidence, and advisory states", () => {
    renderReady();
    expect(screen.getByText("MCP reported")).toBeTruthy();
    expect(screen.getByText("Git observed")).toBeTruthy();
    expect(screen.getByText("Manual intent")).toBeTruthy();
    expect(screen.getByText(/Structural findings remain live/)).toBeTruthy();
    expect(screen.getByText("Large change · 1,000 paths")).toBeTruthy();
    expect(screen.getByLabelText("Selected finding detail").textContent).toContain("Advisory only");
  });

  it("keeps Project fixtures isolated when switching", async () => {
    const user = userEvent.setup();
    renderReady();
    expect(screen.getAllByText("Session contract is changing in two workstreams")).toHaveLength(2);
    await user.click(screen.getByRole("button", { name: /Orchard mobile/ }));
    expect(screen.getByRole("heading", { name: "Orchard mobile" })).toBeTruthy();
    expect(screen.queryByText("Session contract is changing in two workstreams")).toBeNull();
    expect(screen.getByText(/Semantic processing is off/)).toBeTruthy();
    expect(screen.getByText("Workspace sharing is paused")).toBeTruthy();
  });

  it("applies pause, live fixture update, and finding lifecycle changes immediately", async () => {
    const user = userEvent.setup();
    renderReady();
    await user.click(screen.getByRole("button", { name: "Pause sharing" }));
    expect(screen.getByText("Workspace sharing is paused")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Resume sharing" }));
    expect(screen.queryByText("Workspace sharing is paused")).toBeNull();
    const observedPaths = screen.getByText("Observed paths").closest("article");
    expect(observedPaths).not.toBeNull();
    expect(within(observedPaths!).getByText("1,009")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Publish fixture update" }));
    expect(within(observedPaths!).getByText("1,010")).toBeTruthy();
    expect(screen.getByText(/Published one new path-only manifest revision/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));
    expect(screen.getByLabelText("Selected finding detail").textContent).toContain("acknowledged");
    await user.click(screen.getByRole("button", { name: "Mark resolved" }));
    expect(screen.getByLabelText("Selected finding detail").textContent).toContain("resolved");
  });

  it("activates a browser session without asking for a ticket value", async () => {
    const user = userEvent.setup();
    render(<App initialState="activation" source={new FixtureProjectSource()} />);
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.getByText(/never stored in this page/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Activate secure session" }));
    expect(screen.getByRole("heading", { name: "Atlas launch" })).toBeTruthy();
  });
});
