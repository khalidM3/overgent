# Stickguy

Stickguy is a persistent coordination harness for teams building software with coding agents. It acts as air traffic control around existing Codex, Claude, Cursor, and other coding harnesses: combining live Git evidence, reported intent, and semantic coordination intelligence, then routing only relevant findings and decisions to each workstream before merge time.

The repository has completed its L-1 architecture validation and is building the L0 contract scaffold. Start with [`AGENTS.md`](AGENTS.md) and read [`docs/README.md`](docs/README.md) in order.

Core decisions: persistent Projects; standalone Go local core; one service per user; React dashboard and Convex backend; deterministic evidence plus V1 semantic coordination over bounded summaries; no raw transcript, system-prompt, diff, or source-content collection in V1. The intended trust model publishes all installed/collection code and core hosted coordination code while isolating private cloud operations in a separate repository.

Implementation follows [`docs/implementation-plan.md`](docs/implementation-plan.md).

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
