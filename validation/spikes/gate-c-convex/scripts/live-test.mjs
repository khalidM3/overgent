import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { ConvexClient } from "convex/browser";
import { api } from "../convex/_generated/api.js";

const envText = readFileSync(new URL("../.env.local", import.meta.url), "utf8");
const urlMatch = envText.match(/^CONVEX_URL=(.+)$/m);
assert(urlMatch, "anonymous local deployment URL is missing");
const deploymentUrl = urlMatch[1].trim().replace(/^['"]|['"]$/g, "");
const parsedUrl = new URL(deploymentUrl);
assert(
  parsedUrl.hostname === "127.0.0.1" || parsedUrl.hostname === "localhost",
  "Gate C refuses non-loopback deployments",
);

const clientA = new ConvexClient(deploymentUrl);
const clientB = new ConvexClient(deploymentUrl);
const scopeA = {
  scopeKey: "scp_fixture_project_a_repo_a",
  projectId: "prj_fixture_a",
  repositoryId: "repo_fixture_a",
};
const scopeB = {
  scopeKey: "scp_fixture_project_b_repo_b",
  projectId: "prj_fixture_b",
  repositoryId: "repo_fixture_b",
};
const expiresLater = 4_102_444_800_000;
const timings = {};
const manifestMetrics = {
  pathCount: 1_000,
  chunkCount: 10,
  maxPathsPerChunk: 100,
  maxEncodedChunkArgumentBytes: 0,
};

function elapsed(startedAt) {
  return Math.round(performance.now() - startedAt);
}

async function waitUntil(predicate, label, timeoutMs = 5_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
  throw new Error(`timeout:${label}`);
}

async function rejectsCode(operation, expectedCode) {
  await assert.rejects(operation, (error) => {
    assert(String(error).includes(expectedCode), `expected ${expectedCode}`);
    return true;
  });
}

try {
  await clientA.mutation(api.gate.reset, {});
  await clientA.mutation(api.gate.ensureScope, scopeA);
  await clientB.mutation(api.gate.ensureScope, scopeB);

  // Both simulated devices hold a live subscription and each publishes state.
  let seenByA;
  let seenByB;
  let updatesA = 0;
  let updatesB = 0;
  const realtimeStarted = performance.now();
  const unsubscribeA = clientA.onUpdate(api.gate.scopeState, { scopeKey: scopeA.scopeKey }, (state) => {
    seenByA = state;
    updatesA++;
  });
  const unsubscribeB = clientB.onUpdate(api.gate.scopeState, { scopeKey: scopeA.scopeKey }, (state) => {
    seenByB = state;
    updatesB++;
  });
  await waitUntil(() => seenByA?.eventCount === 0 && seenByB?.eventCount === 0, "initial subscriptions");
  await clientA.mutation(api.gate.publishEvent, {
    eventId: "evt_realtime_a",
    scopeKey: scopeA.scopeKey,
    deviceId: "dev_a",
    sequence: 1,
  });
  await clientB.mutation(api.gate.publishEvent, {
    eventId: "evt_realtime_b",
    scopeKey: scopeA.scopeKey,
    deviceId: "dev_b",
    sequence: 1,
  });
  await waitUntil(() => seenByA?.eventCount === 2 && seenByB?.eventCount === 2, "cross-device realtime");
  timings.twoClientRealtimeMs = elapsed(realtimeStarted);
  assert(updatesA >= 2 && updatesB >= 2);
  unsubscribeA();
  unsubscribeB();

  // Concurrent delivery of one stable event ID produces one effect.
  const dedupeStarted = performance.now();
  const dispositions = await Promise.all([
    clientA.mutation(api.gate.publishEvent, {
      eventId: "evt_duplicate",
      scopeKey: scopeA.scopeKey,
      deviceId: "dev_a",
      sequence: 2,
    }),
    clientB.mutation(api.gate.publishEvent, {
      eventId: "evt_duplicate",
      scopeKey: scopeA.scopeKey,
      deviceId: "dev_b",
      sequence: 2,
    }),
  ]);
  assert.deepEqual(
    dispositions.map((result) => result.disposition).sort(),
    ["accepted", "duplicate"],
  );
  const afterDedupe = await clientA.query(api.gate.scopeState, { scopeKey: scopeA.scopeKey });
  assert.equal(afterDedupe.eventCount, 3);
  assert.equal(afterDedupe.contextRevision, 3);
  timings.concurrentDedupeMs = elapsed(dedupeStarted);

  // A 1,000-path snapshot remains invisible until ten bounded chunks activate atomically.
  const manifestStarted = performance.now();
  await clientA.mutation(api.gate.startManifest, {
    manifestId: "mft_1000",
    scopeKey: scopeA.scopeKey,
    revision: 1,
    expectedChunks: 10,
  });
  for (let chunkIndex = 0; chunkIndex < 9; chunkIndex++) {
    const chunkArgs = {
      manifestId: "mft_1000",
      chunkIndex,
      paths: Array.from(
        { length: 100 },
        (_, offset) => `synthetic/component-${String(chunkIndex * 100 + offset).padStart(4, "0")}.ts`,
      ),
    };
    manifestMetrics.maxEncodedChunkArgumentBytes = Math.max(
      manifestMetrics.maxEncodedChunkArgumentBytes,
      Buffer.byteLength(JSON.stringify(chunkArgs)),
    );
    await clientA.mutation(api.gate.addManifestChunk, chunkArgs);
  }
  assert.equal(await clientB.query(api.gate.activeManifest, { scopeKey: scopeA.scopeKey }), null);
  await rejectsCode(
    clientB.mutation(api.gate.completeManifest, { manifestId: "mft_1000" }),
    "manifest_incomplete",
  );
  const finalChunkArgs = {
    manifestId: "mft_1000",
    chunkIndex: 9,
    paths: Array.from(
      { length: 100 },
      (_, offset) => `synthetic/component-${String(900 + offset).padStart(4, "0")}.ts`,
    ),
  };
  manifestMetrics.maxEncodedChunkArgumentBytes = Math.max(
    manifestMetrics.maxEncodedChunkArgumentBytes,
    Buffer.byteLength(JSON.stringify(finalChunkArgs)),
  );
  await clientA.mutation(api.gate.addManifestChunk, finalChunkArgs);
  await rejectsCode(
    clientB.mutation(api.gate.addManifestChunk, {
      manifestId: "mft_1000",
      chunkIndex: 10,
      paths: ["synthetic/out-of-range.ts"],
    }),
    "chunk_index_out_of_range",
  );
  const activatedPathCount = await clientA.mutation(api.gate.completeManifest, {
    manifestId: "mft_1000",
  });
  assert.equal(activatedPathCount, 1_000);
  const activeManifest = await clientB.query(api.gate.activeManifest, { scopeKey: scopeA.scopeKey });
  assert.equal(activeManifest?.manifestId, "mft_1000");
  const afterManifest = await clientA.query(api.gate.scopeState, { scopeKey: scopeA.scopeKey });
  assert.equal(afterManifest.contextRevision, 4);
  await rejectsCode(
    clientA.mutation(api.gate.addManifestChunk, {
      manifestId: "mft_1000",
      chunkIndex: 0,
      paths: Array.from({ length: 101 }, (_, index) => `synthetic/oversized-${index}.ts`),
    }),
    "path_count_out_of_range",
  );
  timings.manifest1000Ms = elapsed(manifestStarted);

  // Scoped, fully current vector retrieval is immediately usable after writes.
  await clientA.mutation(api.gate.upsertSemantic, {
    publicId: "sem_alpha",
    ...scopeA,
    revision: 1,
    modelVersion: "fixture-v1",
    text: "synthetic authentication session intent",
    embedding: [1, 0, 0, 0, 0],
    expiresAt: expiresLater,
  });
  await clientA.mutation(api.gate.upsertSemantic, {
    publicId: "sem_decoy",
    ...scopeA,
    revision: 1,
    modelVersion: "fixture-v1",
    text: "synthetic dashboard colors intent",
    embedding: [0, 1, 0, 0, 0],
    expiresAt: expiresLater,
  });
  await clientB.mutation(api.gate.upsertSemantic, {
    publicId: "sem_other_project",
    ...scopeB,
    revision: 1,
    modelVersion: "fixture-v1",
    text: "synthetic authentication session intent in another project",
    embedding: [1, 0, 0, 0, 0],
    expiresAt: expiresLater,
  });
  const immediateStarted = performance.now();
  const immediate = await clientA.action(api.gate.semanticSearch, {
    scopeKey: scopeA.scopeKey,
    authorizedProjectId: scopeA.projectId,
    authorizedRepositoryId: scopeA.repositoryId,
    embedding: [1, 0, 0, 0, 0],
  });
  assert.equal(immediate.mode, "semantic");
  assert.equal(immediate.ids[0], "sem_alpha");
  assert(!immediate.ids.includes("sem_other_project"));
  timings.immediateVectorReadMs = elapsed(immediateStarted);

  const reauthorizationDenied = await clientA.action(api.gate.semanticSearch, {
    scopeKey: scopeA.scopeKey,
    authorizedProjectId: scopeB.projectId,
    authorizedRepositoryId: scopeB.repositoryId,
    embedding: [1, 0, 0, 0, 0],
  });
  assert.deepEqual(reauthorizationDenied.ids, []);

  // Supersession replaces both content revision and model-versioned vector.
  await clientA.mutation(api.gate.upsertSemantic, {
    publicId: "sem_alpha",
    ...scopeA,
    revision: 2,
    modelVersion: "fixture-v2",
    text: "synthetic dashboard intent after migration",
    embedding: [0, 1, 0, 0, 0],
    expiresAt: expiresLater,
  });
  const migrated = await clientA.query(api.gate.semanticState, { publicId: "sem_alpha" });
  assert.deepEqual(migrated, {
    revision: 2,
    modelVersion: "fixture-v2",
    vectorCount: 1,
    vectorModelVersions: ["fixture-v2"],
  });
  await rejectsCode(
    clientA.mutation(api.gate.upsertSemantic, {
      publicId: "sem_alpha",
      ...scopeA,
      revision: 2,
      modelVersion: "fixture-v2",
      text: "stale synthetic revision",
      embedding: [1, 0, 0, 0, 0],
      expiresAt: expiresLater,
    }),
    "stale_revision",
  );
  assert.equal(await clientA.mutation(api.gate.deleteSemantic, { publicId: "sem_alpha" }), true);
  assert.equal(await clientA.query(api.gate.semanticState, { publicId: "sem_alpha" }), null);
  const afterDelete = await clientA.action(api.gate.semanticSearch, {
    scopeKey: scopeA.scopeKey,
    authorizedProjectId: scopeA.projectId,
    authorizedRepositoryId: scopeA.repositoryId,
    embedding: [0, 1, 0, 0, 0],
  });
  assert(!afterDelete.ids.includes("sem_alpha"));

  // Every forced race changes the scoped revision, exhausting the bounded retries.
  const race = await clientA.action(api.gate.semanticSearch, {
    scopeKey: scopeA.scopeKey,
    authorizedProjectId: scopeA.projectId,
    authorizedRepositoryId: scopeA.repositoryId,
    embedding: [0, 1, 0, 0, 0],
    forceRace: true,
  });
  assert.deepEqual(race, { mode: "structural_fallback", ids: [], attempts: 2 });

  // Both time-based retention and explicit scope deletion remove required artifacts.
  const expiredScope = {
    scopeKey: "scp_fixture_expired",
    projectId: "prj_fixture_expired",
    repositoryId: "repo_fixture_expired",
  };
  await clientA.mutation(api.gate.ensureScope, expiredScope);
  await clientA.mutation(api.gate.upsertSemantic, {
    publicId: "sem_expired",
    ...expiredScope,
    revision: 1,
    modelVersion: "fixture-v1",
    text: "synthetic expired object",
    embedding: [0, 0, 1, 0, 0],
    expiresAt: 1,
  });
  await clientA.mutation(api.gate.seedRetentionArtifacts, {
    scopeKey: expiredScope.scopeKey,
    expiresAt: 1,
  });
  assert.deepEqual(await clientA.query(api.gate.scopeCounts, { scopeKey: expiredScope.scopeKey }), {
    contextDeliveries: 1,
    findings: 1,
    semanticEmbeddings: 1,
    semanticObjects: 1,
  });
  assert.equal(await clientA.mutation(api.gate.cleanupExpired, { now: 2 }), 4);
  assert.deepEqual(await clientA.query(api.gate.scopeCounts, { scopeKey: expiredScope.scopeKey }), {
    contextDeliveries: 0,
    findings: 0,
    semanticEmbeddings: 0,
    semanticObjects: 0,
  });

  await clientA.mutation(api.gate.seedRetentionArtifacts, {
    scopeKey: scopeB.scopeKey,
    expiresAt: expiresLater,
  });
  const beforeScopeDelete = await clientA.query(api.gate.scopeCounts, { scopeKey: scopeB.scopeKey });
  assert.deepEqual(beforeScopeDelete, {
    contextDeliveries: 1,
    findings: 1,
    semanticEmbeddings: 1,
    semanticObjects: 1,
  });
  assert.equal(await clientA.mutation(api.gate.deleteScope, { scopeKey: scopeB.scopeKey }), 4);
  assert.deepEqual(await clientA.query(api.gate.scopeCounts, { scopeKey: scopeB.scopeKey }), {
    contextDeliveries: 0,
    findings: 0,
    semanticEmbeddings: 0,
    semanticObjects: 0,
  });

  console.log(
    JSON.stringify(
      {
        gate: "L-1 Gate C",
        result: "PASS",
        deployment: "anonymous-local-loopback-redacted",
        convexVersion: "1.45.0",
        assertions: {
          twoClientRealtime: true,
          transactionalDedupe: true,
          atomicManifest1000Paths: true,
          monotonicScopeRevision: true,
          mandatoryVectorScope: true,
          postRetrievalReauthorization: true,
          immediateVectorRead: true,
          updateSupersessionDeletion: true,
          modelVersionMigration: true,
          boundedRaceFallback: true,
          retentionAndScopeDeletion: true,
          boundedChunkSize: true,
        },
        manifestMetrics,
        timings,
      },
      null,
      2,
    ),
  );
} finally {
  await Promise.allSettled([clientA.close(), clientB.close()]);
}
