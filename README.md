<p align="center">
  <img src="assets/brand/overgent-mark.svg" width="112" alt="Overgent logo">
</p>

# Overgent

Overgent is air traffic control for multiple coding agents working in one repository. It catches overlapping edits, stale contract assumptions, and duplicated work while agents are working—not at merge time. It observes and routes coordination context; it never edits code, merges changes, or steers an agent's model loop, tools, or permissions.

## Install

macOS and Linux:

```bash
curl -fsSL https://overgent.com/install.sh | sh
```

Windows, in PowerShell:

```powershell
irm https://overgent.com/install.ps1 | iex
```

The installer verifies the signed release manifest, the archive hash, and the
executable signature before it changes anything. Desktop applications are at
[overgent.com](https://overgent.com#download).

| Platform | Status |
| --- | --- |
| macOS 12+, Apple silicon | Qualified on real hardware |
| Linux x86-64 | Builds and passes its tests in CI; not yet run on a Linux desktop by us |
| Windows 10 1809+, x86-64 | Builds and type-checks in CI; not yet run on a Windows machine by us |

Linux and Windows support is complete but unqualified: the code is there and
tested, and we have not yet sat in front of either. Please report what breaks.
For anything else, [build from source](docs/development.md).

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
