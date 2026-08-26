# Stickguy

Stickguy is a persistent coordination harness for teams building software with coding agents. It acts as air traffic control around existing Codex, Claude, Cursor, and other coding harnesses: combining live Git evidence, reported intent, and semantic coordination intelligence, then routing only relevant findings and decisions to each workstream before merge time.

The repository has completed L-1 and L0–L6 and has implemented L8 to its owner-controlled beta gates, including the deterministic vertical slice, MCP lifecycle core, coordination-intelligence loop, signed update/recovery path, fleet/data controls, and Apple Silicon desktop beta. Publication still requires Apple/update signing credentials, a monitored private security channel, clean-machine evidence, and the real-team second-session gate. Start with [`AGENTS.md`](AGENTS.md) and read [`docs/README.md`](docs/README.md) in order.

Core decisions: persistent Projects; standalone Go local core; one service per user; React dashboard and Convex backend; deterministic evidence plus V1 semantic coordination over bounded summaries; no raw transcript, system-prompt, diff, or source-content collection in V1. The intended trust model publishes all installed/collection code and core hosted coordination code while isolating private cloud operations in a separate repository.

Implementation follows [`docs/implementation-plan.md`](docs/implementation-plan.md).

## Coding-agent integration status

The official-SDK MCP lifecycle bridge and bounded Project hook adapters are implemented and locally conformant for dogfooding. The explicit `--development` setup path installs Project-scoped MCP configuration plus supported Codex and Claude Code lifecycle hooks. Hooks observe session state, tool categories, safe repository-relative paths, and an approved bounded session title; relevant coordination briefs remain MCP pull, with the dashboard as the urgent human-attention surface.

Status and cleanup remain available for isolated validation entries:

```bash
stickguy setup status --agent codex --project-root /path/to/project
stickguy setup remove --agent codex --project-root /path/to/project
stickguy setup status --agent claude --project-root /path/to/project
stickguy setup remove --agent claude --project-root /path/to/project
```

Stickguy does not bypass either client's trust boundary or claim an unsupported interrupt channel. See [L5 evidence](validation/evidence/l5-mcp.md) and the current adapter limitations in [`docs/development.md`](docs/development.md).

Adapter setup is profile-aware: partial current-profile entries are repaired,
bindings owned by another Stickguy profile get an explicit reconnect preview,
and the desktop waits for a real provider event before labeling observation as
verified.

For the native hot-reload stack and the two-worktree Codex/Claude collision
exercise, see [`docs/development.md`](docs/development.md).

For the release boundary and owner prerequisites, see [`docs/beta-release.md`](docs/beta-release.md). For a real two-Mac dogfood Project, configure one cloud Convex development
deployment and run `pnpm dev:shared` on both Macs with the same HTTPS
`STICKGUY_SHARED_API_ORIGIN`. This uses an isolated local profile; see the
shared-development section in [`docs/development.md`](docs/development.md).

## Optional managed semantic retrieval

The default local dogfood profile includes the deterministic,
vocabulary-bounded semantic fallback. A hosted deployment can enrich the same
privacy-filtered intent/checkpoint summaries with OpenAI
`text-embedding-3-large`; see [`docs/openai-embeddings.md`](docs/openai-embeddings.md).
The API key is a hosted deployment secret only and is never part of the local
client or agent configuration.

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

The public/private data and repository split is explicit in [`docs/public-repository-boundary.md`](docs/public-repository-boundary.md). Stickguy is licensed under Apache-2.0; see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE). Public launch still requires an operational private security-reporting channel; invited beta publication also requires the owner gates in the beta release guide.
