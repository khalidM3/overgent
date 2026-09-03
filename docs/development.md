# Overgent local development

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
| `pnpm dev:service` | Build `bin/overgent` and run the enrolled default-profile Go service in the foreground. |
| `pnpm desktop:dev` | Start Vite if needed, compile `Overgent Dev.app` once, and keep the native Dock/menu-bar app attached to Vite hot reload. |
| `pnpm dev:install` | Atomically install or replace `~/Applications/Overgent Dev.app`. Run `pnpm dev:ui` while using the installed app. |
| `pnpm dev:agents -- --codex-root A --claude-root B` | Optional advanced setup for two linked worktrees; normal same-checkout session attribution does not require this. |

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
ignore Vite and use the signed beta onboarding/API boundary described in
`beta-release.md`.

## First local Project

The Git repository under test must have exactly one normalized remote. Start
the stack:

```bash
pnpm dev
```

After Convex reports that the local deployment is ready, finish setup in the
desktop window. The equivalent fallback command is:

```bash
./bin/overgent --api http://127.0.0.1:3211 create \
  --label "Local dogfood" \
  --device-label "My Mac" \
  --root /absolute/path/to/repository
```

The command prints the Project, workspace, workstream, and invite IDs. `pnpm
dev` notices the new default profile and starts the service. The macOS Keychain
may request approval for the device credential.

The desktop detects `codex` and `claude` on `PATH` plus standard macOS app,
user-local, and NVM install locations. Detection is advisory: either adapter can
still be selected when a GUI-launched app has a narrower process environment.
Selecting an adapter structurally adds Overgent's Project-scoped MCP entry and
passive activity hooks while preserving unrelated configuration. Restart agent
sessions that were already open so they load the new Project configuration.
Git observation works even when an adapter is missing or declined. Claude Code
sessions in the CLI, IDE, or Desktop app use the same documented hook events;
an ordinary Claude chat that is not a Claude Code repository session is outside
this repository-scoped flow.

## Codex-versus-Claude collision exercise

Use the same registered checkout for the normal exercise. Start a new Codex
session and a new Claude Code session anywhere under that repository. Each
supported session automatically appears as a separate hashed workstream; no
Overgent command, branch, or worktree is required. Ask each agent to edit the
same safe relative path. Their edit-tool hooks report only the path, and the
dashboard should show both sessions plus a deterministic `direct_collision`
finding after its next two-second refresh.

Linked worktrees remain an optional advanced Git-isolation technique. Overgent
never creates, switches, resets, or removes them. One possible user-run setup is:

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
missing roots, and installs that root's managed MCP plus passive activity hooks.
Review `.codex/config.toml`, `.mcp.json`, `.claude/settings.local.json`, and
`$CODEX_HOME/hooks.json` like any other local Project configuration. Codex
hooks are installed once at the user layer rather than per project (ADR-051),
so the same file serves every registered root.

Codex will not run a hook it has not trusted, and it skips an untrusted hook
silently. `setup codex` asks Codex to record that trust through its app-server;
when that is unavailable the binding reports `hooks: "needs_review"` instead of
`"active"`, and the desktop adapter row says so. Confirm the state before
concluding that observation is broken:

```bash
./bin/overgent setup status --agent codex --development --project-root /absolute/path/to/worktree
```

Pass `--development` (or the `--config-root` you installed with). Without either,
the check is made against the *portable* binding a released install uses — the
bare `overgent` on `PATH` with no profile — so a development binding, which names
an explicit executable and config root, is correctly reported as belonging to a
different profile. The reply says which profile it compared against in
`checkedProfile`, and which one is actually bound in `previousProfile`; seeing
your own profile named there means the flag is missing, not that something is
wrong with the install.

A `needs_review` result is cleared by opening Codex → Settings → Hooks and
choosing Trust all, or by running `/hooks` in the Codex CLI. Trust is recorded
against the hook definition, so changing the Overgent executable path or config
root re-arms the review.

## More than one Project on one profile

One per-user service keeps one device identity across every Project it owns, so
`create` on a profile that has already enrolled reuses that identity and creates
an additional Project rather than minting a second credential and stranding the
first one's Projects:

```bash
./bin/overgent create --root /absolute/path/to/second-repository --label "Second Project"
```

A repository that is already connected is refused rather than connected twice.

To add another *local root* to a Project that already exists — two linked
worktrees of the same repository, for example — register the root instead of
creating a Project:

```bash
./bin/overgent workspace add --development --root /absolute/path/to/worktree --project prj_...
```

`workspace list` prints the registered roots with the IDs the other commands
take. This is the verb `pnpm dev:agents` uses underneath.

MCP `begin_work` and `check_coordination` calls add richer intent and context but
are no longer required for session presence or path attribution. Manual intent
remains available with the printed workspace ID:

```bash
./bin/overgent intent --workspace WORKSPACE_ID \
  --title "Rotate browser sessions" \
  --outcome "Rotate browser sessions after privilege changes and revoke prior credentials"
```

Git observation continues to publish the combined checkout manifest. Hook path
evidence supplies the per-session attribution that Git alone cannot provide.

For a semantic proof without overlapping paths, report these bounded outcomes
on the two different workspace IDs, then edit different files:

```text
Rotate browser sessions after privilege changes and revoke prior credentials.
Issue new web login credentials after a member role changes and invalidate old credentials.
```

The vocabulary-bounded `overgent-concepts/v1` provider should create a justified
`redundant_work` radar finding. This is local Convex vector search over approved
intent summaries, not source, diffs, transcripts, prompts, raw commands/output,
or environment values.

## Honest adapter fidelity

- Current documented Codex and Claude Code hooks report supported sessions,
  lifecycle, tools, permission waits, subagents, and safe affected paths.
- At `SessionStart` and `UserPromptSubmit`, both supported adapters fetch a
  bounded current coordination brief through the local service and inject new
  item revisions as vendor `additionalContext`. The two-second handler fails
  open on service or hosted failure, and local revision tracking prevents the
  same session from receiving one item revision twice. MCP pull and the
  dashboard remain available when injection is unavailable.
- Git observation remains the deterministic combined-checkout fallback when a
  vendor tool does not expose path metadata or an adapter is disconnected.
- Existing sessions must restart once after adapter installation. Hook coverage
  is honest: unsupported hosted or specialized tool paths are not inferred.
- Adapter readiness distinguishes configuration from runtime proof. “Configured
  · restart required” remains visible until the current Overgent profile
  receives a real session event from that provider.
- If the repository contains a valid Overgent binding for another local
  profile, onboarding shows **Reconnect to this Project** with an old/new
  preview. It never silently detaches the other profile. Partial entries for the
  current profile are repaired automatically; unknown drift still fails closed.
- Overgent may read the supported vendor session record locally to show the
  session owner their own bounded conversation and project classifier-passing
  messages to enrolled Project members while sharing is unpaused. It never uploads the transcript file itself, scans process
  memory, displays hidden reasoning, or collects source/diffs, raw
  commands/output, environment values, `.env` variants, credentials, or
  system/developer prompts.

This single-Mac exercise validates attribution, live Git collisions, bounded
semantic findings, briefs, and the dashboard. Invite another member by creating
a fresh one-use code in Project Settings. Credentialed production distribution and the real-team
second-session exit remain the owner-controlled L8 gates.

## Shared two-Mac dogfood

The explicit shared-development profile uses a cloud Convex development
deployment while keeping each dashboard and local service on its own Mac. It
uses a separate local profile and accepts only an HTTPS remote API origin.

On the first Mac, sign in and create or select a cloud development deployment:

```bash
pnpm --dir convex exec convex login
pnpm --dir convex exec convex dev --once --configure new --dev-deployment cloud
```

Copy the deployment's `CONVEX_SITE_URL` (the `https://...convex.site` HTTP
actions origin), not the `convex.cloud` client URL. Set the OpenAI key without
putting it in shell history:

```bash
pbpaste | pnpm --dir convex exec convex env set OPENAI_API_KEY
pnpm --dir convex exec convex env list --names-only
```

Push function changes with `convex dev --once`, then start the shared profile:

```bash
OVERGENT_SHARED_API_ORIGIN=https://YOUR-DEPLOYMENT.convex.site pnpm dev:shared
```

Create the Project in the desktop and send the one-use invite privately. On the
second Mac, use the same repository checkout and repository remote, run the same
commit of Overgent with the same `OVERGENT_SHARED_API_ORIGIN`, choose **Join a
Project**, and enter the invite. Each Mac stores a different device credential
and publishes independently to the shared Project.

The shared profile defaults to
`~/Library/Application Support/Overgent Shared Dev`. Override it only with an
absolute `OVERGENT_SHARED_CONFIG_ROOT`. It never reads or mutates the ordinary
loopback development profile.

Remove only Overgent's managed MCP entries without disturbing unrelated agent
configuration:

```bash
./bin/overgent setup remove --development --agent codex --project-root /absolute/path/to/codex-worktree
./bin/overgent setup remove --development --agent claude --project-root /absolute/path/to/claude-worktree
```

For a repository intentionally moved between local profiles, use the desktop
reconnect preview or the equivalent explicit CLI recovery:

```bash
./bin/overgent --config-root "/absolute/path/to/current/profile" setup reconnect --development --agent codex --project-root /absolute/path/to/repository
```

After reconnecting, restart the provider and begin a new task in the enrolled
repository. Overgent keeps the adapter pending until that first event arrives.
