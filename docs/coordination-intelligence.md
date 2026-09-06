# Overgent — Coordination Intelligence

Status: canonical V1 design  
Last updated: 2026-08-26

## 1. Product promise

Coordination intelligence is a V1 capability, not a later semantic-search add-on. Overgent must warn a team while work is in progress when active workstreams are likely to:

- edit the same path, symbol, contract, schema, route, dependency, or shared subsystem;
- implement substantially the same capability in different places;
- make incompatible assumptions;
- invalidate or depend on another active change; or
- create a large change whose blast radius intersects other work.

Overgent is an early-warning and decision-delivery system. It cannot guarantee conflict-free merges, understand every unreported intention, or safely merge work automatically.

## 2. Cross-device model

Each local service observes only workspaces registered on that device. It publishes bounded coordination records to the project service; the service compares records across all authorized members and streams findings back in real time.

```text
Member A workspace ─► manifest + intent/change capsules ─┐
Member B workspace ─► manifest + intent/change capsules ─┼─► project collision engine
Member C workspace ─► manifest + intent/change capsules ─┘             │
                                                                        ▼
                                                        dashboard + MCP findings
```

Do not periodically pull or mutate another member's worktree. Remote Git polling misses unpushed work, creates credentials/network/ref-management complexity, and is not realtime. A later explicit checkpoint feature may publish a commit/ref for deeper Git evidence after a checkpoint exists; it supplements rather than replaces live records.

One service may observe multiple local worktrees, including agent worktrees on the same machine. Their records enter the same pipeline with distinct workspace/session/workstream IDs.

## 3. V1 inputs

### Local change manifest

At session/workstream start, record a Git baseline without changing the repository. The current manifest is the union of:

- committed changes from the baseline to current `HEAD`;
- staged, unstaged, renamed, deleted, and untracked paths;
- locally extracted path, language, coarse component, symbol-name, import/package, route, schema/migration, and lockfile signals when available;
- contract fingerprints for the changed fingerprintable paths, described below; and
- explicit path/glob claims.

This captures a large agent change even after it commits locally and before it pushes. Manifests are snapshot/revision based, chunkable for hundreds or thousands of paths, and replace prior current state atomically after all chunks arrive.

No source lines, patch hunks, blobs, or Git objects are included.

### Semantic coordination objects

Agents, CLI, and UI create bounded, revisioned objects:

| Kind | Required meaning |
|---|---|
| `intent` | intended outcome, approach, affected components/contracts, and anticipated paths |
| `change` | what behavior changed, interfaces/dependencies affected, and related manifest revision |
| `plan` | project work item and assumptions |
| `decision` | durable choice and affected areas/workstreams |

Each object records project/repository/workstream, source and fidelity, lifecycle status, revision, bounded text, structured tags/references, and retention class. Summaries must describe behavior; dumping code into a summary is rejected by policy and discouraged by tool instructions.

MCP-capable agents provide the highest fidelity through `begin_work`/`update_intent` before broad edits and `report_checkpoint` at meaningful checkpoints. Unsupported tools retain deterministic Git/path fidelity and may use manual intent; the UI must never pretend they have semantic fidelity they did not report.

### Contract evidence: read sets and contract fingerprints

ADR-048 adds a deterministic answer to "the contract your session read has changed since you read it."

A **contract fingerprint** is derived structural metadata for one file: its exported symbols, each with a normalized declaration signature (body and comments removed, bounded and marked when truncated) and a hash of that signature, plus a `fileContractHash` over the sorted symbol stream. Only `.go`, `.ts`, `.tsx`, `.py`, `.pyi`, `.js`, `.jsx`, `.mjs`, `.cjs`, `.java`, `.rs`, `.cs`, `.php`, `.c`, `.h`, `.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx`, `.scala`, `.sc`, `.kt`, `.kts`, and `.dart` files are fingerprinted; every other path has no fingerprint and can never produce contract evidence. Extraction is local: Go uses the standard library parser, TypeScript uses a bounded pure-Go scanner, and Python, JavaScript, Java, Rust, C#, PHP, C, C++, Scala, Kotlin and Dart use tree-sitter grammars running as WebAssembly under wazero (ADR-063), which keeps the root module free of CGO and Node. Each grammar is a separate WebAssembly module compiled on first use, so a repository pays only for the languages it contains. On a platform without wazero's compiler the wasm-backed languages produce no fingerprint and say so rather than degrade silently. A parse failure yields no fingerprint and never blocks manifest publication. Because the hash covers only the exported surface, editing a function body or a comment leaves it unchanged and produces no wire traffic and no finding.

A **read set** records, per agent session, which fingerprintable files that session observed and the `fileContractHash` current at that moment. It is fed by hook events whose tool category is a file inspection over a safe repository-relative path, plus the paths an MCP client reports consuming at `begin_work`. Entries are deduplicated locally to one per (session, path): re-observing a path replaces its hash and time.

Each entry carries a **fidelity** naming how the read was learned of (ADR-052): `observed` when the vendor named the file to a file-reading tool, `vendor_inferred` when the vendor's own classifier concluded a command read it, and `self_declared` when an MCP client named the path itself. These sources have different strength, so the strongest evidence for a path wins and a later weaker report never downgrades it; an upgrade republishes, because it can raise the confidence band of a finding already derived from that path.

A session also carries a **read coverage** naming the strongest evidence available for it on this device, which is a statement about capability rather than about what has been read so far. Claude names each file it reads to a file-reading tool, so its reads are `observed`. Reporting this is what stops an empty read set from being read as an all-clear: a session with no read coverage can never receive a `stale_assumption` finding, and silence there means absence of evidence, not absence of drift.

Codex needs a different route, because it inspects source through the shell and a shell observation carries a command rather than a list of files. At a turn boundary — `Stop` or `SessionEnd`, never during a turn — the service starts a private Codex app-server child and reads the session's stored task, which the hook's own session id names. Codex's classification of each completed command supplies `read` actions with a path; `listFiles` and `search` do not, because neither shows that a particular file's contract was examined. Recovered paths are checked against the registered repository root and dropped if they escape it, and the decoder has no field for the command or its output, so neither is ever held. The evidence is `vendor_inferred`: Codex classifies commands best-effort, a compound command that genuinely reads files can come back `unknown`, and coverage is partial by measurement rather than only in principle. A device with no reachable Codex reports `none` instead, because claiming coverage that does not exist is the failure this field exists to remove.

A session counts as live until it is `done`, which makes it important that sessions actually reach `done`. `Stop` reports idle and `SessionEnd` reports done, so a session whose `SessionEnd` never arrives — a closed terminal, a Codex chat left unarchived — would stay live indefinitely and keep colliding with everything that followed it. The retention sweep therefore ends agent sessions that have not reported for thirty minutes, recording the last moment they were actually seen as the end time rather than the moment the sweep noticed, and resolving their routed findings exactly as a real `SessionEnd` would have. The window is far longer than any gap between turns and far shorter than a working day. Expiry is not final: an event from a revived session sets it straight back to active. Only agent sessions expire — a workspace workstream or a manually reported intent has no turn loop, so silence says nothing about whether it is finished. That workspace workstream, which exists only so residual git evidence has an owner, is for the same reason never listed as a session: it declares no goal, nothing ends it, and shown beside real sessions it read as one that had been working forever.

Not every session a vendor reports is a session a member has. Codex runs threads of its own inside the member's checkout — ambient suggestion generation and the safety pass that screens it — and each fires the full hook lifecycle under a session id of its own, down to `Stop`. Those are dropped at the device rather than published, recognized by the fact that Codex records a rollout for every thread a member can open and none for these. The drop is counted and reported by `doctor`, because this gate is also what would silently swallow every Codex session if Codex ever changed how it records threads.

The hosted service keeps the latest fingerprint per (repository scope, path) and the read sets of live session workstreams. When a file's `fileContractHash` moves, it diffs the stored symbol list against the new one and, for every *other* live session in that scope whose read set holds an older hash for the path, upserts one `stale_assumption` finding keyed by (session, path, new hash), so redelivery is a no-op. Severity is `high` when a symbol the session read changed signature or disappeared; a file that only gained exported symbols invalidates nothing a reader already depends on and produces no finding at all. Evidence of kind `symbol` carries the path, the changed symbols with their old and new signatures, the workstream that changed them, and the read and change times, so a brief can name exactly what moved.

Contract evidence from an `observed` read is deterministic and works with AI disabled. Evidence from an inferred or declared read is not: the symbol diff is structural either way, but the claim that a particular session read the file is only as strong as its weakest link, so the finding's confidence band steps down with the fidelity and its wording says which. It is the trigger layer of ADR-045, not a judgment: the finding reports that a contract moved under a session, not whether that session's work is now wrong.

### Dependency readiness

An active workstream may declare up to eight bounded `waiting_on` claims naming
a contract, exported symbol, or path. Readiness is derived only from another
live workstream's contract fingerprints in the same Project/repository scope;
intent prose is not treated as a claim. Matching contract evidence first yields
a medium-severity `stable_wip` finding while the producer's latest checkpoint is
absent or unverified. A passing latest checkpoint upgrades that same finding ID
to high-severity `ready` at a new revision. Dropping the claim or finishing the
claiming workstream resolves the finding. These findings use the ordinary brief
and hook-injection route; there is no planning board or separate delivery path.

## 4. Detection pipeline

Every material manifest or semantic-object revision runs an incremental, project-isolated pipeline:

1. **Eligibility:** compare active, non-stale workstreams for the same registered repository identity.
2. **Structural retrieval:** find exact/normalized path, claim, symbol, package, schema, route, import/dependency, and broad-change intersections.
3. **Lexical retrieval:** compare bounded summaries/tags for exact terms and rare identifiers.
4. **Semantic retrieval:** embed approved coordination summaries and retrieve nearby active objects within the project/repository.
5. **Evidence fusion:** combine independent signals, recency, workstream state, change breadth, and source fidelity into a versioned risk score.
6. **Adjudication:** the judgment layer in section 6 decides relationship, confidence, severity, explanation, and delivery for every candidate. For ambiguous candidates only, a replaceable model may refine that verdict against a strict schema. It receives coordination summaries/evidence, never source or diffs.
7. **Finding lifecycle:** upsert a stable finding fingerprint; notify only on meaningful severity/evidence changes; allow dismiss, snooze, acknowledge, convert to sync card, resolve, and feedback.
8. **Context routing:** deliver the finding only to workstreams with a supported relevance edge, using the budgeted/versioned brief contract in `coordination-harness.md`.

Vector similarity is candidate retrieval, never proof. A direct same-path collision remains understandable and functional when embedding or adjudication providers are unavailable.

## 5. Finding contract

V1 finding kinds:

- `direct_collision` — same path/symbol/claim or incompatible edits to one contract;
- `likely_collision` — different areas with evidence of interacting changes;
- `redundant_work` — active workstreams appear to implement the same capability;
- `shared_dependency` — both rely on or change a shared package/schema/API;
- `assumption_conflict` — stated plans or decisions appear incompatible;
- `downstream_impact` — one change likely invalidates or requires action in another workstream.
- `stale_assumption` — a checkpoint relied on an older brief that has since been materially invalidated for that workstream, or a contract fingerprint in a live session's read set changed after the session read it.
- `dependency_ready` — another workstream has produced matching contract evidence for a declared dependency, labeled `stable_wip` until its latest checkpoint passes and `ready` afterward.

Every visible finding includes: kind, severity, confidence band, affected workstreams/members, evidence with provenance, first/last seen, current status, and a plain-language reason. Do not show an unexplained similarity score as a warning.

These are requirements on the finding as presented, not on every row that mentions it. The workroom carries the summary — kind, the plain-language reason, who is affected, when it was first seen, and the way in — and the inspector beside it carries the whole contract, including severity, confidence band, evidence with provenance, and last-changed. Severity was always inspector-only, which is the precedent (ADR-060). What must never happen is a warning with no reasoning reachable from it: the reason sentence stays on the summary, in plain language, so the row is never an unexplained alarm.

Broad changes receive increased observation priority, not automatic severity. Touching hundreds of files may be a generated or mechanical change; severity requires intersection or semantic evidence.

## 6. Judgment layer

ADR-045 splits the engine in two. Deterministic evidence — path overlap,
contract-fingerprint drift, manifest state — is the trigger layer and always
operates offline. The judgment layer decides what a candidate *means*, how
certain that reading is, and where the answer belongs.

### The adjudicator is a provider behind an interface

`JudgmentProvider` mirrors `EmbeddingProvider`: one operation that takes a
bounded, policy-passed description of two or more candidate workstream states
and returns a structured verdict.

```text
{ relationship, confidence, severity, explanation, delivery }
```

`relationship` is one of `contract_drift`, `duplicate_behavior`,
`shared_dependency`, `path_overlap`, `assumption_conflict`,
`downstream_impact`, or `unrelated`. `delivery` is `next_turn`, `dashboard`, or
`silent`.

- **Managed provider.** Anthropic Claude Sonnet, called only from a hosted
  asynchronous Convex action. Adjudication runs on every ambiguous candidate in
  every project, so it is high volume and cost-sensitive; Sonnet is the
  default. `ANTHROPIC_API_KEY` is a hosted deployment secret exactly as
  ADR-040 defines it for embeddings: it is never available to the local core,
  the dashboard, agent configuration, logs, or Project records, and the
  provider class never reads a process environment itself.
- **Deterministic fallback.** With no key configured, on provider failure, on a
  bounded timeout, or on a response this service cannot validate, the offline
  verdict stands. Failure marks semantic processing degraded; it never removes
  or downgrades a deterministic finding. A managed verdict may refine severity
  and wording, and it is never allowed to silence deterministic evidence.

The deterministic verdict is computed synchronously and is durable before any
managed request is made, so provider latency or outage cannot delay a finding.
The managed request is skipped entirely when a candidate is structurally
unambiguous — an exact same-path collision explains itself — or when the
verdict is already silent, and it is bounded per project per hour. A late
result is discarded when the finding has moved on to a newer revision.

### What the judgment layer produces

**Duplication.** A `redundant_work` finding names both workstreams and the
behavior words they actually shared, drawn from the versioned public
coordination vocabulary, so the reason says *what* is duplicated rather than
that a similarity score was high. It is delivered at a severity that does not
ask either agent to stop.

**Work in progress.** A checkpoint reports what verification it ran. Structured
verification entries are authoritative; a bounded summary that declares its own
state, or that describes work-in-progress, is read only when the publisher sent
none. When the workstream that moved a contract reported an unverified
checkpoint, the resulting drift finding says so explicitly and carries a
severity strictly below the verified case: the reader is told the new shape is
provisional rather than told to stop and adapt to it. Contract evidence usually
arrives before its context does — the scanner publishes the fingerprint change,
and the checkpoint that says whether the change is finished follows it — so
open contract findings are re-judged when their author's verification state
lands. A later passing checkpoint takes the qualifier back off.

**Redundant notices.** A coarse "you both name this contract" notice adds
nothing when the contract-fingerprint engine already reports every change to
that exact symbol with old and new signatures. The judgment layer silences it
and lets the exact evidence carry the finding. It also holds a shared-contract
candidate that rests only on two anticipated contract lists until at least one
side reports actual work: overlapping plans are a candidate, not a finding.

### The delivery decision

Every finding is routed through one function. `silent` means the candidate
never becomes a coordination object at all — it is not a low-priority item, it
is one nobody should be asked to read. `dashboard` is delivered and labeled
`review_recommended`. `next_turn` is delivered and labeled
`coordination_required`, which is what a receiving agent sees as the action on
an injected item. No supported vendor exposes a mid-turn interrupt channel
(ADR-033/046), so `next_turn` describes urgency and labeling, not a separate
transport.

### Where the work is happening, and whether its author means to keep it

Two facts about a candidate's workstreams move a verdict after the relationship
has been decided (ADR-061). Both are offline and deterministic, and neither can
silence a finding or invent one.

**Branch relation.** A candidate whose workstreams are all on one branch is
`shared`; one whose workstreams are on different branches is `divergent`; any
missing branch makes it `unknown` and changes nothing. Divergence is the reason
this is read at all: work on separate branches does not meet until someone
merges, so nothing outside the Project reports the overlap in the meantime, and
a `path_overlap` or `contract_drift` candidate at medium severity is escalated
one step — one step only, never from low, never to critical. A shared branch
keeps its severity and gains a sentence saying Git will surface the same fact at
the next pull, push, or shared write. A shared branch is not evidence of safety;
it is evidence that something else is also watching.

Branch is never a gate. Detecting collisions only within a branch would make
Overgent silent in exactly the case it exists for, and would make the silence
reachable with `git checkout -b`.

**Declared exploratory work.** A workstream whose own title or summary calls the
work a spike, a prototype, a throwaway, or an experiment is read as exploratory.
A `path_overlap` or `duplicate_behavior` candidate involving one is capped at
`dashboard` rather than routed into a turn: it collides like real work, and it
is not worth an interrupt for code its author has said may never land. Only an
explicit statement counts, silence stays standard, and the cap is a
de-escalation, so it cannot spend the precision budget in §8. Contract drift is
never capped this way — a reader's assumption is stale whatever the changer
intended to keep.

## 7. V1 semantic engine

The managed V1 implementation uses a provider-neutral `EmbeddingProvider` plus a `SemanticIndex` domain interface. The initial hosted adapter uses Convex vector search because shared coordination state already lives there. Store vectors separately from readable objects. Use an opaque composite project-and-repository `scopeKey` as the mandatory indexed filter; inactive objects have no searchable vector. Reauthorize and reload current objects after retrieval before creating a finding.

Only approved summaries are sent to the embedding provider. The provider name/model/version and object content revision are recorded so embeddings can be rebuilt or migrated. Provider failure queues/retries semantic work and leaves structural detection operational.

TurboVec is not required for V1. It remains a possible local/self-hosted `SemanticIndex` adapter after real corpus benchmarks justify a Rust sidecar. The product contract must not expose Convex-, model-, or TurboVec-specific IDs.

## 8. Precision and evaluation gate

Ship fixtures and a labeled evaluation corpus before enabling proactive semantic notifications. It must include:

- same behavior implemented under unrelated paths/names;
- conflicting permission/session/schema assumptions before files overlap;
- shared API or migration blast radius;
- two large but independent mechanical changes;
- semantically related work that is not actually conflicting;
- stale/completed workstreams;
- cross-project and cross-repository isolation; and
- an unrelated workstream that must receive no brief item; and
- adversarial summaries containing secrets, code dumps, or prompt injection text.

Measure candidate recall separately from alert precision. Low-confidence candidates stay in a quiet radar view; only precision-qualified findings interrupt members. Feedback (`useful`, `not_related`, `already_coordinated`, `missed_severity`) is versioned evaluation data and never silently trains a model on private project content.

## 9. Delivery latency

The coordination evaluation records `deliveryMillis` per scenario: how long an affected session waited, across real turn boundaries, before a stale-contract correction was injected into its next turn. Delivery is a latency contract, not only a correctness one. A correction that always arrives but arrives late is a correction the agent has already worked past.

### Brief creation is per session, not per deployment

Every turn boundary asks the hosted service for a brief, so `POST /v1/workstreams/{id}/briefs` is the delivery path for every correction. Its edge rate limit used to be keyed by `requestRateKey`, which returns one constant bucket — `shared-unauthenticated` — for all callers. Sixty briefs per minute was therefore the budget for an entire deployment rather than for a caller, and the window is a fixed one that resets only when it rolls over.

The consequence was a delivery tail with a distinctive shape. Once any session had spent the deployment's allowance, every other session's brief request returned `429`, the hook injected nothing, and the correction waited out the remainder of the window. In the evaluation this appeared as `deliveryMillis` of roughly 8–15 seconds in scenario C and occasionally A, in about one run in three, against a normal value near 50 ms. The evidence is direct: in a slow run, the injection path logged an unbroken run of `rate_limited (429)` responses from 12:27:05.2 until 12:27:12.7, and the deployment's single `rateLimits` row for `edge.workstreams.briefs` shows a window opening at 12:27:12.691 immediately after the previous window had reached exactly its limit of 60. Delivery succeeded 20 ms into the new window. The document ID was identical across every eval run and every device, confirming one shared bucket; because the local anonymous deployment's SQLite file persists between runs, an exhausted window also carried over from one run into the next.

The bucket is now keyed per `(credential, workstream)`, so a session's budget belongs to that session. The route keeps a shared bucket as a coarse guard on pre-authentication work under `edge.workstreams.briefs.shared`, sized for a deployment rather than a caller, so an unauthenticated flood still cannot mint a fresh bucket per forged credential. After the change the same suite shows per-session buckets peaking at 1–4 of 60 and the shared guard at 90 of 600, with no `429` on the delivery path in any run.

Two related defects on the same path were fixed with it. `loadContext` in `intelligence.ts` threw bare `"unauthorized"`, `"not_found"`, and `"forbidden"` messages rather than the shared `E:` form the HTTP boundary classifies, so a brief for a session the service had not seen yet was reported as a retryable `500 internal_error` instead of `404`. And the evaluation's `forceScan` treated an empty local queue as proof that a forced scan had published its evidence; on a busy machine the scan outlives the fixed RPC read deadline, and the queue is empty precisely because the scan has not enqueued anything yet. The service now exposes a `scanCycles` counter in `doctor` that advances once per completed scan pass whether or not that pass published anything, and the harness waits for that before draining.

### What the tail was not

The suspects that the shape suggested were checked and ruled out with the same instrumentation. Hosted finding evaluation is synchronous inside the projection mutation: `upsertContractFindings` runs in the same mutation that records the changed fingerprint, so a finding exists as soon as its event is projected. `CreateBrief` itself was never the slow part: measured end to end from the local service it ran in 7–25 ms in normal runs, and during the throttled window every one of its `429` responses came back in under 13 ms. Convex scheduler and dev-deployment recompilation were not involved; the local backend was warm for the whole run and no scheduled function sits between the change and the finding. The local flush loop's exponential `retryDelay` never engaged on the delivery path, because the queue had already drained before each scenario started timing.

Two adjacent limits share the root cause and are recorded here rather than changed, because neither is on the measured delivery path: `edge.events.batch` reached its deployment-wide ceiling of 120 during eval runs, and `edge.presence.heartbeat` sits pinned at its ceiling of 12 in every window, which means presence for a whole deployment is throttled well below the roughly 15-second heartbeat that `architecture.md` assumes. Both are keyed the same shared way on routes that carry a device credential.

### Residual variance

Across thirty-four runs after the rate-limit fix, scenario A ranged 46–96 ms and scenario C 47–172 ms, with a single 2997 ms outlier. That outlier is not a rate-limit artifact: `CreateBrief` returned successfully with zero items for three attempts before the finding appeared, on a run loaded enough that one hook invocation took 1.58 seconds — the shape the `forceScan` precondition above produces. It has not recurred; the ten runs after that precondition was corrected all landed between 46 and 80 ms. Runs on a shared machine are also perturbed by the anonymous deployment's fixed ports 3210/3211, which a second checkout of the repository will take, so a concurrent eval elsewhere on the machine either blocks the run outright or competes for CPU with it.

## 10. Privacy boundary

Coordination summaries, symbol/path metadata, and embeddings are sensitive project metadata. Enrollment and settings disclose their processing and retention. Source, diffs, prompts, transcripts, and environment values remain prohibited as intelligence/embedding inputs. Optional visible conversation events under ADR-027 remain separate activity history unless another focused ADR and evaluation gate permits a bounded derived coordination summary.

Project deletion removes semantic objects, embeddings, findings, and evaluation feedback. Object supersession/deletion removes or tombstones the corresponding vector. Authorization is enforced before retrieval and again before loading results; vector similarity must never cross project boundaries.
