# L5A live client evidence — 2026-08-24

## Scope and consent

The owner explicitly approved reading current Codex tasks and confirmed an
authenticated Claude CLI. Both probes were read-only with respect to the
Stickguy repository and used sanitizer processes that discarded values before
terminal output. The Claude model/tool run used only a disposable synthetic Git
repository, disabled session persistence, and was capped at USD 0.50.

No account email, organization identifier/name, subscription type, session or
thread ID, prompt/message/reasoning text, transcript path/content, tool input or
result, command line/output, source/diff, environment value, credential, or
vendor raw JSON was retained.

## Codex App Server

- Contract reference: [official Codex App Server documentation](https://learn.chatgpt.com/docs/app-server).
- Client: Codex CLI `0.148.0-alpha.15`.
- Initialization with analytics disabled: pass.
- Exact-working-directory `thread/list`: pass; 2 tasks, both normalized source
  `vscode`, both normalized status `notLoaded`.
- `thread/read` with turns for one internally selected task: pass; 9 turns.
- Sanitized item counts: 96 `agentMessage`, 6 `contextCompaction`, 189
  `fileChange`, 136 `reasoning`, 9 `userMessage`, 36 `webSearch`, and 31 unknown
  item kinds. Unknown means deliberately not retained by the sanitizer; no raw
  type or value was recorded.
- Values discarded and analytics disabled: true.

This proves supported enumeration/read of existing Stickguy tasks without
parsing transcript files. It does not prove that a newly launched App Server can
subscribe to the private event stream of another already-running App Server
process. Treat Codex realtime cross-process fidelity as narrowed.

## Claude Code hooks

- Contract reference: [official Claude Code hooks documentation](https://code.claude.com/docs/en/hooks).
- Client: Claude Code `2.1.197`.
- Authentication check: logged in via `claude.ai`; identity and organization
  fields were discarded.
- Synthetic session: exit 0, no session persistence, strict empty MCP config,
  explicit Read/Bash/Agent tool allowlist, synthetic repository only.
- Sanitized stream: 54 JSON lines; Read 2, Bash 1, Agent 1; content and IDs
  discarded.
- Independent command-hook records:
  - `SessionStart` 1 and `SessionEnd` 1;
  - `UserPromptSubmit` 2;
  - `PreToolUse` 4 and `PostToolUse` 4, split into read 2, shell 1, subagent 1;
  - `SubagentStart` 1 and `SubagentStop` 1;
  - `Stop` 2.

This proves project-configured hooks can observe lifecycle, visible prompt
submission, tool category, and subagent lifecycle for a supported Claude Code
session without Stickguy launching or steering the model loop. Permission denial
and partial-message behavior remain unproved and fail closed.

## Local policy and configuration proofs

The nested Go 1.26 module passes `go test ./...`, `go vet ./...`, and
`go test -race ./...` with an isolated Go cache. Tests prove:

- exact profile projection, owner/member minimum, pause/downgrade, Project
  isolation, protected path/secret rejection, unknown-kind failure;
- no prohibited candidate reaches store or sender; preview has no side effect;
- bounded retention and Project-scoped deletion; and
- structural Claude hook install/removal that preserves unrelated settings and
  permissions and refuses duplicates or drift.

## Privacy review

The broad read permission was used only to validate supported APIs. It does not
change ADR-027's data contract: system prompts, hidden reasoning, transcript
files, source/diffs, tool results, raw commands/output, environment values, and
credentials remain prohibited. The probes demonstrate availability, not
authorization to retain or share those values.
