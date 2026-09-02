import { StrictMode, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { CSSProperties, FormEvent, ReactNode } from "react";
import { createRoot } from "react-dom/client";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  Activity,
  AlertTriangle,
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
import type { AgentVendor, DashboardSession, Finding, FindingFeedback, FindingState, LocalSessionDetail, MemberNameSource, ProjectAccess, ProjectSnapshot, Resolution, ScopeSnapshot, ScopeSnapshotFact, ScopeSnapshotField, SessionFocus, SessionMessageKind, SessionMessagesSnapshot, ShellState, SyncCard, Workstream } from "./model";

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

type Selection = { kind: "session"; id: string } | { kind: "finding"; id: string };
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
  // Fixture snapshots are authored at fixed times, so wall-clock arithmetic over
  // them would report every session as silent for months. Live snapshots carry
  // real event times and are measured against the real clock.
  const now = source.live ? Date.now() : newestEventTime(snapshot) ?? Date.now();
  const needsYou = useMemo(() => attentionItems(snapshot, mineIds, now), [snapshot, mineIds, now]);
  // Everything open that the lead block did not admit: findings on other
  // people's work, and findings on yours that the judgment layer routed to
  // the dashboard rather than into a turn. Visible, never competing.
  const needsYouFindingIds = useMemo(() => new Set(needsYou.flatMap((item) => (item.kind === "finding" ? [item.id] : []))), [needsYou]);
  const elsewhere = openFindings.filter((finding) => !needsYouFindingIds.has(finding.id));
  const converging = useMemo(() => needsYou.flatMap((item) => (item.kind === "finding" ? [item.finding] : [])), [needsYou]);
  const convergingWorkstreams = useMemo(() => new Set(converging.flatMap((finding) => finding.workstreamIds)), [converging]);
  // Ranked by what you would do about it, then by the same elapsed label the
  // row shows, so the order always agrees with the clock beside it.
  const mySessions = useMemo(() => orderSessions(mine, now, (session) => elapsedFromLabel(session.updatedLabel) ?? Number.POSITIVE_INFINITY), [mine, now]);

  const defaultSession = mine.find((stream) => stream.agent?.status === "active") ?? mine[0] ?? snapshot.workstreams[0];
  // History gets no default: falling back to "your most active session" is
  // right beside a live list and pure non-sequitur beside a record.
  const effectiveSelection: Selection | null = selection ?? (view === "workroom" && defaultSession ? { kind: "session", id: defaultSession.id } : null);
  const selectedSession = effectiveSelection?.kind === "session" ? snapshot.workstreams.find((stream) => stream.id === effectiveSelection.id) ?? null : null;
  const selectedCollision = effectiveSelection?.kind === "finding" ? snapshot.findings.find((finding) => finding.id === effectiveSelection.id) ?? null : null;
  const previousCollision = trail.length > 1 && trail[trail.length - 2]?.kind === "finding"
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
        label: previousSelection.kind === "finding"
          ? snapshot.findings.find((finding) => finding.id === previousSelection.id)?.title ?? "finding"
          : snapshot.workstreams.find((stream) => stream.id === previousSelection.id)?.agent?.sessionTitle ?? "session",
        onBack: popSelection,
      }
    : null;
  // Whatever was selected belonged to the view being left; carrying it across
  // rendered a stale panel beside an unrelated list.
  const showView = (next: View) => { setView(next); setScreens([]); setSelection(null); };

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

            <NoticeQueue notices={[
              // Most actionable first: a new finding outranks this device's own
              // states, and setup advice comes last. One renders; the rest wait
              // behind a count rather than stacking above the content.
              ...(attention ? [<div key="attention" className="notice alerting" role="alert"><AlertTriangle size={15} /><div className="body"><strong>Coordination update</strong>{attention.reason}</div><div className="notice-actions"><button className="pill" onClick={() => { setView("workroom"); setSelection({ kind: "finding", id: attention.id }); setAttention(null); }}>Review</button><button className="text-button" onClick={() => setAttention(null)}>Dismiss</button></div></div>] : []),
              // Pausing only ever stops this device publishing, so the notice
              // says whose sharing stopped. Nobody can pause a teammate.
              ...(snapshot.workspacePaused ? [<div key="paused" className="notice alerting" role="status"><Pause size={15} /><div className="body"><strong>Your sharing is paused in this Project</strong>This device stopped publishing before the state was shown. Teammates keep publishing, and you keep receiving their work.</div>{!localPause && <div className="notice-actions"><span className="microcopy">Resume it from the Stickguy app or menu bar.</span></div>}</div>] : []),
              ...(offline ? [<div key="offline" className="notice" role="status"><CircleDot size={15} /><div className="body"><strong>Offline</strong>Showing revision {snapshot.contextRevision} from {snapshot.synchronizedAt}.</div></div>] : []),
              ...(identity.source === "device" && !identityPromptDismissed ? [<div key="identity" className="notice" role="status"><UserRound size={15} /><div className="body"><strong>Choose how teammates see you</strong>This Project is still showing your device name, “{identity.name}”. Pick a display name for your live work; the device name stays in Settings under Devices &amp; security.</div><div className="notice-actions"><button className="pill" onClick={() => showScreen("settings")}>Choose a name</button><button className="text-button" onClick={() => setIdentityPromptDismissed(true)}>Later</button></div></div>] : []),
            ]} />

            {view === "workroom"
              ? <WorkroomView
                  snapshot={snapshot} mine={mine} mySessions={mySessions} nearby={nearby} needsYou={needsYou} elsewhere={elsewhere}
                  convergingWorkstreams={convergingWorkstreams} selection={effectiveSelection} tick={tick}
                  onSelectSession={openSession} onSelectFinding={(id) => setSelection({ kind: "finding", id })}
                />
              : <HistoryView snapshot={snapshot} tick={tick} now={now} selection={effectiveSelection} onSelectFinding={(id) => setSelection({ kind: "finding", id })} />}
          </div>
        </div>
      </main>

      <aside className="inspector" aria-label="Details inspector">
      {selectedSession
        ? <SessionInspector key={selectedSession.id} session={selectedSession} source={source} nativeApi={nativeApi} finding={selectedSessionFinding} tick={tick} isViewer={selectedSession.memberName === identity.name} localControl={localPause} back={inspectorBack} />
          : selectedCollision
            ? <FindingInspector
                finding={selectedCollision} sessions={snapshot.workstreams} projectId={projectId} source={source}
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

/**
 * At most one notice above the content.
 *
 * Offline, paused, a name prompt and a coordination update could all be true at
 * once, and four stacked rows pushed the block the screen exists for below the
 * fold. The most actionable notice renders; the rest are a count the reader can
 * open, so nothing is hidden and nothing is in the way.
 */
function NoticeQueue({ notices }: { notices: ReactNode[] }) {
  const [expanded, setExpanded] = useState(false);
  if (notices.length === 0) return null;
  if (notices.length === 1 || expanded) return <>{notices}</>;
  return <>
    {notices[0]}
    <button className="text-button notice-more" onClick={() => setExpanded(true)}>{notices.length - 1} more {notices.length === 2 ? "notice" : "notices"}</button>
  </>;
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

function WorkroomView({ snapshot, mine, mySessions, nearby, needsYou, elsewhere, convergingWorkstreams, selection, tick, onSelectSession, onSelectFinding }: {
  snapshot: ProjectSnapshot; mine: Workstream[]; mySessions: OrderedSessions; nearby: Workstream[]; needsYou: AttentionItem[]; elsewhere: Finding[];
  convergingWorkstreams: Set<string>; selection: Selection | null; tick: number;
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
          ? <ConvergeBlock key={item.id} finding={item.finding} sessions={snapshot.workstreams} tick={tick} selected={selection?.kind === "finding" && selection.id === item.finding.id} onSelect={() => onSelectFinding(item.finding.id)} onOpenSession={onSelectSession} />
          : <HealthBlock key={item.id} session={item.session} signal={item.signal} tick={tick} selected={selection?.kind === "session" && selection.id === item.session.id} onOpen={() => onSelectSession(item.session.id)} />)}

    {/* One block, grouped by the part of the product being touched, across
        everyone. Splitting your sessions from teammates' made the grouping
        unable to do its one job: "who else is in my lane" was answered only
        by reading two lists and matching their headings. The self/other
        distinction survives as row richness - your rows carry live activity,
        a teammate's row carries intent - which is how the two row types
        already differed. */}
    <div className="block-head ambient"><h2>Sessions</h2><span className="count">{mine.length + nearby.length}</span></div>
    {mine.length + nearby.length === 0
      ? <p className="block-empty">No sessions are registered in this Project yet. Start Codex or Claude Code in this repository and the session appears here.</p>
      : <>
          <AreaGroups sessions={[...mySessions.live, ...nearby]} findings={snapshot.findings} render={(stream) => mine.some((candidate) => candidate.id === stream.id)
            ? <SessionRow key={stream.id} session={stream} tick={tick} converging={convergingWorkstreams.has(stream.id)} selected={selection?.kind === "session" && selection.id === stream.id} onClick={() => onSelectSession(stream.id)} />
            : <PersonRow key={stream.id} session={stream} tick={tick} onClick={() => onSelectSession(stream.id)} />} />
          {/* Finished work is worth keeping and not worth scrolling past, so it
              folds into one line rather than moving to a screen of its own. */}
          {mySessions.finished.length > 0 && <details className="fold">
            <summary>{mySessions.finished.length} finished session{mySessions.finished.length === 1 ? "" : "s"}</summary>
            <div className="rows">{mySessions.finished.map((stream) => <SessionRow key={stream.id} session={stream} tick={tick} converging={convergingWorkstreams.has(stream.id)} selected={selection?.kind === "session" && selection.id === stream.id} onClick={() => onSelectSession(stream.id)} />)}</div>
          </details>}
          {/* A Project of one is a finished Project (ADR-054): a fact about
              the Project, not a setup step the member has failed to do. */}
          {nearby.length === 0 && <p className="block-note">You are the only member. Stickguy coordinates your own parallel sessions the same way it coordinates a team; invite someone whenever you want them in here.</p>}
        </>}

    {elsewhere.length > 0 && <>
      <div className="block-head ambient"><h2>Elsewhere in the Project</h2><span className="count">{elsewhere.length}</span></div>
      {elsewhere.map((finding) => <ConvergeBlock key={finding.id} finding={finding} sessions={snapshot.workstreams} tick={tick} quiet selected={selection?.kind === "finding" && selection.id === finding.id} onSelect={() => onSelectFinding(finding.id)} onOpenSession={onSelectSession} />)}
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

export interface AreaGroup { area: string | null; sessions: Workstream[] }

/**
 * The area of the product a session is working in.
 *
 * A declared contract wins, because a contract is the thing two sessions
 * actually collide over: two rows under one contract heading is the collision,
 * shown as structure instead of as a separate card. A declared component is the
 * next best. Failing both — which is every session that never called
 * begin_work — the shared directory of its paths is what the repository itself
 * says about where the work is.
 */
export function sessionArea(session: Workstream): string | null {
  const declared = session.contracts?.[0]?.trim() || session.components?.[0]?.trim();
  if (declared) return declared;
  const directories = session.paths
    .map((path) => path.split("/").slice(0, -1))
    .filter((segments) => segments.length > 0);
  if (directories.length === 0) return null;
  // The deepest directory every path agrees on. One file yields its own folder;
  // work spread across a tree yields the root they share, and nothing shared
  // yields nothing rather than a made-up parent.
  const shared = directories.reduce((common, segments) => {
    const limit = Math.min(common.length, segments.length);
    let index = 0;
    while (index < limit && common[index] === segments[index]) index += 1;
    return common.slice(0, index);
  });
  return shared.length > 0 ? shared.join("/") : null;
}

/**
 * Sessions grouped by the part of the product they are touching, so the reader
 * sees the shape of the work before the list of sessions.
 *
 * Areas holding more than one session come first: that is where work converges,
 * and it is the reason to group at all. Everything Stickguy could not place
 * sits last under its own heading rather than being hidden or guessed at.
 */
export function groupByArea(sessions: readonly Workstream[]): AreaGroup[] {
  const groups: AreaGroup[] = [];
  for (const session of sessions) {
    const area = sessionArea(session);
    const existing = groups.find((group) => group.area === area);
    if (existing) existing.sessions.push(session);
    else groups.push({ area, sessions: [session] });
  }
  // Grouping that produces one heading is chrome around a list that was already
  // legible, so it is not applied at all.
  const labelled = groups.filter((group) => group.area !== null);
  if (labelled.length < 2 && !labelled.some((group) => group.sessions.length > 1)) {
    return [{ area: null, sessions: [...sessions] }];
  }
  return [...groups].sort((left, right) => {
    if (left.area === null) return 1;
    if (right.area === null) return -1;
    if (left.sessions.length !== right.sessions.length) return right.sessions.length - left.sessions.length;
    return left.area.localeCompare(right.area);
  });
}

/**
 * Whether an open finding spans two or more sessions of one area group. When
 * it does, the group label is the collision: showing the relationship as
 * structure beats asking the reader to reassemble it from per-row warnings.
 */
function groupConverges(group: AreaGroup, findings: readonly Finding[] | undefined): boolean {
  if (!findings) return false;
  return findings.some((finding) => finding.state === "open"
    && finding.workstreamIds.filter((id) => group.sessions.some((session) => session.id === id)).length >= 2);
}

function AreaGroups({ sessions, findings, render }: { sessions: readonly Workstream[]; findings?: readonly Finding[]; render: (session: Workstream) => ReactNode }) {
  const groups = groupByArea(sessions);
  if (groups.length === 1 && groups[0]!.area === null) return <div className="rows">{groups[0]!.sessions.map(render)}</div>;
  return <>{groups.map((group) => {
    const converging = groupConverges(group, findings);
    return <div className="area-group" key={group.area ?? "__unplaced"}>
      <div className="area-label">
        {converging ? <AlertTriangle size={12} aria-hidden="true" className="area-warn" /> : <FileCode2 size={12} aria-hidden="true" />}
        {group.area ? <span>{group.area}</span> : <span className="unknown">not yet placed</span>}
        <span className="area-count">{group.sessions.length}{converging ? " · colliding" : ""}</span>
      </div>
      <div className="rows">{group.sessions.map(render)}</div>
    </div>;
  })}</>;
}

function SessionRow({ session, tick, converging, selected, onClick }: { session: Workstream; tick: number; converging: boolean; selected: boolean; onClick: () => void }) {
  const path = currentPath(session);
  const activeSubagents = session.agent?.subagents.filter((agent) => agent.status !== "done") ?? [];
  const snapshot = session.scopeSnapshot;
  return <button className="session-row" aria-current={selected ? "true" : undefined} onClick={onClick} aria-label={openSessionLabel(session)}>
    <span className="session-icon">{session.agent ? <VendorMark vendor={session.agent.vendor} size={19} /> : <Code2 size={18} />}</span>
    <span>
      <h3>{snapshot.goal.text}</h3>
      <span className="session-meta">{vendorLabel(session)}{session.agent?.branch ? ` · ${session.agent.branch}` : ""}{priorGoalCount(snapshot) > 0 ? ` · ${priorGoalCount(snapshot)} earlier ${priorGoalCount(snapshot) === 1 ? "goal" : "goals"}` : ""}{snapshot.goal.provenance === "fallback" ? " · from the opening message" : ""}</span>
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
    <span className="intent"><span>{snapshot.goal.text}</span><small>{snapshot.state}</small></span>
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

/**
 * What to say about a field's evidence, or nothing at all.
 *
 * Evidence quality is an exception, not an attribute. A field that carries the
 * best its vendor can give says nothing: repeating "high evidence" on every
 * field of every session spends the reader's attention to tell them the system
 * is working normally, and buries the one field where it is not. Only a
 * fallback or a low-confidence derivation earns a line, and it says what it was
 * derived from rather than naming an internal grade.
 */
function scopeNote(field: ScopeSnapshotField): string | null {
  if (field.provenance === "fallback") return "no declared intent; taken from the opening message";
  if (field.evidenceQuality !== "low" && field.evidenceQuality !== "none") return null;
  const source = field.facts.map((fact) => scopeFactLabels[fact]).join(" + ");
  return source ? `inferred from ${source}` : "inferred";
}

/**
 * How many goals this session has already been through, including any the
 * bounded history had to drop. The dropped ones are counted here and named as
 * dropped in the inspector, so a truncated history never reads as a whole one.
 */
function priorGoalCount(snapshot: ScopeSnapshot): number {
  return snapshot.priorGoals.length + snapshot.priorGoalsDropped;
}

/** A field with no evidence at all is not rendered; an empty labelled row is
 *  the same mistake as a filled card. */
function scopeFieldPresent(field: ScopeSnapshotField): boolean {
  return field.provenance !== "unavailable";
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
  // A session waiting on nothing has no "Waiting on" to read. Rendering the
  // label anyway asks the reader to check four rows to learn three facts.
  const present = fields.filter(({ key }) => scopeFieldPresent(snapshot[key]));
  if (present.length === 0 && snapshot.priorGoals.length === 0 && !scopeNote(snapshot.goal)) return null;
  return <section className="scope-tail" role="group" aria-label={`Scope snapshot revision ${snapshot.revision}`}>
    <header><span>{snapshot.state}</span><code>scope r{snapshot.revision}</code></header>
    {/* Where the goal came from is worth saying once, down here with the rest
        of the derivation. In the fixed header it cost content area on every
        scroll to answer a question nobody had yet. */}
    {scopeNote(snapshot.goal) && <p className="scope-goal-note">{scopeNote(snapshot.goal)}</p>}
    {/* Goals this session finished with belong before what it is doing now, in
        the order it pursued them. No timestamp: the order is the chronology,
        and the thread above already carries when things happened. */}
    {snapshot.priorGoals.length > 0 && <div className="scope-prior">
      <span className="thread-event-icon" aria-hidden="true"><CircleDot size={13} /></span>
      <span>
        <strong>Earlier in this session</strong>
        <ol>{snapshot.priorGoals.map((goal, index) => <li key={`${goal.endedAt}-${index}`}>{goal.title}</li>)}</ol>
        {snapshot.priorGoalsDropped > 0 && <small>{snapshot.priorGoalsDropped} earlier {snapshot.priorGoalsDropped === 1 ? "goal is" : "goals are"} no longer kept</small>}
      </span>
    </div>}
    <ol>{present.map(({ key, label }) => {
      const field = snapshot[key];
      const note = scopeNote(field);
      return <li className={`scope-tail-${key}`} key={key}><span className="thread-event-icon" aria-hidden="true">{icons[key]}</span><span><strong>{label}</strong><p>{field.text}</p>{note && <small>{note}</small>}</span></li>;
    })}</ol>
  </section>;
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
  return finding.kind === "direct_collision" ? "Collision detected" : finding.kind === "redundant_work" ? "Redundant work" : finding.kind === "shared_dependency" ? "Shared dependency" : finding.kind === "assumption_conflict" ? "Assumption conflict" : finding.kind === "downstream_impact" ? "Downstream impact" : finding.kind === "stale_assumption" ? "Stale assumption" : finding.kind === "dependency_ready" ? "Dependency ready" : "Possible collision";
}

function evidenceKindLabel(kind: Finding["evidence"][number]["kind"]): string {
  return ({ path: "same file", contract: "same contract", dependency: "shared dependency", intent: "related intent" } as const)[kind];
}

/**
 * Whether the reason line says anything the headline has not already said.
 *
 * On live data both sentences are derived from the same evidence, so the
 * reason often restates the title in different words and the card spends a
 * line repeating itself. Only a reason whose every significant word already
 * appears in the title folds away; an uncertain case keeps its explanation,
 * because a summary must never be an unexplained alarm.
 */
function reasonAddsFacts(finding: Finding): boolean {
  const titleWords = new Set(plainWords(finding.title).split(" "));
  return plainWords(finding.reason).split(" ").some((word) => word.length > 3 && !titleWords.has(word));
}

/** Both sides of a collision, each one a line you can open. */
function ConvergeBlock({ finding, sessions, tick, quiet = false, selected, onSelect, onOpenSession }: {
  finding: Finding; sessions: Workstream[]; tick: number; quiet?: boolean; selected: boolean; onSelect: () => void; onOpenSession: (id: string) => void;
}) {
  const affected = sessions.filter((stream) => finding.workstreamIds.includes(stream.id));
  const names = affected.map((stream) => stream.memberName).join(" and ");
  // A ready dependency is the one finding that is good news, so it reads at
  // ambient weight with a neutral mark rather than dressed as an alarm.
  const positive = finding.kind === "dependency_ready";
  return <section className={quiet || positive ? "converge quiet" : "converge"} aria-label={`${findingHeadline(finding)} ${names}`}>
    <span className="converge-icon">{positive ? <Check size={18} /> : <AlertTriangle size={18} />}</span>
    {/* The card's own headline is the control that opens it, stretched across
        the whole block so the affordance is the card rather than one line of
        text inside it. The session rows sit above that overlay and keep their
        own targets. */}
    <h3><button className="converge-open" onClick={onSelect} aria-label={`${findingHeadline(finding)} ${names}`} aria-current={selected ? "true" : undefined}>{finding.title}</button></h3>
    <span className="converge-chev" aria-hidden="true"><ChevronRight size={16} /></span>
    {reasonAddsFacts(finding) && <p className="why">{finding.reason}</p>}
    <div className="pair">{affected.map((stream) => <button className="mini" key={stream.id} onClick={() => onOpenSession(stream.id)} aria-label={`Open ${stream.memberName}'s side of this finding`}>
      <MemberChip name={stream.memberName} />
      <span className="nm">{stream.memberName}<span className="row-vendor" title={vendorLabel(stream)}>{stream.agent ? <VendorMark vendor={stream.agent.vendor} size={13} /> : <Code2 size={12} />}</span> <em>· {stream.agent?.sessionTitle ?? stream.title}</em></span>
      <Elapsed label={stream.updatedLabel} tick={tick} />
      <span className="chev"><ChevronRight size={14} /></span>
    </button>)}</div>
    <div className="converge-actions">
      {/* The headline already opens the finding; a second button opening the
          same panel was two controls for one intent. Severity earns a word
          only when it is the reason this card outranks its neighbours. */}
      <span className="converge-meta">{finding.severity === "high" || finding.severity === "critical" ? <><span className="strong">{finding.severity}</span> · </> : null}first seen <Since label={finding.firstSeen} tick={tick} /> ago</span>
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
 * One case: a finding's whole lifecycle, or a decision that never had one.
 * History used to be three independently ordered lists - Raised, Delivered,
 * Settled - which stored the lifecycle the data already carried as a puzzle
 * for the reader to reassemble. A case is the unit a person actually
 * investigates: what was caught, what was concluded, and whether the loop
 * closed.
 */
interface HistoryCase {
  id: string;
  finding: Finding | null;
  title: string;
  resolution: Resolution | null;
  /** Epoch ms of the case's newest movement, for ordering and day dividers. */
  at: number | null;
}

function caseTime(iso: string | undefined, label: string | undefined, now: number): number | null {
  if (iso) {
    const parsed = Date.parse(iso);
    if (Number.isFinite(parsed)) return parsed;
  }
  const elapsed = label ? elapsedFromLabel(label) : null;
  return elapsed === null ? null : now - elapsed * 1_000;
}

export function historyCases(snapshot: ProjectSnapshot, now: number): HistoryCase[] {
  const cards = snapshot.collaboration.syncCards;
  const cases: HistoryCase[] = snapshot.findings.map((finding) => {
    const card = cards.find((candidate) => candidate.findingId === finding.id);
    const resolution = card?.resolution ?? null;
    const at = caseTime(resolution?.createdAt ?? finding.lastSeenAt, finding.lastSeen, now);
    return { id: finding.id, finding, title: finding.title, resolution, at };
  });
  const claimed = new Set(cases.flatMap((entry) => (entry.resolution ? [entry.resolution.id] : [])));
  for (const resolution of snapshot.collaboration.resolutions) {
    if (claimed.has(resolution.id)) continue;
    const card = cards.find((candidate) => candidate.resolution?.id === resolution.id);
    cases.push({ id: resolution.id, finding: null, title: card?.title ?? "Coordination decision", resolution, at: caseTime(resolution.createdAt, undefined, now) });
  }
  // Newest movement first; anything undatable keeps its place at the end.
  return cases.sort((left, right) => (right.at ?? -Infinity) - (left.at ?? -Infinity));
}

/** Today, Yesterday, or the date - the divider a chronological log reads by. */
function dayLabel(at: number, now: number): string {
  const day = new Date(at); const today = new Date(now);
  const startOf = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  const days = Math.round((startOf(today) - startOf(day)) / 86_400_000);
  if (days === 0) return "Today";
  if (days === 1) return "Yesterday";
  return day.toLocaleDateString([], { day: "numeric", month: "short", year: day.getFullYear() === today.getFullYear() ? undefined : "numeric" });
}

function caseClock(at: number | null): string | null {
  return at === null ? null : new Date(at).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

type HistoryFilter = "all" | "open" | "settled" | "dismissed";

function caseFilterBand(entry: HistoryCase): Exclude<HistoryFilter, "all"> {
  if (entry.finding === null || entry.finding.state === "resolved") return "settled";
  if (entry.finding.state === "dismissed") return "dismissed";
  return "open";
}

/**
 * The arc is the whole story on one mono line: raised, then how it ended,
 * then whether the conclusion reached every affected agent. An unclosed loop
 * is the one thing on this screen still converging on someone, so it is the
 * one thing here allowed to take colour.
 */
function CaseArc({ entry, now }: { entry: HistoryCase; now: number }) {
  const steps: ReactNode[] = [];
  if (entry.finding) {
    const raisedAt = caseClock(caseTime(entry.finding.firstSeenAt, entry.finding.firstSeen, now));
    steps.push(<span className="step" key="raised">{findingHeadline(entry.finding).toLowerCase()}{raisedAt ? ` · ${raisedAt}` : ""}</span>);
  }
  if (entry.resolution) {
    const decidedAt = caseClock(caseTime(entry.resolution.createdAt, undefined, now));
    steps.push(<span className="step" key="decided">decided{decidedAt ? ` ${decidedAt}` : ""}</span>);
    const targets = entry.resolution.affectedWorkstreamIds.length;
    const considered = (entry.resolution.deliveries ?? []).filter((delivery) => delivery.acknowledgedAt).length;
    steps.push(<span className="step" key="delivered">sent to {targets} session{targets === 1 ? "" : "s"}</span>);
    steps.push(considered >= targets
      ? <span className="step done" key="considered">{targets === 1 ? "considered" : "all considered"}</span>
      : <span className="step waiting" key="considered">{targets - considered} not yet considered</span>);
  } else if (entry.finding?.state === "dismissed") {
    steps.push(<span className="step" key="dismissed">dismissed</span>);
  } else if (entry.finding) {
    // Open is a fact, not an alarm: the workroom is the surface that shouts
    // about open findings. Only a decided case whose conclusion has not yet
    // reached every agent takes colour here.
    steps.push(<span className="step" key="open">still open</span>);
  }
  return <div className="case-arc">{steps.flatMap((step, index) => index === 0 ? [step] : [<span className="sep" key={`sep-${index}`} aria-hidden="true">→</span>, step])}</div>;
}

/**
 * What already happened, as a case log.
 *
 * It stops at consideration. Stickguy knows a decision was routed and whether
 * the agent acknowledged reading it, and does not know whether the agent then
 * did the right thing; wording that implied otherwise would fail the fidelity
 * rules this screen is subordinate to.
 */
function HistoryView({ snapshot, tick, now, selection, onSelectFinding }: {
  snapshot: ProjectSnapshot; tick: number; now: number; selection: Selection | null;
  onSelectFinding: (id: string) => void;
}) {
  const [filter, setFilter] = useState<HistoryFilter>("all");
  const cases = useMemo(() => historyCases(snapshot, now), [snapshot, now]);
  const visible = filter === "all" ? cases : cases.filter((entry) => caseFilterBand(entry) === filter);
  const membersOn = (ids: string[]) => [...new Set(snapshot.workstreams.filter((stream) => ids.includes(stream.id)).map((stream) => stream.memberName))];
  const counts = { all: cases.length, open: 0, settled: 0, dismissed: 0 };
  for (const entry of cases) counts[caseFilterBand(entry)] += 1;

  let lastDay: string | null = null;
  return <>
    <div className="block-head lead"><h2>History</h2><span className="count">{cases.length}</span></div>
    <p className="block-note">Everything this Project has caught and what became of it, newest first.</p>
    {cases.length > 0 && <div className="case-filter" role="group" aria-label="Filter history">
      {(["all", "open", "settled", "dismissed"] as const).map((value) => <button key={value} className="text-button" aria-pressed={filter === value} onClick={() => setFilter(value)}>{value === "all" ? "All" : value === "open" ? "Open" : value === "settled" ? "Settled" : "Dismissed"}{value !== "all" && counts[value] > 0 ? ` ${counts[value]}` : ""}</button>)}
    </div>}
    {cases.length === 0 && <p className="block-empty">Nothing has been raised in this Project yet. When a finding is caught, decided, or dismissed, its whole story lands here.</p>}
    {cases.length > 0 && visible.length === 0 && <p className="block-empty">Nothing here under this filter.</p>}
    <ol className="case-log">
      {visible.map((entry) => {
        const day = entry.at === null ? null : dayLabel(entry.at, now);
        const divider = day !== null && day !== lastDay ? day : null;
        if (day !== null) lastDay = day;
        const people = entry.finding ? membersOn(entry.finding.workstreamIds) : membersOn(entry.resolution?.affectedWorkstreamIds ?? []);
        return <li key={entry.id}>
          {divider && <p className="day-divide">{divider}</p>}
          {entry.finding
            ? <button className="case" aria-current={selection?.kind === "finding" && selection.id === entry.finding.id ? "true" : undefined} onClick={() => onSelectFinding(entry.finding!.id)} aria-label={`Open ${findingHeadline(entry.finding)}`}>
                <span className="case-title">{entry.title}</span>
                <span className="case-people">{people.map((name) => <MemberChip key={name} name={name} />)}</span>
                <CaseArc entry={entry} now={now} />
                {entry.resolution && <span className="case-decision"><strong>Decision</strong> {entry.resolution.summary}</span>}
              </button>
            : <div className="case still">
                <span className="case-title">{entry.title}</span>
                <span className="case-people">{people.map((name) => <MemberChip key={name} name={name} />)}</span>
                <CaseArc entry={entry} now={now} />
                {entry.resolution && <span className="case-decision"><strong>Decision</strong> {entry.resolution.summary}</span>}
              </div>}
        </li>;
      })}
    </ol>

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
          {liveFacts && <code>{liveFacts}</code>}
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
 * The one changed declaration set behind a contract-drift finding, shown as
 * the drift it is: what the reading session believed, what is now true, and
 * who moved it. This is the exact point of divergence the engine already
 * stores; describing it in prose while discarding the signatures was the
 * projection throwing away its most persuasive artifact.
 */
function DivergenceBlock({ contract, sessions, onOpenSession }: { contract: NonNullable<Finding["evidence"][number]["contract"]>; sessions: Workstream[]; onOpenSession: (id: string) => void }) {
  const changer = contract.changedByWorkstreamId ? sessions.find((session) => session.id === contract.changedByWorkstreamId) ?? null : null;
  const symbols = contract.changedSymbols ?? [];
  if (symbols.length === 0 && !contract.path) return null;
  const timing = [
    contract.readAt ? `read ${sessionMessageTime(contract.readAt)}` : null,
    contract.changedAt ? `changed ${sessionMessageTime(contract.changedAt)}` : null,
  ].filter((value): value is string => value !== null).join(" · ");
  return <div className="contract-diff">
    <div className="cd-file">
      <code>{contract.path}</code>
      {changer
        ? <button className="text-button" onClick={() => onOpenSession(changer.id)}>changed by {changer.memberName}'s {vendorLabel(changer)} session</button>
        : <span>changed by another session</span>}
      {timing && <span className="cd-timing">{timing}</span>}
    </div>
    {symbols.map((symbol) => <div className="cd-symbol" key={symbol.name}>
      {symbol.oldSignature
        ? <div className="cd-line"><span className="cd-tag was">was</span><code>{symbol.oldSignature}</code></div>
        : <div className="cd-line"><span className="cd-tag now">new</span><code>{symbol.name}</code></div>}
      {symbol.oldSignature && (symbol.newSignature
        ? <div className="cd-line"><span className="cd-tag now">now</span><code>{symbol.newSignature}</code></div>
        : <div className="cd-line"><span className="cd-tag was">now</span><code>removed</code></div>)}
      {!symbol.oldSignature && symbol.newSignature && <div className="cd-line"><span className="cd-tag now">now</span><code>{symbol.newSignature}</code></div>}
    </div>)}
  </div>;
}

/**
 * Both sides of a two-party finding, side by side: who, what they set out to
 * do, and what they are doing right now, from the same scope snapshot the
 * workroom rows already carry. Comparing the sides used to mean opening one
 * session, going back, opening the other, and holding the first in your head.
 * Full transcripts stay behind the drill-in, which each pane still offers.
 */
function TwinPanes({ affected, onOpenSession }: { affected: Workstream[]; onOpenSession: (id: string) => void }) {
  return <div className="twin">
    {affected.map((session) => <div className="twin-side" key={session.id}>
      <button className="twin-who" onClick={() => onOpenSession(session.id)} aria-label={`Open ${session.memberName}'s session detail`}>
        <MemberChip name={session.memberName} />
        <span className="nm">{session.memberName}<span className="row-vendor" title={vendorLabel(session)}>{session.agent ? <VendorMark vendor={session.agent.vendor} size={13} /> : <Code2 size={12} />}</span></span>
        <span className="chev"><ChevronRight size={13} /></span>
      </button>
      <dl>
        <dt>Goal</dt><dd>{session.scopeSnapshot.goal.text}</dd>
        <dt>Now</dt><dd>{session.scopeSnapshot.now.text}</dd>
      </dl>
    </div>)}
  </div>;
}

/**
 * How each side of a decision is named inside a suggested outcome. A member
 * name works until both sides are the same person - the product's first case -
 * where the vendor is what tells the two sessions apart.
 */
function sideName(session: Workstream, affected: Workstream[]): string {
  const sameName = affected.filter((candidate) => candidate.memberName === session.memberName);
  return sameName.length > 1 ? `${session.memberName}'s ${vendorLabel(session)} session` : session.memberName;
}

/**
 * Suggested outcomes, phrased per finding kind. A chip prefills the composer
 * rather than acting on its own: the summary is what gets injected into the
 * affected agents' turns, so the member always sees and can edit the exact
 * words before anything is sent. "Settled outside Stickguy" is a first-class
 * outcome, because a conclusion reached in Slack still has to reach the
 * agents that are working from stale assumptions.
 */
function outcomeTemplates(finding: Finding, affected: Workstream[], sessions: Workstream[]): string[] {
  const templates: string[] = [];
  const [first, second] = affected;
  const a = first ? sideName(first, affected) : "one side";
  const b = second ? sideName(second, affected) : "the other";
  if (finding.kind === "direct_collision" || finding.kind === "likely_collision") {
    if (first && second) {
      templates.push(`${a}'s change lands first — ${b} rebases on the result before continuing. `);
      templates.push(`${b}'s change lands first — ${a} rebases on the result before continuing. `);
      templates.push(`Both continue — the boundary between them is: `);
    }
  } else if (finding.kind === "redundant_work") {
    if (first && second) {
      templates.push(`Keep ${a}'s implementation — ${b} winds theirs down. `);
      templates.push(`Keep ${b}'s implementation — ${a} winds theirs down. `);
    }
  } else if (finding.kind === "stale_assumption") {
    const contract = finding.evidence.find((item) => item.contract)?.contract;
    const subject = finding.evidence.find((item) => item.subject)?.subject ?? contract?.path ?? "the contract";
    const changer = contract?.changedByWorkstreamId ? sessions.find((session) => session.id === contract.changedByWorkstreamId) : undefined;
    templates.push(`The new ${subject} shape is canonical — rework what was built against the old one. `);
    templates.push(`The old ${subject} shape stands — ${changer ? sideName(changer, affected.concat(changer)) : "the session that changed it"} reverts. `);
  } else if (finding.kind === "shared_dependency" || finding.kind === "downstream_impact" || finding.kind === "assumption_conflict") {
    if (first && second) templates.push(`${a} lands first — ${b} picks up the new revision before continuing. `);
  }
  templates.push("Settled outside Stickguy — the conclusion: ");
  return templates;
}

/** The send control names its targets, because delivery is the actual effect. */
function sendLabel(affected: Workstream[]): string {
  if (affected.length === 0) return "Send to the affected sessions";
  if (affected.length === 1) return `Send to ${affected[0]!.memberName}'s session`;
  if (affected.length === 2) return "Send to both sessions";
  return `Send to ${affected.length} sessions`;
}

/**
 * The decision's afterlife, where the decision was made. Delivery and
 * acknowledgement were always recorded; they were only ever readable in
 * History, so the member who just typed a decision was left in exactly the
 * uncertainty the system had already resolved. Wording stays subordinate to
 * honest fidelity: considered proves the agent read it, never that it
 * complied.
 */
function ResolutionTracker({ resolution, sessions, onOpenSession }: { resolution: Resolution; sessions: Workstream[]; onOpenSession: (id: string) => void }) {
  return <div className="deliv">
    {resolution.affectedWorkstreamIds.map((id) => {
      const session = sessions.find((candidate) => candidate.id === id);
      const delivery = resolution.deliveries?.find((candidate) => candidate.workstreamId === id);
      const state = delivery?.acknowledgedAt
        ? { label: `considered · ${sessionMessageTime(delivery.acknowledgedAt)}`, done: true }
        : delivery
          ? { label: `delivered · ${sessionMessageTime(delivery.deliveredAt)}`, done: false }
          : { label: "queued for its next turn", done: false };
      return <div className="deliv-row" key={id}>
        {session
          ? <button className="text-button deliv-who" onClick={() => onOpenSession(id)}><MemberChip name={session.memberName} />{session.memberName}'s {vendorLabel(session)} session</button>
          : <span className="deliv-who gone">a session no longer in this Project</span>}
        <span className={state.done ? "deliv-state done" : "deliv-state"}>{state.label}</span>
      </div>;
    })}
    <p className="deliv-note">Considered records that the agent read the decision, not that it followed it.</p>
  </div>;
}

/**
 * A finding and its decision are one object at two ages, so the divergence,
 * both sides, and the decision live here rather than behind separate tabs.
 * Three exits and no more: send a decision to the affected sessions, record
 * that it was settled elsewhere (which still sends the conclusion), or
 * dismiss it with the reason - which is also the feedback that trains the
 * engine. Acknowledge and the standalone feedback row expressed the same
 * intents twice and are gone.
 */
function FindingInspector({ finding, sessions, projectId, source, card, disabled, onState, onFeedback, onOpenSession, back }: {
  finding: Finding; sessions: Workstream[]; projectId: string; source: FixtureProjectSource; card: SyncCard | null; disabled: boolean;
  onState: (state: FindingState) => Promise<void>; onFeedback: (value: FindingFeedback) => Promise<void>; onOpenSession: (id: string) => void; back: InspectorBack | null;
}) {
  const affected = useMemo(() => sessions.filter((session) => finding.workstreamIds.includes(session.id)), [finding.workstreamIds, sessions]);
  const [decision, setDecision] = useState("");
  const [threadPending, setThreadPending] = useState(false);
  const [threadError, setThreadError] = useState("");
  const [dismissOpen, setDismissOpen] = useState(false);
  const [statePending, setStatePending] = useState(false);
  const [stateError, setStateError] = useState(false);
  const names = affected.map((session) => session.memberName).join(" and ");
  const contract = finding.evidence.find((item) => item.contract?.changedSymbols?.length || item.contract?.path)?.contract ?? null;
  const plainEvidence = finding.evidence.filter((item) => !item.contract);

  const run = (operation: () => Promise<void>) => {
    setThreadPending(true); setThreadError("");
    void operation()
      .catch((cause: unknown) => setThreadError(cause instanceof Error && cause.message.includes("revision") ? "Someone changed this first. Reload and try again." : "That change could not be saved."))
      .finally(() => setThreadPending(false));
  };
  // One step: the card is plumbing, created on the way past when the first
  // decision lands. A button that created an empty card and then revealed the
  // composer was a workflow state with no user meaning.
  const submitDecision = () => {
    const summary = decision.trim();
    if (!summary) return;
    run(async () => {
      const target = card ? { id: card.id, revision: card.revision } : await source.createSyncCard(projectId, finding.id, finding.title, finding.reason);
      await source.resolveSyncCard(projectId, target.id, target.revision, summary, finding.workstreamIds);
      setDecision("");
    });
  };
  // Dismissing and telling the engine why are one gesture: the reasons are the
  // feedback vocabulary, so closing the item is what trains the detector.
  const dismiss = (reason: FindingFeedback) => {
    setStatePending(true); setStateError(false);
    void onFeedback(reason)
      .then(() => onState("dismissed"))
      .catch(() => setStateError(true))
      .finally(() => { setStatePending(false); setDismissOpen(false); });
  };

  const kindLabel = findingHeadline(finding);
  // The hosted snapshot writes a finding's title as its kind with the
  // underscores taken out, so on live data the title and the kind are the same
  // sentence twice. Print the kind as an eyebrow only when the title is
  // genuinely something else; the reason carries the content either way.
  const titleRepeatsKind = plainWords(finding.title) === plainWords(kindLabel);

  return <article className="collision-detail" aria-label="Selected finding detail">
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
          derived is in the evidence blocks below. */}
      <p className="collision-reason">{finding.reason}</p>
      <p className="branch-context">{branchStatement(affected)}</p>

      <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true"><Users size={13} /></span><span>Sessions</span><span className="inspector-head-count">{affected.length}</span></h3>
      {affected.length === 2
        ? <TwinPanes affected={affected} onOpenSession={onOpenSession} />
        : <div className="pair">{affected.map((session) => <button className="mini" key={session.id} onClick={() => onOpenSession(session.id)} aria-label={`Open ${session.memberName}'s session detail`}>
            <span className="mini-icon">{session.agent ? <VendorMark vendor={session.agent.vendor} size={17} /> : <Code2 size={16} />}</span>
            <span className="nm">{session.memberName} <em>· {vendorLabel(session)}</em></span>
            <span className="clock">{session.updatedLabel}</span>
            <span className="chev"><ChevronRight size={14} /></span>
            <span className="doing">{session.outcome}</span>
          </button>)}</div>}

      {contract && <>
        <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true"><FileCode2 size={13} /></span><span>What changed</span></h3>
        <DivergenceBlock contract={contract} sessions={sessions} onOpenSession={onOpenSession} />
      </>}

      {plainEvidence.length > 0 && <>
        <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true"><Search size={13} /></span><span>Evidence</span></h3>
        <ul className="evidence-list">{plainEvidence.map((item) => <li key={`${item.kind}-${item.label}`}>
          <span className="e-label">{item.label}</span>
          <span className="e-src">{evidenceKindLabel(item.kind)} · {item.source.replaceAll("_", " ")}</span>
        </li>)}</ul>
      </>}
      <p className="evidence-age">first seen {finding.firstSeen} · last changed {finding.lastSeen}</p>

      <h3 className="inspector-head"><span className="inspector-head-icon" aria-hidden="true"><Check size={13} /></span><span>Decision</span></h3>
      {/* The decision is the one thing on this screen that reaches an agent,
          so it is the composer rather than a field in a row of fields, it
          names its own delivery before it is written, and after sending it
          shows that delivery actually happening. */}
      {card?.resolution
        ? <>
            <div className="decision-note"><div className="lbl"><Check size={13} />Decision from {names}</div><p>{card.resolution.summary}</p></div>
            <ResolutionTracker resolution={card.resolution} sessions={sessions} onOpenSession={onOpenSession} />
          </>
        : <>
            <div className="outcomes" aria-label="Suggested outcomes">
              {outcomeTemplates(finding, affected, sessions).map((template) => <button key={template} className="pill outcome" disabled={disabled || threadPending} onClick={() => setDecision(template)}>{template.replace(/[—:]\s*$/, "").trim()}</button>)}
            </div>
            <form className="composer" onSubmit={(event) => { event.preventDefault(); submitDecision(); }}>
              <textarea
                value={decision} rows={2} placeholder="How should the work proceed?" aria-label={`Decision for ${finding.title}`}
                onChange={(event) => setDecision(event.target.value)}
                onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === "Enter") { event.preventDefault(); submitDecision(); } }}
              />
              <div className="composer-foot">
                <span className="routing-note"><Route size={13} aria-hidden="true" />{routingStatement(affected)}</span>
                <button className="pill solid" disabled={threadPending || disabled || decision.trim().length === 0}>{sendLabel(affected)}</button>
              </div>
            </form>
          </>}
      <p className="advisory-note">Advisory only. Stickguy delivers the decision and never blocks or controls an agent.</p>
      {threadError && <p className="form-error" role="alert">{threadError}</p>}

      {finding.state === "open" && !card?.resolution && <div className="finding-foot">
        {/* The second exit: this is not worth a decision. The reason is the
            feedback, so one gesture closes the item and trains the engine. */}
        <div className="finding-actions">
          {dismissOpen
            ? <>
                <span className="dismiss-lead">Dismiss because it is</span>
                <button disabled={disabled || statePending} className="pill" onClick={() => dismiss("not_related")}>Not related</button>
                <button disabled={disabled || statePending} className="pill" onClick={() => dismiss("already_coordinated")}>Already coordinated</button>
                <button disabled={statePending} className="text-button" onClick={() => setDismissOpen(false)}>Cancel</button>
              </>
            : <button disabled={disabled || statePending} className="pill" onClick={() => setDismissOpen(true)}>Dismiss</button>}
        </div>
      </div>}
      {stateError && <p className="form-error" role="alert">That change could not be saved.</p>}
    </div>
  </article>;
}

/** Comparable words only, so "stale assumption" and "Stale assumption" match. */
function plainWords(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, " ").trim();
}

function InspectorEmpty() {
  return <div className="inspector-empty"><Bot size={20} /><h2>Nothing selected</h2><p>Choose a session or a finding to inspect it here.</p></div>;
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

export function FixtureDataBanner() {
  return <div className="desktop-preview-banner" role="status"><strong>Sample data</strong><span>Design fixture harness · nothing here is a real Project</span></div>;
}

/**
 * The landing an invite link opens on. It renders before any authentication -
 * the recipient has no session by definition - and it never transmits the
 * invite: the code rides the URL fragment, which browsers keep out of request
 * lines and server logs, and this page only reads it back into instructions.
 *
 * Deliberately minimal until the onboarding UI pass: its job is that a shared
 * link never dead-ends, not to be the finished welcome.
 */
export function JoinLanding({ fragment = window.location.hash.slice(1) }: { fragment?: string }) {
  const valid = /^inv_[A-Za-z0-9]+\.[A-Za-z0-9_-]+$/.test(fragment);
  if (!valid) {
    return <main className="centered-shell"><Brand /><section className="state-card" role="alert"><span className="state-symbol"><AlertTriangle size={20} /></span><p className="eyebrow">Invite link</p><h1>This invite link is incomplete.</h1><p>The part after <code>#</code> is missing or damaged. Ask whoever invited you to copy the link again from People &rarr; Invite a teammate.</p></section></main>;
  }
  const command = `stickguy join ${window.location.origin}/join#${fragment}`;
  return <main className="centered-shell"><Brand /><section className="state-card" aria-labelledby="join-title"><span className="state-symbol"><UserPlus size={20} /></span><p className="eyebrow">Project invite</p><h1 id="join-title">You&rsquo;ve been invited to a Stickguy Project.</h1><p>Stickguy coordinates coding agents working in the same repository. Joining shares session presence and classifier-passing coordination facts &mdash; never source, prompts, or credentials.</p><div className="disclosure"><strong>1. Install Stickguy</strong><p>Already installed? Skip ahead. Otherwise run:</p><code>{`curl -fsSL ${window.location.origin}/install.sh | sh`}</code></div><div className="disclosure"><strong>2. Join from your checkout</strong><p>Run this inside the repository this Project coordinates:</p><code>{command}</code></div><button className="pill solid" onClick={() => { void navigator.clipboard?.writeText(command); }}>Copy the command</button><p className="microcopy">This invite is one-use and expires seven days after it was created. The code after # stays in your browser; this page sends it nowhere.</p></section></main>;
}

const root = document.getElementById("root");
if (root) {
  const parameters = new URLSearchParams(window.location.search);
  const desktopPreview = parameters.get("desktop") === "preview" || window.location.protocol === "wails:";
  const onboarding = parameters.get("desktop") === "onboarding";
  // Fixtures are a design harness, so they are opt-in and always labelled. They
  // used to be what an unrecognised URL fell back to, which meant any plain
  // browser hit on this origin rendered an invented Project - session titles,
  // findings, agent transcripts - with nothing on screen saying it was fake.
  // The live view already has an honest unauthenticated state, so it is the
  // safe thing to land on when the URL asks for nothing in particular.
  const fixtures = parameters.get("fixtures") === "1" || (desktopPreview && parameters.get("live") !== "1");
  const banner = !onboarding && (fixtures || desktopPreview);
  if (banner) document.documentElement.dataset.desktopPreview = "true";
  createRoot(root).render(<StrictMode>
    {banner && (desktopPreview ? <DesktopPreviewBanner live={!fixtures} /> : <FixtureDataBanner />)}
    {window.location.pathname.replace(/\/+$/, "") === "/join" ? <JoinLanding /> : onboarding ? <DesktopOnboarding /> : fixtures ? <App /> : <LiveApp />}
  </StrictMode>);
}
