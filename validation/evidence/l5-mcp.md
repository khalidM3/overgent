# L5 MCP evidence

Date: 2026-08-23

## Outcome

NARROW. The production lifecycle core, official-SDK conformance, stdio subprocess proof, durable lifecycle tests, workspace ambiguity checks, configuration merge/removal tests, and current client configuration verification pass. The explicitly approved credentialed smoke disproved the assumption that the current Codex client can complete the lifecycle through this bridge, while the installed Claude client is unauthenticated. Production agent setup is withheld and ADR-016's deterministic Git/manual fallback remains selected. L6 production work has not started.

## Delivered behavior

- The Go executable serves one stdio MCP bridge built with pinned `github.com/modelcontextprotocol/go-sdk v1.7.0` and front-loads privacy, advisory-only, checkpoint, ambiguity, hook-disabled, and Git/manual fallback instructions within the first 512 initialization characters.
- The bridge exposes `begin_work`, `update_intent`, `check_coordination`, `report_checkpoint`, `acknowledge_context`, `finish_work`, and `report_event`. It sends only bounded structured fields over the existing mode-0600 local IPC socket.
- Workspace resolution accepts an explicit registered ID or exactly one canonical current-directory containment match. Missing and ambiguous matches fail without guessing.
- SQLite persists lifecycle events and `(workspace, method, idempotency key)` request hashes atomically. Intent updates require the current revision. An exact retry returns the prior revision without another event; a changed request under the same key fails.
- Checkpoint verification is bounded structured metadata. Server-assigned `observedAt` values are filled only after idempotency comparison, so an exact retry remains identical. `finish_work` atomically publishes a final checkpoint/verification event, completion outcome, and done status before requesting the unresolved finish brief.
- Hosted brief reads use the frozen authenticated `/v1/workstreams/{id}/briefs` contract. When the hosted provider is absent or unavailable, mutations remain durable and the tool returns `degraded: true` with `hosted_coordination_unavailable`.
- The Codex adapter implementation manages one exact marked project TOML block, preserves unrelated bytes, refuses drift, installs no hooks, and reports `disabled_unverified` in isolated tests.
- The Claude adapter implementation structurally merges one project `.mcp.json` stdio entry, preserves unrelated JSON/server entries, refuses drift, and reports `required_by_claude` in isolated tests.
- Production `setup codex` and `setup claude` commands fail closed with the narrowed L5 outcome. Status/removal remain available to inspect or clean isolated validation entries.

## Executable evidence completed

Official SDK in-memory conformance lists and calls all seven tools. Closing the MCP client/server sessions leaves the local service healthy. The production binary subprocess test connects through the official SDK `CommandTransport`, lists seven tools, calls `begin_work`, exits the MCP process, and proves the service remains healthy:

```text
STICKGUY_BINARY=/private/tmp/stickguy-l5 go test -count=1 -run 'TestProductionBinaryStdioTransport|TestOfficialSDKListsAndCallsAllLifecycleTools|TestWorkspaceResolutionNeverGuesses' -v ./internal/mcp
PASS
```

Focused lifecycle, storage, hosted-client, setup-adapter, command, and MCP tests pass. They cover exact retry, changed-key conflict, stale intent revision, hosted/degraded briefs, acknowledgement bounds, atomic finish evidence, schema-valid queued envelopes, and setup drift refusal:

```text
go test -count=1 ./internal/store ./internal/mcp ./internal/app ./internal/codexsetup ./internal/claudesetup ./internal/hosted ./cmd/stickguy
PASS
```

Final local frozen matrix:

```text
go mod tidy
go test ./...
go vet ./...
go test -race ./...
CI=true pnpm install --frozen-lockfile
CI=true pnpm typecheck
CI=true pnpm test
CI=true pnpm build
CI=true pnpm protocol:check
```

All pass. The credential-gated real-client test is skipped in the ordinary Go matrix because its expected current outcome is vendor-client narrowing rather than a green build requirement. `go mod tidy` required one approved download of missing transitive test metadata for the pinned SDK and is byte-stable afterward.

Installed client contract checks:

- Codex CLI: `0.148.0-alpha.15`; the configuration matches the project-scoped stdio form documented by OpenAI and previously proven for a self-contained L-1 fixture server.
- Claude Code: `2.1.197`; `claude mcp get stickguy` parses the disposable project `.mcp.json` as `Scope: Project`, `Type: stdio`, and honestly reports `Pending approval`.
- No config check retained credentials, prompts, transcripts, session identifiers, source, diffs, command output, or environment values.

## Credentialed real-client result

The user explicitly approved the bounded external-model smoke and a later Codex workspace-write retry. `TestRealCodexAndClaudeLifecycle` created only a disposable synthetic Git repository/config root and a local no-egress brief fixture. It retained normalized tool names/counts, argument key names, fixture match booleans, result shapes/categories, and durable counts; raw client output, prompts generated at runtime, transcripts, session IDs, and credentials were discarded.

Codex discovered and invoked:

```text
acknowledge_context=1 begin_work=1 check_coordination=1 finish_work=1
report_checkpoint=2 report_event=1 update_intent=1
```

Every known synthetic argument value matched. All calls were reported by Codex as a generic `MCP tool call` failure and produced zero Codex idempotency rows. This remained true under read-only and bounded workspace-write access to the disposable repo/state. Immediately before Codex, an official-SDK `CommandTransport` client used the same built binary/config/service to read a brief and durably publish one intent; the database contained exactly that one preflight idempotency row afterward. Therefore the service, SQLite mutation, socket, configuration, and production stdio bridge passed while the current Codex client boundary failed without a diagnostic specific enough to justify reshaping the protocol.

Claude Code exited without a tool call. A separate read-only `claude auth status` returned only `loggedIn: false`, `authMethod: none`, so model-driven Claude support is unavailable rather than disproven. Its project configuration parsing remains verified.

## Security, privacy, and limits

- The MCP process never reads coding-agent transcripts or environment values and never receives source/diffs unless a client violates the documented input contract. Tool schemas and local validators bound every accepted list/string and prohibit raw verification output by instruction and shape.
- Default-root agent config contains the portable `stickguy mcp` PATH command. Explicit isolated-root setup contains the requested absolute executable/config paths. Neither form contains a device token; the service obtains its device credential from macOS Keychain.
- Project trust/approval remains owned by Codex and Claude. The withheld setup path does not weaken permission modes or approve its own server.
- Hooks remain unavailable because ADR-016's trusted hook-injection proof did not pass. No hook file, transcript lookup, session hook, or subagent hook is installed or advertised.
- The local service and HTTP transport already have L4 hosted integration proof. The narrowed real-client result does not invalidate the working lifecycle core, but it prevents claiming Codex/Claude adapter support.
- Unsupported coding agents retain deterministic Git/manual fidelity. Linux and Windows runtime support remains narrowed under ADR-019.
