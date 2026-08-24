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
