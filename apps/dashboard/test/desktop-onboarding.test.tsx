import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DesktopOnboarding } from "../src/desktop-onboarding";
import type { NativeOnboarding, OnboardingState } from "../src/native";

const adapter = { name: "Codex", installed: true, configured: false, fidelity: "Git", detail: "Detected", binding: "not_configured" as const, currentProfile: "test", runtimeVerified: false, restartRequired: false, reconnectAllowed: false, hooksNeedReview: false };
const initial: OnboardingState = { available: true, development: true, enrolled: false, projectId: "", repositoryRoot: "", repositoryLabel: "", deviceLabel: "Test Mac", apiBaseUrl: "https://api.overgent.com", adapters: [adapter], limitation: "", localAvailable: true };
const project = { projectId: "prj_test", repositoryRoot: "/tmp/atlas", repositoryLabel: "atlas", backendId: "bk_local", kind: "local" as const, apiBaseUrl: "http://127.0.0.1:3211", credential: "ok" as const };
const enrolled: OnboardingState = { ...initial, enrolled: true, projectId: project.projectId, repositoryRoot: project.repositoryRoot, repositoryLabel: "atlas", projects: [project], backendId: project.backendId };
function mockAPI(state = initial): NativeOnboarding {
 return { state: vi.fn(async () => state), recheckState: vi.fn(async () => state), chooseRepository: vi.fn(async () => "/tmp/atlas"), createLocalProject: vi.fn(async () => ({ projectId: "prj_test", joinCode: "", warnings: [] })), createProject: vi.fn(async () => ({ projectId: "prj_shared", joinCode: "inv_synthetic.secret", warnings: [] })), createAdditionalProject: vi.fn(), joinProject: vi.fn(async () => ({ projectId: "prj_joined", joinCode: "", warnings: [] })), joinAdditionalProject: vi.fn(async () => ({ projectId: "prj_joined", joinCode: "", warnings: [] })), configureAdapters: vi.fn(), reconnectAdapter: vi.fn(), connectAgentWorktree: vi.fn(), openLiveProject: vi.fn(async (id) => `/?live=1&project=${id}`), resetEnrollment: vi.fn(async () => initial), sessionDetail: vi.fn(), setProjectPaused: vi.fn(), sessionFocus: vi.fn(), setSessionFocus: vi.fn() };
}
beforeEach(() => { localStorage.clear(); window.history.replaceState(null, "", "/"); });
describe("local-first entry", () => {
 it("opens a repository in one form with detected agents and no identity or provider question", async () => {
  const api = mockAPI(), navigate = vi.fn(), user = userEvent.setup(); render(<DesktopOnboarding api={api} navigate={navigate} />);
  await user.click(await screen.findByRole("button", { name: "Choose…" }));
  expect(screen.queryByText(/Step \d/)).toBeNull(); expect(screen.queryByLabelText("Your name")).toBeNull(); expect(screen.queryByLabelText("API key")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Open Project" }));
  await waitFor(() => expect(navigate).toHaveBeenCalledWith("/?live=1&project=prj_test"));
  expect(api.createLocalProject).toHaveBeenCalledWith(expect.objectContaining({ repositoryRoot: "/tmp/atlas", projectLabel: "atlas", enableCodex: true, displayName: "" })); expect(api.createProject).not.toHaveBeenCalled();
 });
 it("does not substitute remote storage when local support is absent", async () => {
  const api = mockAPI({ ...initial, localAvailable: false }), user = userEvent.setup(); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  await user.click(await screen.findByRole("button", { name: "Choose…" })); expect((screen.getByRole("button", { name: "Open Project" }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText(/Local coordination is unavailable/)).toBeTruthy(); expect(api.createProject).not.toHaveBeenCalled();
 });
 it("resumes an enrolled Project without an Open Projects stop", async () => {
  const api = mockAPI(enrolled), navigate = vi.fn(); render(<DesktopOnboarding api={api} navigate={navigate} />);
  await waitFor(() => expect(navigate).toHaveBeenCalledWith("/?live=1&project=prj_test"));
 });
 it("restores only a remembered Project enrolled on this Mac", async () => {
  localStorage.setItem("overgent.last-project", "prj_attacker"); const api = mockAPI(enrolled); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  await waitFor(() => expect(api.openLiveProject).toHaveBeenCalledWith("prj_test"));
 });
 it("opens an already registered repository without enrolling it twice", async () => {
  window.history.replaceState(null, "", "/?add=project"); const api = mockAPI(enrolled), user = userEvent.setup(); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  await user.click(await screen.findByRole("button", { name: "Choose…" })); await user.click(screen.getByRole("button", { name: "Open Project" }));
  expect(api.createLocalProject).not.toHaveBeenCalled(); expect(api.openLiveProject).toHaveBeenCalledWith("prj_test");
 });
 it("joins by invite and reuses the existing enrollment path", async () => {
  window.history.replaceState(null, "", "/?add=project"); const api = mockAPI(enrolled), user = userEvent.setup(); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  await user.click(await screen.findByRole("button", { name: "Join with an invite" })); await user.type(screen.getByLabelText("Invite code"), "https://coord.example/join#inv_synthetic.secret"); await user.click(screen.getByRole("button", { name: "Choose…" })); await user.click(screen.getByRole("button", { name: "Join Project" }));
  expect(api.joinAdditionalProject).toHaveBeenCalledWith(expect.objectContaining({ joinCode: "https://coord.example/join#inv_synthetic.secret" })); expect(api.resetEnrollment).not.toHaveBeenCalled();
 });
 it("keeps optional integration defaults local and respects a disabled agent", async () => {
  localStorage.setItem("overgent.agent.codex", "off"); const api = mockAPI(), user = userEvent.setup(); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  await user.click(await screen.findByRole("button", { name: "Choose…" })); await user.click(screen.getByRole("button", { name: "Open Project" })); expect(api.createLocalProject).toHaveBeenCalledWith(expect.objectContaining({ enableCodex: false }));
 });
 it("never offers credential reset for an uncertain connection", async () => {
  const api = mockAPI({ ...enrolled, credential: "uncertain", projects: [{ ...project, credential: "uncertain" }] }); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  expect(await screen.findByRole("button", { name: "Check connections" })).toBeTruthy(); expect(screen.queryByRole("button", { name: "Reconnect" })).toBeNull(); expect(api.resetEnrollment).not.toHaveBeenCalled();
 });
 it("scopes rejected-credential recovery to one server with a concrete confirmation", async () => {
  const api = mockAPI({ ...enrolled, credential: "revoked", projects: [{ ...project, credential: "revoked" }] }), user = userEvent.setup(); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  await user.click(await screen.findByRole("button", { name: "Reconnect" })); expect(screen.getByText(/removes only this server/)).toBeTruthy(); await user.click(screen.getByRole("button", { name: "Forget this server’s connection" })); expect(api.resetEnrollment).toHaveBeenCalledWith("bk_local");
 });
 it("shows integration trust problems in settings without claiming readiness", async () => {
  window.history.replaceState(null, "", "/?settings=1"); const api = mockAPI({ ...enrolled, adapters: [{ ...adapter, configured: true, hooksNeedReview: true, detail: "Review hooks in Codex before sessions can be observed." }] }); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  const settings = await screen.findByRole("main", { name: "App settings" }); expect(within(settings).getByText(/Review hooks in Codex/)).toBeTruthy(); expect(within(settings).queryByText("Observed session activity")).toBeNull();
 });
 it("returns to the Project that opened Add without trusting a URL destination", async () => {
  window.history.replaceState(null, "", "/?add=project&from=prj_test"); const api = mockAPI(enrolled), user = userEvent.setup(); render(<DesktopOnboarding api={api} navigate={vi.fn()} />);
  await user.click(await screen.findByRole("button", { name: "Back to atlas" })); expect(api.openLiveProject).toHaveBeenCalledWith("prj_test");
 });
});
