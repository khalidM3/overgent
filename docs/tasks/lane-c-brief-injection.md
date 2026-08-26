# Lane C — M3 push delivery into agent turns

Goal: pending relevant coordination items reach the affected agent *inside
its next turn* via vendor hooks, per ADR-046 — observation in, coordination
context out. No more waiting for the agent to call `check_coordination`.

## Read first

- ADR-046 in `docs/decisions.md`
- `internal/hookconfig/` — how Claude Code and Codex hook groups are
  installed and what command they invoke
- The hook handler entrypoint in `cmd/stickguy` / `internal/app` — how hook
  stdin JSON is parsed today and what is written to stdout
- `internal/agentactivity/` — session→workstream resolution from hook events
- The brief renderer and delivery/acknowledgement state from L6
  (`convex/functions/intelligence.ts`, related Go client paths) — reuse it;
  do not invent a second rendering or ack model
- Current official Claude Code hooks documentation for the exact
  `hookSpecificOutput.additionalContext` JSON shape on `UserPromptSubmit`
  and `SessionStart` (verify against the installed version, do not trust
  memory)

## Design (decided — do not revisit)

- **Claude Code first.** Extend the existing `UserPromptSubmit` and
  `SessionStart` hook handling: after recording observation (unchanged), the
  handler asks the local service for pending brief items for the resolved
  workstream and, when any exist, emits the vendor JSON that injects
  bounded additional context into the turn.
- **Fail open, hard time budget.** The whole handler (observation + fetch +
  render) must complete within 2 seconds; on timeout, service-down, or any
  error, emit nothing extra and exit 0. A coordination tool that breaks the
  agent's turn is worse than no delivery. Unit-test the timeout path.
- **Bounded content.** Rendered injection uses the existing brief renderer
  and budget rules (respect the 128–800 token budget; cap the injected
  string ≈ 3200 chars). Content states what changed and what to do, e.g.:
  "Coordination update: backend.Refresh signature changed after you read it
  — old: …, new: …, changed by <member>'s session. Re-read the file before
  continuing dependent work."
- **Fetch-through, not a cache.** (Amended 2026-08-26 after inspection
  showed no local brief cache exists.) Add one new method to the existing
  current-user IPC socket surface, following its existing naming/dispatch
  conventions (see `cmd/stickguy/main.go` hook handling and the IPC methods
  in `internal/app`): given the hook payload's session/workspace identity,
  the service resolves the workstream (reusing the existing agent-activity
  resolution) and fetches the current brief from the hosted API using the
  same retrieval path the MCP lifecycle already uses
  (`internal/app/app.go` brief fetch via `hosted_sender.go`), under a hard
  1500 ms context timeout. Any error, timeout, or offline state returns an
  empty result — the fail-open budget covers the offline case; no local
  brief mirror is built in this lane.
- **No repeat delivery.** Add one SQLite table via the existing migration
  mechanism in `internal/store`: `injection_deliveries` with columns
  (workstream/session key, brief item id, item revision, delivered_at) and
  a uniqueness constraint on (session key, item id, revision). Before
  emitting, filter out items whose current revision is already recorded;
  after emitting, record what was injected. A revised item may be injected
  again. If the existing hosted delivery/acknowledgement API is callable
  from the local service, also mark delivery through it with channel
  semantics unchanged; if that requires new wire fields, skip it, rely on
  the local table alone, and note it in the handoff. Do not modify
  `protocol/`.
- **Hook handler output.** The current hook handler emits no stdout. Extend
  it to emit the vendor JSON only when items pass the dedup filter;
  otherwise keep emitting nothing, preserving current observation behavior
  byte-for-byte.
- **Both vendors.** Claude Code (verified 2.1.197) and current Codex CLI
  both document `hookSpecificOutput.additionalContext` for `SessionStart`
  and `UserPromptSubmit`. Implement injection for both, verifying each
  vendor's exact documented JSON shape against its current official
  documentation at implementation time. If Codex verification fails in
  practice, narrow Codex honestly in the handoff and
  `docs/development.md` (MCP pull + dashboard remains its channel); never
  simulate injection through unsupported channels.

## Acceptance criteria

1. Unit tests: handler emits the exact vendor JSON shape when items are
   pending; emits plain observation output when none are; emits nothing
   extra and exits 0 on service timeout (enforced with a fake slow service).
2. Loopback integration test: create a pending high-relevance item for a
   session's workstream → invoke the hook handler with a realistic
   `UserPromptSubmit` payload → injected context contains the item; second
   invocation at the same revision injects nothing; bump the item revision →
   injected again.
3. Delivery state visible: after injection, the item shows delivered state
   through the existing dashboard/API surface.
4. Hook installation (`internal/hookconfig`) unchanged in shape unless the
   vendor contract requires a new event registration; if it does, preserve
   the existing drift-safe merge/removal guarantees and their tests.
5. All standard checks pass. No `protocol/` changes.

## Out of scope

What counts as "high-relevance" (use existing severity/routing state as-is;
M4 refines it); Codex workarounds beyond documented surfaces; dashboard UI
beyond existing delivery state display.
