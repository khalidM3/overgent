import { v } from "convex/values";
import { internalAction, internalQuery } from "./_generated/server";
import type { QueryCtx } from "./_generated/server";
import { internal } from "./_generated/api";

async function loadContext(ctx: QueryCtx, args: { tokenHash: string; workstreamPublicId: string }) {
    const device = await ctx.db.query("devices").withIndex("by_token_hash", (q) => q.eq("tokenHash", args.tokenHash)).unique();
    if (!device || device.revokedAt !== undefined) throw new Error("unauthorized");
    const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", args.workstreamPublicId)).unique();
    if (!workstream) throw new Error("not_found");
    const member = await ctx.db.query("members").withIndex("by_project_device", (q) => q.eq("projectId", workstream.projectId).eq("deviceId", device._id)).unique();
    if (!member || member.removedAt !== undefined) throw new Error("forbidden");
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).unique();
    const object = (await ctx.db.query("semanticObjects").withIndex("by_workstream_active", (q) => q.eq("workstreamId", workstream._id).eq("active", true)).take(3)).find((candidate) => candidate.kind === "intent");
    const embedding = object ? await ctx.db.query("semanticEmbeddings").withIndex("by_object", (q) => q.eq("objectId", object._id)).unique() : null;
    if (!scope || !embedding) return { available: false as const, scopeKey: workstream.scopeKey, contextRevision: scope?.contextRevision ?? 0, vector: [] };
    return { available: true as const, scopeKey: workstream.scopeKey, contextRevision: scope.contextRevision, vector: embedding.vector };
}

export const semanticSearchContext = internalQuery({
  args: { tokenHash: v.string(), workstreamPublicId: v.string() },
  handler: loadContext,
});

export const loadCurrentSemanticMatches = internalQuery({
  args: { tokenHash: v.string(), workstreamPublicId: v.string(), scopeKey: v.string(), contextRevision: v.number(), embeddingIds: v.array(v.id("semanticEmbeddings")) },
  handler: async (ctx, args) => {
    const context = await loadContext(ctx, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId });
    if (context.scopeKey !== args.scopeKey || context.contextRevision !== args.contextRevision) return { retry: true, objectIds: [] as string[] };
    const objectIds: string[] = [];
    for (const embeddingId of args.embeddingIds) {
      const embedding = await ctx.db.get(embeddingId);
      if (!embedding || embedding.scopeKey !== args.scopeKey) continue;
      const object = await ctx.db.get(embedding.objectId);
      if (!object || !object.active || object.scopeKey !== args.scopeKey || object.revision !== embedding.contentRevision) continue;
      const workstream = await ctx.db.get(object.workstreamId);
      if (!workstream || workstream.publicId === args.workstreamPublicId || workstream.status === "done") continue;
      objectIds.push(object.publicId);
    }
    return { retry: false, objectIds };
  },
});

export const searchSemantic = internalAction({
  args: { tokenHash: v.string(), workstreamPublicId: v.string(), limit: v.number() },
  handler: async (ctx, args): Promise<{ available: boolean; degraded: boolean; contextRevision: number; objectIds: string[] }> => {
    for (let attempt = 0; attempt < 2; attempt++) {
      const context = await ctx.runQuery(internal.intelligence.semanticSearchContext, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId });
      if (!context.available) {
        await ctx.runMutation(internal.service.recordSemanticHealth, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId, degraded: true, now: Date.now() });
        return { available: false, degraded: true, contextRevision: context.contextRevision, objectIds: [] };
      }
      let results;
      try {
        results = await ctx.vectorSearch("semanticEmbeddings", "by_vector", { vector: context.vector, limit: Math.max(1, Math.min(args.limit, 64)), filter: (q) => q.eq("scopeKey", context.scopeKey) });
      } catch {
        if (attempt === 0) continue;
        await ctx.runMutation(internal.service.recordSemanticHealth, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId, degraded: true, now: Date.now() });
        return { available: false, degraded: true, contextRevision: context.contextRevision, objectIds: [] };
      }
      const embeddingIds = results.filter((result) => result._score >= 0.65).map((result) => result._id);
      const loaded = await ctx.runQuery(internal.intelligence.loadCurrentSemanticMatches, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId, scopeKey: context.scopeKey, contextRevision: context.contextRevision, embeddingIds });
      if (!loaded.retry) {
        await ctx.runMutation(internal.service.recordSemanticHealth, { tokenHash: args.tokenHash, workstreamPublicId: args.workstreamPublicId, degraded: false, now: Date.now() });
        return { available: true, degraded: false, contextRevision: context.contextRevision, objectIds: loaded.objectIds };
      }
    }
    return { available: false, degraded: true, contextRevision: 0, objectIds: [] };
  },
});
