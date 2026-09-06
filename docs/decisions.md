# Overgent — Architecture Decision Log

## ADR-001: Project is the persistent container

Use `Project`; sessions/presence are time-bounded children. History, membership, plans, and decisions outlive a live gathering. Accepted 2026-08-23.

## ADR-002: Go is the permanent local-core language

CLI, service, watchers, Git, storage, sync, MCP, and updater use Go. It gives standalone distribution, Tier 1 MCP, simple concurrency, approachable OSS, and sufficient I/O performance. Rust is not a planned migration; add only for measured subsystem need. Accepted 2026-08-23.

## ADR-003: One service per user

One service owns state/watchers; CLI and MCP are clients. This avoids duplicate processes, ports, queues, and conflicting state. Accepted 2026-08-23.

## ADR-004: Convex hosts coordination state

Use Convex for realtime state, transactions, HTTP actions, and schedules. Go speaks only the Overgent `/v1` contract; IDs remain vendor-neutral. Accepted 2026-08-23.

## ADR-005: Hosted dashboard first, Wails later

CLI opens hosted React for alpha. Reuse it inside Wails when native demand is demonstrated. Accepted 2026-08-23.

## ADR-006: Data minimization is the privacy boundary

V1 does not collect source content, raw transcripts, system prompts, or environment values. Redaction cannot make unnecessary collection safe. Accepted 2026-08-23; narrowed only for bounded opt-in visible conversation events by ADR-027.

## ADR-007: At-least-once event delivery

Use SQLite queue, stable IDs, server dedupe, acknowledgements. Intermittent devices do not require exactly-once transport when effects are idempotent. Accepted 2026-08-23.

## ADR-008: Backend is canonical for decisions

Deliver via API/MCP and optional untracked context, not a shared tracked append file that creates Git collisions. Accepted 2026-08-23.

## ADR-009: Apache-2.0 open-source application with managed cloud

Publish the installed Go client, adapters, protocols, installers, release workflows, dashboard, and core backend authorization/retention/coordination code under the unmodified Apache License 2.0. Keep production operations, billing, private admin/abuse systems, runbooks, and private eval data in a separate private repository. Use `Copyright 2026 Overgent contributors` as the initial `NOTICE` attribution; a later legal entity or rights assignment requires an explicit notice review rather than a template substitution. Accepted by the owner 2026-08-23.

## ADR-010: Coordination intelligence is a V1 capability

V1 combines deterministic structural evidence with semantic retrieval over bounded, synchronized intent/change/plan/decision summaries. Semantic results are probabilistic candidates with provenance; they never replace deterministic evidence. Accepted 2026-08-23.

## ADR-011: Publish live manifests; do not poll peer worktrees

Each client reports its own baseline-to-current manifest and coordination summaries. The hosted project service compares members in realtime. Pulling/fetching peer Git state is optional checkpoint evidence only because it misses unpushed work and creates repository/network side effects. Accepted 2026-08-23.

## ADR-012: Hosted semantic index behind portable interfaces

Managed V1 uses provider-neutral embedding/adjudication interfaces and a `SemanticIndex` adapter backed initially by Convex vector search. Only approved coordination summaries reach providers; source and diffs remain prohibited. TurboVec is a future local/self-hosted adapter only when measured need justifies its packaging cost. Accepted 2026-08-23.

## ADR-013: Overgent is a coordination harness, not a coding harness

Existing products retain their model loops, coding tools, execution permissions, test execution, context compression, and model routing. Overgent owns cross-agent observation, shared project memory, relevance routing, advisory coordination, and delivery acknowledgement. Accepted 2026-08-23.

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

The `overgent dashboard --project` command mints a Project-scoped ticket from the Keychain-backed device credential, opens an unpredictable loopback-only handoff page, and renders the ticket only as an escaped hidden form value. The browser submits it by top-level POST to `/v1/dashboard-activations`; the hosted boundary consumes it once, sets a Secure/HttpOnly/SameSite=Strict session cookie, and redirects without putting the ticket in a URL, page input, process argument, browser storage, or retained evidence. Browser session and Project snapshot reads reauthorize the hashed session and its single Project on every request. Production serves the dashboard from the same origin; the loopback redirect is validation-only. Accepted 2026-08-23 during L4 vertical-slice integration.

## ADR-025: Use one official-SDK stdio lifecycle bridge with trust-preserving project adapters

Expose the seven Overgent lifecycle tools from the Go executable through the pinned official Model Context Protocol Go SDK `v1.7.0`. Codex and Claude Code launch the same local stdio bridge; neither adapter gains a coding loop, file tools, shell execution, Git mutation, or agent-permission authority. The bridge resolves an explicit registered workspace or one unique canonical current-directory match and fails closed on zero or multiple matches. Lifecycle mutations are durable, revision-checked, and idempotent before hosted delivery; unavailable hosted context returns an explicit degraded result without losing the local report.

Codex setup owns one exact marked table in trusted project `.codex/config.toml`, following the [official Codex MCP configuration](https://developers.openai.com/codex/mcp). Claude setup structurally merges one `overgent` stdio entry in project `.mcp.json`, following the [official Claude Code MCP scope contract](https://code.claude.com/docs/en/mcp), and surfaces Claude's required one-time project approval. Default-root setup writes the portable PATH command `overgent mcp`; an explicitly isolated config root writes the requested absolute executable/state paths. Both refuse drift and preserve unrelated configuration on removal. No adapter writes credentials into agent configuration. Hooks remain disabled and `available_but_unverified` under ADR-016; MCP plus Git/manual fidelity remains the selected fallback. Accepted 2026-08-23; credentialed external-model smoke remains a separate explicit-consent verification step.

## ADR-026: Withhold coding-agent setup after the L5 real-client gate narrows

Retain the official-SDK lifecycle bridge, durable local operations, configuration merge/removal implementations, and conformance fixtures, but do not enable production `setup codex` or `setup claude`. In an explicitly approved disposable smoke, Codex CLI `0.148.0-alpha.15` discovered all seven tools and issued the complete eight-call lifecycle, including an exact checkpoint retry; every argument key and value matched the synthetic fixture. Codex nevertheless returned a generic MCP failure for every invocation and no Codex request reached durable service storage. The same built bridge connected through the official SDK, read a brief, and durably published an intent against the same running service. Read-only and bounded workspace-write Codex runs produced the same outcome. This isolates the failure to the current Codex client execution boundary without enough non-sensitive diagnostics to select a protocol change safely.

Claude Code `2.1.197` parses the project-scoped stdio configuration, but its installed client is unauthenticated, so a model-driven tool smoke is unavailable rather than passed. Do not infer support from configuration parsing. Continue deterministic Git/manual fidelity under ADR-016, keep hooks disabled, and require a focused vendor-client compatibility spike before re-enabling setup. Accepted as a narrow outcome 2026-08-23.

## ADR-027: Permit explicit opt-in agent activity sharing without expanding the coding harness

Add optional `activity` and `conversation` sharing profiles above the default privacy-minimized `coordination` profile. Installation and Project enrollment do not enable them. A Project owner must make a profile available and each member must independently opt in for their workspace/session; the narrower setting wins. Activity may disclose lifecycle, visible plan/progress, tool category/status, safe paths, permission-needed state, and bounded verification outcomes. Conversation sharing may disclose bounded user-authored prompts and visible assistant messages as individually classified events, but never ingests or uploads transcript files.

Source/diffs, Git objects, file/tool-result content, raw commands/output, environment values, `.env` variants, credentials/tokens, protected credential paths, vendor/organization/developer/system prompts, and hidden reasoning remain prohibited under every profile. Candidate vendor events may exist transiently in local memory only until policy projection; prohibited fields are discarded before durable storage or enqueue. Pause/downgrade is synchronous, consent is versioned and revocable, exact shared data is inspectable/deletable, and Project authorization/retention applies. Overgent remains an observer around vendor-owned sessions and does not start/steer model loops or execute/approve tools. Production contracts wait for the isolated L5A gate in `agent-activity-sharing.md`; until it passes or narrows, current adapters remain withheld under ADR-026. Accepted by the owner 2026-08-23; supersedes ADR-006, ADR-016, and ADR-026 only to authorize this bounded validation and a later reviewed opt-in observation contract.

## ADR-028: Accept Claude activity hooks and narrow Codex App Server observation

An authenticated Claude Code `2.1.197` synthetic run proves project-configured command hooks for session, visible prompt submission, tool pre/post, subagent, stop, and session-end lifecycle while Read, Bash, and Agent execute. Session persistence was disabled, hook inputs were discarded, and isolated structural configuration tests preserve unrelated settings/permissions and refuse drift. Accept this supported hook surface for a future opt-in `activity` adapter; permission-denial and partial-message fidelity remain unavailable until separately proved. The adapter observes project-configured Claude sessions and does not require Overgent to own the model loop.

Codex CLI `0.148.0-alpha.15` App Server proves bounded enumeration of existing tasks by exact working directory and structured turn/item reads without transcript-file parsing. It does not prove subscription to another already-running App Server process. Narrow Codex to bounded refresh/read where supported and retain MCP/Git/manual as the realtime fallback. Unknown vendor events fail closed.

The local projector and sink boundary prove owner/member consent precedence, preview without side effects, synchronous pause/downgrade, Project isolation, bounded retention/deletion, and rejection before storage/send for protected paths, tokens, transcript/system/reasoning/source/diff/tool-result/raw-command/output candidates. This concludes L5A as NARROW and permits reviewed shared-contract implementation only for the accepted surfaces. It does not enable collection by itself. System prompts, hidden reasoning, source, transcripts, environment values, and credentials remain prohibited regardless of broad local read permission. Accepted by the owner-approved live validation 2026-08-24.

## ADR-029: Pull an exact-pinned macOS desktop preview before L6

Native demand is now owner-demonstrated, satisfying ADR-005's condition for reuse of the React dashboard in a desktop shell. Add L5B before L6: a labeled macOS preview embeds the existing dashboard, persists in the system menu bar, and controls health, pause/resume-all, scan, open, and quit through the existing current-user mode-0600 Unix socket. It does not start a second service or local web server, and its window remains fixture-backed until desktop authentication and hosted state are deliberately integrated.

The required tray API is available in Wails `v3.0.0-beta.12`, not stable Wails v2.15.0. Keep the exact beta in a separate `apps/desktop` Go module so the root Go core remains pure-Go and Wails/CGO do not enter the service dependency graph. This is a preview boundary, not a signed, notarized, cross-platform, updater, or release-support claim. The hosted browser remains the fallback; L8 must re-evaluate Wails stability and perform native-runner distribution gates. Accepted by the owner after an isolated native macOS arm64 spike on 2026-08-24.

## ADR-030: Start semantic coordination with a deterministic concept provider and quiet radar delivery

Use the public `EmbeddingProvider` boundary with `overgent-concepts/v1`, a deterministic 32-dimensional normalized concept-vector adapter, and the existing Convex vector index behind the portable `SemanticIndex` contract. This avoids adding a third-party model credential or sending approved summaries to another provider while the public corpus is small. Exact structural and lexical signals remain independently operational; vector similarity is only candidate evidence. Semantic/lexical fusion may create explained findings in the radar and briefs, but L6 adds no proactive semantic interruption channel.

The adapter is deliberately narrow: it recognizes the versioned public coordination vocabulary and does not claim general language understanding. Provider/index failure retries once, persists visible degraded fidelity, and falls back to current structural routing. A later managed or self-hosted provider must pass the expanded public and private precision/cost/privacy gates behind the same interfaces; it does not require a protocol replacement. Accepted 2026-08-24 after the L6 public corpus passed its labeled positive, negative, isolation, routing, budget, stale-state, adversarial-input, and outage cases.

## ADR-031: Add a loopback-only local dogfood profile before L7

Add a development profile that composes the existing public components without
weakening their production boundaries. Vite supplies React hot reload to a
separately identified `Overgent Dev.app`; local Convex remains loopback-only;
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

The normal development workflow begins in `Overgent Dev.app`, not with a
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
only the selected agent in each root. Overgent does not create, switch, remove,
or otherwise mutate worktrees. Claude Code CLI is supported; generic Claude
Desktop sessions do not currently expose a repository-bound lifecycle surface.

Development Vite is proxied through Wails using its documented
`FRONTEND_DEVSERVER_URL` path so the native runtime remains available during hot
reload. Dashboard activation POSTs through the loopback Vite `/api` proxy so
the HttpOnly development cookie is same-origin with the live UI. Production
onboarding, multiple Projects/devices in one local profile, distribution, and
updating remain L8 gates. Accepted by the owner 2026-08-24 before L7.

## ADR-033: Make supported agent sessions first-class repo-scoped workstreams

The owner-demonstrated product requirement is seamless session awareness: after
a member selects a repository and explicitly connects Codex or Claude Code, new
supported sessions opened anywhere under that repository appear automatically.
They do not need to call an Overgent tool, create a branch, or use a distinct
worktree. A linked worktree remains an optional Git-isolation technique, not an
attribution prerequisite.

Use current documented project-local lifecycle hooks as passive observation
surfaces. Codex and Claude Code both expose session, prompt-boundary, tool,
permission, subagent, stop, and session-end events. Install exact managed hook
groups alongside the existing Project MCP entry, preserve unrelated settings,
refuse drift, run observation asynchronously where the vendor permits, and
never return a decision that controls agent execution. Map each vendor session
to a stable hashed Overgent workstream scoped by the registered repository;
vendor session and transcript identifiers remain local and are never sent.

The initial `activity/v1` projection shares lifecycle/status, a bounded
Overgent-generated action label, allowlisted tool name, hashed subagent alias
and type, and safe repository-relative affected paths. It rejects protected or
escaping paths as whole events and discards tool input/output, prompt text,
source/diffs, raw commands/output, transcript paths, system/developer prompts,
reasoning, and environment values before durable storage. Selecting an adapter
during Project enrollment is the member's explicit activity opt-in; later
multi-member controls must retain owner availability plus member consent and
the narrower setting wins.

Agent safe-path sets are independent even in one checkout. Overlap between two
active agent-session workstreams creates a deterministic, evidence-backed
finding. Git remains the authoritative combined repository observation and the
fallback when a vendor event does not expose a path. Existing sessions must be
restarted once after hook installation; unsupported hosted/vendor tool paths
degrade visibly rather than being inferred from process memory or transcript
files. This supersedes ADR-028's Codex hook narrowing and ADR-032's linked-
worktree requirement for local agent attribution while preserving the
coordination-harness and always-prohibited-data boundaries. Accepted by the
owner 2026-08-24 after current official surface review and loopback live proof.

## ADR-034: Add explicit, previewed Project session sharing

Add `session-share/v1` as a per-session, member-controlled disclosure layer on
top of ADR-033 activity. It is disabled by default and never follows merely from
installing Overgent, joining a Project, connecting an adapter, or enabling safe
activity. Before enabling, the member sees a side-effect-free local preview and
chooses `self` or authorized `Project` audience, exact message kinds, and an
expiry. The consent record is versioned; an adapter or schema expansion requires
new consent. Disable stops new enqueue synchronously, and the member or Project
owner can delete already-shared messages.

The allowed projection is bounded user-authored prompts, visible assistant
messages, vendor-exposed reasoning summaries, and explicitly surfaced system
instructions. Each candidate is classified independently before durable local
storage. Any `.env` reference or content, environment value, credential, token,
cookie, private key, protected credential path, source/diff-like content, raw
tool input/result, command/output, transcript path/file, binary data, scanner
failure, unknown kind, or oversize content rejects the whole candidate. Redaction
does not turn prohibited data into allowed data. Overgent may display only what
the vendor-supported event exposes; it does not scrape transcript stores,
inspect process memory, infer hidden chain of thought, or claim unavailable
reasoning fidelity.

Hosted authorization derives the owning member and Project from the authenticated
device/session, enforces the exact active consent version and audience on every
write and read, applies bounded retention, and records deletion. This supersedes
ADR-006, ADR-027, and ADR-028 only for this explicitly consented projection; all
other privacy and coordination-harness boundaries remain. Accepted by the owner
2026-08-24 after explicitly requesting Project-shareable session details while
retaining `.env` and credential exclusion.

## ADR-035: Separate member identity from device and account identifiers

Live-work identity is a member-controlled display name stored per Project on the
member record, not the enrolling device label and not an email address. Members
choose it from Settings; the hosted boundary rejects names containing `@` so a
contact address never becomes the name teammates see, and rejects control
characters and names outside 2–60 characters.

`members.displayNameSource` records whether the current name was seeded from the
device (`device`) or chosen by the member (`member`). Rows written before this
ADR have no source and are read as `device`, which is the migration: existing
Projects keep working and rendering their current name, and the dashboard asks
that member once to choose their own. No hostname is rewritten or guessed on
their behalf, because a silent rename would change attribution on decisions and
plan items that teammates have already read.

Device names remain, but only as security surface under Settings → Devices &
security, where they identify hardware for revocation and audit. Renaming a
member bumps the Project scopes so briefs and rendered coordination items
re-read the name rather than keeping a stale device-derived label.

Live agent sessions also carry the real checked-out branch. Overgent reads it
from the registered worktree with `git symbolic-ref --short HEAD` rather than
trusting an adapter-reported value; a detached HEAD or a slow read reports no
branch instead of failing, and the name is validated as a plain branch name at
both the local and hosted boundaries. A branch name is coordination metadata and
carries none of the always-prohibited repository content. Accepted by the owner
2026-08-24 as part of beta identity readiness.

## ADR-036: Read the local session transcript for owner display and consented sharing

Supersedes the ADR-034 prohibition on reading vendor transcript stores, and the
`AGENTS.md` rule that listed raw transcript files as never-collect, for one
bounded purpose. Hook payloads do not carry assistant text, reasoning, or system
instructions; only `UserPromptSubmit` carries content. Session detail was
therefore structurally empty, and no amount of hook work could fix it.

Overgent may now read the vendor transcript file named by the supported hook's
own `transcript_path` for a session running in a registered repository on this
device. The file is read locally, bounded from the tail, and never copied into a
second durable store; Overgent records only the path, the session title, and the
branch. Because the content stays on the machine that produced it, the session
owner always sees their own session in full without enabling any sharing. This
is the point of the change: a member must be able to see exactly what they would
be sharing before they decide to share it.

Projection to other members remains opt-in, per session, previewed, versioned,
and revocable exactly as ADR-034 defines. Shared candidates are classified
before leaving the device. Fenced code and quoted source in a conversation are
now allowed, because an agent conversation is unreadable without them and the
member has explicitly chosen to share that conversation; this narrows ADR-006's
blanket source prohibition to unattended collection rather than consented
conversation sharing. Never shared, in any mode: `.env` content or references,
environment values, credentials, tokens, cookies, private keys, protected
credential paths, raw tool results, command output, and attachments. Raw
`tool_result` parts are dropped during parsing and never become candidates.

`thinking` parts are vendor-recorded reasoning that the vendor itself persisted.
They are treated as content, not as hidden chain of thought inferred by
Overgent, and they are shared only under the same explicit consent. Where a
vendor records no reasoning, Overgent shows none and claims none. Accepted by
the owner 2026-08-25 after confirming that hook-only session detail cannot
show a usable session.

## ADR-037: Narrow the product to collision coordination

Remove shared plan items and advisory path claims. Planning is a human process
that teams already run elsewhere, and a plan surface inside Overgent competed
with the actual value: seeing what every agent session is doing and catching
collisions before they land. Their tables, contracts, MCP tools, and dashboard
surfaces are deleted rather than hidden, so no partially maintained surface
remains.

Collision resolution stays. A collision opens a sync card; members discuss it;
resolving it records the outcome and delivers it once, idempotently, into the
brief of every affected agent session. What is removed is the standalone
"Decisions" product surface and durable Project decision log, not the delivery
of a resolution to the agents that need it. Accepted by the owner 2026-08-25.

## ADR-038: Classify the material, not the mention

Narrows ADR-036's content rules. Treating the string `.env` as prohibited data
rejected ordinary sentences — "check the .env file before running" — while
disclosing nothing, and agents discuss configuration files constantly. Naming a
file is not disclosing its contents.

A candidate is now rejected for the material itself: environment assignments
(`NAME=value`, including `export`), credential and token patterns, private key
blocks, raw tool results, and command output at line start. An actual `.env`
file pasted into a conversation is still rejected, because its contents *are*
assignments. Rejection remains whole-message; nothing is redacted, so a partial
scrub can never be mistaken for a safe message.

This does not widen what may be shared. Sharing is still off by default, still
per-session, previewed, versioned, and revocable, and every secret class named
in ADR-036 remains prohibited. Accepted by the owner 2026-08-25 after the
filename rule was observed eating harmless lines.

## ADR-039: Per-vendor session record adapters

Codex and Claude Code record sessions differently, so one parser cannot serve
both. `sessiontranscript` detects the format and dispatches to a vendor adapter;
an unrecognized file yields no content rather than a guess.

Claude Code writes one message record per line with the session's fields at the
top level, including its own generated or member-set title and the branch.

Codex writes a rollout file of typed envelopes. Conversation is taken from the
`event_msg` stream, which is what Codex shows its own user. The `response_item`
stream is raw model I/O that also carries injected context and tool framing, so
it contributes only tool names and the operating-instruction turns; reading it
as conversation would present machine-injected text as something a person wrote.
The `reasoning` payload's `encrypted_content` is vendor-held hidden reasoning
and is never read, which keeps the ADR-036 promise that Overgent shows only what
a vendor surfaced. Codex records no title, so a session is labelled by its
opening request, skipping the machine-written preamble of a resumed or compacted
session; where no usable label exists the vendor and alias are shown instead.

Codex does not pass a transcript path to its hooks but names every rollout after
the session id, so the file is located from the id the hook already sends. That
id is used locally for the lookup and is never published; only a UUID is
accepted, so it cannot introduce a path separator. Records above a bounded size
are skipped, since inline image data carries no readable conversation. Accepted
by the owner 2026-08-25.

## ADR-040: Managed OpenAI embeddings with deterministic fallback

Keep the public `EmbeddingProvider` contract and retain
`overgent-concepts/v1` as an offline, deterministic fallback. The managed
provider is OpenAI `text-embedding-3-large`, requested at 1024 dimensions and
called only from a hosted asynchronous action after semantic text has passed
the existing bounded policy. The API key is a hosted deployment secret; it is
never available to the local core, dashboard, agent configuration, logs, or
Project records.

Embedding work is revision-scoped and scheduled after the coordination object
is durable, so provider latency/outage cannot delay manifests, activity, or
checkpoints. A late response is discarded when the object was revised or
deactivated. Failure marks semantic processing degraded and preserves the
structural and deterministic paths. Vector results remain candidate evidence;
they do not enable proactive interruption without the existing precision gate.
The vector index expands from the early 32-dimension fixture shape to 1024
dimensions; deployments must apply that schema migration before enabling the
provider. Accepted by the owner 2026-08-25.

## ADR-041: Add an isolated HTTPS shared-development profile

Superseded by ADR-074 and retired in Lane 06: a profile now binds each Project
to its own backend, so a team Project sits beside a local one on the ordinary
development profile and `pnpm dev:shared` no longer exists. The record below is
kept for the reasoning that led here.

The owner requires a real two-member dogfood before signed production
distribution. Add `pnpm dev:shared` as an explicit development profile that
uses the same Go service, Convex functions, dashboard, enrollment, and adapter
code against a configured HTTPS Convex HTTP-actions origin. It never accepts
remote plaintext HTTP, keeps the dashboard on loopback through the reviewed
same-origin proxy, and uses a separate per-user configuration root so local
loopback credentials and Projects cannot be mixed with shared-development
state. The ordinary `pnpm dev` profile remains anonymous and loopback-only.

This is a beta verification path, not a production deployment claim. Each Mac
still runs its own one-user service and Keychain-backed device credential;
members join through the existing expiring invite and publish only bounded
coordination records. Accepted by the owner 2026-08-25 when requesting the
complete two-person dogfood path.

## ADR-042: A safe vendor-visible session title may seed automatic intent

When a member explicitly connects a supported Project adapter, the already
disclosed `activity/v1` session title may seed that session's current intent.
Before it leaves the device, a new title classifier normalizes it, enforces 160
characters, and rejects credentials, environment assignments, private keys,
raw tool output markers, invalid text, and oversize values. Hosted semantic
policy runs again before storage or embedding; rejection drops only the derived
intent and never blocks the underlying coding-agent event.

The derived object is labeled `hook-derived-title/v1`. If the Project has a
managed embedding provider, the approved title may be sent to it under the same
retention and deletion rules as an MCP/manual intent. No other prompt,
transcript message, source, diff, tool payload, command, or output becomes
semantic input. This gives passive sessions a useful early trajectory while
preserving honest fidelity and the coordination-harness boundary. Accepted by
the owner 2026-08-25 as part of completing automatic traffic-control dogfood.

## ADR-043: Make agent bindings profile-aware and runtime-verified

Codex and Claude Code adapter readiness has two independent dimensions:
configuration ownership and observed runtime delivery. A managed MCP/hook entry
is classified as `current`, `partial`, `other_profile`, `not_configured`, or
`drifted`. A partial current-profile installation may be repaired
automatically. A structurally recognized binding to another Overgent profile
must be shown explicitly and requires a member-confirmed **Reconnect to this
Project** action. Unknown or conflicting managed-looking configuration remains
fail-closed and is never overwritten automatically.

Reconnect snapshots both provider files, replaces only Overgent's recognized
MCP entry and activity hooks, preserves unrelated settings and hooks, and
restores both snapshots if either managed update fails. The preview identifies
the old and new local profiles before mutation. The CLI exposes the same
operation as `setup reconnect` for recovery without the desktop UI.

A configured adapter is not presented as live-ready until the current local
profile records an accepted event from that vendor in the enrolled workspace.
Until then, the UI says that a provider restart and a new task in the repository
are required. Runtime evidence is cleared on reconnect and stored only as local
workspace/vendor/timestamp metadata; no prompt, source, diff, command, output,
or credential is added. Accepted by the owner 2026-08-25 after a shared-profile
dogfood run exposed a valid Codex binding left on the loopback profile.

## ADR-044: Relocate the privacy boundary from abstention to the device

Supersedes ADR-006's collection-abstention model. The local Go service may read
anything on the member's machine that serves coordination: source files, diffs,
Git objects, and vendor transcripts. The privacy boundary is what crosses the
wire, not what the local process reads. Synced data is limited to derived,
structured coordination facts: contract fingerprints (exported symbol names and
signature hashes), bounded diff summaries, intents, dependency claims, path
manifests, and finding evidence. Raw source, raw diffs, environment values,
credentials, and secrets never sync; the existing secret classifier guards the
wire as a hard gate, not a consent feature.

Rationale: every high-value coordination signal — stale contracts, semantic
duplication, architectural conflict — lives in code content. Abstention-based
privacy starved the intelligence while providing a weaker story than
local-first analysis with bounded sync, where raw material stays on the device
by architecture. Accepted by the owner 2026-08-25.

## ADR-045: An LLM is the judgment layer; determinism is the evidence layer

Deterministic signals (path overlap, contract-fingerprint drift, manifest
state) remain the trigger layer and always operate offline. A hosted LLM
(Anthropic Claude, called only from the hosted service like the ADR-040
embedding provider) becomes the judgment layer: it summarizes diff facts,
compares workstream trajectories for duplication and architectural conflict,
adjudicates whether a candidate finding is worth an interruption, and renders
briefs in language a receiving agent can act on. LLM outage degrades to
deterministic findings and dashboard delivery, never to silence about
deterministic evidence. `overgent-concepts/v1` is demoted to a test fixture.
Proactive interruption requires passing the M1 eval-harness precision gate.
Accepted by the owner 2026-08-25.

## ADR-046: Coordination context is pushed into agent turns via hooks

Supersedes the pull-only brief posture of ADR-033. Adapter hooks become
bidirectional: observation in, coordination context out. Pending high-relevance
brief items are injected at vendor-supported turn boundaries (for Claude Code,
hook JSON `additionalContext`/context-injection responses on
`UserPromptSubmit`/`SessionStart`; for Codex, the equivalent supported surface,
verified before claiming it). Injection never blocks or fails the agent's turn:
hook handlers time-bound their work and fail open. Delivery and subsequent
behavioral adjustment are tracked so routing precision is measurable. Where a
vendor offers no injection surface, MCP pull plus dashboard remains the honest
fallback. Accepted by the owner 2026-08-25.

## ADR-047: Project membership is the sharing consent

Supersedes the per-session consent machinery of ADR-034 and its preview/
versioning ceremony. Installing Overgent in a Project and connecting an adapter
is the consent to share activity and session context with that Project; a
single synchronous pause switch (existing) stops sharing. The secret classifier
(credentials, tokens, environment assignments, private keys, raw tool output —
ADR-038 semantics) remains a mandatory wire gate under every mode and is not
user-disableable. Per-session consent records, preview flows, versioned consent
schemas, and their dashboard surfaces are deleted rather than hidden. Rationale:
governance built for enterprise procurement was taxing iteration before
product-market fit; teams installing a coordination tool expect intra-team
sharing. Accepted by the owner 2026-08-25.

## ADR-048: Read sets, contract fingerprints, and dependency readiness

The world model gains three objects. A **read set** records, per agent session,
which repository files/symbols the session observed (from hook file events)
with the contract fingerprint current at observation time. A **contract
fingerprint** is derived structural metadata per file: exported symbol names
and signature hashes, extracted locally by language analyzers. A **dependency
claim** (`waiting_on`) is declared via MCP or inferred by the LLM from intent
text, naming a path, symbol, or contract description another workstream is
expected to produce.

These power two finding kinds: `stale_assumption` (a fingerprint in a live
session's read set changed after the session read it — deterministic, includes
old/new signature) and `dependency_ready` (a claimed dependency's contract now
exists or stabilized in another workstream's write set, including a
"stable-but-WIP" intermediate state from checkpoint evidence). Dependency
claims are observed machine-checkable state, which narrows but does not reverse
ADR-037: no plan items, boards, or human planning surfaces return. Accepted by
the owner 2026-08-25.

## ADR-049: Re-enable production adapter setup from end-to-end evidence

Supersedes ADR-026's production setup hold. The coordination evaluation suite
now drives the built `agent-hook` executable and the official-SDK MCP bridge
through all seven M1 scenarios, with durable observation, brief routing,
revision-aware delivery, and fail-open behavior. Profile-aware binding status
also stays pending until a real event from that vendor reaches the current
local service. This is stronger evidence than the isolated client failure that
caused ADR-026, and supports normal `setup codex` and `setup claude` in the
macOS beta.

The evidence does not prove every vendor release. Claude context injection was
exercised against its real hook contract; Codex context injection was checked
against its documented hook response contract and the shared executable path,
not a separate live credentialed Codex run. Unknown vendor versions and drifted
configuration still fail closed, while Git observation and MCP/dashboard
delivery remain the honest fallback. Accepted 2026-08-26.

## ADR-050: Qualify an Apple Silicon macOS beta, not a cross-platform release

L8 qualifies the signed and notarized Overgent CLI, per-user LaunchAgent, and
desktop app for invited beta testers on Apple Silicon macOS 12 or newer.
Install, update, health validation, automatic rollback, adapter cleanup, and
recoverable local-state removal use the standalone Go executable. Linux,
Windows, and Intel macOS archives remain unverified build artifacts under
ADR-019 and must not be advertised as supported installs.

Wails v3 is still a prerelease and its public beta train remains in progress.
Keep the exact `v3.0.0-beta.12` dependency isolated in the desktop module. The
desktop is qualified only as a labeled Overgent beta on the tested macOS
boundary; the hosted browser remains the fallback. This does not claim Wails
GA stability or qualify another OS. Release publication remains owner-gated on
Apple signing/notarization credentials, the offline update-signing key, a
monitored private security channel, and a two-person second-session beta run.
Accepted 2026-08-26; supersedes ADR-029's fixture-only production boundary.

## ADR-051: Codex hooks install at the user layer and are trusted through the app-server

Codex refuses to run a non-managed lifecycle hook until the exact hook
definition has been reviewed and trusted, recording that decision as a content
hash under `hooks.state."<source>:<event>:<group>:<handler>".trusted_hash` in
the user's `config.toml`. An untrusted hook is discovered, parsed, listed, and
then skipped in silence. Writing `hooks.json` therefore proved nothing: a
member could register a Project, run a full Codex session inside it, and see no
activity at all, while `setup status` reported `hooks: "active"` because
`hookconfig.Inspect` only re-read the file Overgent had just written. This was
observed end to end: a real Codex Desktop session in a registered repository
produced zero rows in `agent_observations`, and a synthetic hook invocation of
the same executable produced one immediately.

Three changes follow. **Codex hooks move to the user layer**
(`$CODEX_HOME/hooks.json`). Trust is recorded per hook definition, so one
user-level definition needs a single review for every Project a member ever
registers, where per-project files need one review each; and Codex silently
ignores a project-level `.codex/hooks.json` when the working directory is a Git
worktree (openai/codex#27133), which is exactly where parallel agent work
happens. The cost is that the hook fires in every repository the member opens,
which `agent-hook` already absorbs by resolving the event against registered
workspaces and exiting without effect otherwise. Because the file is now shared,
`Remove` detaches only the project MCP binding and `RemoveHooks` is the
deliberate teardown, called once after the last binding is gone.

**Trust is repaired through Codex's own app-server.** `internal/codexappserver`
starts a private `codex app-server` stdio child, calls `hooks/list` to obtain
each hook's `key`, `currentHash`, and `trustStatus`, and persists the missing
trust through `config/batchWrite` as narrow `hooks.state` upserts guarded by the
user config layer's `expectedVersion`. Overgent never computes the hash and
never serializes `config.toml`. Reproducing the hash locally was attempted and
rejected: it is derived from a normalized identity, not the bytes on disk —
Codex clamps a SessionEnd timeout before hashing — so a local implementation
would be wrong in ways that only ever surface as silence, and would break on
any upstream change. Asking Codex is version-proof by construction. Selection
is restricted to handlers whose command string matches this profile's exactly,
so another profile's binding, another tool's hooks, and managed hooks are never
touched.

**Status tells the truth.** `Hooks` reports `needs_review` rather than `active`
whenever Codex has not trusted every managed hook, a `TrustReport` carries the
counts and the member-facing guidance, and the desktop adapter row says the
binding is connected but observing nothing. A report that resolved zero hooks is
not satisfied, because that is the silent failure itself.

Degradation is layered and never fails setup: app-server list and write; then
list for hashes with an append-only `hooks.state` write by Overgent when
`config/batchWrite` is unavailable; then `needs_review` plus the review
instruction. The append fallback only ever adds table headers that do not yet
exist, because a duplicate table would make the member's whole Codex config
unparseable. The app-server CLI surface is marked experimental upstream, which
this ladder exists to survive.

Rejected: **`allow_managed_hooks_only = true`** with a managed
`/etc/codex/requirements.toml`, which does grant auto-trust but disables every
user and project hook on the machine, breaking unrelated tools invisibly,
requires root, and squats on the enterprise policy path an employer may later
deploy to. **Shipping as a Codex plugin**, which does not avoid review —
installing a plugin does not trust its hooks. **`--dangerously-bypass-hook-trust`**,
which is per-invocation and Overgent does not launch the member's sessions.
**The `notify` config key**, which is a single global slot, fires only on turn
completion, and carries no tool-level detail. **Attaching to the running
app-server** to observe sessions directly, which is closed today because Codex
Desktop runs its app-server as a private stdio child and the shared daemon
requires the standalone Codex installer; `thread/inject_items` and `turn/steer`
make this worth revisiting under a later ADR if that changes.

Writing trust on the member's behalf converts a security review Codex would
otherwise show into an install-time consent. This is a deliberate product
decision by the owner, disclosed in the privacy policy, and bounded in code:
Overgent only ever upserts trust for hooks whose command is byte-identical to
the one it installed. Accepted by the owner 2026-08-27.

## ADR-052: Read sets carry provenance; Codex read evidence is vendor-inferred

ADR-048 made a session's read set the trigger for `stale_assumption`. That read
set is fed by hook events whose tool is a file inspection, and the inspection
tools Overgent recognizes are `read`, `glob`, and `grep` — Claude's names. Codex
inspects source through the shell, and a shell observation carries a command,
not a list of files. A Codex session therefore contributes no read-set entries
and can never receive a `stale_assumption` finding. Nothing reports this: the
detector is simply silent, which is the failure mode the honest-fidelity rule
exists to prevent. A member could change an exported signature under a Codex
session that had just read it and be told nothing.

**Read-set entries carry a fidelity, and sessions carry read coverage.** An
entry is `observed` when a vendor reported the specific file to a file-reading
tool, `vendor_inferred` when the vendor's own classifier concluded a command
read a path, and `self_declared` when an MCP client named the path itself.
`stale_assumption` raised from evidence that is not `observed` is not
`deterministic`, so the existing `confidenceBand` ladder carries the fidelity
into the finding without a new field. A session whose vendor cannot supply
observed reads reports that coverage rather than presenting an empty read set
as an all-clear. This is required whatever the evidence source turns out to be:
`begin_work` anticipated paths are already a second source of different fidelity
mixed silently into the same table.

Codex read coverage is `none`, not `self_declared`, and the distinction matters
because two independent gaps have to close before it improves. No hook event
names a file Codex read, and self-declaration does not fill that gap either:
Codex passes a minimal environment to MCP servers and exports no session
identity, so declared paths are attributed to the workspace workstream while the
session's own read set stays empty. Restoring session-routed read evidence for
Codex therefore needs the observer below, not better prompting.

**Codex read evidence comes from the app-server.** `internal/codexappserver`
already runs a private version-matched stdio child under ADR-051; this extends
that client with the read-only `thread/read` method and consumes
`commandExecution.commandActions`, keeping the `read` variant's `path`.
`listFiles` and `search` are not read evidence — neither proves a particular
file's contract was examined.

Measured against the bundled `0.149.0-alpha.4.1` on 2026-08-28. The hook's
`session_id` **is** the app-server `threadId`: a rollout UUID passed to
`thread/read` returned that thread with status `notLoaded`, so no discovery
heuristic is needed and the read does not resume or take ownership. Spawn plus
`initialize` cost 39 ms and `thread/read` 34 ms, which is why this is a
spawn-on-demand child and not a supervised long-lived process. In one
99-command thread, 31 command items classified a `read` and 14 more were
`unknown` while naming a reader tool, recovering 36 distinct source paths at
roughly 69% of read-ish commands. Coverage is therefore partial by measurement,
not merely by OpenAI's best-effort caveat.

The binding cost is payload: that thread read returned 1.6 MiB. Refresh is
debounced to turn boundaries (`Stop`, `SessionEnd`) rather than run per
`PostToolUse`, and items are deduplicated by id. Raw `command`, `aggregatedOutput`,
transcript text, and any path outside the registered repository are projected to
repository-relative paths and fingerprints immediately and then discarded; they
are never persisted, logged, or queued for sync. An app-server failure, timeout,
version skew, or unknown action variant degrades to no read evidence and never
blocks or delays Codex.

Rejected: **presenting Codex stale-assumption coverage as complete**, which is
the current silent behavior and the defect itself. **Rollout `parsed_cmd`
parsing** as the primary contract, which is undocumented, spelled differently
from the public protocol, and explicitly unstable; it remains available only as
a feature-detected fallback for a build with no readable app-server surface.
**An independent shell parser**, which must model quoting, wrappers,
substitutions, pipelines, and aliases, duplicates a vendor classifier reachable
through a supported protocol, and would be wrong in both directions. **Offering
an MCP filesystem read tool** to force reads through an observable path, which
would alter agent behavior and fail the passive constraint.

This does not claim complete Codex coverage and must not be described as such
until OpenAI publishes an actual read-observation contract; a `fileRead` item or
`commandActions` on Bash `PostToolUse` would supersede the source here while
leaving the provenance model in place. Accepted by the owner 2026-08-28.

## ADR-053: The lead block answers "does this need me", not only "is this colliding"

The workroom's first block existed to answer one question — is anything about to
hit me — but only one kind of thing could ever answer it. A coordination finding
reached the block; a session of the member's own that had stopped did not, even
though the service already knew it had. An agent sitting on a permission prompt
is blocked, is costing time now, and is the member's to unblock, and it rendered
nowhere near the surface built to be looked at. The block is renamed **Needs
you** and carries both.

Health is derived on the dashboard from data the snapshot already holds. Two
kinds are vendor-reported statuses that already sync — `waiting` from a
permission request, `error` from a failed tool — and the third, silence, is
arithmetic over event times. Nothing new is collected and nothing new crosses
the wire.

Silence is reported as a measurement, never as a diagnosis. A session is called
quiet only after fifteen minutes without a reported event, only while the vendor
actually reports tool activity, and only for the member's own sessions. The
fifteen-minute floor sits well above an ordinary slow turn and well inside the
thirty-minute retention sweep. The observation gate is the same rule ADR-052
applies to read sets: a vendor that reports no tool activity produces an
identical empty stream whether the agent is working or wedged, so calling that
second case a stall would be inventing evidence. The finding says the session
has reported nothing for a duration, with that duration shown; it does not say
the agent is stuck, and it takes reading weight rather than `--alert` because
the evidence does not support an alarm.

**Stall never reaches an agent.** It is a dashboard signal only, and it is not a
finding: it has no kind, no wire representation, and no route into a turn. A
heuristic with a plausible false-positive rate must not be able to spend the
interrupt budget that ADR-045's routing precision is measured on. If it cannot
hold precision as a human-facing signal it is deleted rather than tuned.

Rejected: **a `stalled` finding kind**, which would put a heuristic in the same
lifecycle as deterministic contract evidence and make it routable. **Reporting a
teammate's blocked session to the viewer**, which fails the same test as a
teammate's collision in code the member never touches. **A shorter threshold**,
which fires during ordinary builds and test suites. Accepted by the owner
2026-08-28.

## ADR-054: A Project of one is a finished Project

Every onboarding and empty-state surface described a Project as a thing that
becomes useful when a second member arrives: the created-Project screen led with
"invite the first teammate", the workroom told a lone member that "no teammates
are registered yet", and the README opened with "for teams". None of that is
true of the engine. One member running two agent sessions in one repository
produces two workstreams, two manifests, two read sets and the same collisions,
and the coordination loop treats them identically to two people's — ADR-033 made
agent sessions first-class repo-scoped workstreams precisely so that it would.

The copy is corrected to match the behavior. Invites become an option offered
rather than a step withheld, the solo workroom states a fact about the Project
instead of naming an absent teammate, and the product is described as
coordinating parallel agent sessions whether one person or several are running
them. No schema, permission, or sharing behavior changes: membership remains the
consent model under ADR-047.

This is a positioning decision as much as a copy one. The activation cost of a
team product — every member installing, enrolling, and consenting before anyone
sees value — was being charged to a single developer who could have had value
from the first session. Accepted by the owner 2026-08-28.

## ADR-055: Identity is a colour system separate from status

Rule 2 said the product has exactly two colours, and the vendor marks were
rendered in neutral ink with an explicit note that a provider identity must
never compete with `--alert`. Applying a status rule to identity is what made
the interface read as monotone: every person in a Project was the same grey
circle, so telling two teammates apart on a row meant reading rather than
scanning, and a single-word name rendered as one grey letter.

The constraint that was actually doing the work is narrower than the rule
stated. What protects "one orange sentence is impossible to miss" is that
`--alert` is the only colour in **text**. A small filled mark is not a sentence
and cannot be confused for one. So identity gets its own channel, bounded:

- It appears only in marks — member chips and vendor logos — never in text,
  never as a row background, never as a badge behind a word.
- Member hue is derived from the display name, so one person is one colour on
  every surface, which is what makes it usable for scanning rather than
  decoration.
- Saturation and lightness are fixed per theme in the stylesheet; only hue
  varies per member, so no member can render louder than another.
- The hue ramp starts past the alert band. `--alert` sits near hue 17 on both
  themes and a chip at hue 8 read as a warning even at this saturation.

Vendor marks were left neutral here on a claim that turned out to be false; see
ADR-057, which supersedes this paragraph. Accepted by the owner 2026-08-28.

## ADR-056: Streams run down, lists rank up, and History is one screen

Three ordering defects had one cause: no rule said what order anything was in.

**Sessions were in insertion order.** Teammates were ranked by presence while
the member's own sessions were not sorted at all, so a session started a minute
ago sat below one from the morning and a finished session sat above a live one.
Own sessions are now ranked by what the reader would do about them — needs you,
running, open, finished — and by recency inside each band, using the same
elapsed label the row displays so the order always agrees with the clock beside
it. Finished sessions fold behind one disclosure rather than moving to a screen
of their own: work that is over is worth keeping and not worth scrolling past.

**The session thread duplicated its own newest entry.** The feed was already
strictly chronological, but a pinned strip above it repeated the newest event,
which is why a correctly ordered stream read as though it were not. Status and
stream are different objects: what a session *is* right now belongs to the
session and reads in its header; what it *did* belongs to the thread. The
thread opens at its newest entry and follows the tail until the reader scrolls
up, at which point following stops and a control to return appears — yanking
the view back while someone is reading is worse than asking for one press.

**Harness context outranked conversation.** Sandbox rules, permissions and
repository instructions rendered at message weight, and Codex sends several
blocks of them before a session says anything, so the first real exchange was
pushed below the fold. Consecutive blocks now fold into one line. What the
harness told the agent is provenance; what the person and the agent said to each
other is conversation.

**Ledger and Decisions become History.** Both answered "what has this Project
already handled". "Ledger" named a filing cabinet rather than anything a person
goes looking for, and "Decisions" outlived ADR-037, which deleted the standalone
decision surface it was built for. One screen: what was raised, where it was
delivered, what was settled, with the raw event stream folded at the bottom. A
nav label is a word people already use; the page itself carries the precision,
including the standing limit that delivery and acknowledgement are not evidence
the agent complied. Selecting from History updates the inspector in place rather
than throwing the reader back to the Workroom. Accepted by the owner 2026-08-28.


## ADR-057: Vendor marks carry their own brand colour

Supersedes the vendor-mark paragraph of ADR-055, which rested on a measurement
error. That ADR recorded the Codex mark as achromatic artwork with no brand
colour to restore. Sampling only the most frequent colours in the asset found
its white glyph strokes and missed the artwork underneath: the icon is a
blue-violet gradient centred near hue 230 (`#667cf7`), and `filter:
grayscale(1)` had been stripping it. Blue at hue 230 is 118 ΔE76 from `--alert`
and carries no risk of being read as a warning at any size.

Claude's mark takes its brand terracotta, `#d97757`, lifted slightly to
`#d98668` on the dark ground for legibility. This is the case ADR-055 was
cautious about, because that hue does share `--alert`'s family. Measured, the
separation is 28 ΔE76 in light and 26.5 in dark — clearly distinguishable
rather than unrelated. Three things carry the distinction, and all three must
hold for this to stay acceptable:

- **Identity is a glyph; status is a sentence.** The bound from ADR-055 is what
  does the real work here. An orange starburst is never mistaken for an orange
  line of prose.
- **Shape before hue.** The Claude spark and the warning triangle are different
  silhouettes at every size the product renders them.
- **Tone.** The brand terracotta is markedly softer than either theme's alert,
  which is a saturated signal colour by construction.

The failure mode to watch is a converging Claude session, where the brand mark
and the warning glyph appear on one row. If that reads as two alerts in
practice, the fix is the row's warning treatment rather than the vendor's
colour: the identity of the agent is not negotiable in the way an indicator
glyph is. Accepted by the owner 2026-08-29.

## ADR-058: One row grid for the whole main column

The Workroom read as unrefined for a measurable reason. Four row types shared
one 680px column and each had invented its own gutter — 22px + 10, 18px + 12,
26px + 12 — so the primary text of a finding, a session, a teammate and a
section heading began at four different left edges: 0, 30, 32 and 38px. The eye
had no spine to follow, which is what "no clear hierarchy" describes from the
outside.

The type ramp had the same problem from the other direction. The subject of a
row was 15.5px/650, 14.5px/500 or 13.5px/600 depending on which component drew
it, so "the most important thing here" had no consistent form and every row had
to be read rather than scanned.

Both are now fixed by construction:

- `--row-icon` (22px) and `--row-gap` (12px) define one gutter; every row in the
  column uses it, and every primary text begins at 34px. Section headings stay
  flush at 0, so a heading is the spine and its content hangs beneath it.
- The ramp is three steps and only three: **primary** 14.5px/600 for the subject
  of a row, **secondary** 13.5px/400 at `--ink-2` for what it means, **machine**
  11.5px mono at `--ink-3` for facts. A finding headline at 15.5px/650 in
  `--alert` is the single deliberate exception, and it is the one thing on the
  screen that should outrank everything.
- Peer rows are separated by a hairline and blocks by space, so a four-line
  session cannot run into the next one.
- Teammate rows lose the fixed 84px name column that nothing else shared and
  that truncated longer names early.

Machine facts were also consolidated rather than stacked. A finding ended in
three separate mono rows — evidence, then confidence, then age; confidence and
age qualify the finding rather than evidencing it, so they now ride the action
row. A quiet session ended in three as well, one of which repeated the fact
already shown on the line directly above it. Accepted by the owner 2026-08-29.

## ADR-059: The activation handoff page submits itself

Extends ADR-024, which fixed *how* the ticket crosses to the hosted boundary —
a top-level POST of an escaped hidden form value, so it never enters a URL,
browser history, a Referer header, or browser storage. It did not say who
presses submit, and the page shipped with a button that waited for a person.

That press carries no decision. Nothing is consented to on this page: the member
has already asked to open the Project, and what a Project shares is disclosed
before enrollment and again on the workroom's own activation screen. What the
button actually produced was a stall, in both places the page renders. Inside
the desktop app the webview navigated to an unstyled page of raw user-agent
defaults — a heading, a sentence, a button — with nothing to say it belonged to
Overgent or that pressing it was the last step. From the CLI it opened in
whichever browser the member uses, so `overgent dashboard --project` produced a
new tab among however many were already open, titled only "Activate Overgent",
while the app appeared to have done nothing; the way out was to find that tab
and press a button whose purpose was not evident from the app being looked at.

The form now submits on load and the button remains for the case where the
script cannot run, which is the only case where a person still has to act.

The Content-Security-Policy is tightened rather than relaxed to allow this.
`script-src` and `style-src` are pinned to the SHA-256 hashes of the exact
inline script and stylesheet, never `'unsafe-inline'`, so one known payload runs
and nothing injected does; editing either without updating its hash breaks the
page shut rather than open. Every other directive is unchanged, including
`default-src 'none'` and a `form-action` restricted to the API origin.

Verified in a real browser rather than only in Go: the handoff posts its ticket
with no press and no console error. A CSP mistake here fails silently in a unit
test and appears only as a page that sits there with a button — the defect this
supersedes.

The desktop app never routes this through an external browser — it navigates its
own webview to the loopback URL, so the handoff and the redirect both happen
inside the app. Only the stranded *tab* belongs to the CLI path; the unstyled
page and the pointless press were common to both. Accepted by the owner
2026-08-29.


## ADR-060: The workroom summarises a finding; the inspector is the finding

The workroom's finding card and the inspector permanently beside it rendered
almost the same object. The card carried the headline, the reason, both parties
with their current actions, an evidence grid with provenance, the confidence
band and the first-seen age; the inspector carried all of that plus severity,
last-changed, and the discussion thread. The most important block on the
product's most important screen was a second copy of the panel next to it, and
that duplication is most of why the column read as dense.

The card becomes a summary: the alert glyph and headline, the plain-language
reason, one line per affected party, first seen, and the control that opens the
rest. Evidence with provenance and the confidence band move to the inspector,
where both already existed.

The product spec requires a visible finding to carry kind, severity, confidence
band, affected members, evidence with provenance, first/last seen, status and a
reason. That is a requirement on the finding as presented, not on every row
mentioning it, and the existing behaviour already read it that way: **severity
was never on the card at all** and only ever appeared in the inspector. The
requirement's purpose — never a warning without reachable reasoning — is met by
keeping the plain-language reason on the summary. `coordination-intelligence.md`
§5 now says which surface carries which fact rather than leaving it to be
inferred.

Two smaller repetitions went with it. A colliding session's current action was
printed under each party, which its own row in "Your sessions" and the inspector
both already show. The session alias was printed on every session row, though it
is a debugging identifier whose home is the details popover; individually named
subagents on the row collapse to a count, and stay named in the popover and the
thread. Accepted by the owner 2026-08-29.

## ADR-061: Branch is evidence; quiet is a control that only costs its owner

Four things that looked like one question — how does someone work without
colliding with the rest of the team — are separated here, because the obvious
answer to it is wrong.

**Branch never gates coordination.** The proposal was to detect collisions only
between workstreams on the same branch, so a feature branch would be an escape
hatch. That inverts the product. Work on one branch is invisible to work on
another until someone merges, which is exactly the expensive, silent case
`overgent-v1-spec.md` §2 exists for and the one no other tool reports; work on a
shared branch is the case Git itself surfaces within the hour. Gating on branch
would make Overgent quiet precisely where it is the only thing watching, and the
gate would be opened by the most common command in software.

So branch becomes an input to judgment instead. Two narrow rules, both in
`deterministicJudgment` and both offline: an overlap across divergent branches
is escalated one step and only from medium to high, because nothing outside the
Project will report it before merge; an overlap on a shared branch keeps its
severity and gains a sentence naming where else it will appear. A missing branch
— a detached HEAD, a workstream with no agent, a failed worktree read — reads as
unknown and changes nothing, and no branch relation can ever silence a finding
or create one. Branch was already carried on the workstream and validated at
ingest; nothing new crosses the wire.

**Work its author called a spike is capped at the dashboard.** A throwaway
experiment collides like real work and is not worth an interrupt. This reads the
workstream's own words rather than adding a switch, only an explicit statement
counts, and it is a de-escalation, so it cannot cost the routing precision
ADR-045 is measured on.

**Pause is scoped to what the caller named.** Pause was reachable per workspace
or, from the menu bar, for every workspace on the machine. A member reading one
Project is asking about that Project, so the service accepts a Project id and
the workroom offers that control; the tray switch keeps the machine-wide scope
and now says so in its label. Where the dashboard has no native bridge it prints
the exact command instead of a button that would do nothing.

**Focus is the missing inbound control, and it is deliberately asymmetric.**
Pause stops this device publishing. Nothing stopped the Project reaching an
agent's turns (ADR-046), which is what "I don't want to be interrupted" actually
means. Focus suppresses injection into one agent session and changes nothing
about what is published, because a member who mutes themselves must not thereby
make teammates less able to avoid their work: hiding is an externality, ignoring
is not. It is local state that never crosses the wire — a teammate sees no
change, so there is nothing to tell them. Nothing is consumed while a session is
quiet, so a correction is never retired unread. It always expires, because a
mute nobody remembers setting is worse than no mute in a tool whose value is
being told things; the tray shows a standing mute for the same reason.

**Resolving is recording a decision.** The inspector had two controls claiming
the word: one opened a routed, delivered resolution, the other only changed a
finding's state. The routed one keeps the verb, the other becomes `Dismiss`, and
the decision leads the thread with its delivery targets named before it is
written rather than reported after it has been sent. Comments stay, unrouted and
labelled as such: the thread is a worksheet that produces one routable sentence,
not a chat, and ADR-037's reason for deleting the planning surface applies
equally to conversation.

Rejected: **branch-scoped coordination**, above. **A symmetric mute**, which
buys quiet with someone else's safety. **Agents negotiating a resolution between
themselves**, which violates principle 5 and produces a decision no person is
accountable for. **Merge-base divergence depth as a severity input**, which is
the strongest remaining signal but needs a new local Git computation and a new
wire field; branch identity ships first and depth is a later, separate change.
Accepted by the owner 2026-08-29.

## ADR-062: A card that can be opened says so, and pausing is something you do to yourself

Six corrections to the workroom, from reading it rather than from a spec.

**Pause reports the reader's own state.** The snapshot computed
`workspacePaused` across every workspace in the Project, so a member whose
teammate had paused was told their sharing had stopped and offered a Resume
control that could only ever act on their own device. Pausing writes to the
paused member's machine and gates that machine's outbound queue; nobody can
pause anyone else, and nobody should be able to. The value is now scoped to the
viewer's own workspaces and the notice says whose sharing stopped.

**The toolbar carries controls, not instructions.** Where the page cannot reach
the local service there is no control to offer, so it offers nothing — rather
than a standing paragraph naming a command and a raw Project id. The recovery
moved into the paused notice, which is the one moment anyone needs it. Settings
closes the toolbar row instead of interrupting it.

**The lead card is openable and now looks it.** Only the headline text was
clickable and nothing indicated it, so the most important block on the screen
read as a paragraph. The headline button is stretched across the whole card and
the card takes the hover ground and revealed chevron every other openable row
already uses; the two session rows and the decision control sit above the
overlay and keep their own targets. The chevron takes `--alert` on a converging
card. A tinted background was rejected: Rule 2 forbids colour filling a
background, and the affordance works without breaking it.

**The inspector has a way back.** Opening a session from inside a finding
replaced the finding with no route to it. Selection is a trail rather than a
value: picking from a list starts one, drilling in from the inspector pushes,
and a back link appears naming what sent you there.

**The finding detail leads with the finding.** The kind and the title were
printed as one another — the hosted snapshot writes a finding's title as its
kind with the underscores removed — so the kind is now an eyebrow that appears
only when the title is genuinely something else. Evidence became a list of
statements each carrying its own provenance instead of a two-column table of
one-word keys, and the branch sentence sits with the reason rather than under a
heading of its own.

**The decision is a composer.** Two pill-shaped fields in a row read as a
search box, not as the place a team settles something. The decision is a
textarea with its delivery targets and its action on the line beneath, inside
one bordered control; the discussion below it is a thread with chips, names and
times, and a single-line composer. The border belongs to an input rather than to
a card, so Rule 1 holds: there is no filled panel here.

Fixing the composer exposed a layout bug it did not cause. `.collision-detail`
wraps the header and body in one article and carried no column, so the
inspector body never scrolled and the whole page did instead; it was invisible
only because the detail had never been tall enough to overflow. Accepted by the
owner 2026-08-29.

## ADR-063: Contract extraction runs real grammars in WebAssembly, not hand-written scanners

Contract fingerprints gain languages by linking tree-sitter's runtime and a
chosen grammar set into one standalone `wasm32-wasi` module and executing it
with wazero, a pure-Go runtime. This preserves the CGO-free, never-invoke-Node
boundary ADR-019 rests on: the C toolchain is a build-time dependency producing
a vendored `.wasm` blob, and `go build` stays toolchain-free. Grammars are
statically linked because upstream `web-tree-sitter` loads them as emscripten
side modules through `dlopen`, which wazero does not implement; a grammar set is
therefore chosen at build time and compiled lazily per module at runtime.
Each language is a separate wasm module, compiled on first use rather than at
startup. A combined module has to be compiled in full before any language can be
used, and that cost is paid by every member whether their repository contains
the language or not: measured on macOS arm64, one module carrying eight grammars
took 154 ms and 31.6 MB resident, one carrying seventeen took 555 ms and
110 MB, and the curve reaches roughly 3.3 s and 650 MB at a hundred — which
would make broad language coverage self-defeating. Split per language, the same
eight grammars cost 18.1 MB resident and about 49 ms for a repository that
actually contains Python and JavaScript, and adding a language nobody uses costs
nothing at runtime. The runtime is duplicated into each module at roughly 37 KB
compressed, which is the entire price of the split and is why the eight modules
total 1.24 MB against 0.99 MB combined.

Extraction is 33x slower per file than the existing extractors — 7.9 ms versus
0.24 ms on the same 22 KB file — which the 20-entry wire batch bounds to roughly
460 ms of background work. Python, JavaScript/JSX, Java, Rust, C#, PHP, C, C++,
Scala, Kotlin and Dart are routed — eleven wasm-backed languages beside Go's own
parser and the TypeScript scanner. Go stays on `go/parser`, which fingerprints
137 of 137 files at 73 MB/s and costs nothing. Migrating `.ts`/`.tsx` off the
hand-written scanner is a separate later decision because it re-baselines every
stored fingerprint.

Hand-written scanners are rejected as the expansion path, not merely
deprioritized. The existing 430-line TypeScript scanner already yields no
fingerprint for `convex/src/domain.ts` and `apps/dashboard/test/app.test.tsx`
in this repository, and `convex/functions/schema.ts` — the Convex database
schema, which is exactly the kind of contract a consumer depends on — yields an
empty exported surface because the scanner does not read `export default`.
Reusing that scanner for JavaScript recovers zero symbols from three real
JavaScript files: real-world JavaScript is largely CommonJS, which a token
scanner cannot distinguish from an ordinary assignment without scope tracking. A
path that fingerprints to an empty surface is worse than an unsupported path,
because it reads as a stable contract.

wazero's compiler supports linux/darwin/freebsd/netbsd/windows on arm64 and
amd64 with SSE4.1; elsewhere it falls back to an interpreter measured at 1.01 s
for one 64 KB file. The runtime is therefore requested as
`wazero.NewRuntimeConfigCompiler` rather than `NewRuntimeConfig`, which would
fall back silently: outside compiler-supported platforms the service reports no
fingerprint for wasm-backed languages and says so through `contract.WasmStatus`,
never degrading quietly to a one-second-per-file extractor. `Fingerprintable`
still answers the static question of which languages Overgent fingerprints, so a
platform gap is never disguised as a language gap. This narrows ADR-019's
language limit and adds a platform condition to it; it does not reverse
ADR-019's CGO-free rule, ADR-044's wire privacy boundary, or ADR-048's
fingerprint semantics. Byte offsets, not source text, cross back over the wasm
boundary, and the ADR-038 deny gate still applies to every derived signature
before publication and before the file contract hash.

A language is added by compiling its grammar into a module and writing its
visibility rules; the second half is the real cost, because a declaration that
is not reachable from another workstream must never be recorded as contract
surface. Each language states that rule differently — `public` in Java and C#,
`pub` in Rust, absence of `private` in PHP, Scala and Kotlin, absence of
`static` in C, a positional `public:` section in C++, and a leading underscore
in Python and Dart — and there is no way to derive it from the grammar.
Two errors of that shape have already been caught by tests rather than by
members: Scala recorded local variables inside method bodies as public API, and
a walk depth tuned for flatter languages silently dropped C++ methods declared
inside a namespace.

Ruby is excluded because it carries no structural visibility marker at all, so
its exported surface would be a guess, and a wrong guess is a false
interruption. Swift is excluded because the grammar in the tree-sitter
organisation is an abandoned stub below the runtime's supported ABI and the
maintained grammar requires the tree-sitter CLI to generate its parser. A
language whose exported surface cannot be determined honestly is worth less
than no support at all, because an empty surface reads as a stable contract. Grammars are generated at different parser ABI
generations, so each is compiled against the headers it ships with; one shared
header silently miscompiles whichever half does not match.

The vendored module is a compiled binary in a public repository that executes on
every member's machine, and reading it is not review. Every input is pinned to
an exact commit in `internal/contract/wasmgrammar/PROVENANCE.md`, and a test
asserts the committed artifact's hash and size against that record so it cannot
drift unnoticed. The blob was built on a developer
machine rather than in CI; moving that build into the release workflow is a
prerequisite of the signed-release gate in `docs/release.md`, not of this
decision. `github.com/odvcencio/gotreesitter`, a pure-Go tree-sitter
reimplementation needing no build toolchain, is the named fallback if
maintaining a WASI build step proves worse than expected; it was rejected for
costing 6.73 MB for the same four grammars and being slower on the largest file
tested. Accepted 2026-08-29 on the evidence in
`validation/spikes/multilang-contract`.

## ADR-064: The interface tells the truth about how much the engine already does

The 2026-09-01 workroom refit, from the "Workroom Refit" design pass. One
finding organised it: almost nothing the pass recommended invented capability.
The engine already judged, routed, diffed, delivered, and confirmed; the
interface either discarded those facts at the projection or scattered them
across screens.

Four facts the projection now carries instead of discarding: `delivery` (the
judgment layer's routing verdict, which now decides what "Needs you" admits —
the UI re-deriving admission from ownership alone was second-guessing its own
engine); `evidence[].contract` (the per-symbol old/new signatures behind a
stale assumption, now rendered as a was/now divergence block); per-session
decision delivery state (from `decisionDeliveries`, now a live tracker on the
decision itself — the loop closes where the decision was made, not only in
History); and ISO timestamps beside the prose labels.

The resolution model collapsed to three exits: send an outcome-shaped decision
(chips prefill the composer; "Settled outside Overgent" is a first-class
outcome that still routes the conclusion), or dismiss with a reason that is
the feedback vocabulary. Acknowledge, the standalone feedback row, the
create-card pre-step, and comments are removed — every surviving input has an
observable effect on the affected agents. The umbrella word is "finding";
collision is one kind of eight, and `dependency_ready` (good news) no longer
renders as a possible collision with an alert glyph.

Structure changes: one area-grouped Sessions block across everyone (the group
heading carries the collision when a finding spans two of its sessions; the
self/other split survives as row richness, not as two lists); History is a
case log — one entry per finding lifecycle on a single arc line, where the
only colour is a decided case not yet considered by every agent; identity
chips are solid keycaps; the inspector's grey-capitals headings comply with §4;
fonts are self-hosted. Branch grouping, dead since area grouping shipped, is
deleted. The phone Playwright project is deleted with it: the dashboard is a
desktop surface by design, and the e2e suite — dead since fixtures became
opt-in, because it navigated without `fixtures=1` — runs green again on the
laptop project. The owner directed the full proposal's implementation
2026-09-01; the rendered result awaits his review.

## ADR-065: Rename the product and public brand to Overgent

The owner has renamed the product from Stickguy to **Overgent** and acquired
`overgent.com`. Overgent is now the only current product name in application
copy, command examples, desktop bundle metadata, managed adapter labels, public
contracts, package names, release artifacts, and documentation. The primary
public origins are `overgent.com`, `api.overgent.com`, and
`schemas.overgent.com`; deployment still requires the ordinary release and DNS
gates before those origins are advertised as live.

The rename does not alter the product model or reopen prior architecture:
`Project` remains the user-facing container, the local core remains Go, the
hosted boundary remains the versioned Overgent HTTP contract, and every privacy,
authorization, advisory-control, and one-service rule continues unchanged.
Protocol structure and schema versions do not change merely because the brand
changed, but public identifiers and newly generated artifacts use the Overgent
name.

Existing pre-release installations may still contain the former name in local
paths, keychain records, URL handlers, or managed agent configuration. The
released migration must recognize those exact legacy shapes long enough to
adopt or remove them without touching unrelated configuration; it must never
treat an arbitrary similarly named entry as Overgent-owned. New installations
write only Overgent identifiers. Accepted by the owner 2026-09-02.

## ADR-066: Reset unreleased installs and make the domain the update anchor

Supersedes only ADR-065's pre-release migration requirement. The owner confirmed
that Overgent has no public installs and that the single external dogfood install
can be removed and reinstalled. The first Overgent beta therefore supports a
clean install only; it does not adopt former Stickguy paths, Keychain items, URL
handlers, services, or managed agent configuration. This exception does not
weaken migration requirements after the first published Overgent release.

Launch the public repository at `github.com/khalidM3/overgent` instead of
choosing an awkward placeholder organization name. A later transfer to an
owner-approved organization is allowed, but installed clients must not use the
repository owner as their mutable update-channel identity. Installer and updater
defaults fetch the Ed25519-signed channel manifest from
`https://releases.overgent.com/current/update-manifest.json`; that manifest
points to immutable, versioned GitHub release assets. The release workflow may
change where those assets are published without changing the client-facing
channel. The old repository path must not be reused after a transfer because
doing so would defeat GitHub's redirects. Accepted by the owner 2026-09-02.

## ADR-067: Decouple public beta binaries from source-repository visibility

Supersedes ADR-066 only where it requires the repository itself and its GitHub
release assets to be public at beta launch. The owner directed that the source
repository remain private while the company and long-term source-publication
boundary are decided. This is a launch-state decision, not permission to put
private cloud operations, production secrets, customer data, or hidden wire
behavior in this repository.

GitHub Actions remains the trusted builder. A protected release environment
publishes only signed, notarized release outputs to the public
`overgent-releases` Vercel Blob store under immutable `releases/<version>/`
paths. After clean-machine verification, a separate protected workflow copies
that version's signed manifest to `current/update-manifest.json`. Installed
clients continue to discover updates only through
`https://releases.overgent.com/current/update-manifest.json`; they neither need
GitHub credentials nor depend on the repository owner or future organization.
Accepted by the owner 2026-09-02.

## ADR-068: Overgent repairs its own leftovers instead of asking a member to

Supersedes ADR-066's clean-install exception. That exception rested on the
premise that the one external install would be removed before the first
Overgent build was installed. It was not, and could not reliably have been:
Codex activity hooks live at the user layer, in `$CODEX_HOME/hooks.json`, and
therefore outlive uninstalling the product, deleting its profile, and switching
repositories.

Two beta testers hit the consequence on day one. A brand-new Project on a
brand-new repository reported Codex as belonging to another Overgent profile,
refused to configure it, and offered a Reconnect confirmation whose "current"
and "new" bindings named the same person. Neither Reset nor the reset that runs
from the credential-recovery screen touched adapter bindings at all, so the one
control named after the problem could not fix it.

The rule, from now on and not only for this rename: **a managed binding that
nothing can still be using is adopted automatically, and only a binding a live
profile still depends on is a member's decision.** `hookconfig.Abandoned` makes
that call from facts on disk - a portable binding, the same profile under a
moved executable, a profile directory that is gone, a profile holding no
enrolled device, a superseded product name, or an executable that no longer
exists. Anything it cannot place stays a decision and still requires the
explicit reconnect, because moving a binding another live profile depends on is
not recoverable by the member it surprises.

Adoption runs in `Setup` for the agents a member ticked, and as a repair pass on
every launch of the desktop app and from `overgent setup repair`. The repair
pass never creates a binding: an agent nobody connected here stays unconnected,
because clearing up after an earlier build is Overgent's job and connecting an
agent is the member's.

The desktop app also stops trusting an already-installed `~/.local/bin/overgent`
without looking at it. It compares the installed CLI with the bundled one and
replaces it when they differ, so an app update can no longer ship a new app
driving an old CLI - which is the mechanism that would have reintroduced this
same class of failure at the next rename or wire change.

Recognizing legacy names is exact, not a pattern: only names this product has
actually used qualify, so no directory belonging to anyone else's software can
be adopted. Accepted by the owner 2026-09-03.

## ADR-069: One Mac has one device identity across every Project it joins

`/v1/enrollments` mints a device. That is correct exactly once per Mac, because
the local profile holds a single device ID and `app.Register` refuses a
workspace registered under any other one. Accepting a second invite therefore
enrolled a new device, had the registration rejected, and left the member with
a burned one-use invite and nothing joined. There was no in-app control for it
at all, so the only route was `overgent join` in a terminal, which failed the
same way.

`POST /v1/memberships` closes it. It is authenticated as the device that
already exists, redeems the invite, and inserts one `members` row for
`(project, device)` plus a dashboard ticket. It mints no device and no invite:
a member who has just accepted one has not been handed one to pass on. The
hosted model already supported this - `members` is keyed by project and device,
and `bootstrap` has always resolved every Project a device belongs to - so this
is a missing door, not a new room.

It is deliberately a separate handler from `enroll` rather than a branch inside
it. Every guard protecting a first enrollment is about a caller with no
identity; here the caller has one, and the questions are different: is this
credential still good, is the invite still good, and is this device already in
that Project. The rollback differs for the same reason - `Join` revokes the
device it created when local registration fails, and here that device is the
one holding every other Project on the Mac, so `JoinAdditional` never revokes.
Accepted by the owner 2026-09-03.

## ADR-070: The shared rate bucket is sharded, and its limits are abuse ceilings

The unauthenticated edge bucket cannot be keyed on anything the caller
controls, because no trustworthy client-IP header exists at the Convex boundary
and a key derived from forwarding data is a key an abuser picks. It was
therefore one key per route - and so one document that every request on that
route wrote to.

That had two consequences. Convex serializes writes to a document, so a hot row
turns ordinary concurrency into optimistic-concurrency failures, which surface
as an unexplained 500 rather than as a limit a caller can act on; on the
activation route that is a member being told Overgent "could not open the live
Project" with nothing they can do about it. And limits chosen as if they were
per-caller are absurd as product-wide ones: five Project creations per minute
across all of Overgent is a number three people setting up together will hit.

The bucket is now sharded across sixteen keys, chosen at random per request.
The per-route numbers are unchanged and are now per shard, which makes each one
what a bucket nobody can be identified within should always have been: a
product-wide abuse ceiling. Callers holding a credential are still bounded
individually by the authenticated bucket beside it. Accepted by the owner
2026-09-03.

## ADR-071: Overgent is an open-source application with a hosted default

Supersedes ADR-067. The source repository, including the Convex backend,
dashboard, desktop app, installers, and release workflows, becomes public under
Apache-2.0 as `docs/open-source-strategy.md` recommended. The owner does not
form a company around Overgent now. The hosted deployment at
`api.overgent.com` is run from this public code as the default team backend so
that the first five minutes never require deploying anything; self-hosting is a
documented, supported escape hatch, not the onboarding path.

What stays out of the repository is unchanged from
`docs/public-repository-boundary.md`: production secrets, account identifiers,
private runbooks, incident records, customer data, and abuse-detection
specifics. Future commercial team features, if any, are added on the hosted
side without closing the source (the open-core boundary in
`docs/open-source-strategy.md` §9).

Rationale: the category is contested by funded, closed competitors and by the
agent vendors themselves; a solo, part-time startup racing them is the weakest
version of this project. A well-distributed open-source tool with a
zero-friction hosted default is the version that fits the owner's constraints
and forecloses nothing. Release trust (signed, notarized artifacts discovered
only through `releases.overgent.com`) is unchanged from ADR-066/067.

Consequences: Lane 02 purges the tracked `stickguy` binary and rewrites
history before the repository is public; the Go module path moves to the
public organization; `SECURITY.md` gains a real channel (GitHub private
vulnerability reporting) before visibility flips. Accepted by the owner 2026-09-04; direction confirmed in the migration planning session.

## ADR-072: Local-first by default; the hosted backend is opt-in per Project

Extends ADR-031 and ADR-054; narrows ADR-041. A fresh install creates a
Project against a **local backend**: the open-source `convex-local-backend`
binary, bundled with the desktop release, bound to loopback only, storing its
SQLite database under the profile root. The Go service supervises that process
(start, health, restart with backoff, stop on idle) and the desktop shows it as
ordinary service health. No account, invite, or network access is required, and
nothing about a local Project leaves the device.

The Go core, dashboard, and desktop keep speaking only the `/v1` contract; the
local backend is simply another origin that `internal/hosted.New` already
accepts (loopback HTTP). The Convex functions in `convex/functions/` are the
single coordination engine for every mode; there is no second engine.

A member who wants remote teammates creates or joins a **team Project** on
Overgent Cloud or a self-hosted backend. Mode is a property of the Project, not
of the install (ADR-074). "Sign up" means creating a team Project and receiving
an invite link under the existing ADR-035/ADR-069 device-and-invite identity
model; there are no email accounts, passwords, or SSO in this decision.

Rationale: coordination metadata is tiny and Convex free tiers are generous,
so the hosted default costs almost nothing, but people who only run parallel
sessions on one Mac should not have their coordination facts stored remotely by
default. The development profile has already run this exact loopback shape
since ADR-031, so the risk is packaging, not architecture.

Consequences: the desktop bundle grows by the backend binary (about 160 MB
today; Lane 01 measures the real number and idle memory). The bundled
`convex-backend` is licensed under FSL-1.1-Apache-2.0 and is redistributed,
not modified; `NOTICE` gains a third-party entry. Source builds fetch the
pinned backend release by checksum. Accepted by the owner 2026-09-04; direction confirmed in the migration planning session.

## ADR-073: Semantic providers are configured per Project with the Project's own keys

Supersedes the "hosted deployment secret only" clauses of ADR-040 and ADR-045;
keeps their degradation rules. Each Project carries an AI settings record:
judgment provider (`anthropic`, `openai-compatible`, or `none`), model, optional
base URL, and key; embedding provider (`openai`, or the deterministic fallback),
model, dimensions, optional base URL, and key. The backend resolves the provider
for a Project in this order: the Project's own settings; the deployment's
operator keys, only when the deployment explicitly enables them; otherwise
`none`, which degrades to deterministic evidence with visible fidelity exactly
as today.

Keys are written through a dedicated authenticated `/v1` operation by a Project
owner, encrypted at rest with a deployment secret, never returned by any read
(only a configured flag and a four-character hint), never logged, and never
part of any synced coordination object. This is a deliberate, member-initiated
exception to "credentials never cross the wire": the member sends their own key
to the backend they chose, over TLS or loopback, to be used for that Project's
judgment calls. The secret classifier remains a gate on every other payload.

Overgent Cloud ships with operator keys disabled. The owner may enable them
later behind the ADR-070 abuse ceilings; that is an operational choice, not a
product default.

Rationale: the variable cost of the hosted service is LLM and embedding calls,
not Convex. Bring-your-own-key by default makes hosting a few hundred users
close to free and gives members the tweakability of choosing model and
provider, including OpenAI-compatible local servers.

Consequences: Lane 04 owns the protocol change; `packages/coordination`
providers take model and base URL as parameters instead of constants;
`intelligence.ts` stops reading `process.env` directly for provider keys. Accepted by the owner 2026-09-04; direction confirmed in the migration planning session.

## ADR-074: A profile binds each Project to its own backend

Supersedes the profile-level `apiBaseUrl` in `internal/config.Config` version 1
and ADR-041's "separate profile per backend" workaround. Configuration version
2 records, per Project, the backend origin and the device identity used with
it; workspaces resolve their backend through their Project. One Go service
holds one hosted client per distinct backend, one credential per
(backend, device), one event queue partition per workspace as today, and one
heartbeat loop per backend. Migration from version 1 assigns every existing
Project the old global origin. Hooks and MCP bindings still name the profile
(ADR-043) and need no change.

Rationale: without this, "local by default, team opt-in" is really "switch the
whole app between two profiles", which contradicts ADR-072's per-Project
framing and makes joining a friend's team Project disruptive.

Consequences: Lane 06; largest refactor of the migration; may ship after the
first public release if the README states the profile-switch limitation. Accepted by the owner 2026-09-04; direction confirmed in the migration planning session.

## ADR-075: Publish release assets through GitHub Releases; retain Blob only as the update anchor

Supersedes the blob-copy half of ADR-067. Once the source repository is public
under ADR-071, signed, notarized release artifacts are attached to immutable
draft GitHub Releases. The signed update manifest names those GitHub asset URLs.
After clean-machine verification, the protected promotion workflow publishes the
draft release and copies only `current/update-manifest.json` to the public Blob
store. Clients continue to discover updates at
`https://releases.overgent.com/current/update-manifest.json`; the domain, not
GitHub, remains the update anchor.

The install, uninstall, and macOS download aliases redirect to the latest
published GitHub Release assets. The protected release environment retains the
signing and Blob credentials; forks cannot use either. Accepted by the owner
2026-09-04.

## ADR-076: A browser session is authorized by its device's memberships, not by the one Project that minted it

Narrows the "single Project" half of ADR-024. The ticket stays exactly as
ADR-024 specifies — Project-scoped, minted from the Keychain-backed device
credential, POSTed once, never in a URL or storage. What changes is what the
resulting cookie authorizes: every read and write reauthorizes the hashed
session, resolves the device it belongs to, and then requires an active
`members` row for *that device* in the Project being asked for. For the Project
that minted the session this is identical to what ADR-024 already did.

The reason is that the previous rule made a documented product limitation
unfixable rather than merely unfinished. The workroom sidebar and the command
palette are both written to switch between Projects, but `/v1/dashboard/session`
could only ever return the one Project the cookie was minted for, so a member
with two Projects on one Mac saw a switcher with a single entry and could reach
the second only by re-activating it from the app. Membership was always the real
authority — the same device enrolls in every Project on it, and the desktop app
can mint a ticket for any of them on demand — so the cookie's Project binding
was narrowing the interface without narrowing what the holder could obtain.

Each Project's own `members` row is resolved, never the session's, because
membership is per-Project and callers use it for "mine"; carrying one Project's
member identity into another would attribute its work to the wrong person.
Membership is re-read per request rather than baked into the session, so leaving
or being removed from a Project takes effect immediately rather than at the
session's eight-hour expiry.

The cost is accepted and stated: a stolen session cookie now reaches every
Project enrolled on that Mac rather than one. The cookie remains
Secure/HttpOnly/SameSite=Strict, same-origin, eight-hour, and revocable, and an
attacker holding it is already inside the origin as that device. Device
revocation, which invalidates every session for the device, remains the
containment. `revokeDevice` is deliberately **not** widened: it still requires
membership in the Project the session was minted for, which fails closed rather
than open. Accepted 2026-09-05.

## ADR-077: Intelligence has a device-default tier above per-Project settings

Extends ADR-073 without changing it. A Project's AI settings remain the only
thing that runs, remain stored in that Project's own backend, and remain
encrypted there with that deployment's secret. This adds one tier above:
defaults held on the member's Mac, so intelligence is configured once instead of
re-entered for every Project.

Resolution is unchanged — Project settings, then the deployment's operator keys
where enabled, then `none`. Defaults never enter that order. They are a starting
point for a Project's settings, not a fallback the backend consults, so no
backend gains a new place to look for a key.

Where they apply is the substance of this decision:

- **A Project on this Mac takes them automatically at creation.** Its backend is
  the loopback server on the member's own machine, so applying a default moves
  the key from the login Keychain to a database on the same disk. Nothing
  leaves.
- **A shared Project does not.** Saving a key to a shared Project uploads it to
  a server that other members' sessions spend it from, and possibly one the
  member does not operate. That is a decision they take on that Project's own
  settings screen, behind the existing explicit approval checkbox, next to the
  sentence that says what saving does.

The non-secret half of the defaults is a plain file in the profile
(`ai-defaults.json`). The keys are not: they go to the login Keychain under
per-profile accounts, so a readable configuration file never carries a provider
key and a member can revoke one in Keychain Access without this application. The
file records only that a key was stored; the Keychain decides whether one still
is, so a key deleted outside Overgent is reported as absent rather than as
present-but-unusable. Turning a provider off deletes its stored key rather than
leaving a secret behind for something switched off.

Consequences: `AIDefaults`/`PutAIDefaults` on the desktop bridge; an
Intelligence tab in App settings; nothing on the enrollment path, because a
Project configures itself perfectly well with no defaults set and asking for an
API key during onboarding would gate first run on a decision that can be taken
later. Accepted 2026-09-05.

## ADR-078: The Project is the only top level; Workroom and History are views of one

Revises the sidebar structure ADR-064 left in place, and declines a
cross-Project workroom on the evidence below.

The sidebar listed `Workroom`, then `History`, then every Project, then App
settings. Neither of the first two is a destination beside that list: the
workroom **is** the selected Project's view, and History is that same Project's
record. Promoting them asserted a sibling relationship to the Project list that
does not exist, and put the open Project on screen three ways at once — as the
Workroom item, as the History item, and as the current row below them.

**The Project list is now the top level, and a Project has views.** Workroom and
History are chosen from a tab row directly under the Project's own name, so the
reading order states the real hierarchy: which Project, then which view of it.
The sidebar holds brand, search, the Project list with its add control, and App
settings. The "Needs you" count the Workroom item carried moves to the tab and
was never lost besides — it already read beside the block's own heading, and per
Project on the sidebar rows, which is the duplication design-system.md Rule 7
forbids. The workroom sidebar and the entry shell's sidebar
(`desktop-onboarding.tsx`) now present the same list in the same place; the two
shells stay separate, because the hosted workroom cannot reach the local service
and that is what the entry shell exists for.

Two consequences fall out. Back from a screen names the Project rather than
naming "History" whenever that view happened to be open — a view is not a place
you came from. And the toolbar button that opens People is named **People**, in
every Project: it read "Sharing" on a local Project and "Invite" on a shared one
while announcing "Invite people to this Project" either way, so the visible word
and the accessible name disagreed. "Sharing" also named the wrong control. Pause
is the sharing control, and it keeps its own word; People is who is in the
Project — members, invites, devices, revocation — and is named for the screen it
opens.

**The alternative considered and declined** was making Workroom mean something
Projects cannot: every agent session across every Project, with the Project as a
column. `loadSession` already aggregates across backends, and
`LiveProjectSource.timers` is already keyed per Project, so N concurrent polls
is a small change. The cost is not the timers:

- `onStatus` is a single global setter driving `LiveApp`'s one state. With N
  backends polling, one unreachable team server flips the whole app to `offline`
  while the Project on screen is healthy. Cross-Project liveness needs
  per-Project status first.
- `scopeKey(projectId, repoFingerprint)` in `convex/src/domain.ts` scopes
  findings, contract fingerprints and semantic objects. A cross-Project view
  could list sessions side by side and could not show a single collision between
  them, and must not imply otherwise. That is the whole content of the workroom,
  so the aggregate view would be a launcher wearing the workroom's name — less
  than what is there now, in the place the most valuable screen used to be.

Declined for those reasons rather than for cost, and revisitable: nothing here
depends on one Project being one repository, so multi-repo Projects would add
rows to a list that already exists rather than requiring this to be torn up. If
it is revisited, per-Project poll status is the prerequisite and the absence of
cross-Project coordination is the thing the interface has to say out loud.
Accepted 2026-09-05.
