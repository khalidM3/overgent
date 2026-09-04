import { v } from "convex/values";
import { internalMutation, internalQuery } from "./_generated/server";
import type { MutationCtx, QueryCtx } from "./_generated/server";
import type { Doc, Id } from "./_generated/dataModel";
import { internal } from "./_generated/api";
import { conceptVector, decideDelivery, deterministicJudgment, evaluateWorkstreams, readVerificationState, relationshipForKind, renderBrief, validateSemanticTags, validateSemanticText, INTELLIGENCE_ENGINE_VERSION, PROJECT_HOOK_MCP_CAPABILITIES, vendorCapabilities, SemanticPolicyError, type IntelligenceFinding, type JudgmentCandidate, type JudgmentSeverity, type JudgmentSignalKind, type JudgmentVerdict, type VerificationState, type WorkstreamRecord } from "@overgent/coordination";
import { assertCanonicalManifestOrder, canActivateManifestRevision, contractConfidenceBand, findDependencySatisfaction, manifestContentHash, readCoverageOf, readFidelityOf, readFidelityRank, RETENTION_TABLES, scopeKey, sessionHasGoneQuiet, sha256Hex, SESSION_IDLE_TIMEOUT_MS, SESSION_STOP_TIMEOUT_MS, validateSessionMessageText, ValidationError } from "../src/domain";
import type { ManifestEntry, SupportedVendor } from "../src/domain";
import { deriveScopeSnapshot } from "../src/scope-snapshot";
import { findingTitle } from "../src/finding-title";
import type { ScopeVerificationFact } from "../src/scope-snapshot";

/**
 * Display names for the vendors this deployment accepts, used only when a
 * session has no title of its own yet. A vendor missing from this map falls back
 * to its own identifier rather than to another vendor's name.
 */
const VENDOR_LABELS: Readonly<Record<SupportedVendor, string>> = { codex: "Codex", claude: "Claude Code", cursor: "Cursor" };

const DAY = 86_400_000;
const ACTIVITY_RETENTION = 30 * DAY;
const DELIVERY_RETENTION = 30 * DAY;
const DEFAULT_RETENTION_DAYS = 30;
const CONTRACT_ENGINE_VERSION = "contract-watch/v1";
const DEPENDENCY_ENGINE_VERSION = "dependency-readiness/v1";

type EventInput = {
  schemaVersion: 1;
  eventId: string;
  projectId: string;
  memberId: string;
  deviceId: string;
  workspaceId: string;
  sessionId: string;
  sequence: number;
  observedAt: string;
  sentAt: string;
  source: string;
  type: string;
  payload: Record<string, unknown>;
};

export const createProject = internalMutation({
  args: {
    tokenHash: v.string(),
    projectPublicId: v.string(),
    memberPublicId: v.string(),
    devicePublicId: v.string(),
    label: v.string(),
    deviceLabel: v.string(),
    appVersion: v.string(),
    displayName: v.optional(v.string()),
    now: v.number(),
  },
  handler: async (ctx, args) => {
    await enforceRate(ctx, args.tokenHash, "projects.create", args.now, 5, 60_000);
    let device = await ctx.db.query("devices").withIndex("by_token_hash", (q) => q.eq("tokenHash", args.tokenHash)).unique();
    if (device?.revokedAt !== undefined) fail("credential_revoked");
    if (!device) {
      const id = await ctx.db.insert("devices", {
        publicId: args.devicePublicId,
        tokenHash: args.tokenHash,
        label: args.deviceLabel,
        appVersion: args.appVersion,
        schemaMinimum: 1,
        schemaMaximum: 1,
        createdAt: args.now,
      });
      device = await ctx.db.get(id);
    }
    if (!device) fail("internal_error");
    const projectId = await ctx.db.insert("projects", {
      publicId: args.projectPublicId,
      label: args.label,
      status: "active",
      createdAt: args.now,
      retentionDays: DEFAULT_RETENTION_DAYS,
    });
    await ctx.db.insert("members", {
      publicId: args.memberPublicId,
      projectId,
      deviceId: device._id,
      ...memberIdentity(args.displayName, args.deviceLabel),
      role: "owner",
      joinedAt: args.now,
    });
    return { id: args.projectPublicId, label: args.label };
  },
});

export const createInvite = internalMutation({
  args: {
    tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), invitePublicId: v.string(), secretHash: v.string(),
    expiresAt: v.number(), maxUses: v.number(), now: v.number(),
  },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    if (auth.member.role !== "owner") fail("forbidden");
    await enforceRate(ctx, auth.device.tokenHash, "invites.create", args.now, 20, 60_000);
    await ctx.db.insert("invites", {
      publicId: args.invitePublicId,
      projectId: auth.project._id,
      secretHash: args.secretHash,
      expiresAt: args.expiresAt,
      remainingUses: args.maxUses,
      createdByMemberId: auth.member._id,
      createdAt: args.now,
    });
    return { projectLabel: auth.project.label };
  },
});

export const enroll = internalMutation({
  args: {
    rateKey: v.string(), invitePublicId: v.string(), inviteSecretHash: v.string(), devicePublicId: v.string(),
    memberPublicId: v.string(), deviceTokenHash: v.string(), dashboardTicketHash: v.string(), deviceLabel: v.string(), displayName: v.optional(v.string()),
    appVersion: v.string(), schemaMinimum: v.number(), schemaMaximum: v.number(), now: v.number(), ticketExpiresAt: v.number(),
  },
  handler: async (ctx, args) => {
    await enforceRate(ctx, args.rateKey, "enrollments.create", args.now, 12, 60_000);
    if (args.schemaMinimum > 1 || args.schemaMaximum < 1) fail("schema_version_unsupported");
    const invite = await ctx.db.query("invites").withIndex("by_public_id", (q) => q.eq("publicId", args.invitePublicId)).unique();
    if (!invite || invite.secretHash !== args.inviteSecretHash) fail("invite_invalid");
    if (invite.revokedAt !== undefined) fail("invite_revoked");
    if (invite.expiresAt <= args.now) fail("invite_expired");
    if (invite.remainingUses < 1) fail("invite_consumed");
    const deviceId = await ctx.db.insert("devices", {
      publicId: args.devicePublicId,
      tokenHash: args.deviceTokenHash,
      label: args.deviceLabel,
      appVersion: args.appVersion,
      schemaMinimum: args.schemaMinimum,
      schemaMaximum: args.schemaMaximum,
      createdAt: args.now,
    });
    const memberId = await ctx.db.insert("members", {
      publicId: args.memberPublicId,
      projectId: invite.projectId,
      deviceId,
      ...memberIdentity(args.displayName, args.deviceLabel),
      role: "member",
      joinedAt: args.now,
    });
    await ctx.db.insert("dashboardTickets", {
      secretHash: args.dashboardTicketHash,
      projectId: invite.projectId,
      memberId,
      deviceId,
      expiresAt: args.ticketExpiresAt,
    });
    await ctx.db.patch(invite._id, { remainingUses: invite.remainingUses - 1 });
    return true;
  },
});

export const issueDashboardTicket = internalMutation({
  args: {
    tokenHash: v.string(), projectPublicId: v.string(), ticketHash: v.string(),
    now: v.number(), ticketExpiresAt: v.number(),
  },
  handler: async (ctx, args) => {
    const auth = await requireProjectRole(ctx, args.tokenHash, args.projectPublicId);
    await enforceRate(ctx, args.tokenHash, "dashboard.issue", args.now, 20, 60_000);
    await ctx.db.insert("dashboardTickets", {
      secretHash: args.ticketHash,
      projectId: auth.project._id,
      memberId: auth.member._id,
      deviceId: auth.device._id,
      expiresAt: args.ticketExpiresAt,
    });
    return true;
  },
});

export const exchangeDashboardTicket = internalMutation({
  args: { rateKey: v.string(), ticketHash: v.string(), sessionHash: v.string(), now: v.number(), sessionExpiresAt: v.number() },
  handler: async (ctx, args) => {
    await enforceRate(ctx, args.rateKey, "dashboard.exchange", args.now, 20, 60_000);
    const ticket = await ctx.db.query("dashboardTickets").withIndex("by_secret_hash", (q) => q.eq("secretHash", args.ticketHash)).unique();
    if (!ticket) fail("ticket_invalid");
    if (ticket.usedAt !== undefined) fail("ticket_consumed");
    if (ticket.expiresAt <= args.now) fail("ticket_expired");
    const device = await ctx.db.get(ticket.deviceId);
    const member = await ctx.db.get(ticket.memberId);
    if (!device || device.revokedAt !== undefined || !member || member.removedAt !== undefined) fail("unauthorized");
    await ctx.db.patch(ticket._id, { usedAt: args.now });
    await ctx.db.insert("browserSessions", {
      secretHash: args.sessionHash,
      projectId: ticket.projectId,
      memberId: ticket.memberId,
      deviceId: ticket.deviceId,
      expiresAt: args.sessionExpiresAt,
    });
    return true;
  },
});

/**
 * Join another Project with the device credential this Mac already has.
 *
 * Deliberately not `enroll` with a branch. Enrolling mints a device, and every
 * guard that protects a first enrollment is about a caller with no identity;
 * here the caller already has one, so the questions are different: is this
 * credential still good, is the invite still good, and is this device already
 * in that Project. Sharing a handler would have made the difference invisible.
 */
export const joinProjectAsDevice = internalMutation({
  args: {
    tokenHash: v.string(), rateKey: v.string(), invitePublicId: v.string(), inviteSecretHash: v.string(),
    memberPublicId: v.string(), dashboardTicketHash: v.string(), deviceLabel: v.string(),
    displayName: v.optional(v.string()), schemaMinimum: v.number(), schemaMaximum: v.number(),
    now: v.number(), ticketExpiresAt: v.number(),
  },
  handler: async (ctx, args) => {
    await enforceRate(ctx, args.rateKey, "memberships.create", args.now, 12, 60_000);
    if (args.schemaMinimum > 1 || args.schemaMaximum < 1) fail("schema_version_unsupported");
    const device = await requireDevice(ctx, args.tokenHash);
    const invite = await ctx.db.query("invites").withIndex("by_public_id", (q) => q.eq("publicId", args.invitePublicId)).unique();
    if (!invite || invite.secretHash !== args.inviteSecretHash) fail("invite_invalid");
    if (invite.revokedAt !== undefined) fail("invite_revoked");
    if (invite.expiresAt <= args.now) fail("invite_expired");
    if (invite.remainingUses < 1) fail("invite_consumed");
    const project = await ctx.db.get(invite.projectId);
    if (!project || project.status !== "active") fail("not_found");
    // Redeeming twice must not burn a use or create a second membership row,
    // and it must not read as an error the member can do anything about.
    const existing = (await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", device._id)).collect())
      .find((member) => member.projectId === invite.projectId && member.removedAt === undefined);
    if (existing) fail("already_member");
    const memberId = await ctx.db.insert("members", {
      publicId: args.memberPublicId,
      projectId: invite.projectId,
      deviceId: device._id,
      ...memberIdentity(args.displayName, args.deviceLabel),
      role: "member",
      joinedAt: args.now,
    });
    await ctx.db.insert("dashboardTickets", {
      secretHash: args.dashboardTicketHash,
      projectId: invite.projectId,
      memberId,
      deviceId: device._id,
      expiresAt: args.ticketExpiresAt,
    });
    await ctx.db.patch(invite._id, { remainingUses: invite.remainingUses - 1 });
    return project.publicId;
  },
});

export const dashboardSession = internalQuery({
  args: { sessionHash: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireBrowserSession(ctx, args.sessionHash, args.now);
    const semantic = await projectSemanticStatus(ctx, auth.project._id);
    return {
      memberId: auth.member.publicId,
      memberName: auth.member.displayName,
      // Absent source means the name is still the enrolling device label, so the
      // dashboard must ask the member to choose their own before it is shown as
      // live-work identity.
      memberNameSource: auth.member.displayNameSource ?? "device",
      projects: [{ id: auth.project.publicId, name: auth.project.label, repositoryLabel: "Project repositories", semanticStatus: semantic.status, semanticMode: semantic.mode }],
      selectedProjectId: auth.project.publicId,
    };
  },
});

export const dashboardSnapshot = internalQuery({
  args: { sessionHash: v.string(), projectPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireBrowserSession(ctx, args.sessionHash, args.now);
    if (auth.project.publicId !== args.projectPublicId) fail("forbidden");
    const workspaces = await ctx.db.query("workspaces").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(101);
    if (workspaces.length > 100) fail("page_too_large");
    // Pausing writes to the paused member's own device and stops that device
    // publishing; nobody can pause sharing on anyone else's machine. Reporting
    // the Project-wide value therefore told a member their sharing had stopped
    // when a teammate had paused theirs, and offered them a Resume control that
    // could only ever act on their own workspaces (ADR-061).
    const ownWorkspacePaused = workspaces.some((workspace) => workspace.memberId === auth.member._id && workspace.paused);
    const members = await ctx.db.query("members").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).collect();
    const memberById = new Map(members.map((member) => [member._id, member]));
    const projectWorkstreams = await ctx.db.query("workstreams").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).collect();
    const activityDocs = await ctx.db.query("activityEvents").withIndex("by_project_received", (q) => q.eq("projectId", auth.project._id)).order("desc").take(60);
    const findingDocs = await ctx.db.query("findings").withIndex("by_project_seen", (q) => q.eq("projectId", auth.project._id)).take(100);
    const contractDocs = await ctx.db.query("contractFingerprints").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    if (contractDocs.length > 500) fail("page_too_large");
    const decisionDocs = await ctx.db.query("decisions").withIndex("by_project_updated", (q) => q.eq("projectId", auth.project._id)).order("desc").take(100);
    const deliveryDocs = await ctx.db.query("contextDeliveries").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).order("desc").take(100);
    const coordinationSummary = new Map<string, string>([
      ...findingDocs.map((finding) => [finding.publicId, finding.reason] as const),
      ...decisionDocs.map((decision) => [decision.publicId, decision.summary] as const),
    ]);
    const workstreams = [];
    const devices: Array<{ id: string; label: string; platform: string; status: string; lastSeen: string }> = [];
    let contextRevision = 0;
    let semanticStatus: "enabled" | "degraded" = "enabled";
    let semanticMode: "offline_fallback" | "managed_openai" | "managed_degraded" = "offline_fallback";
    for (const stream of projectWorkstreams) {
      const workspace = workspaces.find((candidate) => candidate._id === stream.workspaceId);
      if (!workspace) continue;
      const member = memberById.get(workspace.memberId);
      const device = await ctx.db.get(workspace.deviceId);
      const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", workspace.scopeKey)).unique();
      contextRevision = Math.max(contextRevision, scope?.contextRevision ?? 0);
      if (scope?.semanticProviderName?.startsWith("openai/")) semanticMode = "managed_openai";
      if ((scope?.semanticDegradedAt ?? 0) > (scope?.semanticHealthyAt ?? 0)) {
        semanticStatus = "degraded";
        semanticMode = "managed_degraded";
      }
      let pathCount = 0;
      let paths: string[] = [];
      let manifestRevision = 0;
      if (stream?.currentManifestId) {
        const manifest = await ctx.db.get(stream.currentManifestId);
        if (manifest) {
          pathCount = manifest.pathCount;
          manifestRevision = manifest.revision;
          const chunks = await ctx.db.query("changeManifestChunks").withIndex("by_manifest_chunk", (q) => q.eq("manifestId", manifest._id)).take(2);
          paths = chunks.flatMap((chunk) => chunk.entries.map((entry) => entry.path)).slice(0, 3);
        }
      }
      if (stream.safePaths) {
        paths = stream.safePaths.slice(0, 3);
        pathCount = stream.safePaths.length;
      }
      const lastSeenAt = stream.vendor ? stream.updatedAt : device?.lastSeenAt ?? 0;
      const presence = workspace.paused ? "paused" : stream.agentStatus === "done" ? "offline" : stream.agentStatus === "waiting" || stream.agentStatus === "idle" ? "idle" : args.now - lastSeenAt <= 35_000 ? "online" : args.now - lastSeenAt <= 120_000 ? "idle" : "offline";
      const sessionActivity = stream.vendor ? activityDocs
        .filter((event) => event.type === "agent.activity_reported" && String((event.payload as Record<string, unknown>).workstreamId ?? "") === stream.publicId)
        .slice(0, 12)
        .map((event) => {
          const payload = event.payload as Record<string, unknown>;
          return {
            id: event.eventId,
            at: relativeLabel(args.now, event.receivedAt),
            occurredAt: Number.isNaN(Date.parse(event.observedAt)) ? new Date(event.receivedAt).toISOString() : new Date(event.observedAt).toISOString(),
            kind: String(payload.kind ?? "Activity"),
            status: dashboardAgentStatus(payload.status),
            action: String(payload.action ?? "Reported agent activity"),
            ...(typeof payload.tool === "string" ? { tool: payload.tool } : {}),
            paths: stringValues(payload.paths).slice(0, 3),
          };
        }) : [];
      const coordination = stream.vendor ? deliveryDocs
        .filter((delivery) => delivery.workstreamId === stream._id && delivery.itemRefs.length > 0)
        .sort((left, right) => left.deliveredAt - right.deliveredAt)
        .slice(-12)
        .map((delivery) => {
          const firstSummary = delivery.itemRefs.map((itemRef) => coordinationSummary.get(itemRef)).find((summary) => summary !== undefined);
          const remaining = Math.max(0, delivery.itemRefs.length - 1);
          const summary = firstSummary
            ? `${firstSummary}${remaining > 0 ? ` + ${remaining} more.` : ""}`.slice(0, 500)
            : `${delivery.itemRefs.length} relevant coordination ${delivery.itemRefs.length === 1 ? "item" : "items"} routed to this session.`;
          return {
            id: delivery.publicId,
            routedAt: new Date(delivery.deliveredAt).toISOString(),
            ...(delivery.acknowledgedAt === undefined ? {} : { acknowledgedAt: new Date(delivery.acknowledgedAt).toISOString() }),
            summary,
            itemCount: delivery.itemRefs.length,
            trigger: delivery.trigger,
          };
        }) : [];
      const scopeSnapshot = deriveScopeSnapshot({
        revision: stream.revision,
        workstreamStatus: stream.status,
        ...(stream.agentStatus === undefined ? {} : { agentStatus: stream.agentStatus }),
        ...(stream.vendor === undefined ? {} : { vendor: stream.vendor }),
        declared: {
          ...(stream.intendedOutcome === undefined ? {} : { intendedOutcome: stream.intendedOutcome }),
          ...(stream.approachSummary === undefined ? {} : { approachSummary: stream.approachSummary }),
          ...(stream.components === undefined ? {} : { components: stream.components }),
          ...(stream.contracts === undefined ? {} : { contracts: stream.contracts }),
          ...(stream.waitingOnDeclared ? { waitingOn: stream.waitingOn ?? [] } : {}),
        },
        observed: {
          ...(stream.currentAction === undefined ? {} : { currentAction: stream.currentAction }),
          writes: paths,
          writeCount: pathCount,
          contractPaths: contractDocs.filter((contract) => contract.changedByWorkstreamPublicId === stream.publicId).map((contract) => contract.path),
          subagents: (stream.subagents ?? []).map((subagent) => ({ agentType: subagent.agentType, status: subagent.status })),
          verification: stream.latestVerification ?? [],
        },
        ...(stream.priorGoals === undefined ? {} : { priorGoals: stream.priorGoals }),
        ...(stream.priorGoalsDropped === undefined ? {} : { priorGoalsDropped: stream.priorGoalsDropped }),
        ...(stream.sessionTitle === undefined ? {} : { fallbackDerivedTitle: stream.sessionTitle }),
      });
      workstreams.push({
        id: stream.publicId, memberName: member?.displayName ?? "Project member", initials: initials(member?.displayName ?? "PM"),
        title: stream.title, outcome: stream.currentAction ?? stream.summary, presence, fidelity: stream.vendor ? "hook" : "manual", updatedLabel: relativeLabel(args.now, stream.updatedAt),
        // The label is what a row prints; the timestamp is ground truth for
        // sorting and for clocks that count from a real moment (§8.2).
        updatedAt: new Date(stream.updatedAt).toISOString(),
        ...(stream.vendor ? { agent: { vendor: stream.vendor, sessionAlias: stream.sessionAlias, status: stream.agentStatus, tool: stream.toolName, ...(stream.branch ? { branch: stream.branch } : {}), ...(stream.sessionTitle ? { sessionTitle: stream.sessionTitle } : {}), ...(stream.startedAt === undefined ? {} : { startedAt: new Date(stream.startedAt).toISOString() }), ...(stream.endedAt === undefined ? {} : { endedAt: new Date(stream.endedAt).toISOString() }), capabilities: { ...vendorCapabilities(stream.vendor), observeReadSet: readCoverageOf(stream.readCoverage) ?? "none" }, subagents: stream.subagents ?? [], activity: sessionActivity, coordination } } : {}),
        pathCount, paths, scopeSnapshot,
        ...(stream.components?.length ? { components: stream.components.slice(0, 16) } : {}),
        ...(stream.contracts?.length ? { contracts: stream.contracts.slice(0, 16) } : {}), ...(pathCount >= 1000 ? { largeChange: { pathCount, summary: "Broad metadata-only change; inspect evidence before inferring severity.", revision: manifestRevision } } : {}),
      });
      if (device && !devices.some((candidate) => candidate.id === device.publicId)) devices.push({ id: device.publicId, label: device.label, platform: device.appVersion, status: presence, lastSeen: relativeLabel(args.now, device.lastSeenAt ?? 0) });
    }
    // Who each session belongs to, so a finding can be titled with people
    // rather than with identifiers.
    const nameByWorkstream = new Map(workstreams.map((stream) => [stream.id, stream.memberName]));
    const findings = findingDocs.map((finding) => {
      const evidence = finding.evidence as Array<{
        kind: string; summary: string; source: string; subject?: string;
        contract?: { path?: string; changedSymbols?: Array<{ name: string; oldSignature?: string; newSignature?: string }>; changedByWorkstreamId?: string; readAt?: string; changedAt?: string };
      }>;
      // Prefer the symbol that actually moved: "a version of Refresh" is the
      // fact the reader acts on, where the file it lives in is only where to
      // look. The path is the fallback, and no subject at all still yields a
      // sentence rather than a category.
      const carrier = evidence.find((item) => item.subject || item.contract);
      const subject = carrier?.subject
        ?? carrier?.contract?.changedSymbols?.[0]?.name
        ?? carrier?.contract?.path;
      // A stale assumption is routed only to the session that read the
      // contract, so the session that moved it has to be recovered from the
      // evidence before the sentence can name both sides.
      const changedBy = evidence.find((item) => item.contract?.changedByWorkstreamId)?.contract?.changedByWorkstreamId;
      return {
      id: finding.publicId, kind: dashboardFindingKind(finding.kind), severity: finding.severity, confidence: finding.confidenceBand, state: dashboardFindingState(finding.state),
      title: findingTitle({
        kind: dashboardFindingKind(finding.kind),
        actors: finding.workstreamPublicIds.map((id: string) => nameByWorkstream.get(id) ?? "").filter(Boolean),
        ...(changedBy && nameByWorkstream.get(changedBy) ? { counterpart: nameByWorkstream.get(changedBy)! } : {}),
        ...(subject ? { subject } : {}),
      }), reason: finding.reason, workstreamIds: finding.workstreamPublicIds,
      // The structured contract survives the projection: the changed symbols
      // with their old and new signatures are the one artifact that shows the
      // exact point of divergence, and flattening them to prose threw away
      // the most persuasive thing the engine produces.
      evidence: evidence.map((item) => ({
        kind: dashboardEvidenceKind(item.kind), label: item.summary, source: dashboardEvidenceSource(item.source),
        ...(item.subject ? { subject: item.subject } : {}),
        ...(item.contract ? { contract: item.contract } : {}),
      })),
      firstSeen: relativeLabel(args.now, finding.firstSeenAt), lastSeen: relativeLabel(args.now, finding.lastSeenAt),
      firstSeenAt: new Date(finding.firstSeenAt).toISOString(), lastSeenAt: new Date(finding.lastSeenAt).toISOString(),
      // Where the judgment layer routed this finding (ADR-045/046). The
      // workroom uses it to decide what reaches "Needs you" rather than
      // re-deriving a weaker version from ownership alone.
      ...(finding.delivery ? { delivery: finding.delivery } : {}),
      };
    });
    const activity = activityDocs.slice(0, 20).map((event) => ({ id: event.eventId, at: relativeLabel(args.now, event.receivedAt), actor: memberById.get(event.memberId)?.displayName ?? "Project member", kind: activityKind(event.type), summary: activitySummary(event.type, event.payload), fidelity: dashboardFidelity(event.source) }));
    return { project: { id: auth.project.publicId, name: auth.project.label, repositoryLabel: "Project repositories", semanticStatus, semanticMode }, contextRevision, synchronizedAt: "just now", workstreams, findings, activity, devices, workspacePaused: ownWorkspacePaused };
  },
});

export const recordSemanticHealth = internalMutation({
  args: {
    tokenHash: v.string(), workstreamPublicId: v.string(), degraded: v.boolean(), now: v.number(),
    reason: v.optional(v.union(v.literal("not_configured"), v.literal("quota"), v.literal("provider_error"), v.literal("offline"), v.literal("paused"))),
  },
  handler: async (ctx, args) => {
    const device = await requireDevice(ctx, args.tokenHash);
    const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", args.workstreamPublicId)).unique();
    if (!workstream) fail("not_found");
    await requireMembership(ctx, device._id, workstream.projectId);
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).unique();
    if (!scope) fail("not_found");
    // A degraded flag without a cause is the state the status banner cannot
    // render; every site that knows why must say so (B34/B37).
    await ctx.db.patch(scope._id, args.degraded
      ? { semanticDegradedAt: args.now, semanticDegradedReason: args.reason ?? "provider_error" }
      : { semanticHealthyAt: args.now, semanticDegradedReason: undefined });
    return true;
  },
});

export const refreshSemanticFindings = internalMutation({
  args: { scopeKey: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", args.scopeKey)).unique();
    if (!scope) return false;
    const project = await ctx.db.get(scope.projectId);
    if (!project || project.status !== "active") return false;
    await bumpScope(ctx, args.scopeKey, args.now);
    await recomputeSemanticFindings(ctx, project, args.scopeKey, args.now);
    return true;
  },
});

export const bootstrap = internalQuery({
  args: { tokenHash: v.string() },
  handler: async (ctx, args) => {
    const device = await requireDevice(ctx, args.tokenHash);
    const members = (await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", device._id)).collect())
      .filter((member) => member.removedAt === undefined);
    const projects = [];
    const cursors: Record<string, string> = {};
    for (const member of members) {
      const project = await ctx.db.get(member.projectId);
      if (!project || project.status !== "active") continue;
      projects.push({ id: project.publicId, label: project.label });
      const cursor = await ctx.db.query("deviceCursors")
        .withIndex("by_device_project", (q) => q.eq("deviceId", device._id).eq("projectId", project._id)).unique();
      cursors[project.publicId] = cursor?.cursor ?? "seq:0";
    }
    return {
      deviceId: device.publicId,
      schemaMinimum: 1,
      schemaMaximum: 1,
      projects: projects.sort((a, b) => a.id.localeCompare(b.id)),
      cursors,
    };
  },
});

export const heartbeat = internalMutation({
  args: { tokenHash: v.string(), workspacePublicId: v.string(), state: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const device = await requireDevice(ctx, args.tokenHash);
    await enforceRate(ctx, args.tokenHash, "presence.heartbeat", args.now, 12, 60_000);
    const workspace = await ctx.db.query("workspaces").withIndex("by_public_id", (q) => q.eq("publicId", args.workspacePublicId)).unique();
    if (!workspace || workspace.deviceId !== device._id) fail("forbidden");
    await ctx.db.patch(device._id, { lastSeenAt: args.now });
    const paused = args.state === "paused";
    if (workspace.paused !== paused) {
      await ctx.db.patch(workspace._id, { paused, updatedAt: args.now });
      await bumpScope(ctx, workspace.scopeKey, args.now);
    }
    return true;
  },
});

export const publishEvents = internalMutation({
  args: { tokenHash: v.string(), events: v.array(v.any()), now: v.number() },
  handler: async (ctx, args) => {
    const device = await requireDevice(ctx, args.tokenHash);
    await enforceRate(ctx, args.tokenHash, "events.batch", args.now, 120, 60_000);
    const acceptedEventIds: string[] = [];
    const projects = new Map<Id<"projects">, number>();
    for (const rawEvent of args.events) {
      const event = rawEvent as EventInput;
      if (event.deviceId !== device.publicId) fail("forbidden");
      const project = await ctx.db.query("projects").withIndex("by_public_id", (q) => q.eq("publicId", event.projectId)).unique();
      if (!project || project.status !== "active") fail("forbidden");
      // The frozen bootstrap/enrollment response does not disclose memberId.
      // Derive the canonical member from authenticated device + Project and
      // treat the envelope memberId as untrusted metadata only.
      const member = await ctx.db.query("members").withIndex("by_project_device", (q) =>
        q.eq("projectId", project._id).eq("deviceId", device._id)).unique();
      if (!member || member.removedAt !== undefined) fail("forbidden");
      const duplicate = await ctx.db.query("activityEvents").withIndex("by_event_id", (q) => q.eq("eventId", event.eventId)).unique();
      if (duplicate) {
        if (duplicate.deviceId !== device._id || duplicate.projectId !== project._id) fail("event_id_conflict");
        acceptedEventIds.push(event.eventId);
        projects.set(project._id, Math.max(projects.get(project._id) ?? 0, event.sequence));
        continue;
      }
      await applyProjection(ctx, event, project, member, device, args.now);
      const retainedPayload = event.type === "agent.conversation_shared" ? {
        messageId: String(event.payload.messageId), workstreamId: String(event.payload.workstreamId), kind: String(event.payload.kind),
      } : event.payload;
      await ctx.db.insert("activityEvents", {
        eventId: event.eventId,
        projectId: project._id,
        memberId: member._id,
        deviceId: device._id,
        workspacePublicId: event.workspaceId,
        sequence: event.sequence,
        observedAt: event.observedAt,
        receivedAt: args.now,
        source: event.source,
        type: event.type,
        payload: retainedPayload,
        expiresAt: args.now + ACTIVITY_RETENTION,
      });
      acceptedEventIds.push(event.eventId);
      projects.set(project._id, Math.max(projects.get(project._id) ?? 0, event.sequence));
    }
    let cursor = "seq:0";
    for (const [projectId, sequence] of projects) {
      const current = await ctx.db.query("deviceCursors")
        .withIndex("by_device_project", (q) => q.eq("deviceId", device._id).eq("projectId", projectId)).unique();
      const acknowledged = Math.max(current?.lastAckedSequence ?? 0, sequence);
      cursor = `seq:${acknowledged}`;
      if (current) await ctx.db.patch(current._id, { lastAckedSequence: acknowledged, cursor });
      else await ctx.db.insert("deviceCursors", { deviceId: device._id, projectId, lastAckedSequence: acknowledged, cursor });
    }
    return { acceptedEventIds, cursor };
  },
});

export const projectChanges = internalQuery({
  args: { tokenHash: v.string(), projectPublicId: v.string(), after: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireProjectRole(ctx, args.tokenHash, args.projectPublicId);
    const findings = await ctx.db.query("findings")
      .withIndex("by_project_seen", (q) => q.eq("projectId", auth.project._id).gt("lastSeenAt", args.after))
      .take(101);
    const decisions = await ctx.db.query("decisions").withIndex("by_project_updated", (q) => q.eq("projectId", auth.project._id).gt("updatedAt", args.after)).take(101);
    if (findings.length > 100 || decisions.length > 100 || findings.length + decisions.length > 100) fail("page_too_large");
    const items = [...findings.map(findingContract), ...await Promise.all(decisions.map((decision) => decisionContract(ctx, decision)))];
    const cursorValue = Math.max(findings.reduce((latest, finding) => Math.max(latest, finding.lastSeenAt), args.after), decisions.reduce((latest, decision) => Math.max(latest, decision.updatedAt), args.after));
    return { items, cursor: `time:${cursorValue}` };
  },
});

export const createBrief = internalMutation({
  args: {
    tokenHash: v.string(), workstreamPublicId: v.string(), trigger: v.string(), requestedBudget: v.number(),
    briefPublicId: v.string(), semanticObjectIds: v.array(v.string()), semanticDegraded: v.boolean(), semanticContextRevision: v.number(), now: v.number(),
  },
  handler: async (ctx, args) => {
    const device = await requireDevice(ctx, args.tokenHash);
    const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", args.workstreamPublicId)).unique();
    if (!workstream) fail("not_found");
    await requireMembership(ctx, device._id, workstream.projectId);
    const project = await ctx.db.get(workstream.projectId);
    const workspace = await ctx.db.get(workstream.workspaceId);
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).unique();
    if (!project || !workspace || !scope) fail("not_found");
    const scopedFindings = await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).take(257);
    if (scopedFindings.length > 256) fail("finding_scope_too_large");
    const findings = scopedFindings
      .filter((finding) => finding.state === "open" && finding.workstreamPublicIds.includes(workstream.publicId))
      .sort((a, b) => severityRank(b.severity) - severityRank(a.severity) || a.publicId.localeCompare(b.publicId));
    const rendered = renderBrief(workstream.publicId, findings.map((finding) => ({
      projectId: project.publicId,
      repositoryId: workspace.repoFingerprint,
      id: finding.publicId,
      kind: finding.kind as "direct_collision" | "likely_collision" | "redundant_work" | "shared_dependency" | "assumption_conflict" | "downstream_impact" | "stale_assumption" | "dependency_ready",
      severity: finding.severity as "low" | "medium" | "high" | "critical",
      confidenceBand: finding.confidenceBand as "deterministic" | "high" | "medium" | "low",
      workstreamIds: finding.workstreamPublicIds,
      evidence: finding.evidence,
      reason: finding.reason,
      revision: finding.revision,
      priority: severityRank(finding.severity) * 25,
      ...(finding.delivery ? { delivery: finding.delivery as "next_turn" | "dashboard" | "silent" } : {}),
    })), args.requestedBudget);
    const items = [...rendered.items];
    let renderedSize = rendered.renderedSize;
    let truncated = rendered.truncated;
    const decisionRows = await ctx.db.query("decisions").withIndex("by_project_updated", (q) => q.eq("projectId", project._id)).order("desc").take(201);
    if (decisionRows.length > 200) fail("page_too_large");
    const relevantDecisions = decisionRows.filter((decision) => decision.affectedWorkstreamIds.includes(workstream._id) || decision.affectedMemberIds.includes(workstream.memberId));
    const deliveredDecisionIds = new Set<string>();
    for (const decision of relevantDecisions) {
      if (items.length >= 64) { truncated = true; break; }
      const item = { id: decision.publicId, revision: decision.revision, kind: "decision" as const, text: decision.summary, relevanceReason: "This durable Project decision explicitly affects this member or workstream.", fidelity: "manual", advisoryAction: "coordination_required" as const, priority: 95 };
      const estimated = Math.ceil(JSON.stringify(item).length / 4);
      if (renderedSize + estimated > args.requestedBudget) { truncated = true; continue; }
      items.push(item); renderedSize += estimated; deliveredDecisionIds.add(decision.publicId);
    }
    const semanticCurrent = !args.semanticDegraded && args.semanticContextRevision === scope.contextRevision;
    for (const objectPublicId of semanticCurrent ? args.semanticObjectIds : []) {
      if (items.length >= 64) { truncated = true; break; }
      const object = await ctx.db.query("semanticObjects").withIndex("by_public_id", (q) => q.eq("publicId", objectPublicId)).unique();
      if (!object || !object.active || object.scopeKey !== workstream.scopeKey || object.projectId !== project._id) continue;
      const peer = await ctx.db.get(object.workstreamId);
      if (!peer || peer._id === workstream._id || peer.status === "done") continue;
      const item = { id: object.publicId, revision: object.revision, kind: "workstream" as const, text: `Potentially related active work: ${peer.title}.`, relevanceReason: "A scoped semantic candidate shares behavior concepts; similarity is not proof of a collision.", fidelity: "semantic", advisoryAction: "informational" as const, priority: 35 };
      const estimated = Math.ceil(JSON.stringify(item).length / 4);
      if (renderedSize + estimated > args.requestedBudget) { truncated = true; continue; }
      items.push(item); renderedSize += estimated;
    }
    await ctx.db.insert("contextDeliveries", {
      publicId: args.briefPublicId,
      projectId: project._id,
      workstreamId: workstream._id,
      contextRevision: scope.contextRevision,
      trigger: args.trigger,
      itemRefs: items.map((item) => item.id),
      itemRevisions: Object.fromEntries(items.map((item) => [item.id, item.revision])),
      requestedBudget: args.requestedBudget,
      renderedSize,
      deliveredAt: args.now,
      expiresAt: args.now + DELIVERY_RETENTION,
    });
    for (const decision of relevantDecisions.filter((candidate) => deliveredDecisionIds.has(candidate.publicId))) {
      const existing = await ctx.db.query("decisionDeliveries").withIndex("by_decision_workstream", (q) => q.eq("decisionId", decision._id).eq("workstreamId", workstream._id)).unique();
      if (existing) {
        if (existing.decisionRevision !== decision.revision) await ctx.db.patch(existing._id, { decisionRevision: decision.revision, deliveredAt: args.now, acknowledgedAt: undefined });
      } else await ctx.db.insert("decisionDeliveries", { decisionId: decision._id, workstreamId: workstream._id, decisionRevision: decision.revision, deliveredAt: args.now });
    }
    return {
      briefId: args.briefPublicId,
      projectId: project.publicId,
      repositoryId: workspace.repoFingerprint,
      workstreamId: workstream.publicId,
      contextRevision: scope.contextRevision,
      generatedAt: new Date(args.now).toISOString(),
      trigger: args.trigger,
      requestedBudget: args.requestedBudget,
      renderedSize,
      truncated,
      items,
      nextCursor: `ctx:${scope.contextRevision}`,
    };
  },
});

export const contextItem = internalQuery({
  args: { tokenHash: v.string(), itemPublicId: v.string() },
  handler: async (ctx, args) => {
    const device = await requireDevice(ctx, args.tokenHash);
    const finding = await ctx.db.query("findings").withIndex("by_public_id", (q) => q.eq("publicId", args.itemPublicId)).unique();
    if (finding) {
      await requireMembership(ctx, device._id, finding.projectId);
      return findingContract(finding);
    }
    const decision = await ctx.db.query("decisions").withIndex("by_public_id", (q) => q.eq("publicId", args.itemPublicId)).unique();
    if (!decision) fail("not_found");
    await requireMembership(ctx, device._id, decision.projectId);
    return decisionContract(ctx, decision);
  },
});

export const recordFindingFeedback = internalMutation({
  args: { sessionHash: v.string(), findingPublicId: v.string(), value: v.string(), feedbackPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireBrowserSession(ctx, args.sessionHash, args.now);
    const finding = await ctx.db.query("findings").withIndex("by_public_id", (q) => q.eq("publicId", args.findingPublicId)).unique();
    if (!finding || finding.projectId !== auth.project._id) fail("not_found");
    const member = auth.member;
    if (!["useful", "not_related", "already_coordinated", "missed_severity"].includes(args.value)) fail("validation_failed");
    const existing = await ctx.db.query("findingFeedback").withIndex("by_finding_member", (q) => q.eq("findingId", finding._id).eq("memberId", member._id)).unique();
    const record = { value: args.value as "useful" | "not_related" | "already_coordinated" | "missed_severity", engineVersion: finding.engineVersion, createdAt: args.now, expiresAt: args.now + DEFAULT_RETENTION_DAYS * DAY };
    if (existing) await ctx.db.patch(existing._id, record);
    else await ctx.db.insert("findingFeedback", { publicId: args.feedbackPublicId, projectId: finding.projectId, findingId: finding._id, memberId: member._id, ...record });
    return true;
  },
});

const collaborationAuthArgs = {
  projectPublicId: v.string(),
  tokenHash: v.optional(v.string()),
  sessionHash: v.optional(v.string()),
  now: v.number(),
};

export const collaborationSnapshot = internalQuery({
  args: { ...collaborationAuthArgs, after: v.optional(v.number()) },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    return collaborationView(ctx, auth.project, args.after ?? 0);
  },
});

/**
 * Acknowledging and dismissing are the only finding states a member sets
 * directly. Resolution stays decision-backed (ADR-061): recording a decision on
 * the sync card is what resolves the finding, so accepting "resolved" here
 * would let a button mark work settled with no record of how.
 */
export const setFindingState = internalMutation({
  args: { sessionHash: v.string(), findingPublicId: v.string(), state: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireBrowserSession(ctx, args.sessionHash, args.now);
    if (!["acknowledged", "dismissed"].includes(args.state)) fail("validation_failed");
    const finding = await ctx.db.query("findings").withIndex("by_public_id", (q) => q.eq("publicId", args.findingPublicId)).unique();
    if (!finding || finding.projectId !== auth.project._id) fail("not_found");
    if (finding.state !== args.state) {
      await ctx.db.patch(finding._id, { state: args.state, revision: finding.revision + 1, lastSeenAt: args.now });
      // An acknowledged or dismissed collision changes what a brief should say
      // about it, so dependents must re-read rather than keep the open wording.
      await bumpProjectScopes(ctx, auth.project._id, args.now);
    }
    return true;
  },
});


export const createSyncCard = internalMutation({
  args: { ...collaborationAuthArgs, cardPublicId: v.string(), findingPublicId: v.optional(v.string()), title: v.string(), summary: v.string() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    let findingId: Id<"findings"> | undefined;
    if (args.findingPublicId) {
      const finding = await ctx.db.query("findings").withIndex("by_public_id", (q) => q.eq("publicId", args.findingPublicId!)).unique();
      if (!finding || finding.projectId !== auth.project._id) fail("not_found");
      findingId = finding._id;
    }
    const id = await ctx.db.insert("syncCards", { publicId: args.cardPublicId, projectId: auth.project._id, ...(findingId ? { findingId } : {}), title: args.title, summary: args.summary, state: "open", revision: 1, createdByMemberId: auth.member._id, createdAt: args.now, updatedAt: args.now });
    const card = await ctx.db.get(id);
    if (!card) fail("not_found");
    return syncCardContract(ctx, card);
  },
});

export const commentOnSyncCard = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), cardPublicId: v.string(), commentPublicId: v.string(), body: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const card = await ctx.db.query("syncCards").withIndex("by_public_id", (q) => q.eq("publicId", args.cardPublicId)).unique();
    if (!card) fail("not_found");
    const project = await ctx.db.get(card.projectId);
    if (!project) fail("not_found");
    const auth = await requireCollaborationActor(ctx, { projectPublicId: project.publicId, tokenHash: args.tokenHash, sessionHash: args.sessionHash, now: args.now });
    if (card.state !== "open") fail("sync_card_resolved");
    const id = await ctx.db.insert("syncComments", { publicId: args.commentPublicId, syncCardId: card._id, projectId: project._id, memberId: auth.member._id, body: args.body, createdAt: args.now });
    await ctx.db.patch(card._id, { revision: card.revision + 1, updatedAt: args.now });
    const comment = await ctx.db.get(id);
    if (!comment) fail("not_found");
    return syncCommentContract(ctx, comment);
  },
});

export const resolveSyncCard = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), cardPublicId: v.string(), decisionPublicId: v.string(), expectedRevision: v.number(), summary: v.string(), affectedMemberPublicIds: v.array(v.string()), affectedWorkstreamPublicIds: v.array(v.string()), now: v.number() },
  handler: async (ctx, args) => {
    const card = await ctx.db.query("syncCards").withIndex("by_public_id", (q) => q.eq("publicId", args.cardPublicId)).unique();
    if (!card) fail("not_found");
    const project = await ctx.db.get(card.projectId);
    if (!project) fail("not_found");
    const auth = await requireCollaborationActor(ctx, { projectPublicId: project.publicId, tokenHash: args.tokenHash, sessionHash: args.sessionHash, now: args.now });
    if (card.state !== "open" || card.revision !== args.expectedRevision) fail("revision_conflict");
    const affectedMemberIds: Id<"members">[] = [];
    for (const publicId of [...new Set(args.affectedMemberPublicIds)]) {
      const member = await ctx.db.query("members").withIndex("by_public_id", (q) => q.eq("publicId", publicId)).unique();
      if (!member || member.projectId !== project._id || member.removedAt !== undefined) fail("forbidden");
      affectedMemberIds.push(member._id);
    }
    const affectedWorkstreamIds: Id<"workstreams">[] = [];
    for (const publicId of [...new Set(args.affectedWorkstreamPublicIds)]) {
      const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", publicId)).unique();
      if (!workstream || workstream.projectId !== project._id) fail("forbidden");
      affectedWorkstreamIds.push(workstream._id);
    }
    if (affectedMemberIds.length === 0 && affectedWorkstreamIds.length === 0) fail("validation_failed");
    const id = await ctx.db.insert("decisions", { publicId: args.decisionPublicId, projectId: project._id, syncCardId: card._id, summary: args.summary, affectedMemberIds, affectedWorkstreamIds, revision: 1, createdByMemberId: auth.member._id, createdAt: args.now, updatedAt: args.now });
    await ctx.db.patch(card._id, { state: "resolved", revision: card.revision + 1, updatedAt: args.now });
    if (card.findingId) {
      const finding = await ctx.db.get(card.findingId);
      if (finding && finding.state !== "resolved") await ctx.db.patch(finding._id, { state: "resolved", revision: finding.revision + 1, lastSeenAt: args.now });
    }
    await bumpProjectScopes(ctx, project._id, args.now);
    const decision = await ctx.db.get(id);
    if (!decision) fail("not_found");
    return decisionContract(ctx, decision);
  },
});

export const sessionSharingSnapshot = internalQuery({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), workstreamPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", args.workstreamPublicId)).unique();
    if (!workstream) fail("not_found");
    const project = await ctx.db.get(workstream.projectId);
    if (!project) fail("not_found");
    const auth = await requireCollaborationActor(ctx, { projectPublicId: project.publicId, tokenHash: args.tokenHash, sessionHash: args.sessionHash, now: args.now });
    return sessionSharingView(ctx, workstream, auth.member, args.now);
  },
});

export const deleteSharedSessionMessages = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), workstreamPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", args.workstreamPublicId)).unique();
    if (!workstream) fail("not_found");
    const project = await ctx.db.get(workstream.projectId);
    if (!project) fail("not_found");
    const auth = await requireCollaborationActor(ctx, { projectPublicId: project.publicId, tokenHash: args.tokenHash, sessionHash: args.sessionHash, now: args.now });
    if (workstream.memberId !== auth.member._id && auth.member.role !== "owner") fail("forbidden");
    const messages = await ctx.db.query("sessionMessages").withIndex("by_workstream_captured", (q) => q.eq("workstreamId", workstream._id)).take(101);
    if (messages.length > 100) fail("page_too_large");
    for (const message of messages) await ctx.db.delete(message._id);
    return true;
  },
});

export const updateMemberDisplayName = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), displayName: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    const displayName = normalizeDisplayName(args.displayName);
    await ctx.db.patch(auth.member._id, { displayName, displayNameSource: "member" });
    // Names appear in briefs and rendered coordination items, so dependents must
    // re-read rather than keep the previous device-derived label.
    await bumpProjectScopes(ctx, auth.project._id, args.now);
    return { memberId: auth.member.publicId, memberName: displayName, memberNameSource: "member" as const };
  },
});

export const projectMembers = internalQuery({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    const members = await ctx.db.query("members").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(101);
    if (members.length > 100) fail("page_too_large");
    return {
      members: members.filter((member) => member.removedAt === undefined).map((member) => ({
        id: member.publicId,
        name: member.displayName,
        nameSource: member.displayNameSource ?? "device",
        role: member.role,
        isSelf: member._id === auth.member._id,
        joinedAt: new Date(member.joinedAt).toISOString(),
      })),
    };
  },
});

export const projectAccess = internalQuery({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    const members = await ctx.db.query("members").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(101);
    if (members.length > 100) fail("page_too_large");
    const activeMembers = members.filter((member) => member.removedAt === undefined);
    const devices: Array<{
      id: string; memberId: string; label: string; appVersion: string;
      isCurrent: boolean; revoked: boolean; lastSeenAt: string | undefined;
    }> = [];
    for (const member of activeMembers) {
      const device = await ctx.db.get(member.deviceId);
      if (device && !devices.some((candidate) => candidate.id === device.publicId)) devices.push({
        id: device.publicId, memberId: member.publicId, label: device.label, appVersion: device.appVersion,
        isCurrent: device._id === auth.device._id, revoked: device.revokedAt !== undefined,
        lastSeenAt: device.lastSeenAt === undefined ? undefined : new Date(device.lastSeenAt).toISOString(),
      });
    }
    const invites = auth.member.role === "owner"
      ? (await ctx.db.query("invites").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(101)).map((invite) => ({
        id: invite.publicId, expiresAt: new Date(invite.expiresAt).toISOString(), remainingUses: invite.remainingUses,
        revoked: invite.revokedAt !== undefined, createdAt: new Date(invite.createdAt).toISOString(),
      }))
      : [];
    if (invites.length > 100) fail("page_too_large");
    return {
      role: auth.member.role,
      members: activeMembers.map((member) => ({
        id: member.publicId, name: member.displayName, nameSource: member.displayNameSource ?? "device", role: member.role,
        isSelf: member._id === auth.member._id, joinedAt: new Date(member.joinedAt).toISOString(),
      })),
      devices,
      invites,
    };
  },
});

export const revokeInvite = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), invitePublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    if (auth.member.role !== "owner") fail("forbidden");
    await enforceRate(ctx, auth.device.tokenHash, "invites.revoke", args.now, 30, 60_000);
    const invite = await ctx.db.query("invites").withIndex("by_public_id", (q) => q.eq("publicId", args.invitePublicId)).unique();
    if (!invite || invite.projectId !== auth.project._id) fail("not_found");
    if (invite.revokedAt === undefined) await ctx.db.patch(invite._id, { revokedAt: args.now, remainingUses: 0 });
    return true;
  },
});

export const removeProjectMember = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), memberPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    if (auth.member.role !== "owner") fail("forbidden");
    await enforceRate(ctx, auth.device.tokenHash, "members.remove", args.now, 20, 60_000);
    const target = await ctx.db.query("members").withIndex("by_public_id", (q) => q.eq("publicId", args.memberPublicId)).unique();
    if (!target || target.projectId !== auth.project._id || target.removedAt !== undefined) fail("not_found");
    if (target._id === auth.member._id) fail("cannot_remove_self");
    await ctx.db.patch(target._id, { removedAt: args.now });
    const sessions = await ctx.db.query("browserSessions").withIndex("by_member", (q) => q.eq("memberId", target._id)).take(101);
    if (sessions.length > 100) fail("page_too_large");
    for (const session of sessions) if (session.revokedAt === undefined) await ctx.db.patch(session._id, { revokedAt: args.now });
    await bumpProjectScopes(ctx, auth.project._id, args.now);
    return true;
  },
});

export const exportProject = internalQuery({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    const allMembers = await ctx.db.query("members").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allWorkspaces = await ctx.db.query("workspaces").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allWorkstreams = await ctx.db.query("workstreams").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allManifests = await ctx.db.query("changeManifests").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allContracts = await ctx.db.query("contractFingerprints").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allReads = await ctx.db.query("sessionReadSets").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allFindings = await ctx.db.query("findings").withIndex("by_project_seen", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allSemantic = await ctx.db.query("semanticObjects").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allDeliveries = await ctx.db.query("contextDeliveries").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allActivity = await ctx.db.query("activityEvents").withIndex("by_project_received", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allCards = await ctx.db.query("syncCards").withIndex("by_project_updated", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allComments = await ctx.db.query("syncComments").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allDecisions = await ctx.db.query("decisions").withIndex("by_project_updated", (q) => q.eq("projectId", auth.project._id)).take(501);
    const allMessages = await ctx.db.query("sessionMessages").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(501);
    const collections = [allMembers, allWorkspaces, allWorkstreams, allManifests, allContracts, allReads, allFindings, allSemantic, allDeliveries, allActivity, allCards, allComments, allDecisions, allMessages];
    if (collections.some((collection) => collection.length > 500)) fail("export_too_large");

    const isOwner = auth.member.role === "owner";
    const workspaces = isOwner ? allWorkspaces : allWorkspaces.filter((workspace) => workspace.memberId === auth.member._id);
    const workspaceIds = new Set(workspaces.map((workspace) => workspace._id));
    const workstreams = isOwner ? allWorkstreams : allWorkstreams.filter((workstream) => workstream.memberId === auth.member._id && workspaceIds.has(workstream.workspaceId));
    const workstreamIds = new Set(workstreams.map((workstream) => workstream._id));
    const workstreamPublicIds = new Set(workstreams.map((workstream) => workstream.publicId));
    const members = isOwner ? allMembers : [auth.member];
    const manifests = isOwner ? allManifests : allManifests.filter((manifest) => workstreamIds.has(manifest.workstreamId));
    const contracts = isOwner ? allContracts : allContracts.filter((contract) => workstreamPublicIds.has(contract.changedByWorkstreamPublicId));
    const reads = isOwner ? allReads : allReads.filter((read) => workstreamIds.has(read.workstreamId));
    const findings = isOwner ? allFindings : allFindings.filter((finding) => finding.workstreamPublicIds.some((id) => workstreamPublicIds.has(id)));
    const semantic = isOwner ? allSemantic : allSemantic.filter((object) => workstreamIds.has(object.workstreamId));
    const deliveries = isOwner ? allDeliveries : allDeliveries.filter((delivery) => workstreamIds.has(delivery.workstreamId));
    const activity = isOwner ? allActivity : allActivity.filter((event) => event.memberId === auth.member._id);
    const cards = isOwner ? allCards : allCards.filter((card) => card.createdByMemberId === auth.member._id);
    const cardIds = new Set(cards.map((card) => card._id));
    const comments = isOwner ? allComments : allComments.filter((comment) => comment.memberId === auth.member._id || cardIds.has(comment.syncCardId));
    const decisions = isOwner ? allDecisions : allDecisions.filter((decision) => decision.createdByMemberId === auth.member._id || decision.affectedMemberIds.includes(auth.member._id));
    const messages = isOwner ? allMessages : allMessages.filter((message) => message.memberId === auth.member._id);
    const memberPublic = new Map(members.map((member) => [member._id, member.publicId]));
    const workspacePublic = new Map(workspaces.map((workspace) => [workspace._id, workspace.publicId]));
    const workstreamPublic = new Map(workstreams.map((workstream) => [workstream._id, workstream.publicId]));
    return {
      schemaVersion: 1,
      exportedAt: new Date(args.now).toISOString(),
      project: { id: auth.project.publicId, name: auth.project.label, status: auth.project.status, createdAt: new Date(auth.project.createdAt).toISOString(), retentionDays: auth.project.retentionDays },
      members: members.map((member) => ({ id: member.publicId, name: member.displayName, role: member.role, joinedAt: new Date(member.joinedAt).toISOString(), removedAt: member.removedAt === undefined ? undefined : new Date(member.removedAt).toISOString() })),
      workspaces: workspaces.map((workspace) => ({ id: workspace.publicId, memberId: memberPublic.get(workspace.memberId), repositoryFingerprint: workspace.repoFingerprint, label: workspace.label, paused: workspace.paused, updatedAt: new Date(workspace.updatedAt).toISOString() })),
      workstreams: workstreams.map((stream) => ({ id: stream.publicId, memberId: memberPublic.get(stream.memberId), workspaceId: workspacePublic.get(stream.workspaceId), title: stream.title, summary: stream.summary, status: stream.status, revision: stream.revision, vendor: stream.vendor, sessionAlias: stream.sessionAlias, updatedAt: new Date(stream.updatedAt).toISOString() })),
      manifests: manifests.map((manifest) => ({ id: manifest.publicId, workstreamId: workstreamPublic.get(manifest.workstreamId), revision: manifest.revision, baselineRef: manifest.baselineRef, headRef: manifest.headRef, contentHash: manifest.contentHash, pathCount: manifest.pathCount, state: manifest.state })),
      contractFingerprints: contracts.map((contract) => ({ path: contract.path, fileContractHash: contract.fileContractHash, symbols: contract.symbols, changedByWorkstreamId: contract.changedByWorkstreamPublicId, revision: contract.revision, updatedAt: new Date(contract.updatedAt).toISOString() })),
      readSets: reads.map((read) => ({ workstreamId: read.workstreamPublicId, path: read.path, fileContractHashAtRead: read.fileContractHashAtRead, readAt: read.readAt })),
      findings: findings.map(findingContract),
      semanticObjects: semantic.map((object) => ({ id: object.publicId, workstreamId: workstreamPublic.get(object.workstreamId), kind: object.kind, text: object.text, source: object.source, fidelity: object.fidelity, tags: object.tags, revision: object.revision, active: object.active })),
      contextDeliveries: deliveries.map((delivery) => ({ id: delivery.publicId, workstreamId: workstreamPublic.get(delivery.workstreamId), contextRevision: delivery.contextRevision, trigger: delivery.trigger, itemRefs: delivery.itemRefs, deliveredAt: new Date(delivery.deliveredAt).toISOString(), acknowledgedAt: delivery.acknowledgedAt === undefined ? undefined : new Date(delivery.acknowledgedAt).toISOString() })),
      activity: activity.map((event) => ({ eventId: event.eventId, memberId: memberPublic.get(event.memberId), workspaceId: event.workspacePublicId, sequence: event.sequence, observedAt: event.observedAt, receivedAt: new Date(event.receivedAt).toISOString(), source: event.source, type: event.type, payload: event.payload })),
      syncCards: cards.map((card) => ({ id: card.publicId, title: card.title, summary: card.summary, state: card.state, revision: card.revision, createdAt: new Date(card.createdAt).toISOString(), updatedAt: new Date(card.updatedAt).toISOString() })),
      syncComments: comments.map((comment) => ({ id: comment.publicId, memberId: memberPublic.get(comment.memberId), body: comment.body, createdAt: new Date(comment.createdAt).toISOString() })),
      resolutions: decisions.map((decision) => ({ id: decision.publicId, summary: decision.summary, revision: decision.revision, createdAt: new Date(decision.createdAt).toISOString(), updatedAt: new Date(decision.updatedAt).toISOString() })),
      sessionMessages: messages.map((message) => ({ id: message.publicId, workstreamId: workstreamPublic.get(message.workstreamId), memberId: memberPublic.get(message.memberId), vendor: message.vendor, kind: message.kind, text: message.text, capturedAt: new Date(message.capturedAt).toISOString(), expiresAt: new Date(message.expiresAt).toISOString() })),
    };
  },
});

export const beginProjectDeletion = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    if (auth.member.role !== "owner") fail("forbidden");
    await enforceRate(ctx, auth.device.tokenHash, "projects.delete", args.now, 3, 60_000);
    await ctx.db.patch(auth.project._id, { status: "deleting" });
    const sessions = await ctx.db.query("browserSessions").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(101);
    const invites = await ctx.db.query("invites").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).take(101);
    if (sessions.length > 100 || invites.length > 100) fail("page_too_large");
    for (const session of sessions) await ctx.db.patch(session._id, { revokedAt: args.now });
    for (const invite of invites) await ctx.db.patch(invite._id, { revokedAt: args.now, remainingUses: 0 });
    await ctx.scheduler.runAfter(0, internal.service.deleteProjectBatch, { projectId: auth.project._id });
    return true;
  },
});

export const beginMemberDataDeletion = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), projectPublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireCollaborationActor(ctx, args);
    if (auth.member.role === "owner") fail("owner_must_delete_project_or_transfer");
    await enforceRate(ctx, auth.device.tokenHash, "members.delete_self", args.now, 3, 60_000);
    await ctx.db.patch(auth.member._id, { removedAt: args.now });
    const sessions = await ctx.db.query("browserSessions").withIndex("by_member", (q) => q.eq("memberId", auth.member._id)).take(101);
    if (sessions.length > 100) fail("page_too_large");
    for (const session of sessions) if (session.revokedAt === undefined) await ctx.db.patch(session._id, { revokedAt: args.now });
    await bumpProjectScopes(ctx, auth.project._id, args.now);
    await ctx.scheduler.runAfter(0, internal.service.deleteMemberDataBatch, { projectId: auth.project._id, memberId: auth.member._id, deviceId: auth.device._id });
    return true;
  },
});

export const deleteMemberDataBatch = internalMutation({
  args: { projectId: v.id("projects"), memberId: v.id("members"), deviceId: v.id("devices") },
  handler: async (ctx, args) => {
    const member = await ctx.db.get(args.memberId);
    if (!member || member.projectId !== args.projectId || member.removedAt === undefined) return false;
    const again = async () => { await ctx.scheduler.runAfter(0, internal.service.deleteMemberDataBatch, args); return true; };
    const workstream = (await ctx.db.query("workstreams").withIndex("by_member", (q) => q.eq("memberId", args.memberId)).take(1))[0];
    if (workstream) {
      const manifest = (await ctx.db.query("changeManifests").withIndex("by_workstream", (q) => q.eq("workstreamId", workstream._id)).take(1))[0];
      if (manifest) {
        const chunks = await ctx.db.query("changeManifestChunks").withIndex("by_manifest_chunk", (q) => q.eq("manifestId", manifest._id)).take(100);
        if (chunks.length) for (const chunk of chunks) await ctx.db.delete(chunk._id); else await ctx.db.delete(manifest._id);
        return again();
      }
      const semantic = (await ctx.db.query("semanticObjects").withIndex("by_workstream_active", (q) => q.eq("workstreamId", workstream._id)).take(1))[0];
      if (semantic) {
        const embeddings = await ctx.db.query("semanticEmbeddings").withIndex("by_object", (q) => q.eq("objectId", semantic._id)).take(100);
        if (embeddings.length) for (const embedding of embeddings) await ctx.db.delete(embedding._id); else await ctx.db.delete(semantic._id);
        return again();
      }
      const contextDeliveries = await ctx.db.query("contextDeliveries").withIndex("by_workstream", (q) => q.eq("workstreamId", workstream._id)).take(100);
      if (contextDeliveries.length) { for (const row of contextDeliveries) await ctx.db.delete(row._id); return again(); }
      const reads = await ctx.db.query("sessionReadSets").withIndex("by_workstream_path", (q) => q.eq("workstreamId", workstream._id)).take(100);
      if (reads.length) { for (const row of reads) await ctx.db.delete(row._id); return again(); }
      const messages = await ctx.db.query("sessionMessages").withIndex("by_workstream_captured", (q) => q.eq("workstreamId", workstream._id)).take(100);
      if (messages.length) { for (const row of messages) await ctx.db.delete(row._id); return again(); }
      const workstreamDecisionDeliveries = await ctx.db.query("decisionDeliveries").withIndex("by_workstream", (q) => q.eq("workstreamId", workstream._id)).take(100);
      if (workstreamDecisionDeliveries.length) { for (const row of workstreamDecisionDeliveries) await ctx.db.delete(row._id); return again(); }
      const finding = (await ctx.db.query("findings").withIndex("by_project", (q) => q.eq("projectId", args.projectId)).take(501)).find((candidate) => candidate.workstreamPublicIds.includes(workstream.publicId));
      if (finding) {
        const feedback = await ctx.db.query("findingFeedback").withIndex("by_finding_member", (q) => q.eq("findingId", finding._id)).take(100);
        if (feedback.length) for (const row of feedback) await ctx.db.delete(row._id); else await ctx.db.delete(finding._id);
        return again();
      }
      const decision = (await ctx.db.query("decisions").withIndex("by_project_updated", (q) => q.eq("projectId", args.projectId)).take(501)).find((candidate) => candidate.affectedWorkstreamIds.includes(workstream._id));
      if (decision) {
        const deliveries = await ctx.db.query("decisionDeliveries").withIndex("by_decision_workstream", (q) => q.eq("decisionId", decision._id)).take(100);
        if (deliveries.length) for (const row of deliveries) await ctx.db.delete(row._id); else await ctx.db.delete(decision._id);
        return again();
      }
      const contract = (await ctx.db.query("contractFingerprints").withIndex("by_project", (q) => q.eq("projectId", args.projectId)).take(501)).find((candidate) => candidate.changedByWorkstreamPublicId === workstream.publicId);
      if (contract) { await ctx.db.delete(contract._id); return again(); }
      await ctx.db.delete(workstream._id);
      return again();
    }
    for (const table of ["activityEvents", "findingFeedback", "syncComments", "sessionMessages", "dashboardTickets", "browserSessions"] as const) {
      const rows = await ctx.db.query(table).withIndex("by_member", (q) => q.eq("memberId", args.memberId)).take(100);
      if (rows.length) { for (const row of rows) await ctx.db.delete(row._id); return again(); }
    }
    const card = (await ctx.db.query("syncCards").withIndex("by_creator", (q) => q.eq("createdByMemberId", args.memberId)).take(1))[0];
    if (card) {
      const comments = await ctx.db.query("syncComments").withIndex("by_card_created", (q) => q.eq("syncCardId", card._id)).take(100);
      if (comments.length) for (const comment of comments) await ctx.db.delete(comment._id); else await ctx.db.delete(card._id);
      return again();
    }
    const decision = (await ctx.db.query("decisions").withIndex("by_creator", (q) => q.eq("createdByMemberId", args.memberId)).take(1))[0];
    if (decision) {
      const deliveries = await ctx.db.query("decisionDeliveries").withIndex("by_decision_workstream", (q) => q.eq("decisionId", decision._id)).take(100);
      if (deliveries.length) for (const delivery of deliveries) await ctx.db.delete(delivery._id); else await ctx.db.delete(decision._id);
      return again();
    }
    const invite = (await ctx.db.query("invites").withIndex("by_creator", (q) => q.eq("createdByMemberId", args.memberId)).take(1))[0];
    if (invite) { await ctx.db.delete(invite._id); return again(); }
    const workspace = (await ctx.db.query("workspaces").withIndex("by_member", (q) => q.eq("memberId", args.memberId)).take(1))[0];
    if (workspace) { await ctx.db.delete(workspace._id); return again(); }
    const cursor = await ctx.db.query("deviceCursors").withIndex("by_device_project", (q) => q.eq("deviceId", args.deviceId).eq("projectId", args.projectId)).unique();
    if (cursor) await ctx.db.delete(cursor._id);
    await ctx.db.delete(member._id);
    const remainingMembership = await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", args.deviceId)).take(1);
    if (remainingMembership.length === 0) await ctx.db.delete(args.deviceId);
    return true;
  },
});

export const deleteProjectBatch = internalMutation({
  args: { projectId: v.id("projects") },
  handler: async (ctx, args) => {
    const project = await ctx.db.get(args.projectId);
    if (!project || project.status !== "deleting") return false;
    const again = async () => { await ctx.scheduler.runAfter(0, internal.service.deleteProjectBatch, args); return true; };
    const manifest = (await ctx.db.query("changeManifests").withIndex("by_project", (q) => q.eq("projectId", args.projectId)).take(1))[0];
    if (manifest) {
      const chunks = await ctx.db.query("changeManifestChunks").withIndex("by_manifest_chunk", (q) => q.eq("manifestId", manifest._id)).take(100);
      if (chunks.length) for (const chunk of chunks) await ctx.db.delete(chunk._id); else await ctx.db.delete(manifest._id);
      return again();
    }
    const object = (await ctx.db.query("semanticObjects").withIndex("by_project", (q) => q.eq("projectId", args.projectId)).take(1))[0];
    if (object) {
      const embeddings = await ctx.db.query("semanticEmbeddings").withIndex("by_object", (q) => q.eq("objectId", object._id)).take(100);
      if (embeddings.length) for (const embedding of embeddings) await ctx.db.delete(embedding._id); else await ctx.db.delete(object._id);
      return again();
    }
    const decision = (await ctx.db.query("decisions").withIndex("by_project_updated", (q) => q.eq("projectId", args.projectId)).take(1))[0];
    if (decision) {
      const deliveries = await ctx.db.query("decisionDeliveries").withIndex("by_decision_workstream", (q) => q.eq("decisionId", decision._id)).take(100);
      if (deliveries.length) for (const delivery of deliveries) await ctx.db.delete(delivery._id); else await ctx.db.delete(decision._id);
      return again();
    }
    const workstream = (await ctx.db.query("workstreams").withIndex("by_project", (q) => q.eq("projectId", args.projectId)).take(1))[0];
    if (workstream) {
      const deliveries = await ctx.db.query("decisionDeliveries").withIndex("by_workstream", (q) => q.eq("workstreamId", workstream._id)).take(100);
      if (deliveries.length) for (const delivery of deliveries) await ctx.db.delete(delivery._id); else await ctx.db.delete(workstream._id);
      return again();
    }
    const directTables = ["contextDeliveries", "sessionReadSets", "contractFingerprints", "findingFeedback", "findings", "sessionMessages", "syncComments", "syncCards", "activityEvents", "deviceCursors", "dashboardTickets", "browserSessions", "invites", "workspaces", "repositoryScopes"] as const;
    for (const table of directTables) {
      const rows = await ctx.db.query(table).withIndex("by_project", (q) => q.eq("projectId", args.projectId)).take(100);
      if (rows.length) { for (const row of rows) await ctx.db.delete(row._id); return again(); }
    }
    const members = await ctx.db.query("members").withIndex("by_project", (q) => q.eq("projectId", args.projectId)).take(100);
    if (members.length) {
      for (const member of members) {
        await ctx.db.delete(member._id);
        const remainingMembership = await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", member.deviceId)).take(1);
        if (remainingMembership.length === 0) await ctx.db.delete(member.deviceId);
      }
      return again();
    }
    await ctx.db.delete(project._id);
    return true;
  },
});

export const revokeDevice = internalMutation({
  args: { tokenHash: v.optional(v.string()), sessionHash: v.optional(v.string()), targetDevicePublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const target = await ctx.db.query("devices").withIndex("by_public_id", (q) => q.eq("publicId", args.targetDevicePublicId)).unique();
    if (!target) fail("not_found");
    let actor: Doc<"devices">;
    if (args.sessionHash) {
      if (args.tokenHash) fail("unauthorized");
      const auth = await requireBrowserSession(ctx, args.sessionHash, args.now);
      actor = auth.device;
      const targetMembership = await ctx.db.query("members").withIndex("by_project_device", (q) => q.eq("projectId", auth.project._id).eq("deviceId", target._id)).unique();
      if (!targetMembership || targetMembership.removedAt !== undefined || actor._id !== target._id && auth.member.role !== "owner") fail("forbidden");
    } else {
      actor = await requireDevice(ctx, args.tokenHash ?? "");
    }
    await enforceRate(ctx, actor.tokenHash, "devices.revoke", args.now, 20, 60_000);
    if (!args.sessionHash && actor._id !== target._id) {
      const actorMemberships = (await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", actor._id)).collect())
        .filter((member) => member.removedAt === undefined && member.role === "owner");
      const targetMemberships = (await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", target._id)).collect())
        .filter((member) => member.removedAt === undefined);
      if (!actorMemberships.some((owner) => targetMemberships.some((member) => member.projectId === owner.projectId))) fail("forbidden");
    }
    await assertOwnerDeviceRevocable(ctx, target);
    await ctx.db.patch(target._id, { revokedAt: args.now });
    return true;
  },
});

// Edge guards run in their own transaction so failed authentication or
// validation attempts still consume quota instead of rolling the counter back.
export const consumeRate = internalMutation({
  args: { key: v.string(), route: v.string(), now: v.number(), limit: v.number(), windowMs: v.number() },
  handler: async (ctx, args) => {
    await enforceRate(ctx, args.key, args.route, args.now, args.limit, args.windowMs);
    return true;
  },
});

export const retentionSweep = internalMutation({
  args: { now: v.optional(v.number()), batchSize: v.optional(v.number()) },
  handler: async (ctx, args) => {
    const now = args.now ?? Date.now();
    const limit = Math.max(1, Math.min(args.batchSize ?? 200, 200));
    let deleted = 0;
    for (const table of RETENTION_TABLES) {
      const expired = await ctx.db.query(table).withIndex("by_expiry", (q) => q.lte("expiresAt", now)).take(limit - deleted);
      for (const document of expired) {
        await ctx.db.delete(document._id);
        deleted++;
      }
      if (deleted >= limit) break;
    }
    const expiredSessions = await expireQuietSessions(ctx, now, limit);
    return deleted + expiredSessions;
  },
});

// expireQuietSessions ends agent sessions that stopped reporting without ever
// sending SessionEnd. Without this the coordination engine keeps counting them
// as live, and a day of abandoned sessions collides with everything that
// follows. The end time recorded is the last moment the session was actually
// seen, not the moment the sweep noticed, so the dashboard clock stays honest.
async function expireQuietSessions(ctx: MutationCtx, now: number, limit: number): Promise<number> {
  let expired = 0;
  for (const status of ["active", "idle", "blocked"] as const) {
    if (expired >= limit) break;
    const candidates = await ctx.db
      .query("workstreams")
      // The index bound must match the per-status window the predicate
      // applies, or a stopped session between the two would never even be
      // considered (B31).
      .withIndex("by_status_updated", (q) => q.eq("status", status).lte("updatedAt", now - (status === "idle" ? SESSION_STOP_TIMEOUT_MS : SESSION_IDLE_TIMEOUT_MS)))
      .take(limit - expired);
    for (const session of candidates) {
      if (!sessionHasGoneQuiet(session, now)) continue;
      await ctx.db.patch(session._id, {
        status: "done", agentStatus: "done", endedAt: session.endedAt ?? session.updatedAt,
        revision: session.revision + 1, updatedAt: now,
      });
      // A finding routed to a session nobody is running any more is noise on
      // every future brief, so it resolves with the session exactly as it would
      // have on a SessionEnd that never came.
      //
      // A scope too large to enumerate makes that resolution fail, and this
      // runs inside a scheduled mutation: letting the error escape would roll
      // back the whole sweep and keep rolling it back on every future run, so
      // one unusual scope would silently stop retention for every Project on
      // the deployment. Ending the session is the part that matters; the
      // findings expire on their own retention.
      try {
        await resolveAgentPathFindings(ctx, session, now);
      } catch (error) {
        if (!(error instanceof Error) || !error.message.startsWith("E:")) throw error;
      }
      expired++;
    }
  }
  return expired;
}

async function applyProjection(
  ctx: MutationCtx,
  event: EventInput,
  project: Doc<"projects">,
  member: Doc<"members">,
  device: Doc<"devices">,
  now: number,
): Promise<void> {
  const payload = event.payload;
  let workspace = await ctx.db.query("workspaces").withIndex("by_public_id", (q) => q.eq("publicId", event.workspaceId)).unique();
  if (event.type === "workspace.registered") {
    const repoFingerprint = String(payload.repoFingerprint);
    const key = scopeKey(project.publicId, repoFingerprint);
    if (workspace && (workspace.deviceId !== device._id || workspace.projectId !== project._id)) fail("forbidden");
    if (!workspace) {
      const workspaceId = await ctx.db.insert("workspaces", {
        publicId: event.workspaceId, projectId: project._id, memberId: member._id, deviceId: device._id,
        repoFingerprint, scopeKey: key, label: String(payload.label), capabilities: payload.capabilities,
        paused: false, lastProjectedSequence: event.sequence, updatedAt: now,
      });
      workspace = await ctx.db.get(workspaceId);
      await ensureScope(ctx, project._id, repoFingerprint, key, now);
      await bumpScope(ctx, key, now);
    } else if (event.sequence > workspace.lastProjectedSequence) {
      const material = workspace.repoFingerprint !== repoFingerprint || workspace.label !== String(payload.label);
      await ctx.db.patch(workspace._id, {
        repoFingerprint, scopeKey: key, label: String(payload.label), capabilities: payload.capabilities,
        lastProjectedSequence: event.sequence, updatedAt: now,
      });
      await ensureScope(ctx, project._id, repoFingerprint, key, now);
      if (material) await bumpScope(ctx, key, now);
    }
    return;
  }
  if (!workspace || workspace.deviceId !== device._id || workspace.projectId !== project._id) fail("workspace_not_registered");
  switch (event.type) {
    case "workspace.paused":
    case "workspace.resumed": {
      if (event.sequence <= workspace.lastProjectedSequence) return;
      const paused = event.type === "workspace.paused";
      await ctx.db.patch(workspace._id, { paused, lastProjectedSequence: event.sequence, updatedAt: now });
      if (paused !== workspace.paused) await bumpScope(ctx, workspace.scopeKey, now);
      return;
    }
    case "workstream.intent_reported": {
      if (event.sequence <= workspace.lastProjectedSequence) return;
      const workstreamPublicId = String(payload.workstreamId);
      const current = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", workstreamPublicId)).unique();
      const summary = String(payload.intendedOutcome);
      const hasApproach = typeof payload.approachSummary === "string";
      const approachSummary = hasApproach ? String(payload.approachSummary) : undefined;
      const hasComponents = Array.isArray(payload.components);
      const components = hasComponents ? stringValues(payload.components) : undefined;
      const hasContracts = Array.isArray(payload.contracts);
      const contracts = hasContracts ? stringValues(payload.contracts) : undefined;
      const hasWaitingOn = Array.isArray(payload.waitingOn);
      const waitingOn = hasWaitingOn ? stringValues(payload.waitingOn) : undefined;
      if (!current) {
        await ctx.db.insert("workstreams", {
          publicId: workstreamPublicId, projectId: project._id, memberId: member._id, workspaceId: workspace._id,
          scopeKey: workspace.scopeKey, title: String(payload.title), summary, intendedOutcome: summary, status: "active", revision: 1, updatedAt: now,
          ...(approachSummary !== undefined ? { approachSummary } : {}),
          ...(components !== undefined ? { components } : {}),
          ...(contracts !== undefined ? { contracts } : {}),
          ...(waitingOn !== undefined ? { waitingOn, waitingOnDeclared: true } : {}),
        });
        await bumpScope(ctx, workspace.scopeKey, now);
      } else {
        if (current.projectId !== project._id || current.workspaceId !== workspace._id) fail("forbidden");
        const material = current.title !== String(payload.title) || current.summary !== summary || current.intendedOutcome !== summary ||
          approachSummary !== undefined && current.approachSummary !== approachSummary ||
          components !== undefined && JSON.stringify(current.components ?? []) !== JSON.stringify(components) ||
          contracts !== undefined && JSON.stringify(current.contracts ?? []) !== JSON.stringify(contracts) ||
          waitingOn !== undefined && JSON.stringify(current.waitingOn ?? []) !== JSON.stringify(waitingOn);
        // A goal closes only when the goal itself moved. `material` is also
        // true for a components, contracts, or waitingOn edit, and treating
        // those as a new objective would manufacture a history of goals the
        // session never actually changed between.
        const goalMoved = current.title !== String(payload.title) || current.intendedOutcome !== summary;
        const priorGoals = goalMoved
          ? [...(current.priorGoals ?? []), {
              title: current.title,
              ...(current.intendedOutcome !== undefined ? { intendedOutcome: current.intendedOutcome } : {}),
              endedAt: new Date(now).toISOString(),
            }]
          : undefined;
        const kept = priorGoals ? priorGoals.slice(-MAX_PRIOR_GOALS) : undefined;
        await ctx.db.patch(current._id, {
          title: String(payload.title), summary, intendedOutcome: summary, revision: current.revision + 1, status: "active", updatedAt: now,
          ...(approachSummary !== undefined ? { approachSummary } : {}),
          ...(components !== undefined ? { components } : {}),
          ...(contracts !== undefined ? { contracts } : {}),
          ...(waitingOn !== undefined ? { waitingOn, waitingOnDeclared: true } : {}),
          ...(kept !== undefined ? {
            priorGoals: kept,
            priorGoalsDropped: (current.priorGoalsDropped ?? 0) + (priorGoals!.length - kept.length),
          } : {}),
        });
        if (material) await bumpScope(ctx, workspace.scopeKey, now);
      }
      await ctx.db.patch(workspace._id, { lastProjectedSequence: Math.max(workspace.lastProjectedSequence, event.sequence), updatedAt: now });
      const projected = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", workstreamPublicId)).unique();
      if (!projected) fail("workstream_not_found");
      await recomputeDependencyReadiness(ctx, project, projected, now);
      const tags = [
        ...stringValues(payload.components).map((value) => `component:${value}`),
        ...stringValues(payload.contracts).map((value) => `contract:${value}`),
        ...stringValues(payload.anticipatedPaths).map((value) => `path:${value}`),
      ];
      await upsertSemanticIntelligence(ctx, project, projected, boundedSemanticText([summary, String(payload.approachSummary ?? "")]), "intent", tags, event.source, now);
      return;
    }
    case "workstream.status_changed": {
      const workstream = await requireWorkstream(ctx, String(payload.workstreamId), project._id, workspace._id);
      if (event.sequence <= workspace.lastProjectedSequence) return;
      const status = String(payload.status) as "active" | "idle" | "done" | "blocked";
      await ctx.db.patch(workstream._id, { status, revision: workstream.revision + 1, updatedAt: now });
      await ctx.db.patch(workspace._id, { lastProjectedSequence: event.sequence, updatedAt: now });
      if (status !== workstream.status) await bumpScope(ctx, workspace.scopeKey, now);
      if (status === "done") {
        await resolveDependencyFindings(ctx, workstream, now);
        await deactivateWorkstreamSemantics(ctx, workstream._id);
        await recomputeSemanticFindings(ctx, project, workspace.scopeKey, now);
      }
      return;
    }
    case "context.acknowledged": {
      const delivery = await ctx.db.query("contextDeliveries").withIndex("by_public_id", (q) => q.eq("publicId", String(payload.briefId))).unique();
      if (!delivery || delivery.projectId !== project._id) fail("brief_not_found");
      const deliveryWorkstream = await ctx.db.get(delivery.workstreamId);
      if (!deliveryWorkstream || deliveryWorkstream.workspaceId !== workspace._id) fail("forbidden");
      const considered = new Set(stringValues(payload.consideredItemIds));
      if ([...considered].some((id) => !delivery.itemRefs.includes(id))) fail("brief_item_invalid");
      await ctx.db.patch(delivery._id, { acknowledgedAt: now });
      await ctx.db.patch(workspace._id, { lastProjectedSequence: Math.max(workspace.lastProjectedSequence, event.sequence), updatedAt: now });
      return;
    }
    case "workstream.checkpoint_reported": {
      const checkpointWorkstream = await requireWorkstream(ctx, String(payload.workstreamId), project._id, workspace._id);
      if (event.sequence <= workspace.lastProjectedSequence) return;
      const checkpointTags = [
        ...stringValues(payload.affectedInterfaces).map((value) => `changes:${value}`),
        ...stringValues(payload.dependencies).map((value) => `dependency:${value}`),
      ];
      const checkpointSummary = String(payload.summary);
      await upsertSemanticIntelligence(ctx, project, checkpointWorkstream, boundedSemanticText([checkpointSummary, ...stringValues(payload.discoveries)]), "change", checkpointTags, event.source, now);
      // Two consumers read verification differently and both are deliberate.
      // Dependency readiness only promotes a claim to `ready` on structured
      // verification, because telling a blocked session to proceed on the
      // strength of prose would be a guess. Judgment also reads the bounded
      // summary, because labelling a contract change provisional is advisory
      // and being approximately right there is better than saying nothing.
      // ScopeSnapshot keeps the structured array itself so the dashboard can
      // show exactly which checks were reported without reading prose or raw
      // command output.
      const verification = scopeVerificationFacts(payload.verification);
      const latestCheckpointPassed = verification.length > 0 && verification.every((item) => item.state === "passed");
      const verificationState = reportedVerificationState(payload.verification) ?? readVerificationState(checkpointSummary);
      await ctx.db.patch(checkpointWorkstream._id, {
        latestCheckpointPassed,
        latestVerification: verification,
        verificationState,
        revision: checkpointWorkstream.revision + 1,
        updatedAt: now,
      });
      await readjudicateContractFindings(ctx, checkpointWorkstream, verificationState, now);
      const basedOnBriefId = typeof payload.basedOnBriefId === "string" ? payload.basedOnBriefId : "";
      if (basedOnBriefId) await upsertStaleAssumption(ctx, project, checkpointWorkstream, basedOnBriefId, now);
      await recomputeDependencyClaimsForProducer(ctx, project, checkpointWorkstream, now);
      await ctx.db.patch(workspace._id, { lastProjectedSequence: event.sequence, updatedAt: now });
      await bumpScope(ctx, workspace.scopeKey, now);
      return;
    }
    case "agent.activity_reported": {
      if (event.sequence <= workspace.lastProjectedSequence) return;
      const workstreamPublicId = String(payload.workstreamId);
      const vendor = String(payload.vendor) as SupportedVendor;
      const agentStatus = String(payload.status) as "active" | "waiting" | "idle" | "done" | "error";
      const activityKind = String(payload.kind);
      const currentAction = String(payload.action);
      const parsedObservedAt = Date.parse(event.observedAt);
      const lifecycleAt = Number.isNaN(parsedObservedAt) ? now : parsedObservedAt;
      let workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", workstreamPublicId)).unique();
      if (workstream && (workstream.projectId !== project._id || workstream.workspaceId !== workspace._id || workstream.vendor !== vendor)) fail("forbidden");
      const previousSessionTitle = workstream?.sessionTitle;
      const incomingPaths = stringValues(payload.paths);
      const safePaths = activityKind === "SessionStart" ? [] : [...new Set([...(workstream?.safePaths ?? []), ...incomingPaths])].sort().slice(0, 100);
      const subagents = [...(workstream?.subagents ?? [])];
      const subagentAlias = typeof payload.subagentAlias === "string" ? payload.subagentAlias : "";
      if (subagentAlias) {
        const current = subagents.find((candidate) => candidate.alias === subagentAlias);
        const next = { alias: subagentAlias, agentType: String(payload.agentType ?? "subagent"), status: activityKind === "SubagentStop" ? "done" : "active" };
        if (current) Object.assign(current, next); else subagents.push(next);
      }
      const record = {
        title: typeof payload.sessionTitle === "string" && payload.sessionTitle !== ""
          ? payload.sessionTitle
          : workstream?.sessionTitle ?? `${VENDOR_LABELS[vendor] ?? vendor} · ${String(payload.sessionAlias)}`,
        summary: currentAction, status: agentStatus === "done" ? "done" as const : agentStatus === "error" ? "blocked" as const : agentStatus === "idle" ? "idle" as const : "active" as const,
        vendor, sessionAlias: String(payload.sessionAlias), agentStatus, activityKind, currentAction,
        toolName: typeof payload.tool === "string" ? payload.tool : undefined,
        branch: typeof payload.branch === "string" && payload.branch !== "" ? payload.branch : workstream?.branch,
        sessionTitle: typeof payload.sessionTitle === "string" && payload.sessionTitle !== "" ? payload.sessionTitle : workstream?.sessionTitle,
        ...(activityKind === "SessionStart" ? { startedAt: lifecycleAt } : workstream?.startedAt === undefined ? {} : { startedAt: workstream.startedAt }),
        ...(activityKind === "SessionEnd" ? { endedAt: lifecycleAt } : workstream?.endedAt === undefined ? {} : { endedAt: workstream.endedAt }),
        safePaths, subagents: subagents.slice(0, 32), updatedAt: now,
        ...(incomingPaths.length > 0 ? { lastWritePaths: [...new Set(incomingPaths)].sort().slice(0, 100), lastWriteAt: now } : {}),
        readCoverage: readCoverageOf(payload.readCoverage) ?? workstream?.readCoverage,
      };
      if (workstream) {
        await ctx.db.patch(workstream._id, { ...record, revision: workstream.revision + 1 });
        workstream = await ctx.db.get(workstream._id);
      } else {
        const id = await ctx.db.insert("workstreams", {
          publicId: workstreamPublicId, projectId: project._id, memberId: member._id, workspaceId: workspace._id,
          scopeKey: workspace.scopeKey, revision: 1, ...record,
        });
        workstream = await ctx.db.get(id);
      }
      await ctx.db.patch(workspace._id, { lastProjectedSequence: event.sequence, updatedAt: now });
      const derivedTitle = typeof payload.sessionTitle === "string" ? payload.sessionTitle : "";
      if (workstream && derivedTitle && derivedTitle !== previousSessionTitle) {
        // A vendor-visible session title is already part of activity/v1. It may
        // seed a bounded, honestly labeled intent, but never at the cost of
        // rejecting the underlying observation when semantic policy blocks it.
        try {
          validateSemanticText(derivedTitle);
          await upsertSemanticIntelligence(ctx, project, workstream, derivedTitle, "intent", safePaths.map((path) => `path:${path}`), "hook-derived-title/v1", now);
        } catch (error) {
          if (!(error instanceof SemanticPolicyError)) throw error;
        }
      }
      if (workstream && incomingPaths.length > 0) await upsertAgentPathFindings(ctx, project, workstream, incomingPaths, now);
      if (workstream && agentStatus === "done") await resolveAgentPathFindings(ctx, workstream, now);
      await bumpScope(ctx, workspace.scopeKey, now);
      return;
    }
    case "workspace.contract_fingerprints_reported": {
      if (event.sequence <= workspace.lastProjectedSequence) return;
      if (String(payload.workspaceId) !== workspace.publicId) fail("forbidden");
      let scopeSnapshotChanged = false;
      const changedByWorkstreams = new Set<string>();
      for (const raw of Array.isArray(payload.entries) ? payload.entries : []) {
        const entry = raw as ContractFingerprintEntry;
        const changedBy = await attributeContractChange(ctx, project, workspace, payload.workstreamId, entry.path);
        const existing = await ctx.db.query("contractFingerprints")
          .withIndex("by_scope_path", (q) => q.eq("scopeKey", workspace.scopeKey).eq("path", entry.path)).unique();
        if (existing && existing.projectId !== project._id) fail("forbidden");
        const record = {
          fileContractHash: entry.fileContractHash, symbols: entry.symbols,
          changedByWorkstreamPublicId: changedBy, updatedAt: now,
          expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
        };
        if (!existing) {
          await ctx.db.insert("contractFingerprints", {
            projectId: project._id, scopeKey: workspace.scopeKey, path: entry.path, revision: 1, ...record,
          });
          await recomputeDependencyClaimsForScope(ctx, project, workspace.scopeKey, now);
          scopeSnapshotChanged = true;
          changedByWorkstreams.add(changedBy);
          continue;
        }
        // A body-only edit leaves the hash alone; nothing is compared and
        // nothing is written beyond the retention refresh.
        if (existing.fileContractHash === entry.fileContractHash) {
          await ctx.db.patch(existing._id, { updatedAt: now, expiresAt: record.expiresAt });
          await recomputeDependencyClaimsForScope(ctx, project, workspace.scopeKey, now);
          continue;
        }
        await ctx.db.patch(existing._id, { ...record, revision: existing.revision + 1 });
        scopeSnapshotChanged = true;
        changedByWorkstreams.add(changedBy);
        await upsertContractFindings(ctx, project, workspace, {
          path: entry.path, previousSymbols: existing.symbols, nextSymbols: entry.symbols,
          nextHash: entry.fileContractHash, changedByWorkstreamPublicId: changedBy, now,
        });
        await recomputeDependencyClaimsForScope(ctx, project, workspace.scopeKey, now);
      }
      if (scopeSnapshotChanged) {
        for (const changedBy of changedByWorkstreams) await bumpWorkstreamRevision(ctx, changedBy, now);
      }
      await ctx.db.patch(workspace._id, { lastProjectedSequence: event.sequence, updatedAt: now });
      await bumpScope(ctx, workspace.scopeKey, now);
      return;
    }
    case "session.read_set_reported": {
      if (event.sequence <= workspace.lastProjectedSequence) return;
      if (String(payload.workspaceId) !== workspace.publicId) fail("forbidden");
      const sessionPublicId = String(payload.sessionWorkstreamId);
      const session = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", sessionPublicId)).unique();
      if (session && (session.projectId !== project._id || session.workspaceId !== workspace._id)) fail("forbidden");
      // A read set is passive observation. A session this device has not
      // published yet is skipped rather than failing the batch behind it.
      if (session) {
        for (const raw of Array.isArray(payload.entries) ? payload.entries : []) {
          const entry = raw as { path: string; fileContractHashAtRead: string; observedAt: string; fidelity?: string };
          const existing = await ctx.db.query("sessionReadSets")
            .withIndex("by_workstream_path", (q) => q.eq("workstreamId", session._id).eq("path", entry.path)).unique();
          // A path may be reported by sources of different strength — declared
          // at begin_work, then actually observed. Keep the strongest, so a
          // later weaker report cannot quietly downgrade real evidence.
          const incoming = readFidelityOf(entry.fidelity);
          const fidelity = readFidelityRank(existing?.fidelity) > readFidelityRank(incoming) ? existing!.fidelity! : incoming;
          const record = {
            fileContractHashAtRead: entry.fileContractHashAtRead, readAt: entry.observedAt, fidelity,
            updatedAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
          };
          if (existing) await ctx.db.patch(existing._id, record);
          else await ctx.db.insert("sessionReadSets", {
            projectId: project._id, scopeKey: workspace.scopeKey, workstreamId: session._id,
            workstreamPublicId: session.publicId, path: entry.path, ...record,
          });
        }
      }
      await ctx.db.patch(workspace._id, { lastProjectedSequence: event.sequence, updatedAt: now });
      return;
    }
    case "agent.conversation_shared": {
      if (event.sequence <= workspace.lastProjectedSequence) return;
      const workstream = await requireWorkstream(ctx, String(payload.workstreamId), project._id, workspace._id);
      if (workstream.memberId !== member._id || workstream.vendor !== payload.vendor || workstream.sessionAlias !== payload.sessionAlias) fail("forbidden");
      const kind = String(payload.kind);
      const text = String(payload.text);
      validateSessionMessageText(text);
      const existing = await ctx.db.query("sessionMessages").withIndex("by_public_id", (q) => q.eq("publicId", String(payload.messageId))).unique();
      if (existing) {
        if (existing.workstreamId !== workstream._id) fail("event_id_conflict");
      } else {
        const expiresAt = now + project.retentionDays * DAY;
        await ctx.db.insert("sessionMessages", {
          publicId: String(payload.messageId), workstreamId: workstream._id, projectId: project._id, memberId: member._id,
          vendor: payload.vendor as SupportedVendor, kind: kind as "user" | "assistant" | "thinking" | "system",
          text, capturedAt: now, expiresAt,
        });
      }
      await ctx.db.patch(workspace._id, { lastProjectedSequence: event.sequence, updatedAt: now });
      return;
    }
    case "workspace.manifest_started": {
      const publicId = String(payload.manifestId);
      if (await ctx.db.query("changeManifests").withIndex("by_public_id", (q) => q.eq("publicId", publicId)).unique()) fail("manifest_exists");
      let workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", String(payload.workstreamId))).unique();
      if (!workstream) {
        const workstreamId = await ctx.db.insert("workstreams", {
          publicId: String(payload.workstreamId), projectId: project._id, memberId: member._id, workspaceId: workspace._id,
          scopeKey: workspace.scopeKey, title: "Manifest workstream", summary: "Structural metadata only",
          status: "active", revision: 1, updatedAt: now,
        });
        workstream = await ctx.db.get(workstreamId);
      }
      if (!workstream || workstream.projectId !== project._id || workstream.workspaceId !== workspace._id) fail("forbidden");
      await ctx.db.insert("changeManifests", {
        publicId, projectId: project._id, scopeKey: workspace.scopeKey, workstreamId: workstream._id,
        revision: Number(payload.revision), baselineRef: String(payload.baselineRef), headRef: String(payload.headRef),
        expectedChunks: Number(payload.chunkCount), pathCount: 0, state: "staging", createdAt: now,
        expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
      });
      return;
    }
    case "workspace.manifest_chunk": {
      const manifest = await requireManifest(ctx, String(payload.manifestId), project._id, workspace.scopeKey);
      if (manifest.state !== "staging") fail("manifest_not_staging");
      const chunkIndex = Number(payload.chunkIndex);
      if (chunkIndex >= manifest.expectedChunks) fail("chunk_index_out_of_range");
      if (await ctx.db.query("changeManifestChunks").withIndex("by_manifest_chunk", (q) => q.eq("manifestId", manifest._id).eq("chunkIndex", chunkIndex)).unique()) {
        fail("duplicate_chunk");
      }
      await ctx.db.insert("changeManifestChunks", {
        manifestId: manifest._id, chunkIndex, entries: payload.entries as ManifestEntry[], expiresAt: manifest.expiresAt,
      });
      return;
    }
    case "workspace.manifest_completed": {
      const manifest = await requireManifest(ctx, String(payload.manifestId), project._id, workspace.scopeKey);
      if (manifest.state !== "staging" || manifest.revision !== Number(payload.revision)) fail("manifest_revision_conflict");
      const chunks = await ctx.db.query("changeManifestChunks").withIndex("by_manifest_chunk", (q) => q.eq("manifestId", manifest._id)).collect();
      chunks.sort((left, right) => left.chunkIndex - right.chunkIndex);
      if (chunks.length !== manifest.expectedChunks || chunks.some((chunk, index) => chunk.chunkIndex !== index)) fail("manifest_incomplete");
      const entries = chunks.flatMap((chunk) => chunk.entries) as ManifestEntry[];
      try {
        assertCanonicalManifestOrder(entries);
      } catch (error) {
        if (error instanceof ValidationError) fail(error.code);
        throw error;
      }
      const hash = manifestContentHash(entries);
      if (hash !== String(payload.contentHash)) fail("manifest_hash_mismatch");
      const workstream = await ctx.db.get(manifest.workstreamId);
      if (!workstream) fail("workstream_not_found");
      if (workstream.currentManifestId) {
        const previous = await ctx.db.get(workstream.currentManifestId);
        if (previous && !canActivateManifestRevision(previous.revision, manifest.revision)) {
          // Keep the out-of-order event durable and acknowledged, but do not
          // let its stale projection replace the newer active snapshot.
          await ctx.db.patch(manifest._id, {
            state: "superseded", contentHash: hash, pathCount: entries.length, activatedAt: now,
          });
          return;
        }
        if (previous) await ctx.db.patch(previous._id, { state: "superseded" });
      }
      await ctx.db.patch(manifest._id, { state: "active", contentHash: hash, pathCount: entries.length, activatedAt: now });
      await ctx.db.patch(workstream._id, { currentManifestId: manifest._id, revision: workstream.revision + 1, updatedAt: now });
      if (workstream.vendor === undefined) {
        // Git observes the combined checkout. Whatever no live agent session
        // accounts for is the adapterless member's work - the only evidence
        // they leave (B29). Claimed-by-anyone paths stay unattributed here:
        // same-file edits by an agent and a manual member in one checkout are
        // genuinely indistinguishable, and claiming otherwise would fabricate
        // coverage.
        const scopePeers = await ctx.db.query("workstreams").withIndex("by_scope", (q) => q.eq("scopeKey", workspace.scopeKey)).collect();
        const claimed = new Set<string>();
        for (const peer of scopePeers) {
          if (!peer.vendor || peer.status === "done") continue;
          for (const path of peer.safePaths ?? []) claimed.add(path);
          for (const path of peer.lastWritePaths ?? []) claimed.add(path);
        }
        await ctx.db.patch(workstream._id, {
          residualPaths: residualManifestPaths(entries.map((entry) => entry.path), claimed),
          residualAt: now,
        });
      }
      await upsertPathFindings(ctx, project, manifest, workstream, entries, now);
      await bumpScope(ctx, workspace.scopeKey, now);
      return;
    }
  }
}

type ContractSymbol = { name: string; kind: string; signature: string; signatureHash: string };
type ContractFingerprintEntry = { path: string; fileContractHash: string; symbols: ContractSymbol[] };
type ChangedSymbol = { name: string; oldSignature: string; newSignature: string };

// attributeContractChange resolves who changed a contract. A publisher that
// names its workstream is believed once the workstream is confirmed to belong
// to this workspace; a publisher that does not, or that names a workstream this
// service has not projected yet, falls back to derivation.
export async function attributeContractChange(
  ctx: MutationCtx,
  project: Doc<"projects">,
  workspace: Doc<"workspaces">,
  reported: unknown,
  path: string,
): Promise<string> {
  let named: Doc<"workstreams"> | null = null;
  if (typeof reported === "string" && reported !== "") {
    named = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", reported)).unique();
    if (named && (named.projectId !== project._id || named.workspaceId !== workspace._id)) fail("forbidden");
    // An agent that explicitly published its own fingerprint remains exact.
    if (named?.vendor) return named.publicId;
  }
  // Git observes the combined checkout, so its event names the workspace
  // workstream. Hook mutation paths are the stronger per-session attribution:
  // choose the most recently active agent in this workspace whose latest
  // mutation event reported this exact path. Read-tool paths never enter this
  // field, and replacing rather than accumulating it avoids attributing a
  // later manual edit to an agent that touched the path much earlier.
  const candidates = await ctx.db.query("workstreams").withIndex("by_scope", (q) => q.eq("scopeKey", workspace.scopeKey)).collect();
  const author = candidates
    .filter((candidate) => candidate.workspaceId === workspace._id && candidate.vendor !== undefined && candidate.status !== "done" && candidate.lastWritePaths?.includes(path))
    .sort((left, right) => (right.lastWriteAt ?? 0) - (left.lastWriteAt ?? 0))[0];
  if (author) return author.publicId;
  if (named) return named.publicId;
  return changingWorkstream(ctx, workspace);
}

// changingWorkstream attributes a workspace-scoped contract change to the
// workstream most plausibly responsible: the workspace's most recently active
// one. It is the fallback for events published without a workstreamId.
async function changingWorkstream(ctx: MutationCtx, workspace: Doc<"workspaces">): Promise<string> {
  const candidates = await ctx.db.query("workstreams").withIndex("by_scope", (q) => q.eq("scopeKey", workspace.scopeKey)).collect();
  const owned = candidates.filter((candidate) => candidate.workspaceId === workspace._id);
  const live = owned.filter((candidate) => candidate.status !== "done");
  const ordered = (live.length > 0 ? live : owned).sort((left, right) => right.updatedAt - left.updatedAt);
  return ordered[0]?.publicId ?? workspace.publicId;
}

// diffContractSymbols reports only the symbols a reader could already depend
// on: those whose signature moved and those that disappeared. A file that only
// gained symbols has nothing that can invalidate an earlier read.
function diffContractSymbols(previous: readonly ContractSymbol[], next: readonly ContractSymbol[]): ChangedSymbol[] {
  const key = (symbol: ContractSymbol) => `${symbol.kind}\u0000${symbol.name}`;
  const nextByKey = new Map(next.map((symbol) => [key(symbol), symbol]));
  const changed: ChangedSymbol[] = [];
  for (const before of previous) {
    const after = nextByKey.get(key(before));
    if (!after) changed.push({ name: before.name, oldSignature: before.signature, newSignature: "" });
    else if (after.signatureHash !== before.signatureHash) changed.push({ name: before.name, oldSignature: before.signature, newSignature: after.signature });
  }
  return changed.sort((left, right) => left.name.localeCompare(right.name));
}

// upsertContractFindings raises one stale_assumption finding per (session,
// path, new hash) for every other live session whose read set holds an older
// contract for the path. The fingerprint makes redelivery a no-op.
export async function upsertContractFindings(
  ctx: MutationCtx,
  project: Doc<"projects">,
  workspace: Doc<"workspaces">,
  change: {
    path: string;
    previousSymbols: readonly ContractSymbol[];
    nextSymbols: readonly ContractSymbol[];
    nextHash: string;
    changedByWorkstreamPublicId: string;
    now: number;
  },
): Promise<void> {
  const changedSymbols = diffContractSymbols(change.previousSymbols, change.nextSymbols).slice(0, 16);
  if (changedSymbols.length === 0) return;
  const readers = await ctx.db.query("sessionReadSets")
    .withIndex("by_scope_path", (q) => q.eq("scopeKey", workspace.scopeKey).eq("path", change.path)).take(129);
  if (readers.length > 128) fail("read_set_scope_too_large");
  const changedAt = new Date(change.now).toISOString();
  const changer = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", change.changedByWorkstreamPublicId)).unique();
  const changerTitle = changer?.title ?? change.changedByWorkstreamPublicId;
  const changerVerification = (changer?.verificationState as VerificationState | undefined) ?? "unknown";
  for (const reader of readers) {
    if (reader.workstreamPublicId === change.changedByWorkstreamPublicId) continue;
    if (reader.fileContractHashAtRead === change.nextHash) continue;
    const session = await ctx.db.get(reader.workstreamId);
    if (!session || session.status === "done") continue;
    const fingerprint = sha256Hex(`${workspace.scopeKey}\u0000contract\u0000${reader.workstreamPublicId}\u0000${change.path}\u0000${change.nextHash}`);
    const primary = changedSymbols[0]!;
    const others = changedSymbols.length > 1 ? ` ${changedSymbols.length - 1} other symbol(s) in the file also changed.` : "";
    // The symbol diff is structural either way; what varies is the strength of
    // the claim that this session read the file at all (ADR-052).
    //
    // These two are declared before `reason`, which reads `readQualifier`.
    // Declared after it, every stale_assumption threw a temporal-dead-zone
    // ReferenceError inside publishEvents, which failed the whole event batch —
    // so no session of any vendor could receive a stale-assumption correction,
    // and the device's queue stalled behind the throwing batch.
    const readFidelity = readFidelityOf(reader.fidelity);
    const readQualifier = readFidelity === "observed"
      ? "read"
      : readFidelity === "vendor_inferred"
        ? "appears to have read"
        : "said it expected to read";
    const reason = boundedText(
      `${change.path}: ${primary.name} changed after this session ${readQualifier} it (was ${primary.oldSignature || "absent"}; now ${primary.newSignature || "removed"}).${others}`,
      500,
    );
    const evidence = [{
      kind: "symbol",
      summary: boundedText(`${change.path}: ${changedSymbols.map((symbol) => symbol.name).join(", ")} no longer match what this session ${readQualifier}.`, 500),
      source: "git",
      fidelity: readFidelity === "observed" ? "structural" : `structural/${readFidelity}`,
      contract: {
        readFidelity,
        path: change.path, changedSymbols, changedByWorkstreamId: change.changedByWorkstreamPublicId,
        readAt: reader.readAt, changedAt,
      },
    }];
    const candidate = contractCandidate(reason, session, reader.workstreamPublicId, change.changedByWorkstreamPublicId, changerTitle, changerVerification, change.path, changer?.branch);
    const verdict = deterministicJudgment(candidate);
    const existing = await ctx.db.query("findings").withIndex("by_fingerprint", (q) => q.eq("fingerprint", fingerprint)).unique();
    if (existing) {
      await ctx.db.patch(existing._id, { lastSeenAt: change.now, evidence, reason: verdict.explanation, severity: verdict.severity, confidenceBand: contractConfidenceBand(readFidelity), delivery: verdict.delivery, state: "open", expiresAt: change.now + DEFAULT_RETENTION_DAYS * DAY });
      continue;
    }
    const publicId = `fnd_${fingerprint.slice(0, 32)}`;
    await ctx.db.insert("findings", {
      publicId, projectId: project._id, scopeKey: workspace.scopeKey,
      kind: "stale_assumption", severity: verdict.severity, confidenceBand: contractConfidenceBand(readFidelity),
      workstreamPublicIds: [reader.workstreamPublicId], evidence, reason: verdict.explanation, state: "open", fingerprint,
      engineVersion: CONTRACT_ENGINE_VERSION, delivery: verdict.delivery, revision: 1, firstSeenAt: change.now, lastSeenAt: change.now,
      expiresAt: change.now + DEFAULT_RETENTION_DAYS * DAY,
    });
    await scheduleAdjudication(ctx, publicId, 1, candidate, verdict);
  }
}

// contractCandidate frames a contract drift for the judgment layer. The reader
// and the changer are distinct roles: whether the change is settled or
// work-in-progress is a fact about the changer, and it is what separates an
// urgent correction from a provisional heads-up.
function contractCandidate(
  reason: string,
  session: Doc<"workstreams">,
  readerPublicId: string,
  changerPublicId: string,
  changerTitle: string,
  changerVerification: VerificationState,
  path: string,
  changerBranch?: string,
): JudgmentCandidate {
  return {
    kind: "stale_assumption", severity: "high", confidence: "high", reason,
    signalKind: "symbol", sharedSignals: [path],
    workstreams: [
      { id: readerPublicId, title: session.title, summary: session.summary, status: session.status, reportedChange: false, verification: "unknown", role: "read", ...(session.branch ? { branch: session.branch } : {}) },
      { id: changerPublicId, title: changerTitle, summary: changerTitle, status: "active", reportedChange: true, verification: changerVerification, role: "changed", ...(changerBranch ? { branch: changerBranch } : {}) },
    ],
    trackedContractSymbols: [], structurallyUnambiguous: false,
  };
}

// readjudicateContractFindings re-runs judgment for open contract drift a
// workstream caused, after that workstream reports what verification it ran.
// Evidence usually arrives before its context does: the fingerprint change is
// published by the scanner, and the checkpoint that says whether the change is
// finished follows it.
async function readjudicateContractFindings(
  ctx: MutationCtx,
  workstream: Doc<"workstreams">,
  verification: VerificationState,
  now: number,
): Promise<void> {
  const findings = await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).take(513);
  if (findings.length > 512) fail("finding_scope_too_large");
  for (const finding of findings) {
    if (finding.engineVersion !== CONTRACT_ENGINE_VERSION || finding.state !== "open") continue;
    const evidence = Array.isArray(finding.evidence) ? finding.evidence as Array<{ contract?: { changedByWorkstreamId?: string; path?: string } }> : [];
    const contract = evidence.find((item) => item.contract?.changedByWorkstreamId === workstream.publicId)?.contract;
    if (!contract) continue;
    const reader = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", finding.workstreamPublicIds[0] ?? "")).unique();
    if (!reader) continue;
    const candidate = contractCandidate(finding.reason, reader, reader.publicId, workstream.publicId, workstream.title, verification, contract.path ?? "", workstream.branch);
    const verdict = deterministicJudgment(candidate);
    if (finding.severity === verdict.severity && finding.reason === verdict.explanation && finding.delivery === verdict.delivery) continue;
    await ctx.db.patch(finding._id, {
      severity: verdict.severity, reason: verdict.explanation, delivery: verdict.delivery,
      revision: finding.revision + 1, lastSeenAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
    });
    await scheduleAdjudication(ctx, finding.publicId, finding.revision + 1, candidate, verdict);
  }
}

// Dependency readiness mirrors contract drift but targets the declaring
// workstream. Its fingerprint excludes readiness state so verification upgrades
// the same finding revision instead of producing a second notification.
async function recomputeDependencyReadiness(
  ctx: MutationCtx,
  project: Doc<"projects">,
  claimant: Doc<"workstreams">,
  now: number,
): Promise<void> {
  const claims = claimant.status === "done" ? [] : claimant.waitingOn ?? [];
  const scopedFindings = await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", claimant.scopeKey)).take(513);
  if (scopedFindings.length > 512) fail("finding_scope_too_large");
  const dependencyFindings = scopedFindings.filter((finding) =>
    finding.engineVersion === DEPENDENCY_ENGINE_VERSION && finding.workstreamPublicIds.length === 1 && finding.workstreamPublicIds[0] === claimant.publicId);
  const activeClaims = new Set(claims.map((claim) => claim.toLocaleLowerCase("en-US")));
  for (const finding of dependencyFindings) {
    const claim = typeof finding.inputRevisions === "object" && finding.inputRevisions !== null &&
      typeof (finding.inputRevisions as Record<string, unknown>).claim === "string"
      ? String((finding.inputRevisions as Record<string, unknown>).claim) : "";
    if (finding.state === "open" && !activeClaims.has(claim.toLocaleLowerCase("en-US"))) {
      await ctx.db.patch(finding._id, { state: "resolved", revision: finding.revision + 1, lastSeenAt: now });
    }
  }
  if (claims.length === 0) return;

  const fingerprints = await ctx.db.query("contractFingerprints")
    .withIndex("by_scope_path", (q) => q.eq("scopeKey", claimant.scopeKey)).take(513);
  if (fingerprints.length > 512) fail("contract_scope_too_large");
  const workstreams = await ctx.db.query("workstreams").withIndex("by_scope", (q) => q.eq("scopeKey", claimant.scopeKey)).take(257);
  if (workstreams.length > 256) fail("workstream_scope_too_large");
  const workstreamByPublicId = new Map(workstreams.map((workstream) => [workstream.publicId, workstream]));
  const candidates = fingerprints.flatMap((fingerprint) => {
    const producer = workstreamByPublicId.get(fingerprint.changedByWorkstreamPublicId);
    if (!producer) return [];
    return [{
      projectId: project.publicId,
      scopeKey: fingerprint.scopeKey,
      workstreamId: producer.publicId,
      status: producer.status,
      path: fingerprint.path,
      symbols: fingerprint.symbols.map((symbol) => symbol.name),
      latestCheckpointPassed: producer.latestCheckpointPassed,
    }];
  });

  for (const claim of claims) {
    const satisfaction = findDependencySatisfaction(project.publicId, claimant.scopeKey, claimant.publicId, claim, candidates);
    const fingerprint = sha256Hex(`${claimant.scopeKey}\0dependency\0${claimant.publicId}\0${claim.toLocaleLowerCase("en-US")}`);
    const existing = await ctx.db.query("findings").withIndex("by_fingerprint", (q) => q.eq("fingerprint", fingerprint)).unique();
    if (!satisfaction) {
      if (existing?.state === "open") await ctx.db.patch(existing._id, { state: "resolved", revision: existing.revision + 1, lastSeenAt: now });
      continue;
    }
    const ready = satisfaction.state === "ready";
    const severity = ready ? "high" : "medium";
    const reason = ready
      ? `${claim} is ready: ${satisfaction.satisfiedBy.path} now exposes the matching contract and its producer reports passing verification.`
      : `${claim} has a stable work-in-progress contract at ${satisfaction.satisfiedBy.path}, but the producing workstream is unverified.`;
    const evidence = [{
      kind: "dependency",
      summary: `${claim} is satisfied by ${satisfaction.satisfiedBy.path} (${satisfaction.state}).`,
      source: "git",
      fidelity: "structural",
      subject: claim,
      dependency: satisfaction,
    }];
    if (existing) {
      const previousState = Array.isArray(existing.evidence)
        ? (existing.evidence as Array<{ dependency?: { state?: string } }>)[0]?.dependency?.state : undefined;
      const material = existing.state !== "open" || previousState !== satisfaction.state || existing.reason !== reason || existing.severity !== severity;
      await ctx.db.patch(existing._id, {
        severity, evidence, reason, state: "open", revision: existing.revision + (material ? 1 : 0),
        lastSeenAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
      });
      continue;
    }
    await ctx.db.insert("findings", {
      publicId: `fnd_${fingerprint.slice(0, 32)}`,
      projectId: project._id,
      scopeKey: claimant.scopeKey,
      kind: "dependency_ready",
      severity,
      confidenceBand: "deterministic",
      workstreamPublicIds: [claimant.publicId],
      evidence,
      reason,
      state: "open",
      fingerprint,
      engineVersion: DEPENDENCY_ENGINE_VERSION,
      inputRevisions: { claim },
      revision: 1,
      firstSeenAt: now,
      lastSeenAt: now,
      expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
    });
  }
}

async function recomputeDependencyClaimsForScope(
  ctx: MutationCtx,
  project: Doc<"projects">,
  scopeKeyValue: string,
  now: number,
): Promise<void> {
  const workstreams = await ctx.db.query("workstreams").withIndex("by_scope", (q) => q.eq("scopeKey", scopeKeyValue)).take(257);
  if (workstreams.length > 256) fail("workstream_scope_too_large");
  for (const workstream of workstreams) {
    if ((workstream.waitingOn?.length ?? 0) > 0) await recomputeDependencyReadiness(ctx, project, workstream, now);
  }
}

async function recomputeDependencyClaimsForProducer(
  ctx: MutationCtx,
  project: Doc<"projects">,
  producer: Doc<"workstreams">,
  now: number,
): Promise<void> {
  await recomputeDependencyClaimsForScope(ctx, project, producer.scopeKey, now);
}

async function resolveDependencyFindings(ctx: MutationCtx, claimant: Doc<"workstreams">, now: number): Promise<void> {
  const findings = await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", claimant.scopeKey)).take(513);
  if (findings.length > 512) fail("finding_scope_too_large");
  for (const finding of findings) {
    if (finding.engineVersion === DEPENDENCY_ENGINE_VERSION && finding.state === "open" &&
      finding.workstreamPublicIds.length === 1 && finding.workstreamPublicIds[0] === claimant.publicId) {
      await ctx.db.patch(finding._id, { state: "resolved", revision: finding.revision + 1, lastSeenAt: now });
    }
  }
}

function boundedText(value: string, maximum: number): string {
  return value.length <= maximum ? value : `${value.slice(0, maximum - 1)}…`;
}

// ---------------------------------------------------------------------------
// Judgment layer (ADR-045)
//
// Deterministic evidence decides that something happened; these helpers decide
// what it means and where the answer belongs. Every verdict on this path is
// computed offline and synchronously, so a finding is never delayed by, or
// dependent on, a hosted model. The managed adjudicator is scheduled after the
// coordination object is durable and only improves the wording.
// ---------------------------------------------------------------------------

const TRACKED_CONTRACT_SYMBOL_LIMIT = 512;

// trackedContractSymbols lists the symbols the contract-fingerprint engine
// already reports exactly for a scope. A coarse notice about one of them
// repeats work the exact engine does better, so the judgment layer silences it.
async function trackedContractSymbols(ctx: MutationCtx, key: string): Promise<string[]> {
  const rows = await ctx.db.query("contractFingerprints").withIndex("by_scope_path", (q) => q.eq("scopeKey", key)).take(257);
  if (rows.length > 256) fail("contract_scope_too_large");
  const symbols = new Set<string>();
  for (const row of rows) {
    for (const symbol of row.symbols) {
      if (symbols.size >= TRACKED_CONTRACT_SYMBOL_LIMIT) break;
      symbols.add(symbol.name);
    }
  }
  return [...symbols];
}

// scheduleAdjudication asks the managed provider to improve an explanation
// after the coordination object is durable. It is skipped for candidates that
// explain themselves and for verdicts nobody will read.
async function scheduleAdjudication(
  ctx: MutationCtx,
  findingPublicId: string,
  revision: number,
  candidate: JudgmentCandidate,
  verdict: JudgmentVerdict,
): Promise<void> {
  if (candidate.structurallyUnambiguous || verdict.delivery === "silent" || candidate.workstreams.length < 2) return;
  await ctx.scheduler.runAfter(0, internal.intelligence.adjudicateFinding, {
    findingPublicId, expectedRevision: revision, candidate: candidate as unknown as Record<string, unknown>,
  });
}

function judgmentSeverity(value: string): JudgmentSeverity {
  return (["low", "medium", "high", "critical"].includes(value) ? value : "medium") as JudgmentSeverity;
}

function judgmentConfidence(band: string): "high" | "medium" | "low" {
  return band === "deterministic" || band === "high" ? "high" : band === "low" ? "low" : "medium";
}

// reportedVerificationState reads the structured verification summaries a
// checkpoint carried, when it carried any. A single failing or unfinished
// check is enough to call the whole checkpoint unverified.
function reportedVerificationState(reported: unknown): VerificationState | undefined {
  if (!Array.isArray(reported) || reported.length === 0) return undefined;
  const states = reported.map((entry) => String((entry as { state?: unknown }).state ?? ""));
  if (states.some((state) => state !== "passed")) return "unverified";
  return "passed";
}

function scopeVerificationFacts(reported: unknown): ScopeVerificationFact[] {
  if (!Array.isArray(reported)) return [];
  return reported.map((raw) => {
    const item = raw as Record<string, unknown>;
    return {
      state: String(item.state) as ScopeVerificationFact["state"],
      checkKind: String(item.checkKind),
      label: String(item.label),
      summary: String(item.summary),
      ...(typeof item.affectedComponent === "string" ? { affectedComponent: item.affectedComponent } : {}),
      ...(typeof item.manifestRevision === "number" ? { manifestRevision: item.manifestRevision } : {}),
      source: String(item.source) as ScopeVerificationFact["source"],
      ...(typeof item.observedAt === "string" ? { observedAt: item.observedAt } : {}),
    };
  });
}

function overlapValues(left: readonly string[] = [], right: readonly string[] = []): string[] {
  const lowered = new Set(right.map((value) => value.toLowerCase()));
  return [...new Set(left.filter((value) => lowered.has(value.toLowerCase())))].sort();
}

// pairCandidate restates a deterministic pair finding as something the
// judgment layer can reason about: which concrete signal the two workstreams
// share, and what each of them has actually reported so far.
function pairCandidate(
  finding: IntelligenceFinding,
  records: Map<string, WorkstreamRecord>,
  states: Map<string, { title: string; reportedChange: boolean; verification: VerificationState; branch?: string }>,
  tracked: readonly string[],
): JudgmentCandidate {
  const left = records.get(finding.workstreamIds[0] ?? "");
  const right = records.get(finding.workstreamIds[1] ?? "");
  let signalKind: JudgmentSignalKind = "semantic";
  let sharedSignals: string[] = [];
  if (left && right) {
    const paths = overlapValues(left.paths, right.paths);
    const dependencies = overlapValues(left.dependencies, right.dependencies);
    // evaluatePair folds declared contracts into the schema comparison, so the
    // judgment layer reads the same union back out.
    const contracts = overlapValues([...(left.schemas ?? []), ...(left.contracts ?? [])], [...(right.schemas ?? []), ...(right.contracts ?? [])]);
    const routes = overlapValues(left.routes, right.routes);
    if (finding.kind === "direct_collision" && paths.length > 0) { signalKind = "path"; sharedSignals = paths; }
    else if (finding.kind === "shared_dependency") {
      if (dependencies.length > 0) { signalKind = "dependency"; sharedSignals = dependencies; }
      else if (contracts.length > 0) { signalKind = "contract"; sharedSignals = contracts; }
      else { signalKind = "dependency"; sharedSignals = routes; }
    } else if (finding.kind === "downstream_impact") { signalKind = "dependency"; sharedSignals = [...dependencies, ...contracts]; }
    else if (finding.kind === "assumption_conflict") { signalKind = "assumption"; sharedSignals = overlapValues(left.assumptions, right.assumptions); }
    else { signalKind = "semantic"; sharedSignals = overlapValues(left.components, right.components); }
  }
  const workstreams = finding.workstreamIds.map((id) => {
    const record = records.get(id);
    const state = states.get(id);
    return {
      id, title: state?.title ?? id, summary: record?.summary ?? "", status: record?.status ?? "active",
      reportedChange: state?.reportedChange ?? false, verification: state?.verification ?? "unknown",
      role: "peer" as const,
      // Branch is coordination metadata the workstream already carries. The
      // judgment layer reads it as evidence of how the overlap will surface,
      // never as a reason to stay quiet (ADR-061).
      ...(state?.branch ? { branch: state.branch } : {}),
    };
  });
  return {
    kind: finding.kind, severity: judgmentSeverity(finding.severity), confidence: judgmentConfidence(finding.confidenceBand),
    reason: finding.reason, signalKind, sharedSignals, workstreams, trackedContractSymbols: tracked,
    structurallyUnambiguous: finding.kind === "direct_collision" && sharedSignals.length > 0,
  };
}

async function upsertPathFindings(
  ctx: MutationCtx,
  project: Doc<"projects">,
  manifest: Doc<"changeManifests">,
  workstream: Doc<"workstreams">,
  entries: ManifestEntry[],
  now: number,
): Promise<void> {
  const ownPaths = new Set(entries.map((entry) => entry.path));
  const peers = await ctx.db.query("workstreams").withIndex("by_scope", (q) => q.eq("scopeKey", manifest.scopeKey)).collect();
  for (const peer of peers) {
    if (peer._id === workstream._id || peer.status === "done" || !peer.currentManifestId) continue;
    const peerManifest = await ctx.db.get(peer.currentManifestId);
    if (!peerManifest || peerManifest.state !== "active") continue;
    const peerChunks = await ctx.db.query("changeManifestChunks").withIndex("by_manifest_chunk", (q) => q.eq("manifestId", peerManifest._id)).collect();
    const overlaps = [...new Set(peerChunks.flatMap((chunk) => chunk.entries.map((entry) => entry.path)).filter((path) => ownPaths.has(path)))].sort();
    for (const path of overlaps.slice(0, 100)) {
      const workstreamIds = [workstream.publicId, peer.publicId].sort();
      const fingerprint = sha256Hex(`${manifest.scopeKey}\0${workstreamIds.join("\0")}\0${path}`);
      const existing = await ctx.db.query("findings").withIndex("by_fingerprint", (q) => q.eq("fingerprint", fingerprint)).unique();
      const evidence = [{ kind: "path", summary: `Both active manifests include ${path}.`, source: "git", fidelity: "structural" }];
      if (existing) {
        await ctx.db.patch(existing._id, {
          lastSeenAt: now,
          evidence,
          state: "open",
          expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
        });
      } else {
        await ctx.db.insert("findings", {
          publicId: `fnd_${fingerprint.slice(0, 32)}`,
          projectId: project._id,
          scopeKey: manifest.scopeKey,
          kind: "direct_collision",
          severity: "medium",
          confidenceBand: "deterministic",
          workstreamPublicIds: workstreamIds,
          evidence,
          reason: `Active workstreams overlap on ${path}.`,
          state: "open",
          delivery: decideDelivery(relationshipForKind("direct_collision"), "medium"),
          fingerprint,
          engineVersion: "structural/v1",
          revision: 1,
          firstSeenAt: now,
          lastSeenAt: now,
          expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
        });
      }
    }
  }
}

// residualManifestPaths keeps the manifest paths no live agent session has
// claimed, bounded and sorted for stable fingerprints.
export function residualManifestPaths(paths: readonly string[], claimed: ReadonlySet<string>): string[] {
  return [...new Set(paths.filter((path) => !claimed.has(path)))].sort().slice(0, 200);
}

export async function upsertAgentPathFindings(
  ctx: MutationCtx,
  project: Doc<"projects">,
  workstream: Doc<"workstreams">,
  incomingPaths: string[],
  now: number,
): Promise<void> {
  const ownPaths = new Set(incomingPaths);
  const peers = await ctx.db.query("workstreams").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).collect();
  for (const peer of peers) {
    if (peer._id === workstream._id || !peer.vendor || peer.status === "done" || now - peer.updatedAt > 120_000) continue;
    const overlaps = [...new Set((peer.safePaths ?? []).filter((path) => ownPaths.has(path)))].sort();
    for (const path of overlaps) {
      const workstreamIds = [workstream.publicId, peer.publicId].sort();
      const fingerprint = sha256Hex(`${workstream.scopeKey}\0agent-hook\0${workstreamIds.join("\0")}\0${path}`);
      const existing = await ctx.db.query("findings").withIndex("by_fingerprint", (q) => q.eq("fingerprint", fingerprint)).unique();
      // Two agents editing one file is real information but not, by itself,
      // interference: different single lines of shared/settings.ts drew a
      // high/next_turn interruption for two fully compatible edits (B26),
      // which is the "trains people to ignore it" mode. Bare file overlap is
      // a quiet structural notice. Interruption needs corroboration, and the
      // deterministic evidence available here is the file's contract having
      // moved while both sessions were live - then someone's assumptions
      // about the file are genuinely at risk, not merely nearby.
      const contract = await ctx.db.query("contractFingerprints")
        .withIndex("by_scope_path", (q) => q.eq("scopeKey", workstream.scopeKey).eq("path", path)).unique();
      const bothLiveSince = Math.max(workstream.startedAt ?? now, peer.startedAt ?? now);
      const corroborated = contract !== null && contract.revision > 1 && contract.updatedAt >= bothLiveSince;
      const severity = corroborated ? ("high" as const) : ("medium" as const);
      const evidence = [{ kind: "path", summary: corroborated
        ? `Both active agent sessions reported work on ${path}, and its contract changed while both were live.`
        : `Both active agent sessions reported work on ${path}.`, source: "hook", fidelity: "structural" }];
      if (existing) {
        await ctx.db.patch(existing._id, {
          lastSeenAt: now, evidence, state: "open", expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
          // Corroboration can arrive after the first sighting; the finding
          // escalates in place rather than waiting to be re-raised. It never
          // steps back down: the contract movement already happened.
          ...(corroborated && existing.severity !== "high"
            ? { severity, delivery: decideDelivery(relationshipForKind("direct_collision"), severity), revision: existing.revision + 1 }
            : {}),
        });
      } else {
        await ctx.db.insert("findings", {
          publicId: `fnd_${fingerprint.slice(0, 32)}`, projectId: project._id, scopeKey: workstream.scopeKey,
          kind: "direct_collision", severity, confidenceBand: "deterministic", workstreamPublicIds: workstreamIds,
          evidence, reason: `Active ${workstream.vendor} and ${peer.vendor} sessions overlap on ${path}.`, state: "open",
          delivery: decideDelivery(relationshipForKind("direct_collision"), severity),
          fingerprint, engineVersion: "agent-path/v1", revision: 1, firstSeenAt: now, lastSeenAt: now,
          expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
        });
      }
    }
  }
  // A member without an adapter leaves only residual git evidence on the
  // workspace workstream (B29). Attribution by elimination is real but weaker
  // than a hook report, so the finding is capped at a quiet dashboard notice
  // with a non-deterministic band - honest about what was actually observed.
  for (const peer of peers) {
    if (peer._id === workstream._id || peer.vendor !== undefined || peer.status === "done") continue;
    if (peer.residualAt === undefined || now - peer.residualAt > 600_000) continue;
    const overlaps = [...new Set((peer.residualPaths ?? []).filter((path) => ownPaths.has(path)))].sort();
    for (const path of overlaps) {
      const workstreamIds = [workstream.publicId, peer.publicId].sort();
      const fingerprint = sha256Hex(`${workstream.scopeKey} residual ${workstreamIds.join(" ")} ${path}`);
      const existing = await ctx.db.query("findings").withIndex("by_fingerprint", (q) => q.eq("fingerprint", fingerprint)).unique();
      const evidence = [{ kind: "path", summary: `An agent session reported work on ${path}, which recent git history attributes to a member working without an adapter.`, source: "git", fidelity: "residual" }];
      if (existing) {
        await ctx.db.patch(existing._id, { lastSeenAt: now, evidence, state: "open", expiresAt: now + DEFAULT_RETENTION_DAYS * DAY });
      } else {
        await ctx.db.insert("findings", {
          publicId: `fnd_${fingerprint.slice(0, 32)}`, projectId: project._id, scopeKey: workstream.scopeKey,
          kind: "direct_collision", severity: "medium", confidenceBand: "medium", workstreamPublicIds: workstreamIds,
          evidence, reason: `An active ${workstream.vendor} session and a member without an adapter overlap on ${path}.`, state: "open",
          delivery: decideDelivery(relationshipForKind("direct_collision"), "medium"),
          fingerprint, engineVersion: "agent-path/v1", revision: 1, firstSeenAt: now, lastSeenAt: now,
          expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
        });
      }
    }
  }
}

async function resolveAgentPathFindings(ctx: MutationCtx, workstream: Doc<"workstreams">, now: number): Promise<void> {
  const findings = await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).take(513);
  if (findings.length > 512) fail("finding_scope_too_large");
  for (const finding of findings) {
    if (finding.engineVersion === "agent-path/v1" && finding.state === "open" && finding.workstreamPublicIds.includes(workstream.publicId)) {
      await ctx.db.patch(finding._id, { state: "resolved", revision: finding.revision + 1, lastSeenAt: now });
    }
  }
}

async function upsertSemanticIntelligence(
  ctx: MutationCtx,
  project: Doc<"projects">,
  workstream: Doc<"workstreams">,
  rawText: string,
  kind: "intent" | "change",
  rawTags: string[],
  source: string,
  now: number,
): Promise<void> {
  let text: string;
  let tags: string[];
  try {
    text = validateSemanticText(rawText);
    tags = validateSemanticTags(rawTags);
  } catch (error) {
    if (error instanceof SemanticPolicyError) fail(error.code);
    throw error;
  }
  const publicId = `sem_${sha256Hex(`${workstream.publicId}\0${kind}`).slice(0, 32)}`;
  let object = await ctx.db.query("semanticObjects").withIndex("by_public_id", (q) => q.eq("publicId", publicId)).unique();
  if (object && (object.projectId !== project._id || object.workstreamId !== workstream._id)) fail("semantic_object_conflict");
  const changed = !object || object.text !== text || object.source !== source || !object.active || JSON.stringify(object.tags ?? []) !== JSON.stringify(tags);
  if (object) {
    await ctx.db.patch(object._id, { text, source, fidelity: "semantic", tags, revision: object.revision + (changed ? 1 : 0), active: true, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY });
    object = await ctx.db.get(object._id);
  } else {
    const id = await ctx.db.insert("semanticObjects", { publicId, projectId: project._id, scopeKey: workstream.scopeKey, workstreamId: workstream._id, kind, text, source, fidelity: "semantic", tags, revision: 1, active: true, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY });
    object = await ctx.db.get(id);
  }
  if (!object) fail("semantic_object_missing");
  const existingEmbedding = await ctx.db.query("semanticEmbeddings").withIndex("by_object", (q) => q.eq("objectId", object!._id)).unique();
  const fallbackModelVersion = "overgent-concepts/v1/1024";
  const embedding = {
    scopeKey: workstream.scopeKey,
    scopeModelKey: `${workstream.scopeKey.length}:${workstream.scopeKey}${fallbackModelVersion}`,
    providerName: "overgent",
    modelVersion: fallbackModelVersion,
    contentRevision: object.revision,
    vector: conceptVector(text),
    expiresAt: now + DEFAULT_RETENTION_DAYS * DAY,
  };
  if (existingEmbedding) await ctx.db.patch(existingEmbedding._id, embedding);
  else await ctx.db.insert("semanticEmbeddings", { objectId: object._id, ...embedding });

  // Embedding generation is intentionally asynchronous: a provider outage or
  // slow network must never delay observation, manifests, or checkpoints.
  if (changed) await ctx.scheduler.runAfter(0, internal.intelligence.embedSemanticObject, {
    semanticObjectPublicId: object.publicId,
    expectedRevision: object.revision,
  });

  if (changed) {
    await bumpScope(ctx, workstream.scopeKey, now);
    await recomputeSemanticFindings(ctx, project, workstream.scopeKey, now);
  }
}

async function recomputeSemanticFindings(ctx: MutationCtx, project: Doc<"projects">, key: string, now: number): Promise<void> {
  const objects = await ctx.db.query("semanticObjects").withIndex("by_scope_active", (q) => q.eq("scopeKey", key).eq("active", true)).take(65);
  if (objects.length > 64) fail("semantic_scope_too_large");
  const grouped = new Map<Id<"workstreams">, Array<Doc<"semanticObjects">>>();
  for (const object of objects) grouped.set(object.workstreamId, [...(grouped.get(object.workstreamId) ?? []), object]);
  const records: WorkstreamRecord[] = [];
  const states = new Map<string, { title: string; reportedChange: boolean; verification: VerificationState; branch?: string }>();
  for (const [workstreamId, group] of grouped) {
    const current = await ctx.db.get(workstreamId);
    if (!current || current.status === "done") continue;
    const change = group.find((object) => object.kind === "change");
    states.set(current.publicId, {
      title: current.title,
      reportedChange: change !== undefined,
      verification: (current.verificationState as VerificationState | undefined) ?? (change ? readVerificationState(change.text) : "unknown"),
      ...(current.branch ? { branch: current.branch } : {}),
    });
    records.push(await semanticRecord(ctx, project.publicId, current, group));
  }
  const tracked = await trackedContractSymbols(ctx, key);
  const recordById = new Map(records.map((record) => [record.id, record]));
  const activeFingerprints = new Set<string>();
  const revisionByWorkstream = new Map(records.map((record) => [record.id, record.revision]));
  for (const finding of evaluateWorkstreams(records)) {
    const candidate = pairCandidate(finding, recordById, states, tracked);
    const verdict = deterministicJudgment(candidate);
    // A silent verdict never becomes a coordination object. Leaving the
    // fingerprint out of the active set also resolves an earlier row that the
    // judgment layer has since decided adds nothing.
    if (verdict.delivery === "silent") continue;
    const fingerprint = sha256Hex(`${key}\0${finding.workstreamIds.join("\0")}\0${finding.kind}`);
    activeFingerprints.add(fingerprint);
    const inputRevisions = Object.fromEntries(finding.workstreamIds.map((id) => [id, revisionByWorkstream.get(id) ?? 0]));
    const existing = await ctx.db.query("findings").withIndex("by_fingerprint", (q) => q.eq("fingerprint", fingerprint)).unique();
    if (existing) {
      const material = existing.kind !== finding.kind || existing.severity !== verdict.severity || existing.confidenceBand !== finding.confidenceBand || existing.reason !== verdict.explanation || existing.state !== "open" || existing.delivery !== verdict.delivery || JSON.stringify(existing.evidence) !== JSON.stringify(finding.evidence) || JSON.stringify(existing.inputRevisions ?? {}) !== JSON.stringify(inputRevisions);
      await ctx.db.patch(existing._id, { kind: finding.kind, severity: verdict.severity, confidenceBand: finding.confidenceBand, evidence: finding.evidence, reason: verdict.explanation, state: "open", delivery: verdict.delivery, inputRevisions, revision: existing.revision + (material ? 1 : 0), lastSeenAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY, engineVersion: INTELLIGENCE_ENGINE_VERSION });
      if (material) await scheduleAdjudication(ctx, existing.publicId, existing.revision + 1, candidate, verdict);
      continue;
    }
    const publicId = `fnd_${fingerprint.slice(0, 32)}`;
    await ctx.db.insert("findings", { publicId, projectId: project._id, scopeKey: key, kind: finding.kind, severity: verdict.severity, confidenceBand: finding.confidenceBand, workstreamPublicIds: [...finding.workstreamIds], evidence: finding.evidence, reason: verdict.explanation, state: "open", delivery: verdict.delivery, fingerprint, engineVersion: INTELLIGENCE_ENGINE_VERSION, inputRevisions, revision: 1, firstSeenAt: now, lastSeenAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY });
    await scheduleAdjudication(ctx, publicId, 1, candidate, verdict);
  }
  const previous = await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", key)).take(513);
  if (previous.length > 512) fail("finding_scope_too_large");
  for (const finding of previous) {
    if (finding.engineVersion === INTELLIGENCE_ENGINE_VERSION && finding.state === "open" && !activeFingerprints.has(finding.fingerprint)) {
      await ctx.db.patch(finding._id, { state: "resolved", revision: finding.revision + 1, lastSeenAt: now });
    }
  }
}

async function semanticRecord(ctx: MutationCtx, projectId: string, workstream: Doc<"workstreams">, objects: Array<Doc<"semanticObjects">>): Promise<WorkstreamRecord> {
  const tags = objects.flatMap((object) => object.tags ?? []);
  const values = (prefix: string) => tags.filter((tag) => tag.startsWith(prefix)).map((tag) => tag.slice(prefix.length));
  const ordered = [...objects].sort((left, right) => left.kind === "intent" ? -1 : right.kind === "intent" ? 1 : right.revision - left.revision);
  const primary = ordered[0]!;
  const summary = ordered.map((object) => object.text).join(" ").slice(0, 2_000);
  const embedding = await ctx.db.query("semanticEmbeddings").withIndex("by_object", (q) => q.eq("objectId", primary._id)).unique();
  const currentProviderEmbedding = embedding && embedding.contentRevision === primary.revision && embedding.providerName.startsWith("openai/")
    ? { semanticVector: embedding.vector, semanticProvider: embedding.providerName }
    : {};
  return { projectId, repositoryId: primary.scopeKey, id: workstream.publicId, revision: Math.max(...objects.map((object) => object.revision)), status: workstream.status, summary, paths: values("path:"), dependencies: values("dependency:"), contracts: values("contract:"), schemas: values("schema:"), routes: values("route:"), components: values("component:"), changes: values("changes:"), assumptions: values("assumption:"), ...currentProviderEmbedding };
}

async function deactivateWorkstreamSemantics(ctx: MutationCtx, workstreamId: Id<"workstreams">): Promise<void> {
  const objects = await ctx.db.query("semanticObjects").withIndex("by_workstream_active", (q) => q.eq("workstreamId", workstreamId).eq("active", true)).collect();
  for (const object of objects) {
    await ctx.db.patch(object._id, { active: false });
    const embedding = await ctx.db.query("semanticEmbeddings").withIndex("by_object", (q) => q.eq("objectId", object._id)).unique();
    if (embedding) await ctx.db.delete(embedding._id);
  }
}

function stringValues(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === "string") : [];
}

function boundedSemanticText(parts: string[]): string {
  try {
    const normalized = parts.filter((part) => part.trim()).map(validateSemanticText);
    return validateSemanticText([...normalized.join(". ")].slice(0, 2_000).join(""));
  } catch (error) {
    if (error instanceof SemanticPolicyError) fail(error.code);
    throw error;
  }
}

async function upsertStaleAssumption(ctx: MutationCtx, project: Doc<"projects">, workstream: Doc<"workstreams">, briefPublicId: string, now: number): Promise<void> {
  const delivery = await ctx.db.query("contextDeliveries").withIndex("by_public_id", (q) => q.eq("publicId", briefPublicId)).unique();
  if (!delivery || delivery.projectId !== project._id || delivery.workstreamId !== workstream._id) fail("brief_not_found");
  const previous = delivery.itemRevisions && typeof delivery.itemRevisions === "object" ? delivery.itemRevisions as Record<string, number> : {};
  const scopedFindings = await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).take(257);
  if (scopedFindings.length > 256) fail("finding_scope_too_large");
  const findings = scopedFindings.filter((finding) => finding.state === "open" && finding.kind !== "stale_assumption" && finding.workstreamPublicIds.includes(workstream.publicId));
  const changed = findings.find((finding) => severityRank(finding.severity) >= 3 && (previous[finding.publicId] ?? 0) < finding.revision);
  if (!changed) return;
  const fingerprint = sha256Hex(`${workstream.scopeKey}\0${workstream.publicId}\0stale\0${changed.publicId}`);
  const existing = await ctx.db.query("findings").withIndex("by_fingerprint", (q) => q.eq("fingerprint", fingerprint)).unique();
  const evidence = [{ kind: "decision", summary: `${changed.publicId} materially changed after ${briefPublicId}.`, source: "context_delivery", fidelity: "structural" }];
  if (existing) await ctx.db.patch(existing._id, { lastSeenAt: now, revision: existing.revision + 1, state: "open", evidence, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY });
  else await ctx.db.insert("findings", { publicId: `fnd_${fingerprint.slice(0, 32)}`, projectId: project._id, scopeKey: workstream.scopeKey, kind: "stale_assumption", severity: "high", confidenceBand: "deterministic", workstreamPublicIds: [workstream.publicId], evidence, reason: "This checkpoint relied on a brief that was materially superseded for this workstream.", state: "open", delivery: decideDelivery(relationshipForKind("stale_assumption"), "high"), fingerprint, engineVersion: INTELLIGENCE_ENGINE_VERSION, revision: 1, firstSeenAt: now, lastSeenAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY });
}

async function requireDevice(ctx: QueryCtx | MutationCtx, tokenHash: string): Promise<Doc<"devices">> {
  const device = await ctx.db.query("devices").withIndex("by_token_hash", (q) => q.eq("tokenHash", tokenHash)).unique();
  if (!device) fail("unauthorized");
  if (device.revokedAt !== undefined) fail("credential_revoked");
  return device;
}

async function requireMembership(ctx: QueryCtx | MutationCtx, deviceId: Id<"devices">, projectId: Id<"projects">): Promise<Doc<"members">> {
  const member = await ctx.db.query("members").withIndex("by_project_device", (q) => q.eq("projectId", projectId).eq("deviceId", deviceId)).unique();
  if (!member || member.removedAt !== undefined) fail("forbidden");
  return member;
}

async function assertOwnerDeviceRevocable(ctx: QueryCtx | MutationCtx, target: Doc<"devices">): Promise<void> {
  const memberships = (await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", target._id)).collect())
    .filter((member) => member.removedAt === undefined && member.role === "owner");
  for (const membership of memberships) {
    const projectMembers = (await ctx.db.query("members").withIndex("by_project", (q) => q.eq("projectId", membership.projectId)).collect())
      .filter((member) => member.removedAt === undefined && member.role === "owner");
    let activeOwnerDevices = 0;
    for (const owner of projectMembers) {
      const device = await ctx.db.get(owner.deviceId);
      if (device && device.revokedAt === undefined) activeOwnerDevices++;
    }
    if (activeOwnerDevices <= 1) fail("cannot_revoke_last_owner_device");
  }
}

async function requireProjectRole(
  ctx: QueryCtx | MutationCtx,
  tokenHash: string,
  projectPublicId: string,
  requiredRole?: "owner",
): Promise<{ device: Doc<"devices">; member: Doc<"members">; project: Doc<"projects"> }> {
  const device = await requireDevice(ctx, tokenHash);
  const project = await ctx.db.query("projects").withIndex("by_public_id", (q) => q.eq("publicId", projectPublicId)).unique();
  if (!project || project.status !== "active") fail("not_found");
  const member = await requireMembership(ctx, device._id, project._id);
  if (requiredRole && member.role !== requiredRole) fail("forbidden");
  return { device, member, project };
}

async function enforceRate(
  ctx: MutationCtx,
  key: string,
  route: string,
  now: number,
  limit: number,
  windowMs: number,
): Promise<void> {
  const current = await ctx.db.query("rateLimits").withIndex("by_key_route", (q) => q.eq("key", key).eq("route", route)).unique();
  if (!current || current.windowStartedAt + windowMs <= now) {
    if (current) await ctx.db.patch(current._id, { windowStartedAt: now, count: 1, expiresAt: now + windowMs * 2 });
    else await ctx.db.insert("rateLimits", { key, route, windowStartedAt: now, count: 1, expiresAt: now + windowMs * 2 });
    return;
  }
  if (current.count >= limit) fail("rate_limited");
  await ctx.db.patch(current._id, { count: current.count + 1 });
}

async function ensureScope(ctx: MutationCtx, projectId: Id<"projects">, repoFingerprint: string, key: string, now: number): Promise<void> {
  const current = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", key)).unique();
  if (current) {
    if (current.projectId !== projectId || current.repoFingerprint !== repoFingerprint) fail("scope_conflict");
    return;
  }
  await ctx.db.insert("repositoryScopes", { scopeKey: key, projectId, repoFingerprint, contextRevision: 0, updatedAt: now });
}

async function bumpScope(ctx: MutationCtx, key: string, now: number): Promise<number> {
  const current = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", key)).unique();
  if (!current) fail("scope_not_found");
  const next = current.contextRevision + 1;
  await ctx.db.patch(current._id, { contextRevision: next, updatedAt: now });
  return next;
}

async function requireWorkstream(ctx: MutationCtx, publicId: string, projectId: Id<"projects">, workspaceId: Id<"workspaces">) {
  const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", publicId)).unique();
  if (!workstream) fail("workstream_not_found");
  if (workstream.projectId !== projectId || workstream.workspaceId !== workspaceId) fail("forbidden");
  return workstream;
}

async function bumpWorkstreamRevision(ctx: MutationCtx, publicId: string, now: number): Promise<void> {
  const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", publicId)).unique();
  if (!workstream) fail("workstream_not_found");
  await ctx.db.patch(workstream._id, { revision: workstream.revision + 1, updatedAt: now });
}

async function requireManifest(ctx: MutationCtx, publicId: string, projectId: Id<"projects">, key: string) {
  const manifest = await ctx.db.query("changeManifests").withIndex("by_public_id", (q) => q.eq("publicId", publicId)).unique();
  if (!manifest) fail("manifest_not_found");
  if (manifest.projectId !== projectId || manifest.scopeKey !== key) fail("forbidden");
  return manifest;
}

function findingContract(finding: Doc<"findings">) {
  return {
    id: finding.publicId,
    kind: finding.kind,
    severity: finding.severity,
    confidenceBand: finding.confidenceBand,
    workstreamIds: finding.workstreamPublicIds,
    evidence: finding.evidence,
    reason: finding.reason,
    state: finding.state,
    revision: finding.revision,
  };
}

function severityRank(severity: string): number {
  return ({ low: 1, medium: 2, high: 3, critical: 4 } as Record<string, number>)[severity] ?? 0;
}

async function requireBrowserSession(ctx: QueryCtx | MutationCtx, sessionHash: string, now: number) {
  const session = await ctx.db.query("browserSessions").withIndex("by_secret_hash", (q) => q.eq("secretHash", sessionHash)).unique();
  if (!session || session.revokedAt !== undefined || session.expiresAt <= now) fail("unauthorized");
  const project = await ctx.db.get(session.projectId);
  const member = await ctx.db.get(session.memberId);
  const device = session.deviceId ? await ctx.db.get(session.deviceId) : null;
  if (!project || project.status !== "active" || !member || member.removedAt !== undefined || member.projectId !== project._id || !device || device.revokedAt !== undefined || member.deviceId !== device._id) fail("unauthorized");
  return { session, project, member, device };
}

async function projectSemanticStatus(ctx: QueryCtx, projectId: Id<"projects">): Promise<{ status: "enabled" | "degraded"; mode: "offline_fallback" | "managed_openai" | "managed_degraded" }> {
  const scopes = await ctx.db.query("repositoryScopes").withIndex("by_project", (q) => q.eq("projectId", projectId)).take(101);
  if (scopes.length > 100) fail("page_too_large");
  if (scopes.some((scope) => (scope.semanticDegradedAt ?? 0) > (scope.semanticHealthyAt ?? 0))) return { status: "degraded", mode: "managed_degraded" };
  return scopes.some((scope) => scope.semanticProviderName?.startsWith("openai/"))
    ? { status: "enabled", mode: "managed_openai" }
    : { status: "enabled", mode: "offline_fallback" };
}

function initials(name: string): string {
  return name.split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? "").join("") || "SG";
}

function relativeLabel(now: number, then: number): string {
  if (!then) return "never";
  const seconds = Math.max(0, Math.floor((now - then) / 1000));
  if (seconds < 5) return "Now";
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return minutes < 60 ? `${minutes} min` : `${Math.floor(minutes / 60)} hr`;
}

/** Matches priorGoals maxItems in scope-snapshot.schema.json. A session that
 *  restates its objective more than this many times is served better by an
 *  honest count of what was dropped than by an unbounded list. */
const MAX_PRIOR_GOALS = 15;

function activityKind(type: string): "intent" | "manifest" | "finding" | "checkpoint" | "pause" | "agent" {
  if (type === "agent.activity_reported" || type === "session.read_set_reported") return "agent";
  if (type === "workstream.intent_reported") return "intent";
  if (type === "workstream.checkpoint_reported") return "checkpoint";
  if (type === "workspace.paused" || type === "workspace.resumed") return "pause";
  return "manifest";
}

function dashboardAgentStatus(value: unknown): "active" | "waiting" | "idle" | "done" | "error" {
  return value === "waiting" || value === "idle" || value === "done" || value === "error" ? value : "active";
}

function activitySummary(type: string, payload: unknown): string {
  const object = payload && typeof payload === "object" ? payload as Record<string, unknown> : {};
  if (type === "workstream.intent_reported") return `reported intent: ${String(object.title ?? "untitled work")}`;
  if (type === "workspace.manifest_completed") return `published manifest revision ${String(object.revision ?? "")}`.trim();
  if (type === "workspace.registered") return "registered a workspace";
  if (type === "agent.activity_reported") return String(object.action ?? "reported agent activity");
  if (type === "workspace.contract_fingerprints_reported") {
    const entries = Array.isArray(object.entries) ? object.entries : [];
    return `published contract fingerprints for ${entries.length} file${entries.length === 1 ? "" : "s"}`;
  }
  if (type === "session.read_set_reported") {
    const entries = Array.isArray(object.entries) ? object.entries : [];
    return `recorded ${entries.length} file${entries.length === 1 ? "" : "s"} in a session read set`;
  }
  return type.replaceAll(".", " ").replaceAll("_", " ");
}

function dashboardFindingKind(kind: string): "direct_collision" | "likely_collision" | "redundant_work" | "shared_dependency" | "assumption_conflict" | "downstream_impact" | "stale_assumption" | "dependency_ready" {
  if (["direct_collision", "likely_collision", "redundant_work", "shared_dependency", "assumption_conflict", "downstream_impact", "stale_assumption", "dependency_ready"].includes(kind)) return kind as ReturnType<typeof dashboardFindingKind>;
  return "likely_collision";
}

function dashboardFindingState(state: string): "open" | "acknowledged" | "resolved" | "dismissed" {
  if (["open", "acknowledged", "resolved", "dismissed"].includes(state)) return state as ReturnType<typeof dashboardFindingState>;
  return "acknowledged";
}

function dashboardEvidenceKind(kind: string): "path" | "contract" | "dependency" | "intent" {
  if (kind === "path") return "path";
  if (kind === "dependency") return "dependency";
  if (["symbol", "schema", "route"].includes(kind)) return "contract";
  return "intent";
}

function dashboardEvidenceSource(source: string): "git" | "mcp" | "manual" | "hook" | "semantic_candidate" {
  if (["git", "mcp", "manual", "hook", "semantic_candidate"].includes(source)) return source as ReturnType<typeof dashboardEvidenceSource>;
  if (source.startsWith("openai/") || source.startsWith("overgent-concepts/")) return "semantic_candidate";
  return "manual";
}

function dashboardFidelity(source: string): "mcp" | "git" | "manual" | "hook" | "hook_unverified" {
  if (["mcp", "git", "manual", "hook", "hook_unverified"].includes(source)) return source as ReturnType<typeof dashboardFidelity>;
  return "manual";
}

async function requireCollaborationActor(
  ctx: QueryCtx | MutationCtx,
  args: { projectPublicId: string; tokenHash?: string; sessionHash?: string; now: number },
): Promise<{ project: Doc<"projects">; member: Doc<"members">; device: Doc<"devices"> }> {
  if ((args.tokenHash ? 1 : 0) + (args.sessionHash ? 1 : 0) !== 1) fail("unauthorized");
  if (args.sessionHash) {
    const auth = await requireBrowserSession(ctx, args.sessionHash, args.now);
    if (auth.project.publicId !== args.projectPublicId) fail("forbidden");
    return { project: auth.project, member: auth.member, device: auth.device };
  }
  return requireProjectRole(ctx, args.tokenHash!, args.projectPublicId);
}

async function sessionSharingView(
  ctx: QueryCtx | MutationCtx,
  workstream: Doc<"workstreams">,
  _actor: Doc<"members">,
  _now: number,
) {
  const messages = await ctx.db.query("sessionMessages")
    .withIndex("by_workstream_captured", (q) => q.eq("workstreamId", workstream._id))
    .order("asc").take(100);
  return {
    workstreamId: workstream.publicId,
    messages: messages.map((message) => ({
      id: message.publicId, kind: message.kind, text: message.text, vendor: message.vendor,
      capturedAt: new Date(message.capturedAt).toISOString(), expiresAt: new Date(message.expiresAt).toISOString(),
    })),
  };
}

// A member who named themselves at enrollment is not asked again; otherwise the
// device label seeds the name and is marked as still owing an explicit choice.
function memberIdentity(displayName: string | undefined, deviceLabel: string): { displayName: string; displayNameSource: "device" | "member" } {
  if (displayName === undefined) return { displayName: deviceLabel, displayNameSource: "device" };
  return { displayName: normalizeDisplayName(displayName), displayNameSource: "member" };
}

// Live-work identity is member-chosen. Email addresses are rejected so a
// Project never turns a contact address into the name teammates see.
export function normalizeDisplayName(value: string): string {
  const displayName = value.trim().replace(/\s+/g, " ");
  if (displayName.length < 2 || displayName.length > 60) fail("validation_failed");
  if (/[\u0000-\u001f\u007f]/.test(displayName)) fail("validation_failed");
  if (/@/.test(displayName)) fail("email_identity_rejected");
  return displayName;
}



async function bumpProjectScopes(ctx: MutationCtx, projectId: Id<"projects">, now: number): Promise<void> {
  const scopes = await ctx.db.query("repositoryScopes").withIndex("by_project", (q) => q.eq("projectId", projectId)).take(101);
  if (scopes.length > 100) fail("page_too_large");
  for (const scope of scopes) await ctx.db.patch(scope._id, { contextRevision: scope.contextRevision + 1, updatedAt: now });
}



async function syncCommentContract(ctx: QueryCtx | MutationCtx, comment: Doc<"syncComments">) {
  const member = await ctx.db.get(comment.memberId);
  if (!member) fail("not_found");
  return { id: comment.publicId, memberName: member.displayName, body: comment.body, createdAt: new Date(comment.createdAt).toISOString() };
}

async function decisionContract(ctx: QueryCtx | MutationCtx, decision: Doc<"decisions">) {
  const card = decision.syncCardId ? await ctx.db.get(decision.syncCardId) : null;
  const affectedMemberIds: string[] = [];
  for (const id of decision.affectedMemberIds) {
    const member = await ctx.db.get(id);
    if (member) affectedMemberIds.push(member.publicId);
  }
  const affectedWorkstreamIds: string[] = [];
  // Per-session delivery state, so the surface that recorded the decision can
  // show the loop closing: queued until the brief is pulled at the session's
  // next turn boundary, then delivered, then considered. This is the fact the
  // member is waiting on after typing a decision, and it already existed in
  // decisionDeliveries - it was only ever readable from History.
  const deliveries: Array<{ workstreamId: string; deliveredAt: string; acknowledgedAt?: string }> = [];
  for (const id of decision.affectedWorkstreamIds) {
    const workstream = await ctx.db.get(id);
    if (!workstream) continue;
    affectedWorkstreamIds.push(workstream.publicId);
    const delivery = await ctx.db.query("decisionDeliveries").withIndex("by_decision_workstream", (q) => q.eq("decisionId", decision._id).eq("workstreamId", workstream._id)).unique();
    if (delivery) {
      deliveries.push({
        workstreamId: workstream.publicId,
        deliveredAt: new Date(delivery.deliveredAt).toISOString(),
        ...(delivery.acknowledgedAt === undefined ? {} : { acknowledgedAt: new Date(delivery.acknowledgedAt).toISOString() }),
      });
    }
  }
  return {
    id: decision.publicId, ...(card ? { syncCardId: card.publicId } : {}), summary: decision.summary,
    affectedMemberIds, affectedWorkstreamIds, deliveries, revision: decision.revision, createdAt: new Date(decision.createdAt).toISOString(),
  };
}

async function syncCardContract(ctx: QueryCtx | MutationCtx, card: Doc<"syncCards">) {
  const finding = card.findingId ? await ctx.db.get(card.findingId) : null;
  const comments = await ctx.db.query("syncComments").withIndex("by_card_created", (q) => q.eq("syncCardId", card._id)).take(101);
  if (comments.length > 100) fail("page_too_large");
  const decisions = await ctx.db.query("decisions").withIndex("by_project_updated", (q) => q.eq("projectId", card.projectId)).collect();
  const decision = decisions.find((candidate) => candidate.syncCardId === card._id);
  return {
    id: card.publicId, ...(finding ? { findingId: finding.publicId } : {}), title: card.title, summary: card.summary,
    state: card.state, revision: card.revision, comments: await Promise.all(comments.map((comment) => syncCommentContract(ctx, comment))),
    ...(decision ? { resolution: await decisionContract(ctx, decision) } : {}), updatedAt: new Date(card.updatedAt).toISOString(),
  };
}

// ADR-037: the coordination surface is collisions and their resolutions. Plan
// items, advisory claims, and the standalone decision log were removed; a
// resolution still reaches every affected agent through the brief.
async function collaborationView(ctx: QueryCtx | MutationCtx, project: Doc<"projects">, after: number) {
  const cardRows = await ctx.db.query("syncCards").withIndex("by_project_updated", (q) => q.eq("projectId", project._id)).order("desc").take(101);
  const resolutionRows = await ctx.db.query("decisions").withIndex("by_project_updated", (q) => q.eq("projectId", project._id)).order("desc").take(201);
  if (cardRows.length > 100 || resolutionRows.length > 200) fail("page_too_large");
  const syncCards = await Promise.all(cardRows.map((card) => syncCardContract(ctx, card)));
  const resolutions = await Promise.all(resolutionRows.map((resolution) => decisionContract(ctx, resolution)));
  const latest = [...cardRows.map((card) => card.updatedAt), ...resolutionRows.map((resolution) => resolution.updatedAt)]
    .filter((at) => at > after)
    .reduce((maximum, at) => Math.max(maximum, at), after);
  return { projectId: project.publicId, syncCards, resolutions, cursor: `time:${latest}` };
}

function fail(code: string): never {
  throw new Error(`E:${code}`);
}
