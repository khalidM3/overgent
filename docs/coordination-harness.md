# Overgent — Coordination Harness

Status: canonical V1 design  
Last updated: 2026-08-23

## 1. Boundary

Overgent is a coordination harness around existing coding-agent harnesses. Codex, Claude Code, Cursor, and similar products continue to own their model loop, repository/file tools, command execution, context compression, coding permissions, test execution, and coding-model selection.

Overgent owns the shared loop those independent harnesses do not naturally have:

```text
observe team work
      ↓
maintain shared project state
      ↓
retrieve structural + semantic relationships
      ↓
route the smallest relevant context
      ↓
surface/resolve coordination situations
      ↓
record acknowledgement, decision, and outcome
```

Overgent never impersonates an agent, executes its coding tools, or claims to have paused it. An integration supplies context through explicit tools or a documented, reviewed lifecycle hook. The dashboard/native notification can notify a person immediately; stdio MCP alone cannot force an already-running agent to take another model turn. Some harnesses, including supported Codex versions, can inject context or apply control at lifecycle/tool hooks, but Overgent uses only capabilities accepted by an adapter-specific privacy/control ADR.

## 2. Harness components

| Component | V1 responsibility |
|---|---|
| Observation | Git baseline/current manifests, workstream/session lifecycle, agent reports, coarse symbols/dependencies, bounded verification state, and explicitly opted-in policy-projected agent activity. |
| Project model | Members, workstreams, plans, manifests, contracts, decisions, findings, revisions, and acknowledgements. |
| Semantic memory | Revisioned intent/change/plan/decision summaries and embeddings; no transcript or source memory. Opt-in visible conversation events are activity history, not automatic semantic-memory input. |
| Context router | Select, rank, budget, render, and version only information relevant to one workstream at one trigger. |
| Coordinator | Fuse evidence, assign advisory severity, manage finding/sync lifecycle, and request human coordination when warranted. |
| Delivery adapters | MCP responses, dashboard/live feed, native notification later, and explicit untracked context export. |
| Evaluation | Measure finding precision/recall, routing relevance, missed context, token/size budgets, and time-to-acknowledgement. |

These are domain boundaries, not independent network services. Keep them as testable modules inside the Go local core and Convex backend until measured scaling requires a split.

## 3. Agent coordination lifecycle

The preferred V1 integration is a short checkpoint protocol:

1. **Begin:** `begin_work` creates/resumes a workstream, records intended outcome/scope, establishes the Git baseline, and returns an initial coordination brief.
2. **Preflight:** `check_coordination(trigger="before_broad_edit")` returns relevant new findings, decisions, claims, and dependency changes before broad/shared edits.
3. **Checkpoint:** `report_checkpoint` records behavioral progress, discoveries, affected contracts/dependencies, structured verification state, related manifest revision, and the brief on which the agent relied.
4. **Refresh:** Overgent returns a new brief when the checkpoint reveals relevant changes or the relied-on context is stale.
5. **Finish:** `finish_work` records outcome/verification, closes or hands off the workstream, releases claims, and returns unresolved coordination items.

Calls are idempotent and workstream-scoped. Integrations should invoke them at natural harness boundaries, not after every tool call. Recommended triggers are start/resume, material plan change, before broad/shared edits, after a meaningful implementation/test checkpoint, when blocked, and before completion.

If an integration cannot support this lifecycle, Git observation and manual intent remain available with lower, honestly displayed fidelity.

## 4. Coordination brief

A `CoordinationBrief` is a deterministic, versioned projection for one workstream—not a generic team digest and not a transcript summary.

Required envelope:

```text
briefId, projectId, repositoryId, workstreamId
contextRevision, generatedAt, trigger
requestedBudget, renderedSize, truncated
items[]
nextCursor
```

Each item includes a stable object/finding ID and revision, kind, compact text, reason it is relevant, evidence/fidelity, advisory action, and priority. The agent can fetch an individual item by ID if the compact representation is insufficient.

### Routing order

The router first establishes authorization and repository/workstream scope, then ranks:

1. unresolved critical/high findings directly involving the workstream;
2. decisions or assumption changes that explicitly affect it;
3. direct path/symbol/contract/dependency intersections;
4. acknowledged dependencies whose revision changed;
5. strong semantic candidates;
6. nearby workstream status relevant to timing or ownership.

Ranking combines directness, severity, recency, source fidelity, novelty since cursor, lifecycle state, and prior acknowledgement. Unrelated team activity is omitted.

V1 accepts a caller-requested approximate budget from 128–800 tokens, default 400. Budgets are testable rendered-size targets, not claims about a coding model's remaining context. Critical items are never silently dropped: when they do not fit, return a compact reference/action and `truncated=true`.

The renderer is deterministic for the same authorized state, router version, and budget. Provider-generated prose is not required to create a brief.

Brief assembly is revision-safe even though semantic retrieval may run outside a database transaction: read the repository-scope context revision, retrieve candidate IDs, reauthorize/load current objects, then confirm the scope revision is unchanged before rendering. Retry a bounded number of times on change; if state remains busy, return a clearly labeled current structural brief rather than a falsely consistent semantic result.

## 5. Stale-assumption detection

Every brief has a monotonic project/repository context revision. Delivery records which item revisions the workstream received; acknowledgement records which it claims to have considered.

`report_checkpoint` carries an optional `basedOnBriefId`. Overgent compares that brief with current relevant state. It creates or updates a `stale_assumption` finding when a material decision, contract, dependency, or high-severity finding changed after the relied-on brief and is relevant to the checkpoint.

This is not triggered merely because unrelated project activity occurred. A newer global revision without a relevance edge does not make an agent stale.

A context acknowledgement proves delivery/consideration only. It does not prove the agent obeyed the information or that its implementation is correct.

## 6. Verification summaries

Overgent does not run tests for an external agent. An agent harness may report bounded verification metadata:

- state: `not_run`, `running`, `passed`, `failed`, or `unknown`;
- check kind/label, affected component, and observed time;
- bounded failure/impact summary;
- related manifest revision; and
- source/fidelity.

Raw command lines, logs, stack traces, snapshots, coverage artifacts, and source excerpts are not uploaded in V1. A failed shared-contract or integration check can raise impact priority for related workstreams; a passing report is evidence, not proof of global safety.

## 7. Advisory policy and permissions

V1 coordination outcomes are:

- `informational` — available in radar/history;
- `review_recommended` — included in the next relevant brief;
- `coordination_required` — prominently delivered to affected people/agents and requires acknowledge/resolve workflow.

They are advisory. Overgent does not block file writes, revoke an agent's tools, stop a coding loop, or create repository locks in V1. A harness-specific adapter may technically support pre-tool or stop policies, but production use requires a separate permission/privacy/UX ADR and must fail open or closed exactly as disclosed.

Overgent permissions govern project data, findings, decisions, integrations, and notification policy. The underlying coding harness continues to govern shell/file/network permissions.

## 8. Memory and compression

Durable memory is structured canonical state: current/completed workstreams, revisioned plans/decisions, significant checkpoints, findings/resolutions, and acknowledged dependencies. Ephemeral events are compacted according to retention policy.

Context compression means rendering relevant canonical objects into a small brief. V1 does not ingest a coding agent's full history and ask a model to summarize it. Optional visible conversation events do not become brief or embedding input unless a future focused ADR defines and validates that separate transformation. This keeps briefs reproducible, deletable, attributable, and portable across agent products.

## 9. Error and degradation behavior

- Backend offline: local observation/checkpoints queue; the brief clearly reports its last synchronized revision.
- Embedding/adjudication unavailable: structural/lexical routing continues; semantic fidelity is degraded.
- Adapter absent: Git/manual fidelity; never claim checkpoint or acknowledgement coverage.
- Budget exceeded: references plus truncation marker; never silently omit critical items.
- Stale cursor/brief: return current revision and relevant delta; do not dump full project history.
- Conflicting report revision: reject with current revision rather than last-write-wins.

## 10. Evaluation contract

Harness evaluation uses multi-workstream scenarios, not isolated answer quality. Fixtures must prove:

- an unrelated fourth agent receives no update;
- an affected agent receives the needed decision/finding within budget;
- a changed dependency invalidates only briefs that depended on it;
- an old but irrelevant project revision does not cause a stale warning;
- critical context is referenced rather than silently truncated;
- checkpoint retry is idempotent;
- provider/backend/adapter degradation is labeled honestly; and
- project/repository authorization survives retrieval, routing, and item fetch.

Store the router/engine versions with delivery and feedback so regressions are reproducible.
