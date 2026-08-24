# Official OpenAI documentation consulted

Accessed 2026-08-23:

- Model Context Protocol: https://learn.chatgpt.com/docs/extend/mcp?surface=cli
  - stdio supported;
  - initialization `instructions` consumed by Codex;
  - first 512 instruction characters should be self-contained;
  - trusted project `.codex/config.toml` supports project MCP scope;
  - desktop app, CLI, and IDE share MCP configuration on one Codex host.
- Hooks: https://learn.chatgpt.com/docs/hooks
  - project `.codex/hooks.json` location and trust review;
  - `SessionStart` sources `startup`, `resume`, `clear`, `compact`;
  - `SubagentStart` context semantics;
  - command stdin/stdout shapes, timeout, and `additionalContextLimit`;
  - oversized context may be written to disk, so output must never contain
    secrets or prohibited data.
- Configuration reference: https://learn.chatgpt.com/docs/config-file/config-reference
  - project config loads only for trusted projects;
  - project config cannot override user notification/telemetry/provider keys.

Official documentation reflects the current surface; the executable evidence
is pinned to Codex `0.148.0-alpha.15` and must not be generalized to an untested
version range.

Official MCP SDK dependency validated:

- module: `github.com/modelcontextprotocol/go-sdk`
- pinned stable version: `v1.7.0`
- exercised APIs: `mcp.NewServer`, typed `mcp.AddTool`, `mcp.ServerOptions`
  initialization instructions, and `mcp.StdioTransport`
- version was selected from published non-prerelease Go module versions; no
  prerelease was substituted.
