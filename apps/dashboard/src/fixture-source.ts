import { snapshotForProject } from "./fixtures";
import type { FindingFeedback, FindingState, LocalSessionDetail, MemberNameSource, ProjectAccess, ProjectMember, ProjectSnapshot, Resolution, SessionMessagesSnapshot, SyncCard, SyncComment } from "./model";

export class FixtureProjectSource {
  readonly live: boolean = false;
  private snapshots = new Map<string, ProjectSnapshot>();
  private listeners = new Map<string, Set<() => void>>();
  private sessionMessages = new Map<string, SessionMessagesSnapshot>();
  private identity: { name: string; source: MemberNameSource } = { name: "Fixture device", source: "device" };
  private access = new Map<string, ProjectAccess>();
  protected localSessions = new Map<string, LocalSessionDetail>([["wrk_agent_fixture_codex", {
    available: true, title: "Rotate the browser session boundary", branch: "feature/session-rotation",
    messages: [
      { kind: "user", text: "Rotate the browser session on every permission change, but keep existing sessions valid until they expire.", at: "2026-08-25T09:58:00Z" },
      { kind: "thinking", text: "The rotation boundary lives in session.ts. I should check whether existing sessions are keyed by permission version before changing anything.", at: "2026-08-25T09:58:04Z" },
      { kind: "assistant", text: "I'll start in apps/dashboard/src/session.ts and keep the existing expiry path untouched.", at: "2026-08-25T09:58:06Z" },
      { kind: "tool", tool: "Read", at: "2026-08-25T09:58:07Z" },
      { kind: "tool", tool: "apply_patch", at: "2026-08-25T09:59:10Z" },
      { kind: "assistant", text: "Rotation now happens on permission change and existing sessions still expire naturally.", at: "2026-08-25T09:59:40Z" },
    ],
  }], ["wrk_agent_fixture_claude", {
    available: false, messages: [],
  }]]);

  constructor(initial: ProjectSnapshot[] = []) {
    for (const snapshot of initial) this.snapshots.set(snapshot.project.id, structuredClone(snapshot));
  }

  get(projectId: string): ProjectSnapshot {
    const current = this.snapshots.get(projectId);
    if (current) return current;
    const snapshot = snapshotForProject(projectId);
    this.snapshots.set(projectId, snapshot);
    return snapshot;
  }

  subscribe(projectId: string, listener: () => void): () => void {
    const listeners = this.listeners.get(projectId) ?? new Set();
    listeners.add(listener);
    this.listeners.set(projectId, listeners);
    return () => listeners.delete(listener);
  }

  togglePause(projectId: string): void {
    this.update(projectId, (snapshot) => ({ ...snapshot, workspacePaused: !snapshot.workspacePaused }));
  }

  setFindingState(projectId: string, findingId: string, state: FindingState): void {
    this.update(projectId, (snapshot) => ({
      ...snapshot,
      findings: snapshot.findings.map((finding) => finding.id === findingId ? { ...finding, state } : finding),
    }));
  }

  async recordFindingFeedback(_findingId: string, _value: FindingFeedback): Promise<void> {
    return Promise.resolve();
  }





  async createSyncCard(projectId: string, findingId: string | undefined, title: string, summary: string): Promise<void> {
    const card: SyncCard = { id: `syn_fixture_${Date.now()}`, ...(findingId ? { findingId } : {}), title, summary, state: "open", revision: 1, comments: [], updatedAt: new Date().toISOString() };
    this.update(projectId, (current) => ({ ...current, collaboration: { ...current.collaboration, syncCards: [card, ...current.collaboration.syncCards] } }));
  }

  async commentSyncCard(projectId: string, cardId: string, body: string): Promise<void> {
    const comment: SyncComment = { id: `cmt_fixture_${Date.now()}`, memberName: "You", body, createdAt: new Date().toISOString() };
    this.update(projectId, (current) => ({ ...current, collaboration: { ...current.collaboration, syncCards: current.collaboration.syncCards.map((card) => card.id === cardId ? { ...card, revision: card.revision + 1, comments: [...card.comments, comment], updatedAt: comment.createdAt } : card) } }));
  }

  async resolveSyncCard(projectId: string, cardId: string, expectedRevision: number, summary: string, affectedWorkstreamIds: string[]): Promise<void> {
    const resolution: Resolution = { id: `res_fixture_${Date.now()}`, syncCardId: cardId, summary, affectedMemberIds: [], affectedWorkstreamIds, revision: 1, createdAt: new Date().toISOString() };
    this.update(projectId, (current) => ({ ...current, collaboration: { ...current.collaboration, resolutions: [resolution, ...current.collaboration.resolutions], syncCards: current.collaboration.syncCards.map((card) => {
      if (card.id !== cardId) return card;
      if (card.revision !== expectedRevision) throw new Error("revision_conflict");
      return { ...card, state: "resolved", revision: card.revision + 1, resolution, updatedAt: resolution.createdAt };
    }) } }));
  }

  /** Own-session content. The fixture source has none for other members' sessions. */
  async getLocalSession(workstreamId: string): Promise<LocalSessionDetail> {
    return structuredClone(this.localSessions.get(workstreamId) ?? { available: false, messages: [] });
  }

  async getSessionMessages(workstreamId: string): Promise<SessionMessagesSnapshot> {
    return structuredClone(this.sessionMessages.get(workstreamId) ?? { workstreamId, messages: [] });
  }

  async listMembers(_projectId: string): Promise<ProjectMember[]> {
    return [{ id: "mem_fixture", name: this.identity.name, nameSource: this.identity.source, role: "owner", isSelf: true, joinedAt: new Date().toISOString() }];
  }

  async getProjectAccess(projectId: string): Promise<ProjectAccess> {
    const existing = this.access.get(projectId);
    if (existing) return structuredClone(existing);
    const snapshot = this.get(projectId);
    const value: ProjectAccess = {
      role: "owner",
      members: [{ id: "mem_fixture", name: this.identity.name, nameSource: this.identity.source, role: "owner", isSelf: true, joinedAt: new Date().toISOString() }, { id: "mem_teammate", name: "Fixture teammate", nameSource: "member", role: "member", isSelf: false, joinedAt: new Date().toISOString() }],
      devices: snapshot.devices.map((device, index) => ({ id: device.id, memberId: index === 0 ? "mem_fixture" : "mem_teammate", label: device.label, appVersion: device.platform, isCurrent: index === 0, revoked: false })),
      invites: [],
    };
    this.access.set(projectId, value);
    return structuredClone(value);
  }

  async createInvite(projectId: string): Promise<{ code: string }> {
    const access = await this.getProjectAccess(projectId);
    const id = `inv_fixture_${Date.now()}`;
    access.invites.unshift({ id, expiresAt: new Date(Date.now() + 600_000).toISOString(), remainingUses: 1, revoked: false, createdAt: new Date().toISOString() });
    this.access.set(projectId, access);
    return { code: `${id}.fixture-secret` };
  }

  async revokeInvite(projectId: string, inviteId: string): Promise<void> {
    const access = await this.getProjectAccess(projectId);
    access.invites = access.invites.map((invite) => invite.id === inviteId ? { ...invite, revoked: true, remainingUses: 0 } : invite);
    this.access.set(projectId, access);
  }

  async revokeDevice(projectId: string, deviceId: string): Promise<void> {
    const access = await this.getProjectAccess(projectId);
    access.devices = access.devices.map((device) => device.id === deviceId ? { ...device, revoked: true } : device);
    this.access.set(projectId, access);
  }

  async removeMember(projectId: string, memberId: string): Promise<void> {
    const access = await this.getProjectAccess(projectId);
    access.members = access.members.filter((member) => member.id !== memberId);
    this.access.set(projectId, access);
  }

  exportURL(projectId: string): string { return `data:application/json,${encodeURIComponent(JSON.stringify({ schemaVersion: 1, projectId }))}`; }
  async deleteProject(_projectId: string): Promise<void> { return Promise.resolve(); }
  async deleteOwnProjectData(_projectId: string): Promise<void> { return Promise.resolve(); }

  async updateDisplayName(_projectId: string, displayName: string): Promise<{ memberName: string; memberNameSource: MemberNameSource }> {
    const name = displayName.trim().replace(/\s+/g, " ");
    if (name.length < 2 || name.length > 60) throw new Error("Choose a name between 2 and 60 characters.");
    if (name.includes("@")) throw new Error("Choose a display name; an email address cannot be your Project identity.");
    this.identity = { name, source: "member" };
    return { memberName: name, memberNameSource: "member" };
  }

  publishSyntheticUpdate(projectId: string): void {
    this.update(projectId, (snapshot) => ({
      ...snapshot,
      contextRevision: snapshot.contextRevision + 1,
      synchronizedAt: "just now",
      workstreams: snapshot.workstreams.map((workstream, index) => index === 0
        ? { ...workstream, pathCount: workstream.pathCount + 1, updatedLabel: "Now" }
        : workstream),
      activity: [{
        id: `act_fixture_${snapshot.contextRevision + 1}`,
        at: "Now",
        actor: "Fixture device",
        kind: "manifest",
        summary: "Published one new path-only manifest revision.",
        fidelity: "git",
      }, ...snapshot.activity],
    }));
  }

  replace(snapshot: ProjectSnapshot): void {
    this.snapshots.set(snapshot.project.id, structuredClone(snapshot));
    for (const listener of this.listeners.get(snapshot.project.id) ?? []) listener();
  }

  private update(projectId: string, updater: (snapshot: ProjectSnapshot) => ProjectSnapshot): void {
    this.snapshots.set(projectId, updater(this.get(projectId)));
    for (const listener of this.listeners.get(projectId) ?? []) listener();
  }
}
