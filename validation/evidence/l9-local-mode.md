# L9 — Local mode on the bundled backend

Date: 2026-09-04
Machine: Apple Silicon (`arm64`), macOS 27.0, Go 1.26.7, Node 22.23.2, pnpm 11.19.0
Backend release: `precompiled-2026-08-25-7cce8fb` (SHA-256 `3fefa471…d604400`, matches Lane 01)
Convex CLI that recorded the payload: 1.45.0
Lane: `docs/migration/03-lane-local-mode.md` plus Lane 05 Deliverable 3

## Result

A Project created with `--local` runs entirely on the bundled Convex backend on
loopback, with no account, no network, and no Node on `PATH`. Two agent
sessions editing one file produced a `direct_collision` finding from the local
backend's own coordination functions. Quit and relaunch keeps the Project, the
findings, and the port. `overgent backend reset` returns the profile to first
run.

**The desktop half of the acceptance criterion was not exercised end to end.**
The app builds, the nested backend is signed with the hardened runtime and
starts from inside the bundle, and the loopback dashboard origin is unit
tested; driving the first-run window and reading the rendered dashboard was not
done in this session, and notarized clean-Mac validation is a release step.
See "Not verified here".

## Measurements

| Measurement | Result |
|---|---:|
| `backend start` on a fresh profile: cold start, deploy2 replay, env step | 5.57 s |
| `create --local` end to end on a fresh profile (includes the above) | 5.63 s |
| Backend cold start alone, healthy `GET /version` (Lane 01, unchanged) | 120 ms |
| Deploy payload | 909,301 bytes |
| Bundled backend Mach-O, after signing | 166,319,456 bytes |
| Signed app bundle backend directory | binary + `backend-push.json` + `LICENSE.md` |

The 5.5 s figure is dominated by the one-time deploy2 replay, not by the
backend. It is paid once per bundle revision: a later `Ensure` with an
unchanged revision skips the push, which is why relaunch is not measured
separately below.

## What was run

### 1. Payload generation and replay verification

`node scripts/fetch-backend.mjs` downloaded and checksummed the pinned binary
and the release's `LICENSE.md`. A scratch backend on 127.0.0.1:3220/3221 was
started by hand, `push.sh build` recorded the payload from this worktree's
Convex functions, and the Go replay was then run against a *fresh* backend
with `PATH=/usr/bin:/bin`:

```
overgent backend verify --binary …/convex-local-backend --bundle …/backend-push.json
{"verified":true}
```

`verify` starts a throwaway backend, replays the three `deploy2` requests, sets
the deployment environment, and asserts `GET /v1/device/bootstrap` answers 401 —
which proves the functions are deployed, not merely that a listener exists.

**This step failed the first time and found a real defect.** `wait_for_schema`
answered HTTP 400 `unknown variant SerializedDeveloperIndexConfig`. The
backend's `start_push` response contains objects with a repeated `"type"` key;
`jq` and JavaScript keep only the last, so the spike's shell replay and the
Convex CLI never saw it, while Go passing the bytes back verbatim as a
`json.RawMessage` did. `normalize` in `internal/localbackend/deploy.go` now
applies the same last-one-wins rule, and
`TestNormalizeCollapsesRepeatedKeysLikeJSONParse` is the regression test.

### 2. Local Project on a fresh temp profile, no Node

```
overgent --config-root $P backend install --binary … --bundle …
overgent --config-root $P create --local --label "Local dogfood" --device-label "L9 Mac" --root $REPO
{"projectId":"prj_74e9…","deviceId":"dev_e4ec…","workspaceId":"wsp_local_7b0a…","workstreamId":"wrk_local_159a…","joinCode":""}
```

Every command ran under `env PATH=/usr/bin:/bin`. `joinCode` is empty: a local
Project mints no invite. `config.json` records
`"apiBaseUrl": "http://127.0.0.1:<sitePort>"`.

`overgent --api https://api.overgent.com create --local` was refused with
`create accepts --local or --api, not both`.

### 3. Collision between two agent sessions

With the service running, two sessions (`--vendor codex` and `--vendor claude`)
each reported a `SessionStart` and a `PostToolUse` edit on the same
`alpha.txt`, through the same `agent-hook` path the installed adapters use. The
Project export from the local backend then contained:

```
FINDING direct_collision medium Both active agent sessions reported work on alpha.txt.
```

The coordination engine is the same `convex/functions/service.ts` the hosted
deployment runs; nothing about it was reimplemented for local mode.

### 4. Loopback only

```
lsof -nP -iTCP -sTCP:LISTEN -a -p <backend pid>
convex-lo 68302 khalidmohamud 10u IPv4 … TCP 127.0.0.1:52543 (LISTEN)
convex-lo 68302 khalidmohamud 11u IPv4 … TCP 127.0.0.1:52544 (LISTEN)
```

Both listeners are `127.0.0.1`. `TestArgumentsBindLoopbackOnly` asserts the
same property on the argument array without starting anything.

### 5. Quit and relaunch

SIGTERM to the service stopped the backend with it (`ps` showed the pid gone).
Restarting the service produced:

```
after relaunch: sitePort 52544 running True pending 0 publishError ''
after relaunch: findings 1 members 1
```

Same port, same Project, same finding, empty queue.

**Port stability was a fix, not a given.** The first run of this exercise
allocated fresh ports on every start, which left `config.APIBaseURL` naming a
closed socket and the queue silently `offline`. `startLocked` now reuses the
recorded ports whenever they are still bindable, and reports
`backend came back on a new port; existing local Projects need overgent backend reset`
when it cannot.

### 6. Reset returns to first run

```
overgent --config-root $P backend reset --yes
{"clearedWorkspaces":1,"configRoot":"…","reset":true}
```

Without `--yes` it prompts and `n` aborts with `backend reset cancelled`. After
reset, `config.json` is `{"version":1,"apiBaseUrl":"","deviceId":"","workspaces":null}`,
the database and storage are gone, `backend.json` keeps only the artifact paths
and the instance name, and `service run` answers
`service is not enrolled; run overgent create or join`.

**This also found a defect.** The service and a CLI command are two managers
over one profile; `Stop` wrote its whole in-memory record, so a service
shutting down after a reset wrote the reset away. `Stop` now clears only the
pid, against whatever is on disk. `TestStopDoesNotResurrectAResetFromAnotherManager`
covers it.

### 7. Signed app bundle

`pnpm desktop:build` produced `Overgent.app` carrying
`Contents/Resources/backend/{convex-local-backend,backend-push.json,LICENSE.md}`.
The nested binary is signed before the enclosing app and carries exactly one
entitlement:

```
<key>com.apple.security.cs.allow-jit</key><true/>
```

`codesign --verify --strict` passes on the app. Installing that bundle's
backend into a second profile and starting it gave
`GET /v1/device/bootstrap -> 401`, so the signed, hardened-runtime binary runs
and serves the deployed contract from inside the app.

## Not verified here

- **The first-run window itself.** "Use on this Mac" → repository → dashboard
  was not driven through the desktop UI. The pieces are covered separately —
  `CreateLocalProject` state transitions and the loopback dashboard origin in
  `apps/desktop/*_test.go`, the first-run screens and the routing fix in
  `apps/dashboard/test/` — but the assembled path has not been walked by a
  person. This is the one acceptance line this file does not close.
- **Notarization and Gatekeeper on a clean Mac.** Ad-hoc signing was used here;
  the release identity, notarization, and clean-machine assessment remain a
  release-time step, as Lane 01 said.
- **A second worktree.** The collision was produced by two agent sessions in
  one checkout, which is what `docs/development.md` calls the normal exercise.
  The linked-worktree variant is described there as an optional advanced setup
  and was not run.
- **Idle shutdown.** Lane 01 measured 56 MB idle RSS, well under the brief's
  300 MB threshold, so the backend is kept running while the service runs. The
  mechanism exists and is unit tested with a non-zero timeout; the shipped
  timeout is zero.
