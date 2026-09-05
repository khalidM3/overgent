import { v } from "convex/values";
import { internalAction, internalMutation, internalQuery } from "./_generated/server";
import type { ActionCtx, QueryCtx } from "./_generated/server";
import type { Id } from "./_generated/dataModel";
import { internal } from "./_generated/api";
import { ANTHROPIC_JUDGMENT_MODEL, AnthropicJudgmentProvider, OpenAICompatibleJudgmentProvider, OpenAIEmbeddingProvider, OPENAI_EMBEDDING_MODEL, conceptVector, deterministicJudgment, judgeCandidate, needsManagedAdjudication, type JudgmentCandidate, type JudgmentProvider } from "@overgent/coordination";

const OPENAI_EMBEDDING_DIMENSIONS = 1024;
const FALLBACK_EMBEDDING_MODEL_VERSION = "overgent-concepts/v1/1024";
const FOREIGN_EMBEDDING_MIGRATION_BATCH = 100;
const DEFAULT_JUDGMENT = { provider: "anthropic" as const, model: ANTHROPIC_JUDGMENT_MODEL };
const DEFAULT_EMBEDDINGS = { provider: "openai" as const, model: OPENAI_EMBEDDING_MODEL, dimensions: OPENAI_EMBEDDING_DIMENSIONS };

type DegradedReason = "not_configured" | "provider_unconfigured" | "quota" | "provider_error" | "offline" | "paused";

type StoredAISettings = {
  revision: number;
  judgmentProvider: "anthropic" | "openai-compatible" | "none";
  judgmentModel: string;
  judgmentBaseUrl?: string;
  judgmentKeyCiphertext?: string;
  judgmentKeyHint?: string;
  embeddingProvider: "openai" | "deterministic";
  embeddingModel: string;
  embeddingDimensions: number;
  embeddingBaseUrl?: string;
  embeddingKeyCiphertext?: string;
  embeddingKeyHint?: string;
  updatedAt: number;
};

function decodeSecretsKey(encoded: string): Uint8Array {
  let raw: string;
  try { raw = atob(encoded); } catch { throw new Error("secrets_key_invalid"); }
  const bytes = Uint8Array.from(raw, (char) => char.charCodeAt(0));
  if (bytes.length !== 32) throw new Error("secrets_key_invalid");
  return bytes;
}

function encodeBase64(bytes: Uint8Array): string {
  let value = "";
  for (const byte of bytes) value += String.fromCharCode(byte);
  return btoa(value);
}

function exactBuffer(bytes: Uint8Array): ArrayBuffer {
  return Uint8Array.from(bytes).buffer;
}

/** AES-256-GCM ciphertext is nonce || sealed bytes and is bound to Project+field. */
export async function encryptProjectSecret(secret: string, deploymentKey: string, projectId: string, field: string, suppliedNonce?: Uint8Array): Promise<string> {
  const key = await crypto.subtle.importKey("raw", exactBuffer(decodeSecretsKey(deploymentKey)), "AES-GCM", false, ["encrypt"]);
  const nonce = suppliedNonce ?? crypto.getRandomValues(new Uint8Array(12));
  if (nonce.length !== 12) throw new Error("secret_nonce_invalid");
  const additionalData = new TextEncoder().encode(`${projectId}:${field}`);
  const encrypted = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv: exactBuffer(nonce), additionalData: exactBuffer(additionalData) }, key, exactBuffer(new TextEncoder().encode(secret))));
  const sealed = new Uint8Array(nonce.length + encrypted.length);
  sealed.set(nonce); sealed.set(encrypted, nonce.length);
  return encodeBase64(sealed);
}

export async function decryptProjectSecret(ciphertext: string, deploymentKey: string, projectId: string, field: string): Promise<string> {
  const sealed = decodeBase64(ciphertext);
  if (sealed.length < 29) throw new Error("secret_ciphertext_invalid");
  const key = await crypto.subtle.importKey("raw", exactBuffer(decodeSecretsKey(deploymentKey)), "AES-GCM", false, ["decrypt"]);
  const additionalData = new TextEncoder().encode(`${projectId}:${field}`);
  const plaintext = await crypto.subtle.decrypt({ name: "AES-GCM", iv: exactBuffer(sealed.slice(0, 12)), additionalData: exactBuffer(additionalData) }, key, exactBuffer(sealed.slice(12)));
  return new TextDecoder().decode(plaintext);
}

function decodeBase64(encoded: string): Uint8Array {
  try { return Uint8Array.from(atob(encoded), (char) => char.charCodeAt(0)); }
  catch { throw new Error("secret_ciphertext_invalid"); }
}

type ResolvedProviders = {
  settings: StoredAISettings | null;
  effective: { judgment: "project" | "operator" | "none"; embeddings: "project" | "operator" | "deterministic" };
  judgment?: { provider: "anthropic" | "openai-compatible"; apiKey: string; model: string; baseUrl?: string };
  embeddings?: { provider: "openai"; apiKey: string; model: string; dimensions: number; baseUrl?: string };
};

export function selectProviderSource(options: { disabled: boolean; projectKeyUsable: boolean; operatorEnabled: boolean; operatorKeyConfigured: boolean; fallback: "none" | "deterministic" }): "project" | "operator" | "none" | "deterministic" {
  if (options.disabled) return options.fallback;
  if (options.projectKeyUsable) return "project";
  if (options.operatorEnabled && options.operatorKeyConfigured) return "operator";
  return options.fallback;
}

async function resolveProviders(ctx: ActionCtx, projectId: Id<"projects">): Promise<ResolvedProviders> {
  const loaded = await ctx.runQuery(internal.service.projectAISettingsForProvider, { projectId }) as { projectPublicId: string; settings: StoredAISettings | null } | null;
  if (!loaded) return { settings: null, effective: { judgment: "none", embeddings: "deterministic" } };
  const settings = loaded.settings;
  const judgmentProvider = settings?.judgmentProvider ?? DEFAULT_JUDGMENT.provider;
  const embeddingProvider = settings?.embeddingProvider ?? DEFAULT_EMBEDDINGS.provider;
  const secretsKey = process.env.OVERGENT_SECRETS_KEY;
  let judgment: ResolvedProviders["judgment"];
  let embeddings: ResolvedProviders["embeddings"];
  let judgmentEffective: ResolvedProviders["effective"]["judgment"] = "none";
  let embeddingEffective: ResolvedProviders["effective"]["embeddings"] = "deterministic";
  const judgmentOperatorKey = judgmentProvider === "anthropic" ? process.env.ANTHROPIC_API_KEY : process.env.OPENAI_API_KEY;
  const judgmentSource = selectProviderSource({ disabled: judgmentProvider === "none", projectKeyUsable: Boolean(settings?.judgmentKeyCiphertext && secretsKey), operatorEnabled: process.env.OVERGENT_OPERATOR_KEYS_ENABLED === "true", operatorKeyConfigured: Boolean(judgmentOperatorKey), fallback: "none" });
  if (judgmentSource === "project" && settings?.judgmentKeyCiphertext && secretsKey && judgmentProvider !== "none") {
    judgment = { provider: judgmentProvider, apiKey: await decryptProjectSecret(settings.judgmentKeyCiphertext, secretsKey, loaded.projectPublicId, "judgment"), model: settings.judgmentModel, ...(settings.judgmentBaseUrl ? { baseUrl: settings.judgmentBaseUrl } : {}) };
    judgmentEffective = "project";
  } else if (judgmentSource === "operator" && judgmentProvider !== "none" && judgmentOperatorKey) {
    judgment = { provider: judgmentProvider, apiKey: judgmentOperatorKey, model: settings?.judgmentModel ?? DEFAULT_JUDGMENT.model, ...(settings?.judgmentBaseUrl ? { baseUrl: settings.judgmentBaseUrl } : {}) };
    judgmentEffective = "operator";
  }
  const embeddingSource = selectProviderSource({ disabled: embeddingProvider === "deterministic", projectKeyUsable: Boolean(settings?.embeddingKeyCiphertext && secretsKey), operatorEnabled: process.env.OVERGENT_OPERATOR_KEYS_ENABLED === "true", operatorKeyConfigured: Boolean(process.env.OPENAI_API_KEY), fallback: "deterministic" });
  if (embeddingSource === "project" && settings?.embeddingKeyCiphertext && secretsKey && embeddingProvider === "openai") {
    embeddings = { provider: "openai", apiKey: await decryptProjectSecret(settings.embeddingKeyCiphertext, secretsKey, loaded.projectPublicId, "embeddings"), model: settings.embeddingModel, dimensions: settings.embeddingDimensions, ...(settings.embeddingBaseUrl ? { baseUrl: settings.embeddingBaseUrl } : {}) };
    embeddingEffective = "project";
  } else if (embeddingSource === "operator" && process.env.OPENAI_API_KEY) {
    embeddings = { provider: "openai", apiKey: process.env.OPENAI_API_KEY, model: settings?.embeddingModel ?? DEFAULT_EMBEDDINGS.model, dimensions: settings?.embeddingDimensions ?? DEFAULT_EMBEDDINGS.dimensions, ...(settings?.embeddingBaseUrl ? { baseUrl: settings.embeddingBaseUrl } : {}) };
    embeddingEffective = "operator";
  }
  return { settings, effective: { judgment: judgmentEffective, embeddings: embeddingEffective }, judgment, embeddings };
}

function publicAISettings(settings: StoredAISettings | null, effective: ResolvedProviders["effective"], revision?: number, updatedAt?: number) {
  return {
    judgment: { provider: settings?.judgmentProvider ?? DEFAULT_JUDGMENT.provider, model: settings?.judgmentModel ?? DEFAULT_JUDGMENT.model, baseUrl: settings?.judgmentBaseUrl ?? null, keyConfigured: Boolean(settings?.judgmentKeyCiphertext), keyHint: settings?.judgmentKeyHint ?? null },
    embeddings: { provider: settings?.embeddingProvider ?? DEFAULT_EMBEDDINGS.provider, model: settings?.embeddingModel ?? DEFAULT_EMBEDDINGS.model, dimensions: settings?.embeddingDimensions ?? DEFAULT_EMBEDDINGS.dimensions, baseUrl: settings?.embeddingBaseUrl ?? null, keyConfigured: Boolean(settings?.embeddingKeyCiphertext), keyHint: settings?.embeddingKeyHint ?? null },
    effective,
    revision: revision ?? settings?.revision ?? 1,
    updatedAt: new Date(updatedAt ?? settings?.updatedAt ?? 0).toISOString(),
  };
}

type AISettingsResponse = ReturnType<typeof publicAISettings>;

function scopeModelKey(scopeKey: string, modelVersion: string): string {
  return `${scopeKey.length}:${scopeKey}${modelVersion}`;
}

function providerFailureReason(error: unknown): DegradedReason {
  const message = error instanceof Error ? error.message : String(error);
  if (/429|quota|rate.?limit/i.test(message)) return "quota";
  if (/abort|timeout|timed out|network|fetch failed|unavailable|connection/i.test(message)) return "offline";
  return "provider_error";
}

export const getProjectAISettings = internalAction({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await ctx.runQuery(internal.service.projectAISettingsAuth, { ...args, ownerOnly: false }) as { projectId: Id<"projects"> };
    const resolved = await resolveProviders(ctx, auth.projectId);
    return publicAISettings(resolved.settings, resolved.effective);
  },
});

export const configureProjectEmbeddingModel = internalMutation({
  args: { projectId: v.id("projects"), provider: v.union(v.literal("openai"), v.literal("deterministic")), model: v.string(), dimensions: v.number(), now: v.number() },
  handler: async (ctx, args) => {
    const modelVersion = args.provider === "deterministic" ? FALLBACK_EMBEDDING_MODEL_VERSION : `${args.model}/${args.dimensions}`;
    const scopes = await ctx.db.query("repositoryScopes").withIndex("by_project", (q) => q.eq("projectId", args.projectId)).take(101);
    if (scopes.length > 100) throw new Error("E:page_too_large");
    for (const scope of scopes) {
      if (scope.semanticModelVersion === modelVersion) continue;
      await ctx.db.patch(scope._id, { semanticProviderName: args.provider === "deterministic" ? "overgent" : `openai/${args.model}`, semanticModelVersion: modelVersion, updatedAt: args.now });
      await ctx.scheduler.runAfter(0, internal.intelligence.convergeForeignEmbeddings, { scopeKey: scope.scopeKey, modelVersion });
    }
  },
});

type PutProjectAISettingsArgs = {
  tokenHash?: string;
  sessionHash?: string;
  projectPublicId: string;
  write: unknown;
  now: number;
};

// The HTTP action calls this handler directly so the plaintext key exists only
// in that request's memory. Passing it through ctx.runAction would make the key
// part of a second Convex invocation's arguments; only the encrypted mutation
// payload is allowed to cross that boundary.
export async function putProjectAISettingsHandler(ctx: ActionCtx, args: PutProjectAISettingsArgs): Promise<AISettingsResponse> {
    const auth = await ctx.runQuery(internal.service.projectAISettingsAuth, {
      tokenHash: args.tokenHash, sessionHash: args.sessionHash, projectPublicId: args.projectPublicId, ownerOnly: true, now: args.now,
    }) as { projectId: Id<"projects">; projectPublicId: string; settings: StoredAISettings | null };
    const write = args.write as {
      judgment: { provider: "anthropic" | "openai-compatible" | "none"; model: string; baseUrl?: string; apiKey?: string };
      embeddings: { provider: "openai" | "deterministic"; model: string; dimensions: number; baseUrl?: string; apiKey?: string };
    };
    let judgmentKeyCiphertext = auth.settings?.judgmentKeyCiphertext;
    let judgmentKeyHint = auth.settings?.judgmentKeyHint;
    let embeddingKeyCiphertext = auth.settings?.embeddingKeyCiphertext;
    let embeddingKeyHint = auth.settings?.embeddingKeyHint;
    const secretsKey = process.env.OVERGENT_SECRETS_KEY;
    if (write.judgment.provider === "none") { judgmentKeyCiphertext = undefined; judgmentKeyHint = undefined; }
    else if (write.judgment.apiKey !== undefined) {
      if (write.judgment.apiKey === "") { judgmentKeyCiphertext = undefined; judgmentKeyHint = undefined; }
      else {
        if (!secretsKey) throw new Error("E:secrets_key_unconfigured");
        judgmentKeyCiphertext = await encryptProjectSecret(write.judgment.apiKey, secretsKey, auth.projectPublicId, "judgment");
        judgmentKeyHint = `…${write.judgment.apiKey.slice(-4)}`;
      }
    }
    if (write.embeddings.provider === "deterministic") { embeddingKeyCiphertext = undefined; embeddingKeyHint = undefined; }
    else if (write.embeddings.apiKey !== undefined) {
      if (write.embeddings.apiKey === "") { embeddingKeyCiphertext = undefined; embeddingKeyHint = undefined; }
      else {
        if (!secretsKey) throw new Error("E:secrets_key_unconfigured");
        embeddingKeyCiphertext = await encryptProjectSecret(write.embeddings.apiKey, secretsKey, auth.projectPublicId, "embeddings");
        embeddingKeyHint = `…${write.embeddings.apiKey.slice(-4)}`;
      }
    }
    const saved: { projectId: Id<"projects">; revision: number; updatedAt: number } = await ctx.runMutation(internal.service.saveProjectAISettings, {
      tokenHash: args.tokenHash, sessionHash: args.sessionHash, projectPublicId: args.projectPublicId, now: args.now,
      judgmentProvider: write.judgment.provider, judgmentModel: write.judgment.model, judgmentBaseUrl: write.judgment.baseUrl,
      judgmentKeyCiphertext, judgmentKeyHint,
      embeddingProvider: write.embeddings.provider, embeddingModel: write.embeddings.model, embeddingDimensions: write.embeddings.dimensions, embeddingBaseUrl: write.embeddings.baseUrl,
      embeddingKeyCiphertext, embeddingKeyHint,
    });
    await ctx.runMutation(internal.intelligence.configureProjectEmbeddingModel, { projectId: auth.projectId, provider: write.embeddings.provider, model: write.embeddings.model, dimensions: write.embeddings.dimensions, now: args.now });
    const resolved = await resolveProviders(ctx, auth.projectId);
    return publicAISettings(resolved.settings, resolved.effective, saved.revision, saved.updatedAt);
}

export const putProjectAISettings = internalAction({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), write: v.any(), now: v.number() },
  handler: putProjectAISettingsHandler,
});

async function loadContext(ctx: QueryCtx, args: { tokenHash: string; workstreamPublicId: string }) {
    const device = await ctx.db.query("devices").withIndex("by_token_hash", (q) => q.eq("tokenHash", args.tokenHash)).unique();
    // Codes travel to the HTTP boundary in the shared `E:` form. A bare message
    // classifies as internal_error, which reported an unknown workstream as a
    // retryable server fault on the brief path.
    if (!device || device.revokedAt !== undefined) throw new Error("E:unauthorized");
    const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", args.workstreamPublicId)).unique();
    if (!workstream) throw new Error("E:not_found");
    const member = await ctx.db.query("members").withIndex("by_project_device", (q) => q.eq("projectId", workstream.projectId).eq("deviceId", device._id)).unique();
    if (!member || member.removedAt !== undefined) throw new Error("E:forbidden");
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).unique();
    const object = (await ctx.db.query("semanticObjects").withIndex("by_workstream_active", (q) => q.eq("workstreamId", workstream._id).eq("active", true)).take(3)).find((candidate) => candidate.kind === "intent");
    const embedding = object ? await ctx.db.query("semanticEmbeddings").withIndex("by_object", (q) => q.eq("objectId", object._id)).unique() : null;
    if (!scope || !embedding) return { available: false as const, scopeKey: workstream.scopeKey, contextRevision: scope?.contextRevision ?? 0, vector: [], modelVersion: "" };
    return { available: true as const, scopeKey: workstream.scopeKey, contextRevision: scope.contextRevision, vector: embedding.vector, modelVersion: embedding.modelVersion };
}

export const semanticSearchContext = internalQuery({
  args: { tokenHash: v.string(), workstreamPublicId: v.string() },
  handler: loadContext,
});

export const loadCurrentSemanticMatches = internalQuery({
  args: { tokenHash: v.string(), workstreamPublicId: v.string(), scopeKey: v.string(), modelVersion: v.string(), contextRevision: v.number(), embeddingIds: v.array(v.id("semanticEmbeddings")) },
  handler: async (ctx, args) => {
    const context = await loadContext(ctx, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId });
    if (context.scopeKey !== args.scopeKey || context.contextRevision !== args.contextRevision) return { retry: true, objectIds: [] as string[] };
    const objectIds: string[] = [];
    for (const embeddingId of args.embeddingIds) {
      const embedding = await ctx.db.get(embeddingId);
      if (!embedding || embedding.scopeKey !== args.scopeKey || embedding.modelVersion !== args.modelVersion) continue;
      const object = await ctx.db.get(embedding.objectId);
      if (!object || !object.active || object.scopeKey !== args.scopeKey || object.revision !== embedding.contentRevision) continue;
      const workstream = await ctx.db.get(object.workstreamId);
      if (!workstream || workstream.publicId === args.workstreamPublicId || workstream.status === "done") continue;
      objectIds.push(object.publicId);
    }
    return { retry: false, objectIds };
  },
});

export const semanticEmbeddingInput = internalQuery({
  args: { semanticObjectPublicId: v.string(), expectedRevision: v.number() },
  handler: async (ctx, args) => {
    const object = await ctx.db.query("semanticObjects").withIndex("by_public_id", (q) => q.eq("publicId", args.semanticObjectPublicId)).unique();
    if (!object || !object.active || object.revision !== args.expectedRevision) return null;
    return { publicId: object.publicId, revision: object.revision, text: object.text, scopeKey: object.scopeKey, projectId: object.projectId };
  },
});

export const applyOpenAIEmbedding = internalMutation({
  args: { semanticObjectPublicId: v.string(), expectedRevision: v.number(), providerName: v.string(), modelVersion: v.string(), vector: v.array(v.float64()), now: v.number() },
  handler: async (ctx, args) => {
    const object = await ctx.db.query("semanticObjects").withIndex("by_public_id", (q) => q.eq("publicId", args.semanticObjectPublicId)).unique();
    if (!object || !object.active || object.revision !== args.expectedRevision || args.vector.length !== OPENAI_EMBEDDING_DIMENSIONS) return { applied: false, modelChanged: false };
    const existing = await ctx.db.query("semanticEmbeddings").withIndex("by_object", (q) => q.eq("objectId", object._id)).unique();
    const record = { scopeKey: object.scopeKey, scopeModelKey: scopeModelKey(object.scopeKey, args.modelVersion), providerName: args.providerName, modelVersion: args.modelVersion, contentRevision: object.revision, vector: args.vector, expiresAt: object.expiresAt };
    if (existing) await ctx.db.patch(existing._id, record);
    else await ctx.db.insert("semanticEmbeddings", { objectId: object._id, ...record });
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", object.scopeKey)).unique();
    const modelChanged = scope !== null && scope.semanticModelVersion !== args.modelVersion;
    if (scope) await ctx.db.patch(scope._id, {
      semanticHealthyAt: args.now,
      semanticDegradedReason: undefined,
      semanticProviderName: args.providerName,
      semanticModelVersion: args.modelVersion,
    });
    return { applied: true, modelChanged };
  },
});

export const configureFallbackEmbeddingModel = internalMutation({
  args: { scopeKey: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey)).unique();
    if (!scope || scope.semanticModelVersion === FALLBACK_EMBEDDING_MODEL_VERSION) return false;
    await ctx.db.patch(scope._id, {
      semanticProviderName: "overgent",
      semanticModelVersion: FALLBACK_EMBEDDING_MODEL_VERSION,
      updatedAt: args.now,
    });
    return true;
  },
});

// Provider switches are a bounded migration, and each object holds exactly one
// embedding row, so convergence means re-embedding that row - never deleting
// it. A deleted row is unrecoverable until the object's content next changes,
// which for a stable intent summary is never; an env blip would have silently
// hollowed out recall. The scope guard makes a queued page harmless if the
// configured provider changes again before it runs.
export const convergeForeignEmbeddings = internalMutation({
  args: { scopeKey: v.string(), modelVersion: v.string(), cursor: v.optional(v.string()) },
  handler: async (ctx, args) => {
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey)).unique();
    if (!scope || scope.semanticModelVersion !== args.modelVersion) return 0;
    const page = await ctx.db.query("semanticEmbeddings")
      .withIndex("by_scope_model", (q) => q.eq("scopeKey", args.scopeKey))
      .paginate({ cursor: args.cursor ?? null, numItems: FOREIGN_EMBEDDING_MIGRATION_BATCH });
    let converged = 0;
    const composite = scopeModelKey(args.scopeKey, args.modelVersion);
    for (const embedding of page.page) {
      if (embedding.modelVersion === args.modelVersion) {
        if (embedding.scopeModelKey !== composite) await ctx.db.patch(embedding._id, { scopeModelKey: composite });
        continue;
      }
      const object = await ctx.db.get(embedding.objectId);
      if (!object) {
        // Only an orphan - a row whose object is gone - has nothing to
        // converge to, and nothing can ever search it again.
        await ctx.db.delete(embedding._id);
        continue;
      }
      if (args.modelVersion === FALLBACK_EMBEDDING_MODEL_VERSION) {
        // The fallback is deterministic, so the row converges in place from
        // the object's own text - this also heals the pre-existing 32-dim
        // population left by an un-migrated CONCEPT_DIMENSIONS change.
        await ctx.db.patch(embedding._id, {
          scopeKey: object.scopeKey, scopeModelKey: composite, providerName: "overgent",
          modelVersion: args.modelVersion, contentRevision: object.revision, vector: conceptVector(object.text),
        });
        converged++;
        continue;
      }
      // A managed provider needs an action; the old row stays search-invisible
      // (the scopeModelKey filter excludes it) until the apply overwrites it.
      // Inactive objects are never matched, so their rows are left as-is.
      if (object.active) {
        await ctx.scheduler.runAfter(0, internal.intelligence.embedSemanticObject, {
          semanticObjectPublicId: object.publicId, expectedRevision: object.revision,
        });
        converged++;
      }
    }
    if (!page.isDone) {
      await ctx.scheduler.runAfter(0, internal.intelligence.convergeForeignEmbeddings, {
        scopeKey: args.scopeKey,
        modelVersion: args.modelVersion,
        cursor: page.continueCursor,
      });
    }
    return converged;
  },
});

export const recordOpenAIEmbeddingFailure = internalMutation({
  args: {
    scopeKey: v.string(),
    now: v.number(),
    reason: v.union(v.literal("not_configured"), v.literal("provider_unconfigured"), v.literal("quota"), v.literal("provider_error"), v.literal("offline"), v.literal("paused")),
    providerName: v.string(),
  },
  handler: async (ctx, args) => {
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey)).unique();
    if (scope) await ctx.db.patch(scope._id, { semanticDegradedAt: args.now, semanticDegradedReason: args.reason, semanticProviderName: args.providerName });
  },
});

export const embedSemanticObject = internalAction({
  args: { semanticObjectPublicId: v.string(), expectedRevision: v.number() },
  handler: async (ctx, args): Promise<{ applied: boolean; mode: "stale" | "fallback" | "openai" }> => {
    const input: { publicId: string; revision: number; text: string; scopeKey: string; projectId: Id<"projects"> } | null = await ctx.runQuery(internal.intelligence.semanticEmbeddingInput, args);
    if (!input) return { applied: false, mode: "stale" as const };
    const resolved = await resolveProviders(ctx, input.projectId);
    if (!resolved.embeddings) {
      const switched: boolean = await ctx.runMutation(internal.intelligence.configureFallbackEmbeddingModel, { scopeKey: input.scopeKey, now: Date.now() });
      if (switched) await ctx.runMutation(internal.intelligence.convergeForeignEmbeddings, { scopeKey: input.scopeKey, modelVersion: FALLBACK_EMBEDDING_MODEL_VERSION });
      await ctx.runMutation(internal.intelligence.recordOpenAIEmbeddingFailure, { scopeKey: input.scopeKey, now: Date.now(), reason: "provider_unconfigured", providerName: "openai" });
      return { applied: false, mode: "fallback" as const };
    }
    try {
      const provider = new OpenAIEmbeddingProvider(resolved.embeddings);
      const [embedded] = await provider.embed([{ projectId: "internal", repositoryId: input.scopeKey, objectId: input.publicId, revision: input.revision, text: input.text }], AbortSignal.timeout(10_000));
      if (!embedded) throw new Error("openai_embedding_missing");
      const modelVersion = `${resolved.embeddings.model}/${resolved.embeddings.dimensions}`;
      const applied: { applied: boolean; modelChanged: boolean } = await ctx.runMutation(internal.intelligence.applyOpenAIEmbedding, {
        semanticObjectPublicId: input.publicId, expectedRevision: input.revision, providerName: provider.name, modelVersion, vector: [...embedded.vector], now: Date.now(),
      });
      if (applied.applied) {
        // Convergence only on an actual provider switch; re-scanning the scope
        // after every routine embed would be an O(N^2) page storm.
        if (applied.modelChanged) await ctx.runMutation(internal.intelligence.convergeForeignEmbeddings, { scopeKey: input.scopeKey, modelVersion });
        await ctx.runMutation(internal.service.refreshSemanticFindings, { scopeKey: input.scopeKey, now: Date.now() });
      }
      return { applied: applied.applied, mode: "openai" as const };
    } catch (error) {
      // The reason a scope degraded is the only thing that can tell an operator
      // whether to wait, add credit, or fix a key. Recording only the boolean
      // made every embedding failure indistinguishable from every other.
      console.error("openai_embedding_failed", { scopeKey: input.scopeKey, reason: providerFailureReason(error) });
      await ctx.runMutation(internal.intelligence.recordOpenAIEmbeddingFailure, { scopeKey: input.scopeKey, now: Date.now(), reason: providerFailureReason(error), providerName: `openai/${resolved.embeddings.model}` });
      return { applied: false, mode: "fallback" as const };
    }
  },
});

export const searchSemantic = internalAction({
  args: { tokenHash: v.string(), workstreamPublicId: v.string(), limit: v.number() },
  handler: async (ctx, args): Promise<{ available: boolean; degraded: boolean; contextRevision: number; objectIds: string[] }> => {
    for (let attempt = 0; attempt < 2; attempt++) {
      const context = await ctx.runQuery(internal.intelligence.semanticSearchContext, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId });
      if (!context.available) {
        await ctx.runMutation(internal.service.recordSemanticHealth, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId, degraded: true, reason: "provider_error", now: Date.now() });
        return { available: false, degraded: true, contextRevision: context.contextRevision, objectIds: [] };
      }
      let results;
      try {
        results = await ctx.vectorSearch("semanticEmbeddings", "by_vector", {
          vector: context.vector,
          limit: Math.max(1, Math.min(args.limit, 64)),
          filter: (q) => q.eq("scopeModelKey", scopeModelKey(context.scopeKey, context.modelVersion)),
        });
      } catch {
        if (attempt === 0) continue;
        await ctx.runMutation(internal.service.recordSemanticHealth, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId, degraded: true, reason: "provider_error", now: Date.now() });
        return { available: false, degraded: true, contextRevision: context.contextRevision, objectIds: [] };
      }
      const embeddingIds = results.filter((result) => result._score >= 0.65).map((result) => result._id);
      const loaded = await ctx.runQuery(internal.intelligence.loadCurrentSemanticMatches, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId, scopeKey: context.scopeKey, modelVersion: context.modelVersion, contextRevision: context.contextRevision, embeddingIds });
      if (!loaded.retry) {
        await ctx.runMutation(internal.service.recordSemanticHealth, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId, degraded: false, now: Date.now() });
        return { available: true, degraded: false, contextRevision: context.contextRevision, objectIds: loaded.objectIds };
      }
    }
    return { available: false, degraded: true, contextRevision: 0, objectIds: [] };
  },
});

// ---------------------------------------------------------------------------
// Managed adjudication (ADR-045)
//
// The deterministic verdict is already durable before this action runs: it set
// the finding's severity, explanation, and delivery synchronously. A managed
// adjudication only improves the wording and the precision of a verdict that
// already exists, so provider latency, outage, or an absent key cannot delay
// or remove a finding. A late result is discarded when the finding moved on.
// ---------------------------------------------------------------------------

const JUDGMENT_TIMEOUT_MILLIS = 10_000;
// Bounded per project per hour. Adjudication is a cost, not a correctness
// requirement, so the budget is enforced before the request is made.
const JUDGMENT_BUDGET_PER_PROJECT = 60;
const JUDGMENT_BUDGET_WINDOW = 3_600_000;

export const judgmentInput = internalQuery({
  args: { findingPublicId: v.string(), expectedRevision: v.number() },
  handler: async (ctx, args) => {
    const finding = await ctx.db.query("findings").withIndex("by_public_id", (q) => q.eq("publicId", args.findingPublicId)).unique();
    if (!finding || finding.state !== "open" || finding.revision !== args.expectedRevision) return null;
    return { scopeKey: finding.scopeKey, projectId: finding.projectId, severity: finding.severity, delivery: finding.delivery ?? "dashboard" };
  },
});

export const claimJudgmentBudget = internalMutation({
  args: { projectId: v.id("projects"), now: v.number() },
  handler: async (ctx, args) => {
    const project = await ctx.db.get(args.projectId);
    if (!project || project.status !== "active") return { claimed: false, recoversAt: args.now };
    const key = project.publicId;
    const existing = await ctx.db.query("rateLimits").withIndex("by_key_route", (q) => q.eq("key", key).eq("route", "judgment.adjudicate")).unique();
    if (!existing || existing.windowStartedAt + JUDGMENT_BUDGET_WINDOW <= args.now) {
      const record = { windowStartedAt: args.now, count: 1, expiresAt: args.now + 2 * JUDGMENT_BUDGET_WINDOW };
      if (existing) await ctx.db.patch(existing._id, record);
      else await ctx.db.insert("rateLimits", { key, route: "judgment.adjudicate", ...record });
      return { claimed: true, recoversAt: args.now + JUDGMENT_BUDGET_WINDOW };
    }
    const recoversAt = existing.windowStartedAt + JUDGMENT_BUDGET_WINDOW;
    if (existing.count >= JUDGMENT_BUDGET_PER_PROJECT) return { claimed: false, recoversAt };
    await ctx.db.patch(existing._id, { count: existing.count + 1 });
    return { claimed: true, recoversAt };
  },
});

export const applyJudgmentVerdict = internalMutation({
  args: {
    findingPublicId: v.string(), expectedRevision: v.number(), providerName: v.string(),
    severity: v.string(), reason: v.string(), delivery: v.string(), now: v.number(),
  },
  handler: async (ctx, args) => {
    const finding = await ctx.db.query("findings").withIndex("by_public_id", (q) => q.eq("publicId", args.findingPublicId)).unique();
    if (!finding || finding.state !== "open" || finding.revision !== args.expectedRevision) return false;
    // A managed verdict refines an existing finding; it is never allowed to
    // retract deterministic evidence by silencing it.
    const delivery = args.delivery === "silent" ? (finding.delivery ?? "dashboard") : args.delivery;
    if (finding.severity === args.severity && finding.reason === args.reason && (finding.delivery ?? "dashboard") === delivery) return false;
    await ctx.db.patch(finding._id, {
      severity: args.severity, reason: args.reason, delivery, judgmentProvider: args.providerName,
      revision: finding.revision + 1, lastSeenAt: args.now,
    });
    return true;
  },
});

export const recordJudgmentDegraded = internalMutation({
  args: {
    scopeKey: v.string(),
    now: v.number(),
    reason: v.union(v.literal("not_configured"), v.literal("provider_unconfigured"), v.literal("quota"), v.literal("provider_error"), v.literal("offline"), v.literal("paused")),
    providerName: v.string(),
    recoversAt: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey)).unique();
    if (scope) await ctx.db.patch(scope._id, {
      judgmentDegradedAt: args.now,
      judgmentDegradedReason: args.reason,
      judgmentProviderName: args.providerName,
      judgmentRecoversAt: args.recoversAt,
    });
  },
});

export const adjudicateFinding = internalAction({
  args: { findingPublicId: v.string(), expectedRevision: v.number(), candidate: v.any() },
  handler: async (ctx, args): Promise<{ applied: boolean; mode: "stale" | "skipped" | "budget" | "fallback" | "project" | "operator" }> => {
    const candidate = args.candidate as JudgmentCandidate;
    const input: { scopeKey: string; projectId: Id<"projects">; severity: string; delivery: string } | null =
      await ctx.runQuery(internal.intelligence.judgmentInput, { findingPublicId: args.findingPublicId, expectedRevision: args.expectedRevision });
    if (!input) return { applied: false, mode: "stale" as const };
    const offline = deterministicJudgment(candidate);
    if (!needsManagedAdjudication(candidate, offline)) return { applied: false, mode: "skipped" as const };
    const resolved = await resolveProviders(ctx, input.projectId);
    if (!resolved.judgment) {
      await ctx.runMutation(internal.intelligence.recordJudgmentDegraded, { scopeKey: input.scopeKey, now: Date.now(), reason: "provider_unconfigured", providerName: resolved.settings?.judgmentProvider ?? "none" });
      return { applied: false, mode: "fallback" as const };
    }
    const providerName = `${resolved.judgment.provider}/${resolved.judgment.model}`;
    const budgetNow = Date.now();
    const budget: { claimed: boolean; recoversAt: number } = await ctx.runMutation(internal.intelligence.claimJudgmentBudget, { projectId: input.projectId, now: budgetNow });
    if (!budget.claimed) {
      await ctx.runMutation(internal.intelligence.recordJudgmentDegraded, { scopeKey: input.scopeKey, now: budgetNow, reason: "quota", providerName, recoversAt: budget.recoversAt });
      return { applied: false, mode: "budget" as const };
    }
    let provider: JudgmentProvider;
    try {
      provider = resolved.judgment.provider === "anthropic"
        ? new AnthropicJudgmentProvider(resolved.judgment)
        : new OpenAICompatibleJudgmentProvider(resolved.judgment);
    } catch (error) {
      await ctx.runMutation(internal.intelligence.recordJudgmentDegraded, { scopeKey: input.scopeKey, now: Date.now(), reason: providerFailureReason(error), providerName });
      return { applied: false, mode: "fallback" as const };
    }
    let judged;
    try {
      judged = await judgeCandidate(provider, candidate, AbortSignal.timeout(JUDGMENT_TIMEOUT_MILLIS));
    } catch (error) {
      await ctx.runMutation(internal.intelligence.recordJudgmentDegraded, { scopeKey: input.scopeKey, now: Date.now(), reason: providerFailureReason(error), providerName: provider.name });
      return { applied: false, mode: "fallback" as const };
    }
    if (judged.degraded) {
      await ctx.runMutation(internal.intelligence.recordJudgmentDegraded, { scopeKey: input.scopeKey, now: Date.now(), reason: "provider_error", providerName: provider.name });
      return { applied: false, mode: "fallback" as const };
    }
    const applied: boolean = await ctx.runMutation(internal.intelligence.applyJudgmentVerdict, {
      findingPublicId: args.findingPublicId, expectedRevision: args.expectedRevision, providerName: judged.provider,
      severity: judged.verdict.severity, reason: judged.verdict.explanation, delivery: judged.verdict.delivery, now: Date.now(),
    });
    return { applied, mode: resolved.effective.judgment === "project" ? "project" as const : "operator" as const };
  },
});
