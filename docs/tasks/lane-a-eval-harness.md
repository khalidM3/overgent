# Lane A — M1 coordination eval harness

Goal: a repeatable, headless, one-command suite that runs seven two-agent
coordination scenarios against the real local stack and reports per-scenario
pass/fail plus precision metrics. This suite is the permanent gate for M2–M5.

## Read first

- `docs/implementation-plan.md` (M1 section) and ADR-044–048 in
  `docs/decisions.md`
- `validation/evals/` — existing eval seed and runner conventions
- `docs/development.md` — the loopback stack and the two-worktree
  Codex/Claude collision exercise; reuse its orchestration, do not invent a
  second way to boot the stack
- The existing anonymous loopback two-device live suite (find it from
  `docs/development.md`; it proved the L6 exit cases)

## Design (decided — do not revisit)

- **Scripted agents, not model-driven agents.** Each scenario drives two
  simulated agent sessions through the real service: real file edits in two
  linked worktrees of a fixture repository, real MCP lifecycle calls
  (`begin_work`, `update_intent`, `report_checkpoint`, `check_coordination`,
  `finish_work`), and hook-shaped activity events where a scenario needs a
  read-set signal. Determinism over realism; model-driven runs are a later
  layer.
- **Fixture repo**: one small module with a `backend` package exposing an
  exported function/type and a `frontend` package consuming it. Committed
  under the suite's fixtures directory and materialized into temp worktrees
  per run, following existing temp-repo test conventions.
- **Capability-tagged assertions.** Every assertion carries a capability tag:
  `structural` (exists today), `contract` (Lane B / M2), `injection`
  (Lane C / M3), `semantic` (M4), `dependency` (M5). The runner reports
  pass / fail / `not_yet_implemented` per tag, so the suite lands green now
  and tightens as lanes merge. A capability flips from `not_yet_implemented`
  to required via a small config in the suite, changed by the integrator.
- **One command.** Wire a root `package.json` script `eval:coordination`
  (or a documented `go run ./validation/evals/coordination` if that matches
  existing conventions better — pick whichever the existing suite uses).
  Output: human-readable table plus a JSON report file.

## Scenarios (all seven required)

Each scenario defines: setup, the two agent scripts, expected findings
(kind, target workstream), expected routing (interrupt / next-turn /
dashboard-only / silence), and the adjustment probe where applicable.

- **A — contract change under a consumer.** WS1 begins work consuming
  `backend.Refresh(userID)`; records reading the backend file. WS2 changes
  the signature to `Refresh(sessionID, policy)` and checkpoints. Expect:
  `stale_assumption` [contract] routed to WS1 naming the symbol with old/new
  signature; WS1's next `check_coordination`/injected context contains it
  [injection].
- **B — semantic duplication.** WS1 implements credential revocation in
  `backend/revoke.go`; WS2 implements equivalent behavior in
  `frontend/session_cleanup.ts` with different naming. Expect:
  `redundant_work`-class finding [semantic] with explanation; no interrupt of
  either agent mid-turn, dashboard + next-turn delivery.
- **C — interface changed after session start.** Same mechanics as A but WS1
  read the interface at `begin_work` and WS2's change lands mid-scenario;
  expect the finding within one publish cycle of WS2's checkpoint [contract],
  delivered inside WS1's next turn boundary [injection], and the WS1 script's
  adjustment probe (it re-reads the contract and reports a changed intent)
  observed [injection].
- **D — dependency wait.** WS1 declares `waiting_on` the session API
  contract; WS2 later creates the exported symbol and checkpoints. Expect:
  `dependency_ready` [dependency] routed to WS1 including the contract; a
  stable-but-WIP notice when WS2 reports an unverified checkpoint first.
- **E — true independence.** Disjoint paths, unrelated intents. Expect: zero
  findings routed to either workstream [structural]. Any routed item fails
  the scenario. This is a hard assertion, not a warning.
- **F — same file, unrelated regions.** Both edit one file; exported
  contracts unchanged. Expect: existing same-path overlap finding at
  non-interrupt severity [structural]; no `stale_assumption` [contract]; no
  injected interrupt [injection].
- **G — WIP uncertainty.** WS2 publishes an unverified checkpoint touching a
  contract WS1 read. Expect: finding communicates WIP/uncertainty state
  rather than presenting the change as final [contract]; severity below
  scenario A/C.

## Metrics in the JSON report

Per scenario: expected vs actual findings; correct-target rate;
false-interrupt count (any interrupt-level routing in E/F);
silence-honored boolean; context-sufficiency assertions (finding evidence
contains the named symbol/path); adjustment-probe result; wall time.
Aggregate: precision = correctly-routed / all-routed across the run.

## Acceptance criteria

1. `pnpm eval:coordination` (or the documented equivalent) boots the
   loopback stack, runs all seven scenarios headlessly, and exits nonzero on
   any required-capability failure.
2. Scenarios E and F pass with `structural` required today; A–D and G run
   with future capabilities reported as `not_yet_implemented`, not skipped
   silently.
3. Repeat-run stability: three consecutive runs produce identical pass/fail
   results.
4. Standard checks pass (`go test ./...`, `go vet ./...`, `pnpm typecheck`,
   `pnpm test`, `pnpm build`). No protocol changes in this lane.

## Out of scope

Real Codex/Claude model runs; CooperBench import (note it as follow-up in
the handoff); any change to service, protocol, or dashboard code.
