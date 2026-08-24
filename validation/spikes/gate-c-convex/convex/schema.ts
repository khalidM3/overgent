import { defineSchema, defineTable } from "convex/server";
import { v } from "convex/values";

export default defineSchema({
  repositoryScopes: defineTable({
    scopeKey: v.string(),
    projectId: v.string(),
    repositoryId: v.string(),
    contextRevision: v.number(),
    currentManifestId: v.optional(v.id("manifests")),
  }).index("by_scope", ["scopeKey"]),

  events: defineTable({
    eventId: v.string(),
    scopeKey: v.string(),
    deviceId: v.string(),
    sequence: v.number(),
  }).index("by_event_id", ["eventId"]),

  manifests: defineTable({
    manifestId: v.string(),
    scopeKey: v.string(),
    revision: v.number(),
    expectedChunks: v.number(),
    state: v.union(v.literal("staging"), v.literal("active"), v.literal("superseded")),
  }).index("by_manifest_id", ["manifestId"]),

  manifestChunks: defineTable({
    manifestId: v.string(),
    chunkIndex: v.number(),
    paths: v.array(v.string()),
  }).index("by_manifest_chunk", ["manifestId", "chunkIndex"]),

  semanticObjects: defineTable({
    publicId: v.string(),
    projectId: v.string(),
    repositoryId: v.string(),
    scopeKey: v.string(),
    revision: v.number(),
    modelVersion: v.string(),
    active: v.boolean(),
    text: v.string(),
    expiresAt: v.number(),
  }).index("by_public_id", ["publicId"]),

  semanticEmbeddings: defineTable({
    objectId: v.id("semanticObjects"),
    scopeKey: v.string(),
    modelVersion: v.string(),
    contentRevision: v.number(),
    embedding: v.array(v.float64()),
    expiresAt: v.number(),
  })
    .index("by_object_id", ["objectId"])
    .vectorIndex("by_embedding", {
      vectorField: "embedding",
      dimensions: 5,
      filterFields: ["scopeKey"],
    }),

  findings: defineTable({
    scopeKey: v.string(),
    fingerprint: v.string(),
    expiresAt: v.number(),
  }).index("by_scope", ["scopeKey"]),

  contextDeliveries: defineTable({
    scopeKey: v.string(),
    briefId: v.string(),
    expiresAt: v.number(),
  }).index("by_scope", ["scopeKey"]),
});
