<p align="center">
  <img src="assets/brand/overgent-mark.svg" width="112" alt="Overgent logo">
</p>

<h1 align="center">Stop coding agents from working past each other.</h1>

<p align="center">
  Overgent is an open-source coordination layer for parallel coding agents.<br>
  It catches conflicting assumptions, overlapping work, and duplicated implementations<br>
  while the work is happening—not after it lands.
</p>

<p align="center">
  <strong>Local-first</strong> · <strong>Works without AI</strong> · <strong>Apache 2.0</strong> · <strong>Self-hostable</strong>
</p>

<p align="center">
  <a href="#quickstart">Get started</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="docs/README.md">Documentation</a> ·
  <a href="docs/self-hosting.md">Self-hosting</a>
</p>

## Your agents can disagree without touching the same file

Agent A changes an authentication contract. Agent B, working in another part of
the repository, builds against the old contract. Agent C starts implementing a
similar feature under a different name.

Git may see three clean branches. The codebase still has three coordination
problems.

Overgent maintains a live model of what each agent is trying to do, what it has
read, what it is changing, and what it depends on. It compares those workstreams
as they evolve and routes a compact, evidence-backed correction only to the
sessions that need it.

```text
Agent A changes a shared contract
                │
                ▼
    Overgent detects the divergence
          ┌─────┴─────┐
          ▼           ▼
 Agent B relied    Agent C is
 on the old API    unrelated
          │           │
          ▼           ▼
 correction at      silence
 the next turn
```

This is air traffic control for coding agents: shared awareness and early
warning, without taking over the aircraft.

## What Overgent catches

| Situation | What Overgent sees | What happens |
| --- | --- | --- |
| Two sessions touch the same code | Paths, symbols, or claims overlap | A direct-collision finding appears with its evidence |
| A contract changes after another session read it | The exported contract fingerprint no longer matches the session's read set | The affected session receives a stale-assumption warning |
| Two agents build the same behavior in different places | Their reported intent and changes overlap in meaning | Optional semantic judgment identifies likely duplicated work |
| Workstreams depend on the same package, schema, route, or API | Shared dependency evidence connects them | Overgent warns the relevant workstreams, not the whole Project |
| One agent is waiting for another | A declared dependency gains stable contract or verification evidence | The waiting session learns that the dependency is ready |

Every finding carries a plain-language reason, provenance, confidence, and the
workstreams it affects. Similarity is candidate evidence, never unexplained
proof. Low-confidence activity stays quiet.

## Built for one developer or a small team

Overgent is useful from the first parallel session. Run Codex and Claude Code
side by side on one Mac, coordinate several worktrees, or share a Project with
a small team working in the same repository.

It is designed for the moment agent throughput outruns human coordination:
startup sprints, hackathons, large refactors, cross-layer features, and any
repository where several fast workstreams are active at once.

Codex and Claude Code adapters are supported today. Git observation and manual
intent remain available when an adapter is absent. Cursor has a configuration
adapter, but its live hook behavior is not yet qualified.

## How it works

1. **Observe locally.** The per-user Go service watches registered Git
   workspaces and receives bounded lifecycle, intent, read, and change evidence
   from connected agents.
2. **Detect divergence.** Deterministic checks find path, symbol, contract, and
   dependency relationships without a model. An optional Project-configured
   provider can judge semantic duplication and architectural conflict.
3. **Route the correction.** Overgent sends a small, versioned coordination
   brief to affected sessions through supported hooks or MCP. Unrelated sessions
   receive nothing.
4. **Record the outcome.** People resolve or dismiss findings in the Project;
   the resulting decision is delivered once to each affected session and kept
   in History.

Overgent coordinates existing coding-agent harnesses. It does not run a model
loop, execute tools, approve permissions, edit code, or merge, rebase, reset, or
otherwise mutate anyone's worktree.

## Quickstart

macOS and Linux:

```bash
curl -fsSL https://overgent.com/install.sh | sh
cd /path/to/your/repository
overgent init
```

Windows, in PowerShell:

```powershell
irm https://overgent.com/install.ps1 | iex
```

Desktop applications for all three platforms are at
[overgent.com](https://overgent.com#download).

| Platform | Status |
| --- | --- |
| macOS 12+, Apple silicon | Qualified on real hardware |
| Linux x86-64 | Ships and passes its tests in CI; not yet run on a Linux desktop by us |
| Windows 10 1809+, x86-64 | Ships and type-checks in CI; not yet run on a Windows machine by us |

Windows binaries are not code-signed yet, so SmartScreen warns on the desktop
app and the PowerShell installer refuses to run until a signed build exists.
Until then, Windows users can download the desktop app and CLI archive directly
from [the latest release](https://github.com/khalidM3/overgent/releases/latest).

`overgent init` guides you through creating a local Project, connecting detected
agents, and opening the workroom. A local Project uses the bundled loopback
backend: it requires no account, no API key, and no network connection.

Then start two supported agent sessions in the registered repository. Overgent
discovers each session as its own workstream. From a terminal, you can ask the
same question as the workroom:

```bash
overgent status
overgent watch
overgent open
```

Use `overgent setup codex` or `overgent setup claude` to connect an adapter
later. Existing agent sessions must restart once after setup so they can load
the integration.

### Team Projects

Create a team Project when the work spans people or computers:

```bash
overgent init --team
```

The hosted default is operated from the same public code in this repository.
You can instead deploy that backend to your own Convex project or infrastructure;
see [self-hosting](docs/self-hosting.md).

## Local-first by architecture

Raw material may be inspected on your machine to derive coordination evidence.
The wire is the privacy boundary.

For a team Project, Overgent may sync bounded coordination facts such as intent,
safe repository-relative paths, exported contract fingerprints, dependency
claims, bounded change summaries, finding evidence, and delivery state.

It does **not** sync raw source files, raw diffs, Git objects, transcript files,
environment values, credentials, private keys, raw tool results, or command
output. A mandatory, non-disableable classifier rejects secret-bearing
candidates before they cross the wire. Pausing a workspace or Project stops new
outbound sharing synchronously.

The complete boundary, retention policy, and threat model are public in
[security and privacy](docs/security-privacy.md).

## AI is optional—and belongs to the Project

Structural coordination works with no model and no key. A Project owner can add
an Anthropic or OpenAI-compatible provider for semantic judgment, and an
OpenAI-compatible embedding provider for broader related-work retrieval.

```bash
printf '%s\n' "$ANTHROPIC_API_KEY" | overgent ai set \
  --judgment-provider anthropic \
  --judgment-model claude-sonnet-5 \
  --judgment-key-stdin \
  --embedding-provider deterministic \
  --embedding-model deterministic-v1
```

Provider failure never disables deterministic evidence. Project keys are
encrypted by the selected backend, never returned by reads, and never mixed
into coordination events. See [AI providers](docs/ai-providers.md).

## Project status

Overgent is beta software. Apple Silicon macOS 12+ is the only platform
qualified on real hardware, and its CLI, background service, local backend, and
desktop beta are covered by the signed-release and clean-machine lifecycle
gates.

Linux and Windows ship as of v0.1.1. Both build and pass their documented
automated checks, and the Linux installer lifecycle passes end to end in a
container, but neither has been run on a native desktop by us: the full install,
credential store, service recovery, update, rollback, and uninstall lifecycles
are unverified there. Windows additionally has no code-signing certificate yet.
Intel macOS is also unqualified. Please report what breaks. See
[release qualification](docs/release.md).

Current adapter fidelity is explicit in the product: configured is not the same
as observed, unavailable semantic judgment is not presented as full
intelligence, and missing evidence is never turned into an all-clear.

## Why open source

Overgent runs beside repositories, coding agents, credentials, and developer
tools. You should be able to inspect what it reads, what it derives, what it
sends, and how the backend authorizes and retains that data.

The installed client, local service, adapters, protocols, dashboard, core
backend, coordination engine, evaluation harness, installers, and release
workflows are public under [Apache 2.0](LICENSE). Production credentials,
private operational records, and customer data do not belong in this
repository. Read the [open-source and trust strategy](docs/open-source-strategy.md).

## Build and contribute

Read [the documentation index](docs/README.md) in order before implementation.
The repository uses Go 1.26 and Node 22 with pnpm 11.

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

Start with [local development](docs/development.md), then see
[contributing](CONTRIBUTING.md), [adapter development](docs/adapter-development.md),
and [security reporting](SECURITY.md).

Overgent is a coordination harness, not a coding-agent harness. If an extension
would make Overgent own an agent's model loop, repository editing, shell/test
execution, model routing, or permission system, it belongs in the coding agent—not
here.
