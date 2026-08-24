# Stickguy local development

The supported dogfood profile runs the public stack on one Mac. It is not a
different product implementation: Vite serves the same React dashboard that is
embedded in production builds, local Convex runs the same hosted functions, and
the foreground Go process runs the same one-service core.

## Commands

The repository requires Node 22. With NVM, select the checked-in version and
activate the pinned package manager before the first install:

```bash
nvm install
nvm use
corepack enable
corepack prepare pnpm@11.19.0 --activate
pnpm install --frozen-lockfile
```

Node 20's older bundled Corepack cannot load pnpm 11's compatibility shim and
fails with `ERR_VM_DYNAMIC_IMPORT_CALLBACK_MISSING`; upgrading the Project to
Node 22 is the supported fix. Then run commands from the repository root:

| Command | Behavior |
|---|---|
| `pnpm dev` | Build the CLI, start local Convex, Vite, the development desktop, and the Go service once an enrolled default profile exists. |
| `pnpm dev:ui` | Start the dashboard at `127.0.0.1:5173` with React hot reload and a proxy to local Convex. |
| `pnpm dev:backend` | Start the anonymous loopback Convex deployment without an account prompt and reload hosted functions on change. |
| `pnpm dev:service` | Build `bin/stickguy` and run the enrolled default-profile Go service in the foreground. |
| `pnpm desktop:dev` | Start Vite if needed, compile `Stickguy Dev.app` once, and keep the native Dock/menu-bar app attached to Vite hot reload. |
| `pnpm dev:install` | Atomically install or replace `~/Applications/Stickguy Dev.app`. Run `pnpm dev:ui` while using the installed app. |
| `pnpm dev:agents -- --codex-root A --claude-root B` | Register two linked worktree roots as separate workstreams and install explicit development MCP configuration. |

React/CSS changes hot reload without a native rebuild. Changes to the Wails
shell require restarting `pnpm desktop:dev`; Go core changes require restarting
`pnpm dev:service` or the full `pnpm dev` stack. Convex functions reload while
`pnpm dev:backend` is running.

The development desktop opens a native first-run screen. Choose a Git
repository, create a Project or join with an invite, select the coding-agent
adapters to install, and press **Open live Project**. No terminal
command is required for this normal path. The app exchanges a one-time ticket
through the loopback Vite proxy so the development session cookie remains
same-origin while the Wails bridge stays attached during hot reload. The
development app refuses non-loopback API/dashboard origins. Production builds
ignore Vite and retain the separately gated preview behavior.

## First local Project

The Git repository under test must have exactly one normalized remote. Start
the stack:

```bash
pnpm dev
```

After Convex reports that the local deployment is ready, finish setup in the
desktop window. The equivalent fallback command is:

```bash
./bin/stickguy --api http://127.0.0.1:3211 create \
  --label "Local dogfood" \
  --device-label "My Mac" \
  --root /absolute/path/to/codex-worktree
```

The command prints the Project, workspace, workstream, and invite IDs. `pnpm
dev` notices the new default profile and starts the service. The macOS Keychain
may request approval for the device credential.

The desktop detects `codex` and `claude` on `PATH` plus standard macOS app,
user-local, and NVM install locations. Detection is advisory: either adapter can
still be selected when a GUI-launched app has a narrower process environment.
Selecting an adapter adds only Stickguy's Project-scoped MCP entry and preserves unrelated configuration.
Restart agent sessions that were already open so they discover the new entry.
Git observation works even when an adapter is missing or declined. Claude Code
CLI is the supported Claude surface here; the general Claude Desktop app does
not expose a repository-bound lifecycle contract to this flow.

## Codex-versus-Claude collision exercise

Use two distinct linked worktrees of the same repository for honest per-agent
attribution. A single checkout is observed automatically, but filesystem events
cannot prove which process made each change, so Codex and Claude activity in one
checkout is one combined workstream. Stickguy never creates, switches, resets,
or removes worktrees on the user's behalf. One possible user-run Git setup is:

```bash
git worktree add /absolute/path/to/claude-worktree -b dogfood/claude
```

Register and configure the two roots:

```bash
pnpm dev:agents -- \
  --codex-root /absolute/path/to/codex-worktree \
  --claude-root /absolute/path/to/claude-worktree
```

The command verifies both roots share one Git common directory, hot-registers
any missing root through the running one-service core, and prints each
workspace/workstream ID. It structurally merges only Stickguy's project MCP
entry. Restart already-running agent sessions afterward. Claude may show its
normal one-time project MCP approval. Review the resulting `.codex/config.toml`
and `.mcp.json` in each worktree like any other project configuration change.

The same registration is available without that command: on the connected
desktop screen choose **Assign Codex worktree…** and **Assign Claude
worktree…**. Each selected directory must already be a distinct linked
worktree. The native boundary validates the shared Git common directory,
registers it through the running one-service IPC path, and installs only that
agent's Project MCP entry. Restart sessions opened before assignment.

Ask both agents to call `begin_work` before editing and `check_coordination`
before broad/shared changes. If a current client does not call MCP reliably,
report its intent manually with the printed workspace ID:

```bash
./bin/stickguy intent --workspace WORKSPACE_ID \
  --title "Rotate browser sessions" \
  --outcome "Rotate browser sessions after privilege changes and revoke prior credentials"
```

For a deterministic structural proof, change the same relative file path in
both worktrees. The service observes path/status metadata only, publishes two
atomic manifests, and the live dashboard should show a `direct_collision`
finding after its next two-second refresh.

For a semantic proof without overlapping paths, report these bounded outcomes
on the two different workspace IDs, then edit different files:

```text
Rotate browser sessions after privilege changes and revoke prior credentials.
Issue new web login credentials after a member role changes and invalidate old credentials.
```

The vocabulary-bounded `stickguy-concepts/v1` provider should create a justified
`redundant_work` radar finding. This is local Convex vector search over approved
intent summaries, not source, diffs, transcripts, prompts, raw commands/output,
or environment values.

## Honest adapter fidelity

- Git observation is the deterministic realtime fallback for both roots.
- Claude can use the project MCP lifecycle; optional hook/activity collection is
  still disabled in production.
- Codex discovers the MCP tools, but its installed version previously failed to
  deliver calls durably in the real-client gate. The development setup is
  therefore labeled `mcp_with_git_fallback`, not full live session observation.
- Stickguy does not subscribe to arbitrary independently running Codex process
  streams, tail transcripts, display hidden reasoning, or collect source/diffs.

This single-Mac exercise validates attribution, live Git collisions, bounded
semantic findings, briefs, and the dashboard. Inviting another member reuses
the existing join code, but production multiplayer reliability and distribution
remain later gates.

Remove only Stickguy's managed MCP entries without disturbing unrelated agent
configuration:

```bash
./bin/stickguy setup remove --development --agent codex --project-root /absolute/path/to/codex-worktree
./bin/stickguy setup remove --development --agent claude --project-root /absolute/path/to/claude-worktree
```
