# Stickguy — Architecture

Status: canonical  
Last updated: 2026-08-23

## 1. System context

```text
Agent harness ──stdio MCP/hooks──► stickguy executable ──local IPC──► per-user service
Git workspaces ─────────────────────────────────────────┤
                                                        │ HTTPS events
                                                        ▼
                                                Stickguy HTTP API
                                                  (Convex actions)
                                                        │
                                             Convex state/live queries
                                                        ▼
                                              React project dashboard
```

The hosted service never accesses a repository. Local clients publish bounded manifests and coordination summaries; the hosted service compares them across authorized project members and returns workstream-scoped briefs. The Go client never depends on Convex-specific APIs. Agents interact through MCP or explicit CLI commands. Stickguy coordinates existing agent harnesses; it does not own their coding loop or tools.

## 2. Local service

Exactly one service per OS user owns registered projects/workspaces, workspace observers, credentials, event queue, backend retries, CLI/MCP IPC, optional authenticated loopback MCP HTTP, pause state, and cleanup.

Use an OS lock plus IPC health check; a PID file alone is insufficient. Unix prefers a user-only domain socket; Windows prefers a named pipe. Loopback HTTP binds only `127.0.0.1`, validates Host/Origin, and requires a random bearer token.

`stickguy mcp` is a thin stdio client of this service. MCP exit never stops the service.

## 3. Workspace observation

Filesystem events are hints; Git is authoritative:

1. receive filesystem event;
2. debounce per workspace;
3. run bounded Git status/name queries plus baseline-to-`HEAD` name queries;
4. normalize repo-relative paths with `/` separators;
5. compare with last published path/status set;
6. emit a revisioned manifest snapshot only when changed; chunk large snapshots and activate them atomically.

Never upload contents. The baseline-to-current manifest preserves locally committed agent changes before push. Ignore `.git`, Stickguy state, caches, and configured ignores. Symlinks must not escape the registered root. Repository identity combines normalized remote identity with explicit project registration; folder-name equality is insufficient. Never fetch/pull peer worktrees as the realtime observation mechanism.

## 4. Hosted state

Convex holds materialized project state plus append-only activity. It is not fully event-sourced: a transaction may update a projection and insert activity.

| Table | Key fields |
|---|---|
| projects | publicId, name, status, createdAt, retentionPolicy |
| members | publicId, projectId, displayName, role, joinedAt, removedAt |
| devices | publicId, memberId, tokenHash, platform, lastSeenAt, revokedAt |
| invites | publicId, projectId, secretHash, expiresAt, remainingUses, revokedAt |
| browserSessions | publicId, memberId, secretHash, expiresAt, revokedAt |
| workspaces | publicId, deviceId, projectId, repoFingerprint, label, paused |
| repositoryScopes | projectId, repoFingerprint, contextRevision, updatedAt |
| sessions | publicId, workspaceId, source, startedAt, endedAt |
| workstreams | publicId, memberId, workspaceId, title, summary, status, updatedAt |
| changeManifests | publicId, workstreamId, revision, baselineRef, headRef, state, pathCount |
| changeManifestChunks | manifestId, chunkIndex, bounded path/symbol/dependency entries |
| semanticObjects | publicId, projectId, repoFingerprint, workstreamId, kind, text, metadata, source, fidelity, revision, active |
| semanticEmbeddings | objectId, scopeKey, kind, modelVersion, contentRevision, vector |
| activityEvents | eventId, projectId, type, actors, observedAt, payload, expiresAt |
| findings | publicId, projectId, repoFingerprint, kind, severity, confidenceBand, workstreamIds, evidence, state, fingerprint, engineVersion |
| verificationReports | publicId, workstreamId, manifestRevision, state, checkKind, label, summary, source, observedAt |
| contextDeliveries | publicId, workstreamId, contextRevision, trigger, itemRefs, routerVersion, requestedBudget, renderedSize, deliveredAt, acknowledgedAt |
| syncCards | publicId, projectId, findingId, state, severity, createdAt |
| decisions | publicId, projectId, syncCardId, summary, affectedMemberIds, revision |
| deviceCursors | deviceId, projectId, lastDecisionRevision, lastContextRevision, lastAckedSequence |

Public IDs are vendor-neutral opaque strings. Every public operation checks membership/role.

## 5. Delivery and presence

Events use at-least-once delivery:

- create stable event ID and monotonic sequence per workspace;
- commit to SQLite before send;
- deduplicate transactionally server-side;
- return per-event accepted/duplicate/rejected and acknowledgement;
- delete acknowledged payloads after a short recovery window;
- accept valid out-of-order events; projection rules consider sequence/observed time;
- retry with bounded exponential backoff and jitter;
- surface terminal auth/schema errors in `doctor`.

Presence is lossy and separate from durable events. Heartbeat about every 15 seconds; server receipt time is authoritative. Online means a valid heartbeat within 35 seconds. Paused is explicit, not offline.

## 6. Coordination intelligence

Alpha findings are deterministic: select active workstreams in the same project/repository identity, intersect normalized paths and other structural evidence, generate a stable fingerprint, and upsert without repeated notification.

V1 extends candidate retrieval with symbols, packages, schemas/routes, dependencies, lexical similarity, and semantic similarity across synchronized intent/change/plan/decision objects. A versioned evidence-fusion engine classifies findings; optional bounded adjudication handles ambiguous candidates. Similarity never overwrites deterministic evidence, crosses authorization boundaries, or appears as unexplained proof. See `coordination-intelligence.md`.

The initial semantic index is hosted so all members' approved coordination objects are comparable in realtime. Convex vector search is an adapter behind a Stickguy-owned interface. Use one opaque composite `scopeKey` derived from project ID and repository identity as the mandatory vector-index filter, then reauthorize and reload current objects after retrieval; never depend on post-filtering for tenant isolation. Embedding/adjudication failures are queued and do not stop deterministic detection.

## 7. Context routing and agent lifecycle

The context router builds a deterministic, workstream-scoped `CoordinationBrief` from current authorized state. It ranks direct findings, affected decisions, structural/dependency changes, and semantic candidates; respects a 128–800-token requested budget; omits unrelated team activity; and returns stable item IDs/revisions for follow-up.

Material project/repository coordination mutations increment `repositoryScopes.contextRevision`. Brief assembly reads this revision, retrieves candidates, reauthorizes/loads current records, and confirms the revision is unchanged before rendering; bounded retry prevents a mixed-state brief when a vector-search action races with writes.

Agent adapters integrate at begin, preflight, checkpoint, and finish boundaries. Delivery/acknowledgement is recorded with a monotonic context revision. A checkpoint may cite `basedOnBriefId`; only a material relevant change after that brief creates a stale-assumption finding. Stdio MCP cannot interrupt an active model turn, so urgent delivery reaches the dashboard/person immediately and the agent at its next supported hook/tool call. Under ADR-033, passive project-local Codex and Claude Code lifecycle hooks automatically project each supported session as its own repo-scoped workstream, including safe path evidence for same-checkout collision detection. Observation never approves, blocks, rewrites, starts, or steers agent work. Optional content-bearing observation remains governed by ADR-027 and `agent-activity-sharing.md` and is disabled in activity/v1.

See `coordination-harness.md` for boundaries, routing order, verification metadata, and degradation behavior.

## 8. Decisions

Backend is canonical. Agents read decisions through MCP. Unsupported agents may receive an enabled, generated, untracked `.stickguy/context.md`. Never have clients append automatically to one tracked decisions file; promotion to a repository ADR is separate and reviewed.

## 9. Failure behavior

- Backend unavailable: observe locally and queue bounded structured events.
- Credential revoked: stop sending and show actionable auth state.
- Git unavailable: workspace degrades; service stays healthy.
- Watch overflow: full Git rescan.
- MCP unavailable: Git/manual fidelity remains.
- Embedding/adjudication unavailable: deterministic detection remains live; semantic work queues with visible degraded fidelity.
- Context budget exceeded: return critical references and a truncation marker; never silently drop required context.
- Agent adapter unavailable: dashboard/Git/manual coordination remains with honest delivery fidelity.
- Corrupt DB: preserve diagnostic copy, recreate safely, retain keychain credential if valid.
- Schema mismatch: reject unsupported major versions with upgrade instruction.

## 10. Replaceability

Go speaks only `/v1` HTTP; OpenAPI/JSON Schema defines it; IDs are vendor-neutral; domain rules are separate from Convex wrappers; data is JSON-exportable; Git objects never depend on hosted storage; production exports are operational routine.
