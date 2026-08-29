import { StrictMode, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { FormEvent, ReactNode } from "react";
import { createRoot } from "react-dom/client";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  Activity,
  AlertTriangle,
  Bot,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleDot,
  Code2,
  Command,
  Eye,
  FileCode2,
  FileText,
  GitBranch,
  Info,
  LayoutGrid,
  Moon,
  MessageSquare,
  Network,
  Pause,
  Play,
  Plus,
  Route,
  Search,
  Settings2,
  ShieldCheck,
  Sun,
  UserPlus,
  UserRound,
  Users,
  X,
  Zap,
} from "lucide-react";
import { FixtureProjectSource } from "./fixture-source";
import { emptyFixtureSession, fixtureSession, parseShellState } from "./fixtures";
import { LiveProjectSource, loadSession, loadSnapshot } from "./live-source";
import { DesktopOnboarding } from "./desktop-onboarding";
import { NewProjectScreen } from "./new-project";
import { PeopleScreen, SettingsScreen, initialsFor } from "./settings";
import { elapsedFromLabel, formatElapsed } from "./elapsed";
import type { DashboardSession, Finding, FindingFeedback, FindingState, LocalSessionDetail, MemberNameSource, ProjectAccess, ProjectSnapshot, SessionMessageKind, SessionMessagesSnapshot, ShellState, SyncCard, Workstream } from "./model";
import { nativeOnboarding, type EnrollmentRequest, type NativeOnboarding } from "./native";
import { fidelityLabel, semanticMessage, semanticModeMessage, stateMessage } from "./state";
import { VendorMark } from "./vendor-marks";
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
/** Settings, People and Add a Project are screens, not dialogs. */
type ScreenName = "settings" | "people" | "new-project";
const screenTitle: Record<ScreenName, string> = { settings: "Settings", people: "People", "new-project": "Add a Project" };

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

// A ticket can only be minted by the local Stickguy app, so this page can never
// activate itself. When a check finds the browser still has no session, say so
// and name the recovery instead of silently re-rendering an identical screen.
function ActivationView({ onActivate, stillInactive = false }: { onActivate: () => void; stillInactive?: boolean }) {
  return <main className="centered-shell"><Brand /><section className="state-card" aria-labelledby="activation-title"><span className="state-symbol"><ShieldCheck size={20} /></span><p className="eyebrow">Browser activation</p><h1 id="activation-title">Open your shared Project workroom.</h1><p>Your one-time access ticket is exchanged server-side. It is never stored in this page, activity, or browser history.</p><div className="disclosure"><strong>What teammates can see</strong><p>Session presence, action categories, safe repository paths, collisions, coordination decisions, and classifier-passing session messages while sharing is unpaused. Never source, diffs, prompts, transcripts, <code>.env</code> values, credentials, or raw tool output.</p></div>{stillInactive && <p role="alert">This browser still has no active session. Only the Stickguy app can issue a ticket, so reopen the Project from Stickguy Dev.app, then check again.</p>}<button className="pill solid" onClick={onActivate}>{stillInactive ? "Check again" : "Activate secure session"}</button><p className="microcopy">{stillInactive ? "Reopening the Project from the app mints a new one-time ticket." : "Sessions are revocable, same-site, and rotated after privilege changes."}</p></section></main>;
}

export function LiveApp() {
  const [state, setState] = useState<"activation" | "loading" | "ready" | "unauthorized" | "version_mismatch" | "offline">("loading");
  const [session, setSession] = useState<DashboardSession | null>(null);
  const [source, setSource] = useState<LiveProjectSource | null>(null);
  const [activationRechecked, setActivationRechecked] = useState(false);

  const load = async (retry = false) => {
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
      const next = status === 401 || status === 403 ? "activation" : status === 409 ? "version_mismatch" : "offline";
      // A retry that lands back on activation proves the browser still has no
      // session; without this the identical re-render reads as a dead button.
      if (next === "activation" && retry) setActivationRechecked(true);
      setState(next);
    }
  };
  useEffect(() => { void load(); }, []);
  useEffect(() => {
    if (!source || !session) return;
    return source.start(session.selectedProjectId);
  }, [source, session]);

  if (state === "loading") return <LoadingView />;
  if (state === "version_mismatch") return <TerminalState state="version_mismatch" />;
  if (state === "activation") return <ActivationView stillInactive={activationRechecked} onActivate={() => void load(true)} />;
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
  const [commandOpen, setCommandOpen] = useState(false);
  // A stack rather than a flag per screen, so "back" returns to whatever opened
  // it. People is reachable from the toolbar and from inside Settings, and it
  // must not always land in the same place.
  const [screens, setScreens] = useState<ScreenName[]>([]);
  const [projects, setProjects] = useState(session.projects);
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

  const screen = screens[screens.length - 1] ?? null;
  // Reached from the sidebar or the toolbar, a screen is top level and back
  // returns to the Project. Reached from inside another screen - People from
  // Settings - it stacks, so back returns to where the member actually was.
  const showScreen = (name: ScreenName) => setScreens([name]);
  const pushScreen = (name: ScreenName) => setScreens((stack) => [...stack, name]);
  const goBack = () => setScreens((stack) => stack.slice(0, -1));
  const previous = screens[screens.length - 2];
  const backLabel = previous ? screenTitle[previous] : view === "decisions" ? "Decisions" : snapshot.project.name;

  const selectProject = (nextId: string) => { setProjectId(nextId); setSelection(null); setView("workroom"); setScreens([]); setCommandOpen(false); };
  // Deleting or leaving a Project must actually leave it. Queuing the request
  // and staying put left the member reading a Project they no longer belong to.
  const removeProject = (removedId: string) => {
    const remaining = projects.filter((project) => project.id !== removedId);
    setProjects(remaining);
    setScreens([]);
    if (remaining.length > 0) selectProject(remaining[0]!.id);
  };
  const openSession = (id: string) => { setView("workroom"); setScreens([]); setSelection({ kind: "session", id }); };
  const showView = (next: View) => { setView(next); setScreens([]); };

  if (projects.length === 0) return <EmptyView />;

  const shellClass = ["workroom-shell", sidebarCollapsed ? "sidebar-collapsed" : "", screen ? "screen-open" : ""].filter(Boolean).join(" ");

  return <div className={shellClass}>
    <aside className="side">
      <div className="side-top">
        <Brand compact={sidebarCollapsed} />
        <button className="icon-button side-toggle" onClick={() => setSidebarCollapsed((value) => !value)} aria-label={sidebarCollapsed ? "Expand Projects sidebar" : "Collapse Projects sidebar"}>{sidebarCollapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}</button>
      </div>

      <button className="command-trigger" onClick={() => setCommandOpen(true)} aria-label="Search Projects and commands"><Search size={15} />{!sidebarCollapsed && <><span>Search</span><kbd>⌘K</kbd></>}</button>

      <div className="side-scroll">
        <button className="nav-item" aria-current={!screen && view === "workroom" ? "page" : undefined} onClick={() => showView("workroom")}>
          <LayoutGrid size={16} />{!sidebarCollapsed && <span>Workroom</span>}
          {!sidebarCollapsed && converging.length > 0 && <span className="nav-count">{converging.length}</span>}
        </button>
        <button className="nav-item" aria-current={!screen && view === "decisions" ? "page" : undefined} onClick={() => showView("decisions")}>
          <Check size={16} />{!sidebarCollapsed && <span>Decisions</span>}
        </button>

        <div className="side-group"><span className="side-label">Projects</span></div>
        {projects.map((project) => {
          const projectSnapshot = source.get(project.id);
          const collisionCount = projectSnapshot.findings.filter((finding) => finding.state === "open").length;
          return <button key={project.id} className="project-item" aria-current={project.id === projectId ? "page" : undefined} onClick={() => selectProject(project.id)} title={sidebarCollapsed ? project.name : undefined}>
            <span className="project-monogram">{project.name.slice(0, 1).toUpperCase()}</span>
            {!sidebarCollapsed && <>{project.name}{collisionCount > 0 && <span className="project-count">{collisionCount}</span>}</>}
          </button>;
        })}
        <button className="project-item new" aria-current={screen === "new-project" ? "page" : undefined} onClick={() => showScreen("new-project")} aria-label="Add a new Project"><span className="project-monogram"><Plus size={11} /></span>{!sidebarCollapsed && "New project"}</button>
      </div>

      <button className="profile-button" aria-current={screen === "settings" ? "page" : undefined} onClick={() => showScreen("settings")} aria-label="Open settings, devices, and privacy">
        <span className="avatar">{initialsFor(identity.name)}</span>
        {!sidebarCollapsed && <span className="who"><strong>{identity.name}</strong><small>Settings &amp; privacy</small></span>}
      </button>
    </aside>

    {screen === null && <>
      <main className="workroom-main">
        <div className="main-bar">
          <span className="spacer" />
          {!source.live && <button className="pill" disabled={offline} onClick={() => source.publishSyntheticUpdate(projectId)}><Zap size={14} />Simulate activity</button>}
          <button className="icon-button" onClick={() => setDark((value) => !value)} aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}>{dark ? <Sun size={16} /> : <Moon size={16} />}</button>
          <button className="icon-button" onClick={() => showScreen("settings")} aria-label="Open Project settings"><Settings2 size={16} /></button>
          <button className="pill" onClick={() => showScreen("people")} aria-label="Invite people to this Project"><UserPlus size={14} />Invite</button>
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
            {identity.source === "device" && !identityPromptDismissed && <div className="notice" role="status"><UserRound size={15} /><div className="body"><strong>Choose how teammates see you</strong>This Project is still showing your device name, “{identity.name}”. Pick a display name for your live work; the device name stays in Settings under Devices &amp; security.</div><div className="notice-actions"><button className="pill" onClick={() => showScreen("settings")}>Choose a name</button><button className="text-button" onClick={() => setIdentityPromptDismissed(true)}>Later</button></div></div>}
            {attention && <div className="notice alerting" role="alert"><AlertTriangle size={15} /><div className="body"><strong>Coordination update</strong>{attention.reason}</div><div className="notice-actions"><button className="pill" onClick={() => { setView("workroom"); setSelection({ kind: "collision", id: attention.id }); setAttention(null); }}>Review</button><button className="text-button" onClick={() => setAttention(null)}>Dismiss</button></div></div>}

            {view === "workroom"
              ? <WorkroomView
                  snapshot={snapshot} mine={mine} nearby={nearby} converging={converging} elsewhere={elsewhere}
                  convergingWorkstreams={convergingWorkstreams} selection={effectiveSelection} viewer={identity.name} tick={tick}
                  onSelectSession={openSession} onSelectFinding={(id) => setSelection({ kind: "collision", id })}
                />
              : <DecisionsView snapshot={snapshot} tick={tick} />}
          </div>
        </div>
      </main>

      <aside className="inspector" aria-label="Details inspector">
      {selectedSession
        ? <SessionInspector key={selectedSession.id} session={selectedSession} source={source} tick={tick} isViewer={selectedSession.memberName === identity.name} />
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
    </>}

    {screen === "settings" && <SettingsScreen
      snapshot={snapshot} dark={dark} identity={identity} projectId={projectId} source={source} offline={offline}
      backLabel={backLabel} onBack={goBack} onIdentity={setIdentity} onTheme={() => setDark((value) => !value)}
      onPeople={() => pushScreen("people")} onRemoved={() => removeProject(projectId)}
    />}
    {screen === "people" && <PeopleScreen projectId={projectId} projectName={snapshot.project.name} source={source} offline={offline} backLabel={backLabel} onBack={goBack} />}
    {screen === "new-project" && <NewProjectScreen api={nativeApi} displayName={identity.source === "member" ? identity.name : ""} navigate={navigate} backLabel={backLabel} onBack={goBack} />}

    {commandOpen && <CommandPalette projects={projects} selectedProjectId={projectId} onSelectProject={selectProject} onSettings={() => { setCommandOpen(false); showScreen("settings"); }} onClose={() => setCommandOpen(false)} />}
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
    <div className="block-head lead"><h2>Converging on you</h2>{converging.length > 0 && <span className="count hot">{converging.length}</span>}</div>
    <SemanticStatus status={snapshot.project.semanticStatus} mode={snapshot.project.semanticMode} />
    {converging.length === 0
      ? <p className="block-empty">Nothing is reaching your work right now.</p>
      : converging.map((finding) => <ConvergeBlock key={finding.id} finding={finding} sessions={snapshot.workstreams} viewer={viewer} tick={tick} selected={selection?.kind === "collision" && selection.id === finding.id} onSelect={() => onSelectFinding(finding.id)} onOpenSession={onSelectSession} />)}

    <div className="block-head ambient"><h2>Your sessions</h2><span className="count">{mine.length}</span></div>
    {mine.length === 0
      ? <p className="block-empty">No sessions are registered to you in this Project yet.</p>
      : <div className="rows">{mine.map((stream) => <SessionRow key={stream.id} session={stream} tick={tick} converging={convergingWorkstreams.has(stream.id)} selected={selection?.kind === "session" && selection.id === stream.id} onClick={() => onSelectSession(stream.id)} />)}</div>}

    <div className="block-head ambient"><h2>Nearby</h2><span className="count">{nearby.length}</span></div>
    {nearby.length === 0
      ? <p className="block-empty">No teammates are registered to this Project yet.</p>
      : <div className="rows">{nearby.map((stream) => <PersonRow key={stream.id} session={stream} tick={tick} onClick={() => onSelectSession(stream.id)} />)}</div>}

    {elsewhere.length > 0 && <>
      <div className="block-head ambient"><h2>Elsewhere in the Project</h2><span className="count">{elsewhere.length}</span></div>
      {elsewhere.map((finding) => <ConvergeBlock key={finding.id} finding={finding} sessions={snapshot.workstreams} viewer={viewer} tick={tick} quiet selected={selection?.kind === "collision" && selection.id === finding.id} onSelect={() => onSelectFinding(finding.id)} onOpenSession={onSelectSession} />)}
    </>}

  </>;
}

function DecisionsView({ snapshot, tick }: { snapshot: ProjectSnapshot; tick: number }) {
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

    {snapshot.activity.length > 0 && <>
      <div className="block-head ambient"><h2>Activity</h2></div>
      <ol className="timeline">{snapshot.activity.map((item) => <li key={item.id}><p><strong>{item.actor}</strong> {item.summary}</p><span className="src"><Since label={item.at} tick={tick} /> · {activitySourceLabel(item.fidelity)}</span></li>)}</ol>
    </>}
  </>;
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
      {path && <span className={isLive(session) ? "session-files live" : "session-files"}><span className="p path-swap" key={path}>{path}</span><span className="c">{session.pathCount.toLocaleString()} {session.pathCount === 1 ? "file" : "files"}</span></span>}
      {activeSubagents.map((agent) => <span className="session-sub" key={agent.alias}><b>{agent.agentType || "subagent"}</b> subagent · {agent.status}</span>)}
    </span>
    <span className="session-right">
      {converging && <span className="session-warn" title="Converging with another session"><AlertTriangle size={14} /></span>}
      <Elapsed label={session.updatedLabel} tick={tick} />
      <span className="chev"><ChevronRight size={15} /></span>
    </span>
  </button>;
}

/** For a teammate the useful fact is intent - what they are about to do.
 *  Which agent is doing it is the next question, and reading it off a name is
 *  impossible, so the vendor is a mark rather than another word of prose. */
function PersonRow({ session, tick, onClick }: { session: Workstream; tick: number; onClick: () => void }) {
  return <button className={`person-row ${session.presence}`} onClick={onClick} aria-label={`Open ${vendorLabel(session)} session for ${session.memberName}`}>
    <span className="person-icon" title={vendorLabel(session)}>{session.agent ? <VendorMark vendor={session.agent.vendor} size={15} /> : <Code2 size={14} />}</span>
    <span className="nm">{session.memberName}</span>
    <span className="intent"><LiveAction session={session} /></span>
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

function SessionInspector({ session, source, tick, isViewer }: { session: Workstream; source: FixtureProjectSource; tick: number; isViewer: boolean }) {
  const [shared, setShared] = useState<SessionMessagesSnapshot | null>(null);
  const [own, setOwn] = useState<LocalSessionDetail | null>(null);
  const [messageError, setMessageError] = useState("");
  const [detailsOpen, setDetailsOpen] = useState(false);
  const detailsAnchorRef = useRef<HTMLDivElement>(null);
  const subagents = session.agent?.subagents ?? [];
  const path = currentPath(session);
  const showPath = path && !session.outcome.includes(path);
  const complete = session.agent?.status === "done";

  useEffect(() => {
    let cancelled = false;
    if (!session.agent) { setShared(null); setOwn(null); return; }
    const refresh = () => {
      void source.getSessionMessages(session.id).then((value) => { if (!cancelled) { setShared(value); setMessageError(""); } }).catch(() => { if (!cancelled) setMessageError("Session messages are unavailable."); });
      // Own-session content is read locally and never uploaded, so it loads
      // whether or not this session is shared.
      void source.getLocalSession(session.id).then((value) => { if (!cancelled) setOwn(value); }).catch(() => { if (!cancelled) setOwn(null); });
    };
    refresh();
    const timer = window.setInterval(refresh, 3_000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [session.id, source, session.agent]);

  useEffect(() => {
    if (!detailsOpen) return;
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape") setDetailsOpen(false); };
    const closeOutside = (event: PointerEvent) => {
      if (detailsAnchorRef.current && event.target instanceof Node && !detailsAnchorRef.current.contains(event.target)) setDetailsOpen(false);
    };
    window.addEventListener("keydown", closeOnEscape);
    document.addEventListener("pointerdown", closeOutside);
    return () => {
      window.removeEventListener("keydown", closeOnEscape);
      document.removeEventListener("pointerdown", closeOutside);
    };
  }, [detailsOpen]);

  // Prefer the member's own local transcript; Project members see the
  // classifier-passing projection for teammate sessions (ADR-047).
  const mine = (own?.messages ?? []).length > 0;
  const conversation: TranscriptMessage[] = mine
    ? (own?.messages ?? []).map((message, index) => ({ id: `own-${index}`, kind: message.kind, text: message.text, tool: message.tool, at: message.at }))
    : (shared?.messages ?? []).map((message) => ({ id: message.id, kind: message.kind, text: message.text, at: message.capturedAt }));
  const feed = sessionTimeline(conversation, session);
  const title = session.agent?.sessionTitle ?? own?.title ?? session.title;
  const branch = session.agent?.branch ?? own?.branch;
  const liveFacts = [session.agent?.tool, showPath ? path : undefined].filter((value): value is string => Boolean(value)).join(" · ");

  return <>
    <div className="inspector-bar session-inspector-bar">
      <span className="inspector-vendor" aria-hidden="true">{session.agent ? <VendorMark vendor={session.agent.vendor} size={19} /> : <Code2 size={18} />}</span>
      <span className="grow">
        <h2>{title}</h2>
        <div className="sub">{session.memberName} · {vendorLabel(session)}</div>
        {branch && <div className="inspector-status"><GitBranch size={12} aria-hidden="true" /><code>{branch}</code></div>}
      </span>
      <div className="session-header-actions" ref={detailsAnchorRef}>
        {complete && <span className="session-complete"><Check size={12} aria-hidden="true" />Complete</span>}
        <button className="icon-button session-details-button" aria-label="Open session details" aria-expanded={detailsOpen} onClick={() => setDetailsOpen((open) => !open)}><Info size={15} /></button>
        {detailsOpen && <SessionDetailsPanel session={session} mine={mine} subagents={subagents} path={path} onClose={() => setDetailsOpen(false)} />}
      </div>
    </div>
    {!complete && <div className="session-live" aria-label="Current session activity" aria-live="polite">
      <span className="session-live-icon" aria-hidden="true"><Activity size={15} /></span>
      <span className="session-live-copy"><span className="session-live-facts"><small>{statusCopy(session)}</small>{liveFacts && <code>{liveFacts}</code>}</span><strong><LiveAction session={session} /></strong></span>
      <Elapsed label={session.updatedLabel} tick={tick} />
    </div>}
    <div className="inspector-body chat-inspector-body">
      {messageError && <p className="form-error" role="alert">{messageError}</p>}
      {feed.length > 0
        ? <ol className="session-thread">{feed.map((item) => <SessionFeedRow key={item.id} item={item} session={session} isViewer={isViewer} tick={tick} />)}</ol>
          : <div className="conversation-empty"><MessageSquare size={18} /><div><strong>{session.agent ? "This session has not said anything yet." : "No agent conversation is available."}</strong><p>{session.outcome}</p></div></div>}
    </div>
  </>;
}

type TranscriptMessage = { id: string; kind: SessionMessageKind | "tool"; text?: string; tool?: string; at?: string };
type SessionFeedItem =
  | { id: string; kind: SessionMessageKind; text?: string; at?: string }
  | { id: string; kind: "tool_group"; tools: string[]; at?: string }
  | { id: string; kind: "lifecycle"; event: "started" | "ended" | "status"; label: string; detail?: string; at?: string }
  | { id: string; kind: "coordination"; state: "routed" | "considered"; summary: string; itemCount: number; trigger: string; at: string }
  | { id: string; kind: "parallel"; label: string; detail: string; at?: string }
  | { id: string; kind: "activity"; action: string; tool?: string; path?: string; activityKind: string; elapsedLabel: string; at?: string };

type TimelineSeed = SessionFeedItem | { id: string; kind: "tool"; tool: string; at?: string };

function sessionTimeline(messages: TranscriptMessage[], session: Workstream): SessionFeedItem[] {
  const activity = session.agent?.activity ?? [];
  const seeds: Array<TimelineSeed & { order: number }> = [];
  let order = 0;
  const add = (item: TimelineSeed) => seeds.push({ ...item, order: order++ });
  const startedAt = session.agent?.startedAt ?? activity.find((item) => item.kind === "SessionStart")?.occurredAt;
  const endedAt = session.agent?.endedAt ?? activity.find((item) => item.kind === "SessionEnd")?.occurredAt;

  if (startedAt) add({ id: "session-started", kind: "lifecycle", event: "started", label: "Session started", at: startedAt });
  for (const message of messages) {
    if (message.kind === "tool") add({ id: message.id, kind: "tool", tool: message.tool || "Tool", at: message.at });
    else add({ id: message.id, kind: message.kind, text: message.text, at: message.at });
  }

  for (const delivery of session.agent?.coordination ?? []) {
    add({ id: `routed-${delivery.id}`, kind: "coordination", state: "routed", summary: delivery.summary, itemCount: delivery.itemCount, trigger: delivery.trigger, at: delivery.routedAt });
    if (delivery.acknowledgedAt) add({ id: `considered-${delivery.id}`, kind: "coordination", state: "considered", summary: delivery.summary, itemCount: delivery.itemCount, trigger: delivery.trigger, at: delivery.acknowledgedAt });
  }

  const hasConversation = messages.length > 0;
  for (const item of activity) {
    if (item.kind === "SessionStart" || item.kind === "SessionEnd") continue;
    if (item.kind === "PermissionRequest" || item.kind === "Stop") {
      const detail = [item.tool, item.paths[0]].filter((value): value is string => Boolean(value)).join(" · ");
      add({ id: `lifecycle-${item.id}`, kind: "lifecycle", event: "status", label: item.action, detail: detail || undefined, at: item.occurredAt });
      continue;
    }
    if (item.kind === "SubagentStart" || item.kind === "SubagentStop") {
      add({ id: `parallel-${item.id}`, kind: "parallel", label: item.action, detail: item.kind === "SubagentStop" ? "Parallel work finished" : "Working in parallel", at: item.occurredAt });
      continue;
    }
    if (!hasConversation) add({ id: `activity-${item.id}`, kind: "activity", action: item.action, tool: item.tool, path: item.paths[0], activityKind: item.kind, elapsedLabel: item.at, at: item.occurredAt });
  }

  if (!(activity.some((item) => item.kind === "SubagentStart" || item.kind === "SubagentStop"))) {
    for (const agent of session.agent?.subagents ?? []) add({ id: `parallel-current-${agent.alias}`, kind: "parallel", label: `${parallelAgentRole(agent.agentType)} working in parallel`, detail: parallelAgentStatus(agent.status) });
  }
  if (endedAt || session.agent?.status === "done") add({ id: "session-ended", kind: "lifecycle", event: "ended", label: "Session ended", at: endedAt });

  seeds.sort((left, right) => {
    const leftTime = timelineTime(left.at);
    const rightTime = timelineTime(right.at);
    if (leftTime !== null && rightTime !== null && leftTime !== rightTime) return leftTime - rightTime;
    if (leftTime !== null && rightTime === null) return -1;
    if (leftTime === null && rightTime !== null) return 1;
    return left.order - right.order;
  });

  const grouped: SessionFeedItem[] = [];
  for (const { order: _order, ...item } of seeds) {
    if (item.kind !== "tool") {
      grouped.push(item);
      continue;
    }
    const previous = grouped[grouped.length - 1];
    if (previous?.kind === "tool_group") previous.tools.push(item.tool);
    else grouped.push({ id: `tools-${item.id}`, kind: "tool_group", tools: [item.tool], at: item.at });
  }
  return grouped;
}

function timelineTime(value: string | undefined): number | null {
  if (!value) return null;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

function SessionFeedRow({ item, session, isViewer, tick }: { item: SessionFeedItem; session: Workstream; isViewer: boolean; tick: number }) {
  if (item.kind === "tool_group") return <li className="thread-tool"><span className="thread-tool-icon"><Command size={12} /></span><span><strong title={item.tools.join(" → ")}>{item.tools.join(" → ")}</strong><small>{item.tools.length === 1 ? "Tool activity" : `${item.tools.length} tool actions`}{item.at ? ` · ${sessionMessageTime(item.at)}` : ""}</small></span></li>;
  if (item.kind === "thinking") return <li className="thread-thinking"><details><summary><span><Bot size={13} />Thinking</span>{item.at && <small>{sessionMessageTime(item.at)}</small>}</summary><MarkdownMessage text={item.text ?? ""} /></details></li>;
  if (item.kind === "user" || item.kind === "assistant" || item.kind === "system") return <li className={`thread-message ${item.kind}`}><header><span className="message-icon" aria-hidden="true"><SessionMessageIcon kind={item.kind} vendor={session.agent?.vendor} /></span><strong>{item.kind === "assistant" ? vendorLabel(session) : messageKindLabel(item.kind)}</strong>{item.at && <small>{sessionMessageTime(item.at)}</small>}</header><MarkdownMessage text={item.text ?? ""} /></li>;
  if (item.kind === "lifecycle") return <li className={`thread-boundary ${item.event}`}><span className="thread-event-icon" aria-hidden="true">{item.event === "started" ? <Play size={12} /> : item.event === "ended" ? <Check size={12} /> : <Activity size={12} />}</span><span><strong>{item.label}</strong>{item.detail && <small>{item.detail}</small>}</span>{item.at && <time>{sessionMessageTime(item.at)}</time>}</li>;
  if (item.kind === "coordination") return <li className={`thread-coordination ${isViewer && item.state === "routed" ? "converging" : ""}`}><span className="thread-event-icon" aria-hidden="true">{item.state === "routed" ? <Route size={13} /> : <Check size={12} />}</span><span><header><strong>{item.state === "routed" ? "Coordination routed" : "Agent considered coordination"}</strong><time>{sessionMessageTime(item.at)}</time></header>{item.state === "routed" ? <p>{item.summary}</p> : <p>Consideration recorded; this does not prove the agent followed it.</p>}<small>{item.state === "routed" ? `${coordinationTriggerLabel(item.trigger)} · ` : ""}{item.itemCount} {item.itemCount === 1 ? "item" : "items"}</small></span></li>;
  if (item.kind === "parallel") return <li className="thread-parallel"><span className="thread-tool-icon"><Network size={12} /></span><span><strong>{item.label}</strong><small>{item.detail}{item.at ? ` · ${sessionMessageTime(item.at)}` : ""}</small></span></li>;
  if (item.kind !== "activity") return null;
  return <li className="thread-tool activity-item"><span className="thread-tool-icon"><Activity size={12} /></span><span><strong>{item.action}</strong><small>{item.tool ? `${item.tool} · ` : ""}{item.path ?? item.activityKind} · {item.at ? sessionMessageTime(item.at) : <Elapsed label={item.elapsedLabel} tick={tick} />}</small></span></li>;
}

function coordinationTriggerLabel(value: string): string {
  return ({ user_prompt_submit: "Next turn", session_start: "Session start", before_broad_edit: "Before broad edit", checkpoint: "Checkpoint", mcp: "Agent check" } as Record<string, string>)[value] ?? value.replaceAll("_", " ");
}

function SessionDetailsPanel({ session, mine, subagents, path, onClose }: { session: Workstream; mine: boolean; subagents: NonNullable<Workstream["agent"]>["subagents"]; path?: string; onClose: () => void }) {
  return <section className="session-details-popover" aria-label="Session details">
    <header><span><strong>Session details</strong><small>{session.pathCount.toLocaleString()} {session.pathCount === 1 ? "file" : "files"}</small></span><button className="icon-button" onClick={onClose} aria-label="Close session details"><X size={14} /></button></header>
    <div className="session-details-scroll">
      <div className="conversation-disclosure"><ShieldCheck size={15} aria-hidden="true" /><p>{mine ? "Read from this machine. Classifier-passing messages are visible to Project members while sharing is active." : session.agent ? `Shared by ${session.memberName} after classification.` : "This workstream has no agent conversation."}</p></div>
      <InspectorHeading icon={<Eye size={13} />}>How this session is connected</InspectorHeading>
      <div className="coverage-list">
        <CoverageRow icon={<Eye size={14} />} label="Source" value={fidelityLabel(session.fidelity)} detail={`${fidelityDetail(session)} ${session.pathCount > 0 ? `${session.pathCount.toLocaleString()} safe ${session.pathCount === 1 ? "path is" : "paths are"} ${session.agent?.capabilities.observeSafePaths ? "observed" : "reported"}.` : "No safe paths are reported yet."}`} />
        {session.agent && <CoverageRow icon={<FileText size={14} />} label="Coordination" value={`${briefDeliveryLabel(session.agent.capabilities.deliverBrief)} · ${session.agent.capabilities.requestAttention === "advisory" ? "advisory" : "dashboard"}`} detail="Context is routed at supported turn boundaries; Stickguy never interrupts an agent mid-turn." />}
        {session.agent && <CoverageRow icon={<FileCode2 size={14} />} label="Contract drift" value={readCoverageLabel(session.agent.capabilities.observeReadSet)} detail={readCoverageDetail(session.agent.capabilities.observeReadSet)} />}
        {session.agent?.sessionAlias && <CoverageRow icon={<Bot size={14} />} label="Session ID" value={session.agent.sessionAlias} machine detail={`${vendorLabel(session)} session identifier.`} />}
        {subagents.map((agent) => <CoverageRow key={agent.alias} icon={<Network size={14} />} label="Parallel ID" value={agent.alias} machine detail={`${parallelAgentRole(agent.agentType)} · ${parallelAgentStatus(agent.status)}.`} />)}
      </div>

      {session.largeChange && <section><InspectorHeading icon={<FileText size={13} />}>Change scope</InspectorHeading><dl className="facts"><dt>paths</dt><dd>{session.largeChange.pathCount.toLocaleString()} paths</dd><dt>summary</dt><dd>{session.largeChange.summary}</dd><dt>revision</dt><dd>manifest {session.largeChange.revision} <span className="q">· size alone does not imply risk</span></dd></dl></section>}

      <section className="session-files-section">
        <InspectorHeading icon={<FileCode2 size={13} />} count={session.pathCount}>Files this session</InspectorHeading>
        {session.paths.length > 0
          ? <div className="file-list">{session.paths.map((entry) => <div className={entry === path ? "file-row hot" : "file-row"} key={entry}><span className="p">{entry}</span><span className="v">{entry === path && isLive(session) ? "touching now" : "reported"}</span></div>)}{session.pathCount > session.paths.length && <div className="file-row"><span className="p">{(session.pathCount - session.paths.length).toLocaleString()} more</span><span className="v">summarized</span></div>}</div>
          : <p className="muted-copy">No safe paths have been reported yet.</p>}
      </section>
    </div>
  </section>;
}

function MarkdownMessage({ text }: { text: string }) {
  return <div className="markdown-message"><ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml components={{
    a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noreferrer" />,
    img: ({ node: _node, alt }) => <span className="markdown-image">Image omitted{alt ? ` · ${alt}` : ""}</span>,
  }}>{text}</ReactMarkdown></div>;
}

function parallelAgentRole(value: string): string {
  const role = value.trim() || "Subagent";
  return `${role.slice(0, 1).toUpperCase()}${role.slice(1)}`;
}

function parallelAgentStatus(value: string): string {
  return ({ active: "Active now", waiting: "Waiting", idle: "Recently active", done: "Finished", error: "Needs attention" } as Record<string, string>)[value] ?? value.replaceAll("_", " ");
}

function InspectorHeading({ icon, count, children }: { icon: ReactNode; count?: number; children: ReactNode }) {
  return <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true">{icon}</span><span>{children}</span>{typeof count === "number" && <span className="inspector-head-count">{count.toLocaleString()}</span>}</h3>;
}

function CoverageRow({ icon, label, value, detail, machine = false }: { icon: ReactNode; label: string; value: string; detail?: string; machine?: boolean }) {
  return <div className="coverage-row"><span className="coverage-icon" aria-hidden="true">{icon}</span><span className="coverage-copy"><span className="coverage-label">{label}</span><strong className={machine ? "machine" : undefined}>{value}</strong>{detail && <small>{detail}</small>}</span></div>;
}

function fidelityDetail(session: Workstream): string {
  if (session.fidelity === "hook") return "Connected agent events provide live session and activity detail.";
  if (session.fidelity === "hook_unverified") return "The agent binding is configured but runtime delivery is not verified.";
  if (session.fidelity === "git") return "Repository observation provides change scope without agent activity.";
  if (session.fidelity === "mcp") return "The coding agent reported this work through the lifecycle protocol.";
  return "A Project member reported this intent manually.";
}

type ReadCoverage = NonNullable<Workstream["agent"]>["capabilities"]["observeReadSet"];

function readCoverageLabel(coverage: ReadCoverage): string {
  return ({ observed: "Observed", vendor_inferred: "Partial", self_declared: "Declared only", none: "Not observed" } as const)[coverage];
}

// Says plainly what silence means for this session. A session whose reads are
// not observed can never receive a stale-assumption finding, and the operator
// has to know that rather than reading quiet as safe (ADR-052).
function readCoverageDetail(coverage: ReadCoverage): string {
  if (coverage === "observed") return "File reads are observed, so this session is told when a contract it read changes underneath it.";
  if (coverage === "vendor_inferred") return "File reads are inferred from the vendor's own classification of the commands it ran, so some reads are missed.";
  if (coverage === "self_declared") return "Only the paths this session declared are known; reads it did not declare are invisible.";
  return "Nothing observes this session's file reads, so it is never told when a contract it read changes. Silence here is missing evidence, not an all-clear.";
}

function briefDeliveryLabel(delivery: NonNullable<Workstream["agent"]>["capabilities"]["deliverBrief"]): string {
  return ({ mcp_pull: "MCP pull", native_pull: "Agent pull", native_push: "Next-turn push", unavailable: "Unavailable" } as const)[delivery];
}

function SessionMessageIcon({ kind, vendor }: { kind: SessionMessageKind; vendor?: "codex" | "claude" }) {
  if (kind === "assistant" && vendor) return <VendorMark vendor={vendor} size={14} />;
  if (kind === "user") return <UserRound size={14} />;
  if (kind === "system") return <ShieldCheck size={14} />;
  return <Bot size={14} />;
}

function sessionMessageTime(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
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
  onState: (state: FindingState) => Promise<void>; onFeedback: (value: FindingFeedback) => Promise<void>; onOpenSession: (id: string) => void;
}) {
  const affected = useMemo(() => sessions.filter((session) => finding.workstreamIds.includes(session.id)), [finding.workstreamIds, sessions]);
  const [feedback, setFeedback] = useState<FindingFeedback | null>(null);
  const [feedbackError, setFeedbackError] = useState(false);
  const [feedbackPending, setFeedbackPending] = useState(false);
  const [statePending, setStatePending] = useState(false);
  const [stateError, setStateError] = useState(false);
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
  const submitState = (state: FindingState) => {
    setStatePending(true);
    setStateError(false);
    void onState(state).catch(() => setStateError(true)).finally(() => setStatePending(false));
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
      {/*
        * Resolving is not offered here: recording the decision on the sync card
        * above is what resolves a collision, so a button would settle the work
        * with no record of how. Acknowledge and dismiss are the member's own.
        */}
      <div className="finding-actions">
        <button disabled={disabled || statePending || finding.state === "acknowledged"} className="pill solid" onClick={() => submitState("acknowledged")}>Acknowledge</button>
        <button disabled={disabled || statePending || finding.state === "dismissed"} className="pill" onClick={() => submitState("dismissed")}>Dismiss</button>
      </div>
      {stateError && <p className="form-error" role="alert">That change could not be saved.</p>}
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

function statusCopy(session: Workstream): string {
  const status = session.agent?.status ?? session.presence;
  if (status === "active" || status === "online") return "Working now";
  if (status === "waiting") return "Waiting for input";
  if (status === "done") return "Complete";
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
