import type { CandidateRouter, EmbeddingProvider, SemanticIndex } from "@overgent/coordination";

export type HostedDependencies = Readonly<{
  embeddingProvider?: EmbeddingProvider;
  semanticIndex?: SemanticIndex;
  router: CandidateRouter;
}>;

export function semanticFidelity(dependencies: HostedDependencies): "structural" | "semantic" {
  return dependencies.embeddingProvider && dependencies.semanticIndex ? "semantic" : "structural";
}
