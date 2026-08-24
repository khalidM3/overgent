import assert from "node:assert/strict";
import { createHash, randomBytes } from "node:crypto";
import { readFileSync } from "node:fs";

const envText = readFileSync(new URL("../.env.local", import.meta.url), "utf8");
const siteMatch = envText.match(/^CONVEX_SITE_URL=(.+)$/m);
assert(siteMatch, "anonymous local HTTP actions URL is missing");
const siteUrl = siteMatch[1].trim().replace(/^['"]|['"]$/g, "");
const parsed = new URL(siteUrl);
assert(["127.0.0.1", "localhost"].includes(parsed.hostname), "live L2 suite refuses non-loopback deployment");

const suffix = randomBytes(6).toString("hex");
const tokenA = randomBytes(32).toString("hex");
const tokenC = randomBytes(32).toString("hex");
const ids = {
  workspaceA: `wsp_a_${suffix}`,
  workspaceB: `wsp_b_${suffix}`,
  workspaceC: `wsp_c_${suffix}`,
  sessionA: `ses_a_${suffix}`,
  sessionB: `ses_b_${suffix}`,
  sessionC: `ses_c_${suffix}`,
  workstreamA: `wrk_a_${suffix}`,
  workstreamB: `wrk_b_${suffix}`,
};
const timings = {};

async function request(method, path, { token, body, expected = 200 } = {}) {
  const headers = {};
  if (token) headers.authorization = `Bearer ${token}`;
  let payload;
  if (body !== undefined) {
    headers["content-type"] = "application/json";
    payload = JSON.stringify(body);
  }
  const started = performance.now();
  const response = await fetch(`${siteUrl}${path}`, { method, headers, body: payload });
  const text = await response.text();
  let parsedBody = null;
  if (text) parsedBody = JSON.parse(text);
  assert.equal(response.status, expected, `${method} ${path}: ${response.status} ${parsedBody?.error?.code ?? ""}`);
  return { body: parsedBody, headers: response.headers, elapsedMs: Math.round(performance.now() - started) };
}

function manifestHash(entries) {
  const hash = createHash("sha256");
  for (const entry of entries) {
    const fields = [entry.path];
    for (const layer of ["baseline", "index", "worktree"]) {
      const change = entry.states[layer];
      fields.push(layer, change?.status ?? "", change?.oldPath ?? "");
    }
    hash.update(`${fields.join("\0")}\0`);
  }
  return hash.digest("hex");
}

function event({ eventId, projectId, deviceId, workspaceId, sessionId, sequence, type, payload }) {
  return {
    schemaVersion: 1,
    eventId,
    projectId,
    memberId: `mem_claim_${suffix}`,
    deviceId,
    workspaceId,
    sessionId,
    sequence,
    observedAt: "2026-08-23T19:00:00Z",
    sentAt: "2026-08-23T19:00:01Z",
    source: "git",
    type,
    payload,
  };
}

const createStarted = performance.now();
const projectA = (await request("POST", "/v1/projects", {
  token: tokenA,
  body: { label: `Synthetic Project A ${suffix}`, deviceLabel: "Synthetic Device A" },
  expected: 201,
})).body;
const bootstrapA = (await request("GET", "/v1/device/bootstrap", { token: tokenA })).body;
assert.equal(bootstrapA.projects[0].id, projectA.id);
assert(bootstrapA.deviceId.startsWith("dev_"));
const creatorTicket = (await request("POST", "/v1/dashboard-tickets", {
  token: tokenA,
  body: { projectId: projectA.id },
  expected: 201,
})).body;
assert(creatorTicket.ticket.length >= 22);
assert(Number.isFinite(Date.parse(creatorTicket.expiresAt)));
const creatorExchange = await request("POST", "/v1/dashboard-tickets/exchange", {
  body: { ticket: creatorTicket.ticket },
  expected: 204,
});
assert((creatorExchange.headers.get("set-cookie") ?? "").includes("HttpOnly"));
const creatorTicketReuse = await request("POST", "/v1/dashboard-tickets/exchange", {
  body: { ticket: creatorTicket.ticket },
  expected: 409,
});
assert.equal(creatorTicketReuse.body.error.code, "ticket_consumed");
timings.creatorEnrollmentMs = Math.round(performance.now() - createStarted);

const invite = (await request("POST", `/v1/projects/${projectA.id}/invites`, {
  token: tokenA,
  body: { expiresInSeconds: 600, maxUses: 1 },
  expected: 201,
})).body;
const enrollmentB = (await request("POST", "/v1/enrollments", {
  body: {
    inviteId: invite.id,
    inviteSecret: invite.secret,
    deviceLabel: "Synthetic Device B",
    appVersion: "fixture/1",
    schemaMinimum: 1,
    schemaMaximum: 1,
  },
  expected: 201,
})).body;
const tokenB = enrollmentB.deviceToken;
const bootstrapB = (await request("GET", "/v1/device/bootstrap", { token: tokenB })).body;
assert.equal(bootstrapB.projects[0].id, projectA.id);

const exchange = await request("POST", "/v1/dashboard-tickets/exchange", {
  body: { ticket: enrollmentB.dashboardTicket },
  expected: 204,
});
const cookie = exchange.headers.get("set-cookie") ?? "";
assert(cookie.includes("HttpOnly"));
assert(cookie.includes("Secure"));
assert(cookie.includes("SameSite=Strict"));
const reusedTicket = await request("POST", "/v1/dashboard-tickets/exchange", {
  body: { ticket: enrollmentB.dashboardTicket },
  expected: 409,
});
assert.equal(reusedTicket.body.error.code, "ticket_consumed");

const consumedInvite = await request("POST", "/v1/enrollments", {
  body: {
    inviteId: invite.id,
    inviteSecret: invite.secret,
    deviceLabel: "Synthetic Device Reuse",
    appVersion: "fixture/1",
    schemaMinimum: 1,
    schemaMaximum: 1,
  },
  expected: 409,
});
assert.equal(consumedInvite.body.error.code, "invite_consumed");

const projectC = (await request("POST", "/v1/projects", {
  token: tokenC,
  body: { label: `Synthetic Project C ${suffix}`, deviceLabel: "Synthetic Device C" },
  expected: 201,
})).body;
const bootstrapC = (await request("GET", "/v1/device/bootstrap", { token: tokenC })).body;

const registerA = event({
  eventId: `evt_reg_a_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
  workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 1, type: "workspace.registered",
  payload: { repoFingerprint: "repo_fixture_shared", label: "Synthetic A", capabilities: { git: true, mcp: true } },
});
const registerB = event({
  eventId: `evt_reg_b_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
  workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 1, type: "workspace.registered",
  payload: { repoFingerprint: "repo_fixture_shared", label: "Synthetic B", capabilities: { git: true, mcp: true } },
});
const registerC = event({
  eventId: `evt_reg_c_${suffix}`, projectId: projectC.id, deviceId: bootstrapC.deviceId,
  workspaceId: ids.workspaceC, sessionId: ids.sessionC, sequence: 1, type: "workspace.registered",
  payload: { repoFingerprint: "repo_fixture_other", label: "Synthetic C", capabilities: { git: true } },
});
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [registerA] } });
await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [registerB] } });
await request("POST", "/v1/events/batch", { token: tokenC, body: { events: [registerC] } });
await request("POST", "/v1/presence/heartbeat", {
  token: tokenB,
  body: { workspaceId: ids.workspaceB, state: "active" },
  expected: 204,
});

function manifestEvents({ side, projectId, deviceId, workspaceId, sessionId, workstreamId, entries }) {
  const baseline = "a".repeat(40);
  const head = side === "a" ? "b".repeat(40) : "c".repeat(40);
  const manifestId = `mft_${side}_${suffix}`;
  return [
    event({
      eventId: `evt_intent_${side}_${suffix}`, projectId, deviceId, workspaceId, sessionId, sequence: 2,
      type: "workstream.intent_reported",
      payload: { workstreamId, title: `Synthetic ${side}`, intendedOutcome: "Exercise deterministic hosted overlap." },
    }),
    event({
      eventId: `evt_start_${side}_${suffix}`, projectId, deviceId, workspaceId, sessionId, sequence: 3,
      type: "workspace.manifest_started",
      payload: { manifestId, revision: 1, workstreamId, baselineRef: baseline, headRef: head, chunkCount: 1 },
    }),
    event({
      eventId: `evt_chunk_${side}_${suffix}`, projectId, deviceId, workspaceId, sessionId, sequence: 4,
      type: "workspace.manifest_chunk",
      payload: { manifestId, chunkIndex: 0, entries },
    }),
    event({
      eventId: `evt_complete_${side}_${suffix}`, projectId, deviceId, workspaceId, sessionId, sequence: 5,
      type: "workspace.manifest_completed",
      payload: { manifestId, revision: 1, contentHash: manifestHash(entries) },
    }),
  ];
}

const entriesA = [
  { path: "synthetic/a-only.ts", states: { worktree: { status: "added" } } },
  { path: "synthetic/shared.ts", states: { baseline: { status: "modified" }, index: { status: "modified" }, worktree: { status: "modified" } } },
];
const entriesB = [
  { path: "synthetic/b-only.ts", states: { index: { status: "added" } } },
  { path: "synthetic/shared.ts", states: { baseline: { status: "modified" }, worktree: { status: "modified" } } },
];
await request("POST", "/v1/events/batch", {
  token: tokenA,
  body: { events: manifestEvents({
    side: "a", projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA,
    sessionId: ids.sessionA, workstreamId: ids.workstreamA, entries: entriesA,
  }) },
});
const publishBStarted = performance.now();
await request("POST", "/v1/events/batch", {
  token: tokenB,
  body: { events: manifestEvents({
    side: "b", projectId: projectA.id, deviceId: bootstrapB.deviceId, workspaceId: ids.workspaceB,
    sessionId: ids.sessionB, workstreamId: ids.workstreamB, entries: entriesB,
  }) },
});
timings.secondManifestAndFindingMs = Math.round(performance.now() - publishBStarted);

const largeEntries = Array.from({ length: 1_000 }, (_, index) => ({
  path: index === 0 ? "synthetic/shared.ts" : `synthetic/large-${String(index).padStart(4, "0")}.ts`,
  states: index === 0
    ? { baseline: { status: "modified" }, index: { status: "modified" }, worktree: { status: "modified" } }
    : { worktree: { status: "added" } },
})).sort((left, right) => Buffer.compare(Buffer.from(left.path), Buffer.from(right.path)));
const largeManifestId = `mft_large_${suffix}`;
const largeEvents = [
  event({
    eventId: `evt_large_start_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 6, type: "workspace.manifest_started",
    payload: {
      manifestId: largeManifestId, revision: 2, workstreamId: ids.workstreamA,
      baselineRef: "a".repeat(40), headRef: "d".repeat(40), chunkCount: 10,
    },
  }),
  ...Array.from({ length: 10 }, (_, chunkIndex) => event({
    eventId: `evt_large_chunk_${chunkIndex}_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 7 + chunkIndex, type: "workspace.manifest_chunk",
    payload: { manifestId: largeManifestId, chunkIndex, entries: largeEntries.slice(chunkIndex * 100, (chunkIndex + 1) * 100) },
  })),
  event({
    eventId: `evt_large_complete_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 17, type: "workspace.manifest_completed",
    payload: { manifestId: largeManifestId, revision: 2, contentHash: manifestHash(largeEntries) },
  }),
];
const largeStarted = performance.now();
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: largeEvents } });
timings.atomicManifest1000PathsMs = Math.round(performance.now() - largeStarted);

const emptyManifestId = `mft_empty_${suffix}`;
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({
    eventId: `evt_empty_start_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 18, type: "workspace.manifest_started",
    payload: { manifestId: emptyManifestId, revision: 3, workstreamId: ids.workstreamA, baselineRef: "a".repeat(40), headRef: "d".repeat(40), chunkCount: 0 },
  }),
  event({
    eventId: `evt_empty_complete_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 19, type: "workspace.manifest_completed",
    payload: { manifestId: emptyManifestId, revision: 3, contentHash: manifestHash([]) },
  }),
] } });

const staleManifestId = `mft_stale_${suffix}`;
const staleEntries = [{ path: "synthetic/stale.ts", states: { worktree: { status: "added" } } }];
const staleAck = (await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({
    eventId: `evt_stale_start_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 20, type: "workspace.manifest_started",
    payload: { manifestId: staleManifestId, revision: 2, workstreamId: ids.workstreamA, baselineRef: "a".repeat(40), headRef: "e".repeat(40), chunkCount: 1 },
  }),
  event({
    eventId: `evt_stale_chunk_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 21, type: "workspace.manifest_chunk",
    payload: { manifestId: staleManifestId, chunkIndex: 0, entries: staleEntries },
  }),
  event({
    eventId: `evt_stale_complete_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 22, type: "workspace.manifest_completed",
    payload: { manifestId: staleManifestId, revision: 2, contentHash: manifestHash(staleEntries) },
  }),
] } })).body;
assert(staleAck.acceptedEventIds.includes(`evt_stale_complete_${suffix}`));

const unorderedManifestId = `mft_unordered_${suffix}`;
const unorderedEntries = [
  { path: "synthetic/z.ts", states: { worktree: { status: "modified" } } },
  { path: "synthetic/a.ts", states: { worktree: { status: "modified" } } },
];
const unordered = await request("POST", "/v1/events/batch", { token: tokenA, expected: 409, body: { events: [
  event({
    eventId: `evt_unordered_start_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 23, type: "workspace.manifest_started",
    payload: { manifestId: unorderedManifestId, revision: 4, workstreamId: ids.workstreamA, baselineRef: "a".repeat(40), headRef: "f".repeat(40), chunkCount: 1 },
  }),
  event({
    eventId: `evt_unordered_chunk_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 24, type: "workspace.manifest_chunk",
    payload: { manifestId: unorderedManifestId, chunkIndex: 0, entries: unorderedEntries },
  }),
  event({
    eventId: `evt_unordered_complete_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 25, type: "workspace.manifest_completed",
    payload: { manifestId: unorderedManifestId, revision: 4, contentHash: manifestHash(unorderedEntries) },
  }),
] } });
assert.equal(unordered.body.error.code, "manifest_path_order_invalid");

const briefA = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, {
  token: tokenA,
  body: { trigger: "before_broad_edit", approximateTokenBudget: 800 },
})).body;
assert.equal(briefA.items.length, 1);
assert.equal(briefA.items[0].kind, "finding");
assert.equal(briefA.items[0].fidelity, "structural");
const findingId = briefA.items[0].id;
const itemA = (await request("GET", `/v1/context-items/${findingId}`, { token: tokenA })).body;
assert.equal(itemA.kind, "direct_collision");

const duplicateEvent = event({
  eventId: `evt_activity_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
  workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 6, type: "activity.reported",
  payload: { kind: "completion", summary: "Synthetic bounded activity." },
});
const firstActivity = (await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [duplicateEvent] } })).body;
const retriedActivity = (await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [duplicateEvent] } })).body;
assert.deepEqual(firstActivity.acceptedEventIds, retriedActivity.acceptedEventIds);

const outOfOrder = event({
  eventId: `evt_old_pause_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
  workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 1, type: "workspace.paused", payload: {},
});
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [outOfOrder] } });
const briefAfterNonMaterial = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, {
  token: tokenA, body: { trigger: "manual", approximateTokenBudget: 800 },
})).body;
assert.equal(briefAfterNonMaterial.contextRevision, briefA.contextRevision);

const pause = event({
  eventId: `evt_pause_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
  workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 10, type: "workspace.paused", payload: {},
});
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [pause] } });
const briefAfterPause = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, {
  token: tokenA, body: { trigger: "manual", approximateTokenBudget: 800 },
})).body;
assert.equal(briefAfterPause.contextRevision, briefA.contextRevision + 1);
const repeatedPause = event({
  eventId: `evt_pause_repeat_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
  workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 11, type: "workspace.paused", payload: {},
});
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [repeatedPause] } });
const briefAfterRepeatedPause = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, {
  token: tokenA, body: { trigger: "manual", approximateTokenBudget: 800 },
})).body;
assert.equal(briefAfterRepeatedPause.contextRevision, briefAfterPause.contextRevision);

const changes = (await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenA })).body;
assert.equal(changes.items.length, 1);
assert.equal(changes.items[0].id, findingId);

const crossProjectItem = await request("GET", `/v1/context-items/${findingId}`, { token: tokenC, expected: 403 });
assert.equal(crossProjectItem.body.error.code, "forbidden");
const crossProjectChanges = await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenC, expected: 403 });
assert.equal(crossProjectChanges.body.error.code, "forbidden");
const crossProjectEvent = event({
  eventId: `evt_cross_${suffix}`, projectId: projectA.id, deviceId: bootstrapC.deviceId,
  workspaceId: ids.workspaceA, sessionId: ids.sessionC, sequence: 2, type: "activity.reported",
  payload: { kind: "completion", summary: "Synthetic cross-project attempt." },
});
const crossPublish = await request("POST", "/v1/events/batch", {
  token: tokenC, body: { events: [crossProjectEvent] }, expected: 403,
});
assert.equal(crossPublish.body.error.code, "forbidden");

const oversizedBatch = await request("POST", "/v1/events/batch", {
  token: tokenA,
  body: { events: Array.from({ length: 101 }, (_, index) => ({ ...duplicateEvent, eventId: `evt_many_${index}_${suffix}` })) },
  expected: 400,
});
assert.equal(oversizedBatch.body.error.code, "batch_count_out_of_range");
const oversizedRequest = await request("POST", "/v1/events/batch", {
  token: tokenA,
  body: { padding: "x".repeat(300_000) },
  expected: 413,
});
assert.equal(oversizedRequest.body.error.code, "request_too_large");

let rateLimited = false;
for (let attempt = 0; attempt < 12; attempt++) {
  const response = await request("POST", "/v1/enrollments", {
    body: {
      inviteId: invite.id,
      inviteSecret: invite.secret,
      deviceLabel: "Synthetic Brute Force",
      appVersion: "fixture/1",
      schemaMinimum: 1,
      schemaMaximum: 1,
    },
    expected: attempt < 10 ? 409 : 429,
  });
  if (response.body.error.code === "rate_limited") {
    rateLimited = true;
    break;
  }
}
assert(rateLimited, "failed enrollment attempts must consume persistent edge quota");

await request("POST", `/v1/devices/${bootstrapB.deviceId}/revoke`, { token: tokenA, expected: 204 });
const revokedBootstrap = await request("GET", "/v1/device/bootstrap", { token: tokenB, expected: 401 });
assert.equal(revokedBootstrap.body.error.code, "credential_revoked");

console.log(JSON.stringify({
  level: "L2 hosted Project service",
  result: "PASS",
  deployment: "anonymous-local-loopback-redacted",
  assertions: {
    creatorAndInviteEnrollment: true,
    creatorDashboardTicketIssuance: true,
    twoDevicePublication: true,
    singleUseDashboardTicket: true,
    singleUseInvite: true,
    transactionalDedupe: true,
    outOfOrderAcceptedWithoutStaleProjection: true,
    atomicManifestAndDeterministicFinding: true,
    atomicManifest1000Paths: true,
    layeredManifestStatesPreserved: true,
    emptyManifestSnapshotActivated: true,
    staleManifestCompletionAcknowledgedWithoutRollback: true,
    nonCanonicalManifestPathOrderRejected: true,
    authorizedBriefAndItem: true,
    materialContextRevisionsOnly: true,
    crossProjectDenied: true,
    revokedDeviceDenied: true,
    batchSizeGuard: true,
    requestByteGuard: true,
    failedAttemptRateGuard: true,
    semanticProvidersAbsent: true
  },
  timings,
}, null, 2));
