export type ShellState =
  | "activation"
  | "loading"
  | "ready"
  | "empty"
  | "offline"
  | "unauthorized"
  | "version_mismatch";

export type Presence = "online" | "idle" | "offline" | "paused";
export type Fidelity = "mcp" | "git" | "manual" | "hook_unverified";
export type SemanticStatus = "enabled" | "degraded" | "disabled";
export type FindingState = "open" | "acknowledged" | "resolved" | "dismissed";
export type FindingFeedback = "useful" | "not_related" | "already_coordinated" | "missed_severity";
export type Severity = "critical" | "high" | "medium" | "low";

export interface ProjectSummary {
  id: string;
  name: string;
  repositoryLabel: string;
  semanticStatus: SemanticStatus;
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
  largeChange?: {
    pathCount: number;
    summary: string;
    revision: number;
  };
}

export interface FindingEvidence {
  kind: "path" | "contract" | "dependency" | "intent";
  label: string;
  source: "git" | "mcp" | "manual" | "semantic_candidate";
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
  kind: "intent" | "manifest" | "finding" | "checkpoint" | "pause";
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

export interface ProjectSnapshot {
  project: ProjectSummary;
  contextRevision: number;
  synchronizedAt: string;
  workstreams: Workstream[];
  findings: Finding[];
  activity: ActivityItem[];
  devices: Device[];
  workspacePaused: boolean;
}

export interface DashboardSession {
  memberName: string;
  projects: ProjectSummary[];
  selectedProjectId: string;
}
