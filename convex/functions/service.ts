import { v } from "convex/values";
import { internalMutation, internalQuery } from "./_generated/server";
import type { MutationCtx, QueryCtx } from "./_generated/server";
import type { Doc, Id } from "./_generated/dataModel";
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
      expiresAt: args.sessionExpiresAt,
    });
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
    briefPublicId: v.string(), now: v.number(),
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
    const findings = (await ctx.db.query("findings").withIndex("by_scope", (q) => q.eq("scopeKey", workstream.scopeKey)).collect())
      .filter((finding) => finding.state === "open" && finding.workstreamPublicIds.includes(workstream.publicId))
      .sort((a, b) => severityRank(b.severity) - severityRank(a.severity) || a.publicId.localeCompare(b.publicId));
    const items = [];
    let renderedSize = 0;
    let truncated = false;
    for (const finding of findings) {
      const item = {
        id: finding.publicId,
        revision: finding.revision,
        kind: "finding" as const,
        text: finding.reason,
        relevanceReason: "The finding directly names this workstream.",
        fidelity: finding.confidenceBand === "deterministic" ? "structural" : "semantic_degraded",
        advisoryAction: severityRank(finding.severity) >= 3 ? "coordination_required" as const : "review_recommended" as const,
        priority: severityRank(finding.severity) * 25,
      };
      const estimated = Math.ceil(JSON.stringify(item).length / 4);
      if (items.length >= 64 || renderedSize + estimated > args.requestedBudget) {
        truncated = true;
        break;
      }
      items.push(item);
      renderedSize += estimated;
    }
    await ctx.db.insert("contextDeliveries", {
      publicId: args.briefPublicId,
      projectId: project._id,
      workstreamId: workstream._id,
      contextRevision: scope.contextRevision,
      trigger: args.trigger,
      itemRefs: items.map((item) => item.id),
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
        if (event.sequence <= workspace.lastProjectedSequence) return;
        const material = current.title !== String(payload.title) || current.summary !== summary;
        await ctx.db.patch(current._id, {
          title: String(payload.title), summary, revision: current.revision + 1, status: "active", updatedAt: now,
        });
        if (material) await bumpScope(ctx, workspace.scopeKey, now);
      }
      await ctx.db.patch(workspace._id, { lastProjectedSequence: Math.max(workspace.lastProjectedSequence, event.sequence), updatedAt: now });
      return;
    }
    case "workstream.status_changed": {
      const workstream = await requireWorkstream(ctx, String(payload.workstreamId), project._id, workspace._id);
      if (event.sequence <= workspace.lastProjectedSequence) return;
      const status = String(payload.status) as "active" | "idle" | "done" | "blocked";
      await ctx.db.patch(workstream._id, { status, revision: workstream.revision + 1, updatedAt: now });
      await ctx.db.patch(workspace._id, { lastProjectedSequence: event.sequence, updatedAt: now });
      if (status !== workstream.status) await bumpScope(ctx, workspace.scopeKey, now);
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

function fail(code: string): never {
  throw new Error(`E:${code}`);
}
