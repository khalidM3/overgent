import { defineSchema, defineTable } from "convex/server";
import { v } from "convex/values";

const role = v.union(v.literal("owner"), v.literal("member"));
const manifestState = v.union(v.literal("staging"), v.literal("active"), v.literal("superseded"));

export default defineSchema({
  projects: defineTable({
    publicId: v.string(),
    label: v.string(),
    status: v.union(v.literal("active"), v.literal("deleting")),
    createdAt: v.number(),
    retentionDays: v.number(),
  }).index("by_public_id", ["publicId"]),

  members: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    deviceId: v.id("devices"),
    displayName: v.string(),
    role,
    joinedAt: v.number(),
    removedAt: v.optional(v.number()),
  })
    .index("by_public_id", ["publicId"])
    .index("by_project", ["projectId"])
    .index("by_device", ["deviceId"])
    .index("by_project_device", ["projectId", "deviceId"]),

  devices: defineTable({
    publicId: v.string(),
    tokenHash: v.string(),
    label: v.string(),
    appVersion: v.string(),
    schemaMinimum: v.number(),
    schemaMaximum: v.number(),
    createdAt: v.number(),
    lastSeenAt: v.optional(v.number()),
    revokedAt: v.optional(v.number()),
  })
    .index("by_public_id", ["publicId"])
    .index("by_token_hash", ["tokenHash"]),

  invites: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    secretHash: v.string(),
    expiresAt: v.number(),
    remainingUses: v.number(),
    revokedAt: v.optional(v.number()),
    createdByMemberId: v.id("members"),
    createdAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_expiry", ["expiresAt"]),

  dashboardTickets: defineTable({
    secretHash: v.string(),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    deviceId: v.id("devices"),
    expiresAt: v.number(),
    usedAt: v.optional(v.number()),
  })
    .index("by_secret_hash", ["secretHash"])
    .index("by_expiry", ["expiresAt"]),

  browserSessions: defineTable({
    secretHash: v.string(),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    deviceId: v.optional(v.id("devices")),
    expiresAt: v.number(),
    revokedAt: v.optional(v.number()),
  })
    .index("by_secret_hash", ["secretHash"])
    .index("by_expiry", ["expiresAt"]),

  workspaces: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    deviceId: v.id("devices"),
    repoFingerprint: v.string(),
    scopeKey: v.string(),
    label: v.string(),
    capabilities: v.any(),
    paused: v.boolean(),
    lastProjectedSequence: v.number(),
    updatedAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_project", ["projectId"])
    .index("by_device", ["deviceId"]),

  repositoryScopes: defineTable({
    scopeKey: v.string(),
    projectId: v.id("projects"),
    repoFingerprint: v.string(),
    contextRevision: v.number(),
    semanticHealthyAt: v.optional(v.number()),
    semanticDegradedAt: v.optional(v.number()),
    updatedAt: v.number(),
  })
    .index("by_scope", ["scopeKey"])
    .index("by_project", ["projectId"])
    .index("by_project_repo", ["projectId", "repoFingerprint"]),

  workstreams: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    workspaceId: v.id("workspaces"),
    scopeKey: v.string(),
    title: v.string(),
    summary: v.string(),
    status: v.union(v.literal("active"), v.literal("idle"), v.literal("done"), v.literal("blocked")),
    revision: v.number(),
    currentManifestId: v.optional(v.id("changeManifests")),
    vendor: v.optional(v.union(v.literal("codex"), v.literal("claude"))),
    sessionAlias: v.optional(v.string()),
    agentStatus: v.optional(v.union(v.literal("active"), v.literal("waiting"), v.literal("idle"), v.literal("done"), v.literal("error"))),
    activityKind: v.optional(v.string()),
    currentAction: v.optional(v.string()),
    toolName: v.optional(v.string()),
    safePaths: v.optional(v.array(v.string())),
    subagents: v.optional(v.array(v.object({ alias: v.string(), agentType: v.string(), status: v.string() }))),
    updatedAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_scope", ["scopeKey"])
    .index("by_project", ["projectId"]),

  changeManifests: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    scopeKey: v.string(),
    workstreamId: v.id("workstreams"),
    revision: v.number(),
    baselineRef: v.string(),
    headRef: v.string(),
    expectedChunks: v.number(),
    contentHash: v.optional(v.string()),
    pathCount: v.number(),
    state: manifestState,
    createdAt: v.number(),
    activatedAt: v.optional(v.number()),
    expiresAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_scope_state", ["scopeKey", "state"])
    .index("by_expiry", ["expiresAt"]),

  changeManifestChunks: defineTable({
    manifestId: v.id("changeManifests"),
    chunkIndex: v.number(),
    entries: v.array(v.object({
      path: v.string(),
      states: v.object({
        baseline: v.optional(v.object({ status: v.string(), oldPath: v.optional(v.string()) })),
        index: v.optional(v.object({ status: v.string(), oldPath: v.optional(v.string()) })),
        worktree: v.optional(v.object({ status: v.string(), oldPath: v.optional(v.string()) })),
      }),
      symbols: v.optional(v.array(v.string())),
      dependencies: v.optional(v.array(v.string())),
    })),
    expiresAt: v.number(),
  })
    .index("by_manifest_chunk", ["manifestId", "chunkIndex"])
    .index("by_expiry", ["expiresAt"]),

  activityEvents: defineTable({
    eventId: v.string(),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    deviceId: v.id("devices"),
    workspacePublicId: v.string(),
    sequence: v.number(),
    observedAt: v.string(),
    receivedAt: v.number(),
    source: v.string(),
    type: v.string(),
    payload: v.any(),
    expiresAt: v.number(),
  })
    .index("by_event_id", ["eventId"])
    .index("by_project_received", ["projectId", "receivedAt"])
    .index("by_expiry", ["expiresAt"]),

  findings: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    scopeKey: v.string(),
    kind: v.string(),
    severity: v.string(),
    confidenceBand: v.string(),
    workstreamPublicIds: v.array(v.string()),
    evidence: v.any(),
    reason: v.string(),
    state: v.string(),
    fingerprint: v.string(),
    engineVersion: v.string(),
    inputRevisions: v.optional(v.any()),
    revision: v.number(),
    firstSeenAt: v.number(),
    lastSeenAt: v.number(),
    expiresAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_fingerprint", ["fingerprint"])
    .index("by_scope", ["scopeKey"])
    .index("by_project_seen", ["projectId", "lastSeenAt"])
    .index("by_expiry", ["expiresAt"]),

  semanticObjects: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    scopeKey: v.string(),
    workstreamId: v.id("workstreams"),
    kind: v.string(),
    text: v.string(),
    source: v.string(),
    fidelity: v.string(),
    tags: v.optional(v.array(v.string())),
    manifestRevision: v.optional(v.number()),
    revision: v.number(),
    active: v.boolean(),
    expiresAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_scope_active", ["scopeKey", "active"])
    .index("by_workstream_active", ["workstreamId", "active"])
    .index("by_expiry", ["expiresAt"]),

  semanticEmbeddings: defineTable({
    objectId: v.id("semanticObjects"),
    scopeKey: v.string(),
    providerName: v.string(),
    modelVersion: v.string(),
    contentRevision: v.number(),
    vector: v.array(v.float64()),
    expiresAt: v.number(),
  })
    .index("by_object", ["objectId"])
    .index("by_scope_model", ["scopeKey", "modelVersion"])
    .index("by_expiry", ["expiresAt"])
    .vectorIndex("by_vector", { vectorField: "vector", dimensions: 32, filterFields: ["scopeKey"] }),

  contextDeliveries: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    workstreamId: v.id("workstreams"),
    contextRevision: v.number(),
    trigger: v.string(),
    itemRefs: v.array(v.string()),
    itemRevisions: v.optional(v.any()),
    requestedBudget: v.number(),
    renderedSize: v.number(),
    deliveredAt: v.number(),
    acknowledgedAt: v.optional(v.number()),
    expiresAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_workstream", ["workstreamId"])
    .index("by_expiry", ["expiresAt"]),

  findingFeedback: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    findingId: v.id("findings"),
    memberId: v.id("members"),
    value: v.union(v.literal("useful"), v.literal("not_related"), v.literal("already_coordinated"), v.literal("missed_severity")),
    engineVersion: v.string(),
    createdAt: v.number(),
    expiresAt: v.number(),
  })
    .index("by_finding_member", ["findingId", "memberId"])
    .index("by_expiry", ["expiresAt"]),

  deviceCursors: defineTable({
    deviceId: v.id("devices"),
    projectId: v.id("projects"),
    lastAckedSequence: v.number(),
    cursor: v.string(),
  }).index("by_device_project", ["deviceId", "projectId"]),

  rateLimits: defineTable({
    key: v.string(),
    route: v.string(),
    windowStartedAt: v.number(),
    count: v.number(),
    expiresAt: v.number(),
  })
    .index("by_key_route", ["key", "route"])
    .index("by_expiry", ["expiresAt"]),
});
