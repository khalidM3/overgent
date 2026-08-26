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

## ADR-033: Make supported agent sessions first-class repo-scoped workstreams

The owner-demonstrated product requirement is seamless session awareness: after
a member selects a repository and explicitly connects Codex or Claude Code, new
supported sessions opened anywhere under that repository appear automatically.
They do not need to call a Stickguy tool, create a branch, or use a distinct
worktree. A linked worktree remains an optional Git-isolation technique, not an
attribution prerequisite.

Use current documented project-local lifecycle hooks as passive observation
surfaces. Codex and Claude Code both expose session, prompt-boundary, tool,
permission, subagent, stop, and session-end events. Install exact managed hook
groups alongside the existing Project MCP entry, preserve unrelated settings,
refuse drift, run observation asynchronously where the vendor permits, and
never return a decision that controls agent execution. Map each vendor session
to a stable hashed Stickguy workstream scoped by the registered repository;
vendor session and transcript identifiers remain local and are never sent.

The initial `activity/v1` projection shares lifecycle/status, a bounded
Stickguy-generated action label, allowlisted tool name, hashed subagent alias
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
installing Stickguy, joining a Project, connecting an adapter, or enabling safe
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
does not turn prohibited data into allowed data. Stickguy may display only what
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

Live agent sessions also carry the real checked-out branch. Stickguy reads it
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

Stickguy may now read the vendor transcript file named by the supported hook's
own `transcript_path` for a session running in a registered repository on this
device. The file is read locally, bounded from the tail, and never copied into a
second durable store; Stickguy records only the path, the session title, and the
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
Stickguy, and they are shared only under the same explicit consent. Where a
vendor records no reasoning, Stickguy shows none and claims none. Accepted by
the owner 2026-08-25 after confirming that hook-only session detail cannot
show a usable session.

## ADR-037: Narrow the product to collision coordination

Remove shared plan items and advisory path claims. Planning is a human process
that teams already run elsewhere, and a plan surface inside Stickguy competed
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
and is never read, which keeps the ADR-036 promise that Stickguy shows only what
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
`stickguy-concepts/v1` as an offline, deterministic fallback. The managed
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
automatically. A structurally recognized binding to another Stickguy profile
must be shown explicitly and requires a member-confirmed **Reconnect to this
Project** action. Unknown or conflicting managed-looking configuration remains
fail-closed and is never overwritten automatically.

Reconnect snapshots both provider files, replaces only Stickguy's recognized
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
deterministic evidence. `stickguy-concepts/v1` is demoted to a test fixture.
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
versioning ceremony. Installing Stickguy in a Project and connecting an adapter
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
