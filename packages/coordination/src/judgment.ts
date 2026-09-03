// sharedBehaviorTerms lives beside the concept vocabulary it reads, and is
// re-exported here because it is part of the judgment surface.
import { joinTerms, sharedBehaviorTerms, SemanticPolicyError, validateSemanticText } from "./intelligence.js";

export { sharedBehaviorTerms };

/**
 * The judgment layer (ADR-045). Deterministic evidence stays the trigger
 * layer; this module decides what a candidate *means*, how certain that
 * reading is, and where the answer belongs. Every path here works offline: the
 * managed provider improves the wording and the precision of a verdict, it is
 * never required to produce one.
 */
export const JUDGMENT_ENGINE_VERSION = "coordination-judgment/v1";

export type JudgmentRelationship =
  | "contract_drift"
  | "duplicate_behavior"
  | "shared_dependency"
  | "path_overlap"
  | "assumption_conflict"
  | "downstream_impact"
  | "unrelated";

export type JudgmentConfidence = "high" | "medium" | "low";
export type JudgmentSeverity = "low" | "medium" | "high" | "critical";

/**
 * Where a judged finding belongs. `next_turn` reaches the receiving agent at
 * its next turn boundary (ADR-046); `dashboard` is visible but never pushed;
 * `silent` means the candidate is not worth a coordination object at all.
 */
export type JudgmentDelivery = "next_turn" | "dashboard" | "silent";

export type JudgmentVerdict = Readonly<{
  relationship: JudgmentRelationship;
  confidence: JudgmentConfidence;
  severity: JudgmentSeverity;
  explanation: string;
  delivery: JudgmentDelivery;
}>;

/** What a reporting workstream said about verification of its own work. */
export type VerificationState = "passed" | "unverified" | "unknown";

/**
 * One workstream as the judgment layer sees it: bounded, policy-passed text
 * and structured coordination facts. Nothing here is source, diff, or
 * transcript content.
 */
export type JudgmentWorkstreamState = Readonly<{
  id: string;
  title: string;
  summary: string;
  status: string;
  /** True once this workstream has published a checkpoint, not just an intent. */
  reportedChange: boolean;
  verification: VerificationState;
  role?: "changed" | "read" | "peer";
  /**
   * The Git branch this workstream is working on, when one was observed.
   *
   * A branch name is coordination metadata, never repository content. It is
   * absent for a detached HEAD, for a workstream with no agent session, and
   * whenever the worktree read failed - and absence is treated as unknown
   * rather than as agreement.
   */
  branch?: string;
}>;

/**
 * How the branches of a candidate's workstreams relate.
 *
 * `divergent` is the expensive case and the reason branch is read at all.
 * Work on one branch is invisible to work on another until someone merges, so
 * nothing outside Overgent will report the overlap in the meantime. `shared`
 * means Git itself will surface the same overlap shortly, at the next pull,
 * push, or write into a shared worktree. `unknown` covers a missing branch on
 * either side and must behave exactly like no evidence at all.
 */
export type BranchRelation = "shared" | "divergent" | "unknown";

export function branchRelation(workstreams: readonly JudgmentWorkstreamState[]): BranchRelation {
  const branches = workstreams.map((workstream) => workstream.branch?.trim()).filter((branch): branch is string => Boolean(branch));
  if (branches.length < 2 || branches.length !== workstreams.length) return "unknown";
  return new Set(branches).size === 1 ? "shared" : "divergent";
}

/**
 * Whether a workstream describes work it intends to keep.
 *
 * A spike, a throwaway experiment, or a scratch reproduction collides with
 * other work in exactly the way a real change does, and reporting it with the
 * same weight spends the interrupt budget on code nobody will merge. This
 * reads the workstream's own words rather than asking for a new switch,
 * because a member who has already written "throwaway spike" in their intent
 * has said the useful thing once.
 *
 * Only an explicit statement counts. Silence stays `standard`, so an
 * unlabelled workstream is never quietly downgraded.
 */
export type WorkIntentClass = "exploratory" | "standard";

const EXPLORATORY_LANGUAGE = /\b(?:spike|throwaway|throw[- ]away|scratch|sandbox|experiment(?:al|ing)?|proof[- ]of[- ]concept|poc|prototyp(?:e|ing)|just (?:testing|trying)|dry[- ]run)\b/i;

export function readWorkIntentClass(text: string): WorkIntentClass {
  return EXPLORATORY_LANGUAGE.test(text) ? "exploratory" : "standard";
}

export type JudgmentSignalKind = "path" | "symbol" | "contract" | "dependency" | "semantic" | "assumption";

export type JudgmentCandidate = Readonly<{
  /** The deterministic finding kind that produced this candidate. */
  kind: string;
  severity: JudgmentSeverity;
  confidence: JudgmentConfidence;
  reason: string;
  signalKind: JudgmentSignalKind;
  sharedSignals: readonly string[];
  workstreams: readonly JudgmentWorkstreamState[];
  /**
   * Symbol names the contract-fingerprint engine already tracks exactly in
   * this scope. A coarse notice about a symbol on this list repeats work the
   * exact engine does better.
   */
  trackedContractSymbols: readonly string[];
  /** An exact same-path collision needs no model to explain it. */
  structurallyUnambiguous: boolean;
}>;

/**
 * One operation, mirroring `EmbeddingProvider`: given a bounded description of
 * two or more candidate workstream states, return a structured verdict.
 */
export interface JudgmentProvider {
  readonly name: string;
  judge(candidate: JudgmentCandidate, signal: AbortSignal): Promise<JudgmentVerdict>;
}

const RELATIONSHIPS: readonly JudgmentRelationship[] = [
  "contract_drift", "duplicate_behavior", "shared_dependency",
  "path_overlap", "assumption_conflict", "downstream_impact", "unrelated",
];
const CONFIDENCES: readonly JudgmentConfidence[] = ["high", "medium", "low"];
const SEVERITIES: readonly JudgmentSeverity[] = ["low", "medium", "high", "critical"];
const DELIVERIES: readonly JudgmentDelivery[] = ["next_turn", "dashboard", "silent"];
const VERDICT_KEYS = ["relationship", "confidence", "severity", "explanation", "delivery"];

export const MAX_EXPLANATION_CHARS = 500;

/**
 * The single place a delivery decision is made. Severity says how much a
 * finding matters; this says where the answer is allowed to go. An unrelated
 * verdict is silent — it never becomes a coordination object at all.
 */
export function decideDelivery(relationship: JudgmentRelationship, severity: JudgmentSeverity): JudgmentDelivery {
  if (relationship === "unrelated") return "silent";
  return severity === "high" || severity === "critical" ? "next_turn" : "dashboard";
}

/** Brief advisory action implied by a delivery decision (ADR-046). */
export function advisoryActionFor(delivery: JudgmentDelivery): "coordination_required" | "review_recommended" {
  return delivery === "next_turn" ? "coordination_required" : "review_recommended";
}

/**
 * Strict, closed parser for a model verdict. Anything unexpected — an extra
 * key, an unknown enum member, prohibited text — throws, and the caller falls
 * back to the deterministic verdict rather than persisting model output it
 * could not validate.
 */
export function parseJudgmentVerdict(value: unknown): JudgmentVerdict {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("judgment_verdict_invalid");
  const record = value as Record<string, unknown>;
  if (VERDICT_KEYS.some((key) => !(key in record))) throw new Error("judgment_verdict_invalid");
  if (Object.keys(record).some((key) => !VERDICT_KEYS.includes(key))) throw new Error("judgment_verdict_invalid");
  if (!RELATIONSHIPS.includes(record.relationship as JudgmentRelationship)) throw new Error("judgment_verdict_invalid");
  if (!CONFIDENCES.includes(record.confidence as JudgmentConfidence)) throw new Error("judgment_verdict_invalid");
  if (!SEVERITIES.includes(record.severity as JudgmentSeverity)) throw new Error("judgment_verdict_invalid");
  if (!DELIVERIES.includes(record.delivery as JudgmentDelivery)) throw new Error("judgment_verdict_invalid");
  if (typeof record.explanation !== "string") throw new Error("judgment_verdict_invalid");
  let explanation: string;
  try {
    explanation = validateSemanticText(record.explanation);
  } catch (error) {
    if (error instanceof SemanticPolicyError) throw new Error("judgment_verdict_invalid");
    throw error;
  }
  if (explanation.length > MAX_EXPLANATION_CHARS) throw new Error("judgment_verdict_invalid");
  return {
    relationship: record.relationship as JudgmentRelationship,
    confidence: record.confidence as JudgmentConfidence,
    severity: record.severity as JudgmentSeverity,
    explanation,
    delivery: record.delivery as JudgmentDelivery,
  };
}

const RELATIONSHIP_BY_KIND: Readonly<Record<string, JudgmentRelationship>> = {
  stale_assumption: "contract_drift",
  redundant_work: "duplicate_behavior",
  shared_dependency: "shared_dependency",
  direct_collision: "path_overlap",
  likely_collision: "path_overlap",
  assumption_conflict: "assumption_conflict",
  downstream_impact: "downstream_impact",
};

export function relationshipForKind(kind: string): JudgmentRelationship {
  return RELATIONSHIP_BY_KIND[kind] ?? "unrelated";
}

/**
 * The symbol a coordination signal names: `backend.Refresh` is a claim about
 * `Refresh`. Matching is exact, so a hand-written dependency label such as
 * `session-api` never silences a symbol the contract engine does not track.
 */
export function signalSymbol(signal: string): string {
  const segments = signal.split(/[./]/).filter((segment) => segment.length > 0);
  return segments[segments.length - 1] ?? "";
}

export function contractSignalTracked(signals: readonly string[], trackedSymbols: readonly string[]): boolean {
  const tracked = new Set(trackedSymbols);
  return signals.some((signal) => tracked.has(signalSymbol(signal)));
}

const VERIFICATION_STATES: Readonly<Record<string, VerificationState>> = {
  passed: "passed", not_run: "unverified", running: "unverified",
  failed: "unverified", unknown: "unverified",
};

/**
 * What a bounded checkpoint summary says about its own verification. An
 * explicit state wins; otherwise work-in-progress language counts, and silence
 * stays `unknown` so an unlabeled checkpoint never downgrades a finding.
 */
export function readVerificationState(text: string): VerificationState {
  const declared = /verification state:\s*([a-z_]+)/i.exec(text);
  if (declared) return VERIFICATION_STATES[declared[1]!.toLowerCase()] ?? "unknown";
  if (/\b(work[- ]in[- ]progress|wip|unverified|not yet verified|verification has not run|prototype|draft(?:ed)?)\b/i.test(text)) {
    return "unverified";
  }
  return "unknown";
}

function bounded(value: string): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized.length <= MAX_EXPLANATION_CHARS ? normalized : `${normalized.slice(0, MAX_EXPLANATION_CHARS - 1)}…`;
}

const WIP_SUFFIX = "The workstream that changed it reported an unverified work-in-progress checkpoint, so treat the new shape as provisional rather than settled.";

/**
 * The offline verdict. It is the fallback when no managed provider is
 * configured and the floor the managed provider is allowed to improve on: it
 * alone must be enough to identify the relationship and its severity.
 */
export function deterministicJudgment(candidate: JudgmentCandidate): JudgmentVerdict {
  return applyWorkContext(candidate, baseJudgment(candidate));
}

/**
 * Where the branch and the declared intent of the work are allowed to move a
 * verdict.
 *
 * Branch is evidence, never a switch. Overgent exists because work on separate
 * branches is invisible until merge, so a branch can never be a reason to stay
 * quiet - it can only change how urgent the answer is and how the answer reads.
 * Two rules, both narrow:
 *
 * An overlap across divergent branches is escalated one step, and only from
 * medium to high, because nothing outside this Project will report it before
 * merge time. An overlap on a shared branch keeps its severity and gains a
 * sentence saying Git will surface it too; a shared branch is not safer, it is
 * merely also visible somewhere else.
 *
 * Work its own author called a spike is capped at the dashboard. That is a
 * de-escalation, so it cannot cost routing precision, and it is the honest
 * reading of a member who wrote "throwaway" in their own intent.
 */
function applyWorkContext(candidate: JudgmentCandidate, verdict: JudgmentVerdict): JudgmentVerdict {
  if (verdict.delivery === "silent" || verdict.relationship === "unrelated") return verdict;

  const relation = branchRelation(candidate.workstreams);
  const collisionLike = verdict.relationship === "path_overlap" || verdict.relationship === "contract_drift";
  const peerLike = verdict.relationship === "path_overlap" || verdict.relationship === "duplicate_behavior";

  let severity = verdict.severity;
  const notes: string[] = [];

  if (relation === "divergent") {
    const branches = branchNames(candidate.workstreams);
    notes.push(`This spans ${joinTerms(branches)}, so nothing outside Overgent reports it until those branches meet at merge.`);
    if (collisionLike && severity === "medium") severity = "high";
  } else if (relation === "shared") {
    const [branch] = branchNames(candidate.workstreams);
    notes.push(`Both are working on ${branch ?? "the same branch"}, so Git surfaces this as well at the next pull, push, or shared write.`);
  }

  let delivery = decideDelivery(verdict.relationship, severity);

  const exploratory = candidate.workstreams.filter((workstream) => readWorkIntentClass(`${workstream.title} ${workstream.summary}`) === "exploratory");
  if (peerLike && exploratory.length > 0) {
    notes.push(`${exploratory.length === candidate.workstreams.length ? "Both sides describe" : `${exploratory[0]!.title} describes`} this as exploratory work, so it is kept on the dashboard rather than routed into a turn.`);
    if (delivery === "next_turn") delivery = "dashboard";
  }

  if (notes.length === 0 && severity === verdict.severity && delivery === verdict.delivery) return verdict;
  return { ...verdict, severity, delivery, explanation: bounded([verdict.explanation, ...notes].join(" ")) };
}

/** Branch names as prose, deduplicated and ordered so the sentence is stable. */
function branchNames(workstreams: readonly JudgmentWorkstreamState[]): string[] {
  return [...new Set(workstreams.map((workstream) => workstream.branch?.trim()).filter((branch): branch is string => Boolean(branch)))].sort();
}

function baseJudgment(candidate: JudgmentCandidate): JudgmentVerdict {
  const relationship = relationshipForKind(candidate.kind);
  // Re-judging reads back an explanation this function may already have
  // written, so the work-in-progress qualifier is stripped before it can be
  // appended a second time — and so a later verified checkpoint removes it.
  const reason = candidate.reason.endsWith(WIP_SUFFIX)
    ? candidate.reason.slice(0, -WIP_SUFFIX.length).trim()
    : candidate.reason;
  if (relationship === "shared_dependency" && (candidate.signalKind === "contract" || candidate.signalKind === "symbol")) {
    if (contractSignalTracked(candidate.sharedSignals, candidate.trackedContractSymbols)) {
      const symbol = signalSymbol(candidate.sharedSignals[0] ?? "");
      return {
        relationship: "unrelated", confidence: "high", severity: "low",
        explanation: bounded(`Contract fingerprints already report every change to ${symbol} with exact old and new signatures, so a second shared-contract notice adds nothing.`),
        delivery: "silent",
      };
    }
    if (!candidate.workstreams.some((workstream) => workstream.reportedChange)) {
      return {
        relationship: "unrelated", confidence: "medium", severity: "low",
        explanation: bounded(`Only anticipated contract lists overlap on ${candidate.sharedSignals[0] ?? "a shared contract"}; neither workstream has reported work on it yet.`),
        delivery: "silent",
      };
    }
  }
  if (relationship === "contract_drift") {
    const changer = candidate.workstreams.find((workstream) => workstream.role === "changed");
    if (changer?.verification === "unverified") {
      const severity: JudgmentSeverity = "medium";
      return {
        relationship, confidence: "medium", severity,
        explanation: bounded(`${reason} ${WIP_SUFFIX}`),
        delivery: decideDelivery(relationship, severity),
      };
    }
  }
  if (relationship === "duplicate_behavior") {
    const [left, right] = candidate.workstreams;
    const terms = left && right ? sharedBehaviorTerms(left.summary, right.summary) : [];
    if (terms.length > 0) {
      return {
        relationship, confidence: candidate.confidence, severity: candidate.severity,
        explanation: bounded(`${reason} Both describe ${joinTerms(terms)} work, so one of them is probably redundant; compare intended outcomes before either lands.`),
        delivery: decideDelivery(relationship, candidate.severity),
      };
    }
  }
  return {
    relationship, confidence: candidate.confidence, severity: candidate.severity,
    explanation: bounded(reason),
    delivery: decideDelivery(relationship, candidate.severity),
  };
}

/**
 * True when a candidate is worth spending a managed adjudication on. An exact
 * same-path collision explains itself, and a verdict that is already silent
 * has nothing for a model to improve.
 */
export function needsManagedAdjudication(candidate: JudgmentCandidate, verdict: JudgmentVerdict): boolean {
  if (candidate.structurallyUnambiguous) return false;
  if (verdict.delivery === "silent") return false;
  return candidate.workstreams.length >= 2;
}
