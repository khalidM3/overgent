# Gate A privacy/control review

Collected by fixture code:

- allowlisted lifecycle tool name;
- bounded synthetic idempotency key/workstream ID/summary;
- current working directory only for local registry matching;
- project/workspace/workstream fixture IDs; and
- hook event name, source, agent type, and cwd.

Explicitly not collected, persisted, logged, or transmitted:

- source, diffs, Git objects, paths from repository inspection;
- prompts, raw transcripts, system prompts, assistant messages;
- environment values or user secrets;
- commands, patches, tool arguments/results, raw test output; and
- `transcript_path` contents (the compatibility field is ignored).

Only `SessionStart` and `SubagentStart` are configured. `UserPromptSubmit`,
`PreToolUse`, `PermissionRequest`, `PostToolUse`, `Stop`, `SessionEnd`, and
`SubagentStop` are absent. The hook cannot block, approve, rewrite, stop, or
mutate agent execution. It returns bounded advisory developer context or a
visible degradation message.

The real-client JSONL harness filtered data in memory and preserved only event
type, MCP server name, and fixture tool name. It did not write raw JSONL.

The SubagentStart attempt used `--ephemeral`. Codex reported that this mode had
no stored parent rollout for the hook runtime. A non-ephemeral retry was not
performed because storing the parent transcript would broaden collection beyond
the approved Gate A boundary.
