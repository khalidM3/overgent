# Research brief — recovering file-read observations from Codex

You are researching one narrow, concrete engineering problem for a product called
Stickguy. Work from primary sources and from the software actually installed on this
machine. Do not speculate, and do not invent APIs.

## What Stickguy is

Stickguy is a coordination layer for teams running multiple coding agents in parallel.
A local Go service (macOS, loopback only) passively observes agent sessions in a
registered Git repository and maintains a shared model of what each session is doing:
its intent, the files it has **read**, the files it has **written**, and the exported
contracts it depends on. When one session changes a contract another session already
read, Stickguy routes a correction into the second agent's next turn.

It supports two vendors through separate adapters: Claude Code and Codex.

## The read set, and why it matters

Two path sets per session drive everything:

- **write evidence** — files the agent modified. Drives collision detection.
- **read set** — files the agent inspected. Drives `stale_assumption`: "the contract
  in `backend/refresh.go` changed after this session read it, old signature X, new
  signature Y."

`stale_assumption` is the product's highest-value finding, verified working end to end
with Claude as the reader.

## The problem

**Codex sessions produce an empty read set, so a Codex session can never receive a
`stale_assumption` finding.** If Codex reads a contract and another agent changes it,
Codex is never told.

## What has already been established on this machine — treat as given, but re-verify if cheap

Environment: macOS. Codex is the **ChatGPT desktop app** build,
`/Applications/ChatGPT.app/Contents/Resources/codex`, reporting
`codex-cli 0.149.0-alpha.4.1`. There is no `codex` on PATH. `CODEX_HOME` is `~/.codex`.

1. Stickguy installs user-level hooks at `~/.codex/hooks.json` covering nine events:
   `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`,
   `PostToolUseFailure`, `PermissionRequest`, `Stop`, `SubagentStart`, `SubagentStop`,
   `SessionEnd`. All are trusted (recorded via the Codex app-server).

2. Across a full observed session, Codex reported exactly **two** tool names:
   `Bash` (13 observations, no path metadata) and `apply_patch` (2 observations, paths
   correctly extracted from patch headers). Claude by contrast reports a structured
   `Read` tool carrying `file_path`.

3. Codex performs all file inspection through the shell. A representative command:
   `["/bin/zsh","-lc","pwd && rg -n \"func .*Describe|Audit\" backend --glob '*.go' && sed -n '1,240p' backend/audit.go && …"]`
   Real reads, real paths, buried in compound shell strings with globs and `&&` chains.

4. Codex rollout files (`~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<session-id>.jsonl`)
   contain these record types: `session_meta`, `turn_context`, `world_state`,
   `event_msg`, `response_item`, `compacted`. Within `event_msg`, `item_completed`
   items are typed: `UserMessage`, `AgentMessage`, `Reasoning`, `CommandExecution`,
   `FileChange`. **There is a `FileChange` item for writes and no read analogue.**

5. `world_state` and `turn_context` were inspected directly and carry environment,
   sandbox and permission-profile configuration — `workspace_roots`, `sandbox_policy`,
   `permission_profile.file_system` with `read`/`write` entries. They describe what the
   agent is *allowed* to touch, not what it *did* touch. Ruled out as a source.

6. Extending the existing read-tool classifier is a dead end: there is no structured
   Codex read tool to classify.

## Your research questions, in priority order

1. **Does any Codex version or configuration expose file reads structurally?** A
   `read_file`/`view` tool, a `FileRead` item type, a richer `PreToolUse`/`PostToolUse`
   payload, a config flag that makes Codex prefer a built-in read tool over shell, or a
   feature that is on in the CLI build but off in the desktop build. Check release
   notes, changelogs, and the config schema — this is an alpha, so behaviour may differ
   between 0.149 and both older and newer builds.
2. **Is the hooks API richer than what is installed here?** Enumerate every hook event
   Codex supports at this version and the full payload schema of each. Confirm whether
   nine is the complete set and whether any carries file-access metadata. Determine
   authoritatively whether `PostToolUse` for `Bash` can include structured file access.
3. **The Codex app-server.** Stickguy already talks to it to record hook trust. Document
   its protocol surface and determine whether it exposes file-access, context, or
   read-tracking events a passive local observer could subscribe to.
4. **OpenAI SDKs.** Does the Agents SDK, the Responses API, or any Codex-adjacent SDK
   surface tool-call metadata that identifies files read during shell execution? Be
   precise about which surface applies to a locally-running Codex session versus a
   hosted API call — do not conflate them.
5. **MCP angle.** Codex loads Stickguy's MCP server. Is there any supported mechanism by
   which an MCP server can observe, or be routed, the host agent's file reads?
6. **Anything else supported.** Any sanctioned mechanism producing read evidence without
   parsing shell strings.

## Constraints any proposal must satisfy

- **Passive only.** Stickguy must never block, delay, or alter agent behaviour. Hook
  handlers run on a 2-second budget and fail open.
- **Privacy boundary is the wire.** Reading locally is fine; what may leave the device
  is derived coordination facts only — repository-relative paths, contract
  fingerprints, bounded summaries. Never source, diffs, raw commands, raw tool output,
  environment values, or credentials. A proposal that would ship command text off-device
  is disqualified.
- **Honest fidelity.** Inferred evidence must be labelled lower-fidelity than observed
  evidence and must never be presented as full intelligence.
- **No forked or patched vendor software**, no injected shims into Codex, no CGO, no
  reliance on undocumented private files that a Codex update would silently break —
  unless you explicitly flag it as fragile and say why it is still worth considering.
- Per-vendor adapters are expected and fine; a Codex-specific mechanism is acceptable.

## Deliverable

A written report containing:

1. **Findings per question**, each with a citation: a documentation URL, a changelog
   entry, a version number, or a concrete path on this machine you inspected. Where you
   verified against the installed binary or `~/.codex`, say exactly what you ran or read.
2. **An explicit negative where the answer is no.** "No such mechanism exists in
   0.149.0-alpha.4.1; checked X, Y, Z" is a valuable result. Say "not found" rather than
   producing a plausible-sounding API. Any uncertainty must be labelled as such.
3. **A ranked recommendation**, with the tradeoffs of each option, including whether
   the honest answer is that no supported mechanism exists today.
4. **Version sensitivity** — if a mechanism exists only in some builds, say which, and
   whether the ChatGPT desktop app build can reach it.

## Known fallback, for comparison only

The fallback under consideration is parsing shell command strings from
`CommandExecution` items to infer read paths — heuristic, wrong in both directions,
and it puts a parser on a privacy-sensitive boundary. **Do not simply endorse this.**
Your job is to determine whether something better and supported exists. If it does not,
say so plainly so the fallback can be judged on its merits.
