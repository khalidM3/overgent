# Overgent — Continuous Implementation Plan

Status: canonical execution order  
Last updated: 2026-08-24

No time estimates. Complete levels in order, verify the exit gate, and immediately continue while work remains. Scaffolding alone never completes a level.

```text
L-1 validation spikes
          ▼
L0 contracts/scaffold
  ├── L1 local core
  ├── L2 hosted service
  └── L3 dashboard
          ▼
L4 deterministic vertical slice
          ▼
L5 MCP integration
          ▼
L5A agent activity validation
          ▼
L5B macOS desktop preview
          ▼
L6 coordination intelligence
          ▼
L7 collisions/session detail
          ▼
M1 coordination eval harness
  ├── M2 read sets/contract fingerprints
  ├── M3 push delivery via hooks
  ├── M6 sharing simplification (parallel)
          ▼
M4 LLM judgment layer
          ▼
M5 dependency readiness
          ▼
L8 distribution/beta hardening
```

Complete the gates in `prebuild-validation.md` before production implementation; its isolated spike lanes may run in parallel after bootstrap under one integrator. L1–L3 may run in parallel only after L0 contracts generate successfully. L4 integrates them.

## L-1 — Architecture and adapter validation

Deliver the bounded Codex MCP/hook, Git/worktree, Convex shared-state/vector, Go service/distribution, and intelligence-eval seed spikes defined in `prebuild-validation.md`. Spikes produce fixtures/evidence and ADRs, not production framework code.

Exit: every gate passes, narrows a capability honestly, or selects an existing portable fallback; no unresolved assumption can force replacement of Go, the Overgent protocol, the manifest model, project isolation, or the coordination-harness lifecycle.

## L0 — Contracts and scaffold

Deliver Git/README; owner-approved license and NOTICE; contribution/code-of-conduct/security files; public/private repository boundary; Go 1.26 module; pnpm dashboard/Convex workspace; canonical repository layout; initial OpenAPI/JSON Schema including manifest/semantic-object/finding/coordination-brief/checkpoint/verification contracts; generated Go/TS types; provider/index/router interfaces; CI; public provenance-ready release skeleton; logging conventions; fixture events and conformance tests; fake producer.

Exit: clean checkout passes documented commands; generation deterministic with drift failure; one fixture validates identically in Go/TS; public repository contains no production credential/private-data placeholders; private security reporting channel and license are decided before public launch.

## L1 — Local Go core

Deliver command routing/config; single-instance service and user IPC; SQLite migrations for projects/workspaces/workstreams/manifests/queue/cursors; repo fingerprint; safe Git adapter; session/workstream baseline; filesystem debounce/full rescan; baseline-to-`HEAD` plus worktree manifest; atomic chunk publication; durable event queue/ack cleanup; pause/resume/doctor; mock-server and temporary-Git tests including local commits, rename/delete/untracked/ignored, and 1,000-path changes.

Exit: one service observes two repositories; a locally committed 1,000-path fixture remains represented against its workstream baseline with atomic hosted activation; restart preserves queue, baselines, manifests, and IDs; pause prevents sends; second instance exits and stale lock recovers; idle resources measured.

## L2 — Hosted project service

Deliver Convex schema/indexes; `/v1` HTTP actions including brief/item retrieval; project/invite/enrollment; hashed credentials/revocation; dashboard tickets; batch validation/dedupe/ack; heartbeat/presence; workstream/manifest projections; repository-scope context revisions; atomic manifest assembly; deterministic finding upsert; semantic-object/vector tables kept separate; retention jobs; authorization/rate/size tests.

Exit: two simulated devices enroll/publish; duplicates/out-of-order do not duplicate activity; brief/item access is workstream/project authorized; context revisions increment only for material scoped changes; cross-project/revoked access fails; cleanup test passes; semantic providers absent.

## L3 — Dashboard

Deliver activation/session, project switcher, live board, presence/fidelity/workstreams, large-change summaries, finding radar/detail/evidence/lifecycle, activity, pause/device entry points, semantic degraded/disabled states, explicit loading/empty/offline/unauthorized/version states, responsive accessible UI, tests and Playwright fixtures.

Exit: synthetic updates appear live; all states intentional; project isolation proven; laptop/phone widths pass.

## L4 — Deterministic vertical slice

Deliver complete `create`/`join`, one-time dashboard activation, two-device presence, CLI intent, real path/overlap, offline/reconnect, instrumentation, and isolated two-device demo.

Exit: clean supported systems need no runtime; second member visible under 60-second median target; same path shows one overlap within five seconds after publication; outage queues and flushes once; pause immediate. This is first dogfoodable product.

## L5 — MCP

Deliver stdio bridge; pinned stable official Go SDK; idempotent/revisioned `begin_work`, `update_intent`, `check_coordination`, `report_checkpoint`, `acknowledge_context`, `finish_work`, and `report_event`; structured verification summaries; workspace/workstream resolution and ambiguity handling; instructions to report before broad edits/at checkpoints/read findings before shared changes; project-scoped Codex MCP setup plus bounded `SessionStart`/supported subagent hook delivery accepted by the L-1 ADR; Claude adapter after verifying its current contract; idempotent setup removal/status; conformance and real-client smoke tests.

Exit: real Codex begins work and receives a brief, preflights broad edits, reports/retries a checkpoint without duplication, acknowledges context, and finishes with verification state; MCP exit does not stop service; ambiguity never guesses; unsupported agents retain Git/manual fidelity.

Current outcome: complete for the supported dogfood adapters under ADR-033 and
ADR-040. The lifecycle core and official-SDK bridge pass; Project-scoped Codex
and Claude Code hooks provide bounded observation while MCP remains the
documented brief-pull channel. The capability model explicitly reports that
provider-native attention is unavailable, so urgent delivery targets the person
in the dashboard and never pretends to interrupt an agent turn.

## L5A — Opt-in agent activity adapter validation

Before adding shared contracts, run isolated synthetic Codex App Server/SDK and Claude Code hooks/Agent SDK spikes under ADR-027 and `agent-activity-sharing.md`. Map independently started versus adapter-connected session coverage; normalize session/turn/message/plan/tool/subagent/permission/file-path/verification events; and prove exact coordination/activity/conversation profile projection. Test owner/member consent precedence, representative preview, synchronous pause/downgrade, deletion/retention, adapter removal/config drift, unknown-event failure, and Project isolation. Use fixture-only content and retain only redacted capability/evidence metadata.

Exit: each vendor is separately passed, narrowed, or assigned an existing Git/manual/MCP fallback; `.env` variants, protected paths, tokens, transcript/system/reasoning/source/diff/tool-result/raw-command/output candidates provably never reach durable storage or a sender; supported activity is sufficient to explain what an agent is doing without Overgent owning or controlling its loop. Only then may the integrator define versioned shared schemas/generated code and enable an opt-in adapter.

Current outcome: complete for `activity/v1` under ADR-033, ADR-039, and ADR-042.
Authenticated Project hooks pass for supported lifecycle/tool/subagent/safe-path
activity. Each vendor has a separate session-record adapter; the record stays
local except for separately previewed, versioned conversation sharing. A
vendor-visible session title may become automatic bounded intent only after the
local title classifier and hosted semantic policy both accept it.

## L5B — macOS desktop preview

Deliver the ADR-029 preview as a separate exact-pinned Wails v3 module: embed the
shared React build without a localhost listener; provide one native window plus
a persistent menu-bar item; read health and invoke pause/resume-all and scan only
through the existing current-user Unix socket; label fixture-backed window data;
and keep close-to-hide, open, and clean quit behavior. The preview must not start
another service or introduce Wails/CGO into the root Go module.

Exit: native macOS arm64 launch renders the shared dashboard; the system-tray
menu remains after closing the window and reflects connected/disconnected and
paused state honestly; pause/resume and scan reach the existing service; no TCP
listener exists; tests, build, ad-hoc signing, privacy review, idle measurement,
and a visual smoke pass. Signed/notarized distribution, updater integration,
native notifications, deep links, cross-platform runtime support, and real
hosted desktop authentication remain L8 or a separately gated milestone.

## L6 — Coordination intelligence vertical slice

Deliver bounded semantic-object validation; embedding-provider and semantic-index adapters; Convex vector search with mandatory composite project/repository scope keys plus post-retrieval authorization/current-state checks; lexical/structural/semantic candidate retrieval; versioned evidence fusion; finding kinds/confidence/severity/explanations; deterministic relevance router/brief renderer with budgets, context revisions, delivery/acknowledgement and stale-assumption detection; optional strict-schema adjudication for ambiguous candidates; retries/degraded mode; radar feedback; labeled fixtures/eval runner; dashboard/MCP delivery.

Exit: two devices with different paths but duplicate behavior produce a justified `redundant_work` finding; incompatible intent is surfaced before either edits; shared schema/package impact reaches only affected workstreams; an unrelated fourth workstream gets no item; a relevant decision change marks an old dependent brief stale; briefs honor budget/truncation rules; independent large changes stay non-interruptive; cross-project retrieval fails; provider outage preserves structural routing. This is the first V1-complete coordination-harness loop.

Current outcome: complete under ADR-030 and extended with the optional managed
OpenAI embedding adapter. The public labeled corpus and anonymous loopback
two-device suite pass every exit case. Revision-matched 1024-dimension managed
vectors now participate in finding evaluation and scoped brief candidate
retrieval; successful embeddings trigger recomputation. Provider-only
similarity remains advisory medium-severity evidence, while provider outage or
absence falls back honestly to the deterministic concept engine and structural
routing.

## L6A — Local dogfood developer loop

Deliver one-command loopback Convex, Vite, Go service, and macOS development
desktop orchestration; React hot reload in the native shell; an atomically
installable `Overgent Dev.app`; development-only in-webview dashboard ticket
activation; hot registration of a second linked-worktree workstream; and
explicit development-only Codex/Claude MCP configuration with Git/manual
fallback fidelity.

Exit: one member can run two distinct linked worktrees through one per-user
service, attribute Codex and Claude to separate workstreams, see same-path Git
overlap in the live native dashboard, and exercise L6 semantic findings using
bounded reported intents. Production adapter setup, transcript monitoring, and
non-loopback development origins remain disabled.

Current outcome: complete under ADR-031 and extended by ADR-041. Frozen
Go/TypeScript/protocol and both desktop build modes pass; the real development
bundle launches against Vite; and the anonymous loopback L6 live suite passes
all structural, semantic, authorization, and dashboard assertions. Two Macs may
connect to the same HTTPS cloud Convex development deployment by adding a team
Project to the ordinary development profile (ADR-074); ADR-041's isolated
`dev:shared` profile is retired, because a profile no longer talks to one
server at a time.

## L6B — Native Project and agent onboarding

Deliver a native first-run surface in the existing development desktop: choose
one canonical Git repository; create or join a Project; detect Codex and Claude
Code; explicitly install their drift-safe Project MCP entries; show honest Git
fallback fidelity; open the authenticated live Project; and assign existing
distinct linked worktrees for local Codex-versus-Claude attribution. Preserve
the single-profile limitation, never create/mutate worktrees, and keep all
development origins loopback-only.

Exit: a fresh local profile can enroll from the app without a terminal; a
connected profile can open live state; React hot reload retains the Wails
bridge; absent/declined adapters still receive Git observation; and selecting
two existing linked worktrees registers separate workstreams without shell
concatenation or Git mutation.

Current outcome: complete under ADR-032. Native/React unit tests, frozen root
checks, both desktop build modes, and the L6 live suite pass. Adding a second
Project to one running local profile and signed production distribution remain
L8 work.

ADR-043 extends onboarding with profile-aware binding states, explicit
transactional reconnect with rollback, partial-install repair, and first-event
runtime verification. A stale loopback/shared-profile binding can no longer be
reported as ready merely because provider config files exist.

## L7 — Collision resolution and session detail

Deliver sync-card create/comment/resolve over collisions; a resolution delivered
once to every affected agent session; local session detail read from the vendor
transcript; and per-session, previewed, revocable sharing of that content.

Exit: two members resolve a collision and both agents receive the outcome;
resolution delivery is idempotent and cursor-based; no tracked file is auto
modified; a member always sees their own session content without sharing it;
sharing is off by default and rejects every secret class as a whole.

Current outcome: complete under ADR-034, ADR-036, and ADR-037. Plan items and
advisory claims were removed rather than hidden: planning is a human process
that competed with the product's actual value. Session detail no longer depends
on hook payloads, which never carried assistant text or reasoning; it is read
from the vendor transcript locally, so the owner sees prompts, replies,
vendor-recorded reasoning, and tool names before deciding to share anything.
Quoted code is allowed inside a consented conversation, and naming a file is not
treated as disclosing it (ADR-038); environment values, credentials, tokens,
keys, and raw tool output are rejected whole at both boundaries. Codex and
Claude Code each have their own record adapter (ADR-039).

## V2 reboot — coordination intelligence that closes the loop

ADR-044 through ADR-048 reorient the product around three layers: a shared
world model (intents, read sets, write sets, contract fingerprints, dependency
claims), a divergence engine (contract drift, semantic collision/duplication,
path overlap, dependency readiness), and routing/actuation (LLM-adjudicated
findings pushed into agent turns via hooks). M-levels below replace the former
L8/L9 ordering; distribution moves after the loop is proven. Task briefs for
parallel execution live in `docs/tasks/`.

## M1 — Coordination eval harness

Deliver scenario-based two-agent coordination evaluations on top of the
existing loopback two-device suite and two-worktree Codex/Claude exercise.
Scenarios: (A) backend changes a response type while frontend builds against
it; (B) two agents independently implement semantically similar functionality
in different files; (C) agent B changes an interface agent A started from;
(D) agent A requires something agent B has not finished; (E) genuinely
independent tasks — silence required; (F) same file, unrelated regions — quiet
warning only; (G) WIP change — uncertainty communicated, not treated as
canonical. Each scenario is scripted, repeatable, and measures: correct
relationship identified; correct workstream interrupted; sufficient context
supplied; downstream agent adjusted; silence honored.

Exit: all seven scenarios run headlessly against a real local stack with
scripted agent behavior; per-scenario pass/fail and precision metrics are
reported by one command; the suite is the gate for every later M-level.

Current outcome: complete. `pnpm eval:coordination` boots the loopback stack,
drives two scripted agents through all seven scenarios, and gates every later
M-level. Scenarios now drive the real `agent-hook` executable rather than
synthesized calls, and the report records routing precision, false interrupts,
and per-scenario delivery latency.

## M2 — Read sets, contract fingerprints, stale-assumption findings

Deliver local language analyzers extracting per-file contract fingerprints
(exported symbols and signature hashes; Go and TypeScript first); read-set
capture from existing hook file events with fingerprint-at-observation;
fingerprint sync as derived facts; deterministic `stale_assumption` finding
with old/new signature evidence; dashboard and brief rendering.

Exit: M1 scenarios A and C pass — a session that read a contract later changed
by another workstream receives a `stale_assumption` finding naming the exact
symbol, while unchanged-contract path touches (scenario F baseline) stay quiet.

Current outcome: complete under ADR-048. Go and TypeScript analyzers derive
exported-surface fingerprints locally; read sets carry the fingerprint current
when a session observed a file; contract changes are attributed to the
publishing workstream rather than inferred.

## M3 — Push delivery into agent turns

Deliver hook-based context injection per ADR-046: pending relevant brief items
render into bounded injected context at Claude Code turn boundaries; delivery
and acknowledgement tracked; injection fails open and never blocks a turn;
Codex surface verified then implemented or honestly narrowed to MCP/dashboard.

Exit: in M1 scenario C, the stale agent receives the correction inside its next
turn without human relay and measurably adjusts; scenario E injects nothing.

Current outcome: complete under ADR-046 for Claude Code and Codex, proven end
to end in scenarios A, C, and E. Observation and delivery hold separate bounded
budgets so a slow observation cannot starve a correction, and an item is never
claimed as delivered unless the caller can still receive it.

## M4 — LLM judgment layer

Deliver hosted LLM adjudication and diff-fact summarization per ADR-045:
bounded diff summaries as derived facts; cross-workstream duplication and
architectural-conflict candidates judged by the LLM with explanations;
interrupt/queue/dashboard/silence routing decisions; cost budgets and outage
degradation to deterministic findings.

Exit: M1 scenarios B and G pass with explanations; scenario E/F false-interrupt
rate meets the documented precision gate; provider outage preserves M2/M3
behavior.

Current outcome: complete under ADR-045. Every finding is judged and routed
through one delivery decision (`next_turn`, `dashboard`, `silent`). The managed
adjudicator is Anthropic behind a provider interface with a deterministic
fallback, so the eval suite passes with no API key present. Aggregate routing
precision rose from 0.529 to 0.833 with zero false interrupts.

## M5 — Dependency readiness

Deliver `waiting_on` claims via MCP plus LLM inference from intent text;
readiness detection from contract fingerprints and checkpoint evidence in other
workstreams' write sets, including a stable-but-WIP intermediate state; routed
unblock notices through the M3 channel.

Exit: M1 scenario D passes — the waiting agent is notified within one turn
boundary of the dependency contract appearing, with the contract included.

Current outcome: complete under ADR-048. Claims are declared through the
`waiting_on` lifecycle argument and satisfied only by observed contract
evidence from another live workstream; an unverified producer yields a
stable-but-WIP notice that upgrades in place when verification lands.

## M6 — Sharing simplification

Deliver ADR-047: delete per-session consent records, preview flows, versioned
consent schemas, and their dashboard/MCP surfaces; project membership plus
pause switch is the consent model; secret classifier remains a mandatory wire
gate with its tests intact. May run in parallel with M2–M5; it only deletes.

Exit: no consent-ceremony code path or UI remains; classifier and pause tests
pass; enrollment-to-visible-activity requires zero additional consent steps.

Current outcome: complete under ADR-047. The per-session consent ceremony is
deleted; the secret classifier and pause switch remain mandatory gates.

## L8 — Distribution and beta

Unchanged in content; now gated on M1–M5. Deliver signed cross-platform releases/checksums/SBOM; installers; update/rollback; OS service recovery; member/device/invite management; export/deletion; privacy-safe diagnostics; load/soak/reconnect/migration/security tests; contributor/adapter guides; re-evaluate the preview's Wails beta and qualify any supported desktop release.

Exit: clean install/update/uninstall; security checklist; no critical data loss in restart/network/update tests; a real team voluntarily completes a second session.

Current outcome: implementation complete to the owner-controlled release gates
under ADR-049 and ADR-050. The repository now contains signed updater metadata,
verified replacement and automatic rollback, a recovering macOS LaunchAgent,
rendered installer/uninstaller, signed/notarized desktop workflow, SBOM and
provenance generation, privacy-safe diagnostics, Project fleet controls,
owner/member exports and deletion, caller-scoped edge limits, and live
authorization/deletion coverage. L8 remains open until the owner supplies the
release trust inputs and security channel, publishes a credentialed candidate,
records clean-machine lifecycle evidence, and a real two-person team completes
the second-session gate. Linux, Windows, and Intel macOS remain unqualified.

## Parallel agent lanes

After L0: Agent A owns Go local core; Agent B Convex/API; Agent C dashboard fixtures; one integrator owns protocol/generated code and end-to-end merge. Agents never redefine shared schemas independently.

Every handoff includes behavior/acceptance criterion, files/contracts changed, verification run, security/privacy considerations, limitations, and next unblocked task.

## Definition of done

Behavior matches acceptance criteria; success/failure tests exist; security/privacy addressed; docs/contracts/generated files current; formatting/lint/typecheck/tests pass; no debug bypass/placeholders on production path; clean process reproduces result.
