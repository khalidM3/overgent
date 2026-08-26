# Coding-agent adapter development

Stickguy adapters observe and route coordination around a vendor-owned coding
session. They do not start model loops, edit repositories, execute tools,
choose models, approve permissions, or absorb the vendor's trust boundary.

## Contract

Each vendor gets its own adapter under the local Go core. Normalize only the
bounded lifecycle facts the coordination harness understands: session alias,
status, generated action label, allowlisted tool/subagent metadata, safe
repository-relative paths, and vendor-visible title. Classifier-approved
visible session messages use the separate message gate. Never put source,
diffs, Git objects, transcript files, system/developer prompts, hidden
reasoning, environment assignments, credentials, raw commands, or command/tool
output into an event.

Hook handling is time-bounded and fails open for the agent turn. Observation
and context delivery use separate budgets. Unknown events produce no guessed
fidelity. A configured adapter remains pending until the current local profile
records a real accepted vendor event.

## Implementation sequence

1. Add an isolated parser/projector in `internal/agentactivity` or
   `internal/sessiontranscript`; do not share a guessed record format across
   vendors.
2. Add exact managed setup/status/remove/reconnect behavior beside
   `internal/codexsetup` and `internal/claudesetup`. Preserve unrelated files,
   use argument arrays, snapshot before a multi-file reconnect, restore on
   partial failure, and refuse unknown drift.
3. Project the vendor event into existing lifecycle contracts. A new wire field
   requires editing JSON Schema/OpenAPI first and running
   `pnpm protocol:generate`; never edit generated files.
4. Add fixture tests for success, malformed/unknown input, size limits,
   protected paths, secrets, environment assignments, raw output, timeout,
   duplicate delivery, reconnect rollback, and removal preservation.
5. Add the vendor to the real executable coordination eval. Document exactly
   which client version and surfaces were exercised, and label everything else
   unavailable or unverified.

## Verification

```bash
go test ./...
go vet ./...
pnpm protocol:check
pnpm typecheck
pnpm test
pnpm build
pnpm eval:coordination
```

The adapter is beta-ready only when the real client discovers the managed
configuration, observation reaches durable local state, relevant context is
delivered once per revision, removal leaves unrelated configuration byte-for-
byte intact, and Git/MCP/dashboard fallback remains usable when the vendor
surface is absent.
