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
    // Legacy: written by the plan surface removed in ADR-037. Retained as
    // optional so documents created before that ADR still validate. Never read
    // or written by current code; drop it once no such rows remain.
    planRevision: v.optional(v.number()),
  }).index("by_public_id", ["publicId"]),

  members: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    deviceId: v.id("devices"),
    displayName: v.string(),
    // Absent means the name was seeded from the enrolling device label before
    // ADR-035; those Projects still owe the member an explicit choice.
    displayNameSource: v.optional(v.union(v.literal("device"), v.literal("member"))),
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
    semanticProviderName: v.optional(v.string()),
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
    branch: v.optional(v.string()),
    sessionTitle: v.optional(v.string()),
    safePaths: v.optional(v.array(v.string())),
    subagents: v.optional(v.array(v.object({ alias: v.string(), agentType: v.string(), status: v.string() }))),
    // What this workstream last said about verification of its own work
    // (ADR-045). Absent until it reports a checkpoint that says.
    verificationState: v.optional(v.string()),
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

  // The latest known exported surface per (repository scope, path). A file is
  // stored only while some workspace in the scope keeps reporting it.
  contractFingerprints: defineTable({
    projectId: v.id("projects"),
    scopeKey: v.string(),
    path: v.string(),
    fileContractHash: v.string(),
    symbols: v.array(v.object({
      name: v.string(),
      kind: v.string(),
      signature: v.string(),
      signatureHash: v.string(),
    })),
    changedByWorkstreamPublicId: v.string(),
    revision: v.number(),
    updatedAt: v.number(),
    expiresAt: v.number(),
  })
    .index("by_scope_path", ["scopeKey", "path"])
    .index("by_expiry", ["expiresAt"]),

  // One row per (session workstream, path): what the session read and the
  // contract hash current when it read it.
  sessionReadSets: defineTable({
    projectId: v.id("projects"),
    scopeKey: v.string(),
    workstreamId: v.id("workstreams"),
    workstreamPublicId: v.string(),
    path: v.string(),
    fileContractHashAtRead: v.string(),
    readAt: v.string(),
    updatedAt: v.number(),
    expiresAt: v.number(),
  })
    .index("by_workstream_path", ["workstreamId", "path"])
    .index("by_scope_path", ["scopeKey", "path"])
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
    // Judgment-layer routing decision: next_turn, dashboard, or silent
    // (ADR-045). Absent on rows written before a verdict existed.
    delivery: v.optional(v.string()),
    judgmentProvider: v.optional(v.string()),
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
    .vectorIndex("by_vector", { vectorField: "vector", dimensions: 1024, filterFields: ["scopeKey"] }),

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

  syncCards: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    findingId: v.optional(v.id("findings")),
    title: v.string(),
    summary: v.string(),
    state: v.union(v.literal("open"), v.literal("resolved")),
    revision: v.number(),
    createdByMemberId: v.id("members"),
    createdAt: v.number(),
    updatedAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_project_updated", ["projectId", "updatedAt"]),

  syncComments: defineTable({
    publicId: v.string(),
    syncCardId: v.id("syncCards"),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    body: v.string(),
    createdAt: v.number(),
  })
    .index("by_card_created", ["syncCardId", "createdAt"])
    .index("by_project", ["projectId"]),

  decisions: defineTable({
    publicId: v.string(),
    projectId: v.id("projects"),
    syncCardId: v.optional(v.id("syncCards")),
    summary: v.string(),
    affectedMemberIds: v.array(v.id("members")),
    affectedWorkstreamIds: v.array(v.id("workstreams")),
    revision: v.number(),
    createdByMemberId: v.id("members"),
    createdAt: v.number(),
    updatedAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_project_updated", ["projectId", "updatedAt"]),

  decisionDeliveries: defineTable({
    decisionId: v.id("decisions"),
    workstreamId: v.id("workstreams"),
    decisionRevision: v.number(),
    deliveredAt: v.number(),
    acknowledgedAt: v.optional(v.number()),
  })
    .index("by_decision_workstream", ["decisionId", "workstreamId"])
    .index("by_workstream", ["workstreamId"]),

  sessionMessages: defineTable({
    publicId: v.string(),
    workstreamId: v.id("workstreams"),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    vendor: v.union(v.literal("codex"), v.literal("claude")),
    // "reasoning_summary" and "system" are legacy: hooks never actually supplied
    // them. Retained so pre-ADR-036 rows validate; current code writes only
    // user, assistant, and thinking.
    kind: v.union(v.literal("user"), v.literal("assistant"), v.literal("thinking"), v.literal("reasoning_summary"), v.literal("system")),
    text: v.string(),
    capturedAt: v.number(),
    expiresAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_workstream_captured", ["workstreamId", "capturedAt"])
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
