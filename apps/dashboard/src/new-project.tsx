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
export function NewProjectScreen({ api, displayName, navigate, backLabel, onBack, mode = "create", placement = "team", onPlacement, localAvailable = false, defaultServer = "" }: {
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
}) {
  const joining = mode === "join";
  const local = placement === "local" && !joining;
  // Focus lands on the first action, which is choosing a repository, not on
  // the name field below it: focusing the name scrolled the screen's own
  // heading and the picker out of view on open.
  const chooseRef = useRef<HTMLButtonElement>(null);
  const [request, setRequest] = useState<EnrollmentRequest>({ repositoryRoot: "", projectLabel: "", deviceLabel: "This Mac", displayName, joinCode: "", serverOrigin: "", enableCodex: false, enableClaude: false, enableCursor: false });
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
        setAdapters(state.adapters ?? []);
        // The same reasoning as first run: this Mac has already been asked which
        // agents are installed, so leaving them unticked makes the likeliest way
        // through the form create a Project that observes Git and nothing else.
        // Installed is a property of the machine, not of the Project being
        // added, so it is the right default for a second one too.
        setRequest((current) => ({
          ...current,
          deviceLabel: state.deviceLabel || current.deviceLabel,
          enableCodex: installed(state.adapters, "Codex"),
          enableClaude: installed(state.adapters, "Claude Code"),
          enableCursor: installed(state.adapters, "Cursor"),
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
      const result = joining
        ? await api.joinAdditionalProject(request)
        : local ? await api.createLocalProject(request) : await api.createAdditionalProject(request);
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

  if (bridge === "probing") {
    return <Screen title={joining ? "Join a Project" : "Add a Project"} backLabel={backLabel} onBack={onBack} lede="Checking whether this window can reach the Overgent service on your Mac.">
      <div className="screen-actions" role="status"><span className="spinner" aria-hidden="true" /><span className="settings-help">Checking this Mac…</span></div>
    </Screen>;
  }

  if (bridge === "unavailable") return <Handoff backLabel={backLabel} onBack={onBack} navigate={navigate} />;

  if (created) {
    return <Screen
      title={joining ? "Project joined" : "Project created"}
      sub={request.repositoryRoot}
      backLabel={backLabel}
      onBack={onBack}
      lede={joining
        ? "This repository is registered with the Overgent service already running on this Mac, under the same device identity as your other Projects. No second background service was started."
        : `${request.projectLabel} is registered with this Mac’s existing Overgent service. No second background service was started. It coordinates the agent sessions you run in this repository straight away.`}
    >
      <ScreenSection title="Open it" help="Opening a Project mints a fresh one-time session for it, so the next step asks you to confirm before the shared view loads.">
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
    title={joining ? "Join a Project" : "Add a Project"}
    backLabel={backLabel}
    onBack={onBack}
    lede={joining
      ? "Paste the invite you were sent, and choose your own checkout of the repository that Project coordinates. Your teammate’s Mac is not involved."
      : "A Project is one Git repository on this Mac. Overgent coordinates every agent session you run in it — yours alone, or a team’s."}
  >
    <form onSubmit={(event) => void submit(event)}>
      {/* Where this Project lives is asked before anything else, because it is
          the only question on this screen whose answer cannot be changed
          afterwards. A Mac that already has one kind can still add the other. */}
      {!joining && onPlacement && <ScreenSection title="Where this Project’s data lives" help={local
        ? "Nothing leaves this computer. A Project on this Mac needs no account and no network."
        : "Coordination data is stored on Overgent Cloud, or on the server you name below."}>
        <div className="screen-actions">
          <button type="button" className={local ? "pill solid" : "pill"} disabled={!localAvailable} onClick={() => onPlacement("local")}>Use on this Mac</button>
          <button type="button" className={local ? "pill" : "pill solid"} onClick={() => onPlacement("team")}>Team Project</button>
        </div>
        {!localAvailable && <p className="field-note" role="status">This build does not carry a backend to run on this Mac, so a Project needs a server.</p>}
      </ScreenSection>}
      <div className="screen-form">
        {joining && <label><span>Invite code</span><input value={request.joinCode} onChange={(event) => setRequest({ ...request, joinCode: event.target.value })} placeholder="inv_abc123.secret" autoComplete="off" /></label>}
        <label><span>Repository</span><div className="repository-picker"><input readOnly value={request.repositoryRoot} placeholder="Choose a local Git repository" /><button ref={chooseRef} type="button" className="pill" onClick={() => void chooseRepository()}>Choose…</button></div></label>
        {!joining && <label><span>Project name</span><input value={request.projectLabel} maxLength={120} onChange={(event) => setRequest({ ...request, projectLabel: event.target.value })} placeholder="Atlas launch" /></label>}
        {/* Self-hosting and Overgent Cloud are the same client against a
            different origin. An invite link carries its own origin, so the
            field is offered only where it decides anything. */}
        {!joining && !local && <details className="field-advanced">
          <summary>Advanced: connect to a different server</summary>
          <label><span>Server address</span><input aria-label="Server address" value={request.serverOrigin} maxLength={200} onChange={(event) => setRequest({ ...request, serverOrigin: event.target.value })} placeholder={defaultServer || "https://api.overgent.com"} autoComplete="off" spellCheck={false} /></label>
          <p className="field-note">Leave this empty to use {defaultServer || "Overgent Cloud"}. Your own deployment is described in docs/self-hosting.md.</p>
        </details>}
      </div>

      <ScreenSection title="Connect coding agents" help="Sessions are observed after the agent restarts in this repository. Overgent marks observation ready only once a real session event arrives.">
        <AgentOptions adapters={adapters} request={request} onChange={setRequest} />
      </ScreenSection>

      <ScreenSection title={local ? "What this Project records" : "What this Project shares"}>
        {/* The section heading already names this, so the block does not say it
            twice - one statement, then the exact boundary one disclosure away. */}
        <div className="privacy-disclosure">
          <p>{local
            ? "Session activity and repository file paths stay in a database on this Mac — never your source, diffs, prompts, or credentials, and nothing over the network."
            : "Shares session titles, activity, and repository file paths with Project members — never your source, diffs, prompts, or credentials."}</p>
          <details className="field-advanced"><summary>Exactly what is and is not shared</summary><p>Shares session presence, the vendor-visible session title as bounded intent, tool category, subagent state, and safe repository-relative paths. Approved titles may be embedded by the Project’s configured semantic provider. Classifier-approved visible session messages may be shared with Project members while unpaused. The raw transcript file, source, diffs, system/developer prompts, command output, .env contents, credentials, and environment values never cross the wire.</p></details>
        </div>
        <p className="field-note">Registers the repository with the Overgent service already running on this Mac, and configures the agents ticked above in it. No second background service is started, and nothing else in your agent settings is changed.</p>
        <div className="screen-actions">
          <button className="pill solid" disabled={pending || !request.repositoryRoot || (joining ? !request.joinCode.trim() : !request.projectLabel.trim())}>{pending ? (joining ? "Joining…" : "Creating…") : (joining ? "Join Project" : "Create Project")}</button>
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
function Handoff({ backLabel, onBack, navigate }: { backLabel: string; onBack: () => void; navigate: (url: string) => void }) {
  const inShell = isDesktopShell;
  const [handedOff, setHandedOff] = useState(false);
  const [stalled, setStalled] = useState(false);
  const handoff = () => {
    setHandedOff(true);
    navigate(desktopHandoffURL());
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
