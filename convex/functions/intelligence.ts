import { v } from "convex/values";
import { internalAction, internalMutation, internalQuery } from "./_generated/server";
import type { QueryCtx } from "./_generated/server";
import type { Id } from "./_generated/dataModel";
import { internal } from "./_generated/api";
import { ANTHROPIC_JUDGMENT_MODEL, AnthropicJudgmentProvider, OpenAIEmbeddingProvider, conceptVector, deterministicJudgment, judgeCandidate, needsManagedAdjudication, type JudgmentCandidate } from "@overgent/coordination";

const OPENAI_EMBEDDING_DIMENSIONS = 1024;
const OPENAI_EMBEDDING_MODEL_VERSION = "text-embedding-3-large/1024";
const FALLBACK_EMBEDDING_MODEL_VERSION = "overgent-concepts/v1/1024";
const FOREIGN_EMBEDDING_MIGRATION_BATCH = 100;
const ANTHROPIC_JUDGMENT_PROVIDER = `anthropic/${ANTHROPIC_JUDGMENT_MODEL}`;

type DegradedReason = "not_configured" | "quota" | "provider_error" | "offline" | "paused";

function scopeModelKey(scopeKey: string, modelVersion: string): string {
  return `${scopeKey.length}:${scopeKey}${modelVersion}`;
}

function providerFailureReason(error: unknown): DegradedReason {
  const message = error instanceof Error ? error.message : String(error);
  if (/429|quota|rate.?limit/i.test(message)) return "quota";
  if (/abort|timeout|timed out|network|fetch failed|unavailable|connection/i.test(message)) return "offline";
  return "provider_error";
}

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
    return { publicId: object.publicId, revision: object.revision, text: object.text, scopeKey: object.scopeKey };
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
    reason: v.union(v.literal("not_configured"), v.literal("quota"), v.literal("provider_error"), v.literal("offline"), v.literal("paused")),
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
    const input: { publicId: string; revision: number; text: string; scopeKey: string } | null = await ctx.runQuery(internal.intelligence.semanticEmbeddingInput, args);
    if (!input) return { applied: false, mode: "stale" as const };
    const apiKey = process.env.OPENAI_API_KEY;
    if (!apiKey) {
      const switched: boolean = await ctx.runMutation(internal.intelligence.configureFallbackEmbeddingModel, { scopeKey: input.scopeKey, now: Date.now() });
      if (switched) await ctx.runMutation(internal.intelligence.convergeForeignEmbeddings, { scopeKey: input.scopeKey, modelVersion: FALLBACK_EMBEDDING_MODEL_VERSION });
      await ctx.runMutation(internal.intelligence.recordOpenAIEmbeddingFailure, { scopeKey: input.scopeKey, now: Date.now(), reason: "not_configured", providerName: "openai/text-embedding-3-large" });
      return { applied: false, mode: "fallback" as const };
    }
    try {
      const provider = new OpenAIEmbeddingProvider(apiKey, OPENAI_EMBEDDING_DIMENSIONS);
      const [embedded] = await provider.embed([{ projectId: "internal", repositoryId: input.scopeKey, objectId: input.publicId, revision: input.revision, text: input.text }], AbortSignal.timeout(10_000));
      if (!embedded) throw new Error("openai_embedding_missing");
      const applied: { applied: boolean; modelChanged: boolean } = await ctx.runMutation(internal.intelligence.applyOpenAIEmbedding, {
        semanticObjectPublicId: input.publicId, expectedRevision: input.revision, providerName: provider.name, modelVersion: OPENAI_EMBEDDING_MODEL_VERSION, vector: [...embedded.vector], now: Date.now(),
      });
      if (applied.applied) {
        // Convergence only on an actual provider switch; re-scanning the scope
        // after every routine embed would be an O(N^2) page storm.
        if (applied.modelChanged) await ctx.runMutation(internal.intelligence.convergeForeignEmbeddings, { scopeKey: input.scopeKey, modelVersion: OPENAI_EMBEDDING_MODEL_VERSION });
        await ctx.runMutation(internal.service.refreshSemanticFindings, { scopeKey: input.scopeKey, now: Date.now() });
      }
      return { applied: applied.applied, mode: "openai" as const };
    } catch (error) {
      // The reason a scope degraded is the only thing that can tell an operator
      // whether to wait, add credit, or fix a key. Recording only the boolean
      // made every embedding failure indistinguishable from every other.
      console.error("openai_embedding_failed", { scopeKey: input.scopeKey, reason: error instanceof Error ? error.message : String(error) });
      await ctx.runMutation(internal.intelligence.recordOpenAIEmbeddingFailure, { scopeKey: input.scopeKey, now: Date.now(), reason: providerFailureReason(error), providerName: "openai/text-embedding-3-large" });
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
    reason: v.union(v.literal("not_configured"), v.literal("quota"), v.literal("provider_error"), v.literal("offline"), v.literal("paused")),
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
  handler: async (ctx, args): Promise<{ applied: boolean; mode: "stale" | "skipped" | "budget" | "fallback" | "anthropic" }> => {
    const candidate = args.candidate as JudgmentCandidate;
    const input: { scopeKey: string; projectId: Id<"projects">; severity: string; delivery: string } | null =
      await ctx.runQuery(internal.intelligence.judgmentInput, { findingPublicId: args.findingPublicId, expectedRevision: args.expectedRevision });
    if (!input) return { applied: false, mode: "stale" as const };
    const offline = deterministicJudgment(candidate);
    if (!needsManagedAdjudication(candidate, offline)) return { applied: false, mode: "skipped" as const };
    const apiKey = process.env.ANTHROPIC_API_KEY;
    if (!apiKey) {
      await ctx.runMutation(internal.intelligence.recordJudgmentDegraded, { scopeKey: input.scopeKey, now: Date.now(), reason: "not_configured", providerName: ANTHROPIC_JUDGMENT_PROVIDER });
      return { applied: false, mode: "fallback" as const };
    }
    const budgetNow = Date.now();
    const budget: { claimed: boolean; recoversAt: number } = await ctx.runMutation(internal.intelligence.claimJudgmentBudget, { projectId: input.projectId, now: budgetNow });
    if (!budget.claimed) {
      await ctx.runMutation(internal.intelligence.recordJudgmentDegraded, { scopeKey: input.scopeKey, now: budgetNow, reason: "quota", providerName: ANTHROPIC_JUDGMENT_PROVIDER, recoversAt: budget.recoversAt });
      return { applied: false, mode: "budget" as const };
    }
    let provider;
    try {
      provider = new AnthropicJudgmentProvider(apiKey);
    } catch (error) {
      await ctx.runMutation(internal.intelligence.recordJudgmentDegraded, { scopeKey: input.scopeKey, now: Date.now(), reason: providerFailureReason(error), providerName: ANTHROPIC_JUDGMENT_PROVIDER });
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
    return { applied, mode: "anthropic" as const };
  },
});
