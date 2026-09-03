import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import { ChevronRight, FileCode2, Laptop2, Moon, Plus, ShieldCheck, Sun, UserPlus } from "lucide-react";
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

/**
 * Settings is a screen, not a dialog. Identity, appearance, devices, privacy,
 * export and the destructive Project actions all live here; People links out to
 * its own screen rather than carrying a second copy of the same controls.
 *
 * Deleting or leaving calls `onRemoved`, so the shell drops the Project and
 * moves to the next one. Queuing the request and leaving the member inside a
 * Project they no longer belong to is the failure mode this exists to prevent.
 */
export function SettingsScreen({ snapshot, dark, identity, projectId, source, offline, backLabel, onBack, onIdentity, onTheme, onPeople, onRemoved }: {
  snapshot: ProjectSnapshot;
  dark: boolean;
  identity: { name: string; source: MemberNameSource };
  projectId: string;
  source: FixtureProjectSource;
  offline: boolean;
  backLabel: string;
  onBack: () => void;
  onIdentity: (value: { name: string; source: MemberNameSource }) => void;
  onTheme: () => void;
  onPeople: () => void;
  onRemoved: () => void;
}) {
  const [nameDraft, setNameDraft] = useState(identity.source === "member" ? identity.name : "");
  const [identityError, setIdentityError] = useState("");
  const [identitySaved, setIdentitySaved] = useState(false);
  const [identityPending, setIdentityPending] = useState(false);
  const [access, setAccess] = useState<ProjectAccess | null>(null);
  const [adminError, setAdminError] = useState("");
  const [adminPending, setAdminPending] = useState(false);
  const [deleteDraft, setDeleteDraft] = useState("");
  const [deletionQueued, setDeletionQueued] = useState(false);
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

  return <Screen title="Settings" sub={snapshot.project.name} backLabel={backLabel} onBack={onBack} lede="How you appear to teammates, how this app looks, which devices can reach this Project, and what leaves your machine.">
    <ScreenSection title="Your identity">
      <form className="screen-form" onSubmit={(event) => {
        event.preventDefault();
        const value = nameDraft.trim();
        setIdentityError(""); setIdentitySaved(false); setIdentityPending(true);
        void source.updateDisplayName(projectId, value)
          .then((result) => { onIdentity({ name: result.memberName, source: result.memberNameSource }); setNameDraft(result.memberName); setIdentitySaved(true); })
          .catch((error: Error & { status?: number }) => setIdentityError(error.status === 400 ? "Choose a display name; an email address cannot be your Project identity." : error.message || "That display name could not be saved."))
          .finally(() => setIdentityPending(false));
      }}>
        <label><span>Display name</span><input value={nameDraft} onChange={(event) => { setNameDraft(event.target.value); setIdentitySaved(false); }} minLength={2} maxLength={60} placeholder={identity.source === "device" ? identity.name : "How teammates see you"} aria-describedby="identity-help" /></label>
        <p id="identity-help" className="settings-help">This is how you appear on live sessions and collision resolutions. It is not your email address or your device name.</p>
        {identity.source === "device" && <p className="settings-help warning">Currently showing the device name this Project was created with.</p>}
        {identityError && <p className="form-error" role="alert">{identityError}</p>}
        {identitySaved && <p className="settings-help success" role="status">Display name updated across this Project.</p>}
        <button className="pill solid" disabled={identityPending || offline || nameDraft.trim().length < 2}>Save name</button>
      </form>
    </ScreenSection>

    <ScreenSection title="Appearance">
      <div className="screen-rows">
        <button className="settings-row" onClick={onTheme}><span className="settings-icon">{dark ? <Moon size={16} /> : <Sun size={16} />}</span><span><strong>Theme</strong><small>{dark ? "Dark" : "Light"}</small></span><ChevronRight size={15} /></button>
      </div>
    </ScreenSection>

    <ScreenSection title="People">
      <div className="screen-rows">
        <button className="settings-row" onClick={onPeople}><span className="settings-icon"><UserPlus size={16} /></span><span><strong>Members &amp; invites</strong><small>See who is in this Project and invite a teammate</small></span><ChevronRight size={15} /></button>
      </div>
    </ScreenSection>

    <ScreenSection title="Devices & security" help="Device names identify hardware for revocation and audit only; they are never shown as your live-work identity. Revoking a device immediately ends its Project access.">
      <div className="screen-rows">{devices}</div>
    </ScreenSection>

    <ScreenSection title="Privacy & data">
      <div className="privacy-card"><ShieldCheck size={17} /><div><strong>Local-first analysis, bounded Project sharing</strong><p>Raw source, diffs, environment values, credentials, and command output never cross the wire. Project members can see classifier-approved coordination facts and session context while sharing is unpaused.</p></div></div>
      {access && <div className="screen-rows"><a className="settings-row" href={source.exportURL(projectId)} download><span className="settings-icon"><FileCode2 size={16} /></span><span><strong>Export retained {access.role === "owner" ? "Project" : "personal"} data</strong><small>Versioned JSON containing the structured records you are authorized to export.</small></span><ChevronRight size={15} /></a></div>}
    </ScreenSection>

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

    {adminError && <p className="form-error" role="alert">{adminError}</p>}
  </Screen>;
}

/**
 * Members and invites. This is the only implementation; Settings links here
 * rather than carrying a second copy, because adding a teammate should never
 * require hunting through Settings.
 */
export function PeopleScreen({ projectId, projectName, source, offline, backLabel, onBack }: {
  projectId: string;
  projectName: string;
  source: FixtureProjectSource;
  offline: boolean;
  backLabel: string;
  onBack: () => void;
}) {
  const [access, setAccess] = useState<ProjectAccess | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [inviteLink, setInviteLink] = useState("");
  const [copied, setCopied] = useState(false);
  const refresh = () => source.getProjectAccess(projectId).then(setAccess).catch(() => setError("Project access controls could not be loaded."));
  useEffect(() => { void refresh(); }, [projectId]);
  const run = (operation: () => Promise<void>, message: string) => {
    setPending(true); setError("");
    void operation().then(refresh).catch(() => setError(message)).finally(() => setPending(false));
  };
  const copy = () => {
    setCopied(false);
    void navigator.clipboard?.writeText(inviteLink).then(() => setCopied(true)).catch(() => setError("The invite link could not be copied. Select it and copy manually."));
  };
  const owner = access?.role === "owner";

  return <Screen title="People" sub={projectName} backLabel={backLabel} onBack={onBack} lede="Everyone who can see this Project's coordination facts, and the one-use invite links that let someone in.">
    <ScreenSection title="Invite a teammate" help="An invite is a one-use link that expires in seven days and can be revoked below. Whoever opens it becomes a member and can see classifier-passing coordination facts while sharing is unpaused.">
      <div className="screen-actions">
        {owner
          ? <button className="pill solid" disabled={pending || offline} onClick={() => { setPending(true); setError(""); setCopied(false); void source.createInvite(projectId).then((result) => { setInviteLink(`${window.location.origin}/join#${result.code}`); return refresh(); }).catch(() => setError("A new invite could not be created.")).finally(() => setPending(false)); }}><Plus size={14} />Create invite link</button>
          : <p className="settings-help warning">Only the Project owner can invite people.</p>}
      </div>
      {inviteLink && <div className="invite-code" role="status">
        <strong>Share this link privately</strong>
        <code>{inviteLink}</code>
        <div className="screen-actions">
          <button className="pill" onClick={copy}>{copied ? "Copied" : "Copy link"}</button>
          <span className="settings-help">Shown once. The code after # never reaches server logs; the same string also works with <code>overgent join</code>.</span>
        </div>
      </div>}
    </ScreenSection>

    <ScreenSection title="Members" count={access?.members.length}>
      <div className="screen-rows">
        {access
          ? access.members.map((member) => <div className="settings-row" key={member.id}>
              <span className="avatar small" style={{ "--member-hue": memberHue(member.name) } as CSSProperties}>{initialsFor(member.name)}</span>
              <span><strong>{member.name}{member.isSelf ? " · you" : ""}</strong><small>{member.role}{member.nameSource === "device" ? " · still using a device name" : ""}</small></span>
              {owner && !member.isSelf && <button className="text-button" disabled={pending || offline} onClick={() => run(() => source.removeMember(projectId, member.id), "That member could not be removed.")}>Remove</button>}
            </div>)
          : <p className="settings-help">Loading members…</p>}
      </div>
    </ScreenSection>

    {owner && (access?.invites.length ?? 0) > 0 && <ScreenSection title="Open invites" count={access?.invites.length}>
      <div className="screen-rows">
        {access?.invites.map((invite) => <div className="settings-row" key={invite.id}>
          <span className="settings-icon"><UserPlus size={16} /></span>
          <span><strong>{invite.id}</strong><small>{invite.revoked ? "Revoked" : `${invite.remainingUses} use remaining · expires ${new Date(invite.expiresAt).toLocaleString()}`}</small></span>
          {!invite.revoked && <button className="text-button" disabled={pending || offline} onClick={() => run(() => source.revokeInvite(projectId, invite.id), "That invite could not be revoked.")}>Revoke</button>}
        </div>)}
      </div>
    </ScreenSection>}

    {error && <p className="form-error" role="alert">{error}</p>}
  </Screen>;
}
