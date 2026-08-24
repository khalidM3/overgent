import { useEffect, useState } from "react";
import { nativeOnboarding, type EnrollmentRequest, type NativeOnboarding, type OnboardingState } from "./native";

const emptyRequest: EnrollmentRequest = { repositoryRoot: "", projectLabel: "", deviceLabel: "This Mac", joinCode: "", enableCodex: false, enableClaude: false };

export function DesktopOnboarding({ api = nativeOnboarding, navigate = (url) => window.location.assign(url) }: { api?: NativeOnboarding; navigate?: (url: string) => void }) {
  const [state, setState] = useState<OnboardingState | null>(null);
  const [request, setRequest] = useState(emptyRequest);
  const [mode, setMode] = useState<"create" | "join">("create");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [joinCode, setJoinCode] = useState("");
  const [warnings, setWarnings] = useState<string[]>([]);
  const [worktreeMessage, setWorktreeMessage] = useState("");

  const refresh = async () => {
    const next = await api.state();
    setState(next);
    setRequest((current) => ({ ...current, deviceLabel: next.deviceLabel || current.deviceLabel }));
  };
  useEffect(() => { void refresh().catch((cause: Error) => setError(cause.message)); }, []);

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
      setWarnings(result.warnings);
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
  const assignWorktree = async (agent: "codex" | "claude") => {
    setError(""); setWorktreeMessage("");
    try {
      const root = await api.chooseRepository();
      if (!root) return;
      await api.connectAgentWorktree(root, agent);
      setWorktreeMessage(`${agent === "codex" ? "Codex" : "Claude Code"} is assigned to ${root}. Restart sessions that were already open.`);
      await refresh();
    } catch (cause) { setError((cause as Error).message); }
  };

  if (!state && !error) return <main className="onboarding-shell"><header><Brand /><span>Desktop development</span></header><section className="onboarding-card" role="status"><span className="spinner" /><h1>Checking this Mac…</h1></section></main>;
  if (!state) return <main className="onboarding-shell"><header><Brand /></header><section className="onboarding-card"><p className="form-error" role="alert">{error}</p><button className="secondary-button" onClick={() => { setError(""); void refresh().catch((cause: Error) => setError(cause.message)); }}>Try again</button></section></main>;
  if (state.enrolled) return <main className="onboarding-shell"><header><Brand /><span>Desktop development</span></header><section className="onboarding-card connected-card"><p className="eyebrow">Connected on this Mac</p><h1>{state.repositoryLabel}</h1><p className="repo-path">{state.repositoryRoot}</p><div className="connection-line"><span aria-hidden="true">✓</span><div><strong>Git observation is active</strong><p>Stickguy scopes activity to this repository and shares path/status metadata—never source, diffs, prompts, transcripts, or environment values.</p></div></div><AdapterList state={state} /><div className="onboarding-actions"><button className="primary-button" disabled={pending} onClick={() => void open()}>{pending ? "Opening…" : "Open live Project"}</button><button className="secondary-button" disabled={pending || state.adapters.every((adapter) => adapter.configured)} onClick={() => void api.configureAdapters(state.repositoryRoot, state.adapters.some((adapter) => adapter.name === "Codex" && !adapter.configured), state.adapters.some((adapter) => adapter.name === "Claude Code" && !adapter.configured)).then(refresh).catch((cause: Error) => setError(cause.message))}>Configure agent adapters</button></div><section className="worktree-assign"><p className="eyebrow">Separate local attribution</p><h2>Give each agent its own existing linked worktree.</h2><p>One checkout shows combined repository activity. Distinct linked worktrees let Stickguy prove Codex-versus-Claude overlap without reading file contents or controlling Git.</p><div><button className="secondary-button" onClick={() => void assignWorktree("codex")}>Assign Codex worktree…</button><button className="secondary-button" onClick={() => void assignWorktree("claude")}>Assign Claude worktree…</button></div>{worktreeMessage && <p className="form-success" role="status">{worktreeMessage}</p>}</section>{joinCode && <Invite code={joinCode} />}{warnings.map((warning) => <p className="form-warning" key={warning}>{warning}</p>)}{error && <p className="form-error" role="alert">{error}</p>}<p className="milestone-note">{state.limitation}</p></section></main>;

  const codex = state.adapters.find((adapter) => adapter.name === "Codex");
  const claude = state.adapters.find((adapter) => adapter.name === "Claude Code");
  return <main className="onboarding-shell"><header><Brand /><span>First-run setup</span></header><section className="onboarding-card"><p className="eyebrow">One repository, one shared view</p><h1>Connect the Project you’re working on.</h1><p className="onboarding-lede">Choose the Git repository once. Stickguy observes it automatically; Codex and Claude Code get optional Project-scoped coordination tools without a command in every chat.</p><div className="mode-switch" role="tablist" aria-label="Enrollment method"><button role="tab" aria-selected={mode === "create"} onClick={() => setMode("create")}>Create Project</button><button role="tab" aria-selected={mode === "join"} onClick={() => setMode("join")}>Join a Project</button></div><label className="field"><span>Repository</span><div className="repository-picker"><input readOnly value={request.repositoryRoot} placeholder="Choose a local Git repository" /><button className="secondary-button" onClick={() => void chooseRepository()}>Choose…</button></div></label>{mode === "create" ? <label className="field"><span>Project name</span><input value={request.projectLabel} maxLength={80} onChange={(event) => setRequest({ ...request, projectLabel: event.target.value })} placeholder="Atlas launch" /></label> : <label className="field"><span>Invite code</span><input value={request.joinCode} onChange={(event) => setRequest({ ...request, joinCode: event.target.value })} placeholder="invite.secret" autoComplete="off" /></label>}<label className="field"><span>Device name</span><input value={request.deviceLabel} maxLength={80} onChange={(event) => setRequest({ ...request, deviceLabel: event.target.value })} /></label><fieldset className="agent-options"><legend>Connect coding agents for this Project</legend><label><input type="checkbox" checked={request.enableCodex} onChange={(event) => setRequest({ ...request, enableCodex: event.target.checked })} /><span><strong>Codex</strong><small>{codex?.installed ? "Detected · MCP intent plus Git observation" : "Not detected in this app process · configure anyway or use Git observation only"}</small></span></label><label><input type="checkbox" checked={request.enableClaude} onChange={(event) => setRequest({ ...request, enableClaude: event.target.checked })} /><span><strong>Claude Code</strong><small>{claude?.installed ? "Detected · MCP intent plus Git observation" : "Not detected in this app process · configure anyway or use Git observation only"}</small></span></label></fieldset><div className="privacy-disclosure"><strong>Shared by default</strong><p>Repository identity, workstream intent, changed path/status metadata, presence, and evidence-backed findings. Never file contents, diffs, raw prompts/transcripts, system prompts, secrets, or environment values.</p></div>{error && <p className="form-error" role="alert">{error}</p>}<button className="primary-button submit-enrollment" disabled={pending || !request.repositoryRoot || (mode === "create" ? !request.projectLabel : !request.joinCode)} onClick={() => void submit()}>{pending ? "Connecting…" : mode === "create" ? "Create and connect" : "Join and connect"}</button></section></main>;
}

function Brand() { return <div className="brand" aria-label="Stickguy"><span className="brand-mark" aria-hidden="true">S</span><span>stickguy</span></div>; }
function AdapterList({ state }: { state: OnboardingState }) { return <div className="adapter-list" aria-label="Agent connections">{state.adapters.map((adapter) => <div key={adapter.name}><span className={adapter.configured ? "adapter-dot connected" : "adapter-dot"} aria-hidden="true" /><span><strong>{adapter.name}</strong><small>{adapter.configured ? adapter.fidelity : adapter.installed ? "Detected · not connected yet" : "Not detected · Git fallback active"}</small></span></div>)}</div>; }
function Invite({ code }: { code: string }) { return <div className="invite-code"><strong>One-use invite code</strong><code>{code}</code><p>Expires in 10 minutes. Share it privately with the next teammate.</p></div>; }
