# Open-source rewire — orchestration guide

Status: proposed; owner reviews `00-decisions.md` first
Last updated: 2026-09-04

This directory turns Overgent from a private, hosted-first beta into an
open-source, **local-first** tool with an **opt-in hosted team backend**. The
briefs are written the same way as `docs/tasks/`: an implementing agent needs
zero architectural judgment calls. If a brief conflicts with the code you find,
STOP and report the conflict in your handoff instead of deciding yourself.

## Target state in one paragraph

Everything in this repository is public under Apache-2.0, including the Convex
backend. A fresh install works with no account and stores nothing off the
device: the desktop app and Go service run the open-source Convex backend on
loopback, and the Go core keeps speaking the same frozen `/v1` HTTP contract it
speaks today. A member who wants remote teammates creates or joins a **team
Project** on Overgent Cloud (`api.overgent.com`, run by the owner from this
public code) or on a self-hosted backend, per Project, without giving up their
local Projects. Semantic judgment and embeddings run on the Project's own
provider keys; the hosted default ships with no operator key, so the cloud
costs the owner close to nothing. Nothing here creates accounts, billing, or
SSO; those remain a later hosted-tier decision.

## Why this shape and not the alternatives

- **The coordination engine lives in Convex functions, not in Go.** The Go
  service is an observer and queue (`internal/store` holds `event_queue`,
  `manifests`, `session_read_sets`, `contract_fingerprints`; no findings).
  Findings, briefs, decisions, membership, and rate limits are all in
  `convex/functions/service.ts` (about 176 KB) and `intelligence.ts`.
  Reimplementing that in Go for a local mode would create two engines that
  drift. Rejected.
- **The development profile already runs the open-source Convex backend on
  loopback** (`pnpm dev` starts `convex-local-backend` on 127.0.0.1:3210/3211
  and the desktop points at it). Local mode is therefore packaging and
  lifecycle work, not new coordination code. Chosen.
- **A "bring the engine to a Node sidecar over SQLite" refactor** would
  require splitting `service.ts` into storage-agnostic domain plus two thin
  adapters. Cleaner long term, far more work now, and still needs a bundled
  JS runtime. Deferred; nothing here forecloses it.

## Phases and dependency graph

```
Phase 0  00-decisions.md ──────────────┐  (owner accepts ADR-071…074)
                                       │
Phase 1  01-spike-bundled-backend.md ──┤  (proves the backend can ship in the app)
                                       │
Phase 2  02-lane-public-readiness ─────┤  independent, start immediately
         03-lane-local-mode ───────────┤  needs 01
         04-lane-byo-ai ───────────────┤  needs 00; owns protocol changes
         05-lane-self-host-and-cloud ──┤  needs 00; docs + small client change
         06-lane-project-backend-binding  needs 03 (and 04 for settings per Project)
                                       │
Phase 3  07-launch-checklist.md ───────┘  after 02, 03, 04, 05 merge (06 may follow)
Phase 4  08-post-launch-product.md        after launch
```

Lane 06 is the one lane that can slip past launch: without it a profile talks
to one backend, so "local by default, team opt-in" becomes "switch the app
between a local profile and a cloud profile". That is acceptable for a first
public release only if the README says so plainly.

The session-by-session schedule, model assignments, kickoff prompts, and
merge order are in [`RUN-PLAN.md`](RUN-PLAN.md); it supersedes the table below
where they differ.

## Which model to use for which brief

The expensive part of this repository is judgment about boundaries, not typing.
Route by the kind of judgment each brief needs:

| Brief | Nature of the work | Suggested executor |
|---|---|---|
| `00-decisions.md` | Paste reviewed ADR text, update cross-references | Haiku 4.5 |
| `01-spike-bundled-backend.md` | Experimental; reads Convex CLI internals; must report honestly | Sonnet 5 |
| `02-lane-public-readiness.md` | Mechanical: history rewrite, renames, doc sweeps, CI hygiene | Haiku 4.5 or Sonnet 5 |
| `03-lane-local-mode.md` | Go process supervision, desktop onboarding, installer changes | Sonnet 5 |
| `04-lane-byo-ai.md` | Protocol + Convex + Go CLI + desktop; touches the wire boundary | Sonnet 5 (Opus 5 for the Convex encryption/ resolution section if it stalls) |
| `05-lane-self-host-and-cloud.md` | Documentation plus one small client change | Haiku 4.5 or Sonnet 5 |
| `06-lane-project-backend-binding.md` | Refactor through `internal/app/app.go` (3.9k lines) and config migration | Opus 5 |
| `07-launch-checklist.md` | Checklists, README, release run | Sonnet 5 |
| `08-post-launch-product.md` | Product work; not part of the migration | later |

Model IDs at the time of writing: `claude-haiku-4-5-20251001`, `claude-sonnet-5`,
`claude-opus-5`.

## Rules for every lane

1. **Worktree isolation.** `git worktree add ../overgent-oss-<lane> -b oss/<lane>`.
   Never commit to `main`. Lane 02 rewrites history; every other worktree must be
   created *after* Lane 02's force-push lands, or rebased onto it.
2. **Protocol ownership.** Only Lane 04 modifies `protocol/openapi.yaml`,
   `protocol/schemas/`, or runs `pnpm protocol:generate`. Lane 06 needs no wire
   change. Never hand-edit `protocol/generated/`.
3. **Config ownership.** Only Lane 06 changes the shape of
   `internal/config.Config`. Lane 03 adds a *sibling* file for backend state
   (see its brief) precisely so the two lanes do not collide.
4. **Read before writing.** Each brief lists files to read first. Match the
   package's existing error handling and test style. Tests use temp config
   roots and temp repositories, never contributor state.
5. **Verification before handoff.** In your worktree:
   ```bash
   go test ./... && go vet ./...
   pnpm install --frozen-lockfile && pnpm typecheck && pnpm test && pnpm build
   pnpm protocol:check        # Lane 04, and anyone who rebased onto it
   pnpm desktop:test          # any lane touching apps/desktop
   ```
6. **Handoff format.** Behavior delivered versus the brief's acceptance
   criteria; files/contracts changed; commands run and results; security and
   privacy notes; known limits; anything the brief got wrong about the code.
7. **Scope discipline.** Do not refactor beyond the brief. Do not rename
   things the brief did not ask you to rename. Do not "improve" the design
   system, the finding engine, or adapter behavior.
8. **Non-negotiables from `AGENTS.md` still apply**: the privacy boundary is
   the wire (ADR-044); the secret classifier stays a non-disableable gate;
   never bind beyond loopback; core behavior works with AI disabled;
   unsupported semantic processing is shown as degraded, never as intelligence.

## Terms used across the briefs

- **Backend**: any server implementing the `/v1` contract in
  `protocol/openapi.yaml`. Today that is the Convex deployment in `convex/`.
- **Local backend**: the open-source `convex-local-backend` binary running on
  loopback on the member's Mac with this repository's functions deployed to it.
- **Overgent Cloud**: the owner-operated deployment of the same public code at
  `https://api.overgent.com`.
- **Self-hosted backend**: the same public code deployed by someone else,
  either to their own Convex account or to their own `convex-backend` server.
- **Mode** of a Project: `local` (loopback backend) or `team` (Cloud or
  self-hosted). A profile may hold both kinds once Lane 06 lands.
- **Profile**: one config root (`~/Library/Application Support/Overgent` by
  default; `OVERGENT_CONFIG_ROOT` overrides) with one Go service, one lock, one
  socket. Unchanged by this migration.
