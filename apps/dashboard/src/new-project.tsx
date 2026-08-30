import { useEffect, useRef, useState, type FormEvent } from "react";
import { Screen, ScreenSection } from "./screen";
import type { EnrollmentRequest, NativeOnboarding } from "./native";

/**
 * Adding a Project is a screen, in both places it can be reached: the hosted
 * workroom sidebar and the desktop shell. It is deliberately the same component
 * and the same layout in both, because the hosted half cannot finish the job -
 * enrollment registers a repository with the local service, and only the desktop
 * shell can reach it. Handing over between the two should read as one screen
 * continuing, not as the app relaunching itself.
 */
export function NewProjectScreen({ api, displayName, navigate, backLabel, onBack }: {
  api: NativeOnboarding;
  displayName: string;
  navigate: (url: string) => void;
  backLabel: string;
  onBack: () => void;
}) {
  const nameRef = useRef<HTMLInputElement>(null);
  const [request, setRequest] = useState<EnrollmentRequest>({ repositoryRoot: "", projectLabel: "", deviceLabel: "This Mac", displayName, joinCode: "", enableCodex: false, enableClaude: false, enableCursor: false });
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
      .then((state) => { if (cancelled) return; setBridge("ready"); setRequest((current) => ({ ...current, deviceLabel: state.deviceLabel || current.deviceLabel })); })
      .catch(() => { if (!cancelled) setBridge("unavailable"); });
    return () => { cancelled = true; };
  }, [api]);
  useEffect(() => { if (bridge === "ready" && !created) nameRef.current?.focus(); }, [bridge, created]);

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

  if (bridge === "probing") {
    return <Screen title="Add a Project" backLabel={backLabel} onBack={onBack} lede="Checking whether this window can reach the Stickguy service on your Mac.">
      <div className="screen-actions" role="status"><span className="spinner" aria-hidden="true" /><span className="settings-help">Checking this Mac…</span></div>
    </Screen>;
  }

  if (bridge === "unavailable") return <ContinueInDesktop backLabel={backLabel} onBack={onBack} />;

  if (created) {
    return <Screen
      title="Project created"
      sub={request.repositoryRoot}
      backLabel={backLabel}
      onBack={onBack}
      lede={`${request.projectLabel} is registered with this Mac’s existing Stickguy service. No second background service was started. It coordinates the agent sessions you run in this repository straight away, with no one else in it.`}
    >
      {/* An invite is an option, not the next step. A Project with one member
          already does the whole job for the sessions that member is running,
          and presenting a code as the thing to do next implied otherwise. */}
      {created.joinCode && <ScreenSection title="Invite someone, when you want to" help="Optional. Nothing here waits on a second member. The code expires in 10 minutes; share it privately.">
        <div className="invite-code"><strong>One-use invite code</strong><code>{created.joinCode}</code></div>
      </ScreenSection>}
      <ScreenSection title="Open it" help="Opening a Project mints a fresh one-time session for it, so the next step asks you to confirm before the shared view loads.">
        <div className="screen-actions">
          <button className="pill solid" disabled={pending} onClick={() => void open()}>{pending ? "Opening…" : "Open Project"}</button>
          <button className="pill" onClick={onBack}>Not now</button>
        </div>
        {created.warnings.map((warning) => <p className="form-warning" key={warning}>{warning}</p>)}
        {error && <p className="form-error" role="alert">{error}</p>}
      </ScreenSection>
    </Screen>;
  }

  return <Screen
    title="Add a Project"
    backLabel={backLabel}
    onBack={onBack}
    lede="Choose a Git repository. Stickguy observes it as a separate Project, using the service already running on this Mac, and coordinates every agent session in it — yours alone, or a team's."
  >
    <form onSubmit={(event) => void submit(event)}>
      <div className="screen-form">
        <label><span>Project name</span><input ref={nameRef} value={request.projectLabel} maxLength={120} onChange={(event) => setRequest({ ...request, projectLabel: event.target.value })} placeholder="Atlas launch" /></label>
        <label><span>Repository</span><div className="repository-picker"><input readOnly value={request.repositoryRoot} placeholder="Choose a local Git repository" /><button type="button" className="pill" onClick={() => void chooseRepository()}>Choose…</button></div></label>
      </div>

      <ScreenSection title="Connect coding agents" help="Sessions are observed after the agent restarts in this repository. Stickguy marks observation ready only once a real session event arrives.">
        <fieldset className="agent-options">
          <label><input type="checkbox" checked={request.enableCodex} onChange={(event) => setRequest({ ...request, enableCodex: event.target.checked })} /><span><strong>Codex</strong><small>Observe new repository-scoped sessions after restart</small></span></label>
          <label><input type="checkbox" checked={request.enableClaude} onChange={(event) => setRequest({ ...request, enableClaude: event.target.checked })} /><span><strong>Claude Code</strong><small>Observe new repository-scoped sessions after restart</small></span></label>
          <label><input type="checkbox" checked={request.enableCursor} onChange={(event) => setRequest({ ...request, enableCursor: event.target.checked })} /><span><strong>Cursor</strong><small>Observe new repository-scoped sessions after restart</small></span></label>
        </fieldset>
      </ScreenSection>

      <ScreenSection title="What this Project shares" help="Classifier-passing coordination facts are visible to enrolled members while sharing is unpaused. Credentials, environment values, raw source, diffs, and command output do not cross the wire.">
        <div className="screen-actions">
          <button className="pill solid" disabled={pending || !request.projectLabel.trim() || !request.repositoryRoot}>{pending ? "Creating…" : "Create Project"}</button>
          <button className="pill" type="button" onClick={onBack}>Cancel</button>
        </div>
        {error && <p className="form-error" role="alert">{error}</p>}
      </ScreenSection>
    </form>
  </Screen>;
}

/**
 * This window is served from the hosted origin, which cannot reach the local
 * service that registers a repository, so the flow continues in the desktop
 * shell through the registered scheme.
 *
 * The control says what happens next rather than "Open Stickguy": a member who
 * is already looking at Stickguy reads that as the app relaunching itself, which
 * is the one thing this hand-off must not look like. The desktop shell lands on
 * the same screen this component renders, so the origin swap reads as the task
 * continuing. The shell command stays as a fallback for a machine where the
 * scheme is unregistered, because a deep link that silently does nothing is
 * worse than a dead end.
 */
function ContinueInDesktop({ backLabel, onBack }: { backLabel: string; onBack: () => void }) {
  const [handedOff, setHandedOff] = useState(false);
  const continueInApp = () => {
    setHandedOff(true);
    window.location.href = `${import.meta.env.DEV ? "stickguy-dev" : "stickguy"}://new-project`;
  };
  return <Screen
    title="Add a Project"
    backLabel={backLabel}
    onBack={onBack}
    lede="A Project registers a Git repository with the Stickguy service on your Mac, so it is created where that service runs. This window is the shared Project view, and it cannot reach that service."
  >
    <ScreenSection title="Continue on this Mac" help="You land on this same screen in the Stickguy app, with the repository picker and agent options supplied by the local service.">
      <div className="screen-actions">
        <button className="pill solid" onClick={continueInApp}>Continue in the Stickguy app</button>
        <button className="pill" onClick={onBack}>Cancel</button>
      </div>
      {handedOff && <p className="handoff-fallback">If the app did not come forward, add the Project from its Projects screen, or run <code>stickguy create</code> in the repository.</p>}
    </ScreenSection>
  </Screen>;
}
