import { useEffect, useRef, useState } from "react";
import { NewProjectScreen } from "./new-project";
import { Screen, ScreenSection } from "./screen";
import { MacSettings } from "./mac-settings";
import { nativeOnboarding, type NativeOnboarding, type OnboardingState } from "./native";

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

  return <div className="workroom-shell screen-open entry-shell"><nav className="side" aria-label="Projects"><div className="side-top"><Brand /></div><div className="side-scroll"><button className="nav-item" aria-current={page === "projects" ? "page" : undefined} onClick={() => setPage("projects")}>Projects</button>{projects.map((project) => <button className="project-item" key={project.projectId} disabled={pending} onClick={() => void open(project.projectId)}><span className="project-monogram">{project.repositoryLabel.slice(0, 1).toUpperCase()}</span>{project.repositoryLabel}</button>)}</div><button className="profile-button" onClick={() => setPage("settings")}>App settings</button></nav>{content}</div>;
}

function Brand() { return <div className="brand" aria-label="Overgent"><span className="brand-mark" aria-hidden="true">O</span><span>overgent</span></div>; }

export { MacSettings } from "./mac-settings";
