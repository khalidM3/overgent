import { describe, expect, it } from "vitest";
import {
  ValidationError,
  assertCanonicalManifestOrder,
  canActivateManifestRevision,
  RETENTION_TABLES,
  expiredRecordIds,
  manifestContentHash,
  scopeKey,
  sha256Hex,
  validateEventBatch,
} from "../src/domain.js";

const baseEvent = {
  schemaVersion: 1,
  eventId: "evt_fixture",
  projectId: "prj_fixture",
  memberId: "mem_fixture",
  deviceId: "dev_fixture",
  workspaceId: "wsp_fixture",
  sessionId: "ses_fixture",
  sequence: 1,
  observedAt: "2026-08-23T18:30:00Z",
  sentAt: "2026-08-23T18:30:02Z",
  source: "git",
};

describe("hosted boundary validation", () => {
  it("accepts a frozen-contract manifest completion event", () => {
    const [event] = validateEventBatch({
      events: [{
        ...baseEvent,
        type: "workspace.manifest_completed",
        payload: { manifestId: "mft_fixture", revision: 1, contentHash: "a".repeat(64) },
      }],
    });
    expect(event.type).toBe("workspace.manifest_completed");
  });

  it("accepts an empty manifest snapshot that clears active paths", () => {
    const [event] = validateEventBatch({ events: [{
      ...baseEvent,
      type: "workspace.manifest_started",
      payload: {
        manifestId: "mft_empty", revision: 2, workstreamId: "wrk_fixture",
        baselineRef: "a".repeat(40), headRef: "b".repeat(40), chunkCount: 0,
      },
    }] });
    expect(event.payload).toMatchObject({ chunkCount: 0 });
    expect(manifestContentHash([])).toBe(sha256Hex(""));
  });

  it("rejects unknown and prohibited payload fields", () => {
    expect(() => validateEventBatch({
      events: [{ ...baseEvent, type: "workspace.resumed", payload: { sourceContent: "synthetic" } }],
    })).toThrowError(ValidationError);
  });

  it("accepts bounded agent activity and rejects raw or protected candidates", () => {
    const valid = {
      ...baseEvent, source: "hook", type: "agent.activity_reported",
      payload: {
        workstreamId: "wrk_agent_0123456789abcdef0123456789abcdef",
        vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "PreToolUse",
        status: "active", action: "editing src/nav.tsx", tool: "apply_patch", paths: ["src/nav.tsx"],
      },
    };
    expect(validateEventBatch({ events: [valid] })).toHaveLength(1);
    expect(() => validateEventBatch({ events: [{ ...valid, payload: { ...valid.payload, prompt: "raw" } }] })).toThrow(ValidationError);
    expect(() => validateEventBatch({ events: [{ ...valid, payload: { ...valid.payload, paths: [".env.local"] } }] })).toThrow(ValidationError);
  });

  it("rejects oversized batches and manifest chunks", () => {
    expect(() => validateEventBatch({ events: [] })).toThrow("batch_count_out_of_range");
    expect(() => validateEventBatch({
      events: [{
        ...baseEvent,
        type: "workspace.manifest_chunk",
        payload: {
          manifestId: "mft_fixture",
          chunkIndex: 0,
          entries: Array.from({ length: 101 }, (_, index) => ({ path: `synthetic/${index}`, states: { worktree: { status: "added" } } })),
        },
      }],
    })).toThrow("path_count_out_of_range");
  });

  it("preserves layered manifest states and rejects ambiguous changes", () => {
    const manifestEvent = (entries: unknown[]) => ({
      ...baseEvent,
      type: "workspace.manifest_chunk",
      payload: { manifestId: "mft_fixture", chunkIndex: 0, entries },
    });
    expect(validateEventBatch({ events: [manifestEvent([{
      path: "synthetic/layered.ts",
      states: {
        baseline: { status: "modified" },
        index: { status: "renamed", oldPath: "synthetic/old.ts" },
        worktree: { status: "copied", oldPath: "synthetic/source.ts" },
      },
    }])] })).toHaveLength(1);
    expect(() => validateEventBatch({ events: [manifestEvent([{ path: "a.ts", states: {} }])] }))
      .toThrow("manifest_states_empty");
    expect(() => validateEventBatch({ events: [manifestEvent([{
      path: "a.ts", states: { worktree: { status: "modified", oldPath: "old.ts" } },
    }])] })).toThrow("old_path_status_invalid");
  });

  it("rejects mixed-workspace batches so the acknowledgement cursor is unambiguous", () => {
    expect(() => validateEventBatch({ events: [
      { ...baseEvent, type: "workspace.resumed", payload: {} },
      { ...baseEvent, eventId: "evt_second", workspaceId: "wsp_other", sequence: 2, type: "workspace.resumed", payload: {} },
    ] })).toThrow("mixed_event_batch");
  });
});

describe("hosted deterministic helpers", () => {
  it("implements SHA-256 and canonical layered manifest hashing", () => {
    expect(sha256Hex("abc")).toBe("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
    const baseline = manifestContentHash([{ path: "a.ts", states: { baseline: { status: "modified" } } }]);
    expect(baseline).toHaveLength(64);
    expect(baseline).not.toBe(manifestContentHash([{ path: "a.ts", states: { worktree: { status: "modified" } } }]));
    expect(baseline).toBe(manifestContentHash([{ path: "a.ts", states: { baseline: { status: "modified" } } }]));
  });

  it("matches the fixed layered manifest hash vector", () => {
    expect(manifestContentHash([
      { path: "a.ts", states: {
        baseline: { status: "modified" },
        index: { status: "renamed", oldPath: "old-a.ts" },
        worktree: { status: "modified" },
      } },
      { path: "z.ts", states: { worktree: { status: "untracked" } } },
    ])).toBe("cb3fc754d48edb8d7be868df86d249942d8832811e0af83fb2f24f022328ea4d");
  });

  it("requires strictly increasing unique paths at completion", () => {
    const entry = (path: string) => ({ path, states: { worktree: { status: "modified" as const } } });
    expect(() => assertCanonicalManifestOrder([entry("b.ts"), entry("a.ts")])).toThrow("manifest_path_order_invalid");
    expect(() => assertCanonicalManifestOrder([entry("a.ts"), entry("a.ts")])).toThrow("manifest_path_order_invalid");
    expect(() => assertCanonicalManifestOrder([entry("a.ts"), entry("b.ts")])).not.toThrow();
  });

  it("allows only monotonically newer manifest activation", () => {
    expect(canActivateManifestRevision(undefined, 1)).toBe(true);
    expect(canActivateManifestRevision(1, 2)).toBe(true);
    expect(canActivateManifestRevision(2, 2)).toBe(false);
    expect(canActivateManifestRevision(3, 2)).toBe(false);
  });

  it("derives stable project/repository scopes", () => {
    expect(scopeKey("prj_a", "repo_a")).toBe(scopeKey("prj_a", "repo_a"));
    expect(scopeKey("prj_a", "repo_a")).not.toBe(scopeKey("prj_b", "repo_a"));
  });

  it("selects only expired non-durable rows in bounded order", () => {
    expect(expiredRecordIds([
      { id: "durable", expiresAt: 1, durable: true },
      { id: "later", expiresAt: 3 },
      { id: "first", expiresAt: 1 },
      { id: "future", expiresAt: 10 },
    ], 5, 2)).toEqual(["first", "later"]);
    expect(RETENTION_TABLES.indexOf("changeManifestChunks")).toBeLessThan(RETENTION_TABLES.indexOf("changeManifests"));
    expect(RETENTION_TABLES.indexOf("semanticEmbeddings")).toBeLessThan(RETENTION_TABLES.indexOf("semanticObjects"));
    expect(RETENTION_TABLES).toEqual(expect.arrayContaining([
      "activityEvents", "findings", "findingFeedback", "contextDeliveries", "semanticObjects", "semanticEmbeddings",
    ]));
  });
});
