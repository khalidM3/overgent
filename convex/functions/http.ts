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
  expectExactKeys(body, ["label", "deviceLabel"]);
  const label = expectString(body.label, 1, 120);
  const deviceLabel = expectString(body.deviceLabel, 1, 120);
  const project = await ctx.runMutation(internal.service.createProject, {
    tokenHash: sha256Hex(token),
    projectPublicId: publicId("prj"),
    memberPublicId: publicId("mem"),
    devicePublicId: publicId("dev"),
    label,
    deviceLabel,
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
  throw new HttpFailure("not_found", 404);
})) });

// The generic workstream prefix cannot share the projects prefix route above.
http.route({ pathPrefix: "/v1/workstreams/", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const match = new URL(request.url).pathname.match(/^\/v1\/workstreams\/([^/]+)\/briefs$/);
  if (!match) throw new HttpFailure("not_found", 404);
  return createBrief(ctx, request, match[1]);
})) });

http.route({ path: "/v1/enrollments", method: "POST", handler: httpAction(async (ctx, request) => withErrors(async () => {
  const body = expectObject(await readJson(request));
  expectExactKeys(body, ["inviteId", "inviteSecret", "deviceLabel", "appVersion", "schemaMinimum", "schemaMaximum"]);
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
  const match = new URL(request.url).pathname.match(/^\/v1\/projects\/([^/]+)\/changes$/);
  if (!match) throw new HttpFailure("not_found", 404);
  const cursor = new URL(request.url).searchParams.get("cursor");
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
  const result = await ctx.runMutation(internal.service.createBrief, {
    tokenHash,
    workstreamPublicId: expectId(workstreamId),
    trigger,
    requestedBudget: expectInteger(body.approximateTokenBudget, 128, 800),
    briefPublicId: publicId("brf"),
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

function bearer(request: Request): string {
  const header = request.headers.get("authorization");
  if (!header?.startsWith("Bearer ")) throw new HttpFailure("unauthorized", 401);
  const token = header.slice(7);
  if (token.length < 32 || token.length > 512 || /\s/.test(token)) throw new HttpFailure("unauthorized", 401);
  return token;
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
    internal_error: "The service could not complete the request.",
  } as Record<string, string>)[code] ?? "The request could not be completed.";
}

export default http;
