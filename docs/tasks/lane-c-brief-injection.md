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
- **No repeat delivery.** Mark items delivered using the existing
  delivery/acknowledgement state at their current revision. The same
  revision is never injected twice; a revised item may be injected again.
  Track the channel (`injection` vs `mcp_pull`) **locally only** — do not
  add protocol fields (Lane B owns protocol; if you believe a wire change is
  required, stop and report instead).
- **Local pending cache.** The local service already syncs brief state; the
  hook handler reads pending items from the local service over the existing
  user IPC socket, never directly from the hosted API, so the 2 s budget is
  realistic offline.
- **Codex: verify, then implement or narrow.** Check the current installed
  Codex CLI's hook/config surface for any supported context-injection
  response. If none exists, implement nothing for Codex and document the
  narrowing in the handoff and in `docs/development.md` (MCP pull +
  dashboard remains Codex's channel). Do not simulate injection through
  unsupported channels.

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
