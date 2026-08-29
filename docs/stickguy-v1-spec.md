# Stickguy — Product Specification

Status: canonical build specification  
Owner: Khalid  
Last updated: 2026-08-23

Stickguy is a persistent coordination harness for people building software with coding agents. It gives a project team a live, shared picture of current intent, changed areas, plans, decisions, and likely collisions, then routes compact relevant context to each workstream without taking control of agents or modifying teammates' work.

## 1. Product vocabulary

Use these names in code, copy, schemas, and documentation.

| Term | Meaning |
|---|---|
| Project | Persistent coordination space for a product or repository; the primary user-facing container. |
| Member | A person with access to a project. |
| Device | One authenticated Stickguy installation belonging to a member. |
| Workspace | One local repository checkout registered to a project on a device. |
| Session | A bounded period of human or agent activity in a workspace. |
| Workstream | The objective a member is currently pursuing. |
| Plan item | Persistent unit of planned project work. |
| Activity event | Append-only structured observation such as intent reported, paths changed, or work completed. |
| Overlap | Deterministic evidence that active workstreams touch the same paths, symbols, dependencies, contracts, or claims. |
| Finding | Evidence-backed warning of a direct/likely collision, redundant work, shared dependency, assumption conflict, or downstream impact. |
| Sync card | A coordination item created from a finding or manually raised concern. |
| Decision | An agreed outcome that affected agents and members must see. |
| Checkpoint | A discoverable Git commit/ref published by a member; deferred until after alpha. |

Do not call a project a "room" in user-facing surfaces. A realtime room/channel may exist only as an implementation detail.

## 2. Problem and target user

Coding agents increase implementation throughput faster than teams can coordinate. Each person and agent develops a separate plan, changes code quickly, and silently accumulates assumptions. Git usually reveals conflicts after work is complete; chat and manual status updates do not match agent speed.

The initial user is a team of 2–5 people working in one Git repository during a hackathon, startup sprint, side project, or other high-velocity session. Members may use Codex, Claude Code, Cursor, another MCP-capable agent, or no agent. One member may participate in multiple projects and register multiple workspaces without running multiple Stickguy services.

## 3. Product principles

1. **Useful in under 60 seconds.** Installation and joining require no language runtime.
2. **Project history is durable.** Later members can understand plans, workstreams, decisions, and significant activity.
3. **Deterministic foundation, intelligent coordination.** Git evidence remains useful without AI; semantic intent/impact detection is a V1 capability and is always labeled as probabilistic.
4. **Data minimization before redaction.** Do not collect content unnecessary for coordination.
5. **Inform, never take control.** Never drive agents, merge code, or mutate a teammate's worktree.
6. **Honest fidelity.** Label whether information came from Git, manual input, MCP, hooks, or an adapter.
7. **Precision over notification volume.** Quiet evidence is better than a noisy alert.
8. **Portable boundaries.** The local client speaks a Stickguy-owned protocol, not a database-vendor protocol.

## 4. Core journeys

### Create and join

1. Install the single Stickguy executable.
2. `stickguy create` creates a project, enrolls the creator device, registers the current Git workspace, and opens the dashboard.
3. Stickguy produces an expiring invite code/link.
4. A teammate runs `stickguy join <code>` inside their checkout.
5. Their device is enrolled, workspace registered, service running, and both members appear in the live project view.

Median target: under 60 seconds from command to mutual visibility.

### Work with an agent

1. An MCP-capable agent calls `begin_work` and receives an initial coordination brief.
2. The local service observes changed paths from Git/filesystem.
3. Dashboard updates intent, touched paths, fidelity, and presence.
4. The agent calls `check_coordination` before broad/shared edits.
5. Stickguy returns only changes relevant to that workspace/workstream within a bounded brief.
6. The agent reports meaningful checkpoints/verification and the brief on which it relied.
7. Stickguy refreshes context if a relevant assumption became stale; completion records unresolved items.

### Resolve overlap

1. Two active workstreams produce structural or semantic evidence of a collision, redundancy, shared dependency, assumption conflict, or downstream impact.
2. Both dashboards show a finding with provenance, severity, and a plain-language reason.
3. A member creates/opens a sync card.
4. Members record a decision.
5. The resolution becomes visible in the Project and through `get_resolutions`.

### Return or join later

Authorized members see current/completed plan items, recent workstream summaries, durable decisions, resolved sync cards, retained significant activity, membership, and fidelity. Members who explicitly enable an owner-allowed activity profile may also share bounded agent progress or visible conversation events under `agent-activity-sharing.md`. Transcript files, system prompts, hidden reasoning, source/diffs, and secrets are never Project content.

## 5. Alpha scope (P0)

The first dogfooded alpha establishes the deterministic coordination loop. V1 is not complete until the coordination-intelligence requirements below also pass.

| ID | Requirement | Acceptance criterion |
|---|---|---|
| P0.1 | Single-binary installation | A supported user installs one signed executable without Go, Node, Python, or a package manager. |
| P0.2 | Project create/join | Two clean devices create/join one project and become mutually visible in under 60 seconds median. |
| P0.3 | One service per user | One service manages multiple projects/workspaces and rejects a second active instance. |
| P0.4 | Device enrollment | Invite exchange yields a revocable credential stored in the OS credential store; secrets are never plaintext server-side. |
| P0.5 | Presence | Dashboard shows online/idle/offline using server time and heartbeat expiry. |
| P0.6 | Workstream intent | Intent set by CLI/MCP updates dashboard within five seconds. |
| P0.7 | Changed paths | Git changes publish debounced paths/status only; file contents never leave the device. |
| P0.8 | Live project view | Dashboard shows members, fidelity, workstreams, paths, presence, and structured activity. |
| P0.9 | File overlap | Same normalized path produces a quiet badge with evidence and zero model calls. |
| P0.10 | Pause/privacy | Device, workspace, or Project pause stops activity payload transmission immediately. |
| P0.11 | Reliable delivery | Events use durable buffering, at-least-once delivery, deduplication, and reconnect backoff. |
| P0.12 | Graceful degradation | Unsupported-agent members remain visible through Git/manual input with honest labels. |

## 6. V1 coordination intelligence (P1)

| ID | Requirement | Acceptance criterion |
|---|---|---|
| P1.1 | Large-change manifest | Changes committed after the workstream baseline plus staged/unstaged/untracked changes are represented without pushing or uploading content; a 1,000-path fixture converges atomically after chunked delivery. |
| P1.2 | Structured intent | Agent/CLI/UI can report intended outcome, approach, affected components/contracts, and anticipated paths with source/fidelity labels. |
| P1.3 | Change capsules | An agent can report a bounded behavioral change summary tied to a manifest revision without transmitting source/diffs. |
| P1.4 | Structural impact | Same paths/symbols/claims and shared package/schema/route/dependency evidence produce stable, deduplicated findings. |
| P1.5 | Semantic radar | Semantically overlapping active intents/changes under different paths enter the candidate set in the labeled V1 evaluation corpus. |
| P1.6 | Evidence fusion | Findings distinguish direct collision, likely collision, redundant work, shared dependency, assumption conflict, and downstream impact; each explains evidence/provenance. |
| P1.7 | Cross-device realtime | A qualifying record from one device updates the other affected member's dashboard/MCP feed without fetching or pushing a Git branch. |
| P1.8 | Precision controls | Low-confidence candidates stay quiet; proactive alerts meet the owner-approved precision threshold on the labeled corpus and can be dismissed/acknowledged/resolved. |
| P1.9 | Failure fallback | Embedding/adjudication outage leaves structural findings live and queues semantic processing without blocking local observation. |
| P1.10 | Project isolation/privacy | Retrieval cannot cross project/repository authorization boundaries; only disclosed coordination metadata reaches hosted/model systems. |
| P1.11 | Context routing | Given four active workstreams, only those with a relevance edge receive each finding/decision; briefs honor the requested 128–800-token budget and explain every included item. |
| P1.12 | Stale assumptions | A checkpoint based on an older brief produces a stale-assumption finding only when a relevant decision/contract/dependency changed afterward. |
| P1.13 | Harness checkpoints | Begin, preflight, checkpoint, acknowledgement, and finish calls are idempotent, revisioned, and available through the official MCP adapter. |
| P1.14 | Verification metadata | Agents can report bounded pass/fail/unknown verification summaries tied to a manifest without raw command output/source upload. |
| P1.15 | Opt-in agent activity | An owner-enabled, member-opted-in supported adapter shares only the selected coordination/activity/conversation profile; exact disclosure is previewable, attributable, pausable, revocable, deletable, and proven not to retain or transmit prohibited fields. |

See `coordination-intelligence.md` for the canonical inputs, pipeline, finding contract, and evaluation model.

## 7. Additional V1 capabilities

Build in observed-demand order:

- structured project plan with items, owners, status, source, and revisions;
- per-session focus: a local, expiring request that coordination not be injected into one agent's turns, which never changes what that device publishes (ADR-061);
- path/glob claims;
- sync cards, discussion, resolution, and decision delivery;
- Codex and Claude Code setup adapters;
- opt-in Codex and Claude Code activity adapters after the L5A privacy/capability gate;
- while-you-were-away digest from structured events;
- auto-update and robust OS service lifecycle;
- explicitly authorized hosted read-only view;
- the ADR-029 macOS desktop preview shell and tray; native notifications, deep links, and supported distribution remain later gates;
- explicit checkpoint publishing/fetching after a Git-host spike;
- additional agent adapters and optional bounded narration.

## 8. Non-goals

- No transcript-file, system/developer-prompt, hidden-reasoning, source/diff, secret, environment-value, raw command/output, or tool-result upload in V1. Bounded visible user/assistant messages are allowed only under the explicit `conversation` profile.
- No automatic merge, rebase, cherry-pick, patch, checkout, reset, or worktree mutation.
- No source/diff upload to embedding or adjudication models and no AI dependency for deterministic findings.
- No assignment authority or blocking locks.
- No ownership of external agents' model loops, file/shell tools, test execution, context compression, or coding permissions.
- No enterprise hierarchy/SSO/audit product.
- No cross-repository project in alpha.
- No repository contents or Git objects stored by the backend.
- No default modification of tracked `AGENTS.md`, `CLAUDE.md`, or editor-rule files.

## 9. Privacy and retention defaults

- Source contents/raw agent logs: never copied or uploaded.
- Changed paths/status: 30-day hosted retention in alpha.
- Raw heartbeats: compacted and retained no longer than 24 hours.
- Workstreams, plans, sync cards, decisions: project lifetime unless deleted.
- Semantic summaries/embeddings/findings: bounded project retention, deleted with the source object/project and configurable before beta.
- Context delivery/acknowledgement receipts and routine verification summaries: 30 days by default; durable decisions retain their own provenance separately.
- Local queue: structured events only; delete after acknowledgement plus short recovery window.
- Paused workspace: only minimal connection health needed to display `paused`.
- Project deletion: revoke credentials and schedule all hosted project data for deletion.

Retention requires implemented cleanup jobs, not policy text alone.

## 10. Success measures

- median time to second member visible;
- percentage of active members with non-stale intent;
- sessions with at least one dashboard/MCP coordination read;
- useful versus noisy overlap feedback;
- semantic candidate recall and proactive-finding precision by finding kind;
- collisions surfaced before conflicting implementation or merge time;
- relevant brief-item precision/recall, rendered budget compliance, and critical-item acknowledgement latency;
- stale-assumption findings caught before completion versus false stale warnings;
- time from overlap detection to decision;
- decisions surfaced to every affected active device;
- projects completing a second collaborative session within 14 days.

Guardrails: join failure by platform, lost/duplicate visible events, idle CPU/memory, false overlaps, pause latency, and cost per active member-day.

## 11. Product gates

Do not confuse semantic capability with mandatory narration. Implement semantic candidate retrieval after the deterministic vertical slice, then enable proactive semantic notifications only after labeled precision evaluation. Do not add transcript or source sharing without a separate threat model and product decision.

The next implementation level begins immediately when the current level passes; estimates do not stop execution. Acceptance criteria and external blockers are the gates.
