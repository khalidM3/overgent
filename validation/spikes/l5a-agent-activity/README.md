# L5A — opt-in agent activity adapter validation

Status: in progress; production collection and setup remain disabled.

This isolated spike implements only a vendor-neutral, fail-closed profile
projector and records redacted capability evidence. It is not imported by the
production Go module and defines no shared protocol.

## Current capability evidence

- Codex CLI `0.148.0-alpha.15` generated its non-experimental App Server JSON
  Schema successfully into a disposable directory. The schema exposes thread,
  turn, item, plan, user/agent message, command execution, file change, MCP tool,
  subagent/collaboration, approval, token usage, reasoning summary, and raw
  reasoning event shapes. This is schema-level availability, not a passed live
  attachment.
- A read-only live App Server aggregate probe was stopped: initialization of the
  normal local runtime can contact ChatGPT services and expose installation or
  session metadata. It requires a separate explicit approval. No prompt, path,
  thread ID, transcript, or message content was retained.
- Claude Code `2.1.197` advertises `--include-hook-events`, streamed JSON, partial
  messages, and `--no-session-persistence`. With an isolated `CLAUDE_CONFIG_DIR`,
  synthetic project, no tools, and no session persistence, real `SessionStart`,
  `UserPromptSubmit`, `MessageDisplay`, and `SessionEnd` command hooks ran. The
  client was unauthenticated, emitted only its local authentication-failure
  display, made no model call, and reported zero cost. Tool/subagent coverage is
  therefore unavailable rather than passed.

Only event names, versions, normalized success/failure, aggregate schema
capabilities, and zero-cost/auth state are recorded here. Random IDs, raw JSONL,
prompts/messages, filesystem paths from vendor events, local installation data,
and transcript locations were discarded.

## Policy projector

The `policy` package proves the pre-storage/pre-enqueue boundary for the three
profiles in `docs/agent-activity-sharing.md`. It applies the narrower owner/member
profile, synchronous pause, exact Project scope, event/field allowlists, bounded
visible text, safe repository-relative paths, protected-path rejection, common
secret markers, and whole-event rejection for transcript/system/reasoning/source/
diff/tool-result/raw-command/output candidates. Unknown vendor kinds fail closed.

Run:

```bash
go test ./...
go vet ./...
```

## Remaining gate work

- Explicitly approve or decline a live sanitized Codex App Server attachment.
- Authenticate a disposable Claude account/session before tool, subagent, partial
  message, and successful assistant-message coverage can be claimed.
- Prove adapter removal/config drift, preview/deletion/retention, and a no-egress
  sender/store boundary around the projector.
- Produce a pass/narrow ADR outcome before any production schema or setup change.
