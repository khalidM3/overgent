import { useEffect, useRef, useState } from "react";
import { NewProjectScreen } from "./new-project";
import { nativeOnboarding, type EnrollmentRequest, type NativeOnboarding, type OnboardingState } from "./native";
import type { AgentVendor } from "./model";

const emptyRequest: EnrollmentRequest = { repositoryRoot: "", projectLabel: "", deviceLabel: "This Mac", displayName: "", joinCode: "", enableCodex: false, enableClaude: false, enableCursor: false };

/** The adapter row each vendor owns, so a reconnect targets the right one. */
const ADAPTER_NAMES: Readonly<Record<AgentVendor, string>> = { codex: "Codex", claude: "Claude Code", cursor: "Cursor" };
const VENDOR_FOR_ADAPTER: Readonly<Record<string, AgentVendor>> = { Codex: "codex", "Claude Code": "claude", Cursor: "cursor" };

export function DesktopOnboarding({ api = nativeOnboarding, navigate = (url) => window.location.assign(url) }: { api?: NativeOnboarding; navigate?: (url: string) => void }) {
  const [state, setState] = useState<OnboardingState | null>(null);
  const [request, setRequest] = useState(emptyRequest);
  const [mode, setMode] = useState<"create" | "join">("create");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [joinCode, setJoinCode] = useState("");
  const [warnings, setWarnings] = useState<string[]>([]);
  const [reconnectTarget, setReconnectTarget] = useState<AgentVendor | null>(null);
  const [confirmingReset, setConfirmingReset] = useState(false);
  // A `overgent://new-project` deep link from the hosted Project view lands
  // here, on the one origin that can reach the local service. Opening the form
  // immediately is the whole point of the handoff; making the member find it
  // again would just move the dead end.
  const [addProject, setAddProject] = useState(() => new URLSearchParams(window.location.search).get("add") === "project");

  const agentDefaultsApplied = useRef(false);
  const refresh = async () => {
    const next = await api.state();
    setState(next);
    setRequest((current) => {
      const merged = { ...current, deviceLabel: next.deviceLabel || current.deviceLabel };
      // This screen has already detected which agents are installed, so leaving
      // them unticked made the likeliest way through the form connect nothing:
      // enrollment succeeded, the Project then observed Git alone, and the
      // product read as inert on first run. Detected agents start on. They stay
      // visible, the privacy disclosure sits directly below them, and they can
      // be unticked before connecting. An agent that is not installed is still
      // never configured on the member's behalf.
      if (!agentDefaultsApplied.current) {
        agentDefaultsApplied.current = true;
        merged.enableCodex = adapterInstalled(next, "Codex");
        merged.enableClaude = adapterInstalled(next, "Claude Code");
        merged.enableCursor = adapterInstalled(next, "Cursor");
      }
      return merged;
    });
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
      const result = mode === "create" ? await api.createProject(request) : await api.joinProject(request);
      setJoinCode(result.joinCode);
      setWarnings(Array.isArray(result.warnings) ? result.warnings : []);
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
    return <NewProjectScreen api={api} displayName="" navigate={navigate} backLabel={state.repositoryLabel || "Overgent"} onBack={() => setAddProject(false)} />;
  }
  if (!state && !error) return <main className="onboarding-shell"><header><Brand /><span>Desktop beta</span></header><section className="onboarding-card" role="status"><span className="spinner" /><h1>Checking this Mac…</h1></section></main>;
  if (!state) return <main className="onboarding-shell"><header><Brand /></header><section className="onboarding-card"><p className="form-error" role="alert">{error}</p><button className="pill" onClick={() => { setError(""); void refresh().catch((cause: Error) => setError(cause.message)); }}>Try again</button></section></main>;
  // A credential the hosted API rejects leaves every action failing with a 401
  // and no way out. Offer the recovery here rather than making the member find
  // a terminal. "uncertain" is deliberately excluded: being offline is not
  // being locked out, and erasing a working enrollment is unrecoverable.
  if (state.enrolled && (state.credential === "revoked" || state.credential === "unknown")) {
    const revoked = state.credential === "revoked";
    return <main className="onboarding-shell">
      <header><Brand /><span>{state.development ? "Desktop development" : "Desktop beta"}</span></header>
      <section className="onboarding-card">
        <p className="eyebrow">This Mac is locked out</p>
        <h1>{revoked ? "This Mac’s access was revoked." : "This Mac’s credential is no longer recognised."}</h1>
        <p className="onboarding-lede">{revoked
          ? "A Project owner removed this device from Overgent, so it can no longer read or publish coordination facts. To use Overgent again, ask an owner for a new invite and reconnect this Mac."
          : "The coordination service has no record of this device’s credential. That happens when the service was restored or reset, or when this profile outlived the account it belonged to. Reconnect this Mac to enroll again."}</p>
        <div className="connection-line needs-attention">
          <span aria-hidden="true">!</span>
          <div>
            <strong>Your repositories and code were not touched</strong>
            <p>Overgent never modifies your working tree. Reconnecting clears only this Mac’s stored credential and its list of Projects.</p>
          </div>
        </div>
        {confirmingReset
          ? <div className="reconnect-preview" role="group" aria-label="Confirm reconnect">
              <p className="eyebrow">Confirm</p>
              <h2>Reconnect this Mac?</h2>
              <dl>
                <div><dt>Removed</dt><dd>This Mac’s stored credential and its Project list</dd></div>
                <div><dt>Kept</dt><dd>Every repository, branch, and file on this Mac</dd></div>
                <div><dt>Next</dt><dd>{revoked ? "Join with a new invite from a Project owner" : "Create a Project, or join one with an invite"}</dd></div>
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
        <p className="milestone-note">Device ID and Project bindings live only on this Mac. Nothing is deleted from the Project for other members.</p>
      </section>
    </main>;
  }

  // The credential check could not complete. Say so plainly and offer a retry;
  // never present this as a reason to reset.
  if (state.enrolled && state.credential === "uncertain") {
    return <main className="onboarding-shell">
      <header><Brand /><span>{state.development ? "Desktop development" : "Desktop beta"}</span></header>
      <section className="onboarding-card">
        <p className="eyebrow">Cannot reach the coordination service</p>
        <h1>Overgent could not confirm this Mac’s access.</h1>
        <p className="onboarding-lede">Local observation keeps running and nothing has been lost. This is usually a dropped connection or a service that is still starting.</p>
        <div className="onboarding-actions"><button className="pill solid" disabled={pending} onClick={() => { setError(""); void refresh().catch((cause: Error) => setError(cause.message)); }}>Check again</button></div>
        {error && <p className="form-error" role="alert">{error}</p>}
      </section>
    </main>;
  }

  if (state.enrolled) {
    const needsSetup = state.adapters.some((adapter) => adapter.binding === "not_configured" || adapter.binding === "partial");
    const needsAttention = state.adapters.some((adapter) => adapter.binding === "other_profile" || adapter.binding === "drifted" || adapter.restartRequired || adapter.hooksNeedReview);
    const target = reconnectTarget ? state.adapters.find((adapter) => adapter.name === ADAPTER_NAMES[reconnectTarget]) : undefined;
    return <main className="onboarding-shell"><header><Brand /><span>{state.development ? "Desktop development" : "Desktop beta"}</span></header><section className="onboarding-card connected-card"><p className="eyebrow">Connected on this Mac</p><h1>{state.repositoryLabel}</h1><p className="repo-path">{state.repositoryRoot}</p><div className={needsAttention ? "connection-line needs-attention" : "connection-line"}><span aria-hidden="true">{needsAttention ? "!" : "✓"}</span><div><strong>{needsAttention ? "Repository connected · agent setup needs attention" : "Repository and live agent observation are ready"}</strong><p>{needsAttention ? "Follow the action shown for each adapter. Overgent marks observation ready only after a real session event arrives." : "New Codex, Claude Code, and Cursor sessions opened in this repository appear automatically with lifecycle, tool category, subagent, and safe path activity."}</p></div></div><AdapterList state={state} onReconnect={setReconnectTarget} /><div className="onboarding-actions"><button className="pill solid" disabled={pending} onClick={() => void open()}>{pending ? "Opening…" : "Open live Project"}</button><button className="pill" disabled={pending} onClick={() => setAddProject(true)}>Add a Project</button><button className="pill" disabled={pending || !needsSetup} onClick={() => void api.configureAdapters(state.repositoryRoot, needsAdapterSetup(state, "Codex"), needsAdapterSetup(state, "Claude Code"), needsAdapterSetup(state, "Cursor")).then(refresh).catch((cause: Error) => setError(cause.message))}>Configure agent adapters</button></div>{target && <div className="reconnect-preview" role="dialog" aria-modal="true" aria-label={`Reconnect ${target.name}`}><p className="eyebrow">Confirm profile change</p><h2>Reconnect {target.name} to this Project?</h2><dl><div><dt>Current binding</dt><dd>{target.previousProfile || "Another Overgent profile"}</dd></div><div><dt>New binding</dt><dd>{target.currentProfile || "This Overgent Project"}</dd></div></dl><p>Overgent will replace only its recognized managed MCP entry and activity hooks. Unrelated agent settings are preserved. If either update fails, the previous binding is restored.</p><div className="onboarding-actions"><button className="pill" disabled={pending} onClick={() => setReconnectTarget(null)}>Cancel</button><button className="pill solid" disabled={pending} onClick={() => void reconnect()}>{pending ? "Reconnecting…" : "Reconnect to this Project"}</button></div></div>}{joinCode && <Invite code={joinCode} />}{warnings.map((warning) => <p className="form-warning" key={warning}>{warning}</p>)}{error && <p className="form-error" role="alert">{error}</p>}<p className="milestone-note">{state.limitation}</p></section></main>;
  }

  const codex = state.adapters.find((adapter) => adapter.name === "Codex");
  const claude = state.adapters.find((adapter) => adapter.name === "Claude Code");
  const cursor = state.adapters.find((adapter) => adapter.name === "Cursor");
  return <main className="onboarding-shell"><header><Brand /><span>First-run setup</span></header><section className="onboarding-card"><h1>Connect the Project you’re working on.</h1><p className="onboarding-lede">Choose the Git repository once. New Codex, Claude Code, and Cursor sessions in that repository are detected automatically—no per-chat command or separate branch required.</p><div className="mode-switch" role="tablist" aria-label="Enrollment method"><button role="tab" aria-selected={mode === "create"} onClick={() => setMode("create")}>Create Project</button><button role="tab" aria-selected={mode === "join"} onClick={() => setMode("join")}>Join a Project</button></div><label className="field"><span>Repository</span><div className="repository-picker"><input readOnly value={request.repositoryRoot} placeholder="Choose a local Git repository" /><button className="pill" onClick={() => void chooseRepository()}>Choose…</button></div></label>{mode === "create" ? <label className="field"><span>Project name</span><input value={request.projectLabel} maxLength={80} onChange={(event) => setRequest({ ...request, projectLabel: event.target.value })} placeholder="Atlas launch" /></label> : <label className="field"><span>Invite code</span><input value={request.joinCode} onChange={(event) => setRequest({ ...request, joinCode: event.target.value })} placeholder="invite.secret" autoComplete="off" /></label>}<label className="field"><span>Your name</span><input aria-label="Your name" value={request.displayName} maxLength={60} onChange={(event) => setRequest({ ...request, displayName: event.target.value })} placeholder="How teammates see you" autoComplete="off" /><small className="field-note">Shown on your live sessions and collision resolutions. Not your email address.</small></label><details className="field-advanced"><summary>Device name &amp; security</summary><label className="field"><span>Device name</span><input aria-label="Device name" value={request.deviceLabel} maxLength={80} onChange={(event) => setRequest({ ...request, deviceLabel: event.target.value })} /><small className="field-note">Identifies this Mac for revocation and audit only. It is never shown as your identity.</small></label></details><fieldset className="agent-options"><legend>Connect coding agents for this Project</legend><label><input type="checkbox" checked={request.enableCodex} onChange={(event) => setRequest({ ...request, enableCodex: event.target.checked })} /><span><strong>Codex</strong><small>{codex?.installed ? "Detected · live title intent, tools, subagents, safe paths" : "Configure anyway · sessions appear when Codex opens this repo"}</small></span></label><label><input type="checkbox" checked={request.enableClaude} onChange={(event) => setRequest({ ...request, enableClaude: event.target.checked })} /><span><strong>Claude Code</strong><small>{claude?.installed ? "Detected · live title intent, tools, subagents, safe paths" : "Configure anyway · sessions appear when Claude opens this repo"}</small></span></label><label><input type="checkbox" checked={request.enableCursor} onChange={(event) => setRequest({ ...request, enableCursor: event.target.checked })} /><span><strong>Cursor</strong><small>{cursor?.installed ? "Detected · live prompt intent, edits, and observed file reads" : "Configure anyway · sessions appear when Cursor opens this repo"}</small></span></label></fieldset><div className="privacy-disclosure"><strong>Project activity sharing</strong><p>Shares session titles, activity, and repository file paths with Project members — never your source, diffs, prompts, or credentials.</p><details className="field-advanced"><summary>Exactly what is and is not shared</summary><p>Shares session presence, the vendor-visible session title as bounded intent, tool category, subagent state, and safe repository-relative paths. Approved titles may be embedded by the Project’s configured semantic provider. Classifier-approved visible session messages may be shared with Project members while unpaused. The raw transcript file, source, diffs, system/developer prompts, command output, .env contents, credentials, and environment values never cross the wire.</p></details></div>{error && <p className="form-error" role="alert">{error}</p>}<p className="field-note">{mode === "create" ? "Creates" : "Joins"} the Project, starts Overgent’s background service on this Mac, and configures the agents ticked above in this repository. Nothing else in your agent settings is changed.</p><button className="pill solid submit-enrollment" disabled={pending || !request.repositoryRoot || (mode === "create" ? !request.projectLabel : !request.joinCode)} onClick={() => void submit()}>{pending ? "Connecting…" : mode === "create" ? "Create and connect" : "Join and connect"}</button></section></main>;
}

/**
 * Whether an adapter row still needs the managed configuration written. An
 * adapter that is drifted or bound to another profile is deliberately excluded:
 * both require an explicit member decision, and repairing them silently is what
 * the reconnect confirmation exists to prevent.
 */
function adapterInstalled(state: OnboardingState, name: string): boolean {
  return state.adapters.some((adapter) => adapter.name === name && adapter.installed);
}

function needsAdapterSetup(state: OnboardingState, name: string): boolean {
  return state.adapters.some((adapter) => adapter.name === name && (adapter.binding === "not_configured" || adapter.binding === "partial"));
}

function Brand() { return <div className="brand" aria-label="Overgent"><span className="brand-mark" aria-hidden="true">O</span><span>overgent</span></div>; }
function AdapterList({ state, onReconnect }: { state: OnboardingState; onReconnect?: (agent: AgentVendor) => void }) { return <div className="adapter-list" aria-label="Agent connections">{state.adapters.map((adapter) => <div key={adapter.name} className={`adapter-row ${adapter.binding}`}><span className={adapter.runtimeVerified ? "adapter-dot connected" : adapter.hooksNeedReview ? "adapter-dot attention" : adapter.configured ? "adapter-dot pending" : "adapter-dot"} aria-hidden="true" /><span><strong>{adapter.name}</strong><small>{adapter.runtimeVerified ? `Verified · ${adapter.fidelity}` : adapter.detail || (adapter.installed ? "Detected · not connected yet" : "Not detected · Git fallback active")}</small></span>{adapter.reconnectAllowed && onReconnect && <button className="text-button" onClick={() => onReconnect(VENDOR_FOR_ADAPTER[adapter.name] ?? "codex")}>Reconnect to this Project</button>}</div>)}</div>; }
function Invite({ code }: { code: string }) { return <div className="invite-code"><strong>One-use invite code</strong><code>{code}</code><p>Expires in 10 minutes. Share it privately with the next teammate.</p></div>; }
