# Stickguy — Coordination Intelligence

Status: canonical V1 design  
Last updated: 2026-08-26

## 1. Product promise

Coordination intelligence is a V1 capability, not a later semantic-search add-on. Stickguy must warn a team while work is in progress when active workstreams are likely to:

- edit the same path, symbol, contract, schema, route, dependency, or shared subsystem;
- implement substantially the same capability in different places;
- make incompatible assumptions;
- invalidate or depend on another active change; or
- create a large change whose blast radius intersects other work.

Stickguy is an early-warning and decision-delivery system. It cannot guarantee conflict-free merges, understand every unreported intention, or safely merge work automatically.

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
- contract fingerprints for the changed `.go`, `.ts`, and `.tsx` paths, described below; and
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

A **contract fingerprint** is derived structural metadata for one file: its exported symbols, each with a normalized declaration signature (body and comments removed, bounded and marked when truncated) and a hash of that signature, plus a `fileContractHash` over the sorted symbol stream. Only `.go`, `.ts`, and `.tsx` files are fingerprinted; every other path has no fingerprint and can never produce contract evidence. Extraction is local: Go uses the standard library parser, and TypeScript uses a bounded pure-Go scanner, because ADR-019 keeps the root module free of CGO and Node. A parse failure yields no fingerprint and never blocks manifest publication. Because the hash covers only the exported surface, editing a function body or a comment leaves it unchanged and produces no wire traffic and no finding.

A **read set** records, per agent session, which fingerprintable files that session observed and the `fileContractHash` current at that moment. It is fed by hook events whose tool category is a file inspection over a safe repository-relative path, plus the paths an MCP client reports consuming at `begin_work`. Entries are deduplicated locally to one per (session, path): re-observing a path replaces its hash and time.

The hosted service keeps the latest fingerprint per (repository scope, path) and the read sets of live session workstreams. When a file's `fileContractHash` moves, it diffs the stored symbol list against the new one and, for every *other* live session in that scope whose read set holds an older hash for the path, upserts one `stale_assumption` finding keyed by (session, path, new hash), so redelivery is a no-op. Severity is `high` when a symbol the session read changed signature or disappeared; a file that only gained exported symbols invalidates nothing a reader already depends on and produces no finding at all. Evidence of kind `symbol` carries the path, the changed symbols with their old and new signatures, the workstream that changed them, and the read and change times, so a brief can name exactly what moved.

Contract evidence is deterministic and works with AI disabled. It is the trigger layer of ADR-045, not a judgment: the finding reports that a contract moved under a session, not whether that session's work is now wrong.

## 4. Detection pipeline

Every material manifest or semantic-object revision runs an incremental, project-isolated pipeline:

1. **Eligibility:** compare active, non-stale workstreams for the same registered repository identity.
2. **Structural retrieval:** find exact/normalized path, claim, symbol, package, schema, route, import/dependency, and broad-change intersections.
3. **Lexical retrieval:** compare bounded summaries/tags for exact terms and rare identifiers.
4. **Semantic retrieval:** embed approved coordination summaries and retrieve nearby active objects within the project/repository.
5. **Evidence fusion:** combine independent signals, recency, workstream state, change breadth, and source fidelity into a versioned risk score.
6. **Adjudication:** for ambiguous high-value candidates only, a replaceable model may classify a bounded pair into a strict schema. It receives coordination summaries/evidence, never source or diffs.
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

Every visible finding includes: kind, severity, confidence band, affected workstreams/members, evidence with provenance, first/last seen, current status, and a plain-language reason. Do not show an unexplained similarity score as a warning.

Broad changes receive increased observation priority, not automatic severity. Touching hundreds of files may be a generated or mechanical change; severity requires intersection or semantic evidence.

## 6. V1 semantic engine

The managed V1 implementation uses a provider-neutral `EmbeddingProvider` plus a `SemanticIndex` domain interface. The initial hosted adapter uses Convex vector search because shared coordination state already lives there. Store vectors separately from readable objects. Use an opaque composite project-and-repository `scopeKey` as the mandatory indexed filter; inactive objects have no searchable vector. Reauthorize and reload current objects after retrieval before creating a finding.

Only approved summaries are sent to the embedding provider. The provider name/model/version and object content revision are recorded so embeddings can be rebuilt or migrated. Provider failure queues/retries semantic work and leaves structural detection operational.

TurboVec is not required for V1. It remains a possible local/self-hosted `SemanticIndex` adapter after real corpus benchmarks justify a Rust sidecar. The product contract must not expose Convex-, model-, or TurboVec-specific IDs.

## 7. Precision and evaluation gate

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

## 8. Privacy boundary

Coordination summaries, symbol/path metadata, and embeddings are sensitive project metadata. Enrollment and settings disclose their processing and retention. Source, diffs, prompts, transcripts, and environment values remain prohibited as intelligence/embedding inputs. Optional visible conversation events under ADR-027 remain separate activity history unless another focused ADR and evaluation gate permits a bounded derived coordination summary.

Project deletion removes semantic objects, embeddings, findings, and evaluation feedback. Object supersession/deletion removes or tombstones the corresponding vector. Authorization is enforced before retrieval and again before loading results; vector similarity must never cross project boundaries.
