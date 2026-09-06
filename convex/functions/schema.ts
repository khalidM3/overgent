import { defineSchema, defineTable } from "convex/server";
import { v } from "convex/values";

const role = v.union(v.literal("owner"), v.literal("member"));
const manifestState = v.union(v.literal("staging"), v.literal("active"), v.literal("superseded"));
const degradedReason = v.union(
  v.literal("not_configured"),
  v.literal("provider_unconfigured"),
  v.literal("quota"),
  v.literal("provider_error"),
  v.literal("offline"),
  v.literal("paused"),
);

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

  projectAISettings: defineTable({
    projectId: v.id("projects"),
    revision: v.number(),
    judgmentProvider: v.union(v.literal("anthropic"), v.literal("openai-compatible"), v.literal("none")),
    judgmentModel: v.string(),
    judgmentBaseUrl: v.optional(v.string()),
    judgmentKeyCiphertext: v.optional(v.string()),
    judgmentKeyHint: v.optional(v.string()),
    embeddingProvider: v.union(v.literal("openai"), v.literal("deterministic")),
    embeddingModel: v.string(),
    embeddingDimensions: v.number(),
    embeddingBaseUrl: v.optional(v.string()),
    embeddingKeyCiphertext: v.optional(v.string()),
    embeddingKeyHint: v.optional(v.string()),
    updatedAt: v.number(),
    updatedByMemberId: v.id("members"),
  }).index("by_project", ["projectId"]),

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
    .index("by_creator", ["createdByMemberId"])
    .index("by_expiry", ["expiresAt"])
    .index("by_project", ["projectId"]),

  dashboardTickets: defineTable({
    secretHash: v.string(),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    deviceId: v.id("devices"),
    expiresAt: v.number(),
    usedAt: v.optional(v.number()),
  })
    .index("by_secret_hash", ["secretHash"])
    .index("by_expiry", ["expiresAt"])
    .index("by_member", ["memberId"])
    .index("by_project", ["projectId"]),

  browserSessions: defineTable({
    secretHash: v.string(),
    projectId: v.id("projects"),
    memberId: v.id("members"),
    deviceId: v.optional(v.id("devices")),
    expiresAt: v.number(),
    revokedAt: v.optional(v.number()),
  })
    .index("by_secret_hash", ["secretHash"])
    .index("by_expiry", ["expiresAt"])
    .index("by_project", ["projectId"])
    .index("by_member", ["memberId"]),

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
    .index("by_member", ["memberId"])
    .index("by_device", ["deviceId"]),

  repositoryScopes: defineTable({
    scopeKey: v.string(),
    projectId: v.id("projects"),
    repoFingerprint: v.string(),
    contextRevision: v.number(),
    semanticHealthyAt: v.optional(v.number()),
    semanticDegradedAt: v.optional(v.number()),
    semanticDegradedReason: v.optional(degradedReason),
    semanticProviderName: v.optional(v.string()),
    // The model currently selected for this repository scope. It drives the
    // bounded migration that removes vectors from an incompatible space.
    semanticModelVersion: v.optional(v.string()),
    judgmentDegradedAt: v.optional(v.number()),
    judgmentDegradedReason: v.optional(degradedReason),
    judgmentProviderName: v.optional(v.string()),
    judgmentRecoversAt: v.optional(v.number()),
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
    vendor: v.optional(v.union(v.literal("codex"), v.literal("claude"), v.literal("cursor"))),
    // A workstream conjured to hang a manifest on, rather than one a member or
    // an agent session ever announced. It exists so residual git evidence has
    // an owner (B29) and is not a session: it has no goal, no turn loop, and
    // nothing to say about itself, so presenting it beside real sessions
    // produced a permanent "No goal reported / implementing" row. Cleared the
    // moment an intent or an agent event gives it a real identity.
    origin: v.optional(v.literal("manifest")),
    sessionAlias: v.optional(v.string()),
    agentStatus: v.optional(v.union(v.literal("active"), v.literal("waiting"), v.literal("idle"), v.literal("done"), v.literal("error"))),
    activityKind: v.optional(v.string()),
    currentAction: v.optional(v.string()),
    toolName: v.optional(v.string()),
    branch: v.optional(v.string()),
    sessionTitle: v.optional(v.string()),
    startedAt: v.optional(v.number()),
    endedAt: v.optional(v.number()),
    safePaths: v.optional(v.array(v.string())),
    // The most recent hook-reported mutation paths are attribution evidence
    // for the next Git-observed contract change. Read-tool paths never enter.
    lastWritePaths: v.optional(v.array(v.string())),
    lastWriteAt: v.optional(v.number()),
    // Manifest paths no live agent session claimed. They are the only work
    // evidence a member without an adapter leaves, so they are what lets that
    // member participate in collision detection at all (B29).
    residualPaths: v.optional(v.array(v.string())),
    residualAt: v.optional(v.number()),
    // Canonical declarations are kept independently from the rolling activity
    // summary so a later hook action cannot overwrite the workstream's stated
    // goal or approach.
    intendedOutcome: v.optional(v.string()),
    approachSummary: v.optional(v.string()),
    components: v.optional(v.array(v.string())),
    contracts: v.optional(v.array(v.string())),
    waitingOn: v.optional(v.array(v.string())),
    waitingOnDeclared: v.optional(v.boolean()),
    // Goals this session pursued and moved on from, oldest first. A session is
    // not one task, and keeping only the current goal let `done` accumulate
    // past the goal shown beside it until the two described different work.
    //
    // This is durable state rather than a query over activityEvents, which
    // carries expiresAt and is read newest-60-first: deriving the history from
    // there would make a session's earlier goals disappear as the events aged
    // out, which is worse than not showing them at all.
    priorGoals: v.optional(v.array(v.object({
      title: v.string(),
      intendedOutcome: v.optional(v.string()),
      endedAt: v.string(),
    }))),
    // What fell off the front of that bounded list, so a truncated history is
    // never presented as a whole one.
    priorGoalsDropped: v.optional(v.number()),
    latestCheckpointPassed: v.optional(v.boolean()),
    latestVerification: v.optional(v.array(v.object({
      state: v.union(v.literal("not_run"), v.literal("running"), v.literal("passed"), v.literal("failed"), v.literal("unknown")),
      checkKind: v.string(),
      label: v.string(),
      summary: v.string(),
      affectedComponent: v.optional(v.string()),
      manifestRevision: v.optional(v.number()),
      source: v.union(v.literal("manual"), v.literal("mcp"), v.literal("hook")),
      observedAt: v.optional(v.string()),
    }))),
    subagents: v.optional(v.array(v.object({ alias: v.string(), agentType: v.string(), status: v.string() }))),
    // What this workstream last said about verification of its own work
    // (ADR-045). Absent until it reports a checkpoint that says.
    verificationState: v.optional(v.string()),
    // The strongest read evidence available for this session's vendor
    // (ADR-052). Codex inspects source through the shell, so nothing observes
    // its reads and an empty read set is a coverage gap, not an all-clear.
    readCoverage: v.optional(v.union(v.literal("observed"), v.literal("vendor_inferred"), v.literal("self_declared"), v.literal("none"))),
    updatedAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_scope", ["scopeKey"])
    .index("by_member", ["memberId"])
    // Lets the retention sweep find sessions that went quiet without ever
    // reporting SessionEnd, without scanning every workstream in the project.
    .index("by_status_updated", ["status", "updatedAt"])
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
    .index("by_workstream", ["workstreamId"])
    .index("by_scope_state", ["scopeKey", "state"])
    .index("by_project", ["projectId"])
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
    .index("by_member", ["memberId"])
    .index("by_project", ["projectId"])
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
    .index("by_project", ["projectId"])
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
    // How this read was learned of (ADR-052). Absent on rows written before
    // provenance existed, which came from the observed hook path.
    fidelity: v.optional(v.union(v.literal("observed"), v.literal("vendor_inferred"), v.literal("self_declared"))),
    updatedAt: v.number(),
    expiresAt: v.number(),
  })
    .index("by_workstream_path", ["workstreamId", "path"])
    .index("by_scope_path", ["scopeKey", "path"])
    .index("by_project", ["projectId"])
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
    .index("by_project", ["projectId"])
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
    .index("by_project", ["projectId"])
    .index("by_scope_active", ["scopeKey", "active"])
    .index("by_workstream_active", ["workstreamId", "active"])
    .index("by_expiry", ["expiresAt"]),

  semanticEmbeddings: defineTable({
    objectId: v.id("semanticObjects"),
    scopeKey: v.string(),
    // Convex vector filters support equality and OR, but not AND. Keep the
    // individual fields for inspection/migration and filter searches on this
    // collision-safe composite so both scope and model are mandatory.
    scopeModelKey: v.optional(v.string()),
    providerName: v.string(),
    modelVersion: v.string(),
    contentRevision: v.number(),
    vector: v.array(v.float64()),
    expiresAt: v.number(),
  })
    .index("by_object", ["objectId"])
    .index("by_scope_model", ["scopeKey", "modelVersion"])
    .index("by_expiry", ["expiresAt"])
    .vectorIndex("by_vector", { vectorField: "vector", dimensions: 1024, filterFields: ["scopeKey", "modelVersion", "scopeModelKey"] }),

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
    .index("by_project", ["projectId"])
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
    .index("by_member", ["memberId"])
    .index("by_project", ["projectId"])
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
    .index("by_creator", ["createdByMemberId"])
    .index("by_project", ["projectId"])
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
    .index("by_member", ["memberId"])
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
    .index("by_creator", ["createdByMemberId"])
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
    // Cursor writes no conversation messages today: it publishes no session
    // record this device can read, so nothing reaches the message gate. It is
    // accepted here because the union states what the contract permits, not
    // which vendors happen to exercise it.
    vendor: v.union(v.literal("codex"), v.literal("claude"), v.literal("cursor")),
    // "reasoning_summary" and "system" are legacy: hooks never actually supplied
    // them. Retained so pre-ADR-036 rows validate; current code writes only
    // user, assistant, and thinking.
    kind: v.union(v.literal("user"), v.literal("assistant"), v.literal("thinking"), v.literal("reasoning_summary"), v.literal("system")),
    text: v.string(),
    capturedAt: v.number(),
    expiresAt: v.number(),
  })
    .index("by_public_id", ["publicId"])
    .index("by_member", ["memberId"])
    .index("by_workstream_captured", ["workstreamId", "capturedAt"])
    .index("by_project", ["projectId"])
    .index("by_expiry", ["expiresAt"]),

  deviceCursors: defineTable({
    deviceId: v.id("devices"),
    projectId: v.id("projects"),
    lastAckedSequence: v.number(),
    cursor: v.string(),
  }).index("by_device_project", ["deviceId", "projectId"]).index("by_project", ["projectId"]),

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
