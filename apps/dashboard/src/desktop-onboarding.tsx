import { useEffect, useState } from "react";
import { nativeOnboarding, type EnrollmentRequest, type NativeOnboarding, type OnboardingState } from "./native";

const emptyRequest: EnrollmentRequest = { repositoryRoot: "", projectLabel: "", deviceLabel: "This Mac", displayName: "", joinCode: "", enableCodex: false, enableClaude: false };

export function DesktopOnboarding({ api = nativeOnboarding, navigate = (url) => window.location.assign(url) }: { api?: NativeOnboarding; navigate?: (url: string) => void }) {
  const [state, setState] = useState<OnboardingState | null>(null);
  const [request, setRequest] = useState(emptyRequest);
  const [mode, setMode] = useState<"create" | "join">("create");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [joinCode, setJoinCode] = useState("");
  const [warnings, setWarnings] = useState<string[]>([]);
  const [reconnectTarget, setReconnectTarget] = useState<"codex" | "claude" | null>(null);

  const refresh = async () => {
    const next = await api.state();
    setState(next);
    setRequest((current) => ({ ...current, deviceLabel: next.deviceLabel || current.deviceLabel }));
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
  if (!state && !error) return <main className="onboarding-shell"><header><Brand /><span>Desktop beta</span></header><section className="onboarding-card" role="status"><span className="spinner" /><h1>Checking this Mac…</h1></section></main>;
  if (!state) return <main className="onboarding-shell"><header><Brand /></header><section className="onboarding-card"><p className="form-error" role="alert">{error}</p><button className="secondary-button" onClick={() => { setError(""); void refresh().catch((cause: Error) => setError(cause.message)); }}>Try again</button></section></main>;
  if (state.enrolled) {
    const needsSetup = state.adapters.some((adapter) => adapter.binding === "not_configured" || adapter.binding === "partial");
    const needsAttention = state.adapters.some((adapter) => adapter.binding === "other_profile" || adapter.binding === "drifted" || adapter.restartRequired);
    const target = reconnectTarget ? state.adapters.find((adapter) => adapter.name === (reconnectTarget === "codex" ? "Codex" : "Claude Code")) : undefined;
    return <main className="onboarding-shell"><header><Brand /><span>{state.development ? "Desktop development" : "Desktop beta"}</span></header><section className="onboarding-card connected-card"><p className="eyebrow">Connected on this Mac</p><h1>{state.repositoryLabel}</h1><p className="repo-path">{state.repositoryRoot}</p><div className={needsAttention ? "connection-line needs-attention" : "connection-line"}><span aria-hidden="true">{needsAttention ? "!" : "✓"}</span><div><strong>{needsAttention ? "Repository connected · agent setup needs attention" : "Repository and live agent observation are ready"}</strong><p>{needsAttention ? "Follow the action shown for each adapter. Stickguy marks observation ready only after a real session event arrives." : "New Codex and Claude Code sessions opened in this repository appear automatically with lifecycle, tool category, subagent, and safe path activity."}</p></div></div><AdapterList state={state} onReconnect={setReconnectTarget} /><div className="onboarding-actions"><button className="primary-button" disabled={pending} onClick={() => void open()}>{pending ? "Opening…" : "Open live Project"}</button><button className="secondary-button" disabled={pending || !needsSetup} onClick={() => void api.configureAdapters(state.repositoryRoot, state.adapters.some((adapter) => adapter.name === "Codex" && (adapter.binding === "not_configured" || adapter.binding === "partial")), state.adapters.some((adapter) => adapter.name === "Claude Code" && (adapter.binding === "not_configured" || adapter.binding === "partial"))).then(refresh).catch((cause: Error) => setError(cause.message))}>Configure agent adapters</button></div>{target && <div className="reconnect-preview" role="dialog" aria-modal="true" aria-label={`Reconnect ${target.name}`}><p className="eyebrow">Confirm profile change</p><h2>Reconnect {target.name} to this Project?</h2><dl><div><dt>Current binding</dt><dd>{target.previousProfile || "Another Stickguy profile"}</dd></div><div><dt>New binding</dt><dd>{target.currentProfile || "This Stickguy Project"}</dd></div></dl><p>Stickguy will replace only its recognized managed MCP entry and activity hooks. Unrelated agent settings are preserved. If either update fails, the previous binding is restored.</p><div className="onboarding-actions"><button className="secondary-button" disabled={pending} onClick={() => setReconnectTarget(null)}>Cancel</button><button className="primary-button" disabled={pending} onClick={() => void reconnect()}>{pending ? "Reconnecting…" : "Reconnect to this Project"}</button></div></div>}{joinCode && <Invite code={joinCode} />}{warnings.map((warning) => <p className="form-warning" key={warning}>{warning}</p>)}{error && <p className="form-error" role="alert">{error}</p>}<p className="milestone-note">{state.limitation}</p></section></main>;
  }

  const codex = state.adapters.find((adapter) => adapter.name === "Codex");
  const claude = state.adapters.find((adapter) => adapter.name === "Claude Code");
  return <main className="onboarding-shell"><header><Brand /><span>First-run setup</span></header><section className="onboarding-card"><p className="eyebrow">One repository, one shared view</p><h1>Connect the Project you’re working on.</h1><p className="onboarding-lede">Choose the Git repository once. New Codex and Claude Code sessions in that repository are detected automatically—no per-chat command or separate branch required.</p><div className="mode-switch" role="tablist" aria-label="Enrollment method"><button role="tab" aria-selected={mode === "create"} onClick={() => setMode("create")}>Create Project</button><button role="tab" aria-selected={mode === "join"} onClick={() => setMode("join")}>Join a Project</button></div><label className="field"><span>Repository</span><div className="repository-picker"><input readOnly value={request.repositoryRoot} placeholder="Choose a local Git repository" /><button className="secondary-button" onClick={() => void chooseRepository()}>Choose…</button></div></label>{mode === "create" ? <label className="field"><span>Project name</span><input value={request.projectLabel} maxLength={80} onChange={(event) => setRequest({ ...request, projectLabel: event.target.value })} placeholder="Atlas launch" /></label> : <label className="field"><span>Invite code</span><input value={request.joinCode} onChange={(event) => setRequest({ ...request, joinCode: event.target.value })} placeholder="invite.secret" autoComplete="off" /></label>}<label className="field"><span>Your name</span><input aria-label="Your name" value={request.displayName} maxLength={60} onChange={(event) => setRequest({ ...request, displayName: event.target.value })} placeholder="How teammates see you" autoComplete="off" /><small className="field-note">Shown on your live sessions and collision resolutions. Not your email address.</small></label><details className="field-advanced"><summary>Device name &amp; security</summary><label className="field"><span>Device name</span><input aria-label="Device name" value={request.deviceLabel} maxLength={80} onChange={(event) => setRequest({ ...request, deviceLabel: event.target.value })} /><small className="field-note">Identifies this Mac for revocation and audit only. It is never shown as your identity.</small></label></details><fieldset className="agent-options"><legend>Connect coding agents for this Project</legend><label><input type="checkbox" checked={request.enableCodex} onChange={(event) => setRequest({ ...request, enableCodex: event.target.checked })} /><span><strong>Codex</strong><small>{codex?.installed ? "Detected · live title intent, tools, subagents, safe paths" : "Configure anyway · sessions appear when Codex opens this repo"}</small></span></label><label><input type="checkbox" checked={request.enableClaude} onChange={(event) => setRequest({ ...request, enableClaude: event.target.checked })} /><span><strong>Claude Code</strong><small>{claude?.installed ? "Detected · live title intent, tools, subagents, safe paths" : "Configure anyway · sessions appear when Claude opens this repo"}</small></span></label></fieldset><div className="privacy-disclosure"><strong>Project activity sharing</strong><p>Shares session presence, the vendor-visible session title as bounded intent, tool category, subagent state, and safe repository-relative paths. Approved titles may be embedded by the Project’s configured semantic provider. Classifier-approved visible session messages may be shared with Project members while unpaused. The raw transcript file, source, diffs, system/developer prompts, command output, .env contents, credentials, and environment values never cross the wire.</p></div>{error && <p className="form-error" role="alert">{error}</p>}<button className="primary-button submit-enrollment" disabled={pending || !request.repositoryRoot || (mode === "create" ? !request.projectLabel : !request.joinCode)} onClick={() => void submit()}>{pending ? "Connecting…" : mode === "create" ? "Create and connect" : "Join and connect"}</button></section></main>;
}

function Brand() { return <div className="brand" aria-label="Stickguy"><span className="brand-mark" aria-hidden="true">S</span><span>stickguy</span></div>; }
function AdapterList({ state, onReconnect }: { state: OnboardingState; onReconnect?: (agent: "codex" | "claude") => void }) { return <div className="adapter-list" aria-label="Agent connections">{state.adapters.map((adapter) => <div key={adapter.name} className={`adapter-row ${adapter.binding}`}><span className={adapter.runtimeVerified ? "adapter-dot connected" : adapter.configured ? "adapter-dot pending" : "adapter-dot"} aria-hidden="true" /><span><strong>{adapter.name}</strong><small>{adapter.runtimeVerified ? `Verified · ${adapter.fidelity}` : adapter.detail || (adapter.installed ? "Detected · not connected yet" : "Not detected · Git fallback active")}</small></span>{adapter.reconnectAllowed && onReconnect && <button className="text-button" onClick={() => onReconnect(adapter.name === "Codex" ? "codex" : "claude")}>Reconnect to this Project</button>}</div>)}</div>; }
function Invite({ code }: { code: string }) { return <div className="invite-code"><strong>One-use invite code</strong><code>{code}</code><p>Expires in 10 minutes. Share it privately with the next teammate.</p></div>; }
