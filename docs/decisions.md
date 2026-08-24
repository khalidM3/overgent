# Stickguy — Architecture Decision Log

## ADR-001: Project is the persistent container

Use `Project`; sessions/presence are time-bounded children. History, membership, plans, and decisions outlive a live gathering. Accepted 2026-08-23.

## ADR-002: Go is the permanent local-core language

CLI, service, watchers, Git, storage, sync, MCP, and updater use Go. It gives standalone distribution, Tier 1 MCP, simple concurrency, approachable OSS, and sufficient I/O performance. Rust is not a planned migration; add only for measured subsystem need. Accepted 2026-08-23.

## ADR-003: One service per user

One service owns state/watchers; CLI and MCP are clients. This avoids duplicate processes, ports, queues, and conflicting state. Accepted 2026-08-23.

## ADR-004: Convex hosts coordination state

Use Convex for realtime state, transactions, HTTP actions, and schedules. Go speaks only the Stickguy `/v1` contract; IDs remain vendor-neutral. Accepted 2026-08-23.

## ADR-005: Hosted dashboard first, Wails later

CLI opens hosted React for alpha. Reuse it inside Wails when native demand is demonstrated. Accepted 2026-08-23.

## ADR-006: Data minimization is the privacy boundary

V1 does not collect source content, raw transcripts, system prompts, or environment values. Redaction cannot make unnecessary collection safe. Accepted 2026-08-23.

## ADR-007: At-least-once event delivery

Use SQLite queue, stable IDs, server dedupe, acknowledgements. Intermittent devices do not require exactly-once transport when effects are idempotent. Accepted 2026-08-23.

## ADR-008: Backend is canonical for decisions

Deliver via API/MCP and optional untracked context, not a shared tracked append file that creates Git collisions. Accepted 2026-08-23.

## ADR-009: Apache-2.0 open-source application with managed cloud

Publish the installed Go client, adapters, protocols, installers, release workflows, dashboard, and core backend authorization/retention/coordination code under the unmodified Apache License 2.0. Keep production operations, billing, private admin/abuse systems, runbooks, and private eval data in a separate private repository. Use `Copyright 2026 Stickguy contributors` as the initial `NOTICE` attribution; a later legal entity or rights assignment requires an explicit notice review rather than a template substitution. Accepted by the owner 2026-08-23.

## ADR-010: Coordination intelligence is a V1 capability

V1 combines deterministic structural evidence with semantic retrieval over bounded, synchronized intent/change/plan/decision summaries. Semantic results are probabilistic candidates with provenance; they never replace deterministic evidence. Accepted 2026-08-23.

## ADR-011: Publish live manifests; do not poll peer worktrees

Each client reports its own baseline-to-current manifest and coordination summaries. The hosted project service compares members in realtime. Pulling/fetching peer Git state is optional checkpoint evidence only because it misses unpushed work and creates repository/network side effects. Accepted 2026-08-23.

## ADR-012: Hosted semantic index behind portable interfaces

Managed V1 uses provider-neutral embedding/adjudication interfaces and a `SemanticIndex` adapter backed initially by Convex vector search. Only approved coordination summaries reach providers; source and diffs remain prohibited. TurboVec is a future local/self-hosted adapter only when measured need justifies its packaging cost. Accepted 2026-08-23.

## ADR-013: Stickguy is a coordination harness, not a coding harness

Existing products retain their model loops, coding tools, execution permissions, test execution, context compression, and model routing. Stickguy owns cross-agent observation, shared project memory, relevance routing, advisory coordination, and delivery acknowledgement. Accepted 2026-08-23.

## ADR-014: Context is routed as versioned workstream briefs

Do not broadcast all team state to every agent. Build deterministic `CoordinationBrief` projections with stable item revisions, relevance reasons, a requested 128–800-token budget, delivery/acknowledgement state, and monotonic context revision. Staleness requires a material relevant change, not merely a newer global revision. Accepted 2026-08-23.

## ADR-015: Validate vendor surfaces before production implementation

Run bounded executable spikes for Codex MCP/hooks, Git baseline/worktree observation, Convex isolation/vector behavior, Go single-service distribution, and the intelligence eval seed before L0. A failed vendor feature narrows its adapter or uses a portable fallback; it does not silently reshape the core protocol. Accepted 2026-08-23.

## ADR-016: Accept Codex stdio MCP; withhold hook fidelity

Codex CLI `0.148.0-alpha.15` discovers a project-scoped stdio server built with the pinned official MCP Go SDK `v1.7.0`, consumes front-loaded initialization instructions, and calls the four fixture lifecycle tools. Accept that exact MCP surface for planning. `SessionStart` and `SubagentStart` remain `available_but_unverified`: ephemeral runs did not prove context injection, and persistent-session testing would have retained transcripts outside the L-1 evidence boundary. Do not install or advertise hooks until L5 proves them in a trusted isolated worktree, including structured config merge/removal and visible degradation. MCP plus Git/manual fidelity is the selected fallback. Accepted 2026-08-23.

## ADR-017: Accept the baseline-to-current Git manifest model

Real Git fixtures prove that a captured workstream baseline plus current committed, staged, unstaged, renamed, deleted, ignored, and untracked path state represents local work before push without uploading content, diffs, or Git objects. Non-ancestor, missing, detached, remote-ambiguous, malicious-path, symlink, overflow, and 1,000-path atomic chunk states have explicit outcomes. L0 encodes independent `baseline`, `index`, and `worktree` change states per path so simultaneous evidence is not collapsed; L1 must add real watcher/platform coverage and a SHA-256 repository fixture. Accepted 2026-08-23; L0 encoding settled 2026-08-23.

## ADR-018: Retain Convex shared state and scoped vector search

An anonymous loopback Convex `1.45.0` deployment proves two-client realtime state, transactional event dedupe, atomic 1,000-path activation, monotonic repository revisions, mandatory composite scope filtering, post-retrieval authorization/current reload, vector update/migration/deletion, bounded race fallback, and retention deletion. Retain ADR-004 and ADR-012. Production must derive authorization scope server-side, batch/index cleanup, and separately measure hosted cost/load and real embedding dimensions. Accepted 2026-08-23.

## ADR-019: Accept the Go single-service boundary on macOS; narrow other platforms

One Go 1.26 executable on macOS arm64 proves CLI/service/stdio modes, lock-before-state-mutation, health-checked current-user Unix IPC, pure-Go SQLite restart recovery, Keychain access without plaintext fallback, and LaunchAgent lifecycle. CGO-free artifacts cross-compile for every planned target, but Linux service/keyring and Windows named-pipe/service/Credential Manager behavior remain unsupported until native-runner validation. Do not advertise those runtime targets based on cross-compilation alone. Accepted 2026-08-23.

## ADR-020: Accept the synthetic intelligence evaluation seed

The versioned public seed executes structural, lexical, and explicitly synthetic concept-vector candidate baselines, expected finding labels, repository/lifecycle isolation, and workstream-scoped routing. It recalls every positive seed case and intentionally preserves an independent-large-change false positive, confirming that vector proximity is candidate evidence rather than a proactive finding. Provider selection and alert thresholds remain L6 work; proactive semantic notifications stay disabled until the labeled precision gate passes. Accepted 2026-08-23.

## ADR-021: Generate public types from a dereferenced OpenAPI 3.1 bundle

Keep `protocol/openapi.yaml` and the bounded files in `protocol/schemas/` as reviewed external contract sources. Pin Redocly CLI `2.47.0` to produce an ephemeral dereferenced bundle, then generate Go with `oapi-codegen` `v2.8.0` and TypeScript with `openapi-typescript` `7.10.1`. Never commit the intermediate bundle. The checked-in language outputs are derived artifacts; `pnpm protocol:check` regenerates them in an isolated temporary directory and requires byte identity. This preserves JSON Schema 2020-12 `$defs` semantics without weakening the public OpenAPI 3.1 contract to fit a generator's external-reference limitation. Accepted 2026-08-23.

## ADR-022: Queue complete typed envelopes and represent empty manifests without chunks

The durable local queue stores complete versioned `EventEnvelope` documents, not private payload fragments requiring an implicit transport translation. The canonical schema applies an exact closed payload shape selected by event type. Manifest chunks carry the ADR-017 layered entry shape in strict unique path order. Go and TypeScript share the explicit NUL-delimited path/layer/status/old-path SHA-256 encoding documented in `protocol.md`; JSON serialization is not the hash input. An empty current snapshot uses `chunkCount: 0`, no chunk events, and a normal hash-checked completion so a workstream can atomically clear previous paths. Event batches contain one workspace because sequence and acknowledgement cursors are workspace-scoped. Accepted 2026-08-23 after L1/L2 integration review exposed the otherwise incompatible queue, hash, and empty-snapshot cases.

## ADR-023: Existing devices mint Project-scoped dashboard tickets

Enrollment may return an initial one-time dashboard ticket, but the Project creator and later browser activations also require a safe path. Add authenticated `POST /v1/dashboard-tickets` with an explicit Project ID. The backend derives current device membership, stores only the ticket hash, returns the short-lived secret once, and preserves the existing unauthenticated single-use exchange into an HTTP-only session. Browsers never receive or store the device credential, and the dashboard never accepts a ticket through a page input or URL. Accepted 2026-08-23 when L4 create-to-activation integration exposed the missing creator path.

## ADR-024: Device-initiated activation POSTs tickets into same-origin browser sessions

The `stickguy dashboard --project` command mints a Project-scoped ticket from the Keychain-backed device credential, opens an unpredictable loopback-only handoff page, and renders the ticket only as an escaped hidden form value. The browser submits it by top-level POST to `/v1/dashboard-activations`; the hosted boundary consumes it once, sets a Secure/HttpOnly/SameSite=Strict session cookie, and redirects without putting the ticket in a URL, page input, process argument, browser storage, or retained evidence. Browser session and Project snapshot reads reauthorize the hashed session and its single Project on every request. Production serves the dashboard from the same origin; the loopback redirect is validation-only. Accepted 2026-08-23 during L4 vertical-slice integration.
