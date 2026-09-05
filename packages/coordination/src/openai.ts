import type { EmbeddedObject, EmbeddingProvider, SemanticInput } from "./index.js";
import { validateSemanticText } from "./intelligence.js";

export const OPENAI_EMBEDDING_MODEL = "text-embedding-3-large";

type FetchLike = (input: string, init: RequestInit) => Promise<Response>;

type OpenAIEmbeddingResponse = Readonly<{
  data?: Array<Readonly<{ index?: number; embedding?: unknown }>>;
}>;

/**
 * Provider-neutral adapter for the OpenAI embeddings endpoint. Callers own
 * secret retrieval: this package never reads a process environment, logs a
 * key, or accepts unfiltered text.
 */
export class OpenAIEmbeddingProvider implements EmbeddingProvider {
  readonly name: string;

  constructor(
    private readonly options: { apiKey: string; model?: string; dimensions: number; baseUrl?: string },
    private readonly fetcher: FetchLike = fetch,
  ) {
    if (!options.apiKey || options.apiKey.length < 8) throw new Error("openai_api_key_invalid");
    if (!options.model || options.model.length > 120) options.model = OPENAI_EMBEDDING_MODEL;
    if (!Number.isInteger(options.dimensions) || options.dimensions < 1 || options.dimensions > 3_072) throw new Error("openai_embedding_dimensions_invalid");
    this.name = `openai/${options.model}`;
  }

  async embed(inputs: readonly SemanticInput[], signal: AbortSignal): Promise<readonly EmbeddedObject[]> {
    if (signal.aborted) throw signal.reason;
    if (inputs.length === 0 || inputs.length > 128) throw new Error("openai_embedding_batch_invalid");
    const text = inputs.map((input) => validateSemanticText(input.text));
    const response = await this.fetcher(`${(this.options.baseUrl ?? "https://api.openai.com").replace(/\/$/, "")}/v1/embeddings`, {
      method: "POST",
      signal,
      headers: { "content-type": "application/json", authorization: `Bearer ${this.options.apiKey}` },
      body: JSON.stringify({ model: this.options.model, input: text, dimensions: this.options.dimensions, encoding_format: "float" }),
    });
    if (!response.ok) throw new Error(`openai_embedding_request_failed_${response.status}`);
    const payload = await response.json() as OpenAIEmbeddingResponse;
    if (!Array.isArray(payload.data) || payload.data.length !== inputs.length) throw new Error("openai_embedding_response_invalid");
    const ordered = [...payload.data].sort((left, right) => (left.index ?? -1) - (right.index ?? -1));
    return ordered.map((item, index) => {
      const vector = item.embedding;
      if (!Array.isArray(vector) || vector.length !== this.options.dimensions || vector.some((value) => typeof value !== "number" || !Number.isFinite(value))) throw new Error("openai_embedding_response_invalid");
      const input = inputs[index]!;
      return { ...input, text: text[index]!, model: this.name, dimensions: this.options.dimensions, vector: vector as number[] };
    });
  }
}
