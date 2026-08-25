import { StrictMode, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { createRoot } from "react-dom/client";
import {
  Activity,
  AlertTriangle,
  Bot,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleDot,
  Code2,
  Command,
  FileCode2,
  GitBranch,
  Laptop2,
  Moon,
  Pause,
  Play,
  Search,
  Settings2,
  ShieldCheck,
  Sun,
  Users,
  X,
} from "lucide-react";
import { FixtureProjectSource } from "./fixture-source";
import { emptyFixtureSession, fixtureSession, parseShellState } from "./fixtures";
import { LiveProjectSource, loadSession, loadSnapshot } from "./live-source";
import { DesktopOnboarding } from "./desktop-onboarding";
import type { DashboardSession, Finding, FindingFeedback, FindingState, ProjectSnapshot, ShellState, Workstream } from "./model";
import { fidelityLabel, semanticMessage, stateMessage } from "./state";
import "./style.css";

const defaultSource = new FixtureProjectSource();

interface AppProps {
  initialState?: ShellState;
  initialSession?: DashboardSession;
  source?: FixtureProjectSource;
}

type Selection = { kind: "session"; id: string } | { kind: "collision"; id: string };

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
  return <ProjectWorkroom session={initialSession} source={source} offline={shellState === "offline"} />;
}

function Brand({ compact = false }: { compact?: boolean }) {
  return <div className={compact ? "brand compact" : "brand"} aria-label="Stickguy"><span className="brand-mark" aria-hidden="true">S</span>{!compact && <span>stickguy</span>}</div>;
}

function ActivationView({ onActivate }: { onActivate: () => void }) {
  return <main className="centered-shell"><Brand /><section className="activation-card" aria-labelledby="activation-title"><span className="state-symbol"><ShieldCheck size={21} /></span><p className="eyebrow">Browser activation</p><h1 id="activation-title">Open your shared Project workroom.</h1><p className="lede">Your one-time access ticket is exchanged server-side. It is never stored in this page, activity, or browser history.</p><div className="disclosure"><strong>What teammates can see</strong><p>Session presence, action categories, safe repository paths, collisions, and coordination decisions. Never source, diffs, prompts, transcripts, environment values, or raw tool output.</p></div><button className="primary-button" onClick={onActivate}>Activate secure session</button><p className="microcopy">Sessions are revocable, same-site, and rotated after privilege changes.</p></section></main>;
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
  return <ProjectWorkroom session={session} source={source} offline={state === "offline"} />;
}

function LoadingView() {
  return <main className="centered-shell"><Brand /><section className="state-card" role="status" aria-live="polite"><span className="spinner" aria-hidden="true" /><p className="eyebrow">Connecting</p><h1>{stateMessage("loading")}</h1><p>Authorizing membership and opening the current Project.</p></section></main>;
}

function TerminalState({ state }: { state: "unauthorized" | "version_mismatch" }) {
  const isVersion = state === "version_mismatch";
  return <main className="centered-shell"><Brand /><section className="state-card" role="alert"><span className="state-symbol"><AlertTriangle size={21} /></span><p className="eyebrow">{isVersion ? "Version mismatch" : "Access denied"}</p><h1>{stateMessage(state)}</h1><p>{isVersion ? "This app cannot safely interpret the service contract. Update the Stickguy executable, then reload." : "No Project metadata was loaded. Ask a Project owner to restore membership or enroll this device again."}</p><button className="secondary-button" onClick={() => window.location.reload()}>{isVersion ? "Check again" : "Retry authorization"}</button></section></main>;
}

function EmptyView() {
  return <main className="centered-shell"><Brand /><section className="state-card"><span className="state-symbol"><GitBranch size={21} /></span><p className="eyebrow">No Projects</p><h1>{stateMessage("empty")}</h1><p>Connect a repository from Stickguy Dev.app or join an existing Project with an invite.</p><code>stickguy create</code></section></main>;
}

function ProjectWorkroom({ session, source, offline }: { session: DashboardSession; source: FixtureProjectSource; offline: boolean }) {
  const [projectId, setProjectId] = useState(session.selectedProjectId);
  const [selection, setSelection] = useState<Selection | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [commandOpen, setCommandOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [dark, setDark] = useState(false);
  const snapshot = useProjectSnapshot(source, projectId);
  const groups = useMemo(() => groupByMember(snapshot.workstreams), [snapshot.workstreams]);
  const defaultSession = snapshot.workstreams.find((stream) => stream.agent?.status === "active") ?? snapshot.workstreams[0];
  const effectiveSelection: Selection | null = selection ?? (defaultSession ? { kind: "session", id: defaultSession.id } : null);
  const selectedSession = effectiveSelection?.kind === "session" ? snapshot.workstreams.find((stream) => stream.id === effectiveSelection.id) ?? null : null;
  const selectedCollision = effectiveSelection?.kind === "collision" ? snapshot.findings.find((finding) => finding.id === effectiveSelection.id) ?? null : null;
  const openCollisions = snapshot.findings.filter((finding) => finding.state === "open");
  const activePeople = groups.filter(([, streams]) => streams.some((stream) => stream.presence === "online")).length;
  const agentSessions = snapshot.workstreams.filter((stream) => stream.agent && stream.agent.status !== "done").length;

  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    return () => { delete document.documentElement.dataset.theme; };
  }, [dark]);
  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen((open) => !open);
      }
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, []);

  const selectProject = (nextId: string) => { setProjectId(nextId); setSelection(null); setCommandOpen(false); };

  return <div className={sidebarCollapsed ? "workroom-shell sidebar-collapsed" : "workroom-shell"}>
    <aside className="project-sidebar">
      <div className="sidebar-brand-row"><Brand compact={sidebarCollapsed} /><button className="icon-button sidebar-toggle" onClick={() => setSidebarCollapsed((value) => !value)} aria-label={sidebarCollapsed ? "Expand Projects sidebar" : "Collapse Projects sidebar"}>{sidebarCollapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}</button></div>
      <button className="command-trigger" onClick={() => setCommandOpen(true)} aria-label="Search Projects and commands"><Search size={15} />{!sidebarCollapsed && <><span>Search</span><kbd>⌘K</kbd></>}</button>
      <nav aria-label="Projects" className="project-list"><div className="sidebar-section-label"><span>{!sidebarCollapsed && "Projects"}</span>{!sidebarCollapsed && <span>{session.projects.length}</span>}</div>{session.projects.map((project) => {
        const projectSnapshot = source.get(project.id);
        const liveCount = projectSnapshot.workstreams.filter((stream) => stream.presence !== "offline" && stream.agent?.status !== "done").length;
        const collisionCount = projectSnapshot.findings.filter((finding) => finding.state === "open").length;
        return <button key={project.id} className={project.id === projectId ? "project-item active" : "project-item"} aria-current={project.id === projectId ? "page" : undefined} onClick={() => selectProject(project.id)} title={sidebarCollapsed ? project.name : undefined}><span className="project-monogram">{project.name.slice(0, 1).toUpperCase()}</span>{!sidebarCollapsed && <span className="project-copy"><strong>{project.name}</strong><small>{project.repositoryLabel}</small></span>}{!sidebarCollapsed && <span className="project-signals">{liveCount > 0 && <span className="live-count" aria-label={`${liveCount} active sessions`}>{liveCount}</span>}{collisionCount > 0 && <span className="collision-count" aria-label={`${collisionCount} open collisions`}>{collisionCount}</span>}</span>}</button>;
      })}</nav>
      <div className="sidebar-bottom"><button className="profile-button" onClick={() => setSettingsOpen(true)} aria-haspopup="dialog" aria-label="Open settings, devices, and privacy"><span className="avatar">{initialsFor(session.memberName)}</span>{!sidebarCollapsed && <span><strong>{session.memberName}</strong><small>Settings & privacy</small></span>}{!sidebarCollapsed && <Settings2 size={15} />}</button></div>
    </aside>

    <main className="workroom-main">
      {offline && <div className="offline-strip" role="status"><CircleDot size={14} /><strong>Offline</strong><span>Showing revision {snapshot.contextRevision} from {snapshot.synchronizedAt}.</span></div>}
      <header className="project-header"><div className="project-heading"><div className="project-title-line"><h1>{snapshot.project.name}</h1><span className={offline ? "live-pill offline" : "live-pill"}><span />{offline ? "Offline" : "Live"}</span></div><div className="project-subtitle"><span>{snapshot.project.repositoryLabel}</span><span>·</span><span>revision {snapshot.contextRevision}</span><SemanticStatus status={snapshot.project.semanticStatus} /></div></div><div className="header-actions">{!source.live && <button className="ghost-button" disabled={offline} onClick={() => source.publishSyntheticUpdate(projectId)}><Activity size={15} />Simulate activity</button>}<button className="icon-button" onClick={() => setDark((value) => !value)} aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}>{dark ? <Sun size={17} /> : <Moon size={17} />}</button><button className="icon-button" onClick={() => setSettingsOpen(true)} aria-label="Open Project settings"><Settings2 size={17} /></button><button className={snapshot.workspacePaused ? "pause-control paused" : "pause-control"} disabled={offline || source.live} title={source.live ? "Use the Stickguy menu bar to pause live sharing" : undefined} onClick={() => source.togglePause(projectId)}>{snapshot.workspacePaused ? <Play size={14} /> : <Pause size={14} />}{source.live ? "Menu bar" : snapshot.workspacePaused ? "Resume" : "Pause"}</button></div></header>
      <p className="sr-only" aria-live="polite">{snapshot.workspacePaused ? "Workspace sharing is paused." : "Workspace sharing is active."}</p>
      {snapshot.workspacePaused && <div className="status-strip paused" role="status"><Pause size={15} /><div><strong>Workspace sharing is paused</strong><span>Activity transmission stopped before this state was shown.</span></div></div>}

      <section className="now-section" aria-labelledby="now-title">
        <div className="section-heading"><div><h2 id="now-title">Now</h2><p>{activePeople} {activePeople === 1 ? "person" : "people"} active · {agentSessions} agent {agentSessions === 1 ? "session" : "sessions"}</p></div>{openCollisions.length > 0 && <span className="attention-count"><AlertTriangle size={13} />{openCollisions.length} {openCollisions.length === 1 ? "collision" : "collisions"}</span>}</div>
        {openCollisions.length > 0 && <div className="collision-stack" aria-label="Open collisions">{openCollisions.map((finding) => <CollisionRow key={finding.id} finding={finding} sessions={snapshot.workstreams} selected={effectiveSelection?.kind === "collision" && effectiveSelection.id === finding.id} onClick={() => setSelection({ kind: "collision", id: finding.id })} />)}</div>}
        <div className="people-list">{groups.map(([memberName, streams]) => <PersonGroup key={memberName} memberName={memberName} streams={streams} selected={effectiveSelection} onSelect={(id) => setSelection({ kind: "session", id })} />)}</div>
      </section>

      <section className="recent-section" aria-labelledby="recent-title"><div className="section-heading"><div><h2 id="recent-title">Recent</h2><p>Project activity, newest first</p></div></div><ol className="timeline-list">{snapshot.activity.map((item) => <li key={item.id}><span className={`timeline-icon ${item.kind}`}><Activity size={13} /></span><div><p><strong>{item.actor}</strong> {item.summary}</p><span>{item.at} · {activitySourceLabel(item.fidelity)}</span></div></li>)}</ol></section>
    </main>

    <aside className="inspector" aria-label="Details inspector">
      {selectedSession ? <SessionInspector session={selectedSession} /> : selectedCollision ? <CollisionInspector finding={selectedCollision} sessions={snapshot.workstreams} disabled={offline} onState={(state) => source.setFindingState(projectId, selectedCollision.id, state)} onFeedback={(value) => source.recordFindingFeedback(selectedCollision.id, value)} /> : <InspectorEmpty />}
    </aside>

    {settingsOpen && <SettingsDialog snapshot={snapshot} dark={dark} onTheme={() => setDark((value) => !value)} onClose={() => setSettingsOpen(false)} />}
    {commandOpen && <CommandPalette projects={session.projects} selectedProjectId={projectId} onSelectProject={selectProject} onSettings={() => { setCommandOpen(false); setSettingsOpen(true); }} onClose={() => setCommandOpen(false)} />}
  </div>;
}

function useProjectSnapshot(source: FixtureProjectSource, projectId: string): ProjectSnapshot {
  return useSyncExternalStore((listener) => source.subscribe(projectId, listener), () => source.get(projectId), () => source.get(projectId));
}

function groupByMember(streams: Workstream[]): Array<[string, Workstream[]]> {
  const groups = new Map<string, Workstream[]>();
  for (const stream of streams) groups.set(stream.memberName, [...(groups.get(stream.memberName) ?? []), stream]);
  return [...groups.entries()].sort(([, left], [, right]) => presenceRank(left) - presenceRank(right));
}

function presenceRank(streams: Workstream[]): number {
  if (streams.some((stream) => stream.presence === "online")) return 0;
  if (streams.some((stream) => stream.presence === "idle")) return 1;
  if (streams.some((stream) => stream.presence === "paused")) return 2;
  return 3;
}

function PersonGroup({ memberName, streams, selected, onSelect }: { memberName: string; streams: Workstream[]; selected: Selection | null; onSelect: (id: string) => void }) {
  const [expanded, setExpanded] = useState(true);
  const presence = streams.some((stream) => stream.presence === "online") ? "online" : streams.some((stream) => stream.presence === "idle") ? "idle" : streams[0]?.presence ?? "offline";
  const groupId = `person-${streams[0]?.id}`;
  return <section className="person-tree-node" aria-labelledby={groupId}><button className="person-tree-row" onClick={() => setExpanded((value) => !value)} aria-expanded={expanded} aria-controls={`${groupId}-sessions`}><span className="tree-chevron">{expanded ? <ChevronDown size={15} /> : <ChevronRight size={15} />}</span><span className="person-avatar">{initialsFor(memberName)}</span><span className="person-tree-copy"><strong id={groupId}>{memberName}</strong><small><span className={`presence-dot ${presence}`} />{presence === "online" ? "Working now" : presence === "idle" ? "Recently active" : presence === "paused" ? "Sharing paused" : "Offline"}</small></span><span className="tree-count">{streams.length}</span></button>{expanded && <div className="session-tree" id={`${groupId}-sessions`}>{streams.map((stream) => <SessionRow key={stream.id} session={stream} selected={selected?.kind === "session" && selected.id === stream.id} onClick={() => onSelect(stream.id)} />)}</div>}</section>;
}

function VendorMark({ vendor, size = 18 }: { vendor: "codex" | "claude"; size?: number }) {
  if (vendor === "claude") return <svg className="vendor-mark" width={size} height={size} viewBox="0 0 24 24" role="img" aria-label="Claude Code"><path d="M12 2.3v19.4M2.3 12h19.4M5.15 5.15l13.7 13.7M18.85 5.15l-13.7 13.7M8.25 2.95l7.5 18.1M2.95 8.25l18.1 7.5M15.75 2.95l-7.5 18.1M21.05 8.25l-18.1 7.5" /></svg>;
  return <svg className="vendor-mark" width={size} height={size} viewBox="0 0 24 24" role="img" aria-label="Codex"><path d="M12 3.1a4.7 4.7 0 0 1 4.45 3.18 4.7 4.7 0 0 1 2.88 7.02 4.7 4.7 0 0 1-4.45 6.6A4.7 4.7 0 0 1 7.55 17.7a4.7 4.7 0 0 1-2.88-7.02 4.7 4.7 0 0 1 4.45-6.6A4.7 4.7 0 0 1 12 3.1Zm0 3.2-4.93 2.85v5.7L12 17.7l4.93-2.85v-5.7L12 6.3Z" /></svg>;
}

function SessionRow({ session, selected, onClick }: { session: Workstream; selected: boolean; onClick: () => void }) {
  const vendor = session.agent?.vendor === "codex" ? "Codex" : session.agent?.vendor === "claude" ? "Claude Code" : "Shared task";
  const activeSubagents = session.agent?.subagents.filter((agent) => agent.status !== "done").length ?? 0;
  return <button className={selected ? "session-row selected" : "session-row"} onClick={onClick} aria-pressed={selected} aria-label={`Open ${vendor} session for ${session.memberName}`}><span className="tree-elbow" aria-hidden="true" /><span className={`agent-icon ${session.agent?.vendor ?? "manual"}`}>{session.agent ? <VendorMark vendor={session.agent.vendor} /> : <Code2 size={16} />}</span><span className="session-copy"><span className="session-title"><strong>{vendor}</strong>{session.agent?.sessionAlias && <code>{session.agent.sessionAlias}</code>}<span className={`session-state ${session.agent?.status ?? session.presence}`}>{session.agent?.status ?? session.presence}</span></span><span className="session-action">{session.outcome}</span><span className="session-meta">{session.agent?.branch && <span><GitBranch size={12} />{session.agent.branch}</span>}{session.agent?.tool && <span><Command size={12} />{session.agent.tool}</span>}{activeSubagents > 0 && <span><Users size={12} />{activeSubagents} {activeSubagents === 1 ? "subagent" : "subagents"}</span>}<span><FileCode2 size={12} />{session.pathCount.toLocaleString()} {session.pathCount === 1 ? "path" : "paths"}</span><span>{session.updatedLabel}</span></span></span><ChevronRight className="row-chevron" size={16} /></button>;
}

function CollisionRow({ finding, sessions, selected, onClick }: { finding: Finding; sessions: Workstream[]; selected: boolean; onClick: () => void }) {
  const affected = sessions.filter((stream) => finding.workstreamIds.includes(stream.id));
  const path = finding.evidence.find((item) => item.kind === "path")?.label;
  return <button className={selected ? "collision-row selected" : "collision-row"} onClick={onClick} aria-pressed={selected}><span className="collision-symbol"><AlertTriangle size={16} /></span><span><strong>{finding.kind === "direct_collision" ? "Collision detected" : "Possible collision"}</strong><span>{affected.map((stream) => stream.memberName).join(" and ") || finding.title}</span>{path && <code>{path}</code>}</span><span className="collision-confidence">{finding.confidence}</span><ChevronRight size={16} /></button>;
}

function SessionInspector({ session }: { session: Workstream }) {
  const vendor = session.agent?.vendor === "codex" ? "Codex" : session.agent?.vendor === "claude" ? "Claude Code" : "Shared task";
  const activeSubagents = session.agent?.subagents.filter((agent) => agent.status !== "done") ?? [];
  const activity = session.agent?.activity ?? [];
  return <div className="inspector-content"><header className="inspector-header"><span className={`agent-icon large ${session.agent?.vendor ?? "manual"}`}>{session.agent ? <VendorMark vendor={session.agent.vendor} size={21} /> : <Code2 size={19} />}</span><div><p>{session.memberName}</p><h2>{vendor}</h2>{session.agent?.sessionAlias && <code>{session.agent.sessionAlias}</code>}</div></header><div className="session-overview"><span className="overview-status"><span className={`presence-dot ${session.presence}`} /><strong>{statusCopy(session)}</strong></span><span>{fidelityLabel(session.fidelity)}</span>{session.agent?.branch && <span><GitBranch size={12} /><code>{session.agent.branch}</code></span>}<span>{session.updatedLabel}</span></div><section className="inspector-section"><h3>Current activity</h3><p className="activity-copy">{session.outcome}</p>{session.agent?.tool && <div className="tool-line"><Command size={14} /><span>Using</span><code>{session.agent.tool}</code></div>}</section>{session.agent && <section className="inspector-section"><h3>Session activity {activity.length > 0 && <span>{activity.length}</span>}</h3><p className="section-note">Safe activity shared by this session. Prompts, source, diffs, raw output, and private reasoning stay on the member’s device.</p>{activity.length > 0 ? <ol className="session-activity-list">{activity.map((item) => <li key={item.id}><span className={`activity-rail-dot ${item.status}`} /><div><strong>{item.action}</strong><span>{item.at}{item.tool ? ` · ${item.tool}` : ""}</span>{item.paths.length > 0 && <code>{item.paths[0]}</code>}</div></li>)}</ol> : <p className="muted-copy">Waiting for the next agent activity event.</p>}</section>}<section className="inspector-section"><h3>Files and paths</h3>{session.paths.length > 0 ? <ul className="path-list">{session.paths.map((path) => <li key={path}><FileCode2 size={14} /><code>{path}</code></li>)}</ul> : <p className="muted-copy">No safe paths have been reported yet.</p>}{session.pathCount > session.paths.length && <p className="muted-copy">{(session.pathCount - session.paths.length).toLocaleString()} additional paths summarized.</p>}</section>{activeSubagents.length > 0 && <section className="inspector-section"><h3>Subagents <span>{activeSubagents.length}</span></h3><ul className="subagent-list">{activeSubagents.map((agent) => <li key={agent.alias}><span className="subagent-icon"><Bot size={13} /></span><span><strong>{agent.agentType || "Subagent"}</strong><code>{agent.alias}</code></span><span className={`session-state ${agent.status}`}>{agent.status}</span></li>)}</ul></section>}{session.largeChange && <section className="inspector-section large-change-detail"><h3>Large change</h3><strong>{session.largeChange.pathCount.toLocaleString()} paths</strong><p>{session.largeChange.summary}</p><small>Manifest revision {session.largeChange.revision}. Size alone does not imply risk.</small></section>}</div>;
}

function CollisionInspector({ finding, sessions, disabled, onState, onFeedback }: { finding: Finding; sessions: Workstream[]; disabled: boolean; onState: (state: FindingState) => void; onFeedback: (value: FindingFeedback) => Promise<void> }) {
  const affected = useMemo(() => sessions.filter((session) => finding.workstreamIds.includes(session.id)), [finding.workstreamIds, sessions]);
  const [feedback, setFeedback] = useState<FindingFeedback | null>(null);
  const [feedbackError, setFeedbackError] = useState(false);
  const [feedbackPending, setFeedbackPending] = useState(false);
  const submitFeedback = (value: FindingFeedback) => {
    setFeedbackPending(true);
    setFeedbackError(false);
    void onFeedback(value).then(() => setFeedback(value)).catch(() => setFeedbackError(true)).finally(() => setFeedbackPending(false));
  };
  const feedbackMessage = feedback ? "Feedback recorded" : feedbackError ? "Feedback could not be recorded" : "Was this collision useful?";
  return <article className="inspector-content collision-detail" aria-label="Selected collision detail"><header className="collision-detail-header"><span className="collision-symbol large"><AlertTriangle size={19} /></span><div><p>{finding.kind === "direct_collision" ? "Collision" : "Possible collision"}</p><h2>{finding.title}</h2></div></header><div className="collision-badges"><span className={`severity-badge ${finding.severity}`}>{finding.severity}</span><span>{finding.confidence} confidence</span><span>{finding.state}</span></div><p className="collision-reason">{finding.reason}</p><section className="inspector-section"><h3>People and sessions</h3><ul className="affected-list">{affected.map((session) => <li key={session.id}><span className="person-avatar small">{initialsFor(session.memberName)}</span><span><strong>{session.memberName}</strong><small>{session.agent?.vendor === "codex" ? "Codex" : session.agent?.vendor === "claude" ? "Claude Code" : session.title}</small></span></li>)}</ul></section><section className="inspector-section"><h3>Why Stickguy flagged it</h3><ul className="evidence-list">{finding.evidence.map((evidence) => <li key={`${evidence.kind}-${evidence.label}`}><span>{evidence.kind}</span><code>{evidence.label}</code><small>{evidence.source.replaceAll("_", " ")}</small></li>)}</ul></section><dl className="session-facts"><div><dt>First seen</dt><dd>{finding.firstSeen}</dd></div><div><dt>Last changed</dt><dd>{finding.lastSeen}</dd></div></dl><p className="advisory-note">Advisory only. Stickguy never blocks or controls an agent.</p><div className="finding-feedback" aria-label="Collision feedback"><span role="status">{feedbackMessage}</span><button disabled={disabled || feedbackPending} className="text-button" onClick={() => submitFeedback("useful")}>Useful</button><button disabled={disabled || feedbackPending} className="text-button" onClick={() => submitFeedback("not_related")}>Not related</button></div><div className="finding-actions"><button disabled={disabled || finding.state === "acknowledged"} className="secondary-button" onClick={() => onState("acknowledged")}>Acknowledge</button><button disabled={disabled || finding.state === "resolved"} className="primary-button" onClick={() => onState("resolved")}>Mark resolved</button></div></article>;
}

function InspectorEmpty() {
  return <div className="inspector-empty"><span><Bot size={20} /></span><h2>Select a session</h2><p>Choose an agent or collision to inspect its live coordination details.</p></div>;
}

function SemanticStatus({ status }: { status: ProjectSnapshot["project"]["semanticStatus"] }) {
  return <span className={`semantic-status ${status}`} aria-label="Semantic processing status" title={semanticMessage(status)}><CircleDot size={11} />Semantic {status}{status !== "enabled" && " · structural live"}</span>;
}

function SettingsDialog({ snapshot, dark, onTheme, onClose }: { snapshot: ProjectSnapshot; dark: boolean; onTheme: () => void; onClose: () => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const dialog = dialogRef.current;
    if (typeof dialog?.showModal === "function") dialog.showModal();
    else dialog?.setAttribute("open", "");
    closeRef.current?.focus();
    return () => {
      if (typeof dialog?.close === "function" && dialog.open) dialog.close();
      else dialog?.removeAttribute("open");
    };
  }, []);
  return <dialog ref={dialogRef} className="settings-dialog" aria-labelledby="settings-title" onCancel={(event) => { event.preventDefault(); onClose(); }}><header><div><p>Stickguy</p><h2 id="settings-title">Settings</h2></div><button ref={closeRef} className="icon-button" onClick={onClose} aria-label="Close settings"><X size={17} /></button></header><section><h3>Appearance</h3><button className="settings-row" onClick={onTheme}><span className="settings-icon">{dark ? <Moon size={16} /> : <Sun size={16} />}</span><span><strong>Theme</strong><small>{dark ? "Dark" : "Light"}</small></span><ChevronRight size={15} /></button></section><section><h3>Devices</h3>{snapshot.devices.map((device) => <div className="settings-row" key={device.id}><span className="settings-icon"><Laptop2 size={16} /></span><span><strong>{device.label}</strong><small>{device.platform} · {device.status} · {device.lastSeen}</small></span><span className={`presence-dot ${device.status}`} /></div>)}</section><section><h3>Privacy</h3><div className="privacy-card"><ShieldCheck size={17} /><div><strong>Coordination metadata only</strong><p>Source, diffs, Git objects, prompts, transcripts, environment values, and raw command output are not collected.</p></div></div></section><button className="secondary-button dialog-done" onClick={onClose}>Done</button></dialog>;
}

function CommandPalette({ projects, selectedProjectId, onSelectProject, onSettings, onClose }: { projects: DashboardSession["projects"]; selectedProjectId: string; onSelectProject: (id: string) => void; onSettings: () => void; onClose: () => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  useEffect(() => {
    const dialog = dialogRef.current;
    if (typeof dialog?.showModal === "function") dialog.showModal();
    else dialog?.setAttribute("open", "");
    inputRef.current?.focus();
    return () => {
      if (typeof dialog?.close === "function" && dialog.open) dialog.close();
      else dialog?.removeAttribute("open");
    };
  }, []);
  const visible = projects.filter((project) => `${project.name} ${project.repositoryLabel}`.toLowerCase().includes(query.toLowerCase()));
  return <dialog ref={dialogRef} className="command-dialog" aria-label="Search Projects and commands" onCancel={(event) => { event.preventDefault(); onClose(); }}><div className="command-search"><Search size={17} /><input ref={inputRef} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search Projects and commands…" aria-label="Search Projects and commands" /><kbd>esc</kbd></div><div className="command-results"><p>Projects</p>{visible.map((project) => <button key={project.id} onClick={() => onSelectProject(project.id)}><span className="project-monogram">{project.name.slice(0, 1)}</span><span><strong>{project.name}</strong><small>{project.repositoryLabel}</small></span>{project.id === selectedProjectId && <Check size={15} />}</button>)}<p>Commands</p><button onClick={onSettings}><span className="settings-icon"><Settings2 size={15} /></span><span><strong>Open settings</strong><small>Appearance, devices, and privacy</small></span></button></div></dialog>;
}

function initialsFor(name: string): string {
  return name.split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]?.toUpperCase()).join("") || "?";
}

function statusCopy(session: Workstream): string {
  const status = session.agent?.status ?? session.presence;
  if (status === "active" || status === "online") return "Working now";
  if (status === "waiting") return "Waiting for input";
  if (status === "done") return "Session finished";
  if (status === "error") return "Needs attention";
  if (status === "paused") return "Sharing paused";
  return status === "idle" ? "Recently active" : "Offline";
}

function activitySourceLabel(fidelity: ProjectSnapshot["activity"][number]["fidelity"]): string {
  return fidelity === "structural" ? "structural" : fidelityLabel(fidelity);
}

export function DesktopPreviewBanner({ live = false }: { live?: boolean }) {
  return <div className="desktop-preview-banner" role="status"><strong>Stickguy Dev</strong><span>{live ? "Local live Project data · menu bar controls available" : "Fixture data · open a live Project from the menu bar"}</span></div>;
}

const root = document.getElementById("root");
if (root) {
  const parameters = new URLSearchParams(window.location.search);
  const desktopPreview = parameters.get("desktop") === "preview" || window.location.protocol === "wails:";
  if (desktopPreview) document.documentElement.dataset.desktopPreview = "true";
  createRoot(root).render(<StrictMode>
    {desktopPreview && parameters.get("desktop") !== "onboarding" && <DesktopPreviewBanner live={parameters.get("live") === "1"} />}
    {parameters.get("desktop") === "onboarding" ? <DesktopOnboarding /> : parameters.get("live") === "1" ? <LiveApp /> : <App />}
  </StrictMode>);
}
