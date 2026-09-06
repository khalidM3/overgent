# Overgent

Overgent is air traffic control for multiple coding agents working in one repository. It catches overlapping edits, stale contract assumptions, and duplicated work while agents are working—not at merge time. It observes and routes coordination context; it never edits code, merges changes, or steers an agent's model loop, tools, or permissions.

## Install

```bash
curl -fsSL https://overgent.com/install.sh | sh
```

Overgent currently supports Apple Silicon Macs running macOS 12 or later. For other platforms, [build from source](docs/development.md).

## Local by default

A new Project runs against the bundled backend on loopback. Nothing leaves your Mac, and its coordination data stays under your Overgent profile root.

```bash
overgent init --local
overgent status
```

## Team mode

Create or join a team Project on Overgent Cloud when you want to work with teammates. Only derived, structured coordination facts sync; source files, raw diffs, Git objects, transcripts, prompts, credentials, and command output do not. See the full [prohibited-data list](docs/security-privacy.md).

You can also run the same backend yourself; see [self-hosting](docs/self-hosting.md).

## Terminal experience

Run `overgent` inside a registered repository for a contextual status, or use
`overgent projects` outside one. `overgent init` provides guided setup on a
terminal and requires explicit flags in scripts. Human output is used on a
terminal; read commands expose versioned JSON with `--json`.

```bash
overgent help
overgent privacy
overgent completion zsh > ~/.zfunc/_overgent
```

The CLI remains a companion to the Project workroom, not a coding-agent shell.
See the [CLI experience contract](docs/cli-experience.md).

## Bring your own model

Configure semantic judgment for a Project with your own provider key:

```bash
export ANTHROPIC_API_KEY='...'
overgent ai set --judgment-provider anthropic --judgment-model claude-sonnet-4-5 --judgment-key-env ANTHROPIC_API_KEY
overgent ai status --json
```

See [AI providers](docs/ai-providers.md) for Project settings, OpenAI-compatible providers, and how Overgent behaves with no key configured.

## Agents and findings

Codex and Claude Code are supported today. Cursor has a configuration adapter, but its live hook behavior is not yet verified. To add another agent adapter, follow [adapter development](docs/adapter-development.md).

Deterministic findings are always available: path overlap, shared dependencies, and contract evidence. Semantic findings are available only when a provider is configured, and remain explicitly probabilistic.

## Development

Read [the documentation](docs/README.md) in order, then use these checks:

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

See [contributing](CONTRIBUTING.md) and [security reporting](SECURITY.md). Overgent is licensed under [Apache-2.0](LICENSE).
