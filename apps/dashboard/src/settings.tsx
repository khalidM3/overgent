import { useEffect, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { ChevronRight, FileCode2, Laptop2, Moon, Plus, ShieldCheck, Sun, UserPlus } from "lucide-react";
import { isDesktopWebview, nativeOnboarding } from "./native";
import { Screen, ScreenSection } from "./screen";
import type { FixtureProjectSource } from "./fixture-source";
import type { MemberNameSource, ProjectAccess, ProjectSnapshot } from "./model";

export function initialsFor(name: string): string {
  const parts = name.split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  // A single-word name gave a single letter, so every mononym in the Project
  // rendered as one character in a circle and told the reader almost nothing.
  // Two letters from the one word carries far more and still fits the chip.
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase();
  return parts.slice(0, 2).map((part) => part[0]!.toUpperCase()).join("");
}

/**
 * A stable hue for a member, so the same person is the same colour on every
 * surface and in every session.
 *
 * Identity is a different colour system from status (design system rule 2): it
 * lives only in small marks, never in text, so it cannot compete with the one
 * orange sentence. The hues are spread evenly and rendered at a fixed low
 * saturation defined in the stylesheet, which keeps any of them from reading as
 * an alert on either theme.
 */
// The ramp starts past the alert band on purpose. `--alert` sits near hue 17 on
// both themes, and a member whose chip landed at hue 8 read as a warning even
// at this saturation, which is the one thing an identity mark must never do.
const MEMBER_HUES = [52, 96, 142, 186, 210, 248, 284, 322];

export function memberHue(name: string): number {
  let hash = 0;
  for (let index = 0; index < name.length; index += 1) hash = (hash * 31 + name.charCodeAt(index)) >>> 0;
  return MEMBER_HUES[hash % MEMBER_HUES.length]!;
}

const projectTabs = ["project", "people", "intelligence", "data"] as const;
type ProjectTab = typeof projectTabs[number];
const projectTabLabel: Record<ProjectTab, string> = { project: "Project", people: "People", intelligence: "Intelligence", data: "Data" };

/**
 * Everything about one Project, in four tabs.
 *
 * It was one column of seven sections, and the sections were not all about the
 * same thing. Your display name sat at the top of every Project even though
 * nobody is called something different in each one; People was a row that
 * opened another screen to do the thing this screen was already claiming to be
 * about; and the Project itself — where it is on disk, whose server holds it,
 * what you are in it — was the one subject the Project's own settings screen
 * never covered.
 *
 * So: identity moved to App settings, where a preference that is the same
 * everywhere belongs. People is a tab that renders the same sections as the
 * People screen — one implementation, two ways in, which is what that screen
 * always promised. And the first tab answers what this Project actually is.
 *
 * Deleting or leaving still calls `onRemoved`, so the shell drops the Project
 * and moves to the next one rather than leaving the member inside a Project
 * they no longer belong to.
 */
export function SettingsScreen({ snapshot, mac, projectId, source, offline, backLabel, onBack, onRemoved, intelligence, onAppSettings }: {
  intelligence?: ReactNode;
  onAppSettings?: () => void;
  snapshot: ProjectSnapshot;
  /** What this Mac knows about the Project: where its checkout is, and which
   *  backend holds it. Absent in a browser with no desktop bridge. */
  mac?: { repositoryRoot: string; kind: "local" | "team" | ""; apiBaseUrl: string; credential?: string };
  projectId: string;
  source: FixtureProjectSource;
  offline: boolean;
  backLabel: string;
  onBack: () => void;
  onRemoved: () => void;
}) {
  const [tab, setTab] = useState<ProjectTab>("project");
  const [access, setAccess] = useState<ProjectAccess | null>(null);
  const [adminError, setAdminError] = useState("");
  const [adminPending, setAdminPending] = useState(false);
  const [deleteDraft, setDeleteDraft] = useState("");
  const [deletionQueued, setDeletionQueued] = useState(false);
  const [copied, setCopied] = useState(false);
  const refreshAccess = () => source.getProjectAccess(projectId).then(setAccess).catch(() => setAdminError("Project access controls could not be loaded."));
  useEffect(() => { void refreshAccess(); }, [projectId]);
  const admin = (operation: () => Promise<void>) => {
    setAdminPending(true); setAdminError("");
    void operation().then(refreshAccess).catch(() => setAdminError("That security change could not be completed.")).finally(() => setAdminPending(false));
  };
  const devices = access?.devices.map((device) => <div className="settings-row" key={device.id}>
    <span className="settings-icon"><Laptop2 size={16} /></span>
    <span><strong>{device.label}{device.isCurrent ? " · this device" : ""}</strong><small>{device.appVersion} · {device.revoked ? "revoked" : device.lastSeenAt ?? "never seen"}</small></span>
    {!device.revoked && access.role === "owner" && !device.isCurrent && <button className="text-button" disabled={adminPending || offline} onClick={() => admin(() => source.revokeDevice(projectId, device.id))}>Revoke</button>}
  </div>) ?? snapshot.devices.map((device) => <div className="settings-row" key={device.id}>
    <span className="settings-icon"><Laptop2 size={16} /></span>
    <span><strong>{device.label}</strong><small>{device.platform} · {device.status} · {device.lastSeen}</small></span>
  </div>);

  const local = mac?.kind === "local";
  return <Screen title="Settings" sub={snapshot.project.name} backLabel={backLabel} onBack={onBack} lede="What this Project is, who is in it, and what it does with your code.">
    <nav className="settings-tabs" aria-label="Project settings sections">
      {projectTabs.map((name) => <button key={name} className="text-button" aria-current={tab === name ? "page" : undefined} onClick={() => setTab(name)}>{projectTabLabel[name]}</button>)}
    </nav>

    {tab === "project" && <>
      {/* The facts a member goes looking for when something is wrong: which
          checkout this is, and whose machine holds the coordination. The path
          is copyable because the next thing anyone does with it is paste it
          into a terminal. */}
      <ScreenSection title="This Project" help="Where this Project's repository and coordination data live.">
        <div className="fact-list">
          <div className="fact-row"><span className="fact-name">Repository</span><span className="fact-value mono">{snapshot.project.repositoryLabel}</span></div>
          {mac?.repositoryRoot && <div className="fact-row">
            <span className="fact-name">Folder on this Mac</span>
            <span className="fact-value mono path">{mac.repositoryRoot}
              <button className="text-button inline" onClick={() => {
                setCopied(false);
                void navigator.clipboard?.writeText(mac.repositoryRoot).then(() => setCopied(true)).catch(() => setAdminError("The path could not be copied. Select it and copy manually."));
              }}>{copied ? "Copied" : "Copy"}</button>
            </span>
          </div>}
          <div className="fact-row">
            <span className="fact-name">Coordination lives</span>
            <span className="fact-value">{mac ? (local ? "On this Mac" : "On a shared server") : "On this Project’s server"}</span>
          </div>
          {mac && !local && <div className="fact-row"><span className="fact-name">Server</span><span className="fact-value mono">{mac.apiBaseUrl}</span></div>}
          {access && <div className="fact-row"><span className="fact-name">You are</span><span className="fact-value">{access.role === "owner" ? "The owner" : "A member"}</span></div>}
          {mac?.credential && mac.credential !== "ok" && <div className="fact-row"><span className="fact-name">Connection</span><span className="fact-value alerting">Needs attention · reconnect from the Overgent app</span></div>}
        </div>
      </ScreenSection>

      {onAppSettings && <ScreenSection title="This Mac" help="Your name, appearance, coding agents, and the intelligence every new Project starts from.">
        <div className="screen-rows">
          <button className="settings-row" onClick={onAppSettings}><span className="settings-icon"><Laptop2 size={16} /></span><span><strong>App settings</strong><small>Preferences that are the same in every Project</small></span><ChevronRight size={15} /></button>
        </div>
      </ScreenSection>}

      {access?.role === "owner" && <ScreenSection danger title="Delete Project" help="Deletion immediately revokes Project sessions and invites, then removes retained hosted records in bounded batches.">
        <div className="screen-form">
          <label><span>Type {snapshot.project.name} to confirm</span><input value={deleteDraft} onChange={(event) => setDeleteDraft(event.target.value)} /></label>
          <button className="pill" disabled={adminPending || offline || deleteDraft !== snapshot.project.name || deletionQueued} onClick={() => { setAdminPending(true); setAdminError(""); void source.deleteProject(projectId).then(() => { setDeletionQueued(true); onRemoved(); }).catch(() => setAdminError("Project deletion could not be started.")).finally(() => setAdminPending(false)); }}>{deletionQueued ? "Deletion queued" : "Delete Project"}</button>
        </div>
      </ScreenSection>}

      {access?.role === "member" && <ScreenSection danger title="Leave and delete my data" help="This immediately removes your Project access and schedules deletion of your retained work records.">
        <div className="screen-form">
          <label><span>Type {snapshot.project.name} to confirm</span><input value={deleteDraft} onChange={(event) => setDeleteDraft(event.target.value)} /></label>
          <button className="pill" disabled={adminPending || offline || deleteDraft !== snapshot.project.name || deletionQueued} onClick={() => { setAdminPending(true); setAdminError(""); void source.deleteOwnProjectData(projectId).then(() => { setDeletionQueued(true); onRemoved(); }).catch(() => setAdminError("Your data deletion could not be started.")).finally(() => setAdminPending(false)); }}>{deletionQueued ? "Deletion queued" : "Leave and delete my data"}</button>
        </div>
      </ScreenSection>}
    </>}

    {/* The same sections the People screen renders, because they are the same
        component. Reaching members from the toolbar and from Settings was
        always meant to reach one implementation. */}
    {tab === "people" && <>
      <PeopleSections projectId={projectId} source={source} offline={offline} local={local} serverOrigin={mac && !local ? mac.apiBaseUrl : undefined} access={access} onChanged={refreshAccess} onError={setAdminError} />
      <ScreenSection title="Devices & security" help="Device names identify hardware for revocation and audit only; they are never shown as your live-work identity. Revoking a device immediately ends its Project access.">
        <div className="screen-rows">{devices}</div>
      </ScreenSection>
    </>}

    {tab === "intelligence" && (intelligence
      ? <>{intelligence}</>
      : <ScreenSection title="Intelligence"><p className="settings-help">This Project’s providers are configured from the Overgent desktop app.</p></ScreenSection>)}

    {tab === "data" && <>
      <ScreenSection title="Privacy & data">
        <div className="privacy-card"><ShieldCheck size={17} /><div><strong>Local-first analysis, bounded Project sharing</strong><p>Raw source, diffs, environment values, credentials, and command output never cross the wire. Project members can see classifier-approved coordination facts and session context while sharing is unpaused.</p></div></div>
        {access && isDesktopWebview && <button className="settings-row" disabled={adminPending || offline} onClick={() => admin(() => nativeOnboarding.exportProject(projectId))}>Export retained {access.role === "owner" ? "Project" : "personal"} data</button>}
        {access && !isDesktopWebview && <div className="screen-rows"><a className="settings-row" href={source.exportURL(projectId)} download><span className="settings-icon"><FileCode2 size={16} /></span><span><strong>Export retained {access.role === "owner" ? "Project" : "personal"} data</strong><small>Versioned JSON containing the structured records you are authorized to export.</small></span><ChevronRight size={15} /></a></div>}
      </ScreenSection>
    </>}

    {adminError && <p className="form-error" role="alert">{adminError}</p>}
  </Screen>;
}

/**
 * Your name, set once for every Project on this Mac.
 *
 * It used to be the first section of every Project's settings, which asked a
 * question nobody answers differently twice: people are not called one thing in
 * one Project and something else in the next. The name is still stored per
 * Project — that is where a member row lives — so saving it here writes it to
 * every Project this device belongs to and says how many that was. A Project
 * that could not be reached is named rather than silently skipped.
 */
export function IdentitySettings({ identity, projects, source, offline, onIdentity }: {
  identity: { name: string; source: MemberNameSource };
  /** Every Project this device belongs to, so one save covers all of them. */
  projects: readonly { id: string; name: string }[];
  source: FixtureProjectSource;
  offline: boolean;
  onIdentity: (value: { name: string; source: MemberNameSource }) => void;
}) {
  const [draft, setDraft] = useState(identity.source === "member" ? identity.name : "");
  const [error, setError] = useState("");
  const [saved, setSaved] = useState("");
  const [pending, setPending] = useState(false);

  const save = async () => {
    const value = draft.trim();
    setError(""); setSaved(""); setPending(true);
    try {
      const results = await Promise.allSettled(projects.map((project) => source.updateDisplayName(project.id, value)));
      const applied = results.filter((result) => result.status === "fulfilled");
      const first = applied[0] as PromiseFulfilledResult<{ memberName: string; memberNameSource: MemberNameSource }> | undefined;
      if (!first) throw (results[0] as PromiseRejectedResult | undefined)?.reason ?? new Error("That display name could not be saved.");
      onIdentity({ name: first.value.memberName, source: first.value.memberNameSource });
      setDraft(first.value.memberName);
      const failed = projects.filter((_, index) => results[index]!.status === "rejected").map((project) => project.name);
      setSaved(failed.length === 0
        ? `Display name updated across ${applied.length === 1 ? "your Project" : `all ${applied.length} Projects on this Mac`}.`
        : `Updated in ${applied.length}. Not applied to ${failed.join(", ")} — open ${failed.length === 1 ? "it" : "them"} and save again.`);
    } catch (cause) {
      const failure = cause as Error & { status?: number };
      setError(failure.status === 400 ? "Choose a display name; an email address cannot be your Project identity." : failure.message || "That display name could not be saved.");
    } finally { setPending(false); }
  };

  // The chip is what teammates actually see on a session row, so showing it
  // here is the answer to "how will I look", not decoration - and it updates as
  // the field is typed, because the initials and the hue are both derived from
  // the name.
  const shown = draft.trim() || identity.name;
  return <ScreenSection title="Your name" help="How teammates see you on live sessions and decisions. It is not your email address or your device name.">
    <div className="identity-preview">
      <span className="avatar large" style={{ "--member-hue": memberHue(shown) } as CSSProperties}>{initialsFor(shown)}</span>
      <span><strong>{shown}</strong><small>{identity.source === "device" ? "Still the device name this Mac enrolled with" : "The mark teammates see on a session row"}</small></span>
    </div>
    <form className="screen-form" onSubmit={(event) => { event.preventDefault(); void save(); }}>
      <label><span>Display name</span><input value={draft} onChange={(event) => { setDraft(event.target.value); setSaved(""); }} minLength={2} maxLength={60} placeholder={identity.source === "device" ? identity.name : "How teammates see you"} aria-describedby="identity-help" /></label>
      <p id="identity-help" className="settings-help">Saved to every Project on this Mac at once, because it is you rather than a per-Project setting.</p>
      {identity.source === "device" && <p className="settings-help warning">Currently showing the device name this Project was created with.</p>}
      {error && <p className="form-error" role="alert">{error}</p>}
      {saved && <p className="settings-help success" role="status">{saved}</p>}
      <button className="pill solid" disabled={pending || offline || draft.trim().length < 2}>Save name</button>
    </form>
    {projects.length > 0 && <p className="field-note">Applies to {projects.map((project) => project.name).join(", ")}.</p>}
  </ScreenSection>;
}

/**
 * Members and invites, as sections rather than a screen.
 *
 * Both the People screen and the Project settings People tab render this, so
 * "adding a teammate should never require hunting through Settings" and
 * "Settings should not be missing the thing it is about" are both true without
 * two copies of the invite flow existing.
 *
 * The caller owns the access snapshot, because it usually already has one.
 */
export function PeopleSections({ projectId, source, offline, local = false, serverOrigin, access, onChanged, onError }: {
  projectId: string;
  source: FixtureProjectSource;
  offline: boolean;
  local?: boolean;
  serverOrigin?: string;
  access: ProjectAccess | null;
  onChanged: () => Promise<unknown> | void;
  onError: (message: string) => void;
}) {
  const [pending, setPending] = useState(false);
  const [inviteLink, setInviteLink] = useState("");
  const [copied, setCopied] = useState(false);
  // A link is only useful if its origin is one a teammate can open. The
  // dashboard is served from the origin that serves this Project's /v1 - the
  // hosted deployment, a self-hosted one, or, for a Project that lives on this
  // Mac, the app itself on loopback. Only the first two can be shared, so the
  // last one hands over the bare code instead of a link naming 127.0.0.1.
  const inviteOrigin = serverOrigin ?? window.location.origin;
  const shareable = inviteOrigin.startsWith("https://") && !local;
  const owner = access?.role === "owner";
  const run = (operation: () => Promise<void>, message: string) => {
    setPending(true);
    void operation().then(() => onChanged()).catch(() => onError(message)).finally(() => setPending(false));
  };
  const copy = () => {
    setCopied(false);
    void navigator.clipboard?.writeText(inviteLink).then(() => setCopied(true)).catch(() => onError("The invite link could not be copied. Select it and copy manually."));
  };

  return <>
    {local
      ? <ScreenSection title="Private to this Mac" help="This Project’s coordination and history are stored here. A local invite cannot connect another Mac."><p className="settings-help">For remote collaboration, create a shared Project from Open a repository → Collaborate remotely. Automatic transfer of an existing local Project is not available yet; its history and provider keys stay here.</p></ScreenSection>
      : <ScreenSection title="Invite a teammate" help="An invite is a one-use link that expires in seven days and can be revoked below. Whoever opens it becomes a member and can see classifier-passing coordination facts while sharing is unpaused.">
          <div className="screen-actions">
            {owner
              ? <button className="pill solid" disabled={pending || offline} onClick={() => { setPending(true); setCopied(false); void source.createInvite(projectId).then((result) => { setInviteLink(shareable ? `${inviteOrigin}/join#${result.code}` : result.code); return onChanged(); }).catch(() => onError("A new invite could not be created.")).finally(() => setPending(false)); }}><Plus size={14} />Create invite link</button>
              : <p className="settings-help warning">Only the Project owner can invite people.</p>}
          </div>
          {inviteLink && <div className="invite-code" role="status">
            <strong>{shareable ? "Share this link privately" : "Share this code privately"}</strong>
            <code>{inviteLink}</code>
            <div className="screen-actions">
              <button className="pill" onClick={copy}>{copied ? "Copied" : shareable ? "Copy link" : "Copy code"}</button>
              <span className="settings-help">{shareable
                ? <>Shown once. The code after # never reaches server logs; the same string also works with <code>overgent join</code>.</>
                : <>Shown once. This Project is served from this Mac, so there is no address a teammate could open — they need a Project on a server both of you can reach.</>}</span>
            </div>
          </div>}
        </ScreenSection>}

    <ScreenSection title="Members" count={access?.members.length}>
      <div className="screen-rows">
        {access
          ? access.members.map((member) => <div className="settings-row" key={member.id}>
              <span className="avatar small" style={{ "--member-hue": memberHue(member.name) } as CSSProperties}>{initialsFor(member.name)}</span>
              <span><strong>{member.name}{member.isSelf ? " · you" : ""}</strong><small>{member.role === "owner" ? "Owner" : "Member"}{member.nameSource === "device" ? " · still using a device name" : ""}</small></span>
              {owner && !member.isSelf && <button className="text-button" disabled={pending || offline} onClick={() => run(() => source.removeMember(projectId, member.id), "That member could not be removed.")}>Remove</button>}
            </div>)
          : <p className="settings-help">Loading members…</p>}
      </div>
    </ScreenSection>

    {!local && owner && (access?.invites.length ?? 0) > 0 && <ScreenSection title="Open invites" count={access?.invites.length}>
      <div className="screen-rows">
        {access?.invites.map((invite) => <div className="settings-row" key={invite.id}>
          <span className="settings-icon"><UserPlus size={16} /></span>
          <span><strong>{invite.id}</strong><small>{invite.revoked ? "Revoked" : `${invite.remainingUses} use remaining · expires ${new Date(invite.expiresAt).toLocaleString()}`}</small></span>
          {!invite.revoked && <button className="text-button" disabled={pending || offline} onClick={() => run(() => source.revokeInvite(projectId, invite.id), "That invite could not be revoked.")}>Revoke</button>}
        </div>)}
      </div>
    </ScreenSection>}
  </>;
}

/**
 * The People screen: the same sections, reached from the workroom toolbar.
 * Adding a teammate should never require hunting through Settings, which is
 * why this entry point exists at all.
 */
export function PeopleScreen({ projectId, projectName, source, offline, backLabel, onBack, local = false, serverOrigin }: {
  projectId: string;
  projectName: string;
  local?: boolean;
  serverOrigin?: string;
  source: FixtureProjectSource;
  offline: boolean;
  backLabel: string;
  onBack: () => void;
}) {
  const [access, setAccess] = useState<ProjectAccess | null>(null);
  const [error, setError] = useState("");
  const refresh = () => source.getProjectAccess(projectId).then(setAccess).catch(() => setError("Project access controls could not be loaded."));
  useEffect(() => { void refresh(); }, [projectId]);

  return <Screen title="People" sub={projectName} backLabel={backLabel} onBack={onBack} lede="Everyone who can see this Project's coordination facts, and the one-use invite links that let someone in.">
    <PeopleSections projectId={projectId} source={source} offline={offline} local={local} serverOrigin={serverOrigin} access={access} onChanged={refresh} onError={setError} />
    {error && <p className="form-error" role="alert">{error}</p>}
  </Screen>;
}
