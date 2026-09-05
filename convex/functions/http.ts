import { httpRouter } from "convex/server";
import { httpAction } from "./_generated/server";
import type { ActionCtx } from "./_generated/server";
import { internal } from "./_generated/api";
import {
  LIMITS,
  ValidationError,
  expectExactKeys,
  expectId,
  expectInteger,
  expectObject,
  expectString,
  publicId,
  randomHex,
  sha256Hex,
  validateEventBatch,
} from "../src/domain";
import { activationFailureResponse } from "../src/activation";
import { putProjectAISettingsHandler } from "./intelligence";

const http = httpRouter();
const JSON_HEADERS = {
  "content-type": "application/json; charset=utf-8",
  "cache-control": "no-store",
  "x-content-type-options": "nosniff",
};

http.route({ path: "/v1/projects", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const token = bearer(request);
  if (token.length < 64) throw new HttpFailure("unauthorized", 401);
  await consumeEdgeRate(ctx, requestRateKey(request, "projects.create"), "projects.create", 5);
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["label", "deviceLabel"], ["displayName", "appVersion"]);
  const label = expectString(body.label, 1, 120);
  const deviceLabel = expectString(body.deviceLabel, 1, 120);
  const displayName = body.displayName === undefined ? undefined : expectString(body.displayName, 2, 60);
  const project = await ctx.runMutation(internal.service.createProject, {
    tokenHash: sha256Hex(token),
    projectPublicId: publicId("prj"),
    memberPublicId: publicId("mem"),
    devicePublicId: publicId("dev"),
    label,
    deviceLabel,
    appVersion: body.appVersion === undefined ? "creator/v1" : expectString(body.appVersion, 1, 64),
    ...(displayName !== undefined ? { displayName } : {}),
    now: Date.now(),
  });
  return json(project, 201);
})) });

http.route({ pathPrefix: "/v1/projects/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const path = new URL(request.url).pathname;
  const inviteMatch = path.match(/^\/v1\/projects\/([^/]+)\/invites$/);
  if (inviteMatch) {
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "invites.create", 20);
    const body = expectObject(await readJson(request));
    expectExactKeys(body, ["expiresInSeconds", "maxUses"]);
    const expiresInSeconds = expectInteger(body.expiresInSeconds, 60, 604_800);
    const maxUses = expectInteger(body.maxUses, 1, 50);
    const secret = randomHex(16);
    const inviteId = publicId("inv");
    const now = Date.now();
    await ctx.runMutation(internal.service.createInvite, {
      ...auth,
      projectPublicId: expectId(inviteMatch[1]),
      invitePublicId: inviteId,
      secretHash: sha256Hex(secret),
      expiresAt: now + expiresInSeconds * 1_000,
      maxUses,
      now,
    });
    return json({ id: inviteId, secret, expiresAt: new Date(now + expiresInSeconds * 1_000).toISOString() }, 201);
  }
  const inviteRevokeMatch = path.match(/^\/v1\/projects\/([^/]+)\/invites\/([^/]+)\/revoke$/);
  if (inviteRevokeMatch) {
    await assertEmptyBody(request);
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "invites.revoke", 30);
    await ctx.runMutation(internal.service.revokeInvite, { ...auth, projectPublicId: expectId(inviteRevokeMatch[1]), invitePublicId: expectId(inviteRevokeMatch[2]), now: Date.now() });
    return new Response(null, { status: 204, headers: JSON_HEADERS });
  }
  const memberRemoveMatch = path.match(/^\/v1\/projects\/([^/]+)\/members\/([^/]+)\/remove$/);
  if (memberRemoveMatch) {
    await assertEmptyBody(request);
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "members.remove", 20);
    await ctx.runMutation(internal.service.removeProjectMember, { ...auth, projectPublicId: expectId(memberRemoveMatch[1]), memberPublicId: expectId(memberRemoveMatch[2]), now: Date.now() });
    return new Response(null, { status: 204, headers: JSON_HEADERS });
  }
  const cardMatch = path.match(/^\/v1\/projects\/([^/]+)\/sync-cards$/);
  if (cardMatch) {
    const body = expectObject(await readJson(request));
    expectExactKeys(body, ["title", "summary"], ["findingId"]);
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "collaboration.sync.create", 30);
    return json(await ctx.runMutation(internal.service.createSyncCard, {
      ...auth, projectPublicId: expectId(cardMatch[1]), cardPublicId: publicId("syn"),
      ...(body.findingId !== undefined ? { findingPublicId: expectId(body.findingId) } : {}),
      title: expectString(body.title, 1, 160), summary: expectString(body.summary, 1, 2000), now: Date.now(),
    }), 201);
  }
  throw new HttpFailure("not_found", 404);
})) });

http.route({ pathPrefix: "/v1/projects/", method: "PATCH", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/projects\/([^/]+)\/member$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["displayName"]);
  const auth = collaborationAuth(request);
  await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "members.rename", 20);
  return json(await ctx.runMutation(internal.service.updateMemberDisplayName, {
    ...auth, projectPublicId: expectId(match[1]), displayName: expectString(body.displayName, 2, 60), now: Date.now(),
  }));
})) });

http.route({ pathPrefix: "/v1/projects/", method: "PUT", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/projects\/([^/]+)\/ai-settings$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const auth = collaborationAuth(request);
  await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "projects.ai-settings.put", 20);
  const write = parseAISettingsWrite(await readJson(request));
  return json(await putProjectAISettingsHandler(ctx, {
    ...auth, projectPublicId: expectId(match[1]), write, now: Date.now(),
  }));
})) });

http.route({ pathPrefix: "/v1/sync-cards/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const path = new URL(request.url).pathname;
  const commentMatch = path.match(/^\/v1\/sync-cards\/([^/]+)\/comments$/);
  if (commentMatch) {
    const body = expectObject(await readJson(request));
    expectExactKeys(body, ["body"]);
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "collaboration.sync.comment", 120);
    return json(await ctx.runMutation(internal.service.commentOnSyncCard, { ...auth, cardPublicId: expectId(commentMatch[1]), commentPublicId: publicId("cmt"), body: expectString(body.body, 1, 2000), now: Date.now() }), 201);
  }
  const resolveMatch = path.match(/^\/v1\/sync-cards\/([^/]+)\/resolve$/);
  if (resolveMatch) {
    const body = expectObject(await readJson(request));
    expectExactKeys(body, ["expectedRevision", "summary", "affectedMemberIds", "affectedWorkstreamIds"]);
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "collaboration.sync.resolve", 30);
    return json(await ctx.runMutation(internal.service.resolveSyncCard, {
      ...auth, cardPublicId: expectId(resolveMatch[1]), decisionPublicId: publicId("dec"), expectedRevision: expectInteger(body.expectedRevision, 1, Number.MAX_SAFE_INTEGER),
      summary: expectString(body.summary, 1, 2000), affectedMemberPublicIds: boundedIds(body.affectedMemberIds, 100), affectedWorkstreamPublicIds: boundedIds(body.affectedWorkstreamIds, 100), now: Date.now(),
    }));
  }
  throw new HttpFailure("not_found", 404);
})) });

// The generic workstream prefix cannot share the projects prefix route above.
http.route({ pathPrefix: "/v1/workstreams/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/workstreams\/([^/]+)\/briefs$/);
  if (!match) throw new HttpFailure("not_found", 404);
  return createBrief(ctx, request, match[1]);
})) });

http.route({ pathPrefix: "/v1/workstreams/", method: "GET", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/workstreams\/([^/]+)\/session-sharing$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const auth = collaborationAuth(request);
  await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "session-sharing.get", 120);
  return json(await ctx.runQuery(internal.service.sessionSharingSnapshot, { ...auth, workstreamPublicId: expectId(match[1]), now: Date.now() }));
})) });

http.route({ pathPrefix: "/v1/workstreams/", method: "DELETE", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/workstreams\/([^/]+)\/session-sharing$/);
  if (!match) throw new HttpFailure("not_found", 404);
  await assertEmptyBody(request);
  const auth = collaborationAuth(request);
  await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "session-sharing.delete", 20);
  await ctx.runMutation(internal.service.deleteSharedSessionMessages, { ...auth, workstreamPublicId: expectId(match[1]), now: Date.now() });
  return new Response(null, { status: 204, headers: { "cache-control": "no-store", "x-content-type-options": "nosniff" } });
})) });

http.route({ path: "/v1/enrollments", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["inviteId", "inviteSecret", "deviceLabel", "appVersion", "schemaMinimum", "schemaMaximum"], ["displayName"]);
  const inviteId = expectId(body.inviteId);
  const inviteSecret = expectString(body.inviteSecret, 22, 512);
  // Derived once: the key is sharded, so deriving it twice would count one
  // request against two shards.
  const rateKey = requestRateKey(request, `enroll:${inviteId}`);
  await consumeEdgeRate(ctx, rateKey, "enrollments.create", 12);
  const deviceToken = randomHex(32);
  const dashboardTicket = randomHex(24);
  const now = Date.now();
  const deviceId = publicId("dev");
  await ctx.runMutation(internal.service.enroll, {
    rateKey,
    invitePublicId: inviteId,
    inviteSecretHash: sha256Hex(inviteSecret),
    devicePublicId: deviceId,
    memberPublicId: publicId("mem"),
    deviceTokenHash: sha256Hex(deviceToken),
    dashboardTicketHash: sha256Hex(dashboardTicket),
    deviceLabel: expectString(body.deviceLabel, 1, 120),
    ...(body.displayName === undefined ? {} : { displayName: expectString(body.displayName, 2, 60) }),
    appVersion: expectString(body.appVersion, 1, 64),
    schemaMinimum: expectInteger(body.schemaMinimum, 1, Number.MAX_SAFE_INTEGER),
    schemaMaximum: expectInteger(body.schemaMaximum, 1, Number.MAX_SAFE_INTEGER),
    now,
    ticketExpiresAt: now + 5 * 60_000,
  });
  return json({ deviceId, deviceToken, dashboardTicket }, 201);
})) });

// Joining another Project as a device that is already enrolled.
//
// /v1/enrollments mints a device; this does not. One Mac runs one Overgent
// service with one device credential, and the local service refuses to register
// a workspace under a second device identity - so without this route the only
// way to accept a second invite was to reset the Mac and lose the first Project.
http.route({ path: "/v1/memberships", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const token = bearer(request);
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["inviteId", "inviteSecret", "deviceLabel", "appVersion", "schemaMinimum", "schemaMaximum"], ["displayName"]);
  const inviteId = expectId(body.inviteId);
  const inviteSecret = expectString(body.inviteSecret, 22, 512);
  // Rated on the credential as well as the invite: a token that can try invites
  // as fast as it likes is an invite-guessing oracle with a valid session.
  await consumeAuthenticatedEdge(ctx, request, "memberships.create", token, 12, 60);
  const dashboardTicket = randomHex(24);
  const now = Date.now();
  const projectId = await ctx.runMutation(internal.service.joinProjectAsDevice, {
    tokenHash: sha256Hex(token),
    rateKey: requestRateKey(request, `join:${inviteId}`),
    invitePublicId: inviteId,
    inviteSecretHash: sha256Hex(inviteSecret),
    memberPublicId: publicId("mem"),
    dashboardTicketHash: sha256Hex(dashboardTicket),
    deviceLabel: expectString(body.deviceLabel, 1, 120),
    ...(body.displayName === undefined ? {} : { displayName: expectString(body.displayName, 2, 60) }),
    schemaMinimum: expectInteger(body.schemaMinimum, 1, Number.MAX_SAFE_INTEGER),
    schemaMaximum: expectInteger(body.schemaMaximum, 1, Number.MAX_SAFE_INTEGER),
    now,
    ticketExpiresAt: now + 5 * 60_000,
  });
  return json({ projectId, dashboardTicket }, 201);
})) });

http.route({ path: "/v1/dashboard-tickets", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const token = bearer(request);
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["projectId"]);
  const projectId = expectId(body.projectId);
  await consumeEdgeRate(ctx, authenticatedRateKey("dashboard.issue", token), "dashboard.issue", 20);
  const ticket = randomHex(24);
  const now = Date.now();
  const expiresAt = now + 5 * 60_000;
  await ctx.runMutation(internal.service.issueDashboardTicket, {
    tokenHash: sha256Hex(token),
    projectPublicId: projectId,
    ticketHash: sha256Hex(ticket),
    now,
    ticketExpiresAt: expiresAt,
  });
  return json({ ticket, expiresAt: new Date(expiresAt).toISOString() }, 201);
})) });

http.route({ path: "/v1/dashboard-tickets/exchange", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["ticket"]);
  const ticket = expectString(body.ticket, 22, 512);
  const rateKey = requestRateKey(request, "ticket-exchange");
  await consumeEdgeRate(ctx, rateKey, "dashboard.exchange", 20);
  const session = randomHex(32);
  const now = Date.now();
  await ctx.runMutation(internal.service.exchangeDashboardTicket, {
    rateKey,
    ticketHash: sha256Hex(ticket),
    sessionHash: sha256Hex(session),
    now,
    sessionExpiresAt: now + 8 * 60 * 60_000,
  });
  return new Response(null, {
    status: 204,
    headers: {
      "cache-control": "no-store",
      "set-cookie": `overgent_session=${session}; Path=/; Max-Age=28800; HttpOnly; Secure; SameSite=Strict`,
      "x-content-type-options": "nosniff",
    },
  });
})) });

http.route({ path: "/v1/dashboard-activations", method: "POST", handler: httpAction(async (ctx, request) => withActivationErrors(request, async () => {
  const ticket = await readActivationTicket(request);
  const rateKey = requestRateKey(request, "dashboard-activation");
  await consumeEdgeRate(ctx, rateKey, "dashboard.activation", 20);
  const session = randomHex(32);
  const now = Date.now();
  await ctx.runMutation(internal.service.exchangeDashboardTicket, {
    rateKey,
    ticketHash: sha256Hex(ticket), sessionHash: sha256Hex(session), now, sessionExpiresAt: now + 8 * 60 * 60_000,
  });
  const requestURL = new URL(request.url);
  const loopback = ["127.0.0.1", "::1", "localhost"].includes(requestURL.hostname);
  return new Response(null, { status: 303, headers: {
    "cache-control": "no-store",
    "location": loopback ? "http://127.0.0.1:5173/?live=1" : "/dashboard?live=1",
    "set-cookie": `overgent_session=${session}; Path=/; Max-Age=28800; HttpOnly; Secure; SameSite=Strict`,
    "x-content-type-options": "nosniff",
  } });
})) });

http.route({ path: "/v1/dashboard/session", method: "GET", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const sessionHash = sha256Hex(browserSession(request));
  await consumeEdgeRate(ctx, sessionHash, "dashboard.session", 120);
  const result = await ctx.runQuery(internal.service.dashboardSession, { sessionHash, now: Date.now() });
  return json(result);
})) });

http.route({ pathPrefix: "/v1/dashboard/projects/", method: "GET", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/dashboard\/projects\/([^/]+)$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const sessionHash = sha256Hex(browserSession(request));
  await consumeEdgeRate(ctx, sessionHash, "dashboard.snapshot", 120);
  const result = await ctx.runQuery(internal.service.dashboardSnapshot, { sessionHash, projectPublicId: expectId(match[1]), now: Date.now() });
  return json(result);
})) });

http.route({ path: "/v1/device/bootstrap", method: "GET", handler: httpAction(async (ctx, request) => withErrors(async () => {
	const token = bearer(request);
	const tokenHash = sha256Hex(token);
	await consumeAuthenticatedEdge(ctx, request, "device.bootstrap", token, 120, 1200);
  const result = await ctx.runQuery(internal.service.bootstrap, { tokenHash });
  return json(result);
})) });

http.route({ path: "/v1/events/batch", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const events = validateEventBatch(await readJson(request));
	const token = bearer(request);
	const tokenHash = sha256Hex(token);
	await consumeAuthenticatedEdge(ctx, request, "events.batch", token, 120, 1200);
  const result = await ctx.runMutation(internal.service.publishEvents, {
    tokenHash,
    events,
    now: Date.now(),
  });
  return json(result);
})) });

http.route({ path: "/v1/presence/heartbeat", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["workspaceId", "state"]);
  const state = expectString(body.state, 4, 6);
  if (!["active", "idle", "paused"].includes(state)) throw new ValidationError("validation_failed");
	const token = bearer(request);
	const tokenHash = sha256Hex(token);
	await consumeAuthenticatedEdge(ctx, request, "presence.heartbeat", token, 12, 2400);
  await ctx.runMutation(internal.service.heartbeat, {
    tokenHash,
    workspacePublicId: expectId(body.workspaceId),
    state,
    now: Date.now(),
  });
  return new Response(null, { status: 204, headers: { "cache-control": "no-store", "x-content-type-options": "nosniff" } });
})) });

http.route({ pathPrefix: "/v1/projects/", method: "GET", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const requestURL = new URL(request.url);
  const aiSettingsMatch = requestURL.pathname.match(/^\/v1\/projects\/([^/]+)\/ai-settings$/);
  if (aiSettingsMatch) {
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "projects.ai-settings.get", 120);
    return json(await ctx.runAction(internal.intelligence.getProjectAISettings, {
      ...auth, projectPublicId: expectId(aiSettingsMatch[1]), now: Date.now(),
    }));
  }
  const accessMatch = requestURL.pathname.match(/^\/v1\/projects\/([^/]+)\/access$/);
  if (accessMatch) {
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "projects.access", 120);
    return json(await ctx.runQuery(internal.service.projectAccess, { ...auth, projectPublicId: expectId(accessMatch[1]), now: Date.now() }));
  }
  const exportMatch = requestURL.pathname.match(/^\/v1\/projects\/([^/]+)\/export$/);
  if (exportMatch) {
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "projects.export", 3);
    const exported = await ctx.runQuery(internal.service.exportProject, { ...auth, projectPublicId: expectId(exportMatch[1]), now: Date.now() });
    return new Response(JSON.stringify(exported, null, 2), { status: 200, headers: { ...JSON_HEADERS, "content-disposition": `attachment; filename="overgent-project-${expectId(exportMatch[1])}.json"` } });
  }
  const collaborationMatch = requestURL.pathname.match(/^\/v1\/projects\/([^/]+)\/collaboration$/);
  if (collaborationMatch) {
    const cursor = requestURL.searchParams.get("cursor");
    if (cursor && cursor.length > 512) throw new ValidationError("validation_failed");
    const after = cursor?.startsWith("time:") ? Number(cursor.slice(5)) : 0;
    if (!Number.isSafeInteger(after) || after < 0) throw new ValidationError("validation_failed");
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "collaboration.snapshot", 120);
    return json(await ctx.runQuery(internal.service.collaborationSnapshot, { ...auth, projectPublicId: expectId(collaborationMatch[1]), after, now: Date.now() }));
  }
  const membersMatch = requestURL.pathname.match(/^\/v1\/projects\/([^/]+)\/members$/);
  if (membersMatch) {
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "members.list", 120);
    return json(await ctx.runQuery(internal.service.projectMembers, { ...auth, projectPublicId: expectId(membersMatch[1]), now: Date.now() }));
  }
  const match = requestURL.pathname.match(/^\/v1\/projects\/([^/]+)\/changes$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const cursor = requestURL.searchParams.get("cursor");
  if (cursor && cursor.length > 512) throw new ValidationError("validation_failed");
  const after = cursor?.startsWith("time:") ? Number(cursor.slice(5)) : 0;
  if (!Number.isSafeInteger(after) || after < 0) throw new ValidationError("validation_failed");
	const token = bearer(request);
	const tokenHash = sha256Hex(token);
	await consumeAuthenticatedEdge(ctx, request, "projects.changes", token, 120, 1200);
  const result = await ctx.runQuery(internal.service.projectChanges, {
    tokenHash,
    projectPublicId: expectId(match[1]),
    after,
  });
  return json(result);
})) });

http.route({ pathPrefix: "/v1/projects/", method: "DELETE", handler: httpAction(async (ctx, request) => withErrors(async () => {
  await assertEmptyBody(request);
  const path = new URL(request.url).pathname;
  const memberMatch = path.match(/^\/v1\/projects\/([^/]+)\/member$/);
  if (memberMatch) {
    const auth = collaborationAuth(request);
    await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "members.delete_self", 3);
    await ctx.runMutation(internal.service.beginMemberDataDeletion, { ...auth, projectPublicId: expectId(memberMatch[1]), now: Date.now() });
    return new Response(null, { status: 202, headers: JSON_HEADERS });
  }
  const match = path.match(/^\/v1\/projects\/([^/]+)$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const auth = collaborationAuth(request);
  await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "projects.delete", 3);
  await ctx.runMutation(internal.service.beginProjectDeletion, { ...auth, projectPublicId: expectId(match[1]), now: Date.now() });
  return new Response(null, { status: 202, headers: JSON_HEADERS });
})) });

http.route({ pathPrefix: "/v1/context-items/", method: "GET", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/context-items\/([^/]+)$/);
  if (!match) throw new HttpFailure("not_found", 404);
	const token = bearer(request);
	const tokenHash = sha256Hex(token);
	await consumeAuthenticatedEdge(ctx, request, "context-items.get", token, 120, 1200);
  const result = await ctx.runQuery(internal.service.contextItem, {
    tokenHash,
    itemPublicId: expectId(match[1]),
  });
  return json(result);
})) });

http.route({ pathPrefix: "/v1/findings/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const path = new URL(request.url).pathname;
  const feedbackMatch = path.match(/^\/v1\/findings\/([^/]+)\/feedback$/);
  if (feedbackMatch) {
    const body = expectObject(await readJson(request));
    expectExactKeys(body, ["value"]);
    const value = expectString(body.value, 6, 32);
    if (!["useful", "not_related", "already_coordinated", "missed_severity"].includes(value)) throw new ValidationError("validation_failed");
    const sessionHash = sha256Hex(browserSession(request));
    await consumeEdgeRate(ctx, sessionHash, "findings.feedback", 60);
    await ctx.runMutation(internal.service.recordFindingFeedback, { sessionHash, findingPublicId: expectId(feedbackMatch[1]), value, feedbackPublicId: publicId("fbk"), now: Date.now() });
    return new Response(null, { status: 204, headers: { "cache-control": "no-store", "x-content-type-options": "nosniff" } });
  }
  const stateMatch = path.match(/^\/v1\/findings\/([^/]+)\/state$/);
  if (stateMatch) {
    const body = expectObject(await readJson(request));
    expectExactKeys(body, ["state"]);
    const state = expectString(body.state, 9, 32);
    if (!["acknowledged", "dismissed"].includes(state)) throw new ValidationError("validation_failed");
    const sessionHash = sha256Hex(browserSession(request));
    await consumeEdgeRate(ctx, sessionHash, "findings.state", 60);
    await ctx.runMutation(internal.service.setFindingState, { sessionHash, findingPublicId: expectId(stateMatch[1]), state, now: Date.now() });
    return new Response(null, { status: 204, headers: { "cache-control": "no-store", "x-content-type-options": "nosniff" } });
  }
  throw new HttpFailure("not_found", 404);
})) });

http.route({ pathPrefix: "/v1/devices/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  await assertEmptyBody(request);
  const match = new URL(request.url).pathname.match(/^\/v1\/devices\/([^/]+)\/revoke$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const auth = collaborationAuth(request);
  await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "devices.revoke", 20);
  await ctx.runMutation(internal.service.revokeDevice, {
    ...auth,
    targetDevicePublicId: expectId(match[1]),
    now: Date.now(),
  });
  return new Response(null, { status: 204, headers: { "cache-control": "no-store", "x-content-type-options": "nosniff" } });
})) });

async function createBrief(ctx: ActionCtx, request: Request, workstreamId: string) {
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["trigger", "approximateTokenBudget"], ["sinceCursor"]);
  const trigger = expectString(body.trigger, 1, 80);
  if (!["begin", "before_broad_edit", "checkpoint", "refresh", "finish", "manual"].includes(trigger)) {
    throw new ValidationError("validation_failed");
  }
  if (body.sinceCursor !== undefined) expectString(body.sinceCursor, 1, 512);
  const token = bearer(request);
  const tokenHash = sha256Hex(token);
  const workstreamPublicId = expectId(workstreamId);
  // Brief creation carries every coordination correction into an agent's next
  // turn, so its budget has to belong to the session that is waiting for one.
  // A single deployment-wide bucket made that budget shared: one session
  // polling for a correction spent the whole fleet's allowance, and every other
  // session's correction was withheld until the fixed window rolled over. The
  // shared bucket stays as a coarse guard on pre-authentication work, sized for
  // a deployment rather than a caller.
  await consumeEdgeRate(ctx, requestRateKey(request, "workstreams.briefs"), "workstreams.briefs.shared", SHARED_BRIEF_CEILING);
  await consumeEdgeRate(ctx, authenticatedRateKey(`workstreams.briefs\0${workstreamPublicId}`, token), "workstreams.briefs", 60);
  const semantic = await ctx.runAction(internal.intelligence.searchSemantic, {
    tokenHash,
    workstreamPublicId,
    limit: 16,
  });
  const result = await ctx.runMutation(internal.service.createBrief, {
    tokenHash,
    workstreamPublicId,
    trigger,
    requestedBudget: expectInteger(body.approximateTokenBudget, 128, 800),
    briefPublicId: publicId("brf"),
    semanticObjectIds: semantic.objectIds,
    semanticDegraded: semantic.degraded,
    semanticContextRevision: semantic.contextRevision,
    now: Date.now(),
  });
  return json(result);
}

async function consumeEdgeRate(ctx: ActionCtx, key: string, route: string, limit: number): Promise<void> {
  await ctx.runMutation(internal.service.consumeRate, {
    key,
    route: `edge.${route}`,
    now: Date.now(),
    limit,
    windowMs: 60_000,
  });
}

async function readJson(request: Request): Promise<unknown> {
  const contentType = request.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase();
  if (contentType !== "application/json") throw new HttpFailure("content_type_unsupported", 415);
  const declared = Number(request.headers.get("content-length") ?? 0);
  if (Number.isFinite(declared) && declared > LIMITS.requestBytes) throw new HttpFailure("request_too_large", 413);
  const bytes = new Uint8Array(await request.arrayBuffer());
  if (bytes.byteLength > LIMITS.requestBytes) throw new HttpFailure("request_too_large", 413);
  try {
    return JSON.parse(new TextDecoder().decode(bytes));
  } catch {
    throw new ValidationError("invalid_json");
  }
}

export function parseAISettingsWrite(value: unknown) {
  const body = expectObject(value);
  expectExactKeys(body, ["judgment", "embeddings"]);
  const judgment = expectObject(body.judgment);
  expectExactKeys(judgment, ["provider", "model"], ["baseUrl", "apiKey"]);
  const judgmentProvider = expectString(judgment.provider, 4, 20);
  if (!["anthropic", "openai-compatible", "none"].includes(judgmentProvider)) throw new ValidationError("validation_failed");
  const embeddings = expectObject(body.embeddings);
  expectExactKeys(embeddings, ["provider", "model", "dimensions"], ["baseUrl", "apiKey"]);
  const embeddingProvider = expectString(embeddings.provider, 6, 13);
  if (!["openai", "deterministic"].includes(embeddingProvider)) throw new ValidationError("validation_failed");
  const dimensions = expectInteger(embeddings.dimensions, 1, 3072);
  if (dimensions !== 1024) throw new ValidationError("unsupported_dimensions");
  return {
    judgment: {
      provider: judgmentProvider,
      model: expectString(judgment.model, 1, 120),
      ...(judgment.baseUrl === undefined ? {} : { baseUrl: providerBaseURL(judgment.baseUrl) }),
      ...(judgment.apiKey === undefined ? {} : { apiKey: providerKey(judgment.apiKey) }),
    },
    embeddings: {
      provider: embeddingProvider,
      model: expectString(embeddings.model, 1, 120),
      dimensions,
      ...(embeddings.baseUrl === undefined ? {} : { baseUrl: providerBaseURL(embeddings.baseUrl) }),
      ...(embeddings.apiKey === undefined ? {} : { apiKey: providerKey(embeddings.apiKey) }),
    },
  };
}

function providerKey(value: unknown): string {
  if (value === "") return "";
  const key = expectString(value, 8, 512);
  if (/\s/.test(key)) throw new ValidationError("validation_failed");
  return key;
}

function providerBaseURL(value: unknown): string {
  const raw = expectString(value, 1, 2048);
  let parsed: URL;
  try { parsed = new URL(raw); } catch { throw new ValidationError("validation_failed"); }
  const loopback = ["127.0.0.1", "::1", "localhost"].includes(parsed.hostname);
  if (parsed.username || parsed.password || parsed.pathname !== "/" || parsed.search || parsed.hash || (parsed.protocol !== "https:" && !(parsed.protocol === "http:" && loopback))) {
    throw new ValidationError("validation_failed");
  }
  return parsed.origin;
}

async function readActivationTicket(request: Request): Promise<string> {
  const contentType = request.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase();
  if (contentType !== "application/x-www-form-urlencoded") throw new HttpFailure("content_type_unsupported", 415);
  const bytes = new Uint8Array(await request.arrayBuffer());
  if (bytes.byteLength > 1024) throw new HttpFailure("request_too_large", 413);
  const form = new URLSearchParams(new TextDecoder().decode(bytes));
  if ([...form.keys()].length !== 1 || !form.has("ticket")) throw new ValidationError("validation_failed");
  return expectString(form.get("ticket"), 22, 512);
}

function bearer(request: Request): string {
  const header = request.headers.get("authorization");
  if (!header?.startsWith("Bearer ")) throw new HttpFailure("unauthorized", 401);
  const token = header.slice(7);
  if (token.length < 32 || token.length > 512 || /\s/.test(token)) throw new HttpFailure("unauthorized", 401);
  return token;
}

function browserSession(request: Request): string {
  const cookie = request.headers.get("cookie") ?? "";
  for (const part of cookie.split(";")) {
    const [name, value] = part.trim().split("=", 2);
    if (name === "overgent_session" && value && /^[a-f0-9]{64}$/.test(value)) return value;
  }
  throw new HttpFailure("unauthorized", 401);
}

function collaborationAuth(request: Request): { tokenHash?: string; sessionHash?: string } {
  const authorization = request.headers.get("authorization");
  if (authorization) return { tokenHash: sha256Hex(bearer(request)) };
  return { sessionHash: sha256Hex(browserSession(request)) };
}

function boundedStrings(value: unknown, minimum: number, maximum: number, maximumLength: number): string[] {
  if (!Array.isArray(value) || value.length < minimum || value.length > maximum) throw new ValidationError("validation_failed");
  const strings = value.map((entry) => expectString(entry, 1, maximumLength));
  if (new Set(strings).size !== strings.length) throw new ValidationError("validation_failed");
  return strings;
}

function boundedIds(value: unknown, maximum: number): string[] {
  if (!Array.isArray(value) || value.length > maximum) throw new ValidationError("validation_failed");
  const ids = value.map(expectId);
  if (new Set(ids).size !== ids.length) throw new ValidationError("validation_failed");
  return ids;
}

// SHARED_BRIEF_CEILING bounds brief creation for a whole deployment, not for a
// caller. It exists only so an unauthenticated flood cannot mint a fresh rate
// bucket per forged credential; honest fleet traffic is governed by the
// per-session bucket well below it.
const SHARED_BRIEF_CEILING = 600;

// Shards for the shared unauthenticated bucket.
//
// The bucket cannot be keyed on anything the caller controls: Convex documents
// no trustworthy client-IP header here, and a key derived from forwarding data
// is a key an abuser chooses. That left one key per route - and therefore one
// document that every request on that route writes to.
//
// Two things followed. Convex serializes writes to a single document, so a hot
// row turns ordinary concurrency into optimistic-concurrency failures, which
// reach the caller as an unexplained 500 rather than as a limit they can act
// on; on the activation route that is a member being told Overgent "could not
// open the live Project" for no reason they can see or fix. And a ceiling low
// enough to be a useful per-caller limit is far too low as a product-wide one:
// five Project creations a minute is a number three people setting up at the
// same time will hit.
//
// Sharding fixes both without touching a single security constant. Writes
// spread across shards, and the per-route limits below are now per shard - so
// each is a product-wide abuse ceiling of limit x SHARED_RATE_SHARDS, which is
// what a bucket nobody can be identified within should always have been.
// Callers that do have a credential are still bounded individually by the
// authenticated bucket alongside this one.
const SHARED_RATE_SHARDS = 16;

function requestRateKey(_request: Request, scope: string): string {
  const shard = Math.floor(Math.random() * SHARED_RATE_SHARDS);
  return sha256Hex(`${scope}\0shared-unauthenticated\0${shard}`);
}

function authenticatedRateKey(scope: string, token: string): string {
  return sha256Hex(`${scope}\0credential\0${sha256Hex(token)}`);
}

async function consumeAuthenticatedEdge(ctx: ActionCtx, request: Request, scope: string, token: string, perCredentialLimit: number, sharedLimit: number): Promise<void> {
  // The shared bucket bounds work performed before a forged credential is
  // rejected. The second bucket gives every real device its own allowance.
  await consumeEdgeRate(ctx, requestRateKey(request, `${scope}.shared`), `${scope}.shared`, sharedLimit);
  await consumeEdgeRate(ctx, authenticatedRateKey(scope, token), scope, perCredentialLimit);
}

async function assertEmptyBody(request: Request): Promise<void> {
  const declared = Number(request.headers.get("content-length") ?? 0);
  if (Number.isFinite(declared) && declared > LIMITS.requestBytes) throw new HttpFailure("request_too_large", 413);
  const bytes = new Uint8Array(await request.arrayBuffer());
  if (bytes.byteLength > LIMITS.requestBytes) throw new HttpFailure("request_too_large", 413);
  if (bytes.byteLength !== 0) throw new ValidationError("validation_failed");
}

function json(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), { status, headers: JSON_HEADERS });
}

async function withErrors(operation: () => Promise<Response>): Promise<Response> {
  try {
    return await operation();
  } catch (error) {
    const failure = classify(error);
    return json({
      error: {
        code: failure.code,
        message: errorMessage(failure.code),
        requestId: publicId("req"),
        retryable: failure.status >= 500,
      },
    }, failure.status);
  }
}

async function withActivationErrors(request: Request, operation: () => Promise<Response>): Promise<Response> {
  try {
    return await operation();
  } catch (error) {
    const failure = classify(error);
    const hostname = new URL(request.url).hostname;
    const development = ["127.0.0.1", "::1", "localhost"].includes(hostname);
    return activationFailureResponse(failure.code, failure.status, development);
  }
}

class HttpFailure extends Error {
  constructor(readonly code: string, readonly status: number) {
    super(code);
  }
}

function classify(error: unknown): { code: string; status: number } {
  if (error instanceof HttpFailure) return error;
  if (error instanceof ValidationError) return { code: error.code, status: error.code === "schema_version_unsupported" ? 409 : 400 };
  const match = String(error).match(/E:([a-z0-9_]+)/);
  const code = match?.[1] ?? "internal_error";
  if (["unauthorized", "credential_revoked"].includes(code)) return { code, status: 401 };
  if (["forbidden"].includes(code)) return { code, status: 403 };
  if (["email_identity_rejected"].includes(code)) return { code, status: 400 };
  if (["unsupported_dimensions"].includes(code)) return { code, status: 400 };
  if (["not_found", "workspace_not_registered", "workstream_not_found", "manifest_not_found"].includes(code)) return { code, status: 404 };
  if (["rate_limited"].includes(code)) return { code, status: 429 };
  if (["internal_error"].includes(code)) return { code, status: 500 };
  if (["secrets_key_unconfigured"].includes(code)) return { code, status: 503 };
  return { code, status: 409 };
}

function errorMessage(code: string): string {
  return ({
    unauthorized: "Authentication is required.",
    credential_revoked: "This device credential has been revoked.",
    forbidden: "This operation is not authorized for the requested Project.",
    not_found: "The requested resource was not found.",
    rate_limited: "Too many requests; retry later.",
    schema_version_unsupported: "Upgrade Overgent to continue.",
    request_too_large: "The request exceeds the supported size.",
    email_identity_rejected: "Choose a display name; an email address cannot be your Project identity.",
    unsupported_dimensions: "This deployment's vector index requires 1024 embedding dimensions.",
    secrets_key_unconfigured: "This backend cannot store provider keys until OVERGENT_SECRETS_KEY is configured.",
    already_member: "This Mac has already joined that Project.",
    invite_invalid: "That invite code was not recognised.",
    invite_revoked: "That invite was revoked. Ask for a new one.",
    invite_expired: "That invite has expired. Ask for a new one.",
    invite_consumed: "That invite has already been used. Ask for a new one.",
    internal_error: "The service could not complete the request.",
  } as Record<string, string>)[code] ?? "The request could not be completed.";
}

export default http;
