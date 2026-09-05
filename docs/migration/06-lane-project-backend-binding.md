# Lane 06 — Bind each Project to its own backend (local and team side by side)

Status: brief
Last updated: 2026-09-04
Executor: Opus 5. This is the one refactor in the migration that needs
judgment inside a large file; do not hand it to a smaller model.
Depends on: ADR-074 accepted; Lane 03 merged (it owns `isLocalBackendOrigin`
and `internal/localbackend`); Lane 05 Deliverable 3 merged (origin entry in the
desktop). Lane 04 is not a dependency, but if it has merged, AI settings are
already per Project and need no change here.
May ship after the first public release if `07-launch-checklist.md` records
the limitation.

## Goal

One profile, one Go service, several Projects, each bound to its own backend:
a local Project on the loopback backend and a team Project on Overgent Cloud
or a self-hosted origin at the same time. Hooks, MCP bindings, the pause
switch, focus, and injection behave exactly as today for each Project.

## Inputs from Lanes 03 and 04 (merged 2026-09-04)

- `internal/localbackend` exists. `localbackend.IsLoopbackOrigin` is the
  single loopback check; `internal/app/app.go` `isLocalBackendOrigin`
  delegates to it. Move the decision to `Backend.Kind` as this brief says.
- `internal/app/app.go` is about 1,500 lines, not 3.9k.
- `overgent backend reset` currently clears a **local** profile's enrollment
  because a profile is one kind. After this lane it must clear only the
  Projects bound to the local backend.
- IPC methods `backend_status`, `backend_ensure`, `backend_stop` exist; keep
  them backend-scoped by id.
- `apps/desktop/dashboard_origin_darwin.go` serves the embedded dashboard on
  a loopback origin and proxies `/api/v1` to the local backend. A team
  Project opens the dashboard against its own origin as before; the desktop
  must choose per Project.
- `onboarding.ValidateAPIOrigin` is the shared origin validator (CLI and
  desktop). AI settings are already per Project (Lane 04) and need no change.
- `credential.Store` does not exist; `localbackend.CredentialStore` and
  `onboarding.CredentialStore` are the interfaces to reuse.

## Read first (all of it, before editing anything)

- `internal/config/config.go` (`Config` v1: `APIBaseURL`, `DeviceID`,
  `Workspaces[]` with `ProjectID`, `WorkstreamID`, `MemberID`, `SessionID`)
- `internal/store/store.go` (`projects`, `workspaces`, `cursors`,
  `event_queue`, `service_state` tables and their migrations)
- `internal/app/app.go`: `Run`, `scanAll`, `flushLoop`, `flush`,
  `retryIndividually`, `heartbeatLoop`, `sendHeartbeats`, `handle`,
  `handleAgentInjection`, `handleCollaboration`, `handleLifecycle`,
  `addWorkspace`, `workspaceForCWD`, `workspaceByID`, `Register`;
  `internal/app/hosted_sender.go` (`Sender`), `session_identity.go`
- `internal/hosted/client.go` (`New(rawBase, token)`, `Bootstrap`,
  `Heartbeat`, `PublishBatch`, `CreateBrief`), `credential_state.go`
- `internal/onboarding/onboarding.go`: `New(apiBaseURL)`, `Create`,
  `CreateAdditional`, `Join`, `JoinAdditional`, `finish`, `finishExisting`,
  and how the Keychain account for the device token is named
- `internal/credential/`
- `apps/desktop/onboarding_service_darwin.go`: `State`, `credentialHealth`,
  `CreateProject`, `CreateAdditionalProject`, `JoinProject`,
  `JoinAdditionalProject`, `ResetEnrollment`, `enroll`, `hotRegister`,
  `OpenLiveProject`, `SetProjectPaused`; `local_service_darwin.go`
- `cmd/overgent/main.go` `create`, `join`, `reset`, `dashboard`, `workspace`
- ADR-035, ADR-041, ADR-043, ADR-054, ADR-069, ADR-072, ADR-074
- `docs/development.md` "More than one Project on one profile"

## Design (fixed)

### Configuration version 2

```go
type Backend struct {
    ID         string `json:"id"`          // opaque, e.g. "bk_..."; stable across renames
    APIBaseURL string `json:"apiBaseUrl"`  // origin only; validated by hosted.New rules
    DeviceID   string `json:"deviceId"`    // ADR-069: one device identity per backend
    Kind       string `json:"kind"`        // "local" | "team"; derived from isLocalBackendOrigin at write time, stored for display
}
type Project struct {
    ID        string `json:"id"`
    BackendID string `json:"backendId"`
}
type Config struct {
    Version    int         `json:"version"`   // 2
    Backends   []Backend   `json:"backends"`
    Projects   []Project   `json:"projects"`
    Workspaces []Workspace `json:"workspaces"` // unchanged shape; ProjectID resolves the backend
}
```

`Load` migrates v1 → v2 in memory: one `Backend` from the old `APIBaseURL`
and `DeviceID`, one `Project` per distinct `Workspaces[].ProjectID` pointing at
it. `Save` always writes v2. Keep `Load` refusing versions above 2. Add
`(Config) BackendForWorkspace(ws) (Backend, bool)` and
`(Config) BackendForProject(id) (Backend, bool)`; every call site that used
`cfg.APIBaseURL` or `cfg.DeviceID` goes through these. Grep for both fields
and list every site in the handoff.

Keychain: the device token account name must include the backend. Read how
`onboarding.finish` names it today; if the account is the device id alone,
the v2 migration keeps the existing entry (the migrated backend keeps the
same `DeviceID`), and new backends get new device ids, so no rename of
Keychain items is needed. State in the handoff which case applied.

### Service

- `Run` builds a `map[backendID]*hosted.Client` lazily (token from the
  Keychain per backend; `CredentialStatus` per backend surfaced in `health`
  as a list, not a single value).
- `flush`: the queue is already per workspace; group the drained window by
  workspace → project → backend and publish per backend. A permanent
  rejection on one backend must not block flushing to another; keep the
  existing per-workspace backoff (`retryDelay`) semantics.
- `heartbeatLoop` / `sendHeartbeats`: one heartbeat per workspace to its own
  backend (the API is per workspace already; only the client differs).
- Local backend: for workspaces whose backend `Kind == "local"`, call
  `localbackend.Manager.Ensure` before use (Lane 03 put this behind
  `isLocalBackendOrigin`; move it to `Backend.Kind`).
- `handleAgentInjection`, `handleCollaboration`, `handleLifecycle`,
  `handleFocus`, `handleSessionDetail`: resolve the client through the
  workstream → workspace → project → backend chain. There must be no
  remaining reference to a single service-wide client when you finish.
- `Register`: takes the backend id (or origin plus device id) instead of the
  global origin.

### Onboarding

- `onboarding.New(apiBaseURL)` becomes `onboarding.New(backend config.Backend)`.
  `Create` (first Project on a backend) and `CreateAdditional` / `JoinAdditional`
  (a further Project on a backend that already has a device identity) stay;
  add `CreateOnNewBackend` / `JoinOnNewBackend` that mint a device identity for a
  backend the profile has not seen. `Join` with an invite from a backend the
  profile has never used is the common "join a friend's team Project" path and
  must work from a purely local profile.
- Invite codes: `ParseInviteCode` already accepts three shapes: a bare
  `invite.secret` code, an `overgent://join/<code>` deep link, and an
  `https://<host>/join#<code>` link. Today it discards the host. Change it to
  return `(code, origin)` where origin is `https://<host>` for the https shape
  and empty otherwise; empty means "the backend the caller selected" (the
  CLI `--api` flag or the desktop's current choice, defaulting to Overgent
  Cloud). Keep parsing strict; add tests for a link with a path other than
  `/join`, a userinfo component, and a non-https scheme carrying a host.
  Confirm where invite links are rendered (desktop share sheet and
  `convex/functions/service.ts` invite creation) so the https link always names
  the origin the Project lives on, including self-hosted origins.

### Desktop and CLI

- Onboarding `State` lists Projects with their backend kind and origin in
  monospace; the first-run choice from Lane 03 is now always available as
  "Add a Project": Use on this Mac / Create team Project / Join with invite.
  Remove the "Reset to switch" line from Lane 03.
- `ResetEnrollment` becomes per backend (`reset --backend <id>` in the CLI),
  with the existing whole-profile reset kept as `reset --all`.
- `overgent create --local`, `create --api`, `join <code>` (origin from the
  code), `join --api` all resolve or create the backend entry.
- `credentialHealth(deviceID, apiBaseURL)` already takes both; call it per
  backend.
- `pnpm dev:shared` and ADR-041's separate shared profile are deleted;
  `docs/development.md` describes adding a team Project to the same
  development profile with `--api https://…`.

### Store

`projects` table gains `backend_id`; add a migration in `internal/store`
following the existing `schema_migrations` pattern; backfill from the config
migration on first run.

## Tests

- `internal/config`: v1 fixture loads as v2 with one backend; save/load
  round trip; refusal of version 3; `BackendForWorkspace` on an orphan
  workspace returns false and the service logs, not panics.
- `internal/app`: two fake backends (httptest servers) with distinct tokens;
  events from two workspaces reach the right server; a 401 from one backend
  does not stop the other; heartbeats go to the right server; injection for a
  workstream on backend B never calls backend A.
- `internal/onboarding`: join with an origin-bearing code from a profile that
  has only a local backend creates a second backend and device id.
- Desktop tests mirror the existing `onboarding_service_darwin_test.go`.
- Live: the two-worktree collision exercise runs for a local Project and a
  team Project in the same profile, sequentially, and each shows only its own
  Project's sessions. Record under `validation/evidence/l9-project-backends.md`.

## Acceptance

- A profile created under Lane 03 (local only) upgrades in place; the local
  Project keeps its data and findings.
- Joining a team Project by pasting a code adds a backend without touching
  the local Project; pausing one Project does not pause the other.
- `git grep -n "cfg.APIBaseURL\|cfg.DeviceID"` returns nothing outside the
  config package and its migration test.
- All verification commands pass, including `pnpm desktop:test`.

## Out of scope

Moving a Project between backends, per-workspace backends, any wire change.
