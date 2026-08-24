# Stickguy local development

The supported dogfood profile runs the public stack on one Mac. It is not a
different product implementation: Vite serves the same React dashboard that is
embedded in production builds, local Convex runs the same hosted functions, and
the foreground Go process runs the same one-service core.

## Commands

Run from the repository root after `pnpm install --frozen-lockfile`:

| Command | Behavior |
|---|---|
| `pnpm dev` | Build the CLI, start local Convex, Vite, the development desktop, and the Go service once an enrolled default profile exists. |
| `pnpm dev:ui` | Start the dashboard at `127.0.0.1:5173` with React hot reload and a proxy to local Convex. |
| `pnpm dev:backend` | Start the loopback Convex deployment and reload hosted functions on change. |
| `pnpm dev:service` | Build `bin/stickguy` and run the enrolled default-profile Go service in the foreground. |
| `pnpm desktop:dev` | Start Vite if needed, compile `Stickguy Dev.app` once, and keep the native Dock/menu-bar app attached to Vite hot reload. |
| `pnpm dev:install` | Atomically install or replace `~/Applications/Stickguy Dev.app`. Run `pnpm dev:ui` while using the installed app. |
| `pnpm dev:agents -- --codex-root A --claude-root B` | Register two linked worktree roots as separate workstreams and install explicit development MCP configuration. |

React/CSS changes hot reload without a native rebuild. Changes to the Wails
shell require restarting `pnpm desktop:dev`; Go core changes require restarting
`pnpm dev:service` or the full `pnpm dev` stack. Convex functions reload while
`pnpm dev:backend` is running.

The development desktop initially shows fixtures. After enrollment, choose
**Open local live Project** from its menu-bar menu and press **Open secure
dashboard**. The app mints and exchanges a one-time ticket inside its webview,
then shows live local Project state. The development app refuses non-loopback
API origins. Production builds ignore the Vite URL and have no development
activation action.

## First local Project

The Git repository under test must have exactly one normalized remote. Start
the stack:

```bash
pnpm dev
```

After Convex reports that the local deployment is ready, use another terminal:

```bash
./bin/stickguy --api http://127.0.0.1:3211 create \
  --label "Local dogfood" \
  --device-label "My Mac" \
  --root /absolute/path/to/codex-worktree
```

The command prints the Project, workspace, workstream, and invite IDs. `pnpm
dev` notices the new default profile and starts the service. The macOS Keychain
may request approval for the device credential.

## Codex-versus-Claude collision exercise

Use two distinct linked worktrees of the same repository. Stickguy never
creates, switches, resets, or removes worktrees on the user's behalf. One
possible user-run Git setup is:

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
