import type { AISettingsWrite } from "./native";

/**
 * Which provider a member can point intelligence at, and what its models are
 * called.
 *
 * There are still only three judgment providers in the schema — `anthropic`,
 * `openai-compatible` and `none` — and two embedding providers. Adding xAI or a
 * llama.cpp server on this Mac is not a new enum value in five files; it is
 * `openai-compatible` with a base URL filled in. That was already true before
 * this file existed. What was missing is that the picker made the member supply
 * the address from memory, so the plumbing supported eight providers and the
 * form offered two.
 *
 * A preset is therefore an **address book entry**, never a capability: it sets
 * `provider` and `baseUrl` and suggests model IDs, and every field it fills the
 * member can still edit.
 *
 * ## The base URL is an origin, not an endpoint
 *
 * `packages/coordination` appends the path itself — `/v1/messages`,
 * `/v1/chat/completions`, `/v1/embeddings`. A preset that stored
 * `https://api.groq.com/openai/v1` would produce `…/v1/v1/chat/completions` and
 * fail with a 404 the member would read as a bad key. Every entry below is the
 * documented endpoint with that suffix removed, and `endpointFor` reconstructs
 * the full URL so the screen can show the member exactly where their key is
 * about to be spent.
 *
 * ## Model IDs go stale, so they are dated and never authoritative
 *
 * These lists were read from each provider's own documentation on the date in
 * `PRESETS_CHECKED`, and they are offered as `<datalist>` suggestions — the
 * field stays free text, nothing is prefilled, and the screen says when the
 * list was checked. A member on a model released after that date types it and
 * is not fought. When refreshing this file, update `PRESETS_CHECKED` in the
 * same commit; a list that lies about its own age is worse than a short one.
 *
 * Sources, in the order the entries appear:
 *   Anthropic   platform.claude.com/docs/en/about-claude/models/overview
 *   OpenAI      developers.openai.com/api/docs/models
 *   xAI         docs.x.ai/docs/models · docs.x.ai/docs/api-reference
 *   Mistral     docs.mistral.ai/api/endpoint/chat · /api/endpoint/embeddings
 *   DeepSeek    api-docs.deepseek.com
 *   Groq        console.groq.com/docs/models
 *   OpenRouter  openrouter.ai/docs/quickstart
 *   Ollama      docs.ollama.com/api/openai-compatibility
 *   OpenAI embeddings  developers.openai.com/api/docs/guides/embeddings
 */
export const PRESETS_CHECKED = "5 September 2026";

type JudgmentProvider = AISettingsWrite["judgment"]["provider"];
type EmbeddingProvider = AISettingsWrite["embeddings"]["provider"];

export type ProviderPreset<Provider> = {
  /** The `<option>` value. `none`, `anthropic` and `deterministic` keep their
   *  enum names so the option a member picks and the value stored agree. */
  id: string;
  label: string;
  /** What to call this provider mid-sentence, when the option's own label does
   *  not read as a name ("Another OpenAI-compatible server accepts…"). */
  short?: string;
  provider: Provider;
  /** Origin only — see the note above. Absent means the provider's own default. */
  baseUrl?: string;
  /** Suggestions, in the order a member is most likely to want them. */
  models: readonly string[];
  /** Shown only when the entry changes what the member has to type. */
  note?: string;
  /** Exposes the address field. One entry per section carries this. */
  custom?: boolean;
};

export const JUDGMENT_PRESETS: readonly ProviderPreset<JudgmentProvider>[] = [
  { id: "none", label: "Off", provider: "none", models: [] },
  {
    id: "anthropic", label: "Anthropic", provider: "anthropic",
    models: ["claude-sonnet-5", "claude-opus-5", "claude-haiku-4-5", "claude-fable-5-1"],
  },
  {
    id: "openai", label: "OpenAI", provider: "openai-compatible",
    models: ["gpt-5.6-terra", "gpt-5.6-luna", "gpt-6-astra", "gpt-5.6-sol"],
  },
  {
    id: "custom", label: "OpenAI-compatible server", short: "this server", provider: "openai-compatible", models: [], custom: true,
    note: "Anything that serves OpenAI’s chat-completions API — a hosted vendor, or a model running on this Mac. The address is the origin only; Overgent adds /v1/chat/completions itself.",
  },
];

export const EMBEDDING_PRESETS: readonly ProviderPreset<EmbeddingProvider>[] = [
  { id: "deterministic", label: "Built-in", provider: "deterministic", models: [] },
  { id: "openai", label: "OpenAI", provider: "openai", models: ["text-embedding-3-large", "text-embedding-3-small"] },
  {
    id: "custom", label: "OpenAI-compatible server", short: "this server", provider: "openai", models: [], custom: true,
    note: "The address is the origin only; Overgent adds /v1/embeddings itself. The model must return exactly 1024 numbers.",
  },
];

/**
 * Vendors that would work today and are deliberately not offered yet.
 *
 * Each is `openai-compatible` with a base URL, so adding one is a single entry
 * in `JUDGMENT_PRESETS` — no enum change, no protocol change, no backend
 * change. They are held back because nothing here has been run against them;
 * an option in a picker is a claim that it works. The addresses below are the
 * documented endpoint with the `/v1` suffix removed, read on the date in
 * `PRESETS_CHECKED`, so whoever adds them is not starting from a search:
 *
 *   xAI         https://api.x.ai              grok-4.6, grok-4.5
 *   Mistral     https://api.mistral.ai        mistral-medium-latest
 *   DeepSeek    https://api.deepseek.com      deepseek-v4-flash, deepseek-v4-pro
 *   Groq        https://api.groq.com/openai   openai/gpt-oss-120b, llama-3.3-70b-versatile
 *   OpenRouter  https://openrouter.ai/api     vendor/model names, its catalogue is the list
 *   Ollama      http://localhost:11434        whatever is pulled; it ignores the key, and
 *                                             Overgent requires one of 8+ characters, so a
 *                                             placeholder is needed
 *
 * Until then the custom entry reaches every one of them: paste the address.
 */

/** Trailing slashes are what a member pastes, not what the client sends. */
const sameOrigin = (left: string | undefined, right: string | undefined): boolean =>
  (left ?? "").replace(/\/+$/, "") === (right ?? "").replace(/\/+$/, "");

/**
 * Which entry a saved configuration came from.
 *
 * Matching on provider *and* address is what keeps `openai-compatible` from
 * collapsing into one line: the five entries that share that provider differ
 * only by base URL. An address no entry claims is a custom server, which is a
 * legitimate answer and not a fallback — the member typed it.
 */
export function presetFor<Provider>(
  presets: readonly ProviderPreset<Provider>[],
  provider: Provider,
  baseUrl: string | null | undefined,
): ProviderPreset<Provider> {
  const named = presets.find((preset) => preset.provider === provider && sameOrigin(preset.baseUrl, baseUrl ?? undefined));
  if (named) return named;
  return presets.find((preset) => preset.provider === provider && preset.custom)
    ?? presets.find((preset) => preset.provider === provider)
    ?? presets[0]!;
}

/**
 * The URL a saved key is spent at, spelled out.
 *
 * The member is about to hand a credential to something; the one fact that
 * settles whether they meant to is the whole address, and until now no screen
 * showed it. Defaults match `packages/coordination` exactly — if one moves,
 * this display becomes a lie, which is why they are named here rather than
 * approximated.
 */
export function endpointFor(kind: "judgment" | "embeddings", provider: string, baseUrl: string | null | undefined): string {
  const fallback = provider === "anthropic" ? "https://api.anthropic.com" : "https://api.openai.com";
  const origin = (baseUrl?.trim() || fallback).replace(/\/+$/, "");
  if (kind === "embeddings") return `${origin}/v1/embeddings`;
  return provider === "anthropic" ? `${origin}/v1/messages` : `${origin}/v1/chat/completions`;
}
