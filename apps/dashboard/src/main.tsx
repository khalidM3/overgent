import { StrictMode, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { createRoot } from "react-dom/client";
import { FixtureProjectSource } from "./fixture-source";
import { emptyFixtureSession, fixtureSession, parseShellState } from "./fixtures";
import { LiveProjectSource, loadSession, loadSnapshot } from "./live-source";
import type { DashboardSession, Finding, FindingState, ProjectSnapshot, ShellState } from "./model";
import { fidelityLabel, semanticMessage, stateMessage } from "./state";
import "./style.css";

const defaultSource = new FixtureProjectSource();

interface AppProps {
  initialState?: ShellState;
  initialSession?: DashboardSession;
  source?: FixtureProjectSource;
}

export function App({
  initialState = parseShellState(window.location.search),
  initialSession = initialState === "empty" ? emptyFixtureSession() : fixtureSession,
  source = defaultSource,
}: AppProps) {
  const [shellState, setShellState] = useState(initialState);
  if (shellState === "activation") return <ActivationView onActivate={() => setShellState("ready")} />;
  if (shellState === "loading") return <LoadingView />;
  if (shellState === "unauthorized" || shellState === "version_mismatch") return <TerminalState state={shellState} />;
  if (shellState === "empty" || initialSession.projects.length === 0) return <EmptyView />;
  return <Dashboard session={initialSession} source={source} offline={shellState === "offline"} />;
}

function Brand() {
  return <div className="brand" aria-label="Stickguy"><span className="brand-mark" aria-hidden="true">S</span><span>stickguy</span></div>;
}

function ActivationView({ onActivate }: { onActivate: () => void }) {
  return <main className="centered-shell"><Brand /><section className="activation-card" aria-labelledby="activation-title"><p className="eyebrow">Browser activation</p><h1 id="activation-title">Bring the live Project view to this browser.</h1><p className="lede">Your one-time dashboard ticket is exchanged server-side. It is never stored in this page, activity, or browser history.</p><div className="disclosure"><strong>What the Project shares</strong><p>Workstream intent, path and dependency metadata, findings, decisions, presence, and fidelity labels. Never source, diffs, prompts, transcripts, environment values, or raw test output.</p></div><button className="primary-button" onClick={onActivate}>Activate secure session</button><p className="microcopy">Session cookies are secure, HTTP-only, same-site, revocable, and rotated after privilege changes.</p></section></main>;
}

function LiveApp() {
  const [state, setState] = useState<"activation" | "loading" | "ready" | "unauthorized" | "version_mismatch" | "offline">("loading");
  const [session, setSession] = useState<DashboardSession | null>(null);
  const [source, setSource] = useState<LiveProjectSource | null>(null);

  const load = async () => {
    setState("loading");
    try {
      const nextSession = await loadSession();
      if (nextSession.projects.length === 0) { setSession(nextSession); setState("ready"); return; }
      const snapshots = await Promise.all(nextSession.projects.map((project) => loadSnapshot(project.id)));
      setSession(nextSession);
      setSource(new LiveProjectSource(snapshots, setState));
      setState("ready");
    } catch (error) {
      const status = (error as { status?: number }).status;
      setState(status === 401 || status === 403 ? "activation" : status === 409 ? "version_mismatch" : "offline");
    }
  };
  useEffect(() => { void load(); }, []);
  useEffect(() => {
    if (!source || !session) return;
    return source.start(session.selectedProjectId);
  }, [source, session]);

  if (state === "loading") return <LoadingView />;
  if (state === "version_mismatch") return <TerminalState state="version_mismatch" />;
  if (state === "activation") return <ActivationView onActivate={() => void load()} />;
  if (state === "unauthorized") return <TerminalState state="unauthorized" />;
  if (!session || session.projects.length === 0) return <EmptyView />;
  if (!source) return <TerminalState state="unauthorized" />;
  return <Dashboard session={session} source={source} offline={state === "offline"} />;
}

function LoadingView() {
  return <main className="centered-shell"><Brand /><section className="state-card" role="status" aria-live="polite"><span className="spinner" aria-hidden="true" /><p className="eyebrow">Connecting</p><h1>{stateMessage("loading")}</h1><p>Authorizing membership and loading the current Project revision.</p></section></main>;
}

function TerminalState({ state }: { state: "unauthorized" | "version_mismatch" }) {
  const isVersion = state === "version_mismatch";
  return <main className="centered-shell"><Brand /><section className="state-card" role="alert"><span className="state-icon" aria-hidden="true">{isVersion ? "↥" : "×"}</span><p className="eyebrow">{isVersion ? "Version mismatch" : "Access denied"}</p><h1>{stateMessage(state)}</h1><p>{isVersion ? "This dashboard cannot safely interpret the service contract. Update the Stickguy executable, then reload." : "No Project metadata was loaded. Ask a Project owner to restore membership or enroll this device again."}</p><button className="secondary-button" onClick={() => window.location.reload()}>{isVersion ? "Check again" : "Retry authorization"}</button></section></main>;
}

function EmptyView() {
  return <main className="centered-shell"><Brand /><section className="state-card"><span className="state-icon" aria-hidden="true">＋</span><p className="eyebrow">No Projects</p><h1>{stateMessage("empty")}</h1><p>Create a Project from the Stickguy CLI or join with an expiring invite. This browser cannot create membership on its own.</p><code>stickguy create</code></section></main>;
}

function Dashboard({ session, source, offline }: { session: DashboardSession; source: FixtureProjectSource; offline: boolean }) {
  const [projectId, setProjectId] = useState(session.selectedProjectId);
  const [selectedFindingId, setSelectedFindingId] = useState<string | null>(null);
  const [devicesOpen, setDevicesOpen] = useState(false);
  const snapshot = useProjectSnapshot(source, projectId);
  const selectedFinding = snapshot.findings.find((finding) => finding.id === selectedFindingId) ?? snapshot.findings[0] ?? null;
  const selectProject = (nextId: string) => { setProjectId(nextId); setSelectedFindingId(null); };

  return <div className="app-shell">
    <aside className="sidebar"><Brand /><nav aria-label="Projects" className="project-nav"><p className="nav-label">Projects</p>{session.projects.map((project) => <button key={project.id} className={project.id === projectId ? "project-link active" : "project-link"} aria-current={project.id === projectId ? "page" : undefined} onClick={() => selectProject(project.id)}><span className="project-dot" aria-hidden="true" /><span><strong>{project.name}</strong><small>{project.repositoryLabel}</small></span></button>)}</nav><div className="sidebar-footer"><button className="profile-button" onClick={() => setDevicesOpen(true)} aria-haspopup="dialog" aria-label="Open devices and privacy"><span className="avatar">KH</span><span><strong>{session.memberName}</strong><small>Devices & privacy</small></span></button></div></aside>
    <main className="dashboard-main">
      {offline && <div className="offline-banner" role="status"><strong>Offline</strong><span>Showing revision {snapshot.contextRevision}, synchronized {snapshot.synchronizedAt}. Actions are unavailable until reconnection.</span></div>}
      <header className="topbar"><div><p className="eyebrow">Live Project</p><h1>{snapshot.project.name}</h1><p className="repo-label">{snapshot.project.repositoryLabel} · revision {snapshot.contextRevision}</p></div><div className="topbar-actions"><span className={offline ? "sync-status offline" : "sync-status"}><span aria-hidden="true" />{offline ? "Offline" : `Live · ${snapshot.synchronizedAt}`}</span><button className={snapshot.workspacePaused ? "pause-button paused" : "pause-button"} disabled={offline || source.live} title={source.live ? "Use the local Stickguy CLI for immediate pause" : undefined} onClick={() => source.togglePause(projectId)}>{source.live ? "Pause from CLI" : snapshot.workspacePaused ? "Resume sharing" : "Pause sharing"}</button></div></header>
      <p className="sr-only" aria-live="polite">{snapshot.workspacePaused ? "Workspace sharing is paused." : "Workspace sharing is active."}</p>
      {snapshot.workspacePaused && <section className="pause-banner" role="status"><span aria-hidden="true">Ⅱ</span><div><strong>Workspace sharing is paused</strong><p>Activity payload transmission stopped before this state was shown. Minimal connection health remains visible.</p></div></section>}
      <SemanticBanner status={snapshot.project.semanticStatus} />
      <section className="summary-grid" aria-label="Project summary"><SummaryStat label="Active workstreams" value={String(snapshot.workstreams.filter((w) => w.presence !== "offline").length)} note={`${snapshot.workstreams.length} total`} /><SummaryStat label="Open findings" value={String(snapshot.findings.filter((f) => f.state === "open").length)} note={`${snapshot.findings.length} in radar`} tone="alert" /><SummaryStat label="Observed paths" value={snapshot.workstreams.reduce((sum, w) => sum + w.pathCount, 0).toLocaleString()} note="metadata only" /><SummaryStat label="Devices" value={String(snapshot.devices.length)} note={`${snapshot.devices.filter((d) => d.status === "online").length} online`} /></section>
      <section className="content-grid"><div className="primary-column"><section className="panel" aria-labelledby="workstreams-title"><PanelHeader eyebrow="Team now" title="Workstreams" action={source.live ? undefined : <button className="text-button" disabled={offline} onClick={() => source.publishSyntheticUpdate(projectId)}>Publish fixture update</button>} id="workstreams-title" /><div className="workstream-list">{snapshot.workstreams.map((workstream) => <WorkstreamCard key={workstream.id} workstream={workstream} />)}</div></section><section className="panel activity-panel" aria-labelledby="activity-title"><PanelHeader eyebrow="Structured history" title="Recent activity" id="activity-title" /><ol className="activity-list">{snapshot.activity.map((item) => <li key={item.id}><span className={`activity-mark ${item.kind}`} aria-hidden="true" /><div><p><strong>{item.actor}</strong> {item.summary}</p><span>{item.at} · {item.fidelity}</span></div></li>)}</ol></section></div>
        <section className="panel radar-panel" aria-labelledby="radar-title"><PanelHeader eyebrow="Evidence, not alarms" title="Finding radar" id="radar-title" />{snapshot.findings.length === 0 ? <div className="panel-empty"><span aria-hidden="true">✓</span><strong>No findings for this Project</strong><p>Structural observation continues. Semantic processing is {snapshot.project.semanticStatus}.</p></div> : <><ul className="finding-tabs" aria-label="Findings">{snapshot.findings.map((finding) => <FindingButton key={finding.id} finding={finding} selected={finding.id === selectedFinding?.id} onClick={() => setSelectedFindingId(finding.id)} />)}</ul>{selectedFinding && <FindingDetail finding={selectedFinding} workstreams={snapshot.workstreams} disabled={offline} onState={(state) => source.setFindingState(projectId, selectedFinding.id, state)} />}</>}</section>
      </section>
    </main>{devicesOpen && <DeviceDialog snapshot={snapshot} onClose={() => setDevicesOpen(false)} />}
  </div>;
}

function useProjectSnapshot(source: FixtureProjectSource, projectId: string): ProjectSnapshot {
  return useSyncExternalStore((listener) => source.subscribe(projectId, listener), () => source.get(projectId), () => source.get(projectId));
}

function SemanticBanner({ status }: { status: ProjectSnapshot["project"]["semanticStatus"] }) {
  return <section className={`semantic-banner ${status}`} aria-label="Semantic processing status"><span className="semantic-icon" aria-hidden="true">{status === "enabled" ? "◎" : status === "degraded" ? "◐" : "○"}</span><div><strong>Semantic coordination {status}</strong><p>{semanticMessage(status)}</p></div></section>;
}

function SummaryStat({ label, value, note, tone }: { label: string; value: string; note: string; tone?: "alert" }) {
  return <article className={tone === "alert" ? "summary-stat alert" : "summary-stat"}><span>{label}</span><strong>{value}</strong><small>{note}</small></article>;
}

function PanelHeader({ eyebrow, title, id, action }: { eyebrow: string; title: string; id: string; action?: React.ReactNode }) {
  return <header className="panel-header"><div><p className="eyebrow">{eyebrow}</p><h2 id={id}>{title}</h2></div>{action}</header>;
}

function WorkstreamCard({ workstream }: { workstream: ProjectSnapshot["workstreams"][number] }) {
  return <article className="workstream-card"><div className="member-column"><span className="avatar">{workstream.initials}</span><span className={`presence-dot ${workstream.presence}`} aria-label={`${workstream.presence} presence`} /></div><div className="workstream-body"><div className="workstream-heading"><div><h3>{workstream.title}</h3><p>{workstream.memberName} · updated {workstream.updatedLabel}</p></div><span className={`fidelity-chip ${workstream.fidelity}`}>{fidelityLabel(workstream.fidelity)}</span></div><p className="outcome">{workstream.outcome}</p><div className="path-row"><span>{workstream.pathCount.toLocaleString()} paths</span>{workstream.paths.map((path) => <code key={path}>{path}</code>)}</div>{workstream.largeChange && <div className="large-change"><span aria-hidden="true">↗</span><div><strong>Large change · {workstream.largeChange.pathCount.toLocaleString()} paths</strong><p>{workstream.largeChange.summary}</p><small>Manifest revision {workstream.largeChange.revision} · broad size does not imply severity</small></div></div>}</div></article>;
}

function FindingButton({ finding, selected, onClick }: { finding: Finding; selected: boolean; onClick: () => void }) {
  return <li><button className={selected ? "finding-button selected" : "finding-button"} aria-pressed={selected} onClick={onClick}><span className={`severity-dot ${finding.severity}`} aria-hidden="true" /><span><strong>{finding.title}</strong><small>{finding.kind.replaceAll("_", " ")} · {finding.state}</small></span><span aria-hidden="true">›</span></button></li>;
}

function FindingDetail({ finding, workstreams, disabled, onState }: { finding: Finding; workstreams: ProjectSnapshot["workstreams"]; disabled: boolean; onState: (state: FindingState) => void }) {
  const affected = useMemo(() => workstreams.filter((workstream) => finding.workstreamIds.includes(workstream.id)), [finding.workstreamIds, workstreams]);
  return <article className="finding-detail" aria-label="Selected finding detail"><div className="finding-meta"><span className={`severity-badge ${finding.severity}`}>{finding.severity}</span><span>{finding.confidence} confidence</span><span>{finding.state}</span></div><h3>{finding.title}</h3><p>{finding.reason}</p><dl className="finding-times"><div><dt>First seen</dt><dd>{finding.firstSeen}</dd></div><div><dt>Last changed</dt><dd>{finding.lastSeen}</dd></div></dl><div className="affected"><h4>Affected workstreams</h4>{affected.map((workstream) => <span key={workstream.id}>{workstream.memberName} · {workstream.title}</span>)}</div><div className="evidence-list"><h4>Evidence</h4>{finding.evidence.map((evidence) => <div key={`${evidence.kind}-${evidence.label}`}><span className="evidence-kind">{evidence.kind}</span><code>{evidence.label}</code><small>{evidence.source.replaceAll("_", " ")}</small></div>)}</div><p className="advisory-note">Advisory only. Stickguy does not block writes or control an agent.</p><div className="finding-actions"><button disabled={disabled || finding.state === "acknowledged"} className="secondary-button" onClick={() => onState("acknowledged")}>Acknowledge</button><button disabled={disabled || finding.state === "resolved"} className="primary-button" onClick={() => onState("resolved")}>Mark resolved</button></div></article>;
}

function DeviceDialog({ snapshot, onClose }: { snapshot: ProjectSnapshot; onClose: () => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    dialogRef.current?.showModal();
    closeRef.current?.focus();
    return () => { if (dialogRef.current?.open) dialogRef.current.close(); };
  }, [onClose]);
  return <dialog ref={dialogRef} className="device-dialog" aria-labelledby="devices-title" aria-describedby="devices-description" onCancel={(event) => { event.preventDefault(); onClose(); }}><header><div><p className="eyebrow">Current Project</p><h2 id="devices-title">Devices & privacy</h2></div><button ref={closeRef} className="icon-button" onClick={onClose} aria-label="Close devices and privacy">×</button></header><p id="devices-description" className="dialog-copy">Only disclosed coordination metadata is synchronized. Revoke and clear controls require the hosted device boundary and are shown here as entry points.</p><ul>{snapshot.devices.map((device) => <li key={device.id}><span className={`device-icon ${device.status}`} aria-hidden="true">◇</span><div><strong>{device.label}</strong><small>{device.platform} · {device.status} · {device.lastSeen}</small></div><button className="text-button" disabled>Manage</button></li>)}</ul><div className="privacy-note"><strong>Not collected in V1</strong><p>Source, diffs, Git objects, prompts, transcripts, environment values, and raw command or test output.</p></div><button className="secondary-button" onClick={onClose}>Done</button></dialog>;
}

export function DesktopPreviewBanner() {
  return <div className="desktop-preview-banner" role="status"><strong>Desktop preview</strong><span>Fixture data · local service controls are available from the menu bar</span></div>;
}

const root = document.getElementById("root");
if (root) {
  const parameters = new URLSearchParams(window.location.search);
  const desktopPreview = parameters.get("desktop") === "preview" || window.location.protocol === "wails:";
  if (desktopPreview) document.documentElement.dataset.desktopPreview = "true";
  createRoot(root).render(<StrictMode>
    {desktopPreview && <DesktopPreviewBanner />}
    {parameters.get("live") === "1" ? <LiveApp /> : <App />}
  </StrictMode>);
}
