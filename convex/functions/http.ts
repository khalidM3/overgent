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
  expectExactKeys(body, ["label", "deviceLabel"], ["displayName"]);
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
    ...(displayName !== undefined ? { displayName } : {}),
    now: Date.now(),
  });
  return json(project, 201);
})) });

http.route({ pathPrefix: "/v1/projects/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const path = new URL(request.url).pathname;
  const inviteMatch = path.match(/^\/v1\/projects\/([^/]+)\/invites$/);
  if (inviteMatch) {
    const token = bearer(request);
    await consumeEdgeRate(ctx, requestRateKey(request, "invites.create"), "invites.create", 20);
    const body = expectObject(await readJson(request));
    expectExactKeys(body, ["expiresInSeconds", "maxUses"]);
    const expiresInSeconds = expectInteger(body.expiresInSeconds, 60, 604_800);
    const maxUses = expectInteger(body.maxUses, 1, 50);
    const secret = randomHex(16);
    const inviteId = publicId("inv");
    const now = Date.now();
    await ctx.runMutation(internal.service.createInvite, {
      tokenHash: sha256Hex(token),
      projectPublicId: expectId(inviteMatch[1]),
      invitePublicId: inviteId,
      secretHash: sha256Hex(secret),
      expiresAt: now + expiresInSeconds * 1_000,
      maxUses,
      now,
    });
    return json({ id: inviteId, secret, expiresAt: new Date(now + expiresInSeconds * 1_000).toISOString() }, 201);
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

http.route({ pathPrefix: "/v1/workstreams/", method: "PUT", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/workstreams\/([^/]+)\/session-sharing$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["profile", "audience", "consentVersion", "allowedKinds"], ["expiresInSeconds"]);
  const profile = expectString(body.profile, 7, 12);
  const audience = expectString(body.audience, 4, 7);
  const consentVersion = expectString(body.consentVersion, 16, 16);
  if (!["private", "conversation"].includes(profile) || !["self", "project"].includes(audience) || consentVersion !== "session-share/v1") throw new ValidationError("validation_failed");
  const allowedKinds = boundedStrings(body.allowedKinds, profile === "conversation" ? 1 : 0, 4, 32);
  if (allowedKinds.some((kind) => !["user", "assistant", "thinking", "system"].includes(kind)) || (profile === "private" && allowedKinds.length > 0)) throw new ValidationError("validation_failed");
  const now = Date.now();
  const expiresInSeconds = body.expiresInSeconds === undefined ? undefined : expectInteger(body.expiresInSeconds, 300, 2_592_000);
  const auth = collaborationAuth(request);
  await consumeEdgeRate(ctx, auth.tokenHash ?? auth.sessionHash!, "session-sharing.update", 30);
  return json(await ctx.runMutation(internal.service.updateSessionSharing, {
    ...auth, workstreamPublicId: expectId(match[1]), profile, audience, consentVersion, allowedKinds,
    ...(expiresInSeconds !== undefined ? { expiresAt: now + expiresInSeconds * 1000 } : {}), now,
  }));
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
  await consumeEdgeRate(ctx, requestRateKey(request, `enroll:${inviteId}`), "enrollments.create", 12);
  const deviceToken = randomHex(32);
  const dashboardTicket = randomHex(24);
  const now = Date.now();
  const deviceId = publicId("dev");
  await ctx.runMutation(internal.service.enroll, {
    rateKey: requestRateKey(request, `enroll:${inviteId}`),
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
  await consumeEdgeRate(ctx, requestRateKey(request, "ticket-exchange"), "dashboard.exchange", 20);
  const session = randomHex(32);
  const now = Date.now();
  await ctx.runMutation(internal.service.exchangeDashboardTicket, {
    rateKey: requestRateKey(request, "ticket-exchange"),
    ticketHash: sha256Hex(ticket),
    sessionHash: sha256Hex(session),
    now,
    sessionExpiresAt: now + 8 * 60 * 60_000,
  });
  return new Response(null, {
    status: 204,
    headers: {
      "cache-control": "no-store",
      "set-cookie": `stickguy_session=${session}; Path=/; Max-Age=28800; HttpOnly; Secure; SameSite=Strict`,
      "x-content-type-options": "nosniff",
    },
  });
})) });

http.route({ path: "/v1/dashboard-activations", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const ticket = await readActivationTicket(request);
  await consumeEdgeRate(ctx, requestRateKey(request, "dashboard-activation"), "dashboard.activation", 20);
  const session = randomHex(32);
  const now = Date.now();
  await ctx.runMutation(internal.service.exchangeDashboardTicket, {
    rateKey: requestRateKey(request, "dashboard-activation"),
    ticketHash: sha256Hex(ticket), sessionHash: sha256Hex(session), now, sessionExpiresAt: now + 8 * 60 * 60_000,
  });
  const requestURL = new URL(request.url);
  const loopback = ["127.0.0.1", "::1", "localhost"].includes(requestURL.hostname);
  return new Response(null, { status: 303, headers: {
    "cache-control": "no-store",
    "location": loopback ? "http://127.0.0.1:5173/?live=1" : "/dashboard?live=1",
    "set-cookie": `stickguy_session=${session}; Path=/; Max-Age=28800; HttpOnly; Secure; SameSite=Strict`,
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
  const tokenHash = sha256Hex(bearer(request));
  await consumeEdgeRate(ctx, requestRateKey(request, "device.bootstrap"), "device.bootstrap", 120);
  const result = await ctx.runQuery(internal.service.bootstrap, { tokenHash });
  return json(result);
})) });

http.route({ path: "/v1/events/batch", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const events = validateEventBatch(await readJson(request));
  const tokenHash = sha256Hex(bearer(request));
  await consumeEdgeRate(ctx, requestRateKey(request, "events.batch"), "events.batch", 120);
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
  const tokenHash = sha256Hex(bearer(request));
  await consumeEdgeRate(ctx, requestRateKey(request, "presence.heartbeat"), "presence.heartbeat", 12);
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
  const tokenHash = sha256Hex(bearer(request));
  await consumeEdgeRate(ctx, requestRateKey(request, "projects.changes"), "projects.changes", 120);
  const result = await ctx.runQuery(internal.service.projectChanges, {
    tokenHash,
    projectPublicId: expectId(match[1]),
    after,
  });
  return json(result);
})) });

http.route({ pathPrefix: "/v1/context-items/", method: "GET", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/context-items\/([^/]+)$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const tokenHash = sha256Hex(bearer(request));
  await consumeEdgeRate(ctx, requestRateKey(request, "context-items.get"), "context-items.get", 120);
  const result = await ctx.runQuery(internal.service.contextItem, {
    tokenHash,
    itemPublicId: expectId(match[1]),
  });
  return json(result);
})) });

http.route({ pathPrefix: "/v1/findings/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/findings\/([^/]+)\/feedback$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["value"]);
  const value = expectString(body.value, 6, 32);
  if (!["useful", "not_related", "already_coordinated", "missed_severity"].includes(value)) throw new ValidationError("validation_failed");
  const sessionHash = sha256Hex(browserSession(request));
  await consumeEdgeRate(ctx, sessionHash, "findings.feedback", 60);
  await ctx.runMutation(internal.service.recordFindingFeedback, { sessionHash, findingPublicId: expectId(match[1]), value, feedbackPublicId: publicId("fbk"), now: Date.now() });
  return new Response(null, { status: 204, headers: { "cache-control": "no-store", "x-content-type-options": "nosniff" } });
})) });

http.route({ pathPrefix: "/v1/devices/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  await assertEmptyBody(request);
  const match = new URL(request.url).pathname.match(/^\/v1\/devices\/([^/]+)\/revoke$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const tokenHash = sha256Hex(bearer(request));
  await consumeEdgeRate(ctx, requestRateKey(request, "devices.revoke"), "devices.revoke", 20);
  await ctx.runMutation(internal.service.revokeDevice, {
    tokenHash,
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
  const tokenHash = sha256Hex(bearer(request));
  await consumeEdgeRate(ctx, requestRateKey(request, "workstreams.briefs"), "workstreams.briefs", 60);
  const semantic = await ctx.runAction(internal.intelligence.searchSemantic, {
    tokenHash,
    workstreamPublicId: expectId(workstreamId),
    limit: 16,
  });
  const result = await ctx.runMutation(internal.service.createBrief, {
    tokenHash,
    workstreamPublicId: expectId(workstreamId),
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
    if (name === "stickguy_session" && value && /^[a-f0-9]{64}$/.test(value)) return value;
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

function requestRateKey(_request: Request, scope: string): string {
  // Convex does not document a trustworthy client-IP header at this boundary.
  // A shared bucket cannot be bypassed with caller-controlled forwarding data.
  return sha256Hex(`${scope}\0shared-unauthenticated`);
}

function authenticatedRateKey(scope: string, token: string): string {
  return sha256Hex(`${scope}\0credential\0${sha256Hex(token)}`);
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
  if (["not_found", "workspace_not_registered", "workstream_not_found", "manifest_not_found"].includes(code)) return { code, status: 404 };
  if (["rate_limited"].includes(code)) return { code, status: 429 };
  if (["internal_error"].includes(code)) return { code, status: 500 };
  return { code, status: 409 };
}

function errorMessage(code: string): string {
  return ({
    unauthorized: "Authentication is required.",
    credential_revoked: "This device credential has been revoked.",
    forbidden: "This operation is not authorized for the requested Project.",
    not_found: "The requested resource was not found.",
    rate_limited: "Too many requests; retry later.",
    schema_version_unsupported: "Upgrade Stickguy to continue.",
    request_too_large: "The request exceeds the supported size.",
    email_identity_rejected: "Choose a display name; an email address cannot be your Project identity.",
    internal_error: "The service could not complete the request.",
  } as Record<string, string>)[code] ?? "The request could not be completed.";
}

export default http;
