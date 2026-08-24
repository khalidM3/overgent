import { describe, expect, it } from "vitest";
import {
  ValidationError,
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

  it("rejects unknown and prohibited payload fields", () => {
    expect(() => validateEventBatch({
      events: [{ ...baseEvent, type: "workspace.resumed", payload: { sourceContent: "synthetic" } }],
    })).toThrowError(ValidationError);
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
          entries: Array.from({ length: 101 }, (_, index) => ({ path: `synthetic/${index}`, status: "added" })),
        },
      }],
    })).toThrow("path_count_out_of_range");
  });
});

describe("hosted deterministic helpers", () => {
  it("implements SHA-256 and the Gate B manifest hash wire format", () => {
    expect(sha256Hex("abc")).toBe("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
    expect(manifestContentHash([{ path: "a.ts", status: "modified" }])).toBe(
      "550c7216ed3b1e5a9022e342749e18347cc6108ff653f363e7440c9d1768cd05",
    );
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
      "activityEvents", "findings", "contextDeliveries", "semanticObjects", "semanticEmbeddings",
    ]));
  });
});
