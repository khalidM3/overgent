# L5A — opt-in agent activity adapter validation

Status: NARROW complete under ADR-028; production collection and setup remain
disabled until versioned contracts, generated code, controls, and integration
tests are implemented.

This isolated spike implements only a vendor-neutral, fail-closed profile
projector and records redacted capability evidence. It is not imported by the
production Go module and defines no shared protocol.

## Current capability evidence

- Codex CLI `0.148.0-alpha.15` generated its non-experimental App Server JSON
  Schema successfully into a disposable directory. The schema exposes thread,
  turn, item, plan, user/agent message, command execution, file change, MCP tool,
  subagent/collaboration, approval, token usage, reasoning summary, and raw
  reasoning event shapes.
- After explicit approval, a sanitizer-fronted App Server process initialized,
  listed two existing Stickguy tasks by exact working directory, and read one
  task's structured turns. It retained only task/source/status and item-kind
  counts. App Server read coverage passes; subscription to another already
  running App Server process was not proved, so live cross-process Codex
  observation is narrowed to refresh/read plus MCP/Git/manual fallback.
- Claude Code `2.1.197` was authenticated through `claude.ai`. In a disposable
  synthetic Git repository with session persistence disabled, it used Read,
  Bash, and one Agent subagent. Command hooks independently recorded
  `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`,
  `SubagentStart`, `SubagentStop`, `Stop`, and `SessionEnd`. The stdout sanitizer
  retained only stream event classes and tool names. Project hook installation
  can therefore observe supported independently started Claude Code sessions;
  the Agent SDK is unnecessary for this coverage and Stickguy need not own the
  model loop.

Only event names, versions, normalized success/failure, aggregate schema
capabilities, authentication boolean/method, and event counts are recorded here.
Account identity, organization/subscription data, random IDs, raw JSONL,
prompts/messages/reasoning, filesystem paths from vendor events, local
installation data, and transcript locations were discarded. See
`evidence/2026-08-24-live-clients.md`.

## Policy projector

The `policy` package proves the pre-storage/pre-enqueue boundary for the three
profiles in `docs/agent-activity-sharing.md`. It applies the narrower owner/member
profile, synchronous pause, exact Project scope, event/field allowlists, bounded
visible text, safe repository-relative paths, protected-path rejection, common
secret markers, and whole-event rejection for transcript/system/reasoning/source/
diff/tool-result/raw-command/output candidates. Unknown vendor kinds fail closed.

The `Boundary` test double proves projection occurs before either durable storage
or sending. It also proves preview has no side effect, pause/downgrade is
synchronous, retention is bounded, deletion is Project-scoped, and rejected data
never crosses the sink boundary. The `adapterconfig` package proves structural
Claude hook merge/removal, unrelated setting/permission preservation, duplicate
install refusal, and drift-safe removal in isolated JSON fixtures.

Run:

```bash
go test ./...
go vet ./...
go test -race ./...
```

## Narrow outcomes carried into production

- Codex cross-process realtime subscription and independently running CLI task
  notification delivery are unproved. Production must use bounded App Server
  refresh/read for supported tasks and the existing MCP/Git/manual fallback.
- Claude permission-denial and partial-message cases were not forced in the live
  run. Unknown or absent events fail closed; they cannot be advertised until a
  fixture proves them.
- Conversation text was proved only at the local projector boundary, not through
  a production vendor adapter, queue, hosted contract, retention job, or UI.
- This spike does not enable collection. Shared schemas/generated code, explicit
  owner/member consent UI, preview/inspection/deletion, production setup, and
  end-to-end security tests remain implementation work.
