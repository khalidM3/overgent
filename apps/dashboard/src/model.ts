export type ShellState =
  | "activation"
  | "loading"
  | "ready"
  | "empty"
  | "offline"
  | "unauthorized"
  | "version_mismatch";

export type Presence = "online" | "idle" | "offline" | "paused";
export type Fidelity = "mcp" | "git" | "manual" | "hook" | "hook_unverified";
export type SemanticStatus = "enabled" | "degraded" | "disabled";
export type SemanticMode = "offline_fallback" | "managed_openai" | "managed_degraded";
export interface HarnessCapabilities {
  observeSession: boolean;
  observeToolActivity: boolean;
  observeReadSet: "observed" | "vendor_inferred" | "self_declared" | "none";
  observeSafePaths: boolean;
  readExistingSession: boolean;
  pollUpdates: boolean;
  deliverBrief: "mcp_pull" | "native_pull" | "native_push" | "unavailable";
  requestAttention: "advisory" | "unavailable";
}
export type FindingState = "open" | "acknowledged" | "resolved" | "dismissed";
export type FindingFeedback = "useful" | "not_related" | "already_coordinated" | "missed_severity";
export type Severity = "critical" | "high" | "medium" | "low";

export interface ProjectSummary {
  id: string;
  name: string;
  repositoryLabel: string;
  semanticStatus: SemanticStatus;
  semanticMode: SemanticMode;
}

export interface Workstream {
  id: string;
  memberName: string;
  initials: string;
  title: string;
  outcome: string;
  presence: Presence;
  fidelity: Fidelity;
  updatedLabel: string;
  pathCount: number;
  paths: string[];
  agent?: {
    vendor: "codex" | "claude";
    sessionAlias?: string;
    branch?: string;
    /** What this chat session is actually about, from the vendor's own session record. */
    sessionTitle?: string;
    status?: "active" | "waiting" | "idle" | "done" | "error";
    tool?: string;
    startedAt?: string;
    endedAt?: string;
    capabilities: HarnessCapabilities;
    subagents: Array<{ alias: string; agentType: string; status: string }>;
    activity?: Array<{
      id: string;
      at: string;
      occurredAt?: string;
      kind: string;
      status: "active" | "waiting" | "idle" | "done" | "error";
      action: string;
      tool?: string;
      paths: string[];
    }>;
    coordination: Array<{
      id: string;
      routedAt: string;
      acknowledgedAt?: string;
      summary: string;
      itemCount: number;
      trigger: string;
    }>;
  };
  largeChange?: {
    pathCount: number;
    summary: string;
    revision: number;
  };
}

export interface FindingEvidence {
  kind: "path" | "contract" | "dependency" | "intent";
  label: string;
  source: "git" | "mcp" | "manual" | "hook" | "semantic_candidate";
}

export interface Finding {
  id: string;
  kind:
    | "direct_collision"
    | "likely_collision"
    | "redundant_work"
    | "shared_dependency"
    | "assumption_conflict"
    | "downstream_impact"
    | "stale_assumption";
  severity: Severity;
  confidence: "deterministic" | "high" | "medium" | "low";
  state: FindingState;
  title: string;
  reason: string;
  workstreamIds: string[];
  evidence: FindingEvidence[];
  firstSeen: string;
  lastSeen: string;
}

export interface ActivityItem {
  id: string;
  at: string;
  actor: string;
  kind: "intent" | "manifest" | "finding" | "checkpoint" | "pause" | "agent";
  summary: string;
  fidelity: Fidelity | "structural";
}

export interface Device {
  id: string;
  label: string;
  platform: string;
  status: Presence;
  lastSeen: string;
}
export interface SyncComment { id: string; memberName: string; body: string; createdAt: string }
export interface Resolution {
  id: string; syncCardId?: string; summary: string; affectedMemberIds: string[]; affectedWorkstreamIds: string[]; revision: number; createdAt: string;
}
export interface SyncCard {
  id: string; findingId?: string; title: string; summary: string; state: "open" | "resolved"; revision: number; comments: SyncComment[]; resolution?: Resolution; updatedAt: string;
}
export interface CollaborationSnapshot {
  projectId: string; syncCards: SyncCard[]; resolutions: Resolution[]; cursor: string;
}
export type SessionMessageKind = "user" | "assistant" | "thinking" | "system";
/** One entry of the viewer's own session, read locally and never uploaded. */
export interface LocalSessionMessage { kind: SessionMessageKind | "tool"; text?: string; tool?: string; at?: string }
export interface LocalSessionDetail { available: boolean; title?: string; branch?: string; messages: LocalSessionMessage[] }
export interface SessionMessagesSnapshot {
  workstreamId: string;
  messages: Array<{ id: string; kind: SessionMessageKind; text: string; vendor: "codex" | "claude"; capturedAt: string; expiresAt: string }>;
}

export interface ProjectSnapshot {
  project: ProjectSummary;
  contextRevision: number;
  synchronizedAt: string;
  workstreams: Workstream[];
  findings: Finding[];
  activity: ActivityItem[];
  devices: Device[];
  workspacePaused: boolean;
  collaboration: CollaborationSnapshot;
}

/**
 * The quiet period on one agent session.
 *
 * Focus is the inbound control: it stops coordination reaching this agent's
 * turns and changes nothing about what this device publishes. It is local to
 * the machine running the session, so it is read through the native bridge
 * rather than the hosted snapshot, and it always expires.
 */
export interface SessionFocus { sessionId: string; focused: boolean; until?: string }

export type MemberNameSource = "device" | "member";
export interface DashboardSession {
  memberId: string;
  memberName: string;
  /** "device" means the name is still the enrolling device label and the member has never chosen one. */
  memberNameSource: MemberNameSource;
  projects: ProjectSummary[];
  selectedProjectId: string;
}
export interface ProjectMember {
  id: string; name: string; nameSource: MemberNameSource; role: "owner" | "member"; isSelf: boolean; joinedAt: string;
}
export interface ProjectDeviceAdmin {
  id: string; memberId: string; label: string; appVersion: string; isCurrent: boolean; revoked: boolean; lastSeenAt?: string;
}
export interface ProjectInviteAdmin {
  id: string; expiresAt: string; remainingUses: number; revoked: boolean; createdAt: string;
}
export interface ProjectAccess {
  role: "owner" | "member"; members: ProjectMember[]; devices: ProjectDeviceAdmin[]; invites: ProjectInviteAdmin[];
}
