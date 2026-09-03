import { afterEach, describe, expect, it, vi } from "vitest";

// Convex functions use Convex's bundler resolution and are intentionally not
// part of this package's NodeNext typecheck. A variable dynamic import lets
// Vitest exercise their handlers without silently changing that boundary.
const intelligenceModulePath = "../functions/intelligence.js";
const serviceModulePath = "../functions/service.js";

afterEach(() => {
  vi.useRealTimers();
  delete process.env.ANTHROPIC_API_KEY;
});

describe("semantic vector search", () => {
  it("never returns an embedding from a foreign model in the same scope", async () => {
    const { searchSemantic } = await import(intelligenceModulePath) as Record<string, unknown>;
    const rows = [
      { _id: "emb_query_model", scopeKey: "scope_a", scopeModelKey: "7:scope_aopenai/text-embedding-3-large/1024", modelVersion: "openai/text-embedding-3-large/1024", _score: 0.99 },
      { _id: "emb_foreign_model", scopeKey: "scope_a", scopeModelKey: "7:scope_aother-provider/embedding-v2", modelVersion: "other-provider/embedding-v2", _score: 0.98 },
    ];
    let queryCount = 0;
    const ctx = {
      runQuery: async (_reference: unknown, args: { embeddingIds?: string[] }) => {
        queryCount++;
        if (queryCount === 1) {
          return {
            available: true,
            scopeKey: "scope_a",
            contextRevision: 7,
            vector: Array.from({ length: 1024 }, () => 0),
            modelVersion: "openai/text-embedding-3-large/1024",
          };
        }
        return {
          retry: false,
          objectIds: (args.embeddingIds ?? []).map((id) => id === "emb_foreign_model" ? "obj_foreign" : "obj_compatible"),
        };
      },
      vectorSearch: async (_table: string, _index: string, options: {
        filter: (q: { eq: (field: string, value: string) => unknown }) => unknown;
      }) => {
        const filters = new Map<string, string>();
        const q = {
          eq(field: string, value: string) {
            filters.set(field, value);
            return q;
          },
        };
        options.filter(q);
        return rows.filter((row) => [...filters].every(([field, value]) => row[field as keyof typeof row] === value));
      },
      runMutation: async () => undefined,
    };

    const handler = (searchSemantic as unknown as { _handler: (ctx: unknown, args: unknown) => Promise<{ objectIds: string[] }> })._handler;
    const result = await handler(ctx, { tokenHash: "token", workstreamPublicId: "ws_query", limit: 10 });

    expect(result.objectIds).toEqual(["obj_compatible"]);
  });
});

describe("judgment degradation", () => {
  it("records quota with the rate-window recovery time when the project budget is exhausted", async () => {
    const { adjudicateFinding } = await import(intelligenceModulePath) as Record<string, unknown>;
    const now = 1_800_000_000_000;
    vi.useFakeTimers();
    vi.setSystemTime(now);
    process.env.ANTHROPIC_API_KEY = "fixture-only";
    const mutationArgs: Array<Record<string, unknown>> = [];
    const ctx = {
      runQuery: async () => ({ scopeKey: "scope_a", projectId: "project_a", severity: "medium", delivery: "dashboard" }),
      runMutation: async (_reference: unknown, args: Record<string, unknown>) => {
        mutationArgs.push(args);
        if ("projectId" in args) return { claimed: false, recoversAt: now + 3_600_000 };
        return false;
      },
    };
    const candidate = {
      kind: "redundant_work",
      severity: "medium",
      confidence: "medium",
      reason: "Two workstreams may implement the same behavior.",
      signalKind: "semantic",
      sharedSignals: ["credential"],
      trackedContractSymbols: [],
      structurallyUnambiguous: false,
      workstreams: [
        { id: "wrk_a", title: "Rotate credentials", summary: "Rotate browser credentials", status: "active", reportedChange: true, verification: "unknown" },
        { id: "wrk_b", title: "Revoke sessions", summary: "Revoke login sessions", status: "active", reportedChange: true, verification: "unknown" },
      ],
    };

    const handler = (adjudicateFinding as unknown as { _handler: (ctx: unknown, args: unknown) => Promise<unknown> })._handler;
    await handler(ctx, { findingPublicId: "fnd_fixture", expectedRevision: 1, candidate });

    expect(mutationArgs).toContainEqual(expect.objectContaining({
      scopeKey: "scope_a",
      reason: "quota",
      recoversAt: now + 3_600_000,
    }));
  });
});

describe("contract change attribution", () => {
  it("attributes a changed path to the agent session that authored it instead of the workspace workstream", async () => {
    const { attributeContractChange, upsertContractFindings } = await import(serviceModulePath) as {
      attributeContractChange: (...args: unknown[]) => Promise<string>;
      upsertContractFindings: (...args: unknown[]) => Promise<void>;
    };
    const workspaceWorkstream = {
      _id: "workspace_workstream_doc",
      publicId: "wrk_local_bbd91f9c4f36af9fe0db67e39a7fa4c4",
      projectId: "project_a",
      workspaceId: "workspace_a",
      scopeKey: "scope_a",
      status: "active",
      updatedAt: 10,
    };
    const authoringAgent = {
      _id: "agent_workstream_doc",
      publicId: "wrk_agent_7932e24a_fixture",
      projectId: "project_a",
      workspaceId: "workspace_a",
      scopeKey: "scope_a",
      vendor: "claude",
      safePaths: ["backend/revoke.go"],
      lastWritePaths: ["backend/revoke.go"],
      lastWriteAt: 20,
      status: "active",
      updatedAt: 20,
    };
    const inserted: unknown[] = [];
    const reader = {
      workstreamPublicId: authoringAgent.publicId,
      workstreamId: authoringAgent._id,
      fileContractHashAtRead: "old_hash",
      fidelity: "self_declared",
      observedAt: "2026-09-01T00:00:00.000Z",
    };
    const ctx = {
      db: {
        query: (table: string) => ({
          withIndex: (_index: string, configure: (q: { eq: (field: string, value: unknown) => unknown }) => unknown) => {
            const equalities = new Map<string, unknown>();
            const q = { eq: (field: string, value: unknown) => { equalities.set(field, value); return q; } };
            configure(q);
            return {
              unique: async () => equalities.get("publicId") === authoringAgent.publicId ? authoringAgent : workspaceWorkstream,
              collect: async () => [workspaceWorkstream, authoringAgent],
              take: async () => table === "sessionReadSets" ? [reader] : [],
            };
          },
        }),
        insert: async (_table: string, value: unknown) => { inserted.push(value); },
      },
    };

    const changedBy = await attributeContractChange(
      ctx as never,
      { _id: "project_a" } as never,
      { _id: "workspace_a", publicId: "wsp_a", scopeKey: "scope_a" } as never,
      workspaceWorkstream.publicId,
      "backend/revoke.go",
    );

    expect(changedBy).toBe(authoringAgent.publicId);
    const fallbackChangedBy = await attributeContractChange(
      ctx as never,
      { _id: "project_a" } as never,
      { _id: "workspace_a", publicId: "wsp_a", scopeKey: "scope_a" } as never,
      workspaceWorkstream.publicId,
      "backend/unclaimed.go",
    );
    expect(fallbackChangedBy).toBe(workspaceWorkstream.publicId);
    await upsertContractFindings(
      ctx as never,
      { _id: "project_a" } as never,
      { _id: "workspace_a", publicId: "wsp_a", scopeKey: "scope_a" } as never,
      {
        path: "backend/revoke.go",
        previousSymbols: [{ name: "AuditEntry", kind: "type", signature: "type AuditEntry struct", signatureHash: "old_signature" }],
        nextSymbols: [{ name: "AuditEntry", kind: "type", signature: "type AuditEntry struct { Category string }", signatureHash: "new_signature" }],
        nextHash: "new_hash",
        changedByWorkstreamPublicId: changedBy,
        now: 1_800_000_000_000,
      },
    );
    expect(inserted).toEqual([]);
  });
});

// B36: the provider migration deleted every row from the other population. An
// env blip (the OPENAI_API_KEY unset/restore cycle is routine here) then wiped
// real embeddings, and nothing re-embeds an object whose content never changes
// again - recall silently degraded to whatever happened to be edited since.
// Convergence must re-embed in place, never delete a live object's only row.
describe("embedding model convergence", () => {
  it("re-embeds a foreign-model row in place instead of deleting it", async () => {
    const { convergeForeignEmbeddings } = await import(intelligenceModulePath) as Record<string, unknown>;
    const fallbackModel = "overgent-concepts/v1/1024";
    const scope = { _id: "scope_doc", scopeKey: "scope_a", semanticModelVersion: fallbackModel };
    const object = { _id: "obj_doc", publicId: "sem_1", scopeKey: "scope_a", text: "rotate credentials for the project", revision: 3, active: true, expiresAt: 9_999_999_999_999 };
    const foreignRow = { _id: "emb_doc", objectId: "obj_doc", scopeKey: "scope_a", modelVersion: "text-embedding-3-large/1024", contentRevision: 2, vector: [0.5, 0.5] };
    const deleted: unknown[] = [];
    const patched: Array<{ id: unknown; value: Record<string, unknown> }> = [];
    const ctx = {
      db: {
        query: () => ({
          withIndex: () => ({
            unique: async () => scope,
            paginate: async () => ({ page: [foreignRow], isDone: true, continueCursor: null }),
          }),
        }),
        get: async () => object,
        delete: async (id: unknown) => { deleted.push(id); },
        patch: async (id: unknown, value: Record<string, unknown>) => { patched.push({ id, value }); },
      },
      scheduler: { runAfter: async () => undefined },
    };

    const handler = (convergeForeignEmbeddings as unknown as { _handler: (ctx: unknown, args: unknown) => Promise<unknown> })._handler;
    await handler(ctx, { scopeKey: "scope_a", modelVersion: fallbackModel });

    expect(deleted).toEqual([]);
    const converged = patched.find((entry) => entry.id === "emb_doc");
    expect(converged?.value.modelVersion).toBe(fallbackModel);
    expect((converged?.value.vector as number[]).length).toBe(1024);
    expect(converged?.value.contentRevision).toBe(3);
  });
});

// B37: the fourth degradation site. searchSemantic's vector-search failure
// path recorded degraded without a reason, leaving a scope in exactly the
// state the status banner cannot render.
describe("semantic search degradation", () => {
  it("records a reason when the vector search itself fails", async () => {
    const { searchSemantic } = await import(intelligenceModulePath) as Record<string, unknown>;
    const mutationArgs: Array<Record<string, unknown>> = [];
    const ctx = {
      runQuery: async () => ({ available: true, scopeKey: "scope_a", contextRevision: 1, vector: [0], modelVersion: "text-embedding-3-large/1024" }),
      vectorSearch: async () => { throw new Error("vector index unavailable"); },
      runMutation: async (_reference: unknown, args: Record<string, unknown>) => { mutationArgs.push(args); },
    };

    const handler = (searchSemantic as unknown as { _handler: (ctx: unknown, args: unknown) => Promise<unknown> })._handler;
    await handler(ctx, { tokenHash: "token", workstreamPublicId: "ws_query", limit: 5 });

    expect(mutationArgs).toContainEqual(expect.objectContaining({ degraded: true, reason: "provider_error" }));
  });
});

// B26: path granularity is file-level, and two agents editing different single
// lines of one file drew direct_collision at high/next_turn - the "trains
// people to ignore it" mode. Bare same-file overlap is a quiet structural
// notice; interruption needs corroboration, and the deterministic evidence for
// it is the file's contract moving while both sessions were live.
describe("agent path collision severity", () => {
  const baseWorkstream = {
    _id: "ws_self", publicId: "wrk_agent_self", vendor: "claude", status: "active",
    scopeKey: "scope_a", startedAt: 1_000, updatedAt: 10_000, safePaths: ["shared/settings.ts"],
  };
  const peer = {
    _id: "ws_peer", publicId: "wrk_agent_peer", vendor: "codex", status: "active",
    scopeKey: "scope_a", startedAt: 2_000, updatedAt: 9_000, safePaths: ["shared/settings.ts"],
  };
  const makeCtx = (contractRow: Record<string, unknown> | null) => {
    const inserted: Array<Record<string, unknown>> = [];
    const ctx = {
      db: {
        query: (table: string) => ({
          withIndex: () => ({
            collect: async () => table === "workstreams" ? [baseWorkstream, peer] : [],
            unique: async () => table === "contractFingerprints" ? contractRow : null,
          }),
        }),
        insert: async (_table: string, value: Record<string, unknown>) => { inserted.push(value); },
        patch: async () => undefined,
      },
    };
    return { ctx, inserted };
  };

  it("keeps a bare same-file overlap quiet", async () => {
    const { upsertAgentPathFindings } = await import(serviceModulePath) as {
      upsertAgentPathFindings: (...args: unknown[]) => Promise<void>;
    };
    const { ctx, inserted } = makeCtx(null);
    await upsertAgentPathFindings(ctx as never, { _id: "project_a" } as never, baseWorkstream as never, ["shared/settings.ts"], 10_000);
    expect(inserted.length).toBe(1);
    expect(inserted[0]?.severity).toBe("medium");
    expect(inserted[0]?.delivery).not.toBe("next_turn");
  });

  it("escalates when the file's contract moved while both sessions were live", async () => {
    const { upsertAgentPathFindings } = await import(serviceModulePath) as {
      upsertAgentPathFindings: (...args: unknown[]) => Promise<void>;
    };
    const { ctx, inserted } = makeCtx({ path: "shared/settings.ts", scopeKey: "scope_a", revision: 2, updatedAt: 5_000 });
    await upsertAgentPathFindings(ctx as never, { _id: "project_a" } as never, baseWorkstream as never, ["shared/settings.ts"], 10_000);
    expect(inserted.length).toBe(1);
    expect(inserted[0]?.severity).toBe("high");
    expect(inserted[0]?.delivery).toBe("next_turn");
  });
});

// B29: a member without a supported adapter emitted no hooks, and the
// workspace workstream carried no path evidence at all, so their edits could
// never pair into collision detection - active but invisible. Manifest paths
// no live agent session claims are that member's work evidence, and an agent
// touching one of them deserves a finding at honestly reduced fidelity.
describe("adapterless member collision", () => {
  it("pairs an agent session against the workspace workstream's residual paths", async () => {
    const { upsertAgentPathFindings } = await import(serviceModulePath) as {
      upsertAgentPathFindings: (...args: unknown[]) => Promise<void>;
    };
    const agent = {
      _id: "ws_agent", publicId: "wrk_agent_self", vendor: "claude", status: "active",
      scopeKey: "scope_a", startedAt: 1_000, updatedAt: 10_000, safePaths: ["backend/audit.go"],
    };
    const workspaceWorkstream = {
      _id: "ws_workspace", publicId: "wrk_local_member", status: "active",
      scopeKey: "scope_a", updatedAt: 9_000, residualPaths: ["backend/audit.go"], residualAt: 9_000,
    };
    const inserted: Array<Record<string, unknown>> = [];
    const ctx = {
      db: {
        query: (table: string) => ({
          withIndex: () => ({
            collect: async () => table === "workstreams" ? [agent, workspaceWorkstream] : [],
            unique: async () => null,
          }),
        }),
        insert: async (_table: string, value: Record<string, unknown>) => { inserted.push(value); },
        patch: async () => undefined,
      },
    };
    await upsertAgentPathFindings(ctx as never, { _id: "project_a" } as never, agent as never, ["backend/audit.go"], 10_000);
    expect(inserted.length).toBe(1);
    expect(inserted[0]?.severity).toBe("medium");
    expect(inserted[0]?.confidenceBand).not.toBe("deterministic");
    expect(inserted[0]?.delivery).not.toBe("next_turn");
    expect(JSON.stringify(inserted[0]?.evidence)).toContain("adapter");
  });

  it("computes residual paths as manifest paths no live agent claims", async () => {
    const { residualManifestPaths } = await import(serviceModulePath) as {
      residualManifestPaths: (paths: readonly string[], claimed: ReadonlySet<string>) => string[];
    };
    const residual = residualManifestPaths(
      ["backend/audit.go", "shared/settings.ts"],
      new Set(["shared/settings.ts"]),
    );
    expect(residual).toEqual(["backend/audit.go"]);
  });
});
