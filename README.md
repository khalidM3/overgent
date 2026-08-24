# Stickguy

Stickguy is a persistent coordination harness for teams building software with coding agents. It acts as air traffic control around existing Codex, Claude, Cursor, and other coding harnesses: combining live Git evidence, reported intent, and semantic coordination intelligence, then routing only relevant findings and decisions to each workstream before merge time.

The repository has completed L-1 and L0–L6, including the deterministic vertical slice, MCP lifecycle core, macOS desktop preview, and coordination-intelligence loop. Start with [`AGENTS.md`](AGENTS.md) and read [`docs/README.md`](docs/README.md) in order.

Core decisions: persistent Projects; standalone Go local core; one service per user; React dashboard and Convex backend; deterministic evidence plus V1 semantic coordination over bounded summaries; no raw transcript, system-prompt, diff, or source-content collection in V1. The intended trust model publishes all installed/collection code and core hosted coordination code while isolating private cloud operations in a separate repository.

Implementation follows [`docs/implementation-plan.md`](docs/implementation-plan.md).

## Coding-agent MCP status

The official-SDK lifecycle bridge is implemented and locally conformant, but production Codex and Claude setup remains withheld. The explicit `--development` setup path is available for local dogfooding; it installs only project MCP configuration and no hooks. Codex `0.148.0-alpha.15` still has Git/manual fallback fidelity because its current-client durable MCP delivery is narrowed. Authenticated Claude `2.1.197` passed the bounded project hook capability spike, while production optional activity collection remains disabled pending shared contracts and consent controls.

Status and cleanup remain available for isolated validation entries:

```bash
stickguy setup status --agent codex --project-root /path/to/project
stickguy setup remove --agent codex --project-root /path/to/project
stickguy setup status --agent claude --project-root /path/to/project
stickguy setup remove --agent claude --project-root /path/to/project
```

Stickguy does not bypass either client's trust boundary. Hooks are not installed: L-1 accepted MCP plus Git/manual observation while hook delivery remains unverified. See [L5 evidence](validation/evidence/l5-mcp.md).

For the native hot-reload stack and the two-worktree Codex/Claude collision
exercise, see [`docs/development.md`](docs/development.md).

## Development checks

Use Go 1.26 and Node 22 or newer. Corepack supplies the pinned pnpm release.

```bash
go test ./...
go vet ./...
pnpm install --frozen-lockfile
pnpm typecheck
pnpm test
pnpm build
pnpm protocol:generate
pnpm protocol:check
```

`protocol:generate` is the only supported way to update generated Go and TypeScript protocol types. `protocol:check` regenerates into an isolated temporary directory and fails on byte drift. Generated files are committed.

The public/private data and repository split is explicit in [`docs/public-repository-boundary.md`](docs/public-repository-boundary.md). Stickguy is licensed under Apache-2.0; see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE). Public launch still requires an operational private security-reporting channel.
