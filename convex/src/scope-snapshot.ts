import type { SupportedVendor } from "./domain.js";

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

export interface ScopeSnapshot {
  revision: number;
  state: ScopeSnapshotState;
  goal: ScopeSnapshotField;
  now: ScopeSnapshotField;
  done: ScopeSnapshotField;
  waitingOn: ScopeSnapshotField;
  verification: ScopeSnapshotField;
  scope: ScopeSnapshotField;
}

export interface ScopeVerificationFact {
  state: "not_run" | "running" | "passed" | "failed" | "unknown";
  checkKind: string;
  label: string;
  summary: string;
  affectedComponent?: string;
  manifestRevision?: number;
  source: "manual" | "mcp" | "hook";
  observedAt?: string;
}

export interface ScopeSnapshotInput {
  revision: number;
  workstreamStatus: "active" | "idle" | "done" | "blocked";
  agentStatus?: "active" | "waiting" | "idle" | "done" | "error";
  vendor?: SupportedVendor;
  declared?: {
    intendedOutcome?: string;
    approachSummary?: string;
    components?: string[];
    contracts?: string[];
    waitingOn?: string[];
  };
  observed?: {
    currentAction?: string;
    writes?: string[];
    writeCount?: number;
    contractPaths?: string[];
    subagents?: Array<{ agentType: string; status: string }>;
    verification?: ScopeVerificationFact[];
  };
  fallbackDerivedTitle?: string;
}

const unavailable = (text: string): ScopeSnapshotField => ({
  text,
  provenance: "unavailable",
  evidenceQuality: "none",
  facts: [],
});

const declared = (text: string, facts: ScopeSnapshotFact[]): ScopeSnapshotField => ({
  text,
  provenance: "declared",
  evidenceQuality: "high",
  facts,
});

function observed(text: string, vendor: SupportedVendor | undefined, facts: ScopeSnapshotFact[]): ScopeSnapshotField {
  return {
    text,
    provenance: "observed",
    // Codex activity is session-shaped but cannot yet bind MCP declarations
    // and checkpoints to that same identity. The facts remain useful, but
    // presenting them at Claude/Cursor strength would disguise that gap.
    evidenceQuality: vendor === "codex" ? "medium" : "high",
    facts,
  };
}

function clean(values: readonly string[] | undefined): string[] {
  return [...new Set((values ?? []).map((value) => value.trim()).filter(Boolean))].sort((left, right) =>
    left < right ? -1 : left > right ? 1 : 0,
  );
}

function list(values: readonly string[], limit = 3): string {
  const visible = values.slice(0, limit);
  const remaining = values.length - visible.length;
  return `${visible.join(", ")}${remaining > 0 ? `, +${remaining} more` : ""}`;
}

function verificationText(values: readonly ScopeVerificationFact[]): string {
  const stateLabel: Record<ScopeVerificationFact["state"], string> = {
    not_run: "Not run",
    running: "Running",
    passed: "Passed",
    failed: "Failed",
    unknown: "Unknown",
  };
  return values.map((value) => {
    const summary = value.summary.trim();
    return `${stateLabel[value.state]}: ${value.label}${summary ? ` — ${summary}` : ""}`;
  }).join(" · ").slice(0, 2_000);
}

function scopeState(input: ScopeSnapshotInput, verification: readonly ScopeVerificationFact[]): ScopeSnapshotState {
  if (input.workstreamStatus === "done" || input.agentStatus === "done") return "complete";
  if (input.workstreamStatus === "blocked" || input.agentStatus === "waiting" || input.agentStatus === "error" || (input.declared?.waitingOn?.length ?? 0) > 0) return "waiting";
  if (verification.some((item) => item.state === "running")) return "verifying";
  return "implementing";
}

/**
 * Build the pull-only workstream projection from approved canonical facts.
 * This function deliberately has no transcript or message input.
 */
export function deriveScopeSnapshot(input: ScopeSnapshotInput): ScopeSnapshot {
  const components = clean(input.declared?.components);
  const contracts = clean(input.declared?.contracts);
  const waitingOn = clean(input.declared?.waitingOn);
  const writes = clean(input.observed?.writes);
  const contractPaths = clean(input.observed?.contractPaths);
  const subagents = input.observed?.subagents ?? [];
  const completedSubagents = subagents.filter((agent) => agent.status === "done");
  const activeSubagents = subagents.filter((agent) => agent.status !== "done");
  const verification = input.observed?.verification ?? [];

  const intendedOutcome = input.declared?.intendedOutcome?.trim();
  const goal = intendedOutcome
    ? declared(intendedOutcome, ["intent.intendedOutcome"])
    : input.fallbackDerivedTitle
      ? { text: input.fallbackDerivedTitle, provenance: "fallback" as const, evidenceQuality: "low" as const, facts: ["session.derivedTitle" as const] }
      : unavailable("No goal reported.");

  const approach = input.declared?.approachSummary?.trim();
  const currentAction = input.observed?.currentAction?.trim();
  const nowParts = [currentAction, activeSubagents.length > 0 ? `${activeSubagents.length} parallel ${activeSubagents.length === 1 ? "agent" : "agents"} active` : ""].filter(Boolean);
  const now = approach
    ? declared(approach, ["intent.approachSummary"])
    : nowParts.length > 0
      ? observed(nowParts.join(" · "), input.vendor, ["activity.currentAction", ...(activeSubagents.length > 0 ? ["activity.subagents" as const] : [])])
      : unavailable("No current action reported.");

  const doneParts: string[] = [];
  const doneFacts: ScopeSnapshotFact[] = [];
  if (writes.length > 0 || (input.observed?.writeCount ?? 0) > 0) {
    const count = Math.max(writes.length, input.observed?.writeCount ?? 0);
    doneParts.push(writes.length > 0 ? `Writes observed in ${list(writes)}${count > writes.length ? ` (${count} reported paths)` : ""}` : `${count} reported paths changed`);
    doneFacts.push("activity.writes");
  }
  if (contractPaths.length > 0) {
    doneParts.push(`Contract fingerprints reported for ${list(contractPaths)}`);
    doneFacts.push("contract.fingerprints");
  }
  if (completedSubagents.length > 0) {
    doneParts.push(`${completedSubagents.length} parallel ${completedSubagents.length === 1 ? "agent" : "agents"} finished`);
    doneFacts.push("activity.subagents");
  }
  const done = doneParts.length > 0
    ? observed(`${doneParts.join(". ")}.`, input.vendor, doneFacts)
    : unavailable("No completed work reported.");

  const waiting = input.declared?.waitingOn !== undefined
    ? declared(waitingOn.length > 0 ? waitingOn.join(" · ") : "Nothing declared.", ["intent.waitingOn"])
    : (input.agentStatus === "waiting" || input.workstreamStatus === "blocked" || input.agentStatus === "error")
      ? observed(currentAction || "Input is required before work can continue.", input.vendor, ["activity.currentAction"])
      : unavailable("Nothing reported.");

  const verificationField = verification.length > 0
    ? observed(verificationText(verification), input.vendor, ["checkpoint.verification"])
    : unavailable("No verification reported.");

  const declaredScopeFacts: ScopeSnapshotFact[] = [
    ...(components.length > 0 ? ["intent.components" as const] : []),
    ...(contracts.length > 0 ? ["intent.contracts" as const] : []),
  ];
  const declaredScopeParts = [
    components.length > 0 ? `Components: ${list(components, 5)}` : "",
    contracts.length > 0 ? `Contracts: ${list(contracts, 5)}` : "",
  ].filter(Boolean);
  const observedScopeFacts: ScopeSnapshotFact[] = [
    ...(writes.length > 0 || (input.observed?.writeCount ?? 0) > 0 ? ["activity.writes" as const] : []),
    ...(contractPaths.length > 0 ? ["contract.fingerprints" as const] : []),
  ];
  const observedScopeParts = [
    writes.length > 0 ? `Paths: ${list(writes, 5)}` : (input.observed?.writeCount ?? 0) > 0 ? `${input.observed!.writeCount} reported paths` : "",
    contractPaths.length > 0 ? `Contracts: ${list(contractPaths, 5)}` : "",
  ].filter(Boolean);
  const scope = declaredScopeParts.length > 0
    ? declared(`${declaredScopeParts.join(". ")}.`, declaredScopeFacts)
    : observedScopeParts.length > 0
      ? observed(`${observedScopeParts.join(". ")}.`, input.vendor, observedScopeFacts)
      : unavailable("No scope reported.");

  return {
    revision: Math.max(1, Math.floor(input.revision)),
    state: scopeState(input, verification),
    goal,
    now,
    done,
    waitingOn: waiting,
    verification: verificationField,
    scope,
  };
}
