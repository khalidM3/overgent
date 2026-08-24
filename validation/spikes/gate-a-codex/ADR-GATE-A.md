# Gate A outcome — Narrow Codex adapter to proven MCP surface

Status: draft validation outcome for integrator review
Date: 2026-08-23
Decision: **NARROW**

## Evidence

Codex `0.148.0-alpha.15` discovered the trusted project-scoped Go stdio MCP,
implemented with pinned official `github.com/modelcontextprotocol/go-sdk`
v1.7.0 (`mcp.NewServer`, typed `mcp.AddTool`, and `mcp.StdioTransport`). A real
ephemeral client called fixture `begin_work`,
`check_coordination`, `report_checkpoint`, and `finish_work`; only redacted
lifecycle metadata was retained. Initialization instructions front-load the
critical workflow/privacy boundary in the first 512 characters. Automated tests
prove explicit cwd resolution, ambiguity failure, bounded schemas, prohibited
argument rejection, and per-process idempotency.

Official current OpenAI documentation states that the desktop app, CLI, and IDE
share host MCP configuration. Desktop GUI discovery was not independently
exercised in this non-UI lane.

The reviewed `SessionStart`/`SubagentStart` hook executable emits bounded
fixture context and ignores the supplied transcript path. Direct fixture tests
and a visible missing-cwd degradation assertion pass. A disposable Git project's
MCP config loaded after a one-invocation inline-table trust override, without
persisting user config. Neither project `hooks.json` nor equivalent inline hook
configuration produced a SessionStart marker during ephemeral `codex exec`.
A synthetic subagent attempt reached Codex's hook runtime, which reported that
the ephemeral parent had no stored rollout for resolving its parent transcript
path; no SubagentStart marker appeared. A non-ephemeral retry was rejected by
the gate's no-transcript evidence boundary.

## Outcome

Accept the official-SDK Codex stdio MCP as the Gate A portable high-fidelity
surface for this pinned build. Narrow hook capability to
`available_but_unverified` until L5 runs
the same fixture at a trusted isolated worktree root and proves startup, resume,
compact, and subagent delivery plus visible timeout/service/stale/oversize
degradation. Do not advertise hook fidelity or install hooks before that test.

MCP-only lifecycle plus Git/manual observation is the existing fallback, so this
narrowing does not replace Go, the public MCP protocol, Project terminology, or
the coordination-harness boundary.

## Required follow-up before production hook enablement

1. Repeat from a trusted isolated worktree root in the desktop app and CLI.
2. Prove exact structured merge/status/removal against unrelated config and
   refuse drift rather than overwrite.
3. Exercise hook failure, service absence, stale brief, timeout, oversized
   output, startup/resume/compact, and `SubagentStart` with automated assertions.
4. Pin the accepted Codex version/range from evidence; revalidate on upgrades.
5. Keep every content-bearing or execution-controlling hook disabled pending a
   separate privacy/control ADR and explicit consent.
