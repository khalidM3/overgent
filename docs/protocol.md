# Overgent — Protocol Contracts

Status: build contract  
Last updated: 2026-08-26

Machine-readable contracts live in `protocol/openapi.yaml` and `protocol/schemas/`. This file defines semantics they must preserve.

## 1. Compatibility

- HTTP base `/v1`; every event carries `schemaVersion`.
- Additive optional fields are compatible; removal/renaming/meaning change requires major version.
- Client sends app version and supported schema range.
- Generated Go/TypeScript types are never hand-edited.

L8 administration adds Project-scoped access snapshots, one-use invite
creation/revocation, member removal, device revocation, member identity update,
owner/member-scoped JSON export, ordinary-member self-deletion, and owner
Project deletion. Browser-cookie and device-token callers both reauthorize
against current server-side membership; client-supplied roles are never
trusted. Revocation/removal takes effect on the next request, while data
deletion proceeds in bounded scheduled batches after authorization is revoked.

## 2. Event envelope

```json
{
  "schemaVersion": 1,
  "eventId": "uuidv7",
  "projectId": "prj_...",
  "memberId": "mem_...",
  "deviceId": "dev_...",
  "workspaceId": "wsp_...",
  "sessionId": "ses_...",
  "sequence": 42,
  "observedAt": "2026-08-23T18:30:00Z",
  "sentAt": "2026-08-23T18:30:02Z",
  "source": "git",
  "type": "workspace.manifest_completed",
  "payload": {}
}
```

`eventId` survives retries; sequence is per workspace; server stores receipt time separately; source is `git`, `manual`, `mcp`, `hook`, or versioned adapter. Payload is selected by type. Secrets, file contents, diffs, transcript files, system/developer prompts, hidden reasoning, environment values, and raw command/test output are forbidden. Classifier-approved visible user/assistant session messages use the explicit `agent.conversation_shared` event under ADR-036/ADR-047; the raw vendor record never crosses the wire.

## 3. Initial event types

| Type | Payload |
|---|---|
| `workspace.registered` | repoFingerprint, label, capabilities |
| `workspace.manifest_started` | manifestId, revision, workstreamId, baselineRef, headRef, chunkCount |
| `workspace.manifest_chunk` | manifestId, chunkIndex, bounded paths with independent baseline/index/worktree states plus optional symbol/dependency metadata |
| `workspace.manifest_completed` | manifestId, revision, contentHash |
| `workspace.paused` / `workspace.resumed` | optional reason / empty |
| `workstream.intent_reported` | title, intendedOutcome, approachSummary, components/contracts, optional `waitingOn`, anticipatedPaths, optional planItemIds |
| `workstream.checkpoint_reported` | checkpointId, summary/discoveries, verification summaries, relatedManifestRevision, basedOnBriefId |
| `workstream.status_changed` | active/idle/done/blocked |
| `context.acknowledged` | briefId, consideredItemIds |
| `activity.reported` | decision/completion/blocker and summary |
| `agent.activity_reported` | hashed session workstream, vendor, lifecycle/status, bounded action label, safe paths, optional tool/subagent metadata |
| `workspace.contract_fingerprints_reported` | workspace, optional publishing workstream, bounded entries of safe path, `fileContractHash`, and exported symbols (`name`, `kind`, normalized `signature`, `signatureHash`) |
| `session.read_set_reported` | workspace, hashed session workstream, bounded entries of safe path, `fileContractHashAtRead`, and observation time |
| `claim.created` / `claim.released` | patterns / claim IDs or patterns |

Manifest revisions represent complete current state, not unreliable filesystem deltas. The server stages chunks and exposes a revision only after completion/hash/count validation. `chunkCount: 0` followed directly by completion is the canonical empty snapshot and clears a prior active manifest without inventing an empty chunk. Entries are strictly ordered by normalized path and paths are unique. Each entry has independent optional `baseline`, `index`, and `worktree` change states so a path can retain simultaneous committed, staged, and unstaged evidence; each layer carries its own status and optional rename/copy source path. Entries contain bounded metadata, never content or patches.

The manifest content hash is SHA-256 over the ordered entry stream. For every entry, serialize these fields in order: `path`, then for each of `baseline`, `index`, and `worktree`, the literal layer name, its status or the empty string, and its old path or the empty string. Separate every field with one NUL byte and include the trailing NUL. The empty manifest hashes the empty byte string. This encoding is shared by Go and TypeScript and must not be inferred from ordinary JSON serialization.

A contract fingerprint is derived structural metadata, never source (ADR-044, ADR-048). Only `.go`, `.ts`, `.tsx`, `.py`, `.pyi`, `.js`, `.jsx`, `.mjs`, `.cjs`, `.java`, `.rs`, `.cs`, `.php`, `.c`, `.h`, `.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx`, `.scala`, `.sc`, `.kt`, `.kts`, and `.dart` paths are fingerprinted (ADR-063); every other path has no fingerprint and can never produce a contract finding. A symbol signature is the normalized declaration with the body and comments removed, bounded to 500 characters and marked when truncated, and `signatureHash` is SHA-256 over that normalized text. `fileContractHash` is SHA-256 over the sorted symbol stream, where each symbol contributes exactly `name`, `:`, `signatureHash`, and a newline; the empty exported surface hashes the empty byte string. A body-only edit leaves `fileContractHash` unchanged and therefore produces no contract evidence. The secret classifier drops a denied signature from both the symbol list and the hash, so the two never disagree.

A fingerprints event names the workstream that published it. That workstream is the change attribution once the service confirms it belongs to the publishing workspace; an event without it is attributed to the workspace's most recently active workstream instead.

A read set is per session workstream and deduplicated by (session, path): re-observing a path replaces its hash and time rather than appending. Finding evidence of kind `symbol` may carry the optional `contract` object naming the path, the changed symbols with their old and new signatures, the workstream that changed them, and the read and change times.

The event-envelope JSON Schema selects an exact, closed payload shape for every event type; language generators may represent conditional payloads generically, but producers and consumers must validate against the schema rather than extend them by hand.

## 4. Initial HTTP API

| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/projects` | Create project and enroll creator device. |
| POST | `/v1/projects/{id}/invites` | Create expiring invite. |
| POST | `/v1/enrollments` | Exchange invite for device credential/dashboard ticket. |
| POST | `/v1/dashboard-tickets` | Authenticated device mints a short-lived, single-use ticket for one authorized Project. |
| POST | `/v1/dashboard-tickets/exchange` | Exchange single-use browser ticket. |
| POST | `/v1/dashboard-activations` | Top-level form activation; exchange ticket, set browser cookie, and redirect without URL disclosure. |
| GET | `/v1/dashboard/session` | Read the current browser member and authorized Project list. |
| GET | `/v1/dashboard/projects/{id}` | Read a bounded, browser-authorized live Project snapshot. |
| GET | `/v1/device/bootstrap` | Memberships, workspaces, compatibility, cursors. |
| POST | `/v1/events/batch` | Validate/deduplicate bounded batch and acknowledge. |
| POST | `/v1/presence/heartbeat` | Update server-time presence. |
| GET | `/v1/projects/{id}/changes` | Compact findings/decisions/state changes since cursor. |
| POST | `/v1/workstreams/{id}/briefs` | Generate a revision-safe, authorized coordination brief for trigger/cursor/budget. |
| GET | `/v1/context-items/{id}` | Fetch one authorized current item referenced by a brief. |
| POST | `/v1/devices/{id}/revoke` | Revoke device. |

OpenAPI/server code explicitly limit bytes, batch count, string lengths, path counts, brief budgets, item fetches, and rates. Brief generation reauthorizes all loaded items and uses repository-scope context revisions to avoid mixed-state rendering.

## 5. Enrollment

- Invite: public ID plus at least 128 random secret bits; store hash only; expiry/use count/revocation.
- Device token: at least 256 random secret bits; store hash only; return once into OS credential store.
- Dashboard ticket: single-use, expires within minutes.
- Exchanged browser session: hashed, revocable, secure/HttpOnly/SameSite cookie; rotate on privilege change.
- Tokens never enter ordinary logs, analytics, URLs after exchange, or event payloads.

## 6. MCP surface

MCP resolves workspace from client working directory or explicit trusted config, then forwards locally.

| Tool | Behavior |
|---|---|
| `begin_work(idempotency_key, title, outcome, approach?, components?, contracts?, waiting_on?, anticipated_paths?, plan_item_ids?)` | Create/resume workstream, establish baseline, declare up to eight bounded dependency claims, and return initial `CoordinationBrief`. |
| `update_intent(workstream_id, revision, ..., waiting_on?)` | Revision-check and update active intent and declared dependency claims. |
| `check_coordination(workstream_id, trigger?, since_cursor?, approximate_token_budget?)` | Return a relevant `CoordinationBrief`; budget range 128–800, default 400. |
| `report_checkpoint(checkpoint_id, workstream_id, summary, discoveries?, affected_interfaces?, dependencies?, verification?, manifest_revision?, based_on_brief_id?)` | Publish idempotent progress/change/verification state and return newly relevant context. |
| `acknowledge_context(brief_id, considered_item_ids)` | Record delivery consideration without claiming compliance/correctness. |
| `finish_work(idempotency_key, workstream_id, outcome, summary, verification?, manifest_revision?, based_on_brief_id?)` | Close/handoff work and return unresolved items. |
| `report_event(kind, summary)` | Report decision/completion/blocker. |
| `get_resolutions(since_revision?)` | Read collision resolutions relevant to the workstream. |

Tools never mutate Git/worktrees or control the external agent loop. `CoordinationBrief` prioritizes directly relevant unresolved decisions/findings, then evidence/dependency changes and workstreams. It includes `briefId`, `contextRevision`, trigger, budget/size/truncation, stable item IDs/revisions/relevance reasons, and cursor. Findings carry kind, confidence band, severity, provenance, and lifecycle state; raw vector scores are not the user contract. Raw test output/commands are forbidden; verification is bounded structured metadata.

`VerificationSummary.observedAt` is optional when the reporting harness did not
provide a timestamp. Overgent does not substitute receipt time or the current
clock, because that would make an idempotent retry byte-different and would
present an invented observation time as evidence.

## 7. Capabilities and errors

Workspace capability example:

```json
{"git":true,"mcp":true,"hooks":false,"manualIntent":true,"agentAdapters":["codex"]}
```

Stable error body:

```json
{"error":{"code":"schema_version_unsupported","message":"Upgrade Overgent to continue.","requestId":"req_...","retryable":false,"details":{}}}
```

Codes are stable; messages may change. Never automatically retry authentication, authorization, validation, or incompatible-version errors.


### Goals a session moved on from

`ScopeSnapshot` carries `priorGoals`, oldest first, plus `priorGoalsDropped`.
A session is not one task: an objective is restated, an agent proposes something
adjacent, and work accumulates past the goal on record. With only a current
goal, `done` and `goal` drift apart until the finished work listed beside a goal
mostly does not belong to it.

A prior goal is appended when `workstream.intent_reported` moves the title or
intended outcome. A components, contracts, or `waitingOn` edit is a material
revision but not a new objective, and treating it as one would manufacture a
history the session never had.

This is durable state on the workstream, not a query over `activityEvents`.
Those rows carry `expiresAt` and are read newest-first under a bound, so a
derived history would lose a session's earliest goals as its events aged out —
a history that silently shortens is worse than none. The list is bounded and
`priorGoalsDropped` counts what fell off the front, so a truncated history is
never presented as a whole one.

## 8. Scope snapshots

Every dashboard workstream may carry a revisioned `ScopeSnapshot`. It is a
pull-only projection and has no route into a brief, hook response, agent turn,
or interruption channel. The projection renders exactly six fields: Goal, Now,
Done, Waiting on, Verification, and Scope, plus one honest state:
`implementing`, `verifying`, `waiting`, `idle`, or `complete`. It never invents
a percentage. `idle` is the state of a session between turns: `Stop` has
arrived and the member has not prompted again. It is not `complete`, because the
same session can be prompted again, and it is not `waiting`, which means blocked
on a person or a permission. Without it a finished turn kept reporting
`implementing` beside its own “Turn finished” line until the retention sweep
ended the session ten minutes later. A future reported step contract may render a count such as “2 of 5
reported steps”; no such count is inferred from paths, tools, or elapsed time.

Field sources follow strict precedence. Declared facts from
`workstream.intent_reported` win first; observed writes, contract fingerprints,
subagent events, and structured checkpoint verification are second; the
verbatim classifier-approved session `derivedTitle` is a low-evidence fallback
for Goal only. A field with no applicable fact says that it was not reported
rather than copying the title into a different meaning.

Every field carries its precedence class, exact canonical fact kinds, and
evidence quality. `high` means the fact is directly declared or directly
attributed to the workstream; `medium` means the observation is useful but its
session attribution is incomplete; `low` is the derived-title fallback; `none`
means no evidence exists. Codex hook observations remain at most `medium` until
the session-identity lane can bind declarations and checkpoints to the same
session workstream. Claude Code and Cursor may show `high` for vendor-observed
facts their adapters attribute directly. These distinctions must remain visible
where the field is rendered.

Rendering is deterministic from approved structured facts. A managed model may
improve wording only from those same facts and may not change meaning, state,
provenance, or evidence quality. Agent history and transcript content are never
ScopeSnapshot inputs, as required by `coordination-harness.md` section 8.
