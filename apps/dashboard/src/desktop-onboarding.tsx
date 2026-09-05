import { useEffect, useRef, useState } from "react";
// The agent checkboxes are the same control in both places a Project is set
// up, and they were drifting apart: first run pre-ticked what it detected and
// the add-Project form did not, so the second Project a member made was the
// one that observed nothing.
import { AgentOptions, NewProjectScreen } from "./new-project";
import { nativeOnboarding, type AdapterState, type EnrollmentRequest, type NativeOnboarding, type OnboardingState } from "./native";
import type { AgentVendor } from "./model";
import { DesktopAISettings } from "./desktop-ai-settings";

/** First run in the order the member can answer: what this is, where, then what to connect. */
const FIRST_RUN_STEPS = ["welcome", "repository", "agents"] as const;
type FirstRunStep = (typeof FIRST_RUN_STEPS)[number];

const emptyRequest: EnrollmentRequest = { repositoryRoot: "", projectLabel: "", deviceLabel: "This Mac", displayName: "", joinCode: "", serverOrigin: "", enableCodex: false, enableClaude: false, enableCursor: false };

/** The adapter row each vendor owns, so a reconnect targets the right one. */
const ADAPTER_NAMES: Readonly<Record<AgentVendor, string>> = { codex: "Codex", claude: "Claude Code", cursor: "Cursor" };
const VENDOR_FOR_ADAPTER: Readonly<Record<string, AgentVendor>> = { Codex: "codex", "Claude Code": "claude", Cursor: "cursor" };

export function DesktopOnboarding({ api = nativeOnboarding, navigate = (url) => window.location.assign(url) }: { api?: NativeOnboarding; navigate?: (url: string) => void }) {
  const [state, setState] = useState<OnboardingState | null>(null);
  const [request, setRequest] = useState(emptyRequest);
  const [mode, setMode] = useState<"create" | "join">("create");
  // Where this Project's coordination data lives. Until Lane 06 lands a profile
  // holds one kind, so this is asked once, first, and never asked again.
  const [placement, setPlacement] = useState<"local" | "team">("local");
  const [step, setStep] = useState<FirstRunStep>("welcome");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [joinCode, setJoinCode] = useState("");
  const [warnings, setWarnings] = useState<string[]>([]);
  const [reconnectTarget, setReconnectTarget] = useState<AgentVendor | null>(null);
  const [confirmingReset, setConfirmingReset] = useState(false);
  // Enrollment finishes on its own screen rather than dropping the member onto
  // the standing home screen. Two different things were being said in one
  // place - "that worked" and "here is everything about this Mac" - and the
  // second buried the first under three buttons and a status list.
  const [justConnected, setJustConnected] = useState(false);
  const [showInvite, setShowInvite] = useState(false);
  // A `overgent://new-project` deep link from the hosted Project view lands
  // here, on the one origin that can reach the local service. Opening the form
  // immediately is the whole point of the handoff; making the member find it
  // again would just move the dead end.
  const [addProject, setAddProject] = useState<"create" | "join" | null>(() => new URLSearchParams(window.location.search).get("add") === "project" ? "create" : null);

  const agentDefaultsApplied = useRef(false);
  const refresh = async () => {
    const next = await api.state();
    setState(next);
    // This screen has already detected which agents are installed, so leaving
    // them unticked made the likeliest way through the form connect nothing:
    // enrollment succeeded, the Project then observed Git alone, and the
    // product read as inert on first run. Detected agents start on. They stay
    // visible, the privacy disclosure sits directly below them, and they can
    // be unticked before connecting. An agent that is not installed is still
    // never configured on the member's behalf.
    //
    // The "once" decision is taken here rather than inside the updater below.
    // React deliberately calls an updater twice in development to catch impure
    // ones, and this one was: the second call saw the ref already set, skipped
    // the defaults, and its result is the one React keeps - so in development
    // the detected agents arrived unticked and the fix above silently did
    // nothing. A member's later choice still survives, because the flags are
    // only written on the first refresh.
    const applyDefaults = !agentDefaultsApplied.current;
    agentDefaultsApplied.current = true;
    const defaults = applyDefaults ? {
      enableCodex: adapterInstalled(next, "Codex"),
      enableClaude: adapterInstalled(next, "Claude Code"),
      enableCursor: adapterInstalled(next, "Cursor"),
    } : {};
    setRequest((current) => ({ ...current, deviceLabel: next.deviceLabel || current.deviceLabel, ...defaults }));
  };
  useEffect(() => { void refresh().catch((cause: Error) => setError(cause.message)); }, []);
  useEffect(() => {
    if (!state?.enrolled || !state.adapters.some((adapter) => adapter.restartRequired)) return;
    const timer = window.setInterval(() => void api.state().then(setState).catch(() => undefined), 2_000);
    return () => window.clearInterval(timer);
  }, [api, state?.enrolled, state?.adapters.map((adapter) => `${adapter.name}:${adapter.restartRequired}`).join("|")]);

  const chooseRepository = async () => {
    setError("");
    try {
      const root = await api.chooseRepository();
      if (root) setRequest((current) => ({ ...current, repositoryRoot: root, projectLabel: current.projectLabel || root.split("/").at(-1) || "My Project" }));
    } catch (cause) { setError((cause as Error).message); }
  };
  const submit = async () => {
    setPending(true); setError(""); setWarnings([]);
    try {
      const result = placement === "local"
        ? await api.createLocalProject(request)
        : mode === "create" ? await api.createProject(request) : await api.joinProject(request);
      setJoinCode(result.joinCode);
      setWarnings(Array.isArray(result.warnings) ? result.warnings : []);
      setJustConnected(true);
      await refresh();
    } catch (cause) { setError((cause as Error).message); }
    finally { setPending(false); }
  };
  const open = async () => {
    if (!state?.projectId) return;
    setPending(true); setError("");
    try { navigate(await api.openLiveProject(state.projectId)); }
    catch (cause) { setError((cause as Error).message); setPending(false); }
  };
  const resetEnrollment = async () => {
    setPending(true); setError("");
    try {
      setState(await api.resetEnrollment());
      setConfirmingReset(false);
    } catch (cause) { setError((cause as Error).message); }
    finally { setPending(false); }
  };
  const reconnect = async () => {
    if (!state || !reconnectTarget) return;
    setPending(true); setError("");
    try {
      await api.reconnectAdapter(state.repositoryRoot, reconnectTarget);
      setReconnectTarget(null);
      await refresh();
    } catch (cause) { setError((cause as Error).message); }
    finally { setPending(false); }
  };
  if (state?.enrolled && addProject) {
    return <NewProjectScreen api={api} displayName="" navigate={navigate} mode={addProject} backLabel={state.repositoryLabel || "Overgent"} onBack={() => setAddProject(null)} />;
  }
  if (!state && !error) return <main className="onboarding-shell"><header><Brand /></header><section className="onboarding-card" role="status"><span className="spinner" /><h1>Checking this Mac…</h1></section></main>;
  if (!state) return <main className="onboarding-shell"><header><Brand /></header><section className="onboarding-card"><p className="form-error" role="alert">{error}</p><button className="pill" onClick={() => { setError(""); void refresh().catch((cause: Error) => setError(cause.message)); }}>Try again</button></section></main>;
  // A credential the hosted API rejects leaves every action failing with a 401
  // and no way out. Offer the recovery here rather than making the member find
  // a terminal. "uncertain" is deliberately excluded: being offline is not
  // being locked out, and erasing a working enrollment is unrecoverable.
  if (state.enrolled && (state.credential === "revoked" || state.credential === "unknown")) {
    const revoked = state.credential === "revoked";
    return <main className="onboarding-shell">
      <header><Brand /><Channel state={state} /></header>
      <section className="onboarding-card">
        <h1>{revoked ? "This Mac’s access was revoked." : "This Mac’s credential is no longer recognised."}</h1>
        <p className="onboarding-lede">{revoked
          ? "A Project owner removed this device. Ask for a new invite to reconnect."
          : "The coordination service has no record of this device. Reconnect to enroll again."}</p>
        <div className="connection-line plain">
          <span aria-hidden="true">✓</span>
          <div>
            <strong>Your repositories and code were not touched</strong>
            <p>Reconnecting clears this Mac’s stored credential and its list of Projects. Nothing else.</p>
          </div>
        </div>
        {confirmingReset
          ? <div className="reconnect-preview" role="group" aria-label="Confirm reconnect">
              <h2>Reconnect this Mac?</h2>
              <dl>
                <div><dt>Removed</dt><dd>This Mac’s stored credential and its Project list</dd></div>
                <div><dt>Kept</dt><dd>Every repository, branch, and file on this Mac</dd></div>
              </dl>
              <div className="onboarding-actions">
                <button className="pill" disabled={pending} onClick={() => setConfirmingReset(false)}>Cancel</button>
                <button className="pill solid" disabled={pending} onClick={() => void resetEnrollment()}>{pending ? "Reconnecting…" : "Reconnect this Mac"}</button>
              </div>
            </div>
          : <div className="onboarding-actions">
              <button className="pill solid" disabled={pending} onClick={() => setConfirmingReset(true)}>Reconnect this Mac</button>
              <button className="pill" disabled={pending} onClick={() => { setError(""); void refresh().catch((cause: Error) => setError(cause.message)); }}>Check again</button>
            </div>}
        {error && <p className="form-error" role="alert">{error}</p>}
      </section>
    </main>;
  }

  // The credential check could not complete. Say so plainly and offer a retry;
  // never present this as a reason to reset.
  if (state.enrolled && state.credential === "uncertain") {
    return <main className="onboarding-shell">
      <header><Brand /><Channel state={state} /></header>
      <section className="onboarding-card">
        <h1>Overgent could not confirm this Mac’s access.</h1>
        <p className="onboarding-lede">Local observation keeps running and nothing has been lost. This is usually a dropped connection, or a service that is still starting.</p>
        <div className="onboarding-actions"><button className="pill solid" disabled={pending} onClick={() => { setError(""); void refresh().catch((cause: Error) => setError(cause.message)); }}>Check again</button></div>
        {error && <p className="form-error" role="alert">{error}</p>}
      </section>
    </main>;
  }

  // Enrollment worked. Say that, and offer the one thing worth doing next.
  //
  // This used to be the standing home screen, reached by the same code path,
  // which meant the moment a member finished setting Overgent up they were
  // handed a status list, three buttons of equal weight, and a note about what
  // this build cannot do yet.
  if (state.enrolled && justConnected) {
    return <main className="onboarding-shell">
      <header><Brand /><Channel state={state} /></header>
      <section className="onboarding-card">
        <h1>{state.repositoryLabel} is connected.</h1>
        <p className="onboarding-lede">Start a session in this repository and it appears here. Agents that were already open need one restart.</p>
        <div className="onboarding-actions">
          <button className="pill solid" disabled={pending} onClick={() => void open()}>{pending ? "Opening…" : "Open Project"}</button>
          {joinCode && !showInvite && <button className="pill" onClick={() => setShowInvite(true)}>Invite a teammate</button>}
          <button className="pill" onClick={() => { setJustConnected(false); setShowInvite(false); }}>Not now</button>
        </div>
        {showInvite && joinCode && <Invite code={joinCode} />}
        {warnings.map((warning) => <p className="form-warning" key={warning}>{warning}</p>)}
        {error && <p className="form-error" role="alert">{error}</p>}
      </section>
    </main>;
  }

  // The standing home screen. Everything a member can do from here is a button
  // they can press, and every screen those buttons lead to comes back.
  if (state.enrolled) {
    const needsSetup = state.adapters.some((adapter) => adapter.installed && (adapter.binding === "not_configured" || adapter.binding === "partial"));
    const attention = state.adapters.filter((adapter) => adapter.binding === "other_profile" || adapter.binding === "drifted" || adapter.hooksNeedReview);
    const target = reconnectTarget ? state.adapters.find((adapter) => adapter.name === ADAPTER_NAMES[reconnectTarget]) : undefined;
    return <main className="onboarding-shell">
      <header><Brand /><Channel state={state} /></header>
      <section className="onboarding-card">
        <h1>{state.repositoryLabel}</h1>
        <p className="repo-path">{state.repositoryRoot}</p>
        <div className="onboarding-actions">
          <button className="pill solid" disabled={pending} onClick={() => void open()}>{pending ? "Opening…" : "Open Project"}</button>
          <button className="pill" disabled={pending} onClick={() => setAddProject("create")}>Add a Project</button>
          {/* An invite has to be acceptable from inside the app. Without this
              the only way to take one was to reset this Mac, which would have
              discarded every Project already on it. */}
          <button className="pill" disabled={pending} onClick={() => setAddProject("join")}>Join a Project</button>
        </div>
        {/* A profile holds local Projects or team Projects, not both, until
            Lane 06 lands the per-Project binding. Saying which one this Mac is
            set up for - once, in a line - is what keeps "Add a Project" from
            reading as an offer to add the other kind. */}
        {state.mode && <p className="field-note">This Mac is set up for {state.mode} Projects. Reset to switch.</p>}
        {state.backend?.present && state.backend.lastError && <p className="form-warning">Backend: {state.backend.lastError}</p>}
        <AdapterList state={state} onReconnect={setReconnectTarget} />
        {api.aiSettings && api.putAISettings && <DesktopAISettings api={{ aiSettings: api.aiSettings, putAISettings: api.putAISettings }} projectId={state.projectId} />}
        {/* Said once, only when it is true, and only about the adapters it is
            true of. The previous version stated the healthy case as a banner on
            every visit, which made the unhealthy case read as more of the same. */}
        {attention.length > 0 && <div className="connection-line needs-attention">
          <span aria-hidden="true">!</span>
          <div>
            <strong>{listed(attention.map((adapter) => adapter.name))} {attention.length === 1 ? "needs" : "need"} attention</strong>
            <p>Follow the action on the row above. Overgent marks an agent ready only after a real session event arrives.</p>
          </div>
        </div>}
        {needsSetup && <button className="pill" disabled={pending} onClick={() => void api.configureAdapters(state.repositoryRoot, needsAdapterSetup(state, "Codex"), needsAdapterSetup(state, "Claude Code"), needsAdapterSetup(state, "Cursor")).then(refresh).catch((cause: Error) => setError(cause.message))}>Connect the remaining agents</button>}
        {target && <div className="reconnect-preview" role="dialog" aria-modal="true" aria-label={`Reconnect ${target.name}`}>
          <h2>Reconnect {target.name} to this Project?</h2>
          <dl>
            <div><dt>Current binding</dt><dd>{target.previousProfile || "Another Overgent profile"}</dd></div>
            <div><dt>New binding</dt><dd>{target.currentProfile || "This Overgent Project"}</dd></div>
          </dl>
          <p>Overgent replaces only its own managed entry and activity hooks. Unrelated agent settings are preserved, and the previous binding is restored if either update fails.</p>
          <div className="onboarding-actions">
            <button className="pill" disabled={pending} onClick={() => setReconnectTarget(null)}>Cancel</button>
            <button className="pill solid" disabled={pending} onClick={() => void reconnect()}>{pending ? "Reconnecting…" : "Reconnect to this Project"}</button>
          </div>
        </div>}
        {error && <p className="form-error" role="alert">{error}</p>}
      </section>
    </main>;
  }

  // First run, as three steps rather than one long form.
  //
  // The order is the order the member can actually answer in: what this is,
  // then which repository, then what to connect and what that shares.
  //
  // Every line on these screens has to earn its place. The version this
  // replaces explained the product twice, annotated all five fields, and put a
  // disclosure, a limitation note and a privacy paragraph on the same screen as
  // the button that does the work - so the two sentences worth reading were the
  // two most likely to be skipped. What is left is the shortest form that still
  // says what is shared before anything is shared.
  const stepIndex = FIRST_RUN_STEPS.indexOf(step);
  const joining = placement === "team" && mode === "join";
  const canContinue = Boolean(request.repositoryRoot) && (joining ? Boolean(request.joinCode.trim()) : Boolean(request.projectLabel.trim()));
  return <main className="onboarding-shell">
    <header><Brand /><Channel state={state} /></header>
    <section className="onboarding-card">
      {step !== "welcome" && <div className="step-track">
        <p className="eyebrow">Step {stepIndex + 1} of {FIRST_RUN_STEPS.length}</p>
        <div className="step-bars" aria-hidden="true">{FIRST_RUN_STEPS.map((name, position) => <span key={name} className={position <= stepIndex ? "on" : ""} />)}</div>
      </div>}

      {step === "welcome" && <>
        <h1>Welcome to Overgent.</h1>
        <ul className="welcome-points">
          <li>Point Overgent at one Git repository.</li>
          <li>Keep running Codex, Claude Code and Cursor exactly as you do now.</li>
          <li>Two sessions heading for the same code hear it from each other, not from a merge conflict.</li>
        </ul>
        {/* The first question is where the coordination data goes, and it is
            asked before anything is set up rather than discovered afterwards.
            "Use on this Mac" is first and is the default because it is the
            answer that requires nothing of the member and shares nothing. */}
        <div className="onboarding-actions">
          <button className="pill solid" disabled={!state.localAvailable} onClick={() => { setPlacement("local"); setMode("create"); setStep("repository"); }}>Use on this Mac</button>
          <button className="pill" onClick={() => { setPlacement("team"); setMode("create"); setStep("repository"); }}>Create or join a team Project</button>
        </div>
        <p className="field-note">Nothing leaves this computer.</p>
        <p className="field-note">A team Project stores coordination data on Overgent Cloud or on a server you name. <a href="https://github.com/khalidM3/overgent/blob/main/docs/security-privacy.md" target="_blank" rel="noreferrer">What is shared</a>.</p>
        {!state.localAvailable && <p className="field-note" role="status">This build does not carry a backend to run on this Mac, so a Project needs a server.</p>}
      </>}

      {step === "repository" && <>
        <h1>{joining ? "Join with your invite." : "Choose a repository."}</h1>
        {placement === "team" && <div className="onboarding-actions">
          <button className={mode === "create" ? "pill solid" : "pill"} onClick={() => setMode("create")}>Create a Project</button>
          <button className={mode === "join" ? "pill solid" : "pill"} onClick={() => setMode("join")}>I have an invite code</button>
        </div>}
        {joining && <label className="field"><span>Invite code</span><input value={request.joinCode} onChange={(event) => setRequest({ ...request, joinCode: event.target.value })} placeholder="inv_abc123.secret" autoComplete="off" /></label>}
        <label className="field"><span>Repository</span><div className="repository-picker"><input readOnly value={request.repositoryRoot} placeholder="Choose a local Git repository" /><button className="pill" onClick={() => void chooseRepository()}>Choose…</button></div></label>
        {!joining && <label className="field"><span>Project name</span><input value={request.projectLabel} maxLength={80} onChange={(event) => setRequest({ ...request, projectLabel: event.target.value })} placeholder="Atlas launch" /></label>}
        <label className="field"><span>Your name</span><input aria-label="Your name" value={request.displayName} maxLength={60} onChange={(event) => setRequest({ ...request, displayName: event.target.value })} placeholder="How teammates see you" autoComplete="off" /></label>
        {/* Self-hosting and Overgent Cloud are the same client against a
            different origin, so the field that chooses between them is one
            text input rather than a mode. It is behind a disclosure because
            almost nobody needs it, and it is here rather than in a settings
            screen because the origin is chosen when the Project is created. */}
        {placement === "team" && <details className="field-advanced">
          <summary>Advanced: connect to a different server</summary>
          <label className="field"><span>Server address</span><input aria-label="Server address" value={request.serverOrigin} maxLength={200} onChange={(event) => setRequest({ ...request, serverOrigin: event.target.value })} placeholder={state.apiBaseUrl || "https://api.overgent.com"} autoComplete="off" spellCheck={false} /></label>
          <p className="field-note">Leave this empty to use {state.apiBaseUrl || "Overgent Cloud"}. Your own deployment is described in docs/self-hosting.md.</p>
        </details>}
        {error && <p className="form-error" role="alert">{error}</p>}
        <div className="onboarding-actions">
          <button className="pill solid" disabled={!canContinue} onClick={() => setStep("agents")}>Continue</button>
          <button className="pill" onClick={() => setStep("welcome")}>Back</button>
        </div>
        {/* A control that cannot be pressed says why, rather than leaving the
            member to guess which field it is waiting on. */}
        {!canContinue && <p className="field-note" role="status">{!request.repositoryRoot
          ? "Choose a repository to continue."
          : joining ? "Paste your invite code to continue." : "Give the Project a name to continue."}</p>}
      </>}

      {step === "agents" && <>
        <h1>Connect your agents.</h1>
        <AgentOptions adapters={state.adapters} request={request} onChange={setRequest} />
        <div className="privacy-disclosure">
          <p>{placement === "local"
            ? "Session activity and repository file paths stay in a database on this Mac — never your source, diffs, prompts, or credentials, and nothing over the network."
            : "Shares session activity and repository file paths with Project members — never your source, diffs, prompts, or credentials."}</p>
          <details className="field-advanced"><summary>Exactly what is and is not shared</summary><p>Shares session presence, the vendor-visible session title as bounded intent, tool category, subagent state, and safe repository-relative paths. Approved titles may be embedded by the Project’s configured semantic provider. Classifier-approved visible session messages may be shared with Project members while unpaused. The raw transcript file, source, diffs, system/developer prompts, command output, .env contents, credentials, and environment values never cross the wire.</p></details>
        </div>
        {error && <p className="form-error" role="alert">{error}</p>}
        <div className="onboarding-actions">
          <button className="pill solid submit-enrollment" disabled={pending || !canContinue} onClick={() => void submit()}>{pending ? "Connecting…" : joining ? "Join and connect" : "Create and connect"}</button>
          <button className="pill" disabled={pending} onClick={() => setStep("repository")}>Back</button>
        </div>
      </>}
    </section>
  </main>;
}

function listed(names: string[]): string {
  if (names.length <= 1) return names[0] ?? "";
  return `${names.slice(0, -1).join(", ")} and ${names.at(-1)}`;
}

/**
 * Whether an adapter row still needs the managed configuration written. An
 * adapter that is drifted or bound to another profile is deliberately excluded:
 * both require an explicit member decision, and repairing them silently is what
 * the reconnect confirmation exists to prevent.
 *
 * A binding an earlier Overgent left behind is not either of those. It is
 * adopted before this screen ever renders, by the repair pass in
 * internal/adapterrepair, so it never reaches a member as a decision at all.
 */
function adapterInstalled(state: OnboardingState, name: string): boolean {
  return state.adapters.some((adapter) => adapter.name === name && adapter.installed);
}

// Only an agent that is actually on this Mac. An agent Overgent cannot find is
// not unfinished setup, so offering to connect it left a permanent button on
// the home screen of everyone who does not use all three - and pressing it
// wrote configuration for something that was not there to read it.
function needsAdapterSetup(state: OnboardingState, name: string): boolean {
  return state.adapters.some((adapter) => adapter.name === name && adapter.installed && (adapter.binding === "not_configured" || adapter.binding === "partial"));
}

function Brand() { return <div className="brand" aria-label="Overgent"><span className="brand-mark" aria-hidden="true">O</span><span>overgent</span></div>; }
function Channel({ state }: { state: OnboardingState }) { return <span>{state.development ? "Development" : "Beta"}</span>; }
function AdapterList({ state, onReconnect }: { state: OnboardingState; onReconnect?: (agent: AgentVendor) => void }) { return <div className="adapter-list" aria-label="Agent connections">{state.adapters.map((adapter) => <div key={adapter.name} className={`adapter-row ${adapter.binding}`}><span className={adapter.runtimeVerified ? "adapter-dot connected" : adapter.hooksNeedReview ? "adapter-dot attention" : adapter.configured ? "adapter-dot pending" : "adapter-dot"} aria-hidden="true" /><span><strong>{adapter.name}</strong><small>{adapter.runtimeVerified ? `Verified · ${adapter.fidelity}` : adapter.detail || (adapter.installed ? "Detected · not connected yet" : "Not detected · Git fallback active")}</small></span>{adapter.reconnectAllowed && onReconnect && <button className="text-button" onClick={() => onReconnect(VENDOR_FOR_ADAPTER[adapter.name] ?? "codex")}>Reconnect to this Project</button>}</div>)}</div>; }
function Invite({ code }: { code: string }) { return <div className="invite-code"><strong>One-use invite code</strong><code>{code}</code><p>Expires in seven days. Share it privately.</p></div>; }
