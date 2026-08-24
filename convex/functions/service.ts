import { v } from "convex/values";
import { internalMutation, internalQuery } from "./_generated/server";
import type { MutationCtx, QueryCtx } from "./_generated/server";
import type { Doc, Id } from "./_generated/dataModel";
import { conceptVector, evaluateWorkstreams, renderBrief, validateSemanticTags, validateSemanticText, INTELLIGENCE_ENGINE_VERSION, SemanticPolicyError, type WorkstreamRecord } from "@stickguy/coordination";
import { assertCanonicalManifestOrder, canActivateManifestRevision, manifestContentHash, RETENTION_TABLES, scopeKey, sha256Hex, ValidationError } from "../src/domain";
import type { ManifestEntry } from "../src/domain";

const DAY = 86_400_000;
const ACTIVITY_RETENTION = 30 * DAY;
const DELIVERY_RETENTION = 30 * DAY;
const DEFAULT_RETENTION_DAYS = 30;

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
        appVersion: "creator/v1",
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
      displayName: args.deviceLabel,
      role: "owner",
      joinedAt: args.now,
    });
    return { id: args.projectPublicId, label: args.label };
  },
});

export const createInvite = internalMutation({
  args: {
    tokenHash: v.string(), projectPublicId: v.string(), invitePublicId: v.string(), secretHash: v.string(),
    expiresAt: v.number(), maxUses: v.number(), now: v.number(),
  },
  handler: async (ctx, args) => {
    const auth = await requireProjectRole(ctx, args.tokenHash, args.projectPublicId, "owner");
    await enforceRate(ctx, args.tokenHash, "invites.create", args.now, 20, 60_000);
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
    memberPublicId: v.string(), deviceTokenHash: v.string(), dashboardTicketHash: v.string(), deviceLabel: v.string(),
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
      displayName: args.deviceLabel,
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

export const dashboardSession = internalQuery({
  args: { sessionHash: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const auth = await requireBrowserSession(ctx, args.sessionHash, args.now);
    const semanticStatus = await projectSemanticStatus(ctx, auth.project._id);
    return {
      memberName: auth.member.displayName,
      projects: [{ id: auth.project.publicId, name: auth.project.label, repositoryLabel: "Project repositories", semanticStatus }],
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
    const members = await ctx.db.query("members").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).collect();
    const memberById = new Map(members.map((member) => [member._id, member]));
    const projectWorkstreams = await ctx.db.query("workstreams").withIndex("by_project", (q) => q.eq("projectId", auth.project._id)).collect();
    const workstreams = [];
    const devices = [];
    let contextRevision = 0;
    let semanticStatus: "enabled" | "degraded" = "enabled";
    for (const workspace of workspaces) {
      const stream = projectWorkstreams.find((candidate) => candidate.workspaceId === workspace._id);
      const member = memberById.get(workspace.memberId);
      const device = await ctx.db.get(workspace.deviceId);
      const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", workspace.scopeKey)).unique();
      contextRevision = Math.max(contextRevision, scope?.contextRevision ?? 0);
      if ((scope?.semanticDegradedAt ?? 0) > (scope?.semanticHealthyAt ?? 0)) semanticStatus = "degraded";
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
      const lastSeenAt = device?.lastSeenAt ?? 0;
      const presence = workspace.paused ? "paused" : args.now - lastSeenAt <= 35_000 ? "online" : args.now - lastSeenAt <= 120_000 ? "idle" : "offline";
      if (stream) workstreams.push({
        id: stream.publicId, memberName: member?.displayName ?? "Project member", initials: initials(member?.displayName ?? "PM"),
        title: stream.title, outcome: stream.summary, presence, fidelity: "manual", updatedLabel: relativeLabel(args.now, stream.updatedAt),
        pathCount, paths, ...(pathCount >= 1000 ? { largeChange: { pathCount, summary: "Broad metadata-only change; inspect evidence before inferring severity.", revision: manifestRevision } } : {}),
      });
      if (device) devices.push({ id: device.publicId, label: device.label, platform: device.appVersion, status: presence, lastSeen: relativeLabel(args.now, lastSeenAt) });
    }
    const findingDocs = await ctx.db.query("findings").withIndex("by_project_seen", (q) => q.eq("projectId", auth.project._id)).take(100);
    const findings = findingDocs.map((finding) => ({
      id: finding.publicId, kind: dashboardFindingKind(finding.kind), severity: finding.severity, confidence: finding.confidenceBand, state: dashboardFindingState(finding.state),
      title: finding.kind.replaceAll("_", " "), reason: finding.reason, workstreamIds: finding.workstreamPublicIds,
      evidence: (finding.evidence as Array<{ kind: string; summary: string; source: string }>).map((item) => ({ kind: dashboardEvidenceKind(item.kind), label: item.summary, source: dashboardEvidenceSource(item.source) })),
      firstSeen: relativeLabel(args.now, finding.firstSeenAt), lastSeen: relativeLabel(args.now, finding.lastSeenAt),
    }));
    const activityDocs = await ctx.db.query("activityEvents").withIndex("by_project_received", (q) => q.eq("projectId", auth.project._id)).order("desc").take(20);
    const activity = activityDocs.map((event) => ({ id: event.eventId, at: relativeLabel(args.now, event.receivedAt), actor: memberById.get(event.memberId)?.displayName ?? "Project member", kind: activityKind(event.type), summary: activitySummary(event.type, event.payload), fidelity: dashboardFidelity(event.source) }));
    return { project: { id: auth.project.publicId, name: auth.project.label, repositoryLabel: "Project repositories", semanticStatus }, contextRevision, synchronizedAt: "just now", workstreams, findings, activity, devices, workspacePaused: workspaces.some((workspace) => workspace.paused) };
  },
});

export const recordSemanticHealth = internalMutation({
  args: { tokenHash: v.string(), workstreamPublicId: v.string(), degraded: v.boolean(), now: v.number() },
  handler: async (ctx, args) => {
    const device = await requireDevice(ctx, args.tokenHash);
    const workstream = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", args.workstreamPublicId)).unique();
    if (!workstream) fail("not_found");
    await requireMembership(ctx, device._id, workstream.projectId);
    const scope = await ctx.db.query("repositoryScopes").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).unique();
    if (!scope) fail("not_found");
    await ctx.db.patch(scope._id, args.degraded ? { semanticDegradedAt: args.now } : { semanticHealthyAt: args.now });
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
        payload: event.payload,
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
    if (findings.length > 100) fail("page_too_large");
    const items = findings.map(findingContract);
    const cursorValue = findings.reduce((latest, finding) => Math.max(latest, finding.lastSeenAt), args.after);
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
      kind: finding.kind as "direct_collision" | "likely_collision" | "redundant_work" | "shared_dependency" | "assumption_conflict" | "downstream_impact" | "stale_assumption",
      severity: finding.severity as "low" | "medium" | "high" | "critical",
      confidenceBand: finding.confidenceBand as "deterministic" | "high" | "medium" | "low",
      workstreamIds: finding.workstreamPublicIds,
      evidence: finding.evidence,
      reason: finding.reason,
      revision: finding.revision,
      priority: severityRank(finding.severity) * 25,
    })), args.requestedBudget);
    const items = [...rendered.items];
    let renderedSize = rendered.renderedSize;
    let truncated = rendered.truncated;
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
    if (!finding) fail("not_found");
    await requireMembership(ctx, device._id, finding.projectId);
    return findingContract(finding);
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

export const revokeDevice = internalMutation({
  args: { tokenHash: v.string(), targetDevicePublicId: v.string(), now: v.number() },
  handler: async (ctx, args) => {
    const actor = await requireDevice(ctx, args.tokenHash);
    await enforceRate(ctx, args.tokenHash, "devices.revoke", args.now, 20, 60_000);
    const target = await ctx.db.query("devices").withIndex("by_public_id", (q) => q.eq("publicId", args.targetDevicePublicId)).unique();
    if (!target) fail("not_found");
    if (actor._id !== target._id) {
      const actorMemberships = (await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", actor._id)).collect())
        .filter((member) => member.removedAt === undefined && member.role === "owner");
      const targetMemberships = (await ctx.db.query("members").withIndex("by_device", (q) => q.eq("deviceId", target._id)).collect())
        .filter((member) => member.removedAt === undefined);
      if (!actorMemberships.some((owner) => targetMemberships.some((member) => member.projectId === owner.projectId))) fail("forbidden");
    }
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
    return deleted;
  },
});

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
      if (!current) {
        await ctx.db.insert("workstreams", {
          publicId: workstreamPublicId, projectId: project._id, memberId: member._id, workspaceId: workspace._id,
          scopeKey: workspace.scopeKey, title: String(payload.title), summary, status: "active", revision: 1, updatedAt: now,
        });
        await bumpScope(ctx, workspace.scopeKey, now);
      } else {
        if (current.projectId !== project._id || current.workspaceId !== workspace._id) fail("forbidden");
        const material = current.title !== String(payload.title) || current.summary !== summary;
        await ctx.db.patch(current._id, {
          title: String(payload.title), summary, revision: current.revision + 1, status: "active", updatedAt: now,
        });
        if (material) await bumpScope(ctx, workspace.scopeKey, now);
      }
      await ctx.db.patch(workspace._id, { lastProjectedSequence: Math.max(workspace.lastProjectedSequence, event.sequence), updatedAt: now });
      const projected = await ctx.db.query("workstreams").withIndex("by_public_id", (q) => q.eq("publicId", workstreamPublicId)).unique();
      if (!projected) fail("workstream_not_found");
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
      await upsertSemanticIntelligence(ctx, project, checkpointWorkstream, boundedSemanticText([String(payload.summary), ...stringValues(payload.discoveries)]), "change", checkpointTags, event.source, now);
      const basedOnBriefId = typeof payload.basedOnBriefId === "string" ? payload.basedOnBriefId : "";
      if (basedOnBriefId) await upsertStaleAssumption(ctx, project, checkpointWorkstream, basedOnBriefId, now);
      await ctx.db.patch(workspace._id, { lastProjectedSequence: event.sequence, updatedAt: now });
      await bumpScope(ctx, workspace.scopeKey, now);
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
      await ctx.db.patch(workstream._id, { currentManifestId: manifest._id, updatedAt: now });
      await upsertPathFindings(ctx, project, manifest, workstream, entries, now);
      await bumpScope(ctx, workspace.scopeKey, now);
      return;
    }
  }
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
  const embedding = { scopeKey: workstream.scopeKey, providerName: "stickguy", modelVersion: "stickguy-concepts/v1", contentRevision: object.revision, vector: conceptVector(text), expiresAt: now + DEFAULT_RETENTION_DAYS * DAY };
  if (existingEmbedding) await ctx.db.patch(existingEmbedding._id, embedding);
  else await ctx.db.insert("semanticEmbeddings", { objectId: object._id, ...embedding });

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
  for (const [workstreamId, group] of grouped) {
    const current = await ctx.db.get(workstreamId);
    if (!current || current.status === "done") continue;
    records.push(semanticRecord(project.publicId, current, group));
  }
  const activeFingerprints = new Set<string>();
  const revisionByWorkstream = new Map(records.map((record) => [record.id, record.revision]));
  for (const finding of evaluateWorkstreams(records)) {
    const fingerprint = sha256Hex(`${key}\0${finding.workstreamIds.join("\0")}\0${finding.kind}`);
    activeFingerprints.add(fingerprint);
    const inputRevisions = Object.fromEntries(finding.workstreamIds.map((id) => [id, revisionByWorkstream.get(id) ?? 0]));
    const existing = await ctx.db.query("findings").withIndex("by_fingerprint", (q) => q.eq("fingerprint", fingerprint)).unique();
    if (existing) {
      const material = existing.kind !== finding.kind || existing.severity !== finding.severity || existing.confidenceBand !== finding.confidenceBand || existing.reason !== finding.reason || existing.state !== "open" || JSON.stringify(existing.evidence) !== JSON.stringify(finding.evidence) || JSON.stringify(existing.inputRevisions ?? {}) !== JSON.stringify(inputRevisions);
      await ctx.db.patch(existing._id, { kind: finding.kind, severity: finding.severity, confidenceBand: finding.confidenceBand, evidence: finding.evidence, reason: finding.reason, state: "open", inputRevisions, revision: existing.revision + (material ? 1 : 0), lastSeenAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY, engineVersion: INTELLIGENCE_ENGINE_VERSION });
    }
    else await ctx.db.insert("findings", { publicId: `fnd_${fingerprint.slice(0, 32)}`, projectId: project._id, scopeKey: key, kind: finding.kind, severity: finding.severity, confidenceBand: finding.confidenceBand, workstreamPublicIds: [...finding.workstreamIds], evidence: finding.evidence, reason: finding.reason, state: "open", fingerprint, engineVersion: INTELLIGENCE_ENGINE_VERSION, inputRevisions, revision: 1, firstSeenAt: now, lastSeenAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY });
  }
  const previous = await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", key)).take(513);
  if (previous.length > 512) fail("finding_scope_too_large");
  for (const finding of previous) {
    if (finding.engineVersion === INTELLIGENCE_ENGINE_VERSION && finding.state === "open" && !activeFingerprints.has(finding.fingerprint)) {
      await ctx.db.patch(finding._id, { state: "resolved", revision: finding.revision + 1, lastSeenAt: now });
    }
  }
}

function semanticRecord(projectId: string, workstream: Doc<"workstreams">, objects: Array<Doc<"semanticObjects">>): WorkstreamRecord {
  const tags = objects.flatMap((object) => object.tags ?? []);
  const values = (prefix: string) => tags.filter((tag) => tag.startsWith(prefix)).map((tag) => tag.slice(prefix.length));
  const summary = objects.sort((left, right) => left.kind === "intent" ? -1 : right.kind === "intent" ? 1 : right.revision - left.revision).map((object) => object.text).join(" ").slice(0, 2_000);
  return { projectId, repositoryId: objects[0]!.scopeKey, id: workstream.publicId, revision: Math.max(...objects.map((object) => object.revision)), status: workstream.status, summary, paths: values("path:"), dependencies: values("dependency:"), contracts: values("contract:"), schemas: values("schema:"), routes: values("route:"), components: values("component:"), changes: values("changes:"), assumptions: values("assumption:") };
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
  else await ctx.db.insert("findings", { publicId: `fnd_${fingerprint.slice(0, 32)}`, projectId: project._id, scopeKey: workstream.scopeKey, kind: "stale_assumption", severity: "high", confidenceBand: "deterministic", workstreamPublicIds: [workstream.publicId], evidence, reason: "This checkpoint relied on a brief that was materially superseded for this workstream.", state: "open", fingerprint, engineVersion: INTELLIGENCE_ENGINE_VERSION, revision: 1, firstSeenAt: now, lastSeenAt: now, expiresAt: now + DEFAULT_RETENTION_DAYS * DAY });
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

async function projectSemanticStatus(ctx: QueryCtx, projectId: Id<"projects">): Promise<"enabled" | "degraded"> {
  const scopes = await ctx.db.query("repositoryScopes").withIndex("by_project", (q) => q.eq("projectId", projectId)).take(101);
  if (scopes.length > 100) fail("page_too_large");
  return scopes.some((scope) => (scope.semanticDegradedAt ?? 0) > (scope.semanticHealthyAt ?? 0)) ? "degraded" : "enabled";
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

function activityKind(type: string): "intent" | "manifest" | "finding" | "checkpoint" | "pause" {
  if (type === "workstream.intent_reported") return "intent";
  if (type === "workstream.checkpoint_reported") return "checkpoint";
  if (type === "workspace.paused" || type === "workspace.resumed") return "pause";
  return "manifest";
}

function activitySummary(type: string, payload: unknown): string {
  const object = payload && typeof payload === "object" ? payload as Record<string, unknown> : {};
  if (type === "workstream.intent_reported") return `reported intent: ${String(object.title ?? "untitled work")}`;
  if (type === "workspace.manifest_completed") return `published manifest revision ${String(object.revision ?? "")}`.trim();
  if (type === "workspace.registered") return "registered a workspace";
  return type.replaceAll(".", " ").replaceAll("_", " ");
}

function dashboardFindingKind(kind: string): "direct_collision" | "likely_collision" | "redundant_work" | "shared_dependency" | "assumption_conflict" | "downstream_impact" | "stale_assumption" {
  if (["direct_collision", "likely_collision", "redundant_work", "shared_dependency", "assumption_conflict", "downstream_impact", "stale_assumption"].includes(kind)) return kind as ReturnType<typeof dashboardFindingKind>;
  return "likely_collision";
}

function dashboardFindingState(state: string): "open" | "acknowledged" | "resolved" | "dismissed" {
  if (["open", "acknowledged", "resolved", "dismissed"].includes(state)) return state as ReturnType<typeof dashboardFindingState>;
  return "acknowledged";
}

function dashboardEvidenceKind(kind: string): "path" | "contract" | "dependency" | "intent" {
  if (kind === "path") return "path";
  if (kind === "dependency") return "dependency";
  if (["schema", "route"].includes(kind)) return "contract";
  return "intent";
}

function dashboardEvidenceSource(source: string): "git" | "mcp" | "manual" | "semantic_candidate" {
  if (["git", "mcp", "manual", "semantic_candidate"].includes(source)) return source as ReturnType<typeof dashboardEvidenceSource>;
  return "manual";
}

function dashboardFidelity(source: string): "mcp" | "git" | "manual" | "hook_unverified" {
  if (["mcp", "git", "manual", "hook_unverified"].includes(source)) return source as ReturnType<typeof dashboardFidelity>;
  return "manual";
}

function fail(code: string): never {
  throw new Error(`E:${code}`);
}
