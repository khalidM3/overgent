import { useEffect, useRef, useState } from "react";
import { NewProjectScreen } from "./new-project";
import { Screen, ScreenSection } from "./screen";
import { AIDefaultsSettings } from "./ai-defaults";
import { nativeOnboarding, type NativeOnboarding, type OnboardingState } from "./native";
import type { AgentVendor } from "./model";

const lastProjectKey = "overgent.last-project";
export function rememberProject(id: string) { try { localStorage.setItem(lastProjectKey, id); } catch { /* Storage is optional. */ } }
function rememberedProject(): string { try { return localStorage.getItem(lastProjectKey) ?? ""; } catch { return ""; } }

/** The entry route restores work. The library is a destination and recovery, not onboarding. */
export function DesktopOnboarding({ api = nativeOnboarding, navigate = (url) => window.location.assign(url) }: { api?: NativeOnboarding; navigate?: (url: string) => void }) {
  const query = new URLSearchParams(window.location.search);
  const [page, setPage] = useState(query.get("settings") ? "settings" : query.get("add") ? "add" : "resume");
  const [mode, setMode] = useState<"create" | "join">("create");
  const [placement, setPlacement] = useState<"local" | "team">("local");
  const [state, setState] = useState<OnboardingState | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [confirmReset, setConfirmReset] = useState(false);
  const resumed = useRef(false);
  const refresh = async () => { const next = await api.state(); setState(next); return next; };
  const open = async (id: string) => {
    setPending(true); setError("");
    try { const url = await api.openLiveProject(id); rememberProject(id); navigate(url); }
    catch (cause) { setError((cause as Error).message); setPage("projects"); }
    finally { setPending(false); }
  };
  useEffect(() => { let active = true; void api.state().then((next) => { if (active) setState(next); }).catch((cause: Error) => { if (active) setError(cause.message); }); return () => { active = false; }; }, [api]);
  useEffect(() => {
    if (!state || page !== "resume" || resumed.current) return;
    resumed.current = true;
    if (!state.enrolled) { setPage("add"); return; }
    const selected = state.projects?.find((project) => project.projectId === rememberedProject()) ?? state.projects?.find((project) => project.projectId === state.projectId);
    if (selected?.credential === "revoked" || selected?.credential === "unknown" || selected?.credential === "uncertain") { setPage("projects"); return; }
    void open(selected?.projectId ?? state.projectId);
  }, [state, page]);
  const projects = state?.projects ?? [];
  const returnProject = projects.find((project) => project.projectId === query.get("from"));
  const back = () => { if (returnProject) void open(returnProject.projectId); else setPage("projects"); };

  if (!state) return <main className="onboarding-shell"><header><Brand /></header><section className="onboarding-card"><h1>{error ? "Overgent couldn’t open." : "Opening Overgent…"}</h1>{error && <><p role="alert" className="form-error">{error}</p><button className="pill" onClick={() => void refresh().catch((cause: Error) => setError(cause.message))}>Try again</button></>}</section></main>;

  const content = page === "settings"
    ? <MacSettings api={api} state={state} projectId={returnProject?.projectId ?? state.projectId} onBack={back} refresh={refresh} />
    : page === "add" || !state.enrolled
      ? <NewProjectScreen api={api} displayName={state.memberName ?? ""} navigate={(url) => navigate(url)} mode={mode} onMode={setMode} placement={placement} onPlacement={setPlacement} localAvailable={Boolean(state.localAvailable)} defaultServer="https://api.overgent.com" backLabel={returnProject?.repositoryLabel ?? "Projects"} onBack={back} />
      : <Screen title={pending ? "Opening your Project…" : "Projects"} backLabel="Overgent" onBack={() => setPage("projects")} lede="Your repositories, ready when you are.">
          <div className="project-list" aria-label="Projects on this Mac">{projects.map((project) => <button className="project-row" key={project.projectId} disabled={pending} onClick={() => void open(project.projectId)}><span><strong>{project.repositoryLabel}</strong><small>{project.kind === "local" ? "On this Mac" : "Shared"}{project.credential && project.credential !== "ok" ? " · connection needs attention" : ""}</small></span><span aria-hidden="true">→</span></button>)}</div>
          {error && <p role="alert" className="form-error">{error}</p>}
          {(state.credential === "revoked" || state.credential === "unknown") && <ScreenSection title="Reconnect this server" help="Access to this server was rejected. Your repositories and Projects on other servers are unaffected.">
            {confirmReset && <p className="settings-help">This removes only this server’s credential and local registrations. You will need a new invite to join again. Repository files remain untouched.</p>}
            <button className="pill" disabled={pending} onClick={() => {
              if (!confirmReset) { setConfirmReset(true); return; }
              setPending(true); void api.resetEnrollment(state.backendId ?? "").then(setState).then(() => setConfirmReset(false)).catch((cause: Error) => setError(cause.message)).finally(() => setPending(false));
            }}>{confirmReset ? "Forget this server’s connection" : "Reconnect"}</button>
          </ScreenSection>}
          <div className="screen-actions"><button className="pill solid" onClick={() => { setMode("create"); setPage("add"); }}>Open a repository</button><button className="pill" onClick={() => { setMode("join"); setPage("add"); }}>Join with an invite</button><button className="text-button" onClick={() => void (api.recheckState ?? api.state)().then(setState).catch((cause: Error) => setError(cause.message))}>Check connections</button></div>
        </Screen>;

  return <div className="workroom-shell screen-open entry-shell"><aside className="side"><div className="side-top"><Brand /></div><div className="side-scroll"><button className="nav-item" aria-current={page === "projects" ? "page" : undefined} onClick={() => setPage("projects")}>Projects</button>{projects.map((project) => <button className="project-item" key={project.projectId} disabled={pending} onClick={() => void open(project.projectId)}><span className="project-monogram">{project.repositoryLabel.slice(0, 1).toUpperCase()}</span>{project.repositoryLabel}</button>)}</div><button className="profile-button" onClick={() => setPage("settings")}>App settings</button></aside>{content}</div>;
}

export function MacSettings({ api, state, projectId, onBack, refresh, dark = false, onTheme }: { api: NativeOnboarding; state: OnboardingState; projectId: string; onBack: () => void; refresh: () => Promise<unknown>; dark?: boolean; onTheme?: () => void }) {
  const [tab, setTab] = useState("integrations");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [reconnect, setReconnect] = useState<AgentVendor | null>(null);
  const project = state.projects?.find((entry) => entry.projectId === projectId);
  const root = project?.repositoryRoot ?? state.repositoryRoot;
  const run = async (operation: () => Promise<unknown>) => { setPending(true); setError(""); try { await operation(); await refresh(); } catch (cause) { setError((cause as Error).message); } finally { setPending(false); } };
  return <Screen title="App settings" backLabel={project?.repositoryLabel ?? "Projects"} onBack={onBack} lede="Preferences for this Mac. Project access and sharing live in Project settings.">
    <nav className="settings-tabs" aria-label="App settings sections">{["appearance", "integrations", "intelligence", "advanced"].map((name) => <button key={name} className="text-button" aria-current={tab === name ? "page" : undefined} onClick={() => setTab(name)}>{name[0]!.toUpperCase() + name.slice(1)}</button>)}</nav>
    {/* Intelligence is a preference for this Mac, so it sits here beside the
        other two. What a given Project actually runs stays on that Project's
        own settings screen, where the sharing consequences are stated. */}
    {tab === "intelligence" && (api.aiDefaults && api.putAIDefaults
      ? <AIDefaultsSettings api={{ aiDefaults: api.aiDefaults, putAIDefaults: api.putAIDefaults }} />
      : <ScreenSection title="Defaults for new Projects"><p className="settings-help">This version of the Overgent app cannot store defaults yet. Each Project’s intelligence settings are on its own settings screen.</p></ScreenSection>)}
    {tab === "integrations" && <ScreenSection title="Coding agents" help="Choose which detected agents connect when you open a repository. Only registered repositories are observed; agents keep their own coding permissions.">
      {state.adapters.map((adapter) => {
        const vendor = ({ Codex: "codex", "Claude Code": "claude", Cursor: "cursor" } as const)[adapter.name as "Codex" | "Claude Code" | "Cursor"] ?? "codex";
        return <div className="settings-row" key={adapter.name}><span><strong>{adapter.name}</strong><small>{adapter.runtimeVerified ? "Observed session activity" : adapter.detail || (adapter.installed ? "Detected on this Mac" : "Not detected")}</small><label className="inline-option"><input type="checkbox" defaultChecked={readAgentPreference(vendor)} onChange={(event) => { try { localStorage.setItem(`overgent.agent.${vendor}`, event.target.checked ? "on" : "off"); } catch { setError("This window could not save the preference."); } }} />Connect in new Projects</label></span>
          {root && adapter.installed && !adapter.configured && !adapter.reconnectAllowed && adapter.binding !== "drifted" && <button className="pill" disabled={pending} onClick={() => void run(() => api.configureAdapters(root, vendor === "codex", vendor === "claude", vendor === "cursor"))}>Connect</button>}
          {adapter.configured && api.disconnectAgent && <button className="pill" disabled={pending} onClick={() => void run(async () => { await api.disconnectAgent!(vendor); try { localStorage.setItem(`overgent.agent.${vendor}`, "off"); } catch { /* Storage is optional. */ } })}>Disconnect on this Mac</button>}
          {adapter.reconnectAllowed && <button className="pill" onClick={() => setReconnect(vendor)}>Reconnect</button>}
        </div>;
      })}
      {reconnect && <div className="reconnect-preview"><h3>Reconnect {reconnect}?</h3><p>This replaces Overgent’s managed connection to another profile. Unrelated agent settings are preserved.</p><button className="pill" onClick={() => setReconnect(null)}>Cancel</button><button className="pill solid" disabled={pending} onClick={() => void run(async () => { await api.reconnectAdapter(root, reconnect); setReconnect(null); })}>Reconnect to this Project</button></div>}
    </ScreenSection>}
    {tab === "appearance" && <ScreenSection title="Appearance"><button className="pill" onClick={() => { if (onTheme) onTheme(); else document.documentElement.dataset.theme = document.documentElement.dataset.theme === "dark" ? "light" : "dark"; }}>{dark ? "Use light appearance" : "Change appearance"}</button></ScreenSection>}
    {tab === "advanced" && <ScreenSection title="Local service"><p className="settings-help">{state.backend?.running ? "Local coordination is running." : "Local coordination starts when you open a repository."}</p>{state.backend?.lastError && <p className="form-warning">{state.backend.lastError}</p>}<p className="settings-help">Self-hosted server addresses are selected when creating a shared Project. They do not change your local Projects.</p></ScreenSection>}
    {error && <p role="alert" className="form-error">{error}</p>}
  </Screen>;
}
function readAgentPreference(vendor: string) { try { return localStorage.getItem(`overgent.agent.${vendor}`) !== "off"; } catch { return true; } }
function Brand() { return <div className="brand" aria-label="Overgent"><span className="brand-mark" aria-hidden="true">O</span><span>overgent</span></div>; }
