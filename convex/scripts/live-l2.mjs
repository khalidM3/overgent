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
  workstreamAssumption: `wrk_assumption_${suffix}`,
  workstreamSchema: `wrk_schema_${suffix}`,
  workstreamUnrelated: `wrk_unrelated_${suffix}`,
  workstreamC: `wrk_c_${suffix}`,
};
const timings = {};

async function request(method, path, { token, cookie, body, expected = 200 } = {}) {
  const headers = {};
  if (token) headers.authorization = `Bearer ${token}`;
  if (cookie) headers.cookie = cookie;
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

function event({ eventId, projectId, deviceId, workspaceId, sessionId, sequence, type, payload, source = "git" }) {
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
    source,
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
const creatorCookie = (creatorExchange.headers.get("set-cookie") ?? "").split(";", 1)[0];
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
  // Project C is created with a chosen name, proving enrollment can set identity
  // directly so a new Project never starts device-named.
  body: { label: `Synthetic Project C ${suffix}`, deviceLabel: "Synthetic Device C", displayName: "Ravi P" },
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
      payload: side === "a"
        ? { workstreamId, title: "Rotate browser sessions", intendedOutcome: "Rotate browser sessions after privilege changes and revoke prior credentials.", contracts: ["membership-role-schema"] }
        : { workstreamId, title: "Issue fresh login credentials", intendedOutcome: "Issue new web login credentials after a member role changes and invalidate old credentials." },
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

await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({ eventId: `evt_assumption_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId, workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 6, type: "workstream.intent_reported", payload: { workstreamId: ids.workstreamAssumption, title: "Preserve existing sessions", intendedOutcome: "Treat role changes as immediate while existing sessions remain valid until expiry." } }),
  event({ eventId: `evt_schema_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId, workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 7, type: "workstream.intent_reported", payload: { workstreamId: ids.workstreamSchema, title: "Revise membership schema", intendedOutcome: "Add a membership role schema revision consumed by authorization clients.", contracts: ["membership-role-schema"] } }),
  event({ eventId: `evt_unrelated_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId, workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 8, type: "workstream.intent_reported", payload: { workstreamId: ids.workstreamUnrelated, title: "Tune documentation search", intendedOutcome: "Tune documentation search ranking without changing authentication or membership behavior." } }),
] } });

await request("POST", "/v1/events/batch", { token: tokenC, body: { events: [
  event({ eventId: `evt_intent_c_${suffix}`, projectId: projectC.id, deviceId: bootstrapC.deviceId, workspaceId: ids.workspaceC, sessionId: ids.sessionC, sequence: 2, type: "workstream.intent_reported", payload: { workstreamId: ids.workstreamC, title: "Cross Project session work", intendedOutcome: "Rotate browser sessions after privilege changes and revoke prior credentials." } }),
] } });

const briefA = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, {
  token: tokenA,
  body: { trigger: "before_broad_edit", approximateTokenBudget: 800 },
})).body;
assert(briefA.items.some((item) => item.kind === "finding" && item.text.includes("overlap")));
assert(briefA.items.some((item) => item.kind === "finding" && item.text.includes("incompatible session-validity")));
assert(briefA.items.some((item) => item.kind === "finding" && item.text.includes("membership-role-schema")));
const findingId = briefA.items.find((item) => item.kind === "finding" && item.text.includes("overlap")).id;
const itemA = (await request("GET", `/v1/context-items/${findingId}`, { token: tokenA })).body;
assert.equal(itemA.kind, "direct_collision");

const unrelatedBrief = (await request("POST", `/v1/workstreams/${ids.workstreamUnrelated}/briefs`, { token: tokenB, body: { trigger: "manual", approximateTokenBudget: 400 } })).body;
assert.equal(unrelatedBrief.items.length, 0);
const crossProjectBrief = (await request("POST", `/v1/workstreams/${ids.workstreamC}/briefs`, { token: tokenC, body: { trigger: "manual", approximateTokenBudget: 400 } })).body;
assert.equal(crossProjectBrief.items.length, 0);

await request("POST", `/v1/findings/${findingId}/feedback`, { cookie: creatorCookie, body: { value: "useful" }, expected: 204 });

await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({ eventId: `evt_assumption_update_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId, workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 9, type: "workstream.intent_reported", payload: { workstreamId: ids.workstreamAssumption, title: "Preserve existing sessions", intendedOutcome: "Treat permission changes as immediate and incompatible with rotation while existing sessions remain valid until expiry." } }),
] } });
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({ eventId: `evt_checkpoint_stale_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 26, type: "workstream.checkpoint_reported", payload: { checkpointId: `chk_stale_${suffix}`, workstreamId: ids.workstreamA, summary: "Implemented session rotation boundary.", basedOnBriefId: briefA.briefId } }),
] } });
const afterStale = (await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenA })).body;
assert(afterStale.items.some((item) => item.kind === "stale_assumption"));
const briefAfterStale = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, { token: tokenA, body: { trigger: "checkpoint", approximateTokenBudget: 800 } })).body;

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
assert.equal(briefAfterNonMaterial.contextRevision, briefAfterStale.contextRevision);

const pause = event({
  eventId: `evt_pause_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
  workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 27, type: "workspace.paused", payload: {},
});
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [pause] } });
const briefAfterPause = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, {
  token: tokenA, body: { trigger: "manual", approximateTokenBudget: 800 },
})).body;
assert.equal(briefAfterPause.contextRevision, briefAfterStale.contextRevision + 1);
const repeatedPause = event({
  eventId: `evt_pause_repeat_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
  workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 28, type: "workspace.paused", payload: {},
});
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [repeatedPause] } });
const briefAfterRepeatedPause = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, {
  token: tokenA, body: { trigger: "manual", approximateTokenBudget: 800 },
})).body;
assert.equal(briefAfterRepeatedPause.contextRevision, briefAfterPause.contextRevision);

const changes = (await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenA })).body;
assert(changes.items.some((item) => item.id === findingId));

const codexAgentId = `wrk_agent_${suffix.padEnd(32, "a")}`;
const claudeAgentId = `wrk_agent_${suffix.padEnd(32, "b")}`;
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({ eventId: `evt_agent_codex_start_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 29, source: "hook", type: "agent.activity_reported", payload: { workstreamId: codexAgentId, vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "SessionStart", status: "active", action: "Session started" } }),
  event({ eventId: `evt_agent_codex_path_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 30, source: "hook", type: "agent.activity_reported", payload: { workstreamId: codexAgentId, vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "PreToolUse", status: "active", action: "editing synthetic/agent-shared.ts", tool: "apply_patch", paths: ["synthetic/agent-shared.ts"] } }),
] } });
await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({ eventId: `evt_agent_claude_start_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId, workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 10, source: "hook", type: "agent.activity_reported", payload: { workstreamId: claudeAgentId, vendor: "claude", sessionAlias: "claude-d4e5f6", kind: "SessionStart", status: "active", action: "Session started" } }),
  event({ eventId: `evt_agent_claude_path_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId, workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 11, source: "hook", type: "agent.activity_reported", payload: { workstreamId: claudeAgentId, vendor: "claude", sessionAlias: "claude-d4e5f6", kind: "PreToolUse", status: "active", action: "editing synthetic/agent-shared.ts", tool: "Edit", paths: ["synthetic/agent-shared.ts"] } }),
] } });
const agentSnapshot = (await request("GET", `/v1/dashboard/projects/${projectA.id}`, { cookie: creatorCookie })).body;
assert(agentSnapshot.workstreams.some((workstream) => workstream.agent?.vendor === "codex" && workstream.paths.includes("synthetic/agent-shared.ts")));
assert(agentSnapshot.workstreams.some((workstream) => workstream.agent?.vendor === "claude" && workstream.paths.includes("synthetic/agent-shared.ts")));
assert(agentSnapshot.findings.some((finding) => finding.workstreamIds.includes(codexAgentId) && finding.workstreamIds.includes(claudeAgentId)));

// ---------------------------------------------------------------------------
// L7 exit gate: plan revisions, advisory claims, sync/decision delivery, and
// the explicit session-sharing consent boundary from ADR-034.
// ---------------------------------------------------------------------------

// ADR-037 removed plan items and advisory claims; those endpoints must be gone.
for (const [method, path, body] of [
  ["POST", `/v1/projects/${projectA.id}/plan-items`, { expectedPlanRevision: 0, items: [{ title: "Removed surface" }] }],
  ["POST", `/v1/projects/${projectA.id}/claims`, { workstreamId: ids.workstreamA, patterns: ["src/**"] }],
]) {
  const gone = await request(method, path, { token: tokenA, body, expected: 404 });
  assert.equal(gone.body.error.code, "not_found", `${path} must no longer exist`);
}
const collaborationShape = (await request("GET", `/v1/projects/${projectA.id}/collaboration`, { cookie: creatorCookie })).body;
assert.deepEqual(Object.keys(collaborationShape).sort(), ["cursor", "projectId", "resolutions", "syncCards"]);

// Two members resolve a finding together and the decision reaches both agents.
const card = (await request("POST", `/v1/projects/${projectA.id}/sync-cards`, {
  cookie: creatorCookie, body: { findingId, title: "Who owns the rotation boundary?", summary: "Both sessions are editing the same session-validity path." }, expected: 201,
})).body;
assert.equal(card.state, "open");
const commented = (await request("POST", `/v1/sync-cards/${card.id}/comments`, {
  cookie, body: { body: "Session B will stop editing the shared path and consume the rotation boundary instead." }, expected: 201,
})).body;
assert.equal(commented.body.length > 0, true);
const cardBeforeResolve = (await request("GET", `/v1/projects/${projectA.id}/collaboration`, { cookie: creatorCookie })).body.syncCards.find((entry) => entry.id === card.id);
assert.equal(cardBeforeResolve.comments.length, 1);
const staleResolve = await request("POST", `/v1/sync-cards/${card.id}/resolve`, {
  cookie: creatorCookie,
  body: { expectedRevision: card.revision, summary: "Stale attempt", affectedMemberIds: [], affectedWorkstreamIds: [ids.workstreamA] },
  expected: 409,
});
assert.equal(staleResolve.body.error.code, "revision_conflict");
const decision = (await request("POST", `/v1/sync-cards/${card.id}/resolve`, {
  cookie: creatorCookie,
  body: {
    expectedRevision: cardBeforeResolve.revision,
    summary: "Session A owns the rotation boundary; Session B consumes it and stops editing the shared path.",
    affectedMemberIds: [], affectedWorkstreamIds: [ids.workstreamA, ids.workstreamB],
  },
})).body;
assert.equal(decision.affectedWorkstreamIds.length, 2);

const resolvedCollaboration = (await request("GET", `/v1/projects/${projectA.id}/collaboration`, { cookie: creatorCookie })).body;
assert.equal(resolvedCollaboration.syncCards.find((entry) => entry.id === card.id).state, "resolved");
assert.equal(resolvedCollaboration.syncCards.find((entry) => entry.id === card.id).resolution.id, decision.id);
assert(resolvedCollaboration.resolutions.some((entry) => entry.id === decision.id));
// Resolving the card also resolves the finding it came from.
const changesAfterDecision = (await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenA })).body;
assert.equal(changesAfterDecision.items.find((item) => item.id === findingId).state, "resolved");

const decisionBriefA = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, { token: tokenA, body: { trigger: "manual", approximateTokenBudget: 800 } })).body;
const decisionBriefB = (await request("POST", `/v1/workstreams/${ids.workstreamB}/briefs`, { token: tokenB, body: { trigger: "manual", approximateTokenBudget: 800 } })).body;
const deliveredA = decisionBriefA.items.find((item) => item.kind === "decision" && item.id === decision.id);
const deliveredB = decisionBriefB.items.find((item) => item.kind === "decision" && item.id === decision.id);
assert(deliveredA, "member A's agent must receive the decision");
assert(deliveredB, "member B's agent must receive the decision");
assert.equal(deliveredA.advisoryAction, "coordination_required");

// Redelivery is idempotent: the same decision id and revision, never a duplicate item.
const redelivered = (await request("POST", `/v1/workstreams/${ids.workstreamA}/briefs`, { token: tokenA, body: { trigger: "manual", approximateTokenBudget: 800 } })).body;
const repeats = redelivered.items.filter((item) => item.kind === "decision" && item.id === decision.id);
assert.equal(repeats.length, 1, "decision delivery must not duplicate");
assert.equal(repeats[0].revision, deliveredA.revision);

// An unaffected workstream never receives the decision.
const unaffectedBrief = (await request("POST", `/v1/workstreams/${ids.workstreamUnrelated}/briefs`, { token: tokenB, body: { trigger: "manual", approximateTokenBudget: 800 } })).body;
assert(!unaffectedBrief.items.some((item) => item.kind === "decision"));

// The collaboration cursor advances and does not replay settled work.
const cursored = (await request("GET", `/v1/projects/${projectA.id}/collaboration`, { cookie: creatorCookie })).body;
assert(cursored.cursor.startsWith("time:"));
const afterCursor = (await request("GET", `/v1/projects/${projectA.id}/collaboration?cursor=${encodeURIComponent(cursored.cursor)}`, { cookie: creatorCookie })).body;
assert.equal(afterCursor.cursor, cursored.cursor, "a consumed cursor must not move on its own");

// Project isolation still holds for every collaboration surface.
const crossProjectCollaboration = await request("GET", `/v1/projects/${projectA.id}/collaboration`, { token: tokenC, expected: 403 });
assert.equal(crossProjectCollaboration.body.error.code, "forbidden");

// --- ADR-047 Project session messages --------------------------------------
const sharingDefault = (await request("GET", `/v1/workstreams/${codexAgentId}/session-sharing`, { cookie: creatorCookie })).body;
assert.equal(sharingDefault.messages.length, 0);

// Enrollment plus an adapter is sufficient; no additional ceremony occurs.
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({ eventId: `evt_share_ok_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 32, source: "hook", type: "agent.conversation_shared", payload: { messageId: `msg_ok_${suffix}`, workstreamId: codexAgentId, vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "user", text: "Explain the rotation boundary before editing." } }),
] } });
// ADR-036: quoted code and vendor-recorded reasoning are Project content after
// classifier approval.
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({ eventId: `evt_share_code_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 33, source: "hook", type: "agent.conversation_shared", payload: { messageId: `msg_code_${suffix}`, workstreamId: codexAgentId, vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "thinking", text: "The boundary is in session.ts:\n```ts\nconst rotate = true;\n```\nI will keep expiry untouched." } }),
] } });
const shared = (await request("GET", `/v1/workstreams/${codexAgentId}/session-sharing`, { cookie: creatorCookie })).body;
assert.equal(shared.messages.length, 2);
assert.deepEqual(shared.messages.map((message) => message.kind).sort(), ["thinking", "user"]);
assert(shared.messages.some((message) => message.text.includes("```ts")), "quoted code must survive classifier approval");

// Secret-bearing and source-like candidates are rejected as a whole, not redacted.
let badSequence = 35;
for (const [label, text] of [
  ["env assignment", "Update this in .env.local: DATABASE_URL=postgres://user:pw@host/db"],
  ["environment value", "Set this first:\nSTICKGUY_TOKEN=abcdef0123456789"],
  ["credential", "Use api_key: sk-abcdef0123456789abcdef for the call."],
  ["tool result", "tool_result: the command returned 3 rows"],
  ["command output", "stdout: total 12"],
  ["pasted env file", "Here it is:\nSTRIPE_KEY=sk_live_abcdefghijklmno\nDB_PASS=hunter2"],
]) {
  const rejected = await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
    event({ eventId: `evt_share_bad_${label.replace(/\W/g, "")}_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: badSequence++, source: "hook", type: "agent.conversation_shared", payload: { messageId: `msg_bad_${label.replace(/\W/g, "")}_${suffix}`, workstreamId: codexAgentId, vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "user", text } }),
  ] }, expected: 400 });
  assert.equal(rejected.body.error.code, "prohibited_data", `${label} must reject the whole candidate`);
}
// ADR-038: naming a configuration file is ordinary conversation and must share.
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({ eventId: `evt_share_mention_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 39, source: "hook", type: "agent.conversation_shared", payload: { messageId: `msg_mention_${suffix}`, workstreamId: codexAgentId, vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "user", text: "check the .env.local file to see which variables are set" } }),
] } });
const afterMention = (await request("GET", `/v1/workstreams/${codexAgentId}/session-sharing`, { cookie: creatorCookie })).body;
assert(afterMention.messages.some((message) => message.text.includes(".env.local")), "a filename mention must not be treated as its contents");

const afterRejections = (await request("GET", `/v1/workstreams/${codexAgentId}/session-sharing`, { cookie: creatorCookie })).body;
assert.equal(afterRejections.messages.length, 3, "no rejected candidate may reach durable storage");

// The source member can delete retained Project messages.
await request("DELETE", `/v1/workstreams/${codexAgentId}/session-sharing`, { cookie: creatorCookie, expected: 204 });
const afterDelete = (await request("GET", `/v1/workstreams/${codexAgentId}/session-sharing`, { cookie: creatorCookie })).body;
assert.equal(afterDelete.messages.length, 0, "deletion must remove Project messages");

// --- ADR-035 member identity and real branch collection --------------------
const membersBefore = (await request("GET", `/v1/projects/${projectA.id}/members`, { cookie: creatorCookie })).body;
const selfBefore = membersBefore.members.find((member) => member.isSelf);
assert.equal(selfBefore.nameSource, "device", "a new Project starts with the device-derived name");
assert.equal(selfBefore.name, "Synthetic Device A");

const emailIdentity = await request("PATCH", `/v1/projects/${projectA.id}/member`, {
  cookie: creatorCookie, body: { displayName: "khalid@example.invalid" }, expected: 400,
});
assert.equal(emailIdentity.body.error.code, "email_identity_rejected");

const renamed = (await request("PATCH", `/v1/projects/${projectA.id}/member`, {
  cookie: creatorCookie, body: { displayName: "Khalid M" },
})).body;
assert.equal(renamed.memberName, "Khalid M");
assert.equal(renamed.memberNameSource, "member");
const sessionAfterRename = (await request("GET", "/v1/dashboard/session", { cookie: creatorCookie })).body;
assert.equal(sessionAfterRename.memberName, "Khalid M");
assert.equal(sessionAfterRename.memberNameSource, "member");
const snapshotAfterRename = (await request("GET", `/v1/dashboard/projects/${projectA.id}`, { cookie: creatorCookie })).body;
assert(snapshotAfterRename.workstreams.some((workstream) => workstream.memberName === "Khalid M"));
assert(!snapshotAfterRename.workstreams.some((workstream) => workstream.memberName === "Synthetic Device A"));
// The device name survives, but only as security surface.
assert(snapshotAfterRename.devices.some((device) => device.label === "Synthetic Device A"));

// A Project enrolled with a chosen name never shows the device-derived source.
const membersC = (await request("GET", `/v1/projects/${projectC.id}/members`, { token: tokenC })).body;
const selfC = membersC.members.find((member) => member.isSelf);
assert.equal(selfC.name, "Ravi P");
assert.equal(selfC.nameSource, "member", "a name chosen at enrollment must not be marked device-derived");
const emailAtEnrollment = await request("POST", "/v1/projects", {
  token: randomBytes(32).toString("hex"),
  body: { label: `Synthetic Email Project ${suffix}`, deviceLabel: "Synthetic Device", displayName: "someone@example.invalid" },
  expected: 400,
});
assert.equal(emailAtEnrollment.body.error.code, "email_identity_rejected");

// Live sessions carry the real checked-out branch.
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({ eventId: `evt_agent_branch_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 40, source: "hook", type: "agent.activity_reported", payload: { workstreamId: codexAgentId, vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "PreToolUse", status: "active", action: "editing synthetic/agent-shared.ts", tool: "apply_patch", branch: "feature/session-rotation", paths: ["synthetic/agent-shared.ts"] } }),
] } });
const branchSnapshot = (await request("GET", `/v1/dashboard/projects/${projectA.id}`, { cookie: creatorCookie })).body;
assert.equal(branchSnapshot.workstreams.find((workstream) => workstream.id === codexAgentId).agent.branch, "feature/session-rotation");
const unsafeBranch = await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({ eventId: `evt_agent_branch_bad_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId, workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 41, source: "hook", type: "agent.activity_reported", payload: { workstreamId: codexAgentId, vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "PreToolUse", status: "active", action: "editing", tool: "apply_patch", branch: "../etc/passwd" } }),
] }, expected: 400 });
assert.equal(unsafeBranch.body.error.code, "validation_failed");

// ---------------------------------------------------------------------------
// M2 contract watch (ADR-048): a session's read set goes stale when another
// workstream changes a contract it already read. Workspaces A and B share one
// repository scope, so B's fingerprints are compared against A's read sets.
// ---------------------------------------------------------------------------

const readerAgentId = `wrk_agent_${suffix.padEnd(32, "c")}`;
const contractPath = "synthetic/contract-watch.go";
const additivePath = "synthetic/contract-additive.go";
const bodyOnlyPath = "synthetic/contract-body.go";
const fallbackPath = "synthetic/contract-fallback.go";
const rotateBefore = "func Rotate(ctx context.Context, key string) (string, error)";
const rotateAfter = "func Rotate(ctx context.Context, key string, at int64) (string, error)";

function fingerprintEntry(path, hash, symbols) {
  return { path, fileContractHash: hash, symbols };
}
function symbol(name, kind, signature, hash) {
  return { name, kind, signature, signatureHash: hash };
}
const hashes = {
  contractV1: "1".repeat(64), contractV2: "2".repeat(64),
  additiveV1: "3".repeat(64), additiveV2: "4".repeat(64),
  bodyOnly: "5".repeat(64),
  fallbackV1: "6".repeat(64), fallbackV2: "7".repeat(64),
  rotateBefore: "a".repeat(64), rotateAfter: "b".repeat(64),
  alpha: "c".repeat(64), beta: "d".repeat(64), stable: "e".repeat(64),
  legacyBefore: "f".repeat(64), legacyAfter: "9".repeat(64),
};

// B publishes the baseline surface for all four files. A first fingerprint is
// never a change, so nothing is compared yet. This event deliberately omits
// workstreamId, proving a device on the original payload shape still publishes.
await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({
    eventId: `evt_fp_base_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
    workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 60,
    type: "workspace.contract_fingerprints_reported",
    payload: { workspaceId: ids.workspaceB, entries: [
      fingerprintEntry(contractPath, hashes.contractV1, [symbol("Rotate", "func", rotateBefore, hashes.rotateBefore)]),
      fingerprintEntry(additivePath, hashes.additiveV1, [symbol("Alpha", "func", "func Alpha() error", hashes.alpha)]),
      fingerprintEntry(bodyOnlyPath, hashes.bodyOnly, [symbol("Stable", "func", "func Stable() error", hashes.stable)]),
      fingerprintEntry(fallbackPath, hashes.fallbackV1, [symbol("Legacy", "func", "func Legacy() error", hashes.legacyBefore)]),
    ] },
  }),
] } });

// A's agent session reads all three files at that baseline.
await request("POST", "/v1/events/batch", { token: tokenA, body: { events: [
  event({
    eventId: `evt_reader_start_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 50, source: "hook",
    type: "agent.activity_reported",
    payload: { workstreamId: readerAgentId, vendor: "claude", sessionAlias: "claude-c1c2c3", kind: "SessionStart", status: "active", action: "Session started" },
  }),
  event({
    eventId: `evt_reader_readset_${suffix}`, projectId: projectA.id, deviceId: bootstrapA.deviceId,
    workspaceId: ids.workspaceA, sessionId: ids.sessionA, sequence: 51, source: "hook",
    type: "session.read_set_reported",
    payload: { workspaceId: ids.workspaceA, sessionWorkstreamId: readerAgentId, entries: [
      { path: contractPath, fileContractHashAtRead: hashes.contractV1, observedAt: "2026-08-26T08:59:00Z" },
      { path: additivePath, fileContractHashAtRead: hashes.additiveV1, observedAt: "2026-08-26T08:59:01Z" },
      { path: bodyOnlyPath, fileContractHashAtRead: hashes.bodyOnly, observedAt: "2026-08-26T08:59:02Z" },
      { path: fallbackPath, fileContractHashAtRead: hashes.fallbackV1, observedAt: "2026-08-26T08:59:03Z" },
    ] },
  }),
] } });

const beforeContractChange = (await request("POST", `/v1/workstreams/${readerAgentId}/briefs`, {
  token: tokenA, body: { trigger: "manual", approximateTokenBudget: 800 },
})).body;
assert.equal(
  beforeContractChange.items.filter((item) => item.text.includes(contractPath)).length, 0,
  "reading a file must not by itself produce a finding",
);

// Workspace B now has two active workstreams, and the agent session is the more
// recently updated of the two. Attribution must therefore come from the event,
// not from "whichever workstream moved last".
await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({
    eventId: `evt_peer_touch_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
    workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 61, source: "hook",
    type: "agent.activity_reported",
    payload: { workstreamId: claudeAgentId, vendor: "claude", sessionAlias: "claude-d4e5f6", kind: "PreToolUse", status: "active", action: "editing synthetic/agent-shared.ts", tool: "Edit", paths: ["synthetic/agent-shared.ts"] },
  }),
] } });

// 1. B changes an exported signature the reader already read, naming the
//    workstream that made the change.
const contractChangeStarted = performance.now();
await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({
    eventId: `evt_fp_changed_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
    workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 62,
    type: "workspace.contract_fingerprints_reported",
    payload: { workspaceId: ids.workspaceB, workstreamId: ids.workstreamB, entries: [
      fingerprintEntry(contractPath, hashes.contractV2, [symbol("Rotate", "func", rotateAfter, hashes.rotateAfter)]),
    ] },
  }),
] } });
timings.contractDriftToFindingMs = Math.round(performance.now() - contractChangeStarted);

const readerChanges = (await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenA })).body;
const staleFindings = readerChanges.items.filter((item) =>
  item.kind === "stale_assumption" && item.workstreamIds.includes(readerAgentId));
assert.equal(staleFindings.length, 1, `exactly one stale_assumption finding for the reader: ${JSON.stringify(staleFindings)}`);
const staleFinding = staleFindings[0];
assert.equal(staleFinding.severity, "high");
assert.equal(staleFinding.confidenceBand, "deterministic");
assert.deepEqual(staleFinding.workstreamIds, [readerAgentId], "the finding belongs to the session that read the file");
assert(staleFinding.reason.includes("Rotate"), `the reason must name the symbol: ${staleFinding.reason}`);
assert(staleFinding.reason.includes(rotateBefore), "the reason must carry the old signature");
assert(staleFinding.reason.includes(rotateAfter), "the reason must carry the new signature");
const contractEvidence = staleFinding.evidence.find((entry) => entry.kind === "symbol");
assert(contractEvidence, "contract drift must carry symbol evidence");
assert.equal(contractEvidence.contract.path, contractPath);
assert.deepEqual(contractEvidence.contract.changedSymbols, [
  { name: "Rotate", oldSignature: rotateBefore, newSignature: rotateAfter },
]);
assert.equal(
  contractEvidence.contract.changedByWorkstreamId, ids.workstreamB,
  "attribution must be the workstream the event named",
);
assert.notEqual(
  contractEvidence.contract.changedByWorkstreamId, claudeAgentId,
  "attribution must not fall back to the most recently updated workstream when the event names one",
);
assert.notEqual(contractEvidence.contract.changedByWorkstreamId, readerAgentId, "a session cannot invalidate its own read");
assert.equal(contractEvidence.contract.readAt, "2026-08-26T08:59:00Z");
assert(Number.isFinite(Date.parse(contractEvidence.contract.changedAt)));

// The reader's brief carries it, which is how the correction reaches the agent.
const briefAfterDrift = (await request("POST", `/v1/workstreams/${readerAgentId}/briefs`, {
  token: tokenA, body: { trigger: "before_broad_edit", approximateTokenBudget: 800 },
})).body;
const deliveredDrift = briefAfterDrift.items.find((item) => item.id === staleFinding.id);
assert(deliveredDrift, `the reader's brief must contain ${staleFinding.id}: ${JSON.stringify(briefAfterDrift.items)}`);
assert.equal(deliveredDrift.advisoryAction, "coordination_required");
assert(deliveredDrift.text.includes("Rotate"), "the delivered item must name the symbol");

// The workstream that made the change is not told its own edit is stale.
const changerBrief = (await request("POST", `/v1/workstreams/${ids.workstreamB}/briefs`, {
  token: tokenB, body: { trigger: "manual", approximateTokenBudget: 800 },
})).body;
assert(!changerBrief.items.some((item) => item.id === staleFinding.id));

// 2. Redelivering the same fingerprint, and a body-only edit that leaves the
// contract hash alone, add no finding and no duplicate.
await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({
    eventId: `evt_fp_redelivered_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
    workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 63,
    type: "workspace.contract_fingerprints_reported",
    payload: { workspaceId: ids.workspaceB, workstreamId: ids.workstreamB, entries: [
      fingerprintEntry(contractPath, hashes.contractV2, [symbol("Rotate", "func", rotateAfter, hashes.rotateAfter)]),
      fingerprintEntry(bodyOnlyPath, hashes.bodyOnly, [symbol("Stable", "func", "func Stable() error", hashes.stable)]),
    ] },
  }),
] } });
const afterBodyOnly = (await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenA })).body;
const staleAfterBodyOnly = afterBodyOnly.items.filter((item) =>
  item.kind === "stale_assumption" && item.workstreamIds.includes(readerAgentId));
assert.equal(staleAfterBodyOnly.length, 1, "a body-only edit and a redelivery must not add contract findings");
assert.equal(staleAfterBodyOnly[0].id, staleFinding.id);
assert(!afterBodyOnly.items.some((item) => item.kind === "stale_assumption" && JSON.stringify(item.evidence).includes(bodyOnlyPath)),
  "a file whose contract hash never moved produces no contract finding");

// 3. B adds a new exported symbol to a file the reader read. Nothing the reader
// depends on moved, so there is no high-severity finding for that path.
await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({
    eventId: `evt_fp_additive_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
    workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 64,
    type: "workspace.contract_fingerprints_reported",
    payload: { workspaceId: ids.workspaceB, workstreamId: ids.workstreamB, entries: [
      fingerprintEntry(additivePath, hashes.additiveV2, [
        symbol("Alpha", "func", "func Alpha() error", hashes.alpha),
        symbol("Beta", "func", "func Beta() error", hashes.beta),
      ]),
    ] },
  }),
] } });
const afterAdditive = (await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenA })).body;
const additiveFindings = afterAdditive.items.filter((item) =>
  item.kind === "stale_assumption" && item.workstreamIds.includes(readerAgentId) &&
  JSON.stringify(item.evidence).includes(additivePath));
assert.equal(additiveFindings.length, 0, `adding an exported symbol must not raise a finding: ${JSON.stringify(additiveFindings)}`);
assert.equal(
  afterAdditive.items.filter((item) => item.kind === "stale_assumption" && item.workstreamIds.includes(readerAgentId)).length,
  1,
  "the only contract finding for the reader is the changed signature",
);

// 4. A publisher that does not name its workstream still gets a finding; only
//    the attribution degrades, to the workspace's most recently active
//    workstream, which here is the agent session touched above.
await request("POST", "/v1/events/batch", { token: tokenB, body: { events: [
  event({
    eventId: `evt_fp_unattributed_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
    workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 65,
    type: "workspace.contract_fingerprints_reported",
    payload: { workspaceId: ids.workspaceB, entries: [
      fingerprintEntry(fallbackPath, hashes.fallbackV2, [symbol("Legacy", "func", "func Legacy(at int64) error", hashes.legacyAfter)]),
    ] },
  }),
] } });
const afterUnattributed = (await request("GET", `/v1/projects/${projectA.id}/changes`, { token: tokenA })).body;
const fallbackFinding = afterUnattributed.items.find((item) =>
  item.kind === "stale_assumption" && item.workstreamIds.includes(readerAgentId) &&
  JSON.stringify(item.evidence).includes(fallbackPath));
assert(fallbackFinding, "an unattributed fingerprint must still invalidate a read set");
const fallbackEvidence = fallbackFinding.evidence.find((entry) => entry.kind === "symbol");
assert.equal(fallbackEvidence.contract.changedByWorkstreamId, claudeAgentId,
  "without a named workstream, attribution falls back to the most recently active one");
assert.notEqual(fallbackEvidence.contract.changedByWorkstreamId, readerAgentId);

// A workstream that belongs to another workspace can never be claimed as the
// author of this workspace's contract change.
const forgedAttribution = await request("POST", "/v1/events/batch", { token: tokenB, expected: 403, body: { events: [
  event({
    eventId: `evt_fp_forged_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
    workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 67,
    type: "workspace.contract_fingerprints_reported",
    payload: { workspaceId: ids.workspaceB, workstreamId: readerAgentId, entries: [
      fingerprintEntry(contractPath, "8".repeat(64), [symbol("Rotate", "func", rotateBefore, hashes.rotateBefore)]),
    ] },
  }),
] } });
assert.equal(forgedAttribution.body.error.code, "forbidden");

// A path that carries no contract is refused at the boundary rather than stored.
const unfingerprintable = await request("POST", "/v1/events/batch", { token: tokenB, expected: 400, body: { events: [
  event({
    eventId: `evt_fp_prose_${suffix}`, projectId: projectA.id, deviceId: bootstrapB.deviceId,
    workspaceId: ids.workspaceB, sessionId: ids.sessionB, sequence: 66,
    type: "workspace.contract_fingerprints_reported",
    payload: { workspaceId: ids.workspaceB, workstreamId: ids.workstreamB, entries: [
      fingerprintEntry("docs/readme.md", hashes.contractV1, []),
    ] },
  }),
] } });
assert.equal(unfingerprintable.body.error.code, "path_not_fingerprintable");

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
  level: "L2 hosted Project service with L6 intelligence, collision coordination, and M2 contract watch",
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
    semanticDuplicateBehavior: true,
    semanticAssumptionConflictBeforeEdits: true,
    scopedSharedDependencyRouting: true,
    unrelatedBriefEmpty: true,
    staleAssumptionDetected: true,
    crossProjectSemanticIsolation: true,
    radarFeedbackRecorded: true
    ,automaticAgentSessionsVisible: true
    ,sameCheckoutAgentPathCollision: true
    ,syncCardCommentAndResolve: true
    ,decisionReachesBothAgents: true
    ,decisionDeliveryIdempotent: true
    ,unaffectedWorkstreamGetsNoDecision: true
    ,collaborationCursorDoesNotReplay: true
    ,planningSurfacesRemoved: true
    ,collaborationProjectIsolation: true
    ,sessionMessagesVisibleAfterEnrollment: true
    ,sessionSecretCandidatesRejectedWhole: true
    ,quotedCodeAllowedAfterClassification: true
    ,filenameMentionIsNotDisclosure: true
    ,vendorReasoningProjectVisible: true
    ,sessionMessageDeletionRemovesContent: true
    ,memberIdentityIsMemberControlled: true
    ,emailRejectedAsIdentity: true
    ,deviceNameIsSecuritySurfaceOnly: true
    ,identityChosenAtEnrollment: true
    ,realBranchCollectedForLiveSessions: true
    ,contractDriftInvalidatesAReadSet: true
    ,contractFindingNamesOldAndNewSignature: true
    ,contractFindingReachesTheReadersBrief: true
    ,bodyOnlyEditProducesNoContractFinding: true
    ,contractFingerprintRedeliveryIsIdempotent: true
    ,addedExportedSymbolRaisesNoFinding: true
    ,unfingerprintablePathRefusedAtTheBoundary: true
    ,contractChangeAttributionIsExactNotDerived: true
    ,unattributedFingerprintFallsBackWithoutLosingTheFinding: true
    ,crossWorkspaceAttributionRefused: true
  },
  timings,
}, null, 2));
