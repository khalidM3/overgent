import type { EmbeddedObject, EmbeddingProvider, Scope, SemanticIndex, SemanticInput, Candidate } from "./index.js";

export const INTELLIGENCE_ENGINE_VERSION = "coordination/v1";
export const ROUTER_VERSION = "brief-router/v1";
// Keep the offline provider index-compatible with the managed provider. This
// is intentionally sparse; it remains an offline fallback, not a claim of
// general semantic understanding.
export const CONCEPT_DIMENSIONS = 1024;

export type WorkstreamRecord = Scope & Readonly<{
  id: string;
  revision: number;
  status: "active" | "idle" | "blocked" | "done";
  summary: string;
  paths?: readonly string[];
  dependencies?: readonly string[];
  schemas?: readonly string[];
  routes?: readonly string[];
  contracts?: readonly string[];
  components?: readonly string[];
  changes?: readonly string[];
  assumptions?: readonly string[];
  pathCount?: number;
  /** Current provider vector for the primary intent object, when available. */
  semanticVector?: readonly number[];
  semanticProvider?: string;
}>;

export type Evidence = Readonly<{
  kind: "path" | "symbol" | "dependency" | "schema" | "route" | "lexical" | "semantic" | "decision";
  summary: string;
  source: string;
  fidelity: string;
  // Contract drift carries the changed declarations alongside the summary so a
  // brief can name what moved (ADR-048). Every other evidence kind omits it.
  contract?: Readonly<{
    path: string;
    changedSymbols: readonly Readonly<{ name: string; oldSignature: string; newSignature: string }>[];
    changedByWorkstreamId: string;
    readAt: string;
    changedAt: string;
  }>;
}>;

export type IntelligenceFinding = Scope & Readonly<{
  id: string;
  kind: "direct_collision" | "likely_collision" | "redundant_work" | "shared_dependency" | "assumption_conflict" | "downstream_impact" | "stale_assumption";
  severity: "low" | "medium" | "high" | "critical";
  confidenceBand: "deterministic" | "high" | "medium" | "low";
  workstreamIds: readonly string[];
  evidence: readonly Evidence[];
  reason: string;
  revision: number;
  priority: number;
}>;

export class SemanticPolicyError extends Error {
  constructor(readonly code: string) { super(code); }
}

export type Adjudication = Readonly<{
  classification: "likely_collision" | "redundant_work" | "assumption_conflict" | "downstream_impact" | "not_related";
  confidence: "high" | "medium" | "low";
  reason: string;
}>;

export interface CandidateAdjudicator {
  readonly name: string;
  adjudicate(left: WorkstreamRecord, right: WorkstreamRecord, signal: AbortSignal): Promise<Adjudication>;
}

export function validateAdjudication(value: unknown): Adjudication {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("adjudication_invalid");
  const record = value as Record<string, unknown>;
  if (Object.keys(record).some((key) => !["classification", "confidence", "reason"].includes(key))) throw new Error("adjudication_invalid");
  const classifications = ["likely_collision", "redundant_work", "assumption_conflict", "downstream_impact", "not_related"];
  if (!classifications.includes(String(record.classification)) || !["high", "medium", "low"].includes(String(record.confidence))) throw new Error("adjudication_invalid");
  const reason = validateSemanticText(String(record.reason ?? ""));
  return { classification: record.classification as Adjudication["classification"], confidence: record.confidence as Adjudication["confidence"], reason };
}

const prohibited = [
  /-----BEGIN [A-Z ]*PRIVATE KEY-----/i,
  /\b(?:api[_-]?key|access[_-]?token|client[_-]?secret|password)\s*[:=]\s*\S+/i,
  /\b(?:sk|ghp|github_pat)_[A-Za-z0-9_-]{16,}\b/,
  /```[\s\S]*```/,
  /\b(?:ignore|override)\b.{0,80}\b(?:previous|system|developer)\b.{0,40}\binstructions\b/i,
  /(?:^|[\s/])\.env(?:\.|$)/i,
];

export function validateSemanticText(text: string): string {
  const normalized = text.normalize("NFC").trim().replace(/\s+/g, " ");
  if (!normalized || normalized.length > 2_000) throw new SemanticPolicyError("semantic_text_size_invalid");
  if (prohibited.some((pattern) => pattern.test(normalized))) throw new SemanticPolicyError("semantic_text_prohibited");
  return normalized;
}

export function validateSemanticTags(tags: readonly string[]): string[] {
  if (tags.length > 256) throw new SemanticPolicyError("semantic_tags_count_invalid");
  const normalized: string[] = [];
  for (const tag of tags) {
    const value = tag.normalize("NFC").trim();
    const protectedPath = value.startsWith("path:") && /(?:^|\/)(?:\.env(?:\.|$)|\.ssh(?:\/|$)|\.aws(?:\/|$)|\.gnupg(?:\/|$)|id_(?:rsa|ed25519)(?:\.|$))/i.test(value.slice(5));
    if (!value || /[\r\n\0]/.test(value) || protectedPath || prohibited.some((pattern) => pattern.test(value))) throw new SemanticPolicyError("semantic_tag_prohibited");
    if ([...value].length <= 80) normalized.push(value);
  }
  return [...new Set(normalized)].sort().slice(0, 32);
}

const concepts: ReadonlyArray<readonly string[]> = [
  ["auth", "authenticate", "authorization", "login", "session", "cookie", "credential", "token", "bearer"],
  ["member", "membership", "role", "privilege", "permission"],
  ["rotate", "revoke", "invalidate", "expiry", "expire"],
  ["schema", "migration", "contract", "interface", "revision"],
  ["dependency", "package", "import", "consumer", "client"],
  ["document", "documentation", "operator", "readme"],
  ["generated", "regenerate", "reformat", "mechanical", "locale", "icon"],
  ["search", "ranking", "query"],
];
const conceptByToken = new Map(concepts.flatMap((group, index) => group.map((word) => [word, `concept:${index}`] as const)));

function tokens(text: string): string[] {
  return [...new Set(text.toLowerCase().match(/[a-z][a-z0-9_-]{2,}/g)?.map((token) => conceptByToken.get(token) ?? token) ?? [])].sort();
}

function hash(value: string): number {
  let result = 2166136261;
  for (let index = 0; index < value.length; index++) result = Math.imul(result ^ value.charCodeAt(index), 16777619);
  return result >>> 0;
}

export function conceptVector(text: string): number[] {
  const vector = Array<number>(CONCEPT_DIMENSIONS).fill(0);
  for (const token of tokens(validateSemanticText(text))) vector[hash(token) % vector.length]! += token.startsWith("concept:") ? 2 : 1;
  const norm = Math.hypot(...vector);
  return norm === 0 ? vector : vector.map((value) => value / norm);
}

export function cosine(left: readonly number[], right: readonly number[]): number {
  if (left.length !== right.length || left.length === 0) return -1;
  return left.reduce((sum, value, index) => sum + value * (right[index] ?? 0), 0);
}

export class DeterministicConceptEmbeddingProvider implements EmbeddingProvider {
  readonly name = "stickguy-concepts/v1";
  async embed(inputs: readonly SemanticInput[], signal: AbortSignal): Promise<readonly EmbeddedObject[]> {
    if (signal.aborted) throw signal.reason;
    return inputs.map((input) => ({ ...input, text: validateSemanticText(input.text), model: this.name, dimensions: CONCEPT_DIMENSIONS, vector: conceptVector(input.text) }));
  }
}

export class MemorySemanticIndex implements SemanticIndex {
  private readonly objects = new Map<string, EmbeddedObject>();
  async activate(objects: readonly EmbeddedObject[], signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw signal.reason;
    for (const object of objects) {
      if (object.vector.length !== object.dimensions || object.dimensions !== CONCEPT_DIMENSIONS) throw new Error("embedding_dimensions_invalid");
      this.objects.set(`${object.projectId}\0${object.repositoryId}\0${object.objectId}`, object);
    }
  }
  async search(scope: Scope, vector: readonly number[], limit: number, signal: AbortSignal): Promise<readonly Candidate[]> {
    if (signal.aborted) throw signal.reason;
    return [...this.objects.values()]
      .filter((object) => object.projectId === scope.projectId && object.repositoryId === scope.repositoryId)
      .map((object) => ({ objectId: object.objectId, revision: object.revision, evidence: "semantic" as const, score: cosine(vector, object.vector) }))
      .sort((a, b) => b.score - a.score || a.objectId.localeCompare(b.objectId)).slice(0, Math.max(0, limit));
  }
  async deleteScope(scope: Scope, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw signal.reason;
    for (const [key, object] of this.objects) if (object.projectId === scope.projectId && object.repositoryId === scope.repositoryId) this.objects.delete(key);
  }
}

export async function retrieveSemanticCandidates(
  provider: EmbeddingProvider | undefined,
  index: SemanticIndex | undefined,
  input: SemanticInput,
  limit: number,
  signal: AbortSignal,
): Promise<{ candidates: readonly Candidate[]; degraded: boolean }> {
  if (!provider || !index) return { candidates: [], degraded: true };
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const [embedded] = await provider.embed([input], signal);
      if (!embedded || embedded.revision !== input.revision || embedded.projectId !== input.projectId || embedded.repositoryId !== input.repositoryId) throw new Error("embedding_scope_invalid");
      return { candidates: await index.search(input, embedded.vector, Math.max(1, Math.min(limit, 64)), signal), degraded: false };
    } catch (error) {
      if (signal.aborted) throw error;
      if (attempt === 1) return { candidates: [], degraded: true };
    }
  }
  return { candidates: [], degraded: true };
}

function overlap(left: readonly string[] = [], right: readonly string[] = []): string[] {
  const rightSet = new Set(right.map((value) => value.toLowerCase()));
  return [...new Set(left.filter((value) => rightSet.has(value.toLowerCase())))].sort();
}

function lexicalScore(left: string, right: string): number {
  const a = new Set(tokens(left)); const b = new Set(tokens(right));
  const shared = [...a].filter((token) => b.has(token)).length;
  return shared / Math.max(1, new Set([...a, ...b]).size);
}

function pairId(scope: Scope, left: string, right: string, kind: string): string {
  const value = `${scope.projectId}\0${scope.repositoryId}\0${[left, right].sort().join("\0")}\0${kind}`;
  return `fnd_${hash(value).toString(16).padStart(8, "0")}`;
}

export function evaluatePair(left: WorkstreamRecord, right: WorkstreamRecord): IntelligenceFinding | null {
  if (left.projectId !== right.projectId || left.repositoryId !== right.repositoryId || left.id === right.id) return null;
  if (left.status === "done" || right.status === "done") return null;
  const scope = { projectId: left.projectId, repositoryId: left.repositoryId };
  const workstreamIds = [left.id, right.id].sort();
  const paths = overlap(left.paths, right.paths);
  const dependencies = overlap(left.dependencies, right.dependencies);
  const schemas = overlap([...(left.schemas ?? []), ...(left.contracts ?? [])], [...(right.schemas ?? []), ...(right.contracts ?? [])]);
  const routes = overlap(left.routes, right.routes);
  const components = overlap(left.components, right.components);
  const downstream = [...overlap(left.changes, [...(right.dependencies ?? []), ...(right.contracts ?? []), ...(right.schemas ?? [])]), ...overlap(right.changes, [...(left.dependencies ?? []), ...(left.contracts ?? []), ...(left.schemas ?? [])])];
  const assumptions = overlap(left.assumptions, right.assumptions);
  const providerCompatible = left.semanticVector !== undefined && right.semanticVector !== undefined &&
    left.semanticProvider !== undefined && left.semanticProvider === right.semanticProvider &&
    left.semanticVector.length === right.semanticVector.length;
  const semantic = providerCompatible
    ? cosine(left.semanticVector!, right.semanticVector!)
    : cosine(conceptVector(left.summary), conceptVector(right.summary));
  const semanticSource = providerCompatible ? left.semanticProvider! : "stickguy-concepts/v1";
  const lexical = lexicalScore(left.summary, right.summary);
  const summaries = `${left.summary} ${right.summary}`.toLowerCase();
  const documentary = /\b(document|documentation|readme)\b/.test(summaries);
  const mechanical = /\b(generated|regenerate|reformat|mechanical)\b/.test(left.summary.toLowerCase()) && /\b(generated|regenerate|reformat|mechanical)\b/.test(right.summary.toLowerCase());
  let kind: IntelligenceFinding["kind"] | null = null;
  let reason = "";
  const evidence: Evidence[] = [];
  if (paths.length) {
    kind = "direct_collision"; reason = `Active workstreams overlap on ${paths[0]}.`;
    evidence.push({ kind: "path", summary: `Both active manifests include ${paths[0]}.`, source: "git", fidelity: "structural" });
  } else if (downstream.length) {
    kind = "downstream_impact"; reason = `A change to ${downstream[0]} affects an active consumer.`;
    evidence.push({ kind: "dependency", summary: `One workstream changes ${downstream[0]} while the other depends on it.`, source: "reported", fidelity: "structural" });
  } else if (dependencies.length || schemas.length || routes.length) {
    kind = "shared_dependency"; const shared = dependencies[0] ?? schemas[0] ?? routes[0]!;
    reason = `Active workstreams share ${shared}; coordinate its revision and consumers.`;
    evidence.push({ kind: dependencies.length ? "dependency" : schemas.length ? "schema" : "route", summary: `Both workstreams report ${shared}.`, source: "reported", fidelity: "structural" });
  } else if ((assumptions.length && /incompatible|conflict|opposite/.test(summaries)) || (/remain valid until expiry/.test(summaries) && /rotate|revoke|invalidate/.test(summaries))) {
    kind = "assumption_conflict"; reason = "The workstreams report incompatible session-validity assumptions.";
    evidence.push({ kind: "semantic", summary: "One intent preserves existing sessions while the other rotates or revokes them.", source: semanticSource, fidelity: "semantic" });
  } else if (components.length && (semantic >= 0.25 || lexical >= 0.05)) {
    kind = "likely_collision"; reason = `Active changes interact within ${components[0]}.`;
    evidence.push({ kind: "lexical", summary: `Both workstreams report the ${components[0]} component with interacting change language.`, source: "coordination/v1", fidelity: "reported" });
  } else if (!documentary && !mechanical && ((providerCompatible && semantic >= 0.86) || (!providerCompatible && semantic >= 0.50 && lexical >= 0.05))) {
    kind = "redundant_work"; reason = providerCompatible
      ? "Active workstreams are strong semantic candidates for duplicate behavior; review their intended outcomes."
      : "Active workstreams appear to implement the same behavior under different paths.";
    evidence.push({ kind: "semantic", summary: providerCompatible
      ? "Approved intent summaries are strongly related under the configured embedding provider; similarity is candidate evidence, not proof."
      : "Bounded intent summaries share a strong behavior concept.", source: semanticSource, fidelity: "semantic" });
  }
  if (!kind) return null;
  const deterministic = kind === "direct_collision" || kind === "shared_dependency" || kind === "downstream_impact";
  return { ...scope, id: pairId(scope, left.id, right.id, kind), kind, severity: kind === "assumption_conflict" ? "high" : "medium", confidenceBand: deterministic ? "deterministic" : semantic >= .82 ? "high" : "medium", workstreamIds, evidence, reason, revision: 1, priority: kind === "assumption_conflict" ? 90 : deterministic ? 75 : 60 };
}

export function evaluateWorkstreams(workstreams: readonly WorkstreamRecord[]): IntelligenceFinding[] {
  const findings: IntelligenceFinding[] = [];
  for (let left = 0; left < workstreams.length; left++) for (let right = left + 1; right < workstreams.length; right++) {
    const finding = evaluatePair(workstreams[left]!, workstreams[right]!); if (finding) findings.push(finding);
  }
  return findings.sort((a, b) => b.priority - a.priority || a.id.localeCompare(b.id));
}

export type BriefItem = Readonly<{ id: string; revision: number; kind: "finding" | "decision" | "dependency" | "workstream" | "truncation"; text: string; relevanceReason: string; fidelity: string; advisoryAction: "informational" | "review_recommended" | "coordination_required"; priority: number }>;

export function renderBrief(workstreamId: string, findings: readonly IntelligenceFinding[], requestedBudget: number): { items: BriefItem[]; renderedSize: number; truncated: boolean } {
  if (!Number.isInteger(requestedBudget) || requestedBudget < 128 || requestedBudget > 800) throw new Error("brief_budget_invalid");
  const candidates = findings.filter((finding) => finding.workstreamIds.includes(workstreamId)).sort((a, b) => b.priority - a.priority || a.id.localeCompare(b.id));
  const items: BriefItem[] = []; let renderedSize = 0; let truncated = false;
  for (const finding of candidates) {
    const item: BriefItem = { id: finding.id, revision: finding.revision, kind: "finding", text: finding.reason, relevanceReason: "This finding directly involves the current workstream.", fidelity: finding.confidenceBand === "deterministic" ? "structural" : "semantic", advisoryAction: finding.severity === "high" || finding.severity === "critical" ? "coordination_required" : "review_recommended", priority: finding.priority };
    const size = Math.ceil(JSON.stringify(item).length / 4);
    if (renderedSize + size > requestedBudget) {
      truncated = true;
      if (finding.priority >= 75) {
        const reference: BriefItem = { ...item, kind: "truncation", text: `Review ${finding.id}.`, relevanceReason: "Required context was compacted to honor the requested budget." };
        const referenceSize = Math.ceil(JSON.stringify(reference).length / 4);
        if (renderedSize + referenceSize <= requestedBudget) { items.push(reference); renderedSize += referenceSize; }
      }
      continue;
    }
    items.push(item); renderedSize += size;
  }
  return { items, renderedSize, truncated };
}

export function staleAssumption(previousItemRevisions: Readonly<Record<string, number>>, currentRelevant: readonly BriefItem[]): boolean {
  return currentRelevant.some((item) => (previousItemRevisions[item.id] ?? 0) < item.revision && item.priority >= 75);
}
