export type Scope = Readonly<{ projectId: string; repositoryId: string }>;

export type SemanticInput = Scope & Readonly<{
  objectId: string;
  revision: number;
  text: string;
}>;

export type EmbeddedObject = SemanticInput & Readonly<{
  model: string;
  dimensions: number;
  vector: readonly number[];
}>;

export interface EmbeddingProvider {
  readonly name: string;
  embed(inputs: readonly SemanticInput[], signal: AbortSignal): Promise<readonly EmbeddedObject[]>;
}

export interface SemanticIndex {
  activate(objects: readonly EmbeddedObject[], signal: AbortSignal): Promise<void>;
  search(scope: Scope, vector: readonly number[], limit: number, signal: AbortSignal): Promise<readonly Candidate[]>;
  deleteScope(scope: Scope, signal: AbortSignal): Promise<void>;
}

export type Candidate = Readonly<{
  objectId: string;
  revision: number;
  evidence: "structural" | "lexical" | "semantic";
  score: number;
}>;

/**
 * A provider capability is deliberately narrower than a product promise. The
 * coordinator uses this record to select a delivery/observation path and must
 * never infer that an adapter can steer a running agent just because it can
 * observe a session.
 */
export type HarnessCapabilities = Readonly<{
  observeSession: boolean;
  observeToolActivity: boolean;
  observeSafePaths: boolean;
  readExistingSession: boolean;
  pollUpdates: boolean;
  deliverBrief: "mcp_pull" | "native_pull" | "native_push" | "unavailable";
  requestAttention: "advisory" | "unavailable";
}>;

export const NO_HARNESS_CAPABILITIES: HarnessCapabilities = {
  observeSession: false,
  observeToolActivity: false,
  observeSafePaths: false,
  readExistingSession: false,
  pollUpdates: false,
  deliverBrief: "unavailable",
  requestAttention: "unavailable",
};

export const PROJECT_HOOK_MCP_CAPABILITIES: HarnessCapabilities = {
  observeSession: true,
  observeToolActivity: true,
  observeSafePaths: true,
  readExistingSession: true,
  pollUpdates: true,
  deliverBrief: "mcp_pull",
  requestAttention: "unavailable",
};

export function canDeliverRelevantUpdate(capabilities: HarnessCapabilities): boolean {
  return capabilities.deliverBrief !== "unavailable";
}

export type RouteInput = Scope & Readonly<{
  structural: readonly Candidate[];
  lexical: readonly Candidate[];
  semantic: readonly Candidate[];
  limit: number;
}>;

export interface CandidateRouter {
  route(input: RouteInput): readonly Candidate[];
}

export class DeterministicCandidateRouter implements CandidateRouter {
  route(input: RouteInput): readonly Candidate[] {
    const priority: Record<Candidate["evidence"], number> = { structural: 0, lexical: 1, semantic: 2 };
    const byObject = new Map<string, Candidate>();
    for (const candidate of [...input.structural, ...input.lexical, ...input.semantic]) {
      const current = byObject.get(candidate.objectId);
      if (!current || priority[candidate.evidence] < priority[current.evidence] ||
        (priority[candidate.evidence] === priority[current.evidence] && candidate.score > current.score)) {
        byObject.set(candidate.objectId, candidate);
      }
    }
    return [...byObject.values()]
      .sort((a, b) => priority[a.evidence] - priority[b.evidence] || b.score - a.score || a.objectId.localeCompare(b.objectId))
      .slice(0, Math.max(0, input.limit));
  }
}

export * from "./intelligence.js";
export * from "./openai.js";
