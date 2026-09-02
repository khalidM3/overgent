export type ShellState =
  | "activation"
  | "loading"
  | "ready"
  | "empty"
  | "offline"
  | "unauthorized"
  | "version_mismatch";

export type Presence = "online" | "idle" | "offline" | "paused";
/**
 * Coding-agent vendors with a local adapter (ADR-039). Kept as one name so a new
 * vendor cannot be added to some surfaces and forgotten on others.
 */
export type AgentVendor = "codex" | "claude" | "cursor";
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
export type ScopeSnapshotState = "implementing" | "verifying" | "waiting" | "complete";
export type ScopeSnapshotProvenance = "declared" | "observed" | "fallback" | "unavailable";
export type ScopeSnapshotEvidenceQuality = "high" | "medium" | "low" | "none";
export type ScopeSnapshotFact =
  | "intent.intendedOutcome"
  | "intent.approachSummary"
  | "intent.components"
  | "intent.contracts"
  | "intent.waitingOn"
  | "activity.currentAction"
  | "activity.writes"
  | "activity.subagents"
  | "contract.fingerprints"
  | "checkpoint.verification"
  | "session.derivedTitle";
export interface ScopeSnapshotField {
  text: string;
  provenance: ScopeSnapshotProvenance;
  evidenceQuality: ScopeSnapshotEvidenceQuality;
  facts: ScopeSnapshotFact[];
}
export interface ScopeGoalRecord {
  title: string;
  intendedOutcome?: string;
  endedAt: string;
}
export interface ScopeSnapshot {
  revision: number;
  state: ScopeSnapshotState;
  goal: ScopeSnapshotField;
  now: ScopeSnapshotField;
  done: ScopeSnapshotField;
  waitingOn: ScopeSnapshotField;
  verification: ScopeSnapshotField;
  scope: ScopeSnapshotField;
  /** Goals this session pursued and moved on from, oldest first. */
  priorGoals: ScopeGoalRecord[];
  /** How many earlier goals were dropped to keep that list bounded. */
  priorGoalsDropped: number;
}

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
  /** ISO ground truth behind updatedLabel, where the service sends it. */
  updatedAt?: string;
  pathCount: number;
  paths: string[];
  scopeSnapshot: ScopeSnapshot;
  /** Components this session declared it is working in. */
  components?: string[];
  /** Contracts this session declared it is changing or consuming. */
  contracts?: string[];
  agent?: {
    vendor: AgentVendor;
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

/** One changed declaration inside a drifted contract: the exact divergence. */
export interface ContractSymbolChange { name: string; oldSignature?: string; newSignature?: string }
/** The structured object behind a contract-drift finding (ADR-048). */
export interface ContractEvidence {
  path?: string;
  changedSymbols?: ContractSymbolChange[];
  changedByWorkstreamId?: string;
  readAt?: string;
  changedAt?: string;
}
export interface FindingEvidence {
  kind: "path" | "contract" | "dependency" | "intent";
  label: string;
  source: "git" | "mcp" | "manual" | "hook" | "semantic_candidate";
  /** The one thing this evidence is about, when the finding names one. */
  subject?: string;
  contract?: ContractEvidence;
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
    | "stale_assumption"
    | "dependency_ready";
  severity: Severity;
  confidence: "deterministic" | "high" | "medium" | "low";
  state: FindingState;
  title: string;
  reason: string;
  workstreamIds: string[];
  evidence: FindingEvidence[];
  firstSeen: string;
  lastSeen: string;
  /** ISO ground truth behind the prose labels, where the service sends it. */
  firstSeenAt?: string;
  lastSeenAt?: string;
  /**
   * Where the judgment layer routed this finding (ADR-045/046): next_turn is
   * interrupt-worthy, dashboard is visible without being pushed. Absent on
   * records that predate a judged verdict; severity is then the fallback.
   */
  delivery?: "next_turn" | "dashboard";
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
/** Delivery of one decision into one session's turn, from decisionDeliveries. */
export interface ResolutionDelivery { workstreamId: string; deliveredAt: string; acknowledgedAt?: string }
export interface Resolution {
  id: string; syncCardId?: string; summary: string; affectedMemberIds: string[]; affectedWorkstreamIds: string[]; revision: number; createdAt: string;
  /** Per-session delivery state; a session absent here is still queued. */
  deliveries?: ResolutionDelivery[];
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
  messages: Array<{ id: string; kind: SessionMessageKind; text: string; vendor: AgentVendor; capturedAt: string; expiresAt: string }>;
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
