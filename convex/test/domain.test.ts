import { describe, expect, it } from "vitest";
import {
  sessionHasGoneQuiet,
  SESSION_IDLE_TIMEOUT_MS,
  contractConfidenceBand,
  readCoverageOf,
  readFidelityOf,
  readFidelityRank,
  ValidationError,
  assertCanonicalManifestOrder,
  canActivateManifestRevision,
  RETENTION_TABLES,
  expiredRecordIds,
  findDependencySatisfaction,
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

    // ADR-063 widened the gate to the wasm-backed languages. Each of these is
    // a language the extractor can now produce, so the wire must store it.
    for (const path of [
      "backend/session.py", "backend/session.pyi", "frontend/uri.js",
      "frontend/view.jsx", "frontend/mod.mjs", "plugins/convert.cjs",
      "app/Session.java", "src/session.rs", "App/Session.cs", "src/Session.php",
      "src/session.c", "src/session.h", "src/session.cpp", "src/session.hpp",
      "src/Session.scala", "src/Session.kt", "lib/session.dart",
    ]) {
      expect(() => validateEventBatch({ events: [{
        ...baseEvent, type: "workspace.contract_fingerprints_reported",
        payload: { workspaceId: "wsp_fixture", entries: [{ ...entry, path }] },
      }] }), path).not.toThrow();
    }

    // The kinds only the tree-sitter extractor emits must survive the wire.
    for (const kind of ["reexport", "namespace"]) {
      expect(() => validateEventBatch({ events: [{
        ...baseEvent, type: "workspace.contract_fingerprints_reported",
        payload: { workspaceId: "wsp_fixture", entries: [{
          ...entry, path: "frontend/uri.js",
          symbols: [{ ...entry.symbols[0], kind }],
        }] },
      }] }), kind).not.toThrow();
    }

    for (const invalid of [
      { ...entry, path: "docs/readme.md" },
      // A language the extractor cannot produce is still a producer defect.
      // Ruby has no structural visibility marker, so it is deliberately not a
      // fingerprintable language (ADR-063).
      { ...entry, path: "backend/session.rb" },
      { ...entry, path: "backend/session.pyc" },
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
        entries: [{ path: "internal/session/rotate.go", fileContractHashAtRead: "c".repeat(64), observedAt: "2026-08-26T08:59:00Z", fidelity: "observed" }],
      },
    }] });
    expect(event.type).toBe("session.read_set_reported");
    for (const payload of [
      { workspaceId: "wsp_fixture", sessionWorkstreamId: "not-a-workstream", entries: [{ path: "a.go", fileContractHashAtRead: "c".repeat(64), observedAt: "2026-08-26T08:59:00Z", fidelity: "observed" }] },
      { workspaceId: "wsp_fixture", sessionWorkstreamId: "wrk_fixture", entries: [] },
      { workspaceId: "wsp_fixture", sessionWorkstreamId: "wrk_fixture", entries: [{ path: "a.go", fileContractHashAtRead: "c".repeat(64), fidelity: "observed" }] },
      { workspaceId: "wsp_fixture", sessionWorkstreamId: "wrk_fixture", entries: [{ path: "a.go", fileContractHashAtRead: "c".repeat(64), observedAt: "yesterday", fidelity: "observed" }] },
    ]) {
      expect(() => validateEventBatch({ events: [{ ...baseEvent, source: "hook", type: "session.read_set_reported", payload }] }),
        JSON.stringify(payload)).toThrowError(ValidationError);
    }
  });

  it("accepts optional bounded dependency claims and rejects over-limit claims", () => {
    const intent = (waitingOn: string[]) => ({
      ...baseEvent, source: "mcp", type: "workstream.intent_reported",
      payload: { workstreamId: "wrk_fixture", title: "Wait", intendedOutcome: "Continue when ready.", waitingOn },
    });
    expect(validateEventBatch({ events: [intent(["session-api"])] })[0].payload).toMatchObject({ waitingOn: ["session-api"] });
    expect(() => validateEventBatch({ events: [intent(Array.from({ length: 9 }, (_, index) => `claim-${index}`))] })).toThrow(ValidationError);
    expect(() => validateEventBatch({ events: [intent(["x".repeat(161)])] })).toThrow(ValidationError);
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

  // Regression for the ADR-052 gap: the Go device has emitted `readCoverage` on
  // every activity event since that ADR, and the wire schema and hosted
  // projection both carry it, but this validator's key allowlist did not — so
  // every hook activity event from a current device was rejected here as
  // validation_failed, for Claude and Codex as much as Cursor, and the local
  // flush loop discarded the reason.
  it("accepts the read-coverage a current device reports and rejects an unknown class", () => {
    const base = {
      ...baseEvent, source: "hook", type: "agent.activity_reported",
      payload: {
        workstreamId: "wrk_agent_0123456789abcdef0123456789abcdef",
        vendor: "cursor", sessionAlias: "cursor-a1b2c3", kind: "PreToolUse",
        status: "active", action: "inspecting files backend/refresh.go", tool: "read",
      },
    };
    for (const readCoverage of ["observed", "vendor_inferred", "self_declared", "none"]) {
      expect(validateEventBatch({ events: [{ ...base, payload: { ...base.payload, readCoverage } }] })).toHaveLength(1);
    }
    expect(() => validateEventBatch({ events: [{ ...base, payload: { ...base.payload, readCoverage: "assumed" } }] })).toThrow(ValidationError);
  });

  it("accepts Cursor as a supported vendor with its own alias shape", () => {
    const base = {
      ...baseEvent, source: "hook", type: "agent.activity_reported",
      payload: {
        workstreamId: "wrk_agent_0123456789abcdef0123456789abcdef",
        vendor: "cursor", sessionAlias: "cursor-a1b2c3", kind: "SessionStart",
        status: "active", action: "Session started", readCoverage: "observed",
      },
    };
    expect(validateEventBatch({ events: [base] })).toHaveLength(1);
    // The alias must name the vendor that sent it.
    expect(() => validateEventBatch({ events: [{ ...base, payload: { ...base.payload, sessionAlias: "cursed-a1b2c3" } }] })).toThrow(ValidationError);
    expect(() => validateEventBatch({ events: [{ ...base, payload: { ...base.payload, vendor: "windsurf" } }] })).toThrow(ValidationError);
  });

  // Regression for the other half of the same ADR-052 gap. `fidelity` is
  // required by the wire schema and read by the hosted projection, but was
  // absent from this entry's key list, so no read set could be stored — and
  // with no read sets, no stale_assumption finding could ever be raised.
  it("requires read-set fidelity so unverified reads are never stored at full confidence", () => {
    const base = {
      ...baseEvent, source: "hook", type: "session.read_set_reported",
      payload: {
        workspaceId: "wsp_0123456789abcdef0123456789abcdef",
        sessionWorkstreamId: "wrk_agent_0123456789abcdef0123456789abcdef",
        entries: [{
          path: "backend/refresh.go",
          fileContractHashAtRead: "a".repeat(64),
          observedAt: "2026-08-30T09:06:51.438Z",
          fidelity: "observed",
        }],
      },
    };
    expect(validateEventBatch({ events: [base] })).toHaveLength(1);
    for (const fidelity of ["vendor_inferred", "self_declared"]) {
      const entries = [{ ...base.payload.entries[0], fidelity }];
      expect(validateEventBatch({ events: [{ ...base, payload: { ...base.payload, entries } }] })).toHaveLength(1);
    }
    const { fidelity: _omitted, ...withoutFidelity } = base.payload.entries[0];
    expect(() => validateEventBatch({ events: [{ ...base, payload: { ...base.payload, entries: [withoutFidelity] } }] })).toThrow(ValidationError);
    expect(() => validateEventBatch({ events: [{ ...base, payload: { ...base.payload, entries: [{ ...base.payload.entries[0], fidelity: "guessed" }] } }] })).toThrow(ValidationError);
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
  it("matches dependency claims only to another live workstream in the same Project scope", () => {
    const candidate = {
      projectId: "prj_a", scopeKey: "scp_a", workstreamId: "wrk_producer", status: "active",
      path: "backend/session_api.go", symbols: ["SessionAPI"], latestCheckpointPassed: false,
    };
    expect(findDependencySatisfaction("prj_a", "scp_a", "wrk_consumer", "session-api", [candidate])).toMatchObject({
      claim: "session-api", satisfiedByWorkstreamId: "wrk_producer", state: "stable_wip",
      satisfiedBy: { path: "backend/session_api.go", symbols: ["SessionAPI"] },
    });
    expect(findDependencySatisfaction("prj_a", "scp_a", "wrk_consumer", "missing-api", [candidate])).toBeUndefined();
    expect(findDependencySatisfaction("prj_a", "scp_a", "wrk_producer", "session-api", [candidate])).toBeUndefined();
    expect(findDependencySatisfaction("prj_b", "scp_a", "wrk_consumer", "session-api", [candidate])).toBeUndefined();
    expect(findDependencySatisfaction("prj_a", "scp_b", "wrk_consumer", "session-api", [candidate])).toBeUndefined();
    expect(findDependencySatisfaction("prj_a", "scp_a", "wrk_consumer", "session-api", [{ ...candidate, latestCheckpointPassed: true }])?.state).toBe("ready");
  });

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

describe("read-set provenance", () => {
  // The whole point of ADR-052: a Codex session's reads are not watched, so a
  // finding built on them must not present itself as deterministic evidence.
  it("only an observed read produces a deterministic stale_assumption", () => {
    expect(contractConfidenceBand("observed")).toBe("deterministic");
    expect(contractConfidenceBand("vendor_inferred")).toBe("high");
    expect(contractConfidenceBand("self_declared")).toBe("medium");
  });

  it("ranks sources so the strongest evidence for a path wins", () => {
    expect(readFidelityRank("observed")).toBeGreaterThan(readFidelityRank("vendor_inferred"));
    expect(readFidelityRank("vendor_inferred")).toBeGreaterThan(readFidelityRank("self_declared"));
    expect(readFidelityRank("self_declared")).toBeGreaterThan(readFidelityRank(undefined));
  });

  it("treats a row written before provenance existed as the observed hook path", () => {
    expect(readFidelityOf(undefined)).toBe("observed");
    expect(readFidelityOf("nonsense")).toBe("observed");
    expect(readFidelityOf("vendor_inferred")).toBe("vendor_inferred");
  });

  it("ignores an unrecognized coverage rather than inventing one", () => {
    expect(readCoverageOf("none")).toBe("none");
    expect(readCoverageOf("self_declared")).toBe("self_declared");
    expect(readCoverageOf("bogus")).toBeUndefined();
  });
});

// Nothing ages a session out on its own: Stop reports idle, SessionEnd reports
// done, and a session whose SessionEnd never arrives stays live forever while
// the engine counts everything not done as live.
describe("quiet session expiry", () => {
  const now = 1_800_000_000_000;
  const session = (over: Partial<{ status: string; vendor?: string; updatedAt: number }> = {}) =>
    ({ status: "idle", vendor: "codex", updatedAt: now - SESSION_IDLE_TIMEOUT_MS - 1, ...over });

  it("ends an agent session that stopped reporting", () => {
    expect(sessionHasGoneQuiet(session(), now)).toBe(true);
    expect(sessionHasGoneQuiet(session({ status: "active" }), now)).toBe(true);
    expect(sessionHasGoneQuiet(session({ status: "blocked" }), now)).toBe(true);
  });

  it("leaves a session that is merely between turns alone", () => {
    expect(sessionHasGoneQuiet(session({ updatedAt: now - 60_000 }), now)).toBe(false);
    expect(sessionHasGoneQuiet(session({ status: "active", updatedAt: now - SESSION_IDLE_TIMEOUT_MS + 1 }), now)).toBe(false);
  });

  // B31: a headless `claude -p` run exits after `Stop` and never sends
  // `SessionEnd`, so it sat live for the full thirty minutes and paired into
  // findings against unrelated later work. A session that reported idle emits
  // nothing while genuinely between turns, so a much shorter quiet window is
  // honest for it - and revival on the next prompt is automatic either way. A
  // mid-turn (active) session keeps the long window: its hooks fire
  // continuously, so silence there needs more benefit of the doubt.
  it("ends an idle session on the short quiet window", () => {
    expect(sessionHasGoneQuiet(session({ updatedAt: now - 12 * 60_000 }), now)).toBe(true);
    expect(sessionHasGoneQuiet(session({ status: "active", updatedAt: now - 12 * 60_000 }), now)).toBe(false);
    expect(sessionHasGoneQuiet(session({ updatedAt: now - 9 * 60_000 }), now)).toBe(false);
  });

  // Completing these on the member's behalf would be a claim Stickguy cannot
  // support: they have no turn loop, so silence says nothing about them.
  it("never completes a workstream that is not an agent session", () => {
    expect(sessionHasGoneQuiet(session({ vendor: undefined, updatedAt: 0 }), now)).toBe(false);
  });

  it("leaves an already finished session untouched", () => {
    expect(sessionHasGoneQuiet(session({ status: "done", updatedAt: 0 }), now)).toBe(false);
  });
});
