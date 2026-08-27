import { StrictMode, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { FormEvent } from "react";
import { createRoot } from "react-dom/client";
import {
  AlertTriangle,
  Bot,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleDot,
  Code2,
  Command,
  FileCode2,
  GitBranch,
  Laptop2,
  LayoutGrid,
  Moon,
  Pause,
  Play,
  Plus,
  Search,
  Settings2,
  ShieldCheck,
  Sun,
  UserRound,
  Users,
  X,
  Zap,
} from "lucide-react";
import { FixtureProjectSource } from "./fixture-source";
import { emptyFixtureSession, fixtureSession, parseShellState } from "./fixtures";
import { LiveProjectSource, loadSession, loadSnapshot } from "./live-source";
import { DesktopOnboarding } from "./desktop-onboarding";
import { elapsedFromLabel, formatElapsed } from "./elapsed";
import type { DashboardSession, Finding, FindingFeedback, FindingState, LocalSessionDetail, MemberNameSource, ProjectAccess, ProjectSnapshot, SessionMessageKind, SessionMessagesSnapshot, ShellState, SyncCard, Workstream } from "./model";
import { nativeOnboarding, type EnrollmentRequest, type NativeOnboarding } from "./native";
import { fidelityLabel, semanticMessage, semanticModeMessage, stateMessage } from "./state";
import "./style.css";

const defaultSource = new FixtureProjectSource();

interface AppProps {
  initialState?: ShellState;
  initialSession?: DashboardSession;
  source?: FixtureProjectSource;
  nativeApi?: NativeOnboarding;
  navigate?: (url: string) => void;
}

type Selection = { kind: "session"; id: string } | { kind: "collision"; id: string };
type View = "workroom" | "decisions";

export function App({
  initialState = parseShellState(window.location.search),
  initialSession = initialState === "empty" ? emptyFixtureSession() : fixtureSession,
  source = defaultSource,
  nativeApi = nativeOnboarding,
  navigate = (url) => window.location.assign(url),
}: AppProps) {
  const [shellState, setShellState] = useState(initialState);
  if (shellState === "activation") return <ActivationView onActivate={() => setShellState("ready")} />;
  if (shellState === "loading") return <LoadingView />;
  if (shellState === "unauthorized" || shellState === "version_mismatch") return <TerminalState state={shellState} />;
  if (shellState === "empty" || initialSession.projects.length === 0) return <EmptyView />;
  return <ProjectWorkroom session={initialSession} source={source} offline={shellState === "offline"} nativeApi={nativeApi} navigate={navigate} />;
}

function Brand({ compact = false }: { compact?: boolean }) {
  return <div className="brand" aria-label="Stickguy"><span className="brand-mark" aria-hidden="true">S</span>{!compact && <span>stickguy</span>}</div>;
}

function ActivationView({ onActivate }: { onActivate: () => void }) {
  return <main className="centered-shell"><Brand /><section className="state-card" aria-labelledby="activation-title"><span className="state-symbol"><ShieldCheck size={20} /></span><p className="eyebrow">Browser activation</p><h1 id="activation-title">Open your shared Project workroom.</h1><p>Your one-time access ticket is exchanged server-side. It is never stored in this page, activity, or browser history.</p><div className="disclosure"><strong>What teammates can see</strong><p>Session presence, action categories, safe repository paths, collisions, coordination decisions, and classifier-passing session messages while sharing is unpaused. Never source, diffs, prompts, transcripts, <code>.env</code> values, credentials, or raw tool output.</p></div><button className="pill solid" onClick={onActivate}>Activate secure session</button><p className="microcopy">Sessions are revocable, same-site, and rotated after privilege changes.</p></section></main>;
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
  return <ProjectWorkroom session={session} source={source} offline={state === "offline"} nativeApi={nativeOnboarding} navigate={(url) => window.location.assign(url)} />;
}

function LoadingView() {
  return <main className="centered-shell"><Brand /><section className="state-card" role="status" aria-live="polite"><span className="spinner" aria-hidden="true" /><p className="eyebrow">Connecting</p><h1>{stateMessage("loading")}</h1><p>Authorizing membership and opening the current Project.</p></section></main>;
}

function TerminalState({ state }: { state: "unauthorized" | "version_mismatch" }) {
  const isVersion = state === "version_mismatch";
  return <main className="centered-shell"><Brand /><section className="state-card" role="alert"><span className="state-symbol"><AlertTriangle size={20} /></span><p className="eyebrow">{isVersion ? "Version mismatch" : "Access denied"}</p><h1>{stateMessage(state)}</h1><p>{isVersion ? "This app cannot safely interpret the service contract. Update the Stickguy executable, then reload." : "No Project metadata was loaded. Ask a Project owner to restore membership or enroll this device again."}</p><button className="pill" onClick={() => window.location.reload()}>{isVersion ? "Check again" : "Retry authorization"}</button></section></main>;
}

function EmptyView() {
  return <main className="centered-shell"><Brand /><section className="state-card"><span className="state-symbol"><GitBranch size={20} /></span><p className="eyebrow">No Projects</p><h1>{stateMessage("empty")}</h1><p>Connect a repository from Stickguy Dev.app or join an existing Project with an invite.</p><code>stickguy create</code></section></main>;
}

/** One interval for the whole app so every elapsed clock advances together. */
function useSecondTick(active: boolean): number {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!active) return;
    const timer = window.setInterval(() => setTick((value) => value + 1), 1_000);
    return () => window.clearInterval(timer);
  }, [active]);
  return tick;
}

function since(label: string | undefined, tick: number): string {
  const base = elapsedFromLabel(label);
  // An unparseable label is shown as the service wrote it rather than guessed at.
  return base === null ? label ?? "" : formatElapsed(base + tick);
}

function Elapsed({ label, tick }: { label: string | undefined; tick: number }) {
  return <span className="clock">{since(label, tick)}</span>;
}

/** Elapsed time inside a sentence, where the mono clock chrome would be noise. */
function Since({ label, tick }: { label: string | undefined; tick: number }) {
  return <>{since(label, tick)}</>;
}

function ProjectWorkroom({ session, source, offline, nativeApi, navigate }: { session: DashboardSession; source: FixtureProjectSource; offline: boolean; nativeApi: NativeOnboarding; navigate: (url: string) => void }) {
  const [projectId, setProjectId] = useState(session.selectedProjectId);
  const [selection, setSelection] = useState<Selection | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [commandOpen, setCommandOpen] = useState(false);
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [dark, setDark] = useState(false);
  const [view, setView] = useState<View>("workroom");
  const [identity, setIdentity] = useState<{ name: string; source: MemberNameSource }>({ name: session.memberName, source: session.memberNameSource });
  const [identityPromptDismissed, setIdentityPromptDismissed] = useState(false);
  const [attention, setAttention] = useState<Finding | null>(null);
  const seenFindings = useRef<Set<string> | null>(null);
  const snapshot = useProjectSnapshot(source, projectId);

  const mine = useMemo(() => snapshot.workstreams.filter((stream) => stream.memberName === identity.name), [snapshot.workstreams, identity.name]);
  const nearby = useMemo(() => snapshot.workstreams.filter((stream) => stream.memberName !== identity.name).sort((left, right) => presenceRank(left) - presenceRank(right)), [snapshot.workstreams, identity.name]);
  const mineIds = useMemo(() => new Set(mine.map((stream) => stream.id)), [mine]);
  const openFindings = snapshot.findings.filter((finding) => finding.state === "open");
  // Only what reaches your own work is yours to act on; the rest stays quiet.
  const converging = openFindings.filter((finding) => finding.workstreamIds.some((id) => mineIds.has(id)));
  const elsewhere = openFindings.filter((finding) => !finding.workstreamIds.some((id) => mineIds.has(id)));
  const convergingWorkstreams = useMemo(() => new Set(converging.flatMap((finding) => finding.workstreamIds)), [converging]);

  const defaultSession = mine.find((stream) => stream.agent?.status === "active") ?? mine[0] ?? snapshot.workstreams[0];
  const effectiveSelection: Selection | null = selection ?? (defaultSession ? { kind: "session", id: defaultSession.id } : null);
  const selectedSession = effectiveSelection?.kind === "session" ? snapshot.workstreams.find((stream) => stream.id === effectiveSelection.id) ?? null : null;
  const selectedCollision = effectiveSelection?.kind === "collision" ? snapshot.findings.find((finding) => finding.id === effectiveSelection.id) ?? null : null;
  const anyLive = snapshot.workstreams.some((stream) => stream.presence === "online" || stream.agent?.status === "active");
  const tick = useSecondTick(anyLive);

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
  useEffect(() => {
    const current = new Set(snapshot.findings.map((finding) => finding.id));
    if (seenFindings.current === null) { seenFindings.current = current; return; }
    const next = snapshot.findings.find((finding) => finding.state === "open" && (finding.severity === "high" || finding.severity === "critical") && !seenFindings.current!.has(finding.id));
    seenFindings.current = current;
    if (next) setAttention(next);
  }, [snapshot.findings]);

  const selectProject = (nextId: string) => { setProjectId(nextId); setSelection(null); setView("workroom"); setCommandOpen(false); };
  const openSession = (id: string) => { setView("workroom"); setSelection({ kind: "session", id }); };

  return <div className={sidebarCollapsed ? "workroom-shell sidebar-collapsed" : "workroom-shell"}>
    <aside className="side">
      <div className="side-top">
        <Brand compact={sidebarCollapsed} />
        <button className="icon-button side-toggle" onClick={() => setSidebarCollapsed((value) => !value)} aria-label={sidebarCollapsed ? "Expand Projects sidebar" : "Collapse Projects sidebar"}>{sidebarCollapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}</button>
      </div>

      <button className="command-trigger" onClick={() => setCommandOpen(true)} aria-label="Search Projects and commands"><Search size={15} />{!sidebarCollapsed && <><span>Search</span><kbd>⌘K</kbd></>}</button>

      <div className="side-scroll">
        <button className="nav-item" aria-current={view === "workroom" ? "page" : undefined} onClick={() => setView("workroom")}>
          <LayoutGrid size={16} />{!sidebarCollapsed && <span>Workroom</span>}
          {!sidebarCollapsed && converging.length > 0 && <span className="nav-count">{converging.length}</span>}
        </button>
        <button className="nav-item" aria-current={view === "decisions" ? "page" : undefined} onClick={() => setView("decisions")}>
          <Check size={16} />{!sidebarCollapsed && <span>Decisions</span>}
        </button>

        <div className="side-group"><span className="side-label">Projects</span></div>
        {session.projects.map((project) => {
          const projectSnapshot = source.get(project.id);
          const collisionCount = projectSnapshot.findings.filter((finding) => finding.state === "open").length;
          return <button key={project.id} className="project-item" aria-current={project.id === projectId ? "page" : undefined} onClick={() => selectProject(project.id)} title={sidebarCollapsed ? project.name : undefined}>
            <span className="project-monogram">{project.name.slice(0, 1).toUpperCase()}</span>
            {!sidebarCollapsed && <>{project.name}{collisionCount > 0 && <span className="project-count">{collisionCount}</span>}</>}
          </button>;
        })}
        <button className="project-item new" onClick={() => setNewProjectOpen(true)} aria-haspopup="dialog" aria-label="Add a new Project"><span className="project-monogram"><Plus size={11} /></span>{!sidebarCollapsed && "New project"}</button>
      </div>

      <button className="profile-button" onClick={() => setSettingsOpen(true)} aria-haspopup="dialog" aria-label="Open settings, devices, and privacy">
        <span className="avatar">{initialsFor(identity.name)}</span>
        {!sidebarCollapsed && <span className="who"><strong>{identity.name}</strong><small>Settings &amp; privacy</small></span>}
      </button>
    </aside>

    <main className="workroom-main">
      <div className="main-bar">
        <span className="spacer" />
        {!source.live && <button className="pill" disabled={offline} onClick={() => source.publishSyntheticUpdate(projectId)}><Zap size={14} />Simulate activity</button>}
        <button className="icon-button" onClick={() => setDark((value) => !value)} aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}>{dark ? <Sun size={16} /> : <Moon size={16} />}</button>
        <button className="icon-button" onClick={() => setSettingsOpen(true)} aria-label="Open Project settings"><Settings2 size={16} /></button>
        <button className={snapshot.workspacePaused ? "pill alerting" : "pill"} disabled={offline || source.live} title={source.live ? "Use the Stickguy menu bar to pause live sharing" : undefined} onClick={() => source.togglePause(projectId)}>
          {snapshot.workspacePaused ? <Play size={14} /> : <Pause size={14} />}{source.live ? "Menu bar" : snapshot.workspacePaused ? "Resume" : "Pause"}
        </button>
      </div>

      <div className="main-scroll">
        <div className="main-column">
          <header className="project-head">
            <h1>{snapshot.project.name}</h1>
            <div className="project-sub">
              <span>{snapshot.project.repositoryLabel}</span><span>·</span>
              <span>rev {snapshot.contextRevision}</span><span>·</span>
              <span>{offline ? "offline, last synced " : "synced "}<Since label={snapshot.synchronizedAt} tick={tick} /> ago</span>
            </div>
          </header>

          <p className="sr-only" aria-live="polite">{snapshot.workspacePaused ? "Workspace sharing is paused." : "Workspace sharing is active."}</p>

          {offline && <div className="notice" role="status"><CircleDot size={15} /><div className="body"><strong>Offline</strong>Showing revision {snapshot.contextRevision} from {snapshot.synchronizedAt}.</div></div>}
          {snapshot.workspacePaused && <div className="notice alerting" role="status"><Pause size={15} /><div className="body"><strong>Workspace sharing is paused</strong>Activity transmission stopped before this state was shown.</div></div>}
          {identity.source === "device" && !identityPromptDismissed && <div className="notice" role="status"><UserRound size={15} /><div className="body"><strong>Choose how teammates see you</strong>This Project is still showing your device name, “{identity.name}”. Pick a display name for your live work; the device name stays in Settings under Devices &amp; security.</div><div className="notice-actions"><button className="pill" onClick={() => setSettingsOpen(true)}>Choose a name</button><button className="text-button" onClick={() => setIdentityPromptDismissed(true)}>Later</button></div></div>}
          {attention && <div className="notice alerting" role="alert"><AlertTriangle size={15} /><div className="body"><strong>Coordination update</strong>{attention.reason}</div><div className="notice-actions"><button className="pill" onClick={() => { setView("workroom"); setSelection({ kind: "collision", id: attention.id }); setAttention(null); }}>Review</button><button className="text-button" onClick={() => setAttention(null)}>Dismiss</button></div></div>}

          {view === "workroom"
            ? <WorkroomView
                snapshot={snapshot} mine={mine} nearby={nearby} converging={converging} elsewhere={elsewhere}
                convergingWorkstreams={convergingWorkstreams} selection={effectiveSelection} viewer={identity.name} tick={tick}
                onSelectSession={openSession} onSelectFinding={(id) => setSelection({ kind: "collision", id })}
              />
            : <DecisionsView snapshot={snapshot} />}
        </div>
      </div>
    </main>

    <aside className="inspector" aria-label="Details inspector">
      {selectedSession
        ? <SessionInspector session={selectedSession} source={source} tick={tick} />
        : selectedCollision
          ? <CollisionInspector
              finding={selectedCollision} sessions={snapshot.workstreams} viewer={identity.name} projectId={projectId} source={source}
              card={snapshot.collaboration.syncCards.find((entry) => entry.findingId === selectedCollision.id) ?? null}
              disabled={offline}
              onState={(state) => source.setFindingState(projectId, selectedCollision.id, state)}
              onFeedback={(value) => source.recordFindingFeedback(selectedCollision.id, value)}
              onOpenSession={openSession}
            />
          : <InspectorEmpty />}
    </aside>

    {settingsOpen && <SettingsDialog snapshot={snapshot} dark={dark} identity={identity} projectId={projectId} source={source} offline={offline} onIdentity={setIdentity} onTheme={() => setDark((value) => !value)} onClose={() => setSettingsOpen(false)} />}
    {commandOpen && <CommandPalette projects={session.projects} selectedProjectId={projectId} onSelectProject={selectProject} onSettings={() => { setCommandOpen(false); setSettingsOpen(true); }} onClose={() => setCommandOpen(false)} />}
    {newProjectOpen && <NewProjectDialog api={nativeApi} displayName={identity.source === "member" ? identity.name : ""} navigate={navigate} onClose={() => setNewProjectOpen(false)} />}
  </div>;
}

function useProjectSnapshot(source: FixtureProjectSource, projectId: string): ProjectSnapshot {
  return useSyncExternalStore((listener) => source.subscribe(projectId, listener), () => source.get(projectId), () => source.get(projectId));
}

function presenceRank(stream: Workstream): number {
  if (stream.presence === "online") return 0;
  if (stream.presence === "idle") return 1;
  if (stream.presence === "paused") return 2;
  return 3;
}

function WorkroomView({ snapshot, mine, nearby, converging, elsewhere, convergingWorkstreams, selection, viewer, tick, onSelectSession, onSelectFinding }: {
  snapshot: ProjectSnapshot; mine: Workstream[]; nearby: Workstream[]; converging: Finding[]; elsewhere: Finding[];
  convergingWorkstreams: Set<string>; selection: Selection | null; viewer: string; tick: number;
  onSelectSession: (id: string) => void; onSelectFinding: (id: string) => void;
}) {
  return <>
    <div className="block-head"><h2>Converging on you</h2>{converging.length > 0 && <span className="count hot">{converging.length}</span>}</div>
    <SemanticStatus status={snapshot.project.semanticStatus} mode={snapshot.project.semanticMode} />
    {converging.length === 0
      ? <p className="block-empty">Nothing is reaching your work right now.</p>
      : converging.map((finding) => <ConvergeBlock key={finding.id} finding={finding} sessions={snapshot.workstreams} viewer={viewer} tick={tick} selected={selection?.kind === "collision" && selection.id === finding.id} onSelect={() => onSelectFinding(finding.id)} onOpenSession={onSelectSession} />)}

    <div className="block-head"><h2>Your sessions</h2><span className="count">{mine.length}</span></div>
    {mine.length === 0
      ? <p className="block-empty">No sessions are registered to you in this Project yet.</p>
      : <div className="rows">{mine.map((stream) => <SessionRow key={stream.id} session={stream} tick={tick} converging={convergingWorkstreams.has(stream.id)} selected={selection?.kind === "session" && selection.id === stream.id} onClick={() => onSelectSession(stream.id)} />)}</div>}

    <div className="block-head"><h2>Nearby</h2><span className="count">{nearby.length}</span></div>
    {nearby.length === 0
      ? <p className="block-empty">No teammates are registered to this Project yet.</p>
      : <div className="rows">{nearby.map((stream) => <PersonRow key={stream.id} session={stream} tick={tick} onClick={() => onSelectSession(stream.id)} />)}</div>}

    {elsewhere.length > 0 && <>
      <div className="block-head"><h2>Elsewhere in the Project</h2><span className="count">{elsewhere.length}</span></div>
      {elsewhere.map((finding) => <ConvergeBlock key={finding.id} finding={finding} sessions={snapshot.workstreams} viewer={viewer} tick={tick} quiet selected={selection?.kind === "collision" && selection.id === finding.id} onSelect={() => onSelectFinding(finding.id)} onOpenSession={onSelectSession} />)}
    </>}

    <div className="block-head"><h2>Recent</h2></div>
    <ol className="timeline">{snapshot.activity.map((item) => <li key={item.id}><p><strong>{item.actor}</strong> {item.summary}</p><span className="src"><Since label={item.at} tick={tick} /> · {activitySourceLabel(item.fidelity)}</span></li>)}</ol>
  </>;
}

function DecisionsView({ snapshot }: { snapshot: ProjectSnapshot }) {
  const resolved = snapshot.collaboration.syncCards.filter((card) => card.state === "resolved" && card.resolution);
  const loose = snapshot.collaboration.resolutions.filter((resolution) => !resolved.some((card) => card.resolution?.id === resolution.id));
  const empty = resolved.length === 0 && loose.length === 0;
  return <>
    <div className="block-head"><h2>Decisions</h2>{!empty && <span className="count">{resolved.length + loose.length}</span>}</div>
    {empty
      ? <p className="block-empty">Nothing has been decided yet. Resolving a collision records the outcome here and delivers it to every affected session.</p>
      : <div>
          {resolved.map((card) => <article className="decision-entry" key={card.id}><h3>{card.title}</h3><p>{card.resolution?.summary}</p><div className="sent">Delivered to {card.resolution?.affectedWorkstreamIds.length ?? 0} session{(card.resolution?.affectedWorkstreamIds.length ?? 0) === 1 ? "" : "s"} · revision {card.revision}</div></article>)}
          {loose.map((resolution) => <article className="decision-entry" key={resolution.id}><h3>Coordination decision</h3><p>{resolution.summary}</p><div className="sent">Delivered to {resolution.affectedWorkstreamIds.length} session{resolution.affectedWorkstreamIds.length === 1 ? "" : "s"} · revision {resolution.revision}</div></article>)}
        </div>}
  </>;
}

function VendorMark({ vendor, size = 18 }: { vendor: "codex" | "claude"; size?: number }) {
  if (vendor === "claude") return <svg className="vendor-mark" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" role="img" aria-label="Claude Code"><path d="M12 2.3v19.4M2.3 12h19.4M5.15 5.15l13.7 13.7M18.85 5.15l-13.7 13.7M8.25 2.95l7.5 18.1M2.95 8.25l18.1 7.5M15.75 2.95l-7.5 18.1M21.05 8.25l-18.1 7.5" /></svg>;
  return <svg className="vendor-mark" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" role="img" aria-label="Codex"><path d="M12 3.1a4.7 4.7 0 0 1 4.45 3.18 4.7 4.7 0 0 1 2.88 7.02 4.7 4.7 0 0 1-4.45 6.6A4.7 4.7 0 0 1 7.55 17.7a4.7 4.7 0 0 1-2.88-7.02 4.7 4.7 0 0 1 4.45-6.6A4.7 4.7 0 0 1 12 3.1Zm0 3.2-4.93 2.85v5.7L12 17.7l4.93-2.85v-5.7L12 6.3Z" /></svg>;
}

function vendorLabel(session: Workstream): string {
  return session.agent?.vendor === "codex" ? "Codex" : session.agent?.vendor === "claude" ? "Claude Code" : "Shared task";
}

/** The file an agent is touching right now, which is newer than its path list. */
function currentPath(session: Workstream): string | undefined {
  return session.agent?.activity?.find((item) => item.paths.length > 0)?.paths[0] ?? session.paths[0];
}

function isLive(session: Workstream): boolean {
  return session.agent?.status === "active" && session.presence !== "paused";
}

/** Running work reads as moving text plus a counting clock, never a status light. */
function LiveAction({ session }: { session: Workstream }) {
  if (!isLive(session)) return <>{session.outcome}</>;
  return <span className="livetext">{session.outcome}<span className="ellipsis" /></span>;
}

function SessionRow({ session, tick, converging, selected, onClick }: { session: Workstream; tick: number; converging: boolean; selected: boolean; onClick: () => void }) {
  const path = currentPath(session);
  const activeSubagents = session.agent?.subagents.filter((agent) => agent.status !== "done") ?? [];
  return <button className="session-row" aria-current={selected ? "true" : undefined} onClick={onClick} aria-label={`Open ${vendorLabel(session)} session for ${session.memberName}`}>
    <span className="session-icon">{session.agent ? <VendorMark vendor={session.agent.vendor} size={19} /> : <Code2 size={18} />}</span>
    <span>
      <h3>{session.agent?.sessionTitle ?? session.title}</h3>
      <span className="session-meta">{vendorLabel(session)}{session.agent?.sessionAlias ? ` · ${session.agent.sessionAlias}` : ""}{session.agent?.branch ? ` · ${session.agent.branch}` : ""}</span>
      <span className="session-doing"><LiveAction session={session} /></span>
      {path && <span className="session-files"><span className="p path-swap" key={path}>{path}</span><span className="c">{session.pathCount.toLocaleString()} {session.pathCount === 1 ? "file" : "files"}</span></span>}
      {activeSubagents.map((agent) => <span className="session-sub" key={agent.alias}><b>{agent.agentType || "subagent"}</b> subagent · {agent.status}</span>)}
    </span>
    <span className="session-right">
      {converging && <span className="session-warn" title="Converging with another session"><AlertTriangle size={14} /></span>}
      <Elapsed label={session.updatedLabel} tick={tick} />
      <span className="chev"><ChevronRight size={15} /></span>
    </span>
  </button>;
}

/** For a teammate the useful fact is intent - what they are about to do. */
function PersonRow({ session, tick, onClick }: { session: Workstream; tick: number; onClick: () => void }) {
  return <button className={`person-row ${session.presence}`} onClick={onClick} aria-label={`Open ${vendorLabel(session)} session for ${session.memberName}`}>
    <span className="nm">{session.memberName}</span>
    <span className="intent">{session.outcome}</span>
    <Elapsed label={session.updatedLabel} tick={tick} />
  </button>;
}

/** Who you would actually go and talk to: everyone on the finding except you. */
function otherNames(affected: Workstream[], viewer: string): string {
  const names = [...new Set(affected.map((stream) => stream.memberName).filter((name) => name !== viewer))];
  return names.length > 0 ? names.join(" and ") : "your teammate";
}

function findingHeadline(finding: Finding): string {
  return finding.kind === "direct_collision" ? "Collision detected" : finding.kind === "redundant_work" ? "Redundant work" : finding.kind === "shared_dependency" ? "Shared dependency" : finding.kind === "assumption_conflict" ? "Assumption conflict" : finding.kind === "downstream_impact" ? "Downstream impact" : finding.kind === "stale_assumption" ? "Stale assumption" : "Possible collision";
}

function evidenceKindLabel(kind: Finding["evidence"][number]["kind"]): string {
  return ({ path: "same file", contract: "same contract", dependency: "shared dependency", intent: "related intent" } as const)[kind];
}

/** Both sides of a collision, each one a line you can open. */
function ConvergeBlock({ finding, sessions, viewer, tick, quiet = false, selected, onSelect, onOpenSession }: {
  finding: Finding; sessions: Workstream[]; viewer: string; tick: number; quiet?: boolean; selected: boolean; onSelect: () => void; onOpenSession: (id: string) => void;
}) {
  const affected = sessions.filter((stream) => finding.workstreamIds.includes(stream.id));
  const names = affected.map((stream) => stream.memberName).join(" and ");
  const others = otherNames(affected, viewer);
  return <section className={quiet ? "converge quiet" : "converge"} aria-label={`${findingHeadline(finding)} ${names}`}>
    <span className="converge-icon"><AlertTriangle size={18} /></span>
    <h3><button onClick={onSelect} aria-label={`${findingHeadline(finding)} ${names}`} aria-current={selected ? "true" : undefined}>{finding.title}</button></h3>
    <p className="why">{finding.reason}</p>
    <div className="pair">{affected.map((stream) => <button className="mini" key={stream.id} onClick={() => onOpenSession(stream.id)} aria-label={`Open ${stream.memberName}'s side of this collision`}>
      <span className="mini-icon">{stream.agent ? <VendorMark vendor={stream.agent.vendor} size={17} /> : <Code2 size={16} />}</span>
      <span className="nm">{stream.memberName} <em>· {stream.agent?.sessionTitle ?? stream.title}</em></span>
      <Elapsed label={stream.updatedLabel} tick={tick} />
      <span className="chev"><ChevronRight size={14} /></span>
      <span className="doing"><LiveAction session={stream} /></span>
    </button>)}</div>
    <div className="evidence">{finding.evidence.map((item) => <span key={`${item.kind}-${item.label}`} style={{ display: "contents" }}>
      <span className="k">{item.label}</span>
      <span className={item.source === "git" ? "v fact" : "v"}>{evidenceKindLabel(item.kind)} · {item.source.replaceAll("_", " ")}</span>
    </span>)}</div>
    <p className="converge-meta">{finding.confidence} confidence · first seen <Since label={finding.firstSeen} tick={tick} /> ago</p>
    <div className="converge-actions">
      <button className="pill solid" onClick={onSelect}>Talk to {others} about this</button>
    </div>
  </section>;
}

function SessionInspector({ session, source, tick }: { session: Workstream; source: FixtureProjectSource; tick: number }) {
  const [shared, setShared] = useState<SessionMessagesSnapshot | null>(null);
  const [own, setOwn] = useState<LocalSessionDetail | null>(null);
  const [messageError, setMessageError] = useState("");
  const activity = session.agent?.activity ?? [];
  const activeSubagents = (session.agent?.subagents ?? []).filter((agent) => agent.status !== "done");
  const path = currentPath(session);

  useEffect(() => {
    let cancelled = false;
    if (!session.agent) { setShared(null); setOwn(null); return; }
    const refresh = () => {
      void source.getSessionMessages(session.id).then((value) => { if (!cancelled) setShared(value); }).catch(() => { if (!cancelled) setMessageError("Session messages are unavailable."); });
      // Own-session content is read locally and never uploaded, so it loads
      // whether or not this session is shared.
      void source.getLocalSession(session.id).then((value) => { if (!cancelled) setOwn(value); }).catch(() => { if (!cancelled) setOwn(null); });
    };
    refresh();
    const timer = window.setInterval(refresh, 3_000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [session.id, source, session.agent]);

  // Prefer the member's own local transcript; Project members see the
  // classifier-passing projection for teammate sessions (ADR-047).
  const mine = (own?.messages ?? []).length > 0;
  const conversation: Array<{ id: string; kind: SessionMessageKind | "tool"; text?: string; tool?: string; at?: string }> = mine
    ? (own?.messages ?? []).map((message, index) => ({ id: `own-${index}`, kind: message.kind, text: message.text, tool: message.tool, at: message.at }))
    : (shared?.messages ?? []).map((message) => ({ id: message.id, kind: message.kind, text: message.text, at: message.capturedAt }));
  const title = session.agent?.sessionTitle ?? own?.title ?? session.title;

  return <>
    <div className="inspector-bar">
      <span className="grow">
        <h2>{title}</h2>
        <div className="sub">{session.memberName} · {vendorLabel(session)}{session.agent?.sessionAlias ? ` · ${session.agent.sessionAlias}` : ""}</div>
      </span>
    </div>
    <div className="inspector-body">
      <h3 className="inspector-head">Intent</h3>
      <div className="inspector-intent">
        {session.outcome}
        <div className="src">{statusCopy(session)}</div>
      </div>

      {activity.length > 0 && <>
        <h3 className="inspector-head">Activity <span>{activity.length}</span></h3>
        {activity.map((item, index) => <div className={index === 0 && isLive(session) ? "phase now" : index === 0 ? "phase" : "phase done"} key={item.id}>
          <div className="phase-head">
            <h3 className={index === 0 && isLive(session) ? "livetext" : undefined}>{item.action}{index === 0 && isLive(session) && <span className="ellipsis" />}</h3>
            <Elapsed label={item.at} tick={tick} />
          </div>
          {(item.tool || item.paths.length > 0) && <ul>
            {item.tool && <li>Using <code>{item.tool}</code>.</li>}
            {item.paths.length > 0 && !item.action.includes(item.paths[0]) && <li><code>{item.paths[0]}</code>{item.paths.length > 1 ? ` and ${item.paths.length - 1} more` : ""}</li>}
            {item.paths.length > 1 && item.action.includes(item.paths[0]) && <li>and {item.paths.length - 1} more path{item.paths.length === 2 ? "" : "s"}</li>}
          </ul>}
        </div>)}
      </>}

      {session.largeChange && <>
        <h3 className="inspector-head">Large change</h3>
        <dl className="facts">
          <dt>paths</dt><dd>{session.largeChange.pathCount.toLocaleString()} paths</dd>
          <dt>summary</dt><dd>{session.largeChange.summary}</dd>
          <dt>revision</dt><dd>manifest {session.largeChange.revision} <span className="q">· size alone does not imply risk</span></dd>
        </dl>
      </>}

      <h3 className="inspector-head">Files this session</h3>
      {session.paths.length > 0
        ? <div className="file-list">
            {session.paths.map((entry) => <div className={entry === path ? "file-row hot" : "file-row"} key={entry}><span className="p">{entry}</span><span className="v">{entry === path && isLive(session) ? "touching now" : "reported"}</span></div>)}
            {session.pathCount > session.paths.length && <div className="file-row"><span className="p">{(session.pathCount - session.paths.length).toLocaleString()} more</span><span className="v">summarized</span></div>}
          </div>
        : <p className="muted-copy">No safe paths have been reported yet.</p>}

      {activeSubagents.length > 0 && <>
        <h3 className="inspector-head">Subagents <span>{activeSubagents.length}</span></h3>
        <dl className="facts">{activeSubagents.map((agent) => <span key={agent.alias} style={{ display: "contents" }}><dt>{agent.alias}</dt><dd>{agent.agentType || "subagent"} · {agent.status}</dd></span>)}</dl>
      </>}

      <h3 className="inspector-head">How we know</h3>
      <dl className="facts">
        <dt>fidelity</dt><dd>{fidelityLabel(session.fidelity)}</dd>
        {session.agent?.branch && <><dt>branch</dt><dd>{session.agent.branch}</dd></>}
        {session.agent && <><dt>paths</dt><dd>{session.agent.capabilities.observeSafePaths ? "observed" : "unavailable"}</dd></>}
        {session.agent && <><dt>briefs</dt><dd>{session.agent.capabilities.deliverBrief.replaceAll("_", " ")}</dd></>}
        {session.agent && <><dt>attention</dt><dd>{session.agent.capabilities.requestAttention === "advisory" ? "advisory only" : "dashboard only"} <span className="q">· Stickguy never interrupts an agent</span></dd></>}
      </dl>

      {messageError && <p className="form-error" role="alert">{messageError}</p>}

      {session.agent && <>
        <h3 className="inspector-head">Session</h3>
        <p className="muted-copy" style={{ marginBottom: 12 }}>{mine ? "Read from this machine; classifier-passing messages are visible to Project members while sharing is active." : `Shared by ${session.memberName}.`}</p>
        {conversation.length > 0
          ? <ol className="conversation-list">{conversation.map((message) => message.kind === "tool"
            ? <li key={message.id} className="tool"><span><Command size={11} />{message.tool}</span></li>
            : <li key={message.id} className={message.kind}><span>{messageKindLabel(message.kind as SessionMessageKind)}</span><p>{message.text}</p>{message.at && <small>{new Date(message.at).toLocaleTimeString()}</small>}</li>)}</ol>
          : <p className="muted-copy">Waiting for the first classifier-passing message in this session.</p>}
      </>}
    </div>
  </>;
}

function messageKindLabel(kind: SessionMessageKind): string {
  return ({ user: "You", assistant: "Assistant", thinking: "Thinking", system: "Instructions" } as const)[kind];
}

/**
 * A finding and its sync card are one object at two ages, so the conversation
 * and the decision live here rather than behind a separate tab.
 */
function CollisionInspector({ finding, sessions, viewer, projectId, source, card, disabled, onState, onFeedback, onOpenSession }: {
  finding: Finding; sessions: Workstream[]; viewer: string; projectId: string; source: FixtureProjectSource; card: SyncCard | null; disabled: boolean;
  onState: (state: FindingState) => void; onFeedback: (value: FindingFeedback) => Promise<void>; onOpenSession: (id: string) => void;
}) {
  const affected = useMemo(() => sessions.filter((session) => finding.workstreamIds.includes(session.id)), [finding.workstreamIds, sessions]);
  const [feedback, setFeedback] = useState<FindingFeedback | null>(null);
  const [feedbackError, setFeedbackError] = useState(false);
  const [feedbackPending, setFeedbackPending] = useState(false);
  const [comment, setComment] = useState("");
  const [decision, setDecision] = useState("");
  const [threadPending, setThreadPending] = useState(false);
  const [threadError, setThreadError] = useState("");
  const others = otherNames(affected, viewer);
  const names = affected.map((session) => session.memberName).join(" and ");

  const submitFeedback = (value: FindingFeedback) => {
    setFeedbackPending(true);
    setFeedbackError(false);
    void onFeedback(value).then(() => setFeedback(value)).catch(() => setFeedbackError(true)).finally(() => setFeedbackPending(false));
  };
  const run = (operation: () => Promise<void>) => {
    setThreadPending(true); setThreadError("");
    void operation()
      .catch((cause: unknown) => setThreadError(cause instanceof Error && cause.message.includes("revision") ? "Someone changed this first. Reload and try again." : "That change could not be saved."))
      .finally(() => setThreadPending(false));
  };
  const feedbackMessage = feedback ? "Feedback recorded" : feedbackError ? "Feedback could not be recorded" : "Was this collision useful?";

  return <article className="collision-detail" aria-label="Selected collision detail">
    <div className="inspector-bar">
      <span className="grow">
        <h2>{finding.title}</h2>
        <div className="severity-row"><span className="sev">{findingHeadline(finding)}</span><span>{finding.severity}</span><span>{finding.confidence} confidence</span><span>{finding.state}</span></div>
      </span>
    </div>
    <div className="inspector-body">
      <p className="collision-reason">{finding.reason}</p>

      <h3 className="inspector-head">Sessions</h3>
      <div className="pair">{affected.map((session) => <button className="mini" key={session.id} onClick={() => onOpenSession(session.id)} aria-label={`Open ${session.memberName}'s session detail`}>
        <span className="mini-icon">{session.agent ? <VendorMark vendor={session.agent.vendor} size={17} /> : <Code2 size={16} />}</span>
        <span className="nm">{session.memberName} <em>· {vendorLabel(session)}</em></span>
        <span className="clock">{session.updatedLabel}</span>
        <span className="chev"><ChevronRight size={14} /></span>
        <span className="doing">{session.outcome}</span>
      </button>)}</div>

      <h3 className="inspector-head">Why Stickguy flagged it</h3>
      <dl className="facts">{finding.evidence.map((item) => <span key={`${item.kind}-${item.label}`} style={{ display: "contents" }}>
        <dt>{evidenceKindLabel(item.kind)}</dt><dd>{item.label} <span className="q">· {item.source.replaceAll("_", " ")}</span></dd>
      </span>)}
        <dt>first seen</dt><dd>{finding.firstSeen}</dd>
        <dt>last changed</dt><dd>{finding.lastSeen}</dd>
      </dl>

      <h3 className="inspector-head">Work it out</h3>
      <div className="thread">
        {!card && <button className="pill solid" disabled={disabled || threadPending} onClick={() => run(() => source.createSyncCard(projectId, finding.id, finding.title, finding.reason))}>Talk to {others} about this</button>}
        {card && card.comments.length > 0 && <ol>{card.comments.map((entry) => <li key={entry.id}><strong>{entry.memberName}</strong><span>{entry.body}</span></li>)}</ol>}
        {card?.resolution && <div className="decision-note"><div className="lbl"><Check size={13} />Decision from {names}</div><p>{card.resolution.summary}</p><div className="sent">Delivered to {card.resolution.affectedWorkstreamIds.length} session{card.resolution.affectedWorkstreamIds.length === 1 ? "" : "s"} · revision {card.revision}</div></div>}
        {card && card.state === "open" && <>
          <form onSubmit={(event) => { event.preventDefault(); const body = comment.trim(); if (!body) return; run(async () => { await source.commentSyncCard(projectId, card.id, body); setComment(""); }); }}>
            <input value={comment} onChange={(event) => setComment(event.target.value)} placeholder="Add a comment…" aria-label={`Comment on ${card.title}`} />
            <button className="pill" disabled={threadPending || disabled}>Comment</button>
          </form>
          <form onSubmit={(event) => { event.preventDefault(); const summary = decision.trim(); if (!summary) return; run(async () => { await source.resolveSyncCard(projectId, card.id, card.revision, summary, finding.workstreamIds); setDecision(""); }); }}>
            <input value={decision} onChange={(event) => setDecision(event.target.value)} placeholder="How did you resolve it?" aria-label={`Resolution for ${card.title}`} />
            <button className="pill solid" disabled={threadPending || disabled}>Resolve</button>
          </form>
        </>}
        {threadError && <p className="form-error" role="alert">{threadError}</p>}
      </div>

      <p className="advisory-note">Advisory only. Stickguy never blocks or controls an agent.</p>

      <div className="finding-feedback" aria-label="Collision feedback">
        <span role="status">{feedbackMessage}</span>
        <button disabled={disabled || feedbackPending} className="text-button" onClick={() => submitFeedback("useful")}>Useful</button>
        <button disabled={disabled || feedbackPending} className="text-button" onClick={() => submitFeedback("not_related")}>Not related</button>
      </div>
      <div className="finding-actions">
        <button disabled={disabled || finding.state === "acknowledged"} className="pill" onClick={() => onState("acknowledged")}>Acknowledge</button>
        <button disabled={disabled || finding.state === "resolved"} className="pill solid" onClick={() => onState("resolved")}>Mark resolved</button>
      </div>
    </div>
  </article>;
}

function InspectorEmpty() {
  return <div className="inspector-empty"><Bot size={20} /><h2>Select a session</h2><p>Choose one of your sessions or a collision to inspect its live coordination detail.</p></div>;
}

/**
 * Honest fidelity (stickguy-v1-spec section 3) requires saying when findings are
 * structural only. Healthy semantic processing is the expected state, so it says
 * nothing; a caveat appears only when one actually applies.
 */
function SemanticStatus({ status, mode }: { status: ProjectSnapshot["project"]["semanticStatus"]; mode: ProjectSnapshot["project"]["semanticMode"] }) {
  if (status === "enabled") return null;
  return <p className="fidelity-note" aria-label="Semantic processing status" title={`${semanticMessage(status)} ${semanticModeMessage(mode)}`}>
    Structural evidence only — semantic matching is {status}.
  </p>;
}

function SettingsDialog({ snapshot, dark, identity, projectId, source, offline, onIdentity, onTheme, onClose }: { snapshot: ProjectSnapshot; dark: boolean; identity: { name: string; source: MemberNameSource }; projectId: string; source: FixtureProjectSource; offline: boolean; onIdentity: (value: { name: string; source: MemberNameSource }) => void; onTheme: () => void; onClose: () => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const [nameDraft, setNameDraft] = useState(identity.source === "member" ? identity.name : "");
  const [identityError, setIdentityError] = useState("");
  const [identitySaved, setIdentitySaved] = useState(false);
  const [identityPending, setIdentityPending] = useState(false);
  const [access, setAccess] = useState<ProjectAccess | null>(null);
  const [adminError, setAdminError] = useState("");
  const [adminPending, setAdminPending] = useState(false);
  const [inviteCode, setInviteCode] = useState("");
  const [deleteDraft, setDeleteDraft] = useState("");
  const [deletionQueued, setDeletionQueued] = useState(false);
  const refreshAccess = () => source.getProjectAccess(projectId).then(setAccess).catch(() => setAdminError("Project access controls could not be loaded."));
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
  useEffect(() => { void refreshAccess(); }, [projectId]);
  const admin = (operation: () => Promise<void>) => {
    setAdminPending(true); setAdminError("");
    void operation().then(refreshAccess).catch(() => setAdminError("That security change could not be completed.")).finally(() => setAdminPending(false));
  };
  return <dialog ref={dialogRef} className="settings-dialog" aria-labelledby="settings-title" onCancel={(event) => { event.preventDefault(); onClose(); }}><header><div><p>Stickguy</p><h2 id="settings-title">Settings</h2></div><button ref={closeRef} className="icon-button" onClick={onClose} aria-label="Close settings"><X size={17} /></button></header><section><h3>Your identity</h3><form className="identity-form" onSubmit={(event) => {
    event.preventDefault();
    const value = nameDraft.trim();
    setIdentityError(""); setIdentitySaved(false); setIdentityPending(true);
    void source.updateDisplayName(projectId, value)
      .then((result) => { onIdentity({ name: result.memberName, source: result.memberNameSource }); setNameDraft(result.memberName); setIdentitySaved(true); })
      .catch((error: Error & { status?: number }) => setIdentityError(error.status === 400 ? "Choose a display name; an email address cannot be your Project identity." : error.message || "That display name could not be saved."))
      .finally(() => setIdentityPending(false));
  }}><label><span>Display name</span><input value={nameDraft} onChange={(event) => { setNameDraft(event.target.value); setIdentitySaved(false); }} minLength={2} maxLength={60} placeholder={identity.source === "device" ? identity.name : "How teammates see you"} aria-describedby="identity-help" /></label><p id="identity-help" className="settings-help">This is how you appear on live sessions and collision resolutions. It is not your email address or your device name.</p>{identity.source === "device" && <p className="settings-help warning">Currently showing the device name this Project was created with.</p>}{identityError && <p className="form-error" role="alert">{identityError}</p>}{identitySaved && <p className="settings-help success" role="status">Display name updated across this Project.</p>}<button className="pill solid" disabled={identityPending || offline || nameDraft.trim().length < 2}>Save name</button></form></section>
    <section><h3>Appearance</h3><button className="settings-row" onClick={onTheme}><span className="settings-icon">{dark ? <Moon size={16} /> : <Sun size={16} />}</span><span><strong>Theme</strong><small>{dark ? "Dark" : "Light"}</small></span><ChevronRight size={15} /></button></section>
    <section><h3>Members</h3>{access?.members.map((member) => <div className="settings-row" key={member.id}><span className="settings-icon"><Users size={16} /></span><span><strong>{member.name}{member.isSelf ? " · you" : ""}</strong><small>{member.role}</small></span>{access.role === "owner" && !member.isSelf && <button className="text-button" disabled={adminPending || offline} onClick={() => admin(() => source.removeMember(projectId, member.id))}>Remove</button>}</div>) ?? <p className="settings-help">Loading members…</p>}</section>
    <section><h3>Devices &amp; security</h3><p className="settings-help">Device names identify hardware for revocation and audit only; they are never shown as your live-work identity. Revoking a device immediately ends its Project access.</p>{access?.devices.map((device) => <div className="settings-row" key={device.id}><span className="settings-icon"><Laptop2 size={16} /></span><span><strong>{device.label}{device.isCurrent ? " · this device" : ""}</strong><small>{device.appVersion} · {device.revoked ? "revoked" : device.lastSeenAt ?? "never seen"}</small></span>{!device.revoked && access.role === "owner" && !device.isCurrent && <button className="text-button" disabled={adminPending || offline} onClick={() => admin(() => source.revokeDevice(projectId, device.id))}>Revoke</button>}</div>) ?? snapshot.devices.map((device) => <div className="settings-row" key={device.id}><span className="settings-icon"><Laptop2 size={16} /></span><span><strong>{device.label}</strong><small>{device.platform} · {device.status} · {device.lastSeen}</small></span></div>)}</section>
    {access?.role === "owner" && <section><h3>Invites</h3><button className="pill" disabled={adminPending || offline} onClick={() => { setAdminPending(true); setAdminError(""); void source.createInvite(projectId).then((result) => { setInviteCode(result.code); return refreshAccess(); }).catch(() => setAdminError("A new invite could not be created.")).finally(() => setAdminPending(false)); }}><Plus size={14} />Create one-use invite</button>{inviteCode && <div className="invite-code" role="status"><strong>Share this code privately</strong><code>{inviteCode}</code><p>Shown once. It expires in 10 minutes.</p></div>}{access.invites.map((invite) => <div className="settings-row" key={invite.id}><span><strong>{invite.id}</strong><small>{invite.revoked ? "Revoked" : `${invite.remainingUses} use remaining · expires ${new Date(invite.expiresAt).toLocaleString()}`}</small></span>{!invite.revoked && <button className="text-button" disabled={adminPending || offline} onClick={() => admin(() => source.revokeInvite(projectId, invite.id))}>Revoke</button>}</div>)}</section>}
    <section><h3>Privacy &amp; data</h3><div className="privacy-card"><ShieldCheck size={17} /><div><strong>Local-first analysis, bounded Project sharing</strong><p>Raw source, diffs, environment values, credentials, and command output never cross the wire. Project members can see classifier-approved coordination facts and session context while sharing is unpaused.</p></div></div>{access && <a className="settings-row" href={source.exportURL(projectId)} download><span className="settings-icon"><FileCode2 size={16} /></span><span><strong>Export retained {access.role === "owner" ? "Project" : "personal"} data</strong><small>Versioned JSON containing the structured records you are authorized to export.</small></span><ChevronRight size={15} /></a>}</section>
    {access?.role === "owner" && <section><h3>Delete Project</h3><p className="settings-help warning">Deletion immediately revokes Project sessions and invites, then removes retained hosted records in bounded batches.</p><label className="identity-form"><span>Type {snapshot.project.name} to confirm</span><input value={deleteDraft} onChange={(event) => setDeleteDraft(event.target.value)} /></label><button className="pill" disabled={adminPending || offline || deleteDraft !== snapshot.project.name || deletionQueued} onClick={() => { setAdminPending(true); setAdminError(""); void source.deleteProject(projectId).then(() => setDeletionQueued(true)).catch(() => setAdminError("Project deletion could not be started.")).finally(() => setAdminPending(false)); }}>{deletionQueued ? "Deletion queued" : "Delete Project"}</button></section>}
    {access?.role === "member" && <section><h3>Leave and delete my data</h3><p className="settings-help warning">This immediately removes your Project access and schedules deletion of your retained work records.</p><label className="identity-form"><span>Type {snapshot.project.name} to confirm</span><input value={deleteDraft} onChange={(event) => setDeleteDraft(event.target.value)} /></label><button className="pill" disabled={adminPending || offline || deleteDraft !== snapshot.project.name || deletionQueued} onClick={() => { setAdminPending(true); setAdminError(""); void source.deleteOwnProjectData(projectId).then(() => setDeletionQueued(true)).catch(() => setAdminError("Your data deletion could not be started.")).finally(() => setAdminPending(false)); }}>{deletionQueued ? "Deletion queued" : "Leave and delete my data"}</button></section>}
    {adminError && <p className="form-error" role="alert">{adminError}</p>}<button className="pill dialog-done" onClick={onClose}>Done</button></dialog>;
}

function NewProjectDialog({ api, displayName, navigate, onClose }: { api: NativeOnboarding; displayName: string; navigate: (url: string) => void; onClose: () => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const nameRef = useRef<HTMLInputElement>(null);
  const [request, setRequest] = useState<EnrollmentRequest>({ repositoryRoot: "", projectLabel: "", deviceLabel: "This Mac", displayName, joinCode: "", enableCodex: false, enableClaude: false });
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState<{ projectId: string; joinCode: string; warnings: string[] } | null>(null);
  useEffect(() => {
    const dialog = dialogRef.current;
    if (typeof dialog?.showModal === "function") dialog.showModal();
    else dialog?.setAttribute("open", "");
    nameRef.current?.focus();
    void api.state().then((state) => setRequest((current) => ({ ...current, deviceLabel: state.deviceLabel || current.deviceLabel }))).catch(() => undefined);
    return () => {
      if (typeof dialog?.close === "function" && dialog.open) dialog.close();
      else dialog?.removeAttribute("open");
    };
  }, [api]);
  const chooseRepository = async () => {
    setError("");
    try {
      const root = await api.chooseRepository();
      if (root) setRequest((current) => ({ ...current, repositoryRoot: root, projectLabel: current.projectLabel || root.split("/").at(-1) || "My Project" }));
    } catch (cause) { setError((cause as Error).message); }
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setPending(true); setError("");
    try {
      const result = await api.createAdditionalProject(request);
      setCreated({ ...result, warnings: Array.isArray(result.warnings) ? result.warnings : [] });
    } catch (cause) { setError((cause as Error).message); }
    finally { setPending(false); }
  };
  const open = async () => {
    if (!created) return;
    setPending(true); setError("");
    try { navigate(await api.openLiveProject(created.projectId)); }
    catch (cause) { setError((cause as Error).message); setPending(false); }
  };
  return <dialog ref={dialogRef} className="new-project-dialog" aria-labelledby="new-project-title" onCancel={(event) => { event.preventDefault(); onClose(); }} onClick={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <header><div><p>Projects</p><h2 id="new-project-title">{created ? "Project created" : "Add a new Project"}</h2></div><button className="icon-button" onClick={onClose} aria-label="Close new Project"><X size={17} /></button></header>
    {created ? <section className="new-project-success"><span className="state-symbol"><Check size={20} /></span><h3>{request.projectLabel}</h3><p>The repository is registered with this Mac’s existing Stickguy service.</p>{created.joinCode && <div className="invite-code"><strong>One-use invite code</strong><code>{created.joinCode}</code><p>Expires in 10 minutes. Share it privately with the next teammate.</p></div>}{created.warnings.map((warning) => <p className="form-error" key={warning}>{warning}</p>)}{error && <p className="form-error" role="alert">{error}</p>}<div className="dialog-actions"><button className="pill" onClick={onClose}>Done</button><button className="pill solid" disabled={pending} onClick={() => void open()}>{pending ? "Opening…" : "Open Project"}</button></div></section> : <form onSubmit={(event) => void submit(event)}>
      <p className="dialog-lede">Choose a Git repository. Stickguy will observe it as a separate Project without starting another background service.</p>
      <label><span>Project name</span><input ref={nameRef} value={request.projectLabel} maxLength={120} onChange={(event) => setRequest({ ...request, projectLabel: event.target.value })} placeholder="Atlas launch" /></label>
      <label><span>Repository</span><div className="repository-field"><input readOnly value={request.repositoryRoot} placeholder="Choose a local Git repository" /><button type="button" className="pill" onClick={() => void chooseRepository()}>Choose…</button></div></label>
      <fieldset><legend>Connect coding agents</legend><label><input type="checkbox" checked={request.enableCodex} onChange={(event) => setRequest({ ...request, enableCodex: event.target.checked })} /><span><strong>Codex</strong><small>Observe new repository-scoped sessions after restart</small></span></label><label><input type="checkbox" checked={request.enableClaude} onChange={(event) => setRequest({ ...request, enableClaude: event.target.checked })} /><span><strong>Claude Code</strong><small>Observe new repository-scoped sessions after restart</small></span></label></fieldset>
      <p className="privacy-note"><strong>Project sharing</strong> Classifier-passing coordination facts are visible to enrolled members while sharing is unpaused. Credentials, environment values, raw source, diffs, and command output do not cross the wire.</p>
      {error && <p className="form-error" role="alert">{error}</p>}<div className="dialog-actions"><button type="button" className="pill" onClick={onClose}>Cancel</button><button className="pill solid" disabled={pending || !request.projectLabel.trim() || !request.repositoryRoot}>{pending ? "Creating…" : "Create Project"}</button></div>
    </form>}
  </dialog>;
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
  return <dialog ref={dialogRef} className="command-dialog" aria-label="Search Projects and commands" onCancel={(event) => { event.preventDefault(); onClose(); }} onClick={(event) => { if (event.target === event.currentTarget) onClose(); }}><div className="command-search"><Search size={17} /><input ref={inputRef} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search Projects and commands…" aria-label="Search Projects and commands" /><button className="dialog-escape" onClick={onClose} aria-label="Close command palette">esc</button></div><div className="command-results"><p>Projects</p>{visible.map((project) => <button key={project.id} onClick={() => onSelectProject(project.id)}><span className="project-monogram">{project.name.slice(0, 1)}</span><span><strong>{project.name}</strong><small>{project.repositoryLabel}</small></span>{project.id === selectedProjectId && <Check size={15} />}</button>)}<p>Commands</p><button onClick={onSettings}><span className="settings-icon"><Settings2 size={15} /></span><span><strong>Open settings</strong><small>Appearance, devices, and privacy</small></span></button></div></dialog>;
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
