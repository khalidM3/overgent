# L9 — A profile binds each Project to its own backend

Date: 2026-09-04
Machine: Apple Silicon (`arm64`), macOS 27.0, Go 1.26.7, Node 22.23.2, pnpm 11.19.0
Lane: `docs/migration/06-lane-project-backend-binding.md` (ADR-074)

## Result

Configuration version 2 lands: a profile holds one `Backend` record per server
with the device identity used against it, one `Project` record binding each
Project to a backend, and the same `Workspaces` list as before. A version 1
profile upgrades in place on first read, keeps its Projects, and keeps the
Keychain entry it already had. One Go service now holds one hosted client per
backend, and every publish, heartbeat, brief, injection, and collaboration read
resolves through workstream → workspace → Project → backend.

**The live two-worktree collision exercise across a local and a team Project
was not run in this session.** It needs two things this session did not have:
the 166 MB bundled backend (Lane 01's `scripts/fetch-backend.mjs` download) and
a real remote deployment to hold the team half. The publish-boundary claim it
would make — each Project's events, heartbeats, and briefs reach only its own
server, with only its own credential — is covered by an automated equivalent
against two live HTTP servers (below). See "Not verified here".

## What was run

### 1. A version 1 profile upgrades in place

A profile was written by hand in the version 1 shape (one `apiBaseUrl`, one
`deviceId`, one registered repository on `prj_legacy`), then read through the
shipped CLI without being rewritten:

```
overgent --config-root <profile> backend list
{"backends":[{"apiBaseUrl":"https://api.overgent.com","id":"bk_54a70a85…","kind":"team","projects":["prj_legacy"]}]}
```

`workspace list` returned the same repository it always had, and `config.json`
on disk was still version 1 afterwards: a read migrates in memory, and only a
write commits version 2. The migrated backend carries the same `deviceId`, so
the Keychain account name that entry is stored under does not change and no
Keychain item is renamed or re-created (see "Keychain" below).

### 2. A local Project is added beside the team one

A second Project was registered against a loopback origin the profile had never
used, through `app.Register`'s new binding path:

```
overgent --config-root <profile> workspace add --project prj_local \
  --api http://127.0.0.1:43103 --device dev_localbackend… --root <repo-b>
```

`config.json` is now version 2 with two backends, two Projects, two
repositories, and the version 1 fields gone:

```json
{"version":2,
 "backends":[{"id":"bk_54a70a85…","apiBaseUrl":"https://api.overgent.com","deviceId":"dev_legacy…","kind":"team"},
             {"id":"bk_3f7386fc…","apiBaseUrl":"http://127.0.0.1:43103","deviceId":"dev_localbackend…","kind":"local"}],
 "projects":[{"id":"prj_legacy","backendId":"bk_54a70a85…"},
             {"id":"prj_local","backendId":"bk_3f7386fc…"}]}
```

### 3. The service reports each backend separately

With both Projects registered, `overgent doctor` against the running service:

```json
{"backends":[{"id":"bk_54a70a85…","kind":"team","apiBaseUrl":"https://api.overgent.com","credential":"unknown"},
             {"id":"bk_3f7386fc…","kind":"local","apiBaseUrl":"http://127.0.0.1:43103","credential":"unknown"}],
 "workspaces":2,"pausedWorkspaces":0,"pending":4,"lastPublishError":"not_configured"}
```

`credential: "unknown"` is correct and is the honest answer for this profile:
the synthetic device identities have no Keychain entry, so no publisher could
be built for either backend. Health states that per backend rather than as one
value for the Mac — a revoked team Project must not present the local Project
beside it as broken.

### 4. Pausing one Project does not pause the other

```
overgent pause --project prj_local     → {"paused":true,"workspaces":1}
doctor                                 → workspaces: 2  paused: 1
overgent resume --project prj_local
overgent pause --project prj_legacy    → {"paused":true,"workspaces":1}
doctor                                 → workspaces: 2  paused: 1
```

### 5. Reset is per backend and refuses to guess

```
overgent reset
this profile has more than one backend; pass --backend <id> or --all

overgent reset --backend bk_3f7386fc…
{"backends":[{"backendId":"bk_3f7386fc…","apiBaseUrl":"http://127.0.0.1:43103","clearedWorkspaces":1,…}]}

overgent backend list
{"backends":[{"id":"bk_54a70a85…","kind":"team","projects":["prj_legacy"]}]}
overgent workspace list → ['wsp_legacy_a']
```

The local backend, its Project, and its repository are gone; the team Project
beside it is untouched. `overgent backend reset` (deleting the local database)
uses the same scoping: it clears every `local` backend and leaves team ones
alone, which `cmd/overgent` `TestBackendResetForgetsOnlyALocalEnrollment` now
covers for a profile holding both.

### 6. Two backends at the publish boundary, automated

`internal/app/backend_binding_test.go` stands up two `httptest` servers with
different device credentials, binds one Project to each on one profile, and
drives the real `internal/hosted` client through the service:

- events from two workspaces reach their own server, and the other sees none
  of them;
- a 401 from one backend leaves that workspace's window pending and does not
  stop the other backend draining (before this lane, one unreachable server
  stopped the whole profile publishing);
- heartbeats go to the server the workspace belongs to;
- a `begin_work` brief for a Project on backend B never reaches backend A;
- `health` names both backends with a credential state each.

Only the credential distinguishes the two servers, which is exactly the mistake
a single service-wide client would make.

## Keychain

`onboarding.finish` stores the device token under the device id alone
(`credential.Put(ctx, deviceID, token)`, service `com.overgent.comice`). The
version 1 → 2 migration keeps that device id on the migrated backend, so the
existing entry is still the right one and **no Keychain item is renamed,
copied, or re-created**. New backends mint new device ids and get their own
entries. This is the first of the two cases the brief asked to be stated.

## Not verified here

- **The live local + team collision exercise.** Needs the bundled backend
  download and a remote deployment. Carries to Session 7's launch verification
  alongside Lane 03's open desktop walk and Lane 04's real adjudication.
- **The desktop first-run and "Add a Project" windows.** The Go half is unit
  tested (`apps/desktop/*_test.go`) and the dashboard half is component tested
  (`apps/dashboard/test/desktop-onboarding.test.tsx`); driving the rendered
  window was not done in this session.
- **A real https invite link joined from a purely local profile.** The parsing,
  origin extraction, and identity-minting path are unit tested
  (`internal/onboarding`), but no invite was redeemed against a live server.
