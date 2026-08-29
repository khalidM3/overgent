# Manual dogfood findings — 2026-08-27

Single-Mac local run against a synthetic fixture. Claude Code (CLI, 2.1.197) and
Codex (ChatGPT desktop app, codex-cli 0.149.0-alpha.4.1) on one enrolled Project.

- Fixture: `~/stickguy-eval-fixture` @ tag `eval-base-v2`
- Profile: `~/Library/Application Support/Stickguy`
- Project: `prj_d47289ed2ca9834aa42b8d7b94629105`
- Inspector: `SINCE_MS=<ms> python3 ~/.stickguy-eval/sgpeek.py all 14`

## What is proven to work

| Capability | Evidence |
|---|---|
| Per-session attribution across vendors | Two agent workstreams, correctly tagged `claude` / `codex`, distinct from the workspace workstream |
| Deterministic path collision | `direct_collision` / `agent-path/v1`, high, routed to exactly the two agent sessions |
| Read-set tracking + contract fingerprinting | `stale_assumption` / `contract-watch/v1` fired only after a consumer read the file and a producer changed it |
| Correct single-target routing | `stale_assumption` routed to the reader only; the writer was not targeted |
| Next-turn hook injection | `refresh` delivery to `wrk_agent_*`, `renderedSize` 76 and 177, carrying real finding IDs |
| Agent behavior change | Claude volunteered the collision unprompted, then adapted `frontend/session.ts` to the new signature |
| Codex hook trust handshake | `setup codex` recorded trust via the desktop app-server, 9/9 events trusted |

## Bugs

### B7 — Deleted repository root prevents the service from booting (CRITICAL, FIXED)
A workspace whose root no longer exists aborted `app.Run`, so the service exited
for **every** Project on the device. CLI, agent hooks and MCP all failed with
`connection refused`; `pnpm dev` swallowed the reason.
Root cause: the `watch.Add` loop returned an error instead of degrading.
Fix applied in `internal/app/app.go`: a missing root now logs a warning and skips
only that workspace. `go build` clean, `go test ./internal/app/` passes.

**Regression test** `TestRunSkipsWorkspacesWhoseRootHasDisappeared` in
`internal/app/app_test.go`: registers two workspaces, deletes one root, and asserts
the service still reaches health, keeps both registrations, and continues publishing
batches for the survivor. Verified to fail with the fix reverted
(*"service exited: watch wsp_a: lstat …: no such file or directory"*). Its teardown
deliberately does not block on the `done` channel, because `waitHealth` drains that
channel on the failure path and an earlier version of the test hung instead of
failing.

### B8 — MCP briefs and findings use different workstream identities (CRITICAL, FIXED for Claude; Codex degrades honestly)
`check_coordination` always returns empty. Findings are routed to per-session
workstreams (`wrk_agent_*`) created by hooks, but MCP resolves only by workspace
and reports `workspace.WorkstreamID` (`internal/mcp/server.go:201`). `createBrief`
filters on `workstreamPublicIds.includes(workstream.publicId)`
(`convex/functions/service.ts:474`), so the two identities can never intersect.
`internal/mcp/server.go` has no session identity at all — no env var, no correlation.

Measured in one Claude turn, same finding, same moment:

| Path | Trigger | Workstream | Size | Items |
|---|---|---|---|---|
| Hook injection | `refresh` | `wrk_agent_8de5f6` (claude) | 76 | `fnd_b564b42…` |
| MCP pull | `manual` | `wrk_d38baf33` (workspace) | 0 | `[]` |

Impact is worse than silence. In SG-03 both findings were injected into Claude's
turn (`injection_deliveries` rows for `wrk_agent_eaa5707c`, item revision 1,
`fnd_e3436a3b` + `fnd_fae3873c`), and in that same turn Claude called the MCP tool
three times and told the user: *"I checked stickguy coordination (no active
resolutions or brief items flagged)"*. The agent relayed the empty MCP result as the
authoritative Stickguy state while two high-severity findings were open, routed to
it, and already injected. The product caused a **false all-clear**.
**The identity was already available and simply never read** — no contract change was
needed, and the earlier ADR-level sizing was wrong.

The workstream derivation is a pure function of `(vendor, sessionID)`
(`internal/agentactivity/activity.go`). Claude Code exports
`CLAUDE_CODE_SESSION_ID` into the environment of every MCP server it spawns, so the
MCP process can compute the identical `wrk_agent_*` id its own hooks use. Verified on a
live session: the env var hashed to `wrk_agent_bfc4a95a15efb39f…`, exactly matching the
running Claude workstream.

Codex is different, and this was measured rather than assumed. Comparing the
environments of live MCP servers: the Claude-spawned server (parent `claude`) carried
47 variables including the session id; the Codex-spawned servers (parent `codex`)
carried 7, essentially `PATH`. **Codex passes a minimal environment to MCP servers and
exports no session identity.** The binary contains the strings `CODEX_SESSION_ID` and
`CODEX_THREAD_ID`, but neither reaches an MCP child process at 0.149.0-alpha.4.1.

**Fix applied.**
- `agentactivity.WorkstreamIDFor(vendor, sessionID)` extracted and exported, so the hook
  path and the MCP path share one derivation and cannot drift. Two copies of that hash
  is exactly how this bug returns.
- `internal/mcp/server.go` resolves the session identity from the environment once at
  startup and attaches it to every lifecycle call; the tool output now reports the
  session workstream too.
- `internal/app/app.go` `handleLifecycle` prefers a valid `AgentWorkstreamID` over the
  workspace workstream, and validates it before use so a malformed value can never be
  trusted into that field.
- A vendor exposing no session identity falls back to the workspace workstream, which is
  honest but still cannot see session-routed findings.

Because that one value also feeds intent payloads, checkpoints and read sets, this is
expected to close **B11** (workspace workstream absorbing agent intent) and **B15**
(agents colliding with themselves) at the same time.

**Regression tests**, each verified to fail with the fix reverted:
`TestSessionWorkstreamMatchesTheHookDerivedIdentity` and
`TestSessionWorkstreamIsEmptyWithoutAVendorSessionIdentity` in `internal/mcp/`, and
`TestLifecyclePrefersTheCallingAgentSessionWorkstream` in `internal/app/`, which also
covers the fallback and the malformed-identity rejection.

**Confirmed live (round 8).** `mcp__stickguy__check_coordination` at 17:13:54 produced
a brief of `renderedSize` 177 carrying `['stale_assumption', 'direct_collision']`,
attributed to `wrk_agent_7011056abdc00e6b` — the calling Claude session. The identical
tool returned `"items": []` in every previous round. Every brief in the round was
session-attributed; `wrk_d38baf33` appears nowhere, and no finding targeted a workspace
workstream, so **B11 and B15 are closed with it**.

The agent's follow-through was the full intended loop: `check_coordination` returned the
collision, `get_resolutions` confirmed none was recorded, `acknowledge_context`
acknowledged the delivery, and the agent then **stopped and escalated to the operator**
rather than overwriting a peer's in-flight edit.

Attribution, stated carefully: Claude's edit failed first with "String not found in
file", which by itself reveals that the file changed. It does not reveal *who* changed
it. Three facts in the agent's report could only have come from Stickguy — that the
editor was a **codex agent session**, that it is **active right now**, and that **no
resolution is recorded**. The bare "file changed" is not attributable; those three are.

**Codex remains unfixed** — recovering a Codex session identity is the same research
question as the Codex app-server surface in `b22-research-brief.md`.

### B9 — Sessions stay live forever unless explicitly archived (HIGH, OPEN)
`Stop` sets `idle`, `SessionEnd` sets `done` (`internal/agentactivity/activity.go:110`).
Nothing ages `idle` into `done`. The collision engine counts anything not `done`
as live (`convex/functions/service.ts:1466`) and agent-path findings resolve only
on `SessionEnd` (`convex/functions/service.ts:1275`).

In the Codex desktop app, **starting a new chat does not end the previous session**;
only explicit Archive emits `SessionEnd`. Observed: a Codex session sat `idle` for
90 minutes still claiming `backend/refresh.go`, eligible for new collisions.
Dashboard presence decays to "offline" after 120s (`service.ts:258`) while the
engine still treats the session as live — an honest-looking UI over a stale engine.

Suggested: an idle→done timeout, and/or exclude sessions whose presence is offline
from collision eligibility.

### B10 — Path evidence conflates reads with writes (HIGH, FIXED)
**Second confirmed instance (SG-06 re-run).** A `direct_collision` fired on
`backend/sessions.go` at **high** severity with **`next_turn`** delivery, evidence
*"Both active agent sessions reported work on backend/sessions.go."* `git diff` shows
a single hunk wiring `Lookup` to `activeLoginSession` — unambiguously Codex's work.
Claude only read the file. The evidence sentence is therefore an overclaim: one
session did not "work on" that path at all. A high-severity next-turn interruption was
generated from a read.

**Root cause.** The separation already existed and leaked. `internal/app/app.go:657`
routes inspection-tool paths to the read set with the comment *"A read set is fed by
inspection tools only; an edit is a write, and the manifest pipeline already reports
it"*, and `agentactivity.ReadTool` already classifies `read`/`glob`/`grep`. But 43
lines earlier the same paths were also attached unconditionally to the
`agent.activity_reported` payload, Convex unioned them into `workstream.safePaths`
(`convex/functions/service.ts:1230`), and the collision engine computes overlap
directly from `safePaths` (`convex/functions/service.ts:1917`). A read and a write on
one path were therefore indistinguishable to the collision engine.

This is also the mechanism behind B14: Claude's hooks fire `Read` constantly while
Codex mutates through `apply_patch`, whose paths come from patch headers and are
genuine writes — hence roughly 5x the path surface for equivalent work.

**Fix applied** in `internal/app/app.go`: work-evidence paths are now gated on
`!agentactivity.ReadTool(event.Tool)`, so `safePaths` means "wrote" while reads keep
flowing to the read set that drives stale-assumption detection. No schema or protocol
change. Chosen over adding a separate `readPaths` field, which would have touched the
JSON Schema contract. Consequence accepted by the owner: the dashboard's per-session
file list now shows only files an agent actually edited.

**Regression test** `TestAgentEventKeepsReadToolPathsOutOfWorkEvidence` in
`internal/app/app_test.go`, verified to fail with the fix reverted and pass with it.
Full `go build ./...`, `go vet ./internal/...` and `go test ./internal/...` are clean.
**Confirmed end to end (SG-03 re-run, round 6).** Claude's `safePaths` fell from 8
entries (6 of them reads) to exactly `frontend/session.ts`, the one file it edited.
`stale_assumption` still fired, high, routed to Claude alone and delivered
(`refresh`, size 101, carrying `fnd_c546b5c3`). No spurious `direct_collision` on
`backend/refresh.go` appeared. Codex reported only genuine writes
(`backend/refresh.go`, `backend/refresh_test.go`). The fix did not cost SG-03.

`safePaths` records files an agent merely read. In SG-03, Claude was instructed not
to modify backend code; it read `backend/refresh.go` and wrote only
`frontend/session.ts`, yet its path set held 8 entries including
`shared/settings.ts`, `shared/user.ts`, `frontend/theme.ts`, `frontend/user_avatar.ts`,
`frontend/dashboard.ts`, `backend/sessions.go` — none of which it modified
(confirmed against `git status`).

Consequence: a spurious `direct_collision` (high, `next_turn`) was raised on
`backend/refresh.go` between the reader and the writer, **duplicating** the correct
`stale_assumption` from the same evidence and routing to the writer, who has nothing
to fix. SG-03's oracle requires the writer receive no corrective brief.

Two next-turn findings were delivered where one was warranted. This directly
threatens the ≥90% next-turn precision gate.

### B17 — Empty briefs are reported as `degraded: false` (CRITICAL, OPEN)
Every MCP brief in SG-06 returned `"items": []` alongside `"degraded": false`, so
Stickguy actively asserted that coordination intelligence was healthy while it was
structurally incapable of producing a finding (B16: no Codex intent to compare, B8:
wrong workstream identity). AGENTS.md requires the opposite — *"unsupported/disabled
semantic processing degrades to structural evidence and is never presented as full
intelligence."*

The consequence is visible in the agent's own reasoning. Claude called the tools
deliberately, at the right moments, and drew exactly the wrong conclusion from each:

- after `begin_work` → *"No relevant coordination items yet."*
- after `check_coordination` → *"No collisions or coordination notes. Now implementing
  backend/revoke.go…"*
- after `finish_work` → *"Workstream finished with no coordination conflicts reported."*

A duplicate implementation was being written in `backend/security.go` at that moment.
The agent used Stickguy's empty, confidently-non-degraded answer as positive evidence
that it was safe to proceed. A tool that returns nothing is ignorable; a tool that
returns "all clear" is actively harmful, because it suppresses the caution the agent
would otherwise have applied.

All three briefs also carried `"workstreamId": "wrk_d38baf33a75021126a8e0896d0c8c0df"`
— the workspace workstream — which is direct wire-level confirmation of B8.

### B16 — Codex sessions publish no intent, so semantic detection is Claude-only (CRITICAL, FIXED)
SG-06 (semantic duplicate under different paths and vocabulary) produced **zero
findings**. Claude wrote `backend/revoke.go` ("revocation of active browser
credentials on role change"); Codex wrote `backend/security.go` ("invalidation of all
current login sessions after a privilege change"). Same behaviour, no shared file, no
shared vocabulary — the canonical positive case.

Cause: every Codex session workstream carries a placeholder identity.

| Workstream | vendor | title | summary |
|---|---|---|---|
| `wrk_agent_6bfb6152` | claude | Implement credential revocation on role change | Finished subagent |
| `wrk_agent_6827ca8c` | codex | **Codex · codex-6827ca** | **Turn finished** |
| `wrk_agent_ac55b152` | codex | **Codex · codex-ac55b1** | **Session ended** |

`semanticObjects` exist for Claude sessions and for the workspace workstream, and
none for any Codex session — Codex's "summary" is a lifecycle status string, not an
intent. With no Codex side to embed, the comparison never happens.

Impact: `redundant_work` can never fire between a Claude session and a Codex session.
Mixed Claude+Codex is the configuration being shipped, and the semantic judgment layer
(ADR-045) is inert across it. Every real-agent `redundant_work` observed in this
Project came from synthetic `wrk_eval_*` fixtures or from the self-collision in B15;
none from two real agents.

**Root cause.** The Codex transcript parser read conversation turns from `event_msg`
records of type `user_message` / `agent_message` — the Codex **CLI** rollout shape.
The Codex **desktop app** (0.149.0-alpha.4.1) emits `task_started`, `item_completed`,
`token_count`, `task_complete` and never those two types. The `response_item/message`
records that do hold the turns are deliberately dropped except `role: "developer"`
(`internal/sessiontranscript/codex.go:122`), so the parser saw zero `KindUser`
messages, `derivedTitle` returned empty, `ClassifyCoordinationTitle` rejected it, and
`payload["sessionTitle"]` was never set (`internal/app/app.go:638`).

**Fix applied** in `internal/sessiontranscript/codex.go`: handle `event_msg` →
`item_completed` → `item.type` of `UserMessage` / `AgentMessage`, with an `itemText`
helper that takes only `type: "text"` parts. `CommandExecution`, `FileChange` and
`Reasoning` items stay dropped, so raw tool output and vendor-held reasoning still
never become content. The desktop app emits exactly one `UserMessage` per session —
the human's prompt — while its injected `<recommended_plugins>` block appears only in
the `response_item` view, so this shape preserves the "never mistake injected context
for something a person wrote" invariant rather than breaking it. `Reasoning` was
deliberately not mapped to `KindThinking` despite the CLI doing so for
`agent_reasoning`; it is unverified whether the desktop item is vendor-held reasoning.

**Verified end to end** (see also B18, a casing defect in the first version of this
fix). A fresh Codex desktop session now yields
`title: "change the accentColor in frontend/theme.ts to green"` and a live
`semanticObject` — the first for any Codex session in this Project. This also confirms
the Codex hook reports the main session id, not a guardian subagent thread id.
`go build`, `go vet`, and the `sessiontranscript`, `app` and `agentactivity` test
packages all pass.

**Regression test** `TestCodexReadsDesktopAppItemCompletedTurns` in
`internal/sessiontranscript/codex_test.go`, built on a synthetic desktop-shaped
rollout: it asserts the derived title, exactly one user and one assistant turn,
the mixed `"text"`/`"Text"` part casing from B18, and that command output, file
changes and reasoning items never become content. Verified to fail with the casing
fix reverted. SG-06 was re-run and passed (round 5).

### B18 — Codex assistant turns never reached the session view (HIGH, FIXED)
Reported from the desktop app: a Codex session showed the operating instructions and
the member's own prompt, then "Turn finished", with none of Codex's replies. Claude
sessions rendered thinking and replies correctly. Confirmed on the wire — for the
session in question only `system` (27) and `user` (7) `agent.conversation_shared`
events existed, and no `assistant` event at all, while the rollout on disk held two
`AgentMessage` items.

Cause: the Codex desktop app is inconsistent about the case of its content part type.
`UserMessage` writes `"type": "text"`; `AgentMessage` writes `"type": "Text"`. The
first version of the B16 fix compared `part.Type == "text"`, so user turns parsed and
assistant turns were silently dropped.

Fix: `itemText` now folds case (`strings.EqualFold`). Verified against the real
rollout — the session parses as system/system/system/user/assistant/assistant with
title intact. Not a dashboard bug; nothing assistant-side had ever reached the wire.

### B21 — The browser activation screen is a dead end (HIGH, FIXED)
Reported after a laptop shutdown: the dashboard shows "Browser activation … Activate
secure session" and the button does nothing.

`LiveApp` renders that screen whenever `loadSession()` returns 401/403, and wires the
button to `load()` — the same request that just failed
(`apps/dashboard/src/main.tsx:118`). With no valid browser session the retry returns
401 again and the state is set straight back to `activation`, so the screen re-renders
identically and reads as an unresponsive button. Nothing on the screen performs an
activation.

Real activation requires the desktop app to mint a one-time ticket that the dashboard
exchanges server-side, so a browser that has lost its cookie cannot recover from this
screen at all. Confirmed: the deployment currently holds zero `browserSessions`.

Two defects in one: the copy promises an action the button cannot perform, and there
is no route back to a working state from inside the page.

**Fix applied** in `apps/dashboard/src/main.tsx`: `load` now takes a `retry` flag, and
a retry that lands back on `activation` sets `activationRechecked`. `ActivationView`
takes `stillInactive` and then states that only the Stickguy app can issue a ticket,
names reopening the Project from Stickguy Dev.app as the recovery, and relabels the
button "Check again". The message is neutral body text rather than `--alert`, which
the design system reserves for work converging on the viewer. Workaround while
unpatched: `stickguy dashboard --project <id>`.

**Regression test** "tells a browser with no session that only the Stickguy app can
issue a ticket" in `apps/dashboard/test/app.test.tsx`, verified to fail with the fix
reverted. `LiveApp` is now exported for that test. All 41 dashboard tests pass and
`tsc --noEmit` is clean.

### B20 — `redundant_work` never names the shared behaviour (MEDIUM, OPEN)
The SG-06 re-run succeeded: `redundant_work` (`coordination/v1`, medium, dashboard)
naming both agent sessions, with semantic evidence
`{"fidelity":"semantic","kind":"semantic","source":"stickguy-concepts/v1","summary":"Bounded intent summaries share a strong behavior concept."}`.
Honesty is good — *"appear to"*, confidence `medium`, no claim that similarity proves
duplication, which is exactly what the SG-06 oracle demands.

But the oracle also requires *"an explanation naming the shared behavior"*, and the
reason is entirely generic: *"Active workstreams appear to implement the same
behavior under different paths."* It never says the shared behaviour is revoking
credentials on a privilege change. A reader cannot tell why these two workstreams
were linked, which is the one fact that makes the finding actionable. Earlier
synthetic `wrk_eval_*` findings carried richer text (*"Both describe session and
contract work…"*), so a more specific rendering exists somewhere and did not fire here.

Note the provider was `stickguy-concepts/v1`, the vocabulary-bounded offline fallback
— not managed embeddings. Good news for degradation behaviour; it also means the
managed adjudication path remains completely untested.

### B19 — `internal/app` test is flaky under parallel package runs (LOW, OPEN)
`go test ./internal/sessiontranscript/ ./internal/agentactivity/ ./internal/app/`
failed once at `app_test.go:565` with
`service exited: migrate sqlite: database is locked (5) (SQLITE_BUSY)`; the same
package passes alone and on re-run. Go runs packages in parallel by default, so this
will surface as random CI failures.

### B15 — Agents collide with themselves (HIGH, FIXED with B8)
SG-07 produced a `redundant_work` finding (`coordination/v1`, medium, dashboard)
between `wrk_agent_f173fad6` (the Claude session) and `wrk_d38baf33` (the workspace
workstream) — *"Active workstreams appear to implement the same behavior under
different paths."* They are the same agent. MCP `begin_work` wrote intent to the
workspace workstream while hooks created the session workstream, so both carry the
title "Refresh browser session tokens after auth renewal" and the semantic layer
correctly concluded they duplicate each other.

Same root cause as B8/B11. The identity split does not merely break MCP briefs; it
manufactures false `redundant_work` findings against the agent itself. Any session
that calls `begin_work` is a candidate. Severity is limited only because delivery was
`dashboard` and the finding auto-resolved — it never reached an agent turn.

### B13 — Stickguy's own MCP tools generate an approval storm (MEDIUM, OPEN)
Claude Code prompts separately for `begin_work`, `check_coordination`,
`report_checkpoint` and `finish_work` on first use in a repository — four
interruptions from the coordination layer itself, before any edit or shell approval.
Onboarding never mentions it. Consider shipping pre-approved entries in
`.claude/settings.local.json` alongside the `.mcp.json` install.

### B14 — Path evidence is vendor-asymmetric (HIGH, OPEN)
For equivalent SG-07 tasks: Codex reported 1 path (the file it edited); Claude
reported 5 (`README.md`, `backend/audit.go`, `backend/refresh.go`,
`backend/sessions.go`, `frontend/session.ts`), four of them reads it was told not to
touch. Identical behaviour yields roughly 5x the path surface on Claude, so collision
detection is biased toward flagging Claude sessions and mixed-vendor teams get skewed
results. Compounds B10.

### B12 — Behaviour change is not attributable in SG-03 (RESOLVED)
Re-run with a deliberately neutral step-4 prompt (`continue`). Claude replied:
*"No new coordination items beyond what the hook already surfaced. I'll update
frontend/session.ts to mirror the new `Refresh(sessionID string, policy Policy)
string` signature."* The operator supplied no hint, and the agent credited the hook
explicitly, so the adaptation is attributable to delivered context rather than to the
question. The full chain — contract changed, finding routed, context injected, agent
adapted — is now demonstrated with real agents.

It also handled B8 gracefully here: it called the MCP tool, received nothing, and
attributed the real information to the hook instead of declaring an all-clear as it
did in round 4.

Original note follows.

#### Original observation
Claude did adapt `frontend/session.ts` to the new signature, but its transcript
attributes the diagnosis to re-reading the file, not to the brief, and it explicitly
reported no flagged items (see B8). Because the operator's step-3 question ("is your
implementation still correct?") is itself a strong prompt to re-check, this run cannot
separate a Stickguy-caused fix from one the agent would have made anyway.
Contrast SG-01 round 1b, which *is* attributable: Claude volunteered that a codex
session had the file open, a fact it had no other way to know.
Fix the evaluation, not the product: SG-03's step 3 needs a neutral prompt
(e.g. "continue") so adaptation is not operator-induced.

### B1 — `setup status` misreports a correctly bound repo (MEDIUM, OPEN)
The exact command in `docs/development.md` (no `--config-root`) reports
`"binding":"other_profile"` with `"previousProfile"` naming *the user's own current
profile*. Without the flag the CLI assumes the portable/installed layout and expects
a bare `stickguy mcp` entry, so `classifyServer` falls through to `other_profile`
(`internal/claudesetup/setup.go:333`).
Suggested: when the resolved `previousProfile` equals the current root, report `current`.

### B5 — `stickguy create` cannot add a second Project (MEDIUM, OPEN)
Fails with `device ID does not match registered service device`
(`internal/app/app.go:1050`) because `create` always enrolls a fresh device.
The working verb is `workspace add --development --root <dir> --project <id>`,
which appears nowhere in the docs.

### B6 — Self-contradicting error from `workspace add` (LOW, OPEN)
`add stopped-service development workspace after IPC unavailable: register workspace:
service already running`. "IPC unavailable" and "service already running" contradict
each other in one message.

### B2 — Adapter detection unverified for desktop-only Codex (MEDIUM, OPEN)
This machine has no `codex` on PATH; the only binary is inside
`/Applications/ChatGPT.app`. `docs/development.md` claims detection covers PATH plus
standard macOS app locations. The desktop-only case is the most likely real-world
configuration and needs an explicit test.

### B4 — `pnpm dev` fails on a default shell with no guidance (MEDIUM, OPEN)
A plain shell here resolves Node 14; `pnpm dev` aborts with
`This version of pnpm requires at least Node.js v18.12` and no hint that `nvm use`
is the fix. First command a new contributor runs.

### B11 — Workspace workstream absorbs agent intent (MEDIUM, FIXED with B8)
The workspace-level workstream took Claude's title ("Add TTL argument to
backend.Refresh"). MCP intent lands on the workspace identity while hook path
evidence lands on the session identity. Same root cause as B8; also renders a
confusing duplicate row.

## Testing hygiene (not product bugs)

- Do not place fixtures under `/private/tmp` — it was purged mid-session, taking the
  fixture and scratch files with it.
- In the Codex desktop app, **archive the chat between scenarios**. Ctrl+Esc is
  sufficient for the Claude CLI.

## Scenario results

| # | Scenario | Detect | Route | Deliver | React |
|---|---|---|---|---|---|
| 1 | SG-01 direct collision | PASS | PASS | not exercised | unclear |
| 1b | SG-01 next-turn injection | PASS | PASS | PASS (hook) / FAIL (MCP) | PASS |
| 2 | SG-03 stale contract after read | PASS | PARTIAL (B10) | PASS | unclear (B12) |
| 3 | SG-07 lexical false neighbour | PASS (no cross-agent link) | n/a | PASS (no injection) | n/a — but one false dashboard `redundant_work`, see B15 |
| 4 | SG-06 semantic duplicate | **MISS** | n/a | n/a | n/a — root cause B16, since fixed |
| 5 | SG-06 re-run after B16/B18 | **PASS** | PASS | not exercised | n/a — content partial, see B20 |
| 6 | SG-03 re-run after B10 | **PASS** | **PASS** | **PASS** | **PASS (attributable)** |
| 7 | B14 measurement, symmetric mixed-vendor | n/a | n/a | PASS (silence) | n/a — path ratio 1:1, B14 dissolved |
| 8 | SG-01 after B8 | **PASS** | **PASS** | **PASS (MCP pull, first time)** | **PASS — agent halted and escalated** |

## Not yet run

SG-02 same-file unrelated regions, SG-05 new unrelated export, SG-08 identifier
false neighbour, SG-12 fan-out routing,
SG-14 unsupported adapter, SG-16 pause boundary, SG-17 offline recovery.
Negative/silence scenarios are the most important remaining gap: nothing so far
tests whether Stickguy stays quiet when it should.
