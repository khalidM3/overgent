import { useEffect, useRef, useState, type FormEvent } from "react";
import { Screen, ScreenSection } from "./screen";
import { desktopHandoffURL, isDesktopShell, type AdapterState, type EnrollmentRequest, type NativeOnboarding } from "./native";

/**
 * Adding a Project is a screen, in both places it can be reached: the hosted
 * workroom sidebar and the desktop shell. It is deliberately the same component
 * and the same layout in both, because the hosted half cannot finish the job -
 * enrollment registers a repository with the local service, and only the desktop
 * shell can reach it. Handing over between the two should read as one screen
 * continuing, not as the app relaunching itself.
 */
export function NewProjectScreen({ api, displayName, navigate, backLabel, onBack, mode: initialMode = "create", placement: initialPlacement = "local", onPlacement: externalPlacement, localAvailable = false, defaultServer = "", returnProjectId = "", onMode: externalMode }: {
  onMode?: (mode: "create" | "join") => void;
  api: NativeOnboarding;
  displayName: string;
  navigate: (url: string) => void;
  backLabel: string;
  onBack: () => void;
  /**
   * Creating a Project and accepting an invite to one are the same screen with
   * one field swapped, and both end in the same place. They are separate calls
   * underneath - joining reuses this Mac's device identity for that backend
   * rather than minting one - but that is not a distinction a member should
   * have to make.
   */
  mode?: "create" | "join";
  /**
   * Where this Project's coordination data lives. Every Project binds to its
   * own backend (ADR-074), so the answer for this one says nothing about the
   * Projects already on this Mac and is asked again each time.
   */
  placement?: "local" | "team";
  onPlacement?: (placement: "local" | "team") => void;
  localAvailable?: boolean;
  /** The server a team Project defaults to, shown as the field's placeholder. */
  defaultServer?: string;
  /**
   * The Project this screen was opened from, when it was opened from one.
   * Registering a repository happens on the shell's own origin, so reaching
   * this screen from a live Project navigates the window off it; this is what
   * lets the setup screen put the member back where they were.
   */
  returnProjectId?: string;
}) {
  const [mode, setMode] = useState(initialMode);
  const [placement, setPlacement] = useState(initialPlacement);
  useEffect(() => setMode(initialMode), [initialMode]);
  useEffect(() => setPlacement(initialPlacement), [initialPlacement]);
  const onMode = (value: "create" | "join") => { setMode(value); externalMode?.(value); };
  const onPlacement = (value: "local" | "team") => { setPlacement(value); externalPlacement?.(value); };
  const joining = mode === "join";
  const local = placement === "local" && !joining;
  // Focus lands on the first action, which is choosing a repository, not on
  // the name field below it: focusing the name scrolled the screen's own
  // heading and the picker out of view on open.
  const chooseRef = useRef<HTMLButtonElement>(null);
  const [request, setRequest] = useState<EnrollmentRequest>({ repositoryRoot: "", projectLabel: "", deviceLabel: "This Mac", displayName, joinCode: "", serverOrigin: "", enableCodex: false, enableClaude: false, enableCursor: false });
  const [knownProjects, setKnownProjects] = useState<import("./native").ProjectState[]>([]);
  const [enrolled, setEnrolled] = useState(false);
  const [canRunLocal, setCanRunLocal] = useState(localAvailable);
  const [adapters, setAdapters] = useState<AdapterState[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState<{ projectId: string; joinCode: string; warnings: string[] } | null>(null);
  // Enrollment runs through the local service, which only exists inside the
  // desktop shell. Until the probe answers, say it is being checked. The
  // previous version rendered the form while probing and then replaced it with
  // the hand-off, so a member could start filling in a form that vanished under
  // them and be told to open the app they were already looking at.
  const [bridge, setBridge] = useState<"probing" | "ready" | "unavailable">("probing");
  useEffect(() => {
    let cancelled = false;
    void api.state()
      .then((state) => {
        if (cancelled) return;
        setBridge("ready");
        setKnownProjects(state.projects ?? []);
        setEnrolled(state.enrolled);
        setCanRunLocal(Boolean(state.localAvailable));
        setAdapters(state.adapters ?? []);
        // The same reasoning as first run: this Mac has already been asked which
        // agents are installed, so leaving them unticked makes the likeliest way
        // through the form create a Project that observes Git and nothing else.
        // Installed is a property of the machine, not of the Project being
        // added, so it is the right default for a second one too.
        setRequest((current) => ({
          ...current,
          deviceLabel: state.deviceLabel || current.deviceLabel,
          enableCodex: agentDefault("codex", installed(state.adapters, "Codex")),
          enableClaude: agentDefault("claude", installed(state.adapters, "Claude Code")),
          enableCursor: agentDefault("cursor", installed(state.adapters, "Cursor")),
        }));
      })
      .catch(() => { if (!cancelled) setBridge("unavailable"); });
    return () => { cancelled = true; };
  }, [api]);
  useEffect(() => { if (bridge === "ready" && !created) chooseRef.current?.focus({ preventScroll: true }); }, [bridge, created]);

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
      const existing = knownProjects.find((project) => project.repositoryRoot === request.repositoryRoot);
      if (existing && !joining) {
        navigate(await api.openLiveProject(existing.projectId));
        return;
      }
      const result = joining
        ? await (enrolled ? api.joinAdditionalProject(request) : api.joinProject(request))
        : local ? await api.createLocalProject(request) : await (enrolled ? api.createAdditionalProject(request) : api.createProject(request));
      setCreated({ ...result, warnings: Array.isArray(result.warnings) ? result.warnings : [] });
      if (!result.warnings?.length && !result.joinCode) navigate(await api.openLiveProject(result.projectId));
    } catch (cause) { setError((cause as Error).message); }
    finally { setPending(false); }
  };
  const open = async () => {
    if (!created) return;
    setPending(true); setError("");
    try { navigate(await api.openLiveProject(created.projectId)); }
    catch (cause) { setError((cause as Error).message); setPending(false); }
  };

  if (bridge === "probing") {
    return <Screen title={joining ? "Join a Project" : "Open a repository"} backLabel={backLabel} onBack={onBack} lede="Checking whether this window can reach the Overgent service on your Mac.">
      <div className="screen-actions" role="status"><span className="spinner" aria-hidden="true" /><span className="settings-help">Checking this Mac…</span></div>
    </Screen>;
  }

  if (bridge === "unavailable") return <Handoff backLabel={backLabel} onBack={onBack} navigate={navigate} returnProjectId={returnProjectId} />;

  if (created) {
    return <Screen
      title={joining ? "You’re in." : `${request.projectLabel} is ready.`}
      sub={request.repositoryRoot}
      backLabel={backLabel}
      onBack={onBack}
      lede="Start a new agent session in this repository to see it here."
    >
      <ScreenSection title="Your Project">
        <div className="screen-actions">
          <button className="pill solid" disabled={pending} onClick={() => void open()}>{pending ? "Opening…" : "Open Project"}</button>
          <button className="pill" onClick={onBack}>Not now</button>
        </div>
        {created.warnings.map((warning) => <p className="form-warning" key={warning}>{warning}</p>)}
        {error && <p className="form-error" role="alert">{error}</p>}
      </ScreenSection>
      {/* An invite is an option, not the next step. A Project with one member
          already does the whole job for the sessions that member is running,
          and presenting a code as the thing to do next implied otherwise. */}
      {created.joinCode && <ScreenSection title="Invite someone, when you want to" help="Optional. Nothing here waits on a second member. The code is one use and expires in seven days; share it privately.">
        <div className="invite-code"><strong>One-use invite code</strong><code>{created.joinCode}</code></div>
      </ScreenSection>}
    </Screen>;
  }

  return <Screen
    title={joining ? "Join a Project" : "Open a repository"}
    backLabel={backLabel}
    onBack={onBack}
    lede={joining
      ? "Paste your invite and choose your checkout. Your code stays on this Mac."
      : "Coordinate the agent sessions in your repository. No account required."}
  >
    {onMode && <nav className="settings-tabs" aria-label="Add Project options">
      <button className="text-button" aria-current={!joining ? "page" : undefined} onClick={() => onMode("create")}>Open a repository</button>
      <button className="text-button" aria-current={joining ? "page" : undefined} onClick={() => onMode("join")}>Join with an invite</button>
    </nav>}
    <form onSubmit={(event) => void submit(event)}>
      <div className="screen-form">
        {joining && <label><span>Invite code</span><input value={request.joinCode} onChange={(event) => setRequest({ ...request, joinCode: event.target.value })} placeholder="Paste an invite link or code" autoComplete="off" /></label>}
        <label><span>Repository</span><div className="repository-picker"><input readOnly value={request.repositoryRoot} placeholder="Choose a local Git repository" /><button ref={chooseRef} type="button" className="pill" onClick={() => void chooseRepository()}>Choose…</button></div></label>
        {!joining && <details className="field-advanced"><summary>Project name</summary><label><span>Name</span><input value={request.projectLabel} maxLength={120} onChange={(event) => setRequest({ ...request, projectLabel: event.target.value })} placeholder="Repository name" /></label></details>}
        {(joining || !local) && <label><span>Your name</span><input value={request.displayName} maxLength={60} placeholder="How collaborators see you" onChange={(event) => setRequest({ ...request, displayName: event.target.value })} /></label>}
        {!joining && onPlacement && <details className="field-advanced"><summary>Collaborate remotely</summary>
          <label className="inline-option"><input type="checkbox" checked={!local} onChange={(event) => onPlacement(event.target.checked ? "team" : "local")} /> Create a shared Project</label>
          <p className="field-note">Shared Projects store coordination and approved session context on Overgent Cloud. No organization or account setup.</p>
        </details>}
        {/* Self-hosting and Overgent Cloud are the same client against a
            different origin. An invite link carries its own origin, so the
            field is offered only where it decides anything. */}
        {!joining && !local && <details className="field-advanced">
          <summary>Advanced: connect to a different server</summary>
          <label><span>Server address</span><input aria-label="Server address" value={request.serverOrigin} maxLength={200} onChange={(event) => setRequest({ ...request, serverOrigin: event.target.value })} placeholder={defaultServer || "https://api.overgent.com"} autoComplete="off" spellCheck={false} /></label>
          <p className="field-note">Leave this empty to use {"Overgent Cloud"}. Your own deployment is described in docs/self-hosting.md.</p>
        </details>}
      </div>

      <details className="field-advanced agent-disclosure"><summary>Coding agents · {adapters.filter((adapter) => adapter.installed).map((adapter) => adapter.name).join(", ") || "none detected"}</summary>
        <p className="field-note">Detected agents are connected by default. Change this here or in Integrations. Existing sessions need a restart.</p>
        <AgentOptions adapters={adapters} request={request} onChange={setRequest} />
      </details>

      <ScreenSection title={local ? "Private to this Mac" : "Shared with Project members"}>
        <p className="settings-help">{local
          ? "Coordination and session context stay on this Mac. Optional AI providers can be configured later in Intelligence."
          : "Members can see activity, file paths, coordination facts, and classifier-approved session messages while sharing is unpaused. Source files, raw diffs, transcript files, secrets, and command output are never uploaded."}</p>
        <p className="field-note">Opening connects the selected agents and starts background observation for this repository.</p>
        {local && !canRunLocal && <p className="form-warning" role="status">Local coordination is unavailable in this build. Install a build with local support, or explicitly choose a shared Project above.</p>}
        <div className="screen-actions">
          <button className="pill solid" disabled={pending || (local && !canRunLocal) || !request.repositoryRoot || (joining ? !request.joinCode.trim() : !request.projectLabel.trim())}>{pending ? (joining ? "Joining…" : "Creating…") : (joining ? "Join Project" : local ? "Open Project" : "Create shared Project")}</button>
          <button className="pill" type="button" onClick={onBack}>Cancel</button>
        </div>
        {error && <p className="form-error" role="alert">{error}</p>}
      </ScreenSection>
    </form>
  </Screen>;
}

/** Whether this Mac has the vendor's executable, whatever any Project has bound. */
function installed(adapters: AdapterState[] | undefined, name: string): boolean {
  return (adapters ?? []).some((adapter) => adapter.name === name && adapter.installed);
}

export function AgentOptions({ adapters, request, onChange }: {
  adapters: AdapterState[];
  request: EnrollmentRequest;
  onChange: (request: EnrollmentRequest) => void;
}) {
  const rows: Array<{ name: string; key: "enableCodex" | "enableClaude" | "enableCursor"; found: string; missing: string }> = [
    { name: "Codex", key: "enableCodex", found: "Found on this Mac · live title intent, tools, subagents, safe paths", missing: "Not found · connect anyway and sessions appear once Codex opens this repository" },
    { name: "Claude Code", key: "enableClaude", found: "Found on this Mac · live title intent, tools, subagents, safe paths", missing: "Not found · connect anyway and sessions appear once Claude Code opens this repository" },
    { name: "Cursor", key: "enableCursor", found: "Found on this Mac · live prompt intent, edits, and observed file reads", missing: "Not found · connect anyway and sessions appear once Cursor opens this repository" },
  ];
  return <fieldset className="agent-options">
    {rows.map((row) => <label key={row.name}>
      <input type="checkbox" checked={request[row.key]} onChange={(event) => onChange({ ...request, [row.key]: event.target.checked })} />
      <span><strong>{row.name}</strong><small>{installed(adapters, row.name) ? row.found : row.missing}</small></span>
    </label>)}
  </fieldset>;
}

/**
 * The window cannot reach the local service, so this screen continues the task
 * somewhere that can.
 *
 * Where that is depends on where this page is, and the previous version got it
 * exactly backwards. It always offered `overgent://new-project`, which is right
 * in a browser and inert inside the desktop window: WKWebView never hands a
 * custom scheme to the system, so the one control on the screen did nothing in
 * the one place it was most likely to be pressed - a member looking at the live
 * Project view *inside* the app, being told to open the app.
 *
 * So: inside the shell this navigates to the shell's own origin and reads as
 * this screen carrying on. In a browser it is an app hand-off and says so. Both
 * name a fallback that does not depend on the hand-off working, because a
 * control that silently does nothing is the failure this replaces.
 */
function Handoff({ backLabel, onBack, navigate, returnProjectId = "" }: { backLabel: string; onBack: () => void; navigate: (url: string) => void; returnProjectId?: string }) {
  const inShell = isDesktopShell;
  const [handedOff, setHandedOff] = useState(false);
  const [stalled, setStalled] = useState(false);
  const handoff = () => {
    setHandedOff(true);
    // The Project being left travels with the hand-off, so the screen on the
    // other side can come back to it rather than stranding the member on the
    // setup screen's own home.
    navigate(desktopHandoffURL(returnProjectId));
  };
  // Inside the app there is nothing to decide: the member asked to add a
  // Project and the only screen that can is one navigation away, on this same
  // window. Asking them to press a second button to continue somewhere they
  // cannot see is the interstitial this used to be.
  useEffect(() => { if (inShell) handoff(); }, [inShell]);
  useEffect(() => {
    if (!handedOff) return;
    const timer = window.setTimeout(() => setStalled(true), 2_500);
    return () => window.clearTimeout(timer);
  }, [handedOff]);

  if (inShell) {
    return <Screen
      title="Add a Project"
      backLabel={backLabel}
      onBack={onBack}
      lede="Registering a repository happens where the Overgent service runs, so this screen continues on this Mac. The repository picker and agent options are supplied by the service itself."
    >
      <div className="screen-actions" role="status"><span className="spinner" aria-hidden="true" /><span className="settings-help">Continuing on this Mac…</span></div>
      {stalled && <ScreenSection title="Still on this screen?" help="The window did not move. Nothing was created, and nothing was changed.">
        <div className="screen-actions">
          <button className="pill solid" onClick={handoff}>Continue on this Mac</button>
          <button className="pill" onClick={onBack}>Cancel</button>
        </div>
        <p className="handoff-fallback">Or open <strong>Add a project…</strong> from the Overgent icon in the menu bar, which reaches the same screen without leaving this window to do it.</p>
      </ScreenSection>}
    </Screen>;
  }

  return <Screen
    title="Add a Project"
    backLabel={backLabel}
    onBack={onBack}
    lede="A Project registers a Git repository with the Overgent service running on the Mac that holds it. This is the shared Project view in a browser, and it cannot reach that service — on this Mac or anyone else’s."
  >
    <ScreenSection title="Continue in the Overgent app" help="You land on this same screen there, with the repository picker and agent options supplied by the local service.">
      <div className="screen-actions">
        <button className="pill solid" onClick={handoff}>Open the Overgent app</button>
        <button className="pill" onClick={onBack}>Cancel</button>
      </div>
      {stalled && <p className="handoff-fallback">If the app did not come forward, open it yourself and choose <strong>Add a project…</strong> from the Overgent icon in the menu bar — or run <code>overgent create</code> inside the repository.</p>}
    </ScreenSection>
  </Screen>;
}

/** Only non-secret local preferences; an unavailable storage area keeps detection defaults. */
export function agentDefault(vendor: string, detected: boolean): boolean {
  try { return detected && localStorage.getItem(`overgent.agent.${vendor}`) !== "off"; }
  catch { return detected; }
}
