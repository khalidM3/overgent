# Stickguy — Protocol Contracts

Status: build contract  
Last updated: 2026-08-23

Machine-readable contracts live in `protocol/openapi.yaml` and `protocol/schemas/`. This file defines semantics they must preserve.

## 1. Compatibility

- HTTP base `/v1`; every event carries `schemaVersion`.
- Additive optional fields are compatible; removal/renaming/meaning change requires major version.
- Client sends app version and supported schema range.
- Generated Go/TypeScript types are never hand-edited.

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

`eventId` survives retries; sequence is per workspace; server stores receipt time separately; source is `git`, `manual`, `mcp`, `hook`, or versioned adapter. Payload is selected by type. Secrets, file contents, diffs, prompts, transcripts, environment values, and raw command/test output are forbidden by this initial contract. ADR-027 authorizes an isolated L5A validation only; any later bounded visible-conversation event requires a separately reviewed versioned schema/generated-code change after that gate.

## 3. Initial event types

| Type | Payload |
|---|---|
| `workspace.registered` | repoFingerprint, label, capabilities |
| `workspace.manifest_started` | manifestId, revision, workstreamId, baselineRef, headRef, chunkCount |
| `workspace.manifest_chunk` | manifestId, chunkIndex, bounded paths with independent baseline/index/worktree states plus optional symbol/dependency metadata |
| `workspace.manifest_completed` | manifestId, revision, contentHash |
| `workspace.paused` / `workspace.resumed` | optional reason / empty |
| `workstream.intent_reported` | title, intendedOutcome, approachSummary, components/contracts, anticipatedPaths, optional planItemIds |
| `workstream.checkpoint_reported` | checkpointId, summary/discoveries, verification summaries, relatedManifestRevision, basedOnBriefId |
| `workstream.status_changed` | active/idle/done/blocked |
| `context.acknowledged` | briefId, consideredItemIds |
| `activity.reported` | decision/completion/blocker and summary |
| `agent.activity_reported` | hashed session workstream, vendor, lifecycle/status, bounded action label, safe paths, optional tool/subagent metadata |
| `claim.created` / `claim.released` | patterns / claim IDs or patterns |

Manifest revisions represent complete current state, not unreliable filesystem deltas. The server stages chunks and exposes a revision only after completion/hash/count validation. `chunkCount: 0` followed directly by completion is the canonical empty snapshot and clears a prior active manifest without inventing an empty chunk. Entries are strictly ordered by normalized path and paths are unique. Each entry has independent optional `baseline`, `index`, and `worktree` change states so a path can retain simultaneous committed, staged, and unstaged evidence; each layer carries its own status and optional rename/copy source path. Entries contain bounded metadata, never content or patches.

The manifest content hash is SHA-256 over the ordered entry stream. For every entry, serialize these fields in order: `path`, then for each of `baseline`, `index`, and `worktree`, the literal layer name, its status or the empty string, and its old path or the empty string. Separate every field with one NUL byte and include the trailing NUL. The empty manifest hashes the empty byte string. This encoding is shared by Go and TypeScript and must not be inferred from ordinary JSON serialization.

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
| `begin_work(idempotency_key, title, outcome, approach?, components?, contracts?, anticipated_paths?, plan_item_ids?)` | Create/resume workstream, establish baseline, and return initial `CoordinationBrief`. |
| `update_intent(workstream_id, revision, ...)` | Revision-check and update active intent. |
| `check_coordination(workstream_id, trigger?, since_cursor?, approximate_token_budget?)` | Return a relevant `CoordinationBrief`; budget range 128–800, default 400. |
| `report_checkpoint(checkpoint_id, workstream_id, summary, discoveries?, affected_interfaces?, dependencies?, verification?, manifest_revision?, based_on_brief_id?)` | Publish idempotent progress/change/verification state and return newly relevant context. |
| `acknowledge_context(brief_id, considered_item_ids)` | Record delivery consideration without claiming compliance/correctness. |
| `finish_work(idempotency_key, workstream_id, outcome, summary, verification?, manifest_revision?, based_on_brief_id?)` | Close/handoff work and return unresolved items. |
| `report_event(kind, summary)` | Report decision/completion/blocker. |
| `get_resolutions(since_revision?)` | Read collision resolutions relevant to the workstream. |

Tools never mutate Git/worktrees or control the external agent loop. `CoordinationBrief` prioritizes directly relevant unresolved decisions/findings, then evidence/dependency changes and workstreams. It includes `briefId`, `contextRevision`, trigger, budget/size/truncation, stable item IDs/revisions/relevance reasons, and cursor. Findings carry kind, confidence band, severity, provenance, and lifecycle state; raw vector scores are not the user contract. Raw test output/commands are forbidden; verification is bounded structured metadata.

## 7. Capabilities and errors

Workspace capability example:

```json
{"git":true,"mcp":true,"hooks":false,"manualIntent":true,"agentAdapters":["codex"]}
```

Stable error body:

```json
{"error":{"code":"schema_version_unsupported","message":"Upgrade Stickguy to continue.","requestId":"req_...","retryable":false,"details":{}}}
```

Codes are stable; messages may change. Never automatically retry authentication, authorization, validation, or incompatible-version errors.
