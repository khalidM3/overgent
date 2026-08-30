import { StrictMode, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { CSSProperties, FormEvent, ReactNode } from "react";
import { createRoot } from "react-dom/client";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  Activity,
  AlertTriangle,
  ArrowUp,
  BellOff,
  Bot,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleDot,
  Code2,
  Command,
  Copy,
  Eye,
  ExternalLink,
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
import { attentionItems, newestEventTime, orderSessions, type AttentionItem, type HealthSignal, type OrderedSessions } from "./attention";
import { FixtureProjectSource } from "./fixture-source";
import { emptyFixtureSession, fixtureSession, parseShellState } from "./fixtures";
import { LiveProjectSource, loadSession, loadSnapshot } from "./live-source";
import { DesktopOnboarding } from "./desktop-onboarding";
import { NewProjectScreen } from "./new-project";
import { PeopleScreen, SettingsScreen, initialsFor, memberHue } from "./settings";
import { elapsedFromLabel, formatElapsed } from "./elapsed";
import type { AgentVendor, DashboardSession, Finding, FindingFeedback, FindingState, LocalSessionDetail, MemberNameSource, ProjectAccess, ProjectSnapshot, ScopeSnapshot, ScopeSnapshotFact, ScopeSnapshotField, SessionFocus, SessionMessageKind, SessionMessagesSnapshot, ShellState, SyncCard, Workstream } from "./model";

/** How each connected vendor is named in the interface. */
const VENDOR_LABELS: Readonly<Record<AgentVendor, string>> = { codex: "Codex", claude: "Claude Code", cursor: "Cursor" };
import { nativeOnboarding, type EnrollmentRequest, type NativeOnboarding, type NativeSessionOpenResult } from "./native";
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
type View = "workroom" | "history";
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
/**
 * Reached when the hosted API answers 401: this browser holds no session.
 *
 * The button used to say "Activate secure session", which this page cannot do.
 * Only the Stickguy app can mint a ticket, so the control is a re-check and
 * nothing more, and a reader with no session could press it forever. The
 * recovery was written on the page too — but only *after* the first press
 * failed, so the one instruction that resolves the state was hidden behind the
 * dead end it explains.
 *
 * The recovery is now stated before the control, the control says what it
 * actually does, and pressing it confirms rather than reveals.
 */
function ActivationView({ onActivate, stillInactive = false }: { onActivate: () => void; stillInactive?: boolean }) {
  return <main className="centered-shell"><Brand /><section className="state-card" aria-labelledby="activation-title"><span className="state-symbol"><ShieldCheck size={20} /></span><p className="eyebrow">Browser activation</p><h1 id="activation-title">This browser has no Stickguy session yet.</h1><p>A session can only be minted by the Stickguy app on this Mac, and the one-time ticket is exchanged server-side — it is never stored in this page, its activity, or browser history.</p><div className="disclosure"><strong>To open the workroom</strong><p>Open the Project from the Stickguy app, or run this in your checkout:</p><code>stickguy dashboard --project &lt;project-id&gt;</code></div><div className="disclosure"><strong>What teammates can see</strong><p>Session presence, action categories, safe repository paths, collisions, coordination decisions, and classifier-passing session messages while sharing is unpaused. Never source, diffs, prompts, transcripts, <code>.env</code> values, credentials, or raw tool output.</p></div>{stillInactive && <p role="alert">This browser still has no active session. Reopen the Project from Stickguy Dev.app to mint a new one-time ticket, then check again.</p>}<button className="pill solid" onClick={onActivate}>Check for a session</button><p className="microcopy">Sessions are revocable, same-site, and rotated after privilege changes.</p></section></main>;
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
  // A Project is worth creating before anyone else is in it: two of your own
  // agent sessions in one repository already collide with each other, and that
  // is the case Stickguy was built for. Joining someone else's is the second
  // sentence rather than half the first.
  return <main className="centered-shell"><Brand /><section className="state-card"><span className="state-symbol"><GitBranch size={20} /></span><p className="eyebrow">No Projects</p><h1>{stateMessage("empty")}</h1><p>Point Stickguy at a repository and it starts coordinating the agent sessions you run in it, on your own. Invite people later, or join an existing Project with an invite code.</p><code>stickguy create</code></section></main>;
}

/**
 * A thread that opens at its newest entry and stays there until the reader
 * leaves.
 *
 * The session feed is a stream: it runs oldest to newest and the interesting
 * end is the bottom. Opening one at the top asked the reader to scroll through
 * history they had usually already seen to reach the only part that was moving.
 *
 * Following stops the moment the reader scrolls up, because yanking the view
 * back while someone is reading is worse than making them press a control to
 * return, and it resumes on its own when they scroll back down to the end.
 * `key` is whatever should force a fresh landing - the session being inspected -
 * so switching sessions always arrives at that session's newest entry rather
 * than inheriting the last one's scroll position.
 */
const TAIL_SLACK_PX = 24;

function useFollowTail(key: string, depth: number) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [following, setFollowing] = useState(true);

  const scrollToEnd = () => {
    const element = scrollRef.current;
    if (element) element.scrollTop = element.scrollHeight;
  };

  // Landing is per session, so it runs on the key rather than on every new
  // entry; growth while following is handled below.
  useEffect(() => {
    setFollowing(true);
    scrollToEnd();
  }, [key]);

  useEffect(() => {
    if (following) scrollToEnd();
  }, [depth, following]);

  const onScroll = () => {
    const element = scrollRef.current;
    if (!element) return;
    // jsdom and a thread shorter than its container both report zero scroll
    // height, which is the tail by definition rather than a reader who has
    // scrolled away.
    const distance = element.scrollHeight - element.clientHeight - element.scrollTop;
    setFollowing(distance <= TAIL_SLACK_PX);
  };

  const jumpToNow = () => {
    setFollowing(true);
    scrollToEnd();
  };

  return { scrollRef, following, onScroll, jumpToNow };
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
  /* A trail rather than one value, so opening a session from inside a collision
      can return to the collision that sent you there. Picking something from a
      list starts a new trail; drilling in from the inspector pushes onto it. */
  const [trail, setTrail] = useState<Selection[]>([]);
  const selection = trail[trail.length - 1] ?? null;
  const setSelection = (next: Selection | null) => setTrail(next ? [next] : []);
  const pushSelection = (next: Selection) => setTrail((current) => [...current, next]);
  const popSelection = () => setTrail((current) => current.slice(0, -1));
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
  // Pause and focus are local-service state. Probing once tells the toolbar
  // whether to offer a control and the paused notice whether to name a
  // recovery, so the two can never disagree about what this page can do.
  const localPause = useLocalControl(source);

  const mine = useMemo(() => snapshot.workstreams.filter((stream) => stream.memberName === identity.name), [snapshot.workstreams, identity.name]);
  const nearby = useMemo(() => snapshot.workstreams.filter((stream) => stream.memberName !== identity.name).sort((left, right) => presenceRank(left) - presenceRank(right)), [snapshot.workstreams, identity.name]);
  const mineIds = useMemo(() => new Set(mine.map((stream) => stream.id)), [mine]);
  const openFindings = snapshot.findings.filter((finding) => finding.state === "open");
  // Only what reaches your own work is yours to act on; the rest stays quiet.
  const elsewhere = openFindings.filter((finding) => !finding.workstreamIds.some((id) => mineIds.has(id)));
  // Fixture snapshots are authored at fixed times, so wall-clock arithmetic over
  // them would report every session as silent for months. Live snapshots carry
  // real event times and are measured against the real clock.
  const now = source.live ? Date.now() : newestEventTime(snapshot) ?? Date.now();
  const needsYou = useMemo(() => attentionItems(snapshot, mineIds, now), [snapshot, mineIds, now]);
  const converging = useMemo(() => needsYou.flatMap((item) => (item.kind === "finding" ? [item.finding] : [])), [needsYou]);
  const convergingWorkstreams = useMemo(() => new Set(converging.flatMap((finding) => finding.workstreamIds)), [converging]);
  // Ranked by what you would do about it, then by the same elapsed label the
  // row shows, so the order always agrees with the clock beside it.
  const mySessions = useMemo(() => orderSessions(mine, now, (session) => elapsedFromLabel(session.updatedLabel) ?? Number.POSITIVE_INFINITY), [mine, now]);

  const defaultSession = mine.find((stream) => stream.agent?.status === "active") ?? mine[0] ?? snapshot.workstreams[0];
  const effectiveSelection: Selection | null = selection ?? (defaultSession ? { kind: "session", id: defaultSession.id } : null);
  const selectedSession = effectiveSelection?.kind === "session" ? snapshot.workstreams.find((stream) => stream.id === effectiveSelection.id) ?? null : null;
  const selectedCollision = effectiveSelection?.kind === "collision" ? snapshot.findings.find((finding) => finding.id === effectiveSelection.id) ?? null : null;
  const previousCollision = trail.length > 1 && trail[trail.length - 2]?.kind === "collision"
    ? snapshot.findings.find((finding) => finding.id === trail[trail.length - 2]!.id) ?? null
    : null;
  const selectedSessionFinding = selectedSession
    ? previousCollision ?? openFindings.find((finding) => finding.workstreamIds.includes(selectedSession.id)) ?? null
    : null;
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
  const backLabel = previous ? screenTitle[previous] : view === "history" ? "History" : snapshot.project.name;

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
  // Reached from inside a collision, so back returns to the collision.
  const drillIntoSession = (id: string) => { setView("workroom"); setScreens([]); pushSelection({ kind: "session", id }); };
  const previousSelection = trail.length > 1 ? trail[trail.length - 2]! : null;
  const inspectorBack = previousSelection
    ? {
        label: previousSelection.kind === "collision"
          ? snapshot.findings.find((finding) => finding.id === previousSelection.id)?.title ?? "collision"
          : snapshot.workstreams.find((stream) => stream.id === previousSelection.id)?.agent?.sessionTitle ?? "session",
        onBack: popSelection,
      }
    : null;
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
          {!sidebarCollapsed && needsYou.length > 0 && <span className="nav-count">{needsYou.length}</span>}
        </button>
        <button className="nav-item" aria-current={!screen && view === "history" ? "page" : undefined} onClick={() => showView("history")}>
          <Route size={16} />{!sidebarCollapsed && <span>History</span>}
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
        <MemberChip name={identity.name} size="large" />
        {!sidebarCollapsed && <span className="who"><strong>{identity.name}</strong><small>Settings &amp; privacy</small></span>}
      </button>
    </aside>

    {screen === null && <>
      <main className="workroom-main">
        <div className="main-bar">
          <span className="spacer" />
          {!source.live && <button className="pill" disabled={offline} onClick={() => source.publishSyntheticUpdate(projectId)}><Zap size={14} />Simulate activity</button>}
          <PauseControl source={source} projectId={projectId} paused={snapshot.workspacePaused} offline={offline} controllable={localPause} />
          <button className="pill" onClick={() => showScreen("people")} aria-label="Invite people to this Project"><UserPlus size={14} />Invite</button>
          {/* Theme is a preference set once and it lives in Settings. The
              toolbar is for things you act on while reading this Project, and
              settings closes the row rather than interrupting it. */}
          <button className="icon-button" onClick={() => showScreen("settings")} aria-label="Open Project settings"><Settings2 size={16} /></button>
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
            {/* Pausing only ever stops this device publishing, so the notice
                says whose sharing stopped. Nobody can pause a teammate. */}
            {snapshot.workspacePaused && <div className="notice alerting" role="status"><Pause size={15} /><div className="body"><strong>Your sharing is paused in this Project</strong>This device stopped publishing before the state was shown. Teammates keep publishing, and you keep receiving their work.</div>{!localPause && <div className="notice-actions"><span className="microcopy">Resume it from the Stickguy app or menu bar.</span></div>}</div>}
            {identity.source === "device" && !identityPromptDismissed && <div className="notice" role="status"><UserRound size={15} /><div className="body"><strong>Choose how teammates see you</strong>This Project is still showing your device name, “{identity.name}”. Pick a display name for your live work; the device name stays in Settings under Devices &amp; security.</div><div className="notice-actions"><button className="pill" onClick={() => showScreen("settings")}>Choose a name</button><button className="text-button" onClick={() => setIdentityPromptDismissed(true)}>Later</button></div></div>}
            {attention && <div className="notice alerting" role="alert"><AlertTriangle size={15} /><div className="body"><strong>Coordination update</strong>{attention.reason}</div><div className="notice-actions"><button className="pill" onClick={() => { setView("workroom"); setSelection({ kind: "collision", id: attention.id }); setAttention(null); }}>Review</button><button className="text-button" onClick={() => setAttention(null)}>Dismiss</button></div></div>}

            {view === "workroom"
              ? <WorkroomView
                  snapshot={snapshot} mine={mine} mySessions={mySessions} nearby={nearby} needsYou={needsYou} elsewhere={elsewhere}
                  convergingWorkstreams={convergingWorkstreams} selection={effectiveSelection} viewer={identity.name} tick={tick}
                  onSelectSession={openSession} onSelectFinding={(id) => setSelection({ kind: "collision", id })}
                />
              : <HistoryView snapshot={snapshot} tick={tick} selection={effectiveSelection} onSelectFinding={(id) => setSelection({ kind: "collision", id })} onSelectSession={(id) => setSelection({ kind: "session", id })} />}
          </div>
        </div>
      </main>

      <aside className="inspector" aria-label="Details inspector">
      {selectedSession
        ? <SessionInspector key={selectedSession.id} session={selectedSession} source={source} nativeApi={nativeApi} finding={selectedSessionFinding} tick={tick} isViewer={selectedSession.memberName === identity.name} localControl={localPause} back={inspectorBack} />
          : selectedCollision
            ? <CollisionInspector
                finding={selectedCollision} sessions={snapshot.workstreams} viewer={identity.name} projectId={projectId} source={source}
                card={snapshot.collaboration.syncCards.find((entry) => entry.findingId === selectedCollision.id) ?? null}
                disabled={offline}
                onState={(state) => source.setFindingState(projectId, selectedCollision.id, state)}
                onFeedback={(value) => source.recordFindingFeedback(selectedCollision.id, value)}
                onOpenSession={drillIntoSession}
                back={inspectorBack}
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

/**
 * Pausing sharing for the Project the member is actually reading.
 *
 * This control used to be a sentence pointing at the menu bar, which was honest
 * about where the switch lived and wrong about what it did: the menu-bar item
 * stops sharing for every Project on the machine, and someone reading one
 * Project is asking about that Project. The service now scopes pause to a
 * Project, so the workroom offers the request the reader actually made.
 *
 * Pause is local-service state, so a browser tab with no native bridge cannot
 * set it. That case gets the exact command instead of a button that would do
 * nothing - the same rule the activation screen follows.
 */
function useLocalControl(source: FixtureProjectSource): boolean {
  const [controllable, setControllable] = useState(false);
  useEffect(() => {
    let cancelled = false;
    void source.localControl().then((value) => { if (!cancelled) setControllable(value); }).catch(() => { if (!cancelled) setControllable(false); });
    return () => { cancelled = true; };
  }, [source]);
  return controllable;
}

function PauseControl({ source, projectId, paused, offline, controllable }: { source: FixtureProjectSource; projectId: string; paused: boolean; offline: boolean; controllable: boolean }) {
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);

  // A toolbar is for controls. Where this page cannot reach the local service
  // there is no control to offer, so it shows nothing at all rather than a
  // standing paragraph of instructions; the recovery is printed in the paused
  // notice instead, which is the only moment anyone needs it.
  if (!controllable) return null;
  return <>
    <button
      className={paused ? "pill alerting" : "pill"}
      disabled={offline || pending}
      onClick={() => {
        setPending(true);
        setFailed(false);
        void source.setProjectPaused(projectId, !paused).catch(() => setFailed(true)).finally(() => setPending(false));
      }}
    >{paused ? <Play size={14} /> : <Pause size={14} />}{paused ? "Resume" : "Pause"}</button>
    {failed && <span className="toolbar-note" role="alert">Sharing could not be changed.</span>}
  </>;
}

/**
 * Focus: the request of one agent session not to be interrupted.
 *
 * This is the inbound control and Pause is the outbound one, and they are
 * deliberately not symmetric. Pausing hides this device's work from the
 * Project, which quietly makes teammates less safe - they can no longer avoid
 * what they cannot see. Focus stops the Project reaching this agent's turns and
 * changes nothing about what is published, so the member who wants quiet keeps
 * their own risk instead of handing it to people who never asked for it.
 *
 * Nothing is consumed while a session is quiet: every pending correction is
 * still waiting when the hour is up. And it is always an hour - a mute that
 * outlives the reason for it is worse than no mute at all in a tool whose
 * whole value is being told things.
 */
const FOCUS_MINUTES = 60;

function FocusControl({ source, session }: { source: FixtureProjectSource; session: Workstream }) {
  const [focus, setFocus] = useState<SessionFocus | null>(null);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const read = () => void source.getSessionFocus(session.id).then((value) => { if (!cancelled) setFocus(value); }).catch(() => { if (!cancelled) setFocus(null); });
    read();
    // The deadline passes on its own, so the control has to notice that it has.
    const timer = window.setInterval(read, 30_000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [source, session.id]);

  if (!focus) return null;
  const apply = (minutes: number) => {
    setPending(true);
    void source.setSessionFocus(session.id, minutes).then(setFocus).catch(() => undefined).finally(() => setPending(false));
  };
  if (focus.focused) {
    return <button className="pill alerting session-focus" disabled={pending} onClick={() => apply(0)} title="Coordination is still being collected for this session; it is not being injected into its turns.">
      <BellOff size={13} />Muted{focus.until ? ` until ${sessionMessageTime(focus.until)}` : ""}
    </button>;
  }
  return <button className="pill session-focus" disabled={pending} onClick={() => apply(FOCUS_MINUTES)} title="Stop coordination reaching this agent's turns for an hour. Your work stays visible to the Project.">
    <BellOff size={13} />Mute for an hour
  </button>;
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

function WorkroomView({ snapshot, mine, mySessions, nearby, needsYou, elsewhere, convergingWorkstreams, selection, viewer, tick, onSelectSession, onSelectFinding }: {
  snapshot: ProjectSnapshot; mine: Workstream[]; mySessions: OrderedSessions; nearby: Workstream[]; needsYou: AttentionItem[]; elsewhere: Finding[];
  convergingWorkstreams: Set<string>; selection: Selection | null; viewer: string; tick: number;
  onSelectSession: (id: string) => void; onSelectFinding: (id: string) => void;
}) {
  return <>
    {/* One block answers "is anything about to hit me", and a session of your own
        that has stopped is as much an answer as a collision is. */}
    <div className="block-head lead"><h2>Needs you</h2>{needsYou.length > 0 && <span className="count hot">{needsYou.length}</span>}</div>
    <SemanticStatus status={snapshot.project.semanticStatus} mode={snapshot.project.semanticMode} />
    {needsYou.length === 0
      ? <p className="block-empty">Nothing is reaching your work right now.</p>
      : needsYou.map((item) => item.kind === "finding"
          ? <ConvergeBlock key={item.id} finding={item.finding} sessions={snapshot.workstreams} viewer={viewer} tick={tick} selected={selection?.kind === "collision" && selection.id === item.finding.id} onSelect={() => onSelectFinding(item.finding.id)} onOpenSession={onSelectSession} />
          : <HealthBlock key={item.id} session={item.session} signal={item.signal} tick={tick} selected={selection?.kind === "session" && selection.id === item.session.id} onOpen={() => onSelectSession(item.session.id)} />)}

    <div className="block-head ambient"><h2>Your sessions</h2><span className="count">{mine.length}</span></div>
    {mine.length === 0
      ? <p className="block-empty">No sessions are registered to you in this Project yet. Start Codex or Claude Code in this repository and the session appears here.</p>
      : <>
          <BranchGroups sessions={mySessions.live} render={(stream) => <SessionRow key={stream.id} session={stream} tick={tick} converging={convergingWorkstreams.has(stream.id)} selected={selection?.kind === "session" && selection.id === stream.id} onClick={() => onSelectSession(stream.id)} />} />
          {/* Finished work is worth keeping and not worth scrolling past, so it
              folds into one line rather than moving to a screen of its own. */}
          {mySessions.finished.length > 0 && <details className="fold">
            <summary>{mySessions.finished.length} finished session{mySessions.finished.length === 1 ? "" : "s"}</summary>
            <div className="rows">{mySessions.finished.map((stream) => <SessionRow key={stream.id} session={stream} tick={tick} converging={convergingWorkstreams.has(stream.id)} selected={selection?.kind === "session" && selection.id === stream.id} onClick={() => onSelectSession(stream.id)} />)}</div>
          </details>}
        </>}

    {/* A Project of one is a finished Project, so this block states a fact about
        the Project rather than naming a setup step the member has not done. */}
    <div className="block-head ambient"><h2>Nearby</h2><span className="count">{nearby.length}</span></div>
    {nearby.length === 0
      ? <p className="block-empty">You are the only member. Stickguy coordinates your own parallel sessions the same way it coordinates a team; invite someone whenever you want them in here.</p>
      : <BranchGroups sessions={nearby} render={(stream) => <PersonRow key={stream.id} session={stream} tick={tick} onClick={() => onSelectSession(stream.id)} />} />}

    {elsewhere.length > 0 && <>
      <div className="block-head ambient"><h2>Elsewhere in the Project</h2><span className="count">{elsewhere.length}</span></div>
      {elsewhere.map((finding) => <ConvergeBlock key={finding.id} finding={finding} sessions={snapshot.workstreams} viewer={viewer} tick={tick} quiet selected={selection?.kind === "collision" && selection.id === finding.id} onSelect={() => onSelectFinding(finding.id)} onOpenSession={onSelectSession} />)}
    </>}

  </>;
}

function vendorLabel(session: Workstream): string {
  return session.agent ? VENDOR_LABELS[session.agent.vendor] : "No agent connected";
}

/** The file an agent is touching right now, which is newer than its path list. */
/**
 * How a row announces itself. A workstream with no observed agent has no vendor
 * to name, and splicing the fallback label into the agent sentence produced
 * "Open No agent connected session for Ravi".
 */
function openSessionLabel(session: Workstream): string {
  return session.agent ? `Open ${vendorLabel(session)} session for ${session.memberName}` : `Open ${session.memberName}'s work`;
}

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

/**
 * Sessions grouped by the branch they are working on.
 *
 * Branch answers "who else is in my lane", and until now it was a word at the
 * end of a row rather than something the eye could group by. A Project working
 * on one branch gains nothing from a heading that repeats itself, so grouping
 * appears only once a list actually spans more than one branch; sessions with
 * no reported branch keep their place in a final unlabelled group rather than
 * being given a branch they never claimed.
 */
interface BranchGroup { branch: string | null; sessions: Workstream[] }

export function groupByBranch(sessions: readonly Workstream[]): BranchGroup[] {
  const groups: BranchGroup[] = [];
  for (const session of sessions) {
    const branch = session.agent?.branch?.trim() || null;
    const existing = groups.find((group) => group.branch === branch);
    if (existing) existing.sessions.push(session);
    else groups.push({ branch, sessions: [session] });
  }
  // One branch, or none at all, is the ordinary case and needs no chrome.
  return groups.filter((group) => group.branch !== null).length > 1 ? groups : [{ branch: null, sessions: [...sessions] }];
}

function BranchGroups({ sessions, render }: { sessions: readonly Workstream[]; render: (session: Workstream) => ReactNode }) {
  const groups = groupByBranch(sessions);
  if (groups.length === 1 && groups[0]!.branch === null) return <div className="rows">{groups[0]!.sessions.map(render)}</div>;
  return <>{groups.map((group) => <div className="branch-group" key={group.branch ?? "__unlabelled"}>
    <div className="branch-label">{group.branch
      ? <><GitBranch size={12} aria-hidden="true" /><code>{group.branch}</code><span>{group.sessions.length}</span></>
      : <><GitBranch size={12} aria-hidden="true" /><span className="unknown">no branch reported</span><span>{group.sessions.length}</span></>}</div>
    <div className="rows">{group.sessions.map(render)}</div>
  </div>)}</>;
}

function SessionRow({ session, tick, converging, selected, onClick }: { session: Workstream; tick: number; converging: boolean; selected: boolean; onClick: () => void }) {
  const path = currentPath(session);
  const activeSubagents = session.agent?.subagents.filter((agent) => agent.status !== "done") ?? [];
  const snapshot = session.scopeSnapshot;
  return <button className="session-row" aria-current={selected ? "true" : undefined} onClick={onClick} aria-label={openSessionLabel(session)}>
    <span className="session-icon">{session.agent ? <VendorMark vendor={session.agent.vendor} size={19} /> : <Code2 size={18} />}</span>
    <span>
      <h3>{snapshot.goal.text}</h3>
      <span className="session-meta">{vendorLabel(session)} · Goal {snapshot.goal.evidenceQuality} evidence · Now {snapshot.now.evidenceQuality} evidence{session.agent?.branch ? ` · ${session.agent.branch}` : ""}</span>
      <span className="session-doing"><ScopeStateIcon state={snapshot.state} /><span>{snapshot.now.text}</span></span>
      {path && <span className={isLive(session) ? "session-files live" : "session-files"}><span className="p path-swap" key={path}>{path}</span><span className="c">{session.pathCount.toLocaleString()} {session.pathCount === 1 ? "file" : "files"}</span></span>}
      {activeSubagents.length > 0 && <span className="session-sub">{activeSubagents.length} working in parallel</span>}
    </span>
    <span className="session-right">
      {converging && <span className="session-warn" title="Converging with another session"><AlertTriangle size={14} /></span>}
      <span className="session-state">{snapshot.state}</span>
      <Elapsed label={session.updatedLabel} tick={tick} />
      <span className="chev"><ChevronRight size={15} /></span>
    </span>
  </button>;
}

/** For a teammate the useful fact is intent - what they are about to do.
 *  Which agent is doing it is the next question, and reading it off a name is
 *  impossible, so the vendor is a mark rather than another word of prose. */
function PersonRow({ session, tick, onClick }: { session: Workstream; tick: number; onClick: () => void }) {
  const snapshot = session.scopeSnapshot;
  return <button className={`person-row ${session.presence}`} onClick={onClick} aria-label={openSessionLabel(session)}>
    <MemberChip name={session.memberName} />
    <span className="nm">{session.memberName}<span className="row-vendor" title={vendorLabel(session)}>{session.agent ? <VendorMark vendor={session.agent.vendor} size={13} /> : <Code2 size={12} />}</span></span>
    <span className="intent"><span>{snapshot.goal.text}</span><small>{snapshot.state} · {snapshot.goal.evidenceQuality} goal evidence</small></span>
    <Elapsed label={session.updatedLabel} tick={tick} />
  </button>;
}

const scopeFields: Array<{ key: keyof Pick<ScopeSnapshot, "goal" | "now" | "done" | "waitingOn" | "verification" | "scope">; label: string }> = [
  { key: "goal", label: "Goal" },
  { key: "now", label: "Now" },
  { key: "done", label: "Done" },
  { key: "waitingOn", label: "Waiting on" },
  { key: "verification", label: "Verification" },
  { key: "scope", label: "Scope" },
];

const scopeFactLabels: Record<ScopeSnapshotFact, string> = {
  "intent.intendedOutcome": "intended outcome",
  "intent.approachSummary": "approach summary",
  "intent.components": "components",
  "intent.contracts": "contracts",
  "intent.waitingOn": "waiting on",
  "activity.currentAction": "current action",
  "activity.writes": "observed writes",
  "activity.subagents": "subagent events",
  "contract.fingerprints": "contract fingerprints",
  "checkpoint.verification": "checkpoint verification",
  "session.derivedTitle": "derived title",
};

function scopeEvidence(field: ScopeSnapshotField): string {
  const source = field.facts.map((fact) => scopeFactLabels[fact]).join(" + ");
  return `${field.provenance} · ${field.evidenceQuality} evidence${source ? ` · ${source}` : ""}`;
}

function ScopeStateIcon({ state }: { state: ScopeSnapshot["state"] }) {
  if (state === "complete") return <Check size={12} aria-hidden="true" />;
  if (state === "waiting") return <Pause size={12} aria-hidden="true" />;
  if (state === "verifying") return <ShieldCheck size={12} aria-hidden="true" />;
  return <Activity size={12} aria-hidden="true" />;
}

function ScopeSnapshotTail({ snapshot }: { snapshot: ScopeSnapshot }) {
  const fields = [scopeFields[2]!, scopeFields[3]!, scopeFields[5]!, scopeFields[4]!];
  const icons: Record<(typeof fields)[number]["key"], ReactNode> = {
    goal: <CircleDot size={13} />,
    now: <Activity size={13} />,
    done: <Check size={13} />,
    waitingOn: <Pause size={13} />,
    verification: <ShieldCheck size={13} />,
    scope: <FileCode2 size={13} />,
  };
  return <section className="scope-tail" role="group" aria-label={`Scope snapshot revision ${snapshot.revision}`}>
    <header><span>{snapshot.state}</span><code>scope r{snapshot.revision}</code></header>
    <ol>{fields.map(({ key, label }) => {
      const field = snapshot[key];
      return <li className={`scope-tail-${key}`} key={key}><span className="thread-event-icon" aria-hidden="true">{icons[key]}</span><span><strong>{label}</strong><p>{field.text}</p><small>{scopeEvidence(field)}</small></span></li>;
    })}</ol>
  </section>;
}

/** Who you would actually go and talk to: everyone on the finding except you. */
function otherNames(affected: Workstream[], viewer: string): string {
  const names = [...new Set(affected.map((stream) => stream.memberName).filter((name) => name !== viewer))];
  return names.length > 0 ? names.join(" and ") : "your teammate";
}

/**
 * Where a collision will show up other than here.
 *
 * The branch each session is on is already carried on the workstream, and it
 * answers the question a reader asks next: is anything else going to tell me
 * about this. Two sessions on one branch will meet in Git shortly. Two sessions
 * on different branches will not meet until someone merges, which is the case
 * this product exists for and the case no other tool reports.
 *
 * A branch is never a reason to hide a finding, so an unknown branch says it is
 * unknown rather than implying either answer.
 */
function branchStatement(affected: Workstream[]): string {
  const branches = affected.map((stream) => stream.agent?.branch?.trim()).filter((branch): branch is string => Boolean(branch));
  if (affected.length < 2 || branches.length !== affected.length) {
    return "Not every session here reported a branch, so Stickguy cannot say whether Git will surface this before merge.";
  }
  const distinct = [...new Set(branches)];
  if (distinct.length === 1) {
    return `Both sessions are working on ${distinct[0]}, so Git surfaces this as well at the next pull, push, or shared write.`;
  }
  const sides = affected.map((stream) => `${stream.memberName} on ${stream.agent!.branch!.trim()}`);
  return `${joinNames(sides)}. Nothing outside this Project reports this until those branches meet at merge.`;
}

/** Which agent sessions a recorded decision is actually delivered into. */
function routingStatement(affected: Workstream[]): string {
  if (affected.length === 0) return "A decision is delivered into every affected session.";
  const targets = affected.map((stream) => `${stream.memberName}'s ${vendorLabel(stream)} session`);
  return `Goes to ${joinNames(targets)}.`;
}

function joinNames(values: string[]): string {
  if (values.length <= 1) return values[0] ?? "";
  return `${values.slice(0, -1).join(", ")} and ${values[values.length - 1]}`;
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
    {/* The card's own headline is the control that opens it, stretched across
        the whole block so the affordance is the card rather than one line of
        text inside it. The two session rows and the decision button sit above
        that overlay and keep their own targets. */}
    <h3><button className="converge-open" onClick={onSelect} aria-label={`${findingHeadline(finding)} ${names}`} aria-current={selected ? "true" : undefined}>{finding.title}</button></h3>
    <span className="converge-chev" aria-hidden="true"><ChevronRight size={16} /></span>
    <p className="why">{finding.reason}</p>
    <div className="pair">{affected.map((stream) => <button className="mini" key={stream.id} onClick={() => onOpenSession(stream.id)} aria-label={`Open ${stream.memberName}'s side of this collision`}>
      <MemberChip name={stream.memberName} />
      <span className="nm">{stream.memberName}<span className="row-vendor" title={vendorLabel(stream)}>{stream.agent ? <VendorMark vendor={stream.agent.vendor} size={13} /> : <Code2 size={12} />}</span> <em>· {stream.agent?.sessionTitle ?? stream.title}</em></span>
      <Elapsed label={stream.updatedLabel} tick={tick} />
      <span className="chev"><ChevronRight size={14} /></span>
    </button>)}</div>
    <div className="converge-actions">
      <button className="pill solid" onClick={onSelect}>Decide this with {others}</button>
      <span className="converge-meta">first seen <Since label={finding.firstSeen} tick={tick} /> ago</span>
    </div>
  </section>;
}

/**
 * A person, as a small mark rather than a word.
 *
 * Every name in the Project was rendering as the same grey circle, so telling
 * two teammates apart meant reading. The hue is derived from the name and is
 * therefore stable everywhere the person appears, which is what makes it usable
 * for scanning rather than decoration.
 */
function MemberChip({ name, size = "small" }: { name: string; size?: "small" | "large" }) {
  return <span className={size === "large" ? "avatar" : "avatar small"} style={{ "--member-hue": memberHue(name) } as CSSProperties} aria-hidden="true">{initialsFor(name)}</span>;
}

function healthHeadline(signal: HealthSignal, session: Workstream): string {
  const who = session.agent?.sessionTitle ?? session.title;
  if (signal.kind === "error") return `${who} stopped on an error`;
  if (signal.kind === "waiting") return `${who} is waiting for you`;
  return `${who} has gone quiet`;
}

/**
 * A session of your own that has stopped, rendered in the same block as a
 * collision because it is the same question: does this need me right now.
 *
 * Colour tracks how strong the evidence is, not how loud the row should be.
 * `waiting` and `error` are facts a vendor reported about its own session, so
 * they take `--alert` like any other converging work. A stall is arithmetic
 * over silence - true, useful, and not proof that anything is wrong - so it
 * stays at reading weight and lets the measurement speak.
 */
function HealthBlock({ session, signal, tick, selected, onOpen }: {
  session: Workstream; signal: HealthSignal; tick: number; selected: boolean; onOpen: () => void;
}) {
  const icon = signal.kind === "error" ? <AlertTriangle size={18} /> : signal.kind === "waiting" ? <Pause size={18} /> : <CircleDot size={18} />;
  return <section className={signal.kind === "stalled" ? "converge health quiet" : "converge health"} aria-label={healthHeadline(signal, session)}>
    <span className="converge-icon">{icon}</span>
    <h3><button className="converge-open" onClick={onOpen} aria-current={selected ? "true" : undefined}>{healthHeadline(signal, session)}</button></h3>
    <span className="converge-chev" aria-hidden="true"><ChevronRight size={16} /></span>
    <p className="why">{signal.statement}</p>
    <div className="pair"><button className="mini" onClick={onOpen} aria-label={openSessionLabel(session)}>
      <MemberChip name={session.memberName} />
      <span className="nm">{session.memberName} <em>· {vendorLabel(session)}</em></span>
      <Elapsed label={session.updatedLabel} tick={tick} />
      <span className="chev"><ChevronRight size={14} /></span>
    </button></div>
    <p className="converge-meta solo">
      {signal.silentSeconds !== undefined && <><span className="strong">silent for {formatElapsed(signal.silentSeconds)}</span> · </>}
      {signal.kind === "stalled" ? "observed silence, not a diagnosis" : "reported by the agent"}
    </p>
  </section>;
}

/**
 * What already happened, in one place.
 *
 * This was two tabs. "Ledger" named a filing cabinet rather than anything a
 * person goes looking for, and "Decisions" survived ADR-037 deleting the
 * standalone decision surface it was built for, so half the sidebar answered a
 * question nobody had asked in words they did not use. Both answered the same
 * question - what has this Project already handled - so they are one screen
 * with a name people already own.
 *
 * It stops at delivery. Stickguy knows a correction was routed and whether the
 * agent acknowledged reading it, and does not know whether the agent then did
 * the right thing; wording that implied otherwise would fail the fidelity rules
 * this screen is subordinate to.
 */
function HistoryView({ snapshot, tick, selection, onSelectFinding, onSelectSession }: {
  snapshot: ProjectSnapshot; tick: number; selection: Selection | null;
  onSelectFinding: (id: string) => void; onSelectSession: (id: string) => void;
}) {
  // `firstSeen` is still a prose label from the service, so comparing the
  // strings sorted "44m ago" above "12 min ago". Parse to seconds and put the
  // most recent first; anything unparseable keeps its place at the end.
  const findings = [...snapshot.findings].sort((left, right) => (elapsedFromLabel(left.firstSeen) ?? Infinity) - (elapsedFromLabel(right.firstSeen) ?? Infinity));
  const deliveries = snapshot.workstreams
    .flatMap((session) => (session.agent?.coordination ?? []).map((entry) => ({ session, entry })))
    .sort((left, right) => (left.entry.routedAt < right.entry.routedAt ? 1 : -1));
  const acknowledged = deliveries.filter((item) => item.entry.acknowledgedAt).length;
  const resolved = snapshot.collaboration.syncCards.filter((card) => card.state === "resolved" && card.resolution);
  const loose = snapshot.collaboration.resolutions.filter((resolution) => !resolved.some((card) => card.resolution?.id === resolution.id));
  const membersOn = (ids: string[]) => [...new Set(snapshot.workstreams.filter((stream) => ids.includes(stream.id)).map((stream) => stream.memberName))];

  return <>
    <div className="block-head lead"><h2>Raised</h2><span className="count">{findings.length}</span></div>
    <p className="block-note">Everything this Project has caught, whether or not it reached you.</p>
    {findings.length === 0
      ? <p className="block-empty">Nothing has been raised in this Project yet.</p>
      : <ol className="ledger">{findings.map((finding) => <li key={finding.id}>
          <button aria-current={selection?.kind === "collision" && selection.id === finding.id ? "true" : undefined} onClick={() => onSelectFinding(finding.id)} aria-label={`Open ${findingHeadline(finding)}`}>
            <span className="lt">{finding.title}</span>
            <span className="lw">{finding.reason}</span>
            <span className="lp">{membersOn(finding.workstreamIds).map((name) => <MemberChip key={name} name={name} />)}</span>
            <span className="lf">
              <span>{findingHeadline(finding).toLowerCase()}</span>
              <span>{finding.severity} severity</span>
              <span>{finding.confidence} confidence</span>
              <span>{finding.state}</span>
              <span>raised <Since label={finding.firstSeen} tick={tick} /> ago</span>
            </span>
          </button>
        </li>)}</ol>}

    <div className="block-head ambient"><h2>Delivered into a turn</h2><span className="count">{deliveries.length}</span></div>
    {deliveries.length === 0
      ? <p className="block-empty">No coordination has been routed into an agent turn yet.</p>
      : <>
          <ol className="ledger">{deliveries.map(({ session, entry }) => <li key={entry.id}>
            <button aria-current={selection?.kind === "session" && selection.id === session.id ? "true" : undefined} onClick={() => onSelectSession(session.id)} aria-label={openSessionLabel(session)}>
              <span className="lt">{entry.summary}</span>
              <span className="lp"><MemberChip name={session.memberName} /><span className="row-vendor" title={vendorLabel(session)}>{session.agent ? <VendorMark vendor={session.agent.vendor} size={13} /> : <Code2 size={12} />}</span></span>
              <span className="lf">
                <span>{session.memberName}</span>
                <span>{entry.itemCount} item{entry.itemCount === 1 ? "" : "s"}</span>
                <span>at {coordinationTriggerLabel(entry.trigger).toLowerCase()}</span>
                <span>{entry.acknowledgedAt ? "acknowledged" : "not yet acknowledged"}</span>
              </span>
            </button>
          </li>)}</ol>
          <p className="block-note">{acknowledged} of {deliveries.length} delivered brief{deliveries.length === 1 ? "" : "s"} {acknowledged === 1 ? "was" : "were"} acknowledged by the receiving agent. Acknowledgement records that the agent read the correction, not that it followed it.</p>
        </>}

    <div className="block-head ambient"><h2>Settled</h2><span className="count">{resolved.length + loose.length}</span></div>
    {resolved.length + loose.length === 0
      ? <p className="block-empty">Nothing has been settled yet. Resolving a collision records the outcome here and delivers it to every affected session.</p>
      : <div>
          {resolved.map((card) => <article className="decision-entry" key={card.id}><h3>{card.title}</h3><p>{card.resolution?.summary}</p><div className="sent">Delivered to {card.resolution?.affectedWorkstreamIds.length ?? 0} session{(card.resolution?.affectedWorkstreamIds.length ?? 0) === 1 ? "" : "s"} · revision {card.revision}</div></article>)}
          {loose.map((resolution) => <article className="decision-entry" key={resolution.id}><h3>Coordination decision</h3><p>{resolution.summary}</p><div className="sent">Delivered to {resolution.affectedWorkstreamIds.length} session{resolution.affectedWorkstreamIds.length === 1 ? "" : "s"} · revision {resolution.revision}</div></article>)}
        </div>}

    {/* The raw event stream is the least-read thing on the screen and the
        hardest to scan, so it is available rather than in the way. */}
    {snapshot.activity.length > 0 && <details className="fold">
      <summary>{snapshot.activity.length} recorded event{snapshot.activity.length === 1 ? "" : "s"}</summary>
      <ol className="timeline">{snapshot.activity.map((item) => <li key={item.id}><p><strong>{item.actor}</strong> {item.summary}</p><span className="src"><Since label={item.at} tick={tick} /> · {activitySourceLabel(item.fidelity)}</span></li>)}</ol>
    </details>}
  </>;
}

/** The way back to whatever sent you here, when something did. */
interface InspectorBack { label: string; onBack: () => void }

function InspectorBackLink({ back }: { back: InspectorBack | null }) {
  if (!back) return null;
  return <button className="inspector-back" onClick={back.onBack}><ChevronLeft size={13} aria-hidden="true" />{back.label}</button>;
}

function SessionInspector({ session, source, nativeApi, finding, tick, isViewer, localControl, back }: { session: Workstream; source: FixtureProjectSource; nativeApi: NativeOnboarding; finding: Finding | null; tick: number; isViewer: boolean; localControl: boolean; back: InspectorBack | null }) {
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
  const { scrollRef, following, onScroll, jumpToNow } = useFollowTail(session.id, feed.length);
  const snapshot = session.scopeSnapshot;
  const title = snapshot.goal.text;
  const branch = session.agent?.branch ?? own?.branch;
  const liveFacts = [session.agent?.tool, showPath ? path : undefined].filter((value): value is string => Boolean(value)).join(" · ");

  return <>
    <InspectorBackLink back={back} />
    <div className="inspector-bar session-inspector-bar">
      <span className="inspector-vendor" aria-hidden="true">{session.agent ? <VendorMark vendor={session.agent.vendor} size={19} /> : <Code2 size={18} />}</span>
      <span className="grow">
        <h2>{title}</h2>
        <div className="sub">{session.memberName} · {vendorLabel(session)}</div>
        <div className="scope-goal-evidence"><span>Goal</span> · {scopeEvidence(snapshot.goal)} · <code>scope r{snapshot.revision}</code></div>
        {branch && <div className="inspector-status"><GitBranch size={12} aria-hidden="true" /><code>{branch}</code></div>}
        {/* What the session *is* right now belongs to the session, so it reads
            in the header beside its name. What the session *did* belongs to the
            thread below. Carrying the newest event in both places was what made
            a strictly chronological feed read as though it were out of order. */}
        {!complete && <div className="inspector-live" aria-label="Current session activity" aria-live="polite">
          <ScopeStateIcon state={snapshot.state} />
          <small>{statusCopy(session)}</small>
          <em>Now</em>
          <span>{snapshot.now.text}</span>
          <code>{snapshot.now.evidenceQuality} evidence{liveFacts ? ` · ${liveFacts}` : ""}</code>
          <Elapsed label={session.updatedLabel} tick={tick} />
        </div>}
      </span>
      <div className="session-header-actions" ref={detailsAnchorRef}>
        {/* Only your own session, and only while it is still running: muting a
            session that has finished would be a control with nothing to mute,
            and a teammate's session is not yours to quiet. */}
        {isViewer && session.agent && !complete && localControl && <FocusControl source={source} session={session} />}
        {complete && <span className="session-complete"><Check size={12} aria-hidden="true" />Complete</span>}
        <button className="icon-button session-details-button" aria-label="Open session details" aria-expanded={detailsOpen} onClick={() => setDetailsOpen((open) => !open)}><Info size={15} /></button>
        {detailsOpen && <SessionDetailsPanel session={session} mine={mine} subagents={subagents} path={path} onClose={() => setDetailsOpen(false)} />}
      </div>
    </div>
    {isViewer && localControl && (session.agent?.vendor === "codex" || session.agent?.vendor === "claude") && nativeApi.openOwningSession && <OwningSessionActions session={session} finding={finding} nativeApi={nativeApi} />}
    <div className="inspector-body chat-inspector-body" ref={scrollRef} onScroll={onScroll}>
      {messageError && <p className="form-error" role="alert">{messageError}</p>}
      {feed.length > 0
        ? <ol className="session-thread">{feed.map((item) => <SessionFeedRow key={item.id} item={item} session={session} isViewer={isViewer} tick={tick} />)}</ol>
          : <div className="conversation-empty"><MessageSquare size={18} /><div><strong>{session.agent ? "This session has not said anything yet." : "No agent conversation is available."}</strong><p>{session.outcome}</p></div></div>}
      <ScopeSnapshotTail snapshot={snapshot} />
    </div>
    {/* Only once the reader has left the tail does a control to return to it
        mean anything. At the tail it would be a button that does nothing. */}
    {!following && <button className="jump-to-now" onClick={jumpToNow}>
      <Activity size={13} aria-hidden="true" />
      <span><LiveAction session={session} /></span>
      <ChevronRight size={14} aria-hidden="true" />
    </button>}
  </>;
}

function OwningSessionActions({ session, finding, nativeApi }: { session: Workstream; finding: Finding | null; nativeApi: NativeOnboarding }) {
  const [pending, setPending] = useState(false);
  const [confirmCodex, setConfirmCodex] = useState(false);
  const [result, setResult] = useState<NativeSessionOpenResult | null>(null);
  const [copied, setCopied] = useState(false);
  const vendor = session.agent?.vendor;
  if (vendor !== "codex" && vendor !== "claude") return null;
  const prompt = finding
    ? `Stickguy found: ${finding.title}\n\n${finding.reason}\n\nReview this coordination finding before continuing.`
    : `Review the current Stickguy coordination context for “${session.agent?.sessionTitle ?? session.title}” before continuing.`;

  const open = (target: "vendor" | "vscode" = "vendor", confirmed = false) => {
    if (!nativeApi.openOwningSession || !vendor) return;
    // Continuing an already active Codex task in another process can interleave
    // history. Make that choice explicit instead of treating "open" as harmless.
    if (vendor === "codex" && session.agent?.status === "active" && !confirmed) {
      setConfirmCodex(true);
      setResult(null);
      return;
    }
    setConfirmCodex(false);
    setPending(true);
    setCopied(false);
    void nativeApi.openOwningSession(session.id, prompt, target)
      .then(setResult)
      .catch((error: unknown) => setResult({ vendor, opened: false, detail: error instanceof Error ? error.message : "The owning session could not be opened." }))
      .finally(() => setPending(false));
  };

  const copyFallback = () => {
    if (!result?.fallbackCommand) return;
    void navigator.clipboard.writeText(result.fallbackCommand).then(() => setCopied(true));
  };

  return <section className="session-open" aria-label="Open the owning session">
    <div className="session-open-actions">
      <button className="pill" disabled={pending} onClick={() => open("vendor")}><ExternalLink size={13} aria-hidden="true" />{vendor === "codex" ? "Continue in Codex" : "Open in Claude Code"}</button>
      {vendor === "claude" && <button className="text-button" disabled={pending} onClick={() => open("vscode")}>Open in VS Code</button>}
    </div>
    {vendor === "claude" && !result && !confirmCodex && <p className="session-open-note">Uses Claude Code's local open handler, which may be unavailable before its first interactive prompt or when an organization disables it. A copyable command is provided if opening fails.</p>}
    {confirmCodex && <div className="session-open-state" role="alert"><p>This Codex session is still reported active. Continuing it elsewhere can interleave its history.</p><div><button className="pill" onClick={() => open("vendor", true)}>Continue exact session</button><button className="text-button" onClick={() => setConfirmCodex(false)}>Cancel</button></div></div>}
    {result && <div className={`session-open-state${result.opened ? " opened" : ""}`} role="status"><p>{result.detail}</p>{result.fallbackCommand && <button className="text-button" onClick={copyFallback}><Copy size={12} aria-hidden="true" />{copied ? "Command copied" : "Copy command"}</button>}</div>}
  </section>;
}

type TranscriptMessage = { id: string; kind: SessionMessageKind | "tool"; text?: string; tool?: string; at?: string };
type SessionFeedItem =
  | { id: string; kind: SessionMessageKind; text?: string; at?: string }
  | { id: string; kind: "tool_group"; tools: string[]; at?: string }
  | { id: string; kind: "context_group"; blocks: string[]; at?: string }
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
    if (item.kind === "tool") {
      const previous = grouped[grouped.length - 1];
      if (previous?.kind === "tool_group") previous.tools.push(item.tool);
      else grouped.push({ id: `tools-${item.id}`, kind: "tool_group", tools: [item.tool], at: item.at });
      continue;
    }
    // What the harness told the agent - sandbox rules, permissions, repository
    // instructions - is provenance rather than conversation, and Codex sends
    // several blocks of it before a session says anything. Rendered at message
    // weight they buried the first real exchange, so consecutive blocks fold
    // into one line that opens on demand.
    if (item.kind === "system") {
      const previous = grouped[grouped.length - 1];
      if (previous?.kind === "context_group") previous.blocks.push(item.text ?? "");
      else grouped.push({ id: `context-${item.id}`, kind: "context_group", blocks: [item.text ?? ""], at: item.at });
      continue;
    }
    grouped.push(item);
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
  if (item.kind === "context_group") return <li className="thread-context"><details>
    <summary><span className="thread-tool-icon" aria-hidden="true"><ShieldCheck size={12} /></span><span>{item.blocks.length === 1 ? "Harness instructions" : `${item.blocks.length} blocks of harness instructions`}</span>{item.at && <small>{sessionMessageTime(item.at)}</small>}</summary>
    {item.blocks.map((block, index) => <MarkdownMessage key={index} text={block} />)}
  </details></li>;
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
        {session.agent && <CoverageRow icon={<FileText size={14} />} label="Coordination" value={`${briefDeliveryLabel(session.agent.capabilities.deliverBrief)} · ${session.agent.capabilities.requestAttention === "advisory" ? "advisory" : "dashboard"}`} detail="Context is routed at supported turn boundaries; Stickguy never interrupts an agent mid-turn. Muting a session stops delivery into its turns and holds every pending item until the hour is up; it never stops this session being published to the Project." />}
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

function SessionMessageIcon({ kind, vendor }: { kind: SessionMessageKind; vendor?: AgentVendor }) {
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
function CollisionInspector({ finding, sessions, viewer, projectId, source, card, disabled, onState, onFeedback, onOpenSession, back }: {
  finding: Finding; sessions: Workstream[]; viewer: string; projectId: string; source: FixtureProjectSource; card: SyncCard | null; disabled: boolean;
  onState: (state: FindingState) => void; onFeedback: (value: FindingFeedback) => Promise<void>; onOpenSession: (id: string) => void; back: InspectorBack | null;
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
  const submitDecision = (open: SyncCard) => {
    const summary = decision.trim();
    if (!summary) return;
    run(async () => { await source.resolveSyncCard(projectId, open.id, open.revision, summary, finding.workstreamIds); setDecision(""); onState("resolved"); });
  };

  const kindLabel = findingHeadline(finding);
  // The hosted snapshot writes a finding's title as its kind with the
  // underscores taken out, so on live data the title and the kind are the same
  // sentence twice. Print the kind as an eyebrow only when the title is
  // genuinely something else; the reason carries the content either way.
  const titleRepeatsKind = plainWords(finding.title) === plainWords(kindLabel);

  return <article className="collision-detail" aria-label="Selected collision detail">
    <InspectorBackLink back={back} />
    <div className="inspector-bar">
      <span className="grow">
        {!titleRepeatsKind && <p className="finding-kind"><AlertTriangle size={13} aria-hidden="true" />{kindLabel}</p>}
        <h2>{titleRepeatsKind ? kindLabel : finding.title}</h2>
        <div className="severity-row"><span className="sev">{finding.severity} severity</span><span>{finding.confidence} confidence</span><span>{finding.state}</span></div>
      </span>
    </div>
    <div className="inspector-body">
      {/* What is true, then where else it will show up. Both are prose because
          both are statements a person could have written; everything machine
          derived is in the evidence block below. */}
      <p className="collision-reason">{finding.reason}</p>
      <p className="branch-context">{branchStatement(affected)}</p>

      <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true"><Users size={13} /></span><span>Sessions</span><span className="inspector-head-count">{affected.length}</span></h3>
      <div className="pair">{affected.map((session) => <button className="mini" key={session.id} onClick={() => onOpenSession(session.id)} aria-label={`Open ${session.memberName}'s session detail`}>
        <span className="mini-icon">{session.agent ? <VendorMark vendor={session.agent.vendor} size={17} /> : <Code2 size={16} />}</span>
        <span className="nm">{session.memberName} <em>· {vendorLabel(session)}</em></span>
        <span className="clock">{session.updatedLabel}</span>
        <span className="chev"><ChevronRight size={14} /></span>
        <span className="doing">{session.outcome}</span>
      </button>)}</div>

      <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true"><Search size={13} /></span><span>Evidence</span></h3>
      <ul className="evidence-list">{finding.evidence.map((item) => <li key={`${item.kind}-${item.label}`}>
        <span className="e-label">{item.label}</span>
        <span className="e-src">{evidenceKindLabel(item.kind)} · {item.source.replaceAll("_", " ")}</span>
      </li>)}</ul>
      <p className="evidence-age">first seen {finding.firstSeen} · last changed {finding.lastSeen}</p>

      <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true"><Check size={13} /></span><span>Decision</span></h3>
      {/* The decision is the one thing on this screen that reaches an agent, so
          it is the composer rather than a field in a row of fields, it names
          its own delivery before it is written, and the discussion below it is
          plainly labelled as going nowhere. */}
      {card?.resolution
        ? <div className="decision-note"><div className="lbl"><Check size={13} />Decision from {names}</div><p>{card.resolution.summary}</p><div className="sent">Delivered to {card.resolution.affectedWorkstreamIds.length} session{card.resolution.affectedWorkstreamIds.length === 1 ? "" : "s"} · revision {card.revision}</div></div>
        : card
          ? <form className="composer" onSubmit={(event) => { event.preventDefault(); submitDecision(card); }}>
              <textarea
                value={decision} rows={2} placeholder="What did you decide?" aria-label={`Decision for ${card.title}`}
                onChange={(event) => setDecision(event.target.value)}
                onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === "Enter") { event.preventDefault(); submitDecision(card); } }}
              />
              <div className="composer-foot">
                <span className="routing-note"><Route size={13} aria-hidden="true" />{routingStatement(affected)}</span>
                <button className="pill solid" disabled={threadPending || disabled || decision.trim().length === 0}>Record decision</button>
              </div>
            </form>
          : <div className="decision-invite">
              <p>{routingStatement(affected)}</p>
              <button className="pill solid" disabled={disabled || threadPending} onClick={() => run(() => source.createSyncCard(projectId, finding.id, finding.title, finding.reason))}>Decide this with {others}</button>
            </div>}
      <p className="advisory-note">Advisory only. Stickguy delivers the decision and never blocks or controls an agent.</p>

      {card && <>
        <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true"><MessageSquare size={13} /></span><span>Discussion</span>{card.comments.length > 0 && <span className="inspector-head-count">{card.comments.length}</span>}</h3>
        {card.comments.length > 0 && <ol className="comment-thread">{card.comments.map((entry) => <li key={entry.id}>
          <MemberChip name={entry.memberName} />
          <span className="c-who">{entry.memberName}<time>{sessionMessageTime(entry.createdAt)}</time></span>
          <p className="c-body">{entry.body}</p>
        </li>)}</ol>}
        {card.state === "open" && <form className="composer inline" onSubmit={(event) => { event.preventDefault(); const body = comment.trim(); if (!body) return; run(async () => { await source.commentSyncCard(projectId, card.id, body); setComment(""); }); }}>
          <input value={comment} onChange={(event) => setComment(event.target.value)} placeholder="Add a comment…" aria-label={`Comment on ${card.title}`} />
          <button className="icon-button send" disabled={threadPending || disabled || comment.trim().length === 0} aria-label="Add comment"><ArrowUp size={15} /></button>
        </form>}
        <p className="thread-note">Comments stay here for the people reading this Project. Only the decision above reaches an agent.</p>
      </>}
      {threadError && <p className="form-error" role="alert">{threadError}</p>}

      <div className="finding-foot">
        <div className="finding-feedback" aria-label="Collision feedback">
          <span role="status">{feedbackMessage}</span>
          <button disabled={disabled || feedbackPending} className="text-button" onClick={() => submitFeedback("useful")}>Useful</button>
          <button disabled={disabled || feedbackPending} className="text-button" onClick={() => submitFeedback("not_related")}>Not related</button>
        </div>
        {/* Resolving is recording a decision, which is the composer above: it is
            the only control here that reaches an agent, so there is no second
            button claiming the same word. What is left are the two ways to stop
            reading a finding without deciding anything. */}
        <div className="finding-actions">
          <button disabled={disabled || finding.state === "acknowledged"} className="pill" onClick={() => onState("acknowledged")}>Acknowledge</button>
          <button disabled={disabled || finding.state === "dismissed"} className="pill" onClick={() => onState("dismissed")}>Dismiss</button>
        </div>
      </div>
    </div>
  </article>;
}

/** Comparable words only, so "stale assumption" and "Stale assumption" match. */
function plainWords(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, " ").trim();
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
