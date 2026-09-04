# One-day run plan

Status: active
Last updated: 2026-09-04

This is the execution schedule for the briefs in this directory. Each
**session** is one agent conversation in its own worktree. Sessions in the
same block run in parallel. The owner merges in the order given and runs the
integration check after each merge. Read `README.md` in this directory for
the rules every session follows.

## Model assignment, plainly

| Session | Brief(s) | Model | Why this level |
|---|---|---|---|
| 0 | ADRs appended, history purge, docs committed | done in the planning session | unblocks everything |
| 1 | `01-spike-bundled-backend.md` | Codex (high reasoning). Escalate to Opus 5 if stuck 45 min | exploratory, reads Convex CLI source, must report honestly |
| 2 | `02-lane-public-readiness.md` minus §1 and §3 | Codex or Haiku 4.5 | mechanical files, no design |
| 3 | `03-lane-local-mode.md` plus Lane 05 Deliverable 3 | Opus 5 | Go process supervision, desktop, signing; mistakes here cost a day |
| 4 | `04-lane-byo-ai.md` | Sonnet 5 | protocol and Convex work with clear contracts |
| 5 | `05-lane-self-host-and-cloud.md` Deliverables 1, 2, 4 only | Codex or Haiku 4.5 | documentation |
| 6 | `06-lane-project-backend-binding.md` | Opus 5 | the one real refactor; optional today |
| 7 | `07-launch-checklist.md` §1, §2, §4 prep, §5, §6, plus module rename (Lane 02 §3) | Sonnet 5 | writing plus a mechanical rename at the end |

## Schedule

```
Block A (start now, parallel):   Session 1 (spike)   Session 2 (public readiness)   Session 5 (docs)
Block B (after 1 accepted):      Session 3 (local mode)   Session 4 (BYO AI)
Block C (after 3 and 4 merged):  Session 6 (project binding)  ← start only if Block B merged by mid-afternoon
Block D (after everything):      Session 7 (launch)
```

Session 4 does not depend on the spike; start it with Session 3 to keep the
day short. Sessions 2 and 5 are cheap and independent; run them first while
the spike is exploring.

## Merge order and integration check

Merge into `main` in this order: 2 → 5 → 1 → 3 → 4 → 6 → 7. After each merge:

```bash
go test ./... && go vet ./...
pnpm install --frozen-lockfile && pnpm typecheck && pnpm test && pnpm build
pnpm protocol:check
pnpm desktop:test
```

If a merge conflicts, the later session rebases; the owner does not resolve
conflicts by hand.

## File-placement rules that keep Block B conflict-free

- Session 3 adds CLI subcommands in a new file `cmd/overgent/backend.go`;
  Session 4 adds them in `cmd/overgent/ai.go`. Each adds exactly one `case`
  line to `cmd/overgent/main.go`.
- Session 3 owns `apps/desktop/onboarding_service_darwin.go` and the first-run
  UI. Session 4 puts its settings pane in new files
  (`apps/desktop/ai_settings_darwin.go`, dashboard component under
  `apps/dashboard/src/desktop-ai-settings.tsx`) and touches the onboarding
  file only to register the binding.
- Session 3 owns `internal/app/app.go`. Session 4 must not edit it.
- Session 4 owns `protocol/`, `convex/`, `packages/coordination/`,
  `internal/hosted/client.go`. Session 3 must not edit those.
- Session 3 owns `apps/desktop/desktop_production.go` and
  `desktop_development.go` (including Lane 05 Deliverable 3, the "connect to
  a different server" field). Session 5 is documentation only.
- Nobody but Session 7 touches `README.md` or `go.mod`'s module line.

## Definition of done per session

- **1 Spike**: `validation/spikes/bundled-backend/README.md` names Option A or
  B with measured cold start, idle RSS, and bundle size; `push.sh` and
  `scripts/fetch-backend.mjs` exist; a backend started by hand with no Node on
  `PATH` accepts `overgent --api http://127.0.0.1:<port> create`. The owner
  reads the README and replies "accepted" before Session 3 starts.
- **2 Public readiness**: `SECURITY.md`, `CONTRIBUTING.md`, issue and PR
  templates, `CODEOWNERS`, `codeql.yml`, `dependabot.yml`, `.gitleaks.toml`
  exist; `ci.yml` has `permissions: contents: read`; private-beta wording is
  gone from `docs/` and `install/`; the doc sweep in `00-decisions.md` is
  applied; all checks pass.
- **3 Local mode**: fresh temp profile, built app, no Node: "Use on this Mac"
  → repository → dashboard within the cold-start budget; two worktrees
  collide; `lsof` shows loopback only; relaunch keeps data; `overgent backend
  reset` returns to first-run; the server field accepts an HTTPS origin;
  `validation/evidence/l9-local-mode.md` records it.
- **4 BYO AI**: `pnpm protocol:check` passes; `overgent ai set` then `ai status
  --json` shows `keyConfigured: true` and `effective.judgment: "project"`; the
  backend database contains no plaintext key; with no key a judgment
  candidate records `provider_unconfigured`; one real adjudication succeeded
  in a throwaway Project.
- **5 Docs**: `docs/self-hosting.md` and `docs/hosted-operations.md` exist;
  a second profile follows Option A end to end from the doc alone; no
  account identifiers in either file.
- **6 Project binding**: a Lane 03 profile upgrades in place; pasting an
  https invite link adds a team Project next to the local one; pausing one
  does not pause the other; `git grep "cfg.APIBaseURL\|cfg.DeviceID"` is
  empty outside `internal/config`.
- **7 Launch**: new `README.md`; `docs/README.md` index final; module path
  renamed and every check green; `.goreleaser.yml` has the cask; landing
  copy matches the README; announcement paragraph drafted. The owner then
  enables private vulnerability reporting, flips visibility, and tags.

## Kickoff prompts (paste as the first message of each session)

Every prompt starts with the same preamble:

> You are executing one brief of the Overgent open-source migration. Create
> a worktree first: `git worktree add ../overgent-oss-<name> -b oss/<name>`
> and work only there. Read `docs/migration/README.md` and
> `docs/migration/RUN-PLAN.md`, then the brief named below, then every file
> the brief lists under "Read first". Follow the brief exactly; if it
> conflicts with the code, stop and report the conflict. Do not edit files
> the run plan assigns to another session. Finish with the handoff format in
> `README.md`, including every command you ran and its result.

Then one line each:

- **Session 1**: `Brief: docs/migration/01-spike-bundled-backend.md. Name: spike. Report measured numbers; "does not work" is a valid result.`
- **Session 2**: `Brief: docs/migration/02-lane-public-readiness.md, skipping §1 (already done) and §3 (Session 7 does the rename). Also apply the doc sweep at the bottom of docs/migration/00-decisions.md. Name: public.`
- **Session 3**: `Brief: docs/migration/03-lane-local-mode.md plus Deliverable 3 of docs/migration/05-lane-self-host-and-cloud.md. The spike result is in validation/spikes/bundled-backend/README.md and is accepted. Name: local.`
- **Session 4**: `Brief: docs/migration/04-lane-byo-ai.md. Name: ai. You own protocol/, convex/, packages/coordination/, internal/hosted/client.go; do not edit internal/app/app.go.`
- **Session 5**: `Brief: docs/migration/05-lane-self-host-and-cloud.md, Deliverables 1, 2 and 4 only (documentation). Name: docs.`
- **Session 6**: `Brief: docs/migration/06-lane-project-backend-binding.md. Lanes 03 and 04 are merged on main. Name: binding.`
- **Session 7**: `Brief: docs/migration/07-launch-checklist.md §1, §2, §5, §6 and the §4 preparation, plus §3 of docs/migration/02-lane-public-readiness.md (module rename to github.com/<org>/overgent). Name: launch. Everything else is merged on main.`

## If the day runs short

Ship without Session 6 and keep the "profile is local or team; reset to
switch" sentence in the README. Ship without Session 4 only if Session 3
merged but 4 did not; semantic features are then off in local mode and the
README says "bring-your-own-key lands next release". Never ship without
Sessions 2 and 3.
