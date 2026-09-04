# Lane 03 — Local mode: a Project that never leaves the Mac

Status: brief
Last updated: 2026-09-04
Executor: Sonnet 5.
Depends on: ADR-072 accepted; Lane 01 result accepted (it names Option A or B
and the exact admin endpoints; this brief refers to them as "the push step"
and "the env step").
Does not depend on: Lane 04, Lane 06. Do not change `internal/config.Config`.
Also delivers Lane 05 Deliverable 3 (the configurable production API origin
and the "connect to a different server" field), because it edits the same
desktop files; read that section of `05-lane-self-host-and-cloud.md` too.

## Spike outcome (binding inputs from Lane 01, accepted 2026-09-04)

Read `validation/spikes/bundled-backend/README.md` in full; these are the
decisions it fixes for this lane.

- **Option A.** The Go manager replays the release-time `deploy2` request
  sequence exactly as `validation/spikes/bundled-backend/push.sh replay` does
  with `curl`. The `deploy2` endpoints are internal to Convex, so the backend
  release (`scripts/backend-version.json`, currently
  `precompiled-2026-08-25-7cce8fb`), the npm CLI version (1.45.0), the
  payload, and the Go replay code are pinned together. `push.sh build`
  regenerates the payload at release time. Add a `release.yml` step that
  replays the fresh payload against a fresh backend and fails the release on
  error, so a broken pin never reaches a member. If the replay needs
  `Content-Encoding: br`, add `github.com/andybalholm/brotli` (pure Go).
- **Numbers.** Cold start 120 ms; idle RSS 56 MB; binary 160 MB, 51 MB
  compressed; push artifact under 1 MB. Idle shutdown is therefore **not**
  required: keep the backend running while the service runs. Health budget
  stays 10 s.
- **Upgrades.** A compatible schema push takes under 1 s and preserves rows.
  An incompatible push is rejected by the backend with rows preserved, so no
  pre-push database backup is needed. On rejection keep serving the previous
  bundle and surface `health.backend.lastError = "update needs data migration"`.
- **Signing.** The nested binary must be signed with the release identity,
  hardened runtime, and the `com.apple.security.cs.allow-jit` entitlement
  before the enclosing app is signed and notarized. No other entitlement was
  needed. Validate the notarized artifact on a clean Mac.
- **Outbound fetch policy (decided).** Start the backend without
  `--convex-http-proxy`. Only Overgent's own functions run on it, and their
  only outbound requests go to AI provider origins the Project owner
  configured (Lane 04 validates those as HTTPS or loopback HTTP). Record one
  sentence to that effect in `docs/security-privacy.md` under "Local".
- **Environment variables** are set through the admin endpoint the spike
  documented in its §3; `OVERGENT_SECRETS_KEY` is generated per install.
- **NOTICE** gets the paragraph drafted in the spike's §6 (FSL-1.1-Apache-2.0,
  Copyright 2026 Convex, Inc.) and the backend's `LICENSE.md` ships inside
  `Contents/Resources/backend/`.
- **Dashboard routing bug you must fix.** `apps/dashboard/src/main.tsx`
  decides `landing` with `!isDesktopWebview`, which is false when the
  development desktop loads `http://127.0.0.1:5173`, so opening a Project
  renders `LandingPage` instead of `LiveApp` (pre-existing since commit
  `b847761`; production `wails://` is unaffected). Use `isDesktopShell` for
  that decision and never show the landing page when `?live=1` is present.
  Add a test under `apps/dashboard/test/`.

## Goal

A member installs Overgent, opens it, picks a repository, and has a working
Project with collision detection between their own sessions, with no account,
no invite, no network, and nothing stored off the device. The Go service runs
and supervises the bundled Convex backend on loopback. Everything above the
`/v1` wire is unchanged.

## Read first

- `docs/development.md` ("First local Project", "Commands"); `scripts/dev.mjs`
  and `scripts/dev-service.mjs` to see how the loopback backend is used today
- `internal/app/app.go` `Run` (service start, `flushLoop`, `heartbeatLoop`)
- `internal/daemon/` (IPC request/response, `health` method)
- `internal/service/service_darwin.go` (LaunchAgent lifecycle)
- `internal/onboarding/onboarding.go` `Create`, `finish` (the `createInvite`
  boolean), `Join`
- `apps/desktop/onboarding_service_darwin.go` `State`, `CreateProject`,
  `enroll`, `ensureService`; `apps/desktop/desktop_production.go`
  (`apiBaseURL` ldflag, `desktopCLIBinary`) and `desktop_development.go`
- `internal/update/` and `install/install.sh`, `install/uninstall.sh`
- `scripts/build-desktop.mjs`, `scripts/sign-darwin-artifact.sh`,
  `.github/workflows/release.yml`
- `validation/spikes/bundled-backend/README.md` (Lane 01 result)

## Design (fixed; do not revisit)

### Files and layout

Under the profile root `<root>` (see `internal/config.DefaultRoot`):

```
<root>/backend/
  backend.json        # {"version":"<backend release>","bundleRevision":"<sha>","port":N,"sitePort":M,"instanceName":"..."}  0600
  state.sqlite3       # the backend's database                                                                              0600
  storage/            # --local-storage
  backend.log         # rotated by the service, 5 MB cap
```

The instance secret lives in the Keychain under account
`overgent.local-backend.<instanceName>` via `internal/credential`, never in a
file. The backend binary and the release-time deploy artifact (Lane 01's
payload or seed) live in the app bundle at
`Overgent.app/Contents/Resources/backend/`. The desktop records their
absolute paths in `<root>/backend/backend.json` (`binaryPath`,
`bundlePath`) on every launch so the service can find them after an app
update. CLI-only installs (no app) set them with
`overgent backend install --binary <path> --bundle <path>`.

`backend.json` is a new file owned by this lane. It is deliberately **not** a
field in `config.json`, so Lane 06 can change `config.json` independently.

### New package `internal/localbackend`

```go
type Manager struct { /* paths, logger, credential store, clock */ }
func New(root string, creds credential.Store, logger *slog.Logger) (*Manager, error)
func (m *Manager) Ensure(ctx context.Context) (Endpoint, error) // start if needed, wait healthy, deploy bundle if revision differs, set env, return site origin
func (m *Manager) Status(ctx context.Context) Status            // running, pid, ports, version, bundleRevision, lastError, idleSince
func (m *Manager) Stop(ctx context.Context) error
func (m *Manager) Touch()                                        // marks activity; used by idle shutdown
```

Rules:
- Spawn with `exec.CommandContext`, argument arrays only, `--interface
  127.0.0.1`, ports chosen by binding `127.0.0.1:0` twice and releasing (retry
  on race), `--convex-origin http://127.0.0.1:<port>`, `--convex-site
  http://127.0.0.1:<sitePort>`, positional SQLite path, `--local-storage`.
  Stdout/stderr to `backend.log`.
- Health: poll the health route Lane 01 identified, 100 ms interval, 10 s
  budget for cold start (use the measured number plus margin).
- Deploy: if `backend.json.bundleRevision` differs from the bundle shipped
  with the app, run the push step and then the env step
  (`OVERGENT_SECRETS_KEY` generated once per install and stored in the
  Keychain under `overgent.local-backend.secrets-key`; Lane 04 reads it from
  the deployment, not from the Go side). Record the new revision only after
  both succeed.
- Restart with exponential backoff (1 s, 2 s, 4 s … cap 60 s) on unexpected
  exit; after 5 consecutive failures inside 5 minutes stop retrying and
  surface `lastError` through `health`.
- Idle shutdown: if Lane 01 measured idle RSS above 300 MB, stop the backend
  after 30 minutes without queue activity, heartbeats, or IPC calls, and
  restart on the next `Ensure`. Otherwise keep it running while the service
  runs. Either way `Ensure` is called before every hosted client use for a
  local Project (see wiring).
- On service shutdown, send SIGTERM, wait 5 s, then SIGKILL. Never leave an
  orphan: also write the child pid to `backend.json` and kill a stale pid on
  start if it is still a `convex-local-backend` process owned by this user.

### Wiring in the service

In `internal/app/app.go` `Run`: when `cfg.APIBaseURL` is a loopback origin
**and** `<root>/backend/backend.json` exists, construct the manager and call
`Ensure` before the first flush and heartbeat, and again lazily whenever a
send fails with connection refused. Add `backend` to the `health` IPC
response (`Status` fields). Do not change the flush or heartbeat logic itself.

Lane 06 later moves the "is this Project local?" decision from
`cfg.APIBaseURL` to the per-Project binding; keep the check in one helper
function `isLocalBackendOrigin(origin string) bool` so that move is a
one-line change.

### CLI

Add to `cmd/overgent/main.go`:

```
overgent backend status        # JSON with --json: running, ports, version, bundleRevision, db path, size on disk
overgent backend start|stop
overgent backend install --binary <path> --bundle <path>
overgent backend reset         # stops, deletes <root>/backend/state.sqlite3 and storage/, keeps backend.json paths; asks for confirmation unless --yes
overgent create --local ...    # equivalent to --api http://127.0.0.1:<sitePort> after Ensure; the invite creation in onboarding.finish is skipped
```

`create --local` and `--api` are mutually exclusive.

### Desktop

- First-run onboarding shows two choices, in this order: **Use on this Mac**
  (local, default, one sentence: "Nothing leaves this computer.") and
  **Create or join a team Project** (existing flow against
  `desktopAPIBaseURL()`; text says data is stored on Overgent Cloud, link to
  the privacy doc). Follow `docs/design-system.md`; no cards, no new colour.
- "Use on this Mac" calls a new `OnboardingService.CreateLocalProject`, which
  runs the manager's `Ensure` (via the CLI or service IPC; the desktop must not
  spawn the backend itself, the service owns it), then the existing `enroll`
  path with `apiBaseURL` set to the returned loopback origin and
  `createInvite=false`.
- Until Lane 06 lands, a profile is either local or team. If the config
  already has the other kind, the onboarding shows the existing profile and a
  single explanatory line: "This Mac is set up for <local|team> Projects.
  Reset to switch." Do not build a switcher.
- Service health in the menu shows the backend state using the same
  vocabulary as service health (`formatElapsed` for uptime, monospace for
  ports and version). No pulsing dots.
- The dashboard opens against the loopback origin exactly as the development
  desktop does (`OVERGENT_DASHBOARD_ORIGIN` path in `desktop_development.go`);
  in production derive it from the manager's endpoint instead of the env var.

### Packaging and release

- `scripts/build-desktop.mjs`: run `scripts/fetch-backend.mjs` (Lane 01),
  copy the binary and the deploy artifact into
  `Contents/Resources/backend/`, and include them in signing
  (`scripts/sign-darwin-artifact.sh` must sign the nested executable with the
  same identity and hardened runtime; Lane 01 recorded whether an entitlement
  is needed).
- `.github/workflows/release.yml`: produce the deploy artifact at release
  time from the tagged commit (Lane 01's `push.sh`), so the bundle revision
  is the tag's commit.
- `install/install.sh`: no change if the app bundle carries the backend.
  `install/uninstall.sh`: also remove `<root>/backend/` and the two Keychain
  items; keep the existing confirmation behavior.
- `NOTICE`: add the Convex backend third-party entry drafted in Lane 01.
- `internal/update/`: after a verified app update, the service re-runs
  `Ensure` which re-pushes the bundle when the revision changed. Lane 01 task 4
  says whether a schema-incompatible push can fail; if it can, the update
  path must keep the previous `state.sqlite3` as `state.sqlite3.bak-<rev>`
  before pushing and restore it on failure, surfacing the failure in `health`.

### Data and privacy

- The local backend binds loopback only; `--interface 127.0.0.1` is asserted
  in a test that inspects the argument array.
- The backend database contains coordination facts for the member's own
  Projects. `overgent diagnostics` must not include it; it may include
  `backend.json` minus `instanceName`.
- Export: `overgent backend export --out <dir>` copies `state.sqlite3` while
  the backend is stopped, or uses the backend's export route if Lane 01 found
  one. Minimal; JSON export via the existing owner-export endpoints is already
  available against any backend.

## Tests

- `internal/localbackend`: unit tests with a fake binary (a shell script or a
  tiny Go test helper built with `-o`) that honors the same flags, prints a
  health response, and can be told to crash; cover start, health timeout,
  restart backoff, stale pid cleanup, idle stop, bundle revision gating.
  Tests use temp roots; never the real `~/Library`.
- `cmd/overgent`: `backend status --json` shape; `create --local` refuses
  `--api`.
- Desktop: `apps/desktop/*_test.go` for `CreateLocalProject` state
  transitions, mirroring the existing `onboarding_service_darwin_test.go`
  patterns.
- Live: extend `docs/development.md` with a "Local mode from a built app"
  section and run it once by hand on this Mac; record the evidence file under
  `validation/evidence/l9-local-mode.md` with the two-worktree Codex/Claude
  collision exercise from `docs/development.md` executed against the bundled
  backend.

## Acceptance

- Fresh temp profile, no Node on `PATH`, built app: "Use on this Mac" →
  choose a repository → dashboard shows the Project within the cold-start
  budget; a second worktree session in the same repository produces a
  collision finding; `lsof -nP -iTCP -sTCP:LISTEN` shows the backend only on
  `127.0.0.1`.
- Quit and relaunch: the same Project and findings are still there.
- `overgent backend reset` followed by relaunch returns to first-run.
- `pnpm dev` still works unchanged (development keeps using `convex dev`).
- All verification commands in `migration/README.md` pass; `pnpm desktop:test`
  included.

## Out of scope

Per-Project mode (Lane 06), AI keys (Lane 04), non-macOS packaging (ADR-050
still applies), migrating a local Project to a team Project.
