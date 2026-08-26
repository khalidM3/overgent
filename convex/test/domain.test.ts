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
  validateContractSignature,
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

  it("accepts bounded contract fingerprints and refuses unfingerprintable or protected paths", () => {
    const entry = {
      path: "internal/session/rotate.go",
      fileContractHash: "a".repeat(64),
      symbols: [{
        name: "Rotate", kind: "func",
        signature: "func Rotate(ctx context.Context, key string) (string, error)",
        signatureHash: "b".repeat(64),
      }],
    };
    const [event] = validateEventBatch({ events: [{
      ...baseEvent, type: "workspace.contract_fingerprints_reported",
      payload: { workspaceId: "wsp_fixture", entries: [entry] },
    }] });
    expect(event.type).toBe("workspace.contract_fingerprints_reported");

    // Naming the publishing workstream is accepted, and stays optional so a
    // device on the original shape keeps publishing.
    const [attributed] = validateEventBatch({ events: [{
      ...baseEvent, type: "workspace.contract_fingerprints_reported",
      payload: { workspaceId: "wsp_fixture", workstreamId: "wrk_fixture", entries: [entry] },
    }] });
    expect(attributed.payload).toMatchObject({ workstreamId: "wrk_fixture" });
    expect(() => validateEventBatch({ events: [{
      ...baseEvent, type: "workspace.contract_fingerprints_reported",
      payload: { workspaceId: "wsp_fixture", workstreamId: "not-a-workstream", entries: [entry] },
    }] })).toThrowError(ValidationError);

    // A file with no exported surface is a valid, empty fingerprint.
    expect(() => validateEventBatch({ events: [{
      ...baseEvent, type: "workspace.contract_fingerprints_reported",
      payload: { workspaceId: "wsp_fixture", entries: [{ ...entry, symbols: [] }] },
    }] })).not.toThrow();

    for (const invalid of [
      { ...entry, path: "docs/readme.md" },
      { ...entry, path: "config/secrets/keys.ts" },
      { ...entry, path: "../outside/escape.go" },
      { ...entry, fileContractHash: "not-a-hash" },
      { ...entry, symbols: [{ ...entry.symbols[0], kind: "macro" }] },
      { ...entry, symbols: [{ ...entry.symbols[0], signature: "const Token = \"ghp_aaaaaaaaaaaaaaaaaaaa\"" }] },
      { ...entry, symbols: [{ ...entry.symbols[0], signature: "func Read() string", extra: 1 }] },
    ]) {
      expect(() => validateEventBatch({ events: [{
        ...baseEvent, type: "workspace.contract_fingerprints_reported",
        payload: { workspaceId: "wsp_fixture", entries: [invalid] },
      }] }), JSON.stringify(invalid)).toThrowError(ValidationError);
    }
  });

  it("keeps ordinary constant declarations off the credential gate", () => {
    expect(() => validateContractSignature("const MAX_RETRIES = 3")).not.toThrow();
    expect(() => validateContractSignature("export const LIMIT: number")).not.toThrow();
    expect(() => validateContractSignature("const Token = \"ghp_aaaaaaaaaaaaaaaaaaaa\"")).toThrowError(ValidationError);
  });

  it("accepts a bounded session read set and refuses a malformed one", () => {
    const [event] = validateEventBatch({ events: [{
      ...baseEvent, source: "hook", type: "session.read_set_reported",
      payload: {
        workspaceId: "wsp_fixture",
        sessionWorkstreamId: "wrk_agent_0123456789abcdef0123456789abcdef",
        entries: [{ path: "internal/session/rotate.go", fileContractHashAtRead: "c".repeat(64), observedAt: "2026-08-26T08:59:00Z" }],
      },
    }] });
    expect(event.type).toBe("session.read_set_reported");
    for (const payload of [
      { workspaceId: "wsp_fixture", sessionWorkstreamId: "not-a-workstream", entries: [{ path: "a.go", fileContractHashAtRead: "c".repeat(64), observedAt: "2026-08-26T08:59:00Z" }] },
      { workspaceId: "wsp_fixture", sessionWorkstreamId: "wrk_fixture", entries: [] },
      { workspaceId: "wsp_fixture", sessionWorkstreamId: "wrk_fixture", entries: [{ path: "a.go", fileContractHashAtRead: "c".repeat(64) }] },
      { workspaceId: "wsp_fixture", sessionWorkstreamId: "wrk_fixture", entries: [{ path: "a.go", fileContractHashAtRead: "c".repeat(64), observedAt: "yesterday" }] },
    ]) {
      expect(() => validateEventBatch({ events: [{ ...baseEvent, source: "hook", type: "session.read_set_reported", payload }] }),
        JSON.stringify(payload)).toThrowError(ValidationError);
    }
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

  it("accepts a real branch name and rejects unsafe branch metadata", () => {
    const base = {
      ...baseEvent, source: "hook", type: "agent.activity_reported",
      payload: {
        workstreamId: "wrk_agent_0123456789abcdef0123456789abcdef",
        vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "PreToolUse",
        status: "active", action: "editing src/nav.tsx",
      },
    };
    for (const branch of ["main", "feature/session-rotation", "release/1.2.3"]) {
      expect(validateEventBatch({ events: [{ ...base, payload: { ...base.payload, branch } }] })).toHaveLength(1);
    }
    for (const branch of ["", "-delete", "has space", "a..b", "ref@{0}", "topic.lock", "star*", "back\\slash", "a".repeat(256)]) {
      expect(() => validateEventBatch({ events: [{ ...base, payload: { ...base.payload, branch } }] })).toThrow(ValidationError);
    }
  });

  it("accepts Project conversation content but rejects secrets and raw output", () => {
    const base = {
      ...baseEvent, source: "hook", type: "agent.conversation_shared",
      payload: {
        messageId: "msg_0123456789abcdef0123456789abcdef", workstreamId: "wrk_agent_0123456789abcdef0123456789abcdef",
        vendor: "codex", sessionAlias: "codex-a1b2c3", kind: "user", text: "Explain the rotation boundary.",
      },
    };
    expect(validateEventBatch({ events: [base] })).toHaveLength(1);
    // ADR-036: quoted code and diffs belong in a Project conversation.
    for (const text of ["```ts\nconst x = 1;\n```", "diff --git a/a.ts b/a.ts", "Call sessionRotation() in src/auth.ts"]) {
      expect(validateEventBatch({ events: [{ ...base, payload: { ...base.payload, text } }] })).toHaveLength(1);
    }
    // Vendor-recorded reasoning and surfaced instructions are supported kinds.
    for (const kind of ["thinking", "system"]) {
      expect(validateEventBatch({ events: [{ ...base, payload: { ...base.payload, kind } }] })).toHaveLength(1);
    }
    // Tool steps are activity metadata, and hooks never supplied a summary kind.
    for (const kind of ["reasoning_summary", "tool"]) {
      expect(() => validateEventBatch({ events: [{ ...base, payload: { ...base.payload, kind } }] })).toThrow(ValidationError);
    }
    // ADR-038: a passing mention of a credential file is ordinary conversation.
    for (const text of ["read .env.local to see which variables are set", "check the .env file first", "Compare with MAX_RETRIES == 5 first"]) {
      expect(validateEventBatch({ events: [{ ...base, payload: { ...base.payload, text } }] })).toHaveLength(1);
    }
    for (const text of [
      "API_KEY=super-secret", "export DATABASE_URL=postgres://x",
      "Update this in .env.local: DATABASE_URL=postgres://user:pw@host/db",
      "Here is the file:\nSTRIPE_KEY=sk_live_abcdefghijklmno\nDB_PASS=hunter2",
      "Bearer abcdefghijklmnopqrstuvwxyz", "-----BEGIN RSA PRIVATE KEY-----", "tool_result: 42 rows", "stdout: total 12",
    ]) {
      expect(() => validateEventBatch({ events: [{ ...base, payload: { ...base.payload, text } }] })).toThrow(ValidationError);
    }
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
