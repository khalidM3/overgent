# Stickguy — Continuous Implementation Plan

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
L7 plan/sync/decisions
          ▼
L8 distribution/beta hardening
          ▼
L9 intelligence expansion/adapters
```

Complete the gates in `prebuild-validation.md` before production implementation; its isolated spike lanes may run in parallel after bootstrap under one integrator. L1–L3 may run in parallel only after L0 contracts generate successfully. L4 integrates them.

## L-1 — Architecture and adapter validation

Deliver the bounded Codex MCP/hook, Git/worktree, Convex shared-state/vector, Go service/distribution, and intelligence-eval seed spikes defined in `prebuild-validation.md`. Spikes produce fixtures/evidence and ADRs, not production framework code.

Exit: every gate passes, narrows a capability honestly, or selects an existing portable fallback; no unresolved assumption can force replacement of Go, the Stickguy protocol, the manifest model, project isolation, or the coordination-harness lifecycle.

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

Current outcome: narrowed by ADR-026. The lifecycle core and official-SDK bridge pass, but production Codex/Claude setup remains withheld pending focused current-client compatibility evidence.

## L5A — Opt-in agent activity adapter validation

Before adding shared contracts, run isolated synthetic Codex App Server/SDK and Claude Code hooks/Agent SDK spikes under ADR-027 and `agent-activity-sharing.md`. Map independently started versus adapter-connected session coverage; normalize session/turn/message/plan/tool/subagent/permission/file-path/verification events; and prove exact coordination/activity/conversation profile projection. Test owner/member consent precedence, representative preview, synchronous pause/downgrade, deletion/retention, adapter removal/config drift, unknown-event failure, and Project isolation. Use fixture-only content and retain only redacted capability/evidence metadata.

Exit: each vendor is separately passed, narrowed, or assigned an existing Git/manual/MCP fallback; `.env` variants, protected paths, tokens, transcript/system/reasoning/source/diff/tool-result/raw-command/output candidates provably never reach durable storage or a sender; supported activity is sufficient to explain what an agent is doing without Stickguy owning or controlling its loop. Only then may the integrator define versioned shared schemas/generated code and enable an opt-in adapter.

Current outcome: NARROW complete under ADR-028. Authenticated Claude project
hooks pass for supported lifecycle/tool/subagent activity. Codex App Server
passes bounded existing-task enumeration/read but not cross-process realtime
subscription, so MCP/Git/manual remains its live fallback. Production collection
is still disabled pending reviewed contracts, generated code, consent controls,
retention/deletion integration, and end-to-end security tests.

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

Current outcome: complete under ADR-030. The public labeled corpus and anonymous
loopback two-device suite pass every exit case. The initial deterministic
concept provider is intentionally vocabulary-bounded; semantic findings remain
quiet radar/brief evidence, and a broader provider or proactive interruption
requires the later precision/cost/privacy gate.

## L6A — Local dogfood developer loop

Deliver one-command loopback Convex, Vite, Go service, and macOS development
desktop orchestration; React hot reload in the native shell; an atomically
installable `Stickguy Dev.app`; development-only in-webview dashboard ticket
activation; hot registration of a second linked-worktree workstream; and
explicit development-only Codex/Claude MCP configuration with Git/manual
fallback fidelity.

Exit: one member can run two distinct linked worktrees through one per-user
service, attribute Codex and Claude to separate workstreams, see same-path Git
overlap in the live native dashboard, and exercise L6 semantic findings using
bounded reported intents. Production adapter setup, transcript monitoring, and
non-loopback development origins remain disabled.

Current outcome: complete under ADR-031. Frozen Go/TypeScript/protocol and both
desktop build modes pass; the real development bundle launches against Vite;
and the anonymous loopback L6 live suite passes all structural, semantic,
authorization, and dashboard assertions.

## L7 — Plan, claims, sync, decisions

Deliver revisioned plan items; ownership/status; normalized path/glob claims; sync-card create/comment/resolve; durable affected-member decisions; remaining MCP tools; optional untracked local context; structured while-away digest.

Exit: two members resolve a finding and both agents receive the decision; concurrent plan edits conflict rather than overwrite; no tracked file auto-modified; decision delivery idempotent/cursor-based.

## L8 — Distribution and beta

Deliver signed cross-platform releases/checksums/SBOM; installers; update/rollback; OS service recovery; member/device/invite management; export/deletion; privacy-safe diagnostics; load/soak/reconnect/migration/security tests; contributor/adapter guides; re-evaluate the preview's Wails beta and qualify any supported desktop release.

Exit: clean install/update/uninstall; security checklist; no critical data loss in restart/network/update tests; a real team voluntarily completes a second session.

## L9 — Intelligence expansion and adapters

After the V1 intelligence loop. Candidates: richer local language analyzers, plan reconciliation, bounded narration, improved impact models, cost controls, additional adapters, explicit Git checkpoints, local/self-hosted semantic-index adapters including a benchmarked TurboVec sidecar.

Each AI exit gate: validated structured output; documented offline threshold; useful/noisy instrumentation; outage-safe fallback; prohibited-data policy satisfied.

## Parallel agent lanes

After L0: Agent A owns Go local core; Agent B Convex/API; Agent C dashboard fixtures; one integrator owns protocol/generated code and end-to-end merge. Agents never redefine shared schemas independently.

Every handoff includes behavior/acceptance criterion, files/contracts changed, verification run, security/privacy considerations, limitations, and next unblocked task.

## Definition of done

Behavior matches acceptance criteria; success/failure tests exist; security/privacy addressed; docs/contracts/generated files current; formatting/lint/typecheck/tests pass; no debug bypass/placeholders on production path; clean process reproduces result.
