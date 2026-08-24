import { v } from "convex/values";
import {
  action,
  internalMutation,
  internalQuery,
  mutation,
  query,
} from "./_generated/server";
import type { MutationCtx } from "./_generated/server";
import type { Id } from "./_generated/dataModel";
import { internal } from "./_generated/api";

const scopeArgs = {
  scopeKey: v.string(),
  projectId: v.string(),
  repositoryId: v.string(),
};

export const reset = mutation({
  args: {},
  handler: async (ctx) => {
    for (const table of [
      "contextDeliveries",
      "findings",
      "semanticEmbeddings",
      "semanticObjects",
      "manifestChunks",
      "manifests",
      "events",
      "repositoryScopes",
    ] as const) {
      const docs = await ctx.db.query(table).collect();
      for (const doc of docs) await ctx.db.delete(doc._id);
    }
  },
});

export const ensureScope = mutation({
  args: scopeArgs,
  handler: async (ctx, args) => {
    const existing = await ctx.db
      .query("repositoryScopes")
      .withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey))
      .unique();
    if (existing) return existing.contextRevision;
    await ctx.db.insert("repositoryScopes", { ...args, contextRevision: 0 });
    return 0;
  },
});

export const scopeState = query({
  args: { scopeKey: v.string() },
  handler: async (ctx, args) => {
    const scope = await ctx.db
      .query("repositoryScopes")
      .withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey))
      .unique();
    const events = await ctx.db.query("events").collect();
    return {
      contextRevision: scope?.contextRevision ?? -1,
      eventCount: events.filter((event) => event.scopeKey === args.scopeKey).length,
    };
  },
});

export const publishEvent = mutation({
  args: {
    eventId: v.string(),
    scopeKey: v.string(),
    deviceId: v.string(),
    sequence: v.number(),
  },
  handler: async (ctx, args) => {
    const duplicate = await ctx.db
      .query("events")
      .withIndex("by_event_id", (q) => q.eq("eventId", args.eventId))
      .unique();
    if (duplicate) return { disposition: "duplicate" as const };
    await ctx.db.insert("events", args);
    await bumpScope(ctx, args.scopeKey);
    return { disposition: "accepted" as const };
  },
});

export const startManifest = mutation({
  args: {
    manifestId: v.string(),
    scopeKey: v.string(),
    revision: v.number(),
    expectedChunks: v.number(),
  },
  handler: async (ctx, args) => {
    if (args.expectedChunks < 1 || args.expectedChunks > 100) throw new Error("chunk_count_out_of_range");
    const existing = await ctx.db
      .query("manifests")
      .withIndex("by_manifest_id", (q) => q.eq("manifestId", args.manifestId))
      .unique();
    if (existing) throw new Error("manifest_already_exists");
    await ctx.db.insert("manifests", { ...args, state: "staging" });
  },
});

export const addManifestChunk = mutation({
  args: {
    manifestId: v.string(),
    chunkIndex: v.number(),
    paths: v.array(v.string()),
  },
  handler: async (ctx, args) => {
    if (args.paths.length < 1 || args.paths.length > 100) throw new Error("path_count_out_of_range");
    if (args.paths.some((path) => path.length > 512)) throw new Error("path_too_long");
    const manifest = await ctx.db
      .query("manifests")
      .withIndex("by_manifest_id", (q) => q.eq("manifestId", args.manifestId))
      .unique();
    if (!manifest) throw new Error("manifest_not_found");
    if (manifest.state !== "staging") throw new Error("manifest_not_staging");
    if (args.chunkIndex < 0 || args.chunkIndex >= manifest.expectedChunks) {
      throw new Error("chunk_index_out_of_range");
    }
    const duplicate = await ctx.db
      .query("manifestChunks")
      .withIndex("by_manifest_chunk", (q) => q.eq("manifestId", args.manifestId).eq("chunkIndex", args.chunkIndex))
      .unique();
    if (duplicate) throw new Error("duplicate_chunk");
    await ctx.db.insert("manifestChunks", args);
  },
});

export const completeManifest = mutation({
  args: { manifestId: v.string() },
  handler: async (ctx, args) => {
    const manifest = await ctx.db
      .query("manifests")
      .withIndex("by_manifest_id", (q) => q.eq("manifestId", args.manifestId))
      .unique();
    if (!manifest) throw new Error("manifest_not_found");
    const chunks = await ctx.db
      .query("manifestChunks")
      .withIndex("by_manifest_chunk", (q) => q.eq("manifestId", args.manifestId))
      .collect();
    if (chunks.length !== manifest.expectedChunks) throw new Error("manifest_incomplete");
    const indexes = chunks.map((chunk) => chunk.chunkIndex).sort((a, b) => a - b);
    if (indexes.some((chunkIndex, position) => chunkIndex !== position)) {
      throw new Error("manifest_chunk_sequence_invalid");
    }
    const scope = await ctx.db
      .query("repositoryScopes")
      .withIndex("by_scope", (q) => q.eq("scopeKey", manifest.scopeKey))
      .unique();
    if (!scope) throw new Error("scope_not_found");
    if (scope.currentManifestId) {
      const previous = await ctx.db.get(scope.currentManifestId);
      if (previous) await ctx.db.patch(previous._id, { state: "superseded" });
    }
    await ctx.db.patch(manifest._id, { state: "active" });
    await ctx.db.patch(scope._id, {
      currentManifestId: manifest._id,
      contextRevision: scope.contextRevision + 1,
    });
    return chunks.reduce((count, chunk) => count + chunk.paths.length, 0);
  },
});

export const activeManifest = query({
  args: { scopeKey: v.string() },
  handler: async (ctx, args) => {
    const scope = await ctx.db
      .query("repositoryScopes")
      .withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey))
      .unique();
    if (!scope?.currentManifestId) return null;
    const manifest = await ctx.db.get(scope.currentManifestId);
    return manifest?.state === "active" ? manifest : null;
  },
});

export const upsertSemantic = mutation({
  args: {
    publicId: v.string(),
    projectId: v.string(),
    repositoryId: v.string(),
    scopeKey: v.string(),
    revision: v.number(),
    modelVersion: v.string(),
    text: v.string(),
    embedding: v.array(v.float64()),
    expiresAt: v.number(),
  },
  handler: async (ctx, args) => {
    if (args.embedding.length !== 5) throw new Error("embedding_dimension_invalid");
    const existing = await ctx.db
      .query("semanticObjects")
      .withIndex("by_public_id", (q) => q.eq("publicId", args.publicId))
      .unique();
    if (existing && args.revision <= existing.revision) throw new Error("stale_revision");
    const { embedding, ...objectFields } = args;
    let objectId: Id<"semanticObjects">;
    if (existing) {
      if (
        existing.projectId !== args.projectId ||
        existing.repositoryId !== args.repositoryId ||
        existing.scopeKey !== args.scopeKey
      ) {
        throw new Error("semantic_scope_immutable");
      }
      await ctx.db.patch(existing._id, { ...objectFields, active: true });
      objectId = existing._id;
      const vectors = await ctx.db
        .query("semanticEmbeddings")
        .withIndex("by_object_id", (q) => q.eq("objectId", objectId))
        .collect();
      for (const vector of vectors) await ctx.db.delete(vector._id);
    } else {
      objectId = await ctx.db.insert("semanticObjects", { ...objectFields, active: true });
    }
    await ctx.db.insert("semanticEmbeddings", {
      objectId,
      scopeKey: args.scopeKey,
      modelVersion: args.modelVersion,
      contentRevision: args.revision,
      embedding,
      expiresAt: args.expiresAt,
    });
    await bumpScope(ctx, args.scopeKey);
    return objectId;
  },
});

export const deleteSemantic = mutation({
  args: { publicId: v.string() },
  handler: async (ctx, args) => {
    const object = await ctx.db
      .query("semanticObjects")
      .withIndex("by_public_id", (q) => q.eq("publicId", args.publicId))
      .unique();
    if (!object) return false;
    const vectors = await ctx.db
      .query("semanticEmbeddings")
      .withIndex("by_object_id", (q) => q.eq("objectId", object._id))
      .collect();
    for (const vector of vectors) await ctx.db.delete(vector._id);
    await ctx.db.delete(object._id);
    await bumpScope(ctx, object.scopeKey);
    return true;
  },
});

export const semanticState = query({
  args: { publicId: v.string() },
  handler: async (ctx, args) => {
    const object = await ctx.db
      .query("semanticObjects")
      .withIndex("by_public_id", (q) => q.eq("publicId", args.publicId))
      .unique();
    if (!object) return null;
    const vectors = await ctx.db
      .query("semanticEmbeddings")
      .withIndex("by_object_id", (q) => q.eq("objectId", object._id))
      .collect();
    return {
      revision: object.revision,
      modelVersion: object.modelVersion,
      vectorCount: vectors.length,
      vectorModelVersions: vectors.map((vector) => vector.modelVersion),
    };
  },
});

export const semanticSearch = action({
  args: {
    scopeKey: v.string(),
    authorizedProjectId: v.string(),
    authorizedRepositoryId: v.string(),
    embedding: v.array(v.float64()),
    forceRace: v.optional(v.boolean()),
  },
  handler: async (ctx, args): Promise<{ mode: "semantic" | "structural_fallback"; ids: string[]; attempts: number }> => {
    for (let attempt = 1; attempt <= 2; attempt++) {
      const before = await ctx.runQuery(internal.gate.readScopeRevision, { scopeKey: args.scopeKey });
      const candidates = await ctx.vectorSearch("semanticEmbeddings", "by_embedding", {
        vector: args.embedding,
        limit: 16,
        filter: (q) => q.eq("scopeKey", args.scopeKey),
      });
      if (args.forceRace) await ctx.runMutation(internal.gate.bumpForRace, { scopeKey: args.scopeKey });
      const ids = await ctx.runQuery(internal.gate.authorizeAndLoad, {
        ids: candidates.map((candidate) => candidate._id),
        scopeKey: args.scopeKey,
        projectId: args.authorizedProjectId,
        repositoryId: args.authorizedRepositoryId,
      });
      const after = await ctx.runQuery(internal.gate.readScopeRevision, { scopeKey: args.scopeKey });
      if (before === after) return { mode: "semantic", ids, attempts: attempt };
    }
    return { mode: "structural_fallback", ids: [], attempts: 2 };
  },
});

export const readScopeRevision = internalQuery({
  args: { scopeKey: v.string() },
  handler: async (ctx, args) => {
    const scope = await ctx.db
      .query("repositoryScopes")
      .withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey))
      .unique();
    return scope?.contextRevision ?? -1;
  },
});

export const authorizeAndLoad = internalQuery({
  args: {
    ids: v.array(v.id("semanticEmbeddings")),
    scopeKey: v.string(),
    projectId: v.string(),
    repositoryId: v.string(),
  },
  handler: async (ctx, args) => {
    const ids: string[] = [];
    for (const id of args.ids) {
      const vector = await ctx.db.get(id);
      if (!vector || vector.scopeKey !== args.scopeKey) continue;
      const object = await ctx.db.get(vector.objectId);
      if (
        !object ||
        !object.active ||
        object.scopeKey !== args.scopeKey ||
        object.projectId !== args.projectId ||
        object.repositoryId !== args.repositoryId ||
        object.revision !== vector.contentRevision ||
        object.modelVersion !== vector.modelVersion
      ) continue;
      ids.push(object.publicId);
    }
    return ids;
  },
});

export const bumpForRace = internalMutation({
  args: { scopeKey: v.string() },
  handler: async (ctx, args) => bumpScope(ctx, args.scopeKey),
});

export const seedRetentionArtifacts = mutation({
  args: { scopeKey: v.string(), expiresAt: v.number() },
  handler: async (ctx, args) => {
    await ctx.db.insert("findings", {
      scopeKey: args.scopeKey,
      fingerprint: "fnd_fixture",
      expiresAt: args.expiresAt,
    });
    await ctx.db.insert("contextDeliveries", {
      scopeKey: args.scopeKey,
      briefId: "brf_fixture",
      expiresAt: args.expiresAt,
    });
  },
});

export const cleanupExpired = mutation({
  args: { now: v.number() },
  handler: async (ctx, args) => {
    let deleted = 0;
    const expiredObjects = (await ctx.db.query("semanticObjects").collect()).filter(
      (object) => object.expiresAt <= args.now,
    );
    for (const object of expiredObjects) {
      const vectors = await ctx.db
        .query("semanticEmbeddings")
        .withIndex("by_object_id", (q) => q.eq("objectId", object._id))
        .collect();
      for (const vector of vectors) {
        await ctx.db.delete(vector._id);
        deleted++;
      }
      await ctx.db.delete(object._id);
      deleted++;
    }
    for (const table of ["findings", "contextDeliveries"] as const) {
      const expired = (await ctx.db.query(table).collect()).filter(
        (document) => document.expiresAt <= args.now,
      );
      for (const document of expired) {
        await ctx.db.delete(document._id);
        deleted++;
      }
    }
    return deleted;
  },
});

export const deleteScope = mutation({
  args: { scopeKey: v.string() },
  handler: async (ctx, args) => {
    let deleted = 0;
    for (const table of ["contextDeliveries", "findings", "semanticEmbeddings", "semanticObjects"] as const) {
      const docs = await ctx.db.query(table).collect();
      for (const doc of docs) {
        if ("scopeKey" in doc && doc.scopeKey === args.scopeKey) {
          await ctx.db.delete(doc._id);
          deleted++;
        }
      }
    }
    return deleted;
  },
});

export const scopeCounts = query({
  args: { scopeKey: v.string() },
  handler: async (ctx, args) => {
    const counts: Record<string, number> = {};
    for (const table of ["contextDeliveries", "findings", "semanticEmbeddings", "semanticObjects"] as const) {
      const docs = await ctx.db.query(table).collect();
      counts[table] = docs.filter((doc) => "scopeKey" in doc && doc.scopeKey === args.scopeKey).length;
    }
    return counts;
  },
});

async function bumpScope(ctx: MutationCtx, scopeKey: string) {
  const scope = await ctx.db
    .query("repositoryScopes")
    .withIndex("by_scope", (q) => q.eq("scopeKey", scopeKey))
    .unique();
  if (!scope) throw new Error("scope_not_found");
  await ctx.db.patch(scope._id, { contextRevision: scope.contextRevision + 1 });
  return scope.contextRevision + 1;
}
