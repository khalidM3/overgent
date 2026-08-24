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

V1 does not collect source content, raw transcripts, system prompts, or environment values. Redaction cannot make unnecessary collection safe. Accepted 2026-08-23; narrowed only for bounded opt-in visible conversation events by ADR-027.

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

## ADR-025: Use one official-SDK stdio lifecycle bridge with trust-preserving project adapters

Expose the seven Stickguy lifecycle tools from the Go executable through the pinned official Model Context Protocol Go SDK `v1.7.0`. Codex and Claude Code launch the same local stdio bridge; neither adapter gains a coding loop, file tools, shell execution, Git mutation, or agent-permission authority. The bridge resolves an explicit registered workspace or one unique canonical current-directory match and fails closed on zero or multiple matches. Lifecycle mutations are durable, revision-checked, and idempotent before hosted delivery; unavailable hosted context returns an explicit degraded result without losing the local report.

Codex setup owns one exact marked table in trusted project `.codex/config.toml`, following the [official Codex MCP configuration](https://developers.openai.com/codex/mcp). Claude setup structurally merges one `stickguy` stdio entry in project `.mcp.json`, following the [official Claude Code MCP scope contract](https://code.claude.com/docs/en/mcp), and surfaces Claude's required one-time project approval. Default-root setup writes the portable PATH command `stickguy mcp`; an explicitly isolated config root writes the requested absolute executable/state paths. Both refuse drift and preserve unrelated configuration on removal. No adapter writes credentials into agent configuration. Hooks remain disabled and `available_but_unverified` under ADR-016; MCP plus Git/manual fidelity remains the selected fallback. Accepted 2026-08-23; credentialed external-model smoke remains a separate explicit-consent verification step.

## ADR-026: Withhold coding-agent setup after the L5 real-client gate narrows

Retain the official-SDK lifecycle bridge, durable local operations, configuration merge/removal implementations, and conformance fixtures, but do not enable production `setup codex` or `setup claude`. In an explicitly approved disposable smoke, Codex CLI `0.148.0-alpha.15` discovered all seven tools and issued the complete eight-call lifecycle, including an exact checkpoint retry; every argument key and value matched the synthetic fixture. Codex nevertheless returned a generic MCP failure for every invocation and no Codex request reached durable service storage. The same built bridge connected through the official SDK, read a brief, and durably published an intent against the same running service. Read-only and bounded workspace-write Codex runs produced the same outcome. This isolates the failure to the current Codex client execution boundary without enough non-sensitive diagnostics to select a protocol change safely.

Claude Code `2.1.197` parses the project-scoped stdio configuration, but its installed client is unauthenticated, so a model-driven tool smoke is unavailable rather than passed. Do not infer support from configuration parsing. Continue deterministic Git/manual fidelity under ADR-016, keep hooks disabled, and require a focused vendor-client compatibility spike before re-enabling setup. Accepted as a narrow outcome 2026-08-23.

## ADR-027: Permit explicit opt-in agent activity sharing without expanding the coding harness

Add optional `activity` and `conversation` sharing profiles above the default privacy-minimized `coordination` profile. Installation and Project enrollment do not enable them. A Project owner must make a profile available and each member must independently opt in for their workspace/session; the narrower setting wins. Activity may disclose lifecycle, visible plan/progress, tool category/status, safe paths, permission-needed state, and bounded verification outcomes. Conversation sharing may disclose bounded user-authored prompts and visible assistant messages as individually classified events, but never ingests or uploads transcript files.

Source/diffs, Git objects, file/tool-result content, raw commands/output, environment values, `.env` variants, credentials/tokens, protected credential paths, vendor/organization/developer/system prompts, and hidden reasoning remain prohibited under every profile. Candidate vendor events may exist transiently in local memory only until policy projection; prohibited fields are discarded before durable storage or enqueue. Pause/downgrade is synchronous, consent is versioned and revocable, exact shared data is inspectable/deletable, and Project authorization/retention applies. Stickguy remains an observer around vendor-owned sessions and does not start/steer model loops or execute/approve tools. Production contracts wait for the isolated L5A gate in `agent-activity-sharing.md`; until it passes or narrows, current adapters remain withheld under ADR-026. Accepted by the owner 2026-08-23; supersedes ADR-006, ADR-016, and ADR-026 only to authorize this bounded validation and a later reviewed opt-in observation contract.

## ADR-028: Accept Claude activity hooks and narrow Codex App Server observation

An authenticated Claude Code `2.1.197` synthetic run proves project-configured command hooks for session, visible prompt submission, tool pre/post, subagent, stop, and session-end lifecycle while Read, Bash, and Agent execute. Session persistence was disabled, hook inputs were discarded, and isolated structural configuration tests preserve unrelated settings/permissions and refuse drift. Accept this supported hook surface for a future opt-in `activity` adapter; permission-denial and partial-message fidelity remain unavailable until separately proved. The adapter observes project-configured Claude sessions and does not require Stickguy to own the model loop.

Codex CLI `0.148.0-alpha.15` App Server proves bounded enumeration of existing tasks by exact working directory and structured turn/item reads without transcript-file parsing. It does not prove subscription to another already-running App Server process. Narrow Codex to bounded refresh/read where supported and retain MCP/Git/manual as the realtime fallback. Unknown vendor events fail closed.

The local projector and sink boundary prove owner/member consent precedence, preview without side effects, synchronous pause/downgrade, Project isolation, bounded retention/deletion, and rejection before storage/send for protected paths, tokens, transcript/system/reasoning/source/diff/tool-result/raw-command/output candidates. This concludes L5A as NARROW and permits reviewed shared-contract implementation only for the accepted surfaces. It does not enable collection by itself. System prompts, hidden reasoning, source, transcripts, environment values, and credentials remain prohibited regardless of broad local read permission. Accepted by the owner-approved live validation 2026-08-24.

## ADR-029: Pull an exact-pinned macOS desktop preview before L6

Native demand is now owner-demonstrated, satisfying ADR-005's condition for reuse of the React dashboard in a desktop shell. Add L5B before L6: a labeled macOS preview embeds the existing dashboard, persists in the system menu bar, and controls health, pause/resume-all, scan, open, and quit through the existing current-user mode-0600 Unix socket. It does not start a second service or local web server, and its window remains fixture-backed until desktop authentication and hosted state are deliberately integrated.

The required tray API is available in Wails `v3.0.0-beta.12`, not stable Wails v2.15.0. Keep the exact beta in a separate `apps/desktop` Go module so the root Go core remains pure-Go and Wails/CGO do not enter the service dependency graph. This is a preview boundary, not a signed, notarized, cross-platform, updater, or release-support claim. The hosted browser remains the fallback; L8 must re-evaluate Wails stability and perform native-runner distribution gates. Accepted by the owner after an isolated native macOS arm64 spike on 2026-08-24.

## ADR-030: Start semantic coordination with a deterministic concept provider and quiet radar delivery

Use the public `EmbeddingProvider` boundary with `stickguy-concepts/v1`, a deterministic 32-dimensional normalized concept-vector adapter, and the existing Convex vector index behind the portable `SemanticIndex` contract. This avoids adding a third-party model credential or sending approved summaries to another provider while the public corpus is small. Exact structural and lexical signals remain independently operational; vector similarity is only candidate evidence. Semantic/lexical fusion may create explained findings in the radar and briefs, but L6 adds no proactive semantic interruption channel.

The adapter is deliberately narrow: it recognizes the versioned public coordination vocabulary and does not claim general language understanding. Provider/index failure retries once, persists visible degraded fidelity, and falls back to current structural routing. A later managed or self-hosted provider must pass the expanded public and private precision/cost/privacy gates behind the same interfaces; it does not require a protocol replacement. Accepted 2026-08-24 after the L6 public corpus passed its labeled positive, negative, isolation, routing, budget, stale-state, adversarial-input, and outage cases.

## ADR-031: Add a loopback-only local dogfood profile before L7

Add a development profile that composes the existing public components without
weakening their production boundaries. Vite supplies React hot reload to a
separately identified `Stickguy Dev.app`; local Convex remains loopback-only;
one default-profile Go service may hot-register distinct linked-worktree roots;
and the development desktop exchanges a one-time dashboard ticket inside its
own webview. The ticket remains server-side until form POST and never enters a
URL, JavaScript, or browser storage. Production builds ignore the development
URL and do not expose local activation.

Permit `setup codex|claude --development` only as an explicit local dogfood
action. It installs the already-reviewed project MCP configuration with the
absolute development binary and no hooks, transcript parsing, agent control, or
optional activity collection. Claude may provide reported lifecycle fidelity;
Codex remains `mcp_with_git_fallback` because current-client durable delivery is
still narrowed by ADR-026. Linked worktrees provide distinct workstream
attribution while retaining one repository fingerprint and one per-user
service. Accepted by the owner 2026-08-24 before L7.

## ADR-032: Make the repository the native onboarding anchor

The normal development workflow begins in `Stickguy Dev.app`, not with a
per-chat command. The user chooses one canonical Git repository and creates or
joins a Project. Git observation starts from that registered root regardless of
which editor or coding agent changes it. Detected Codex and Claude Code adapters
are explicit opt-ins that structurally add only the reviewed Project MCP entry;
existing sessions must restart to discover it. Missing, declined, or narrowed
adapters retain deterministic Git fidelity.

One working tree cannot honestly attribute a filesystem change to a particular
process. For local Codex-versus-Claude attribution, the desktop accepts existing
distinct linked worktrees, validates their shared Git common directory, hot
registers separate workstreams through the one-service boundary, and configures
only the selected agent in each root. Stickguy does not create, switch, remove,
or otherwise mutate worktrees. Claude Code CLI is supported; generic Claude
Desktop sessions do not currently expose a repository-bound lifecycle surface.

Development Vite is proxied through Wails using its documented
`FRONTEND_DEVSERVER_URL` path so the native runtime remains available during hot
reload. Dashboard activation POSTs through the loopback Vite `/api` proxy so
the HttpOnly development cookie is same-origin with the live UI. Production
onboarding, multiple Projects/devices in one local profile, distribution, and
updating remain L8 gates. Accepted by the owner 2026-08-24 before L7.
