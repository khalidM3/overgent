# Bundled Convex backend spike

Date: 2026-09-04

Machine: Apple Silicon (`arm64`), macOS 27.0 beta, Go 1.26.7, Node 23.6.0, pnpm 11.19.0

Convex npm package: 1.45.0

Backend release: `precompiled-2026-08-25-7cce8fb`

## Result

**Recommend Option A: replay a release-time `deploy2` request.** A payload produced once by the pinned Convex 1.45.0 CLI was replayed against an empty backend using `curl`, `jq`, and `brotli`, with no Node, npm, pnpm, or Convex CLI on the replay path. The deployed `/v1` routes worked and the Overgent CLI created a Project with `PATH=/usr/bin:/bin`.

The backend portion of the acceptance criterion passes. The desktop portion does not pass on current `main`: the documented Vite activation redirects the webview to `http://127.0.0.1:5173/?live=1`, where `apps/dashboard/src/main.tsx` treats the page as a normal browser and renders `LandingPage` instead of `LiveApp`. This is a code/brief conflict in files owned by Lane 03, so this spike does not change them.

The `deploy2` endpoints are internal rather than a promised public API. Option A is acceptable only while the backend release, npm CLI version, request payload, and replay implementation are pinned together and regenerated/tested for every release.

## Measurements

| Measurement | Result |
|---|---:|
| Cold start, new SQLite database to healthy `GET /version` | 120.530 ms |
| Idle RSS at 72 s | 57,200 KiB (55.86 MiB) |
| RSS after the L2 live suite had run | 207,616 KiB (202.75 MiB) |
| Backend Mach-O size | 167,291,456 bytes (159.54 MiB) |
| Release zip size | 53,835,331 bytes (51.34 MiB) |
| Full release-time push request | 909,367 bytes (888.05 KiB), 8 modules |
| Compatible optional-field push | 828.040 ms |
| Rejected incompatible required-field push | 632.470 ms |

Both thresholds pass: cold start is below 5 seconds and idle RSS is below 300 MB. The L2 live suite itself failed twice at `live-l2.mjs:383` because the resulting brief omitted the expected `membership-role-schema` finding. The RSS number above is therefore after the complete request sequence up to that deterministic assertion, not after a green suite. This failure reproduced on two consecutive runs.

## 1. Standalone start

The release binary started successfully with a new SQLite file, explicit per-install instance name and 64-hex secret, `--interface 127.0.0.1`, separate cloud/site ports, explicit loopback origins, local storage, and `--disable-beacon`. The healthy route is `GET /version`; this build returns the text `unknown` with HTTP 200.

The sandboxed launch panicked in macOS `system-configuration` initialization (`Attempted to create a NULL object`). The same binary and arguments worked outside the command sandbox. This is a test-harness artifact, not an observed end-user failure.

## 2. Push without the CLI

The pinned CLI's hidden `--write-push-request` option emits the request for:

```text
POST /api/deploy2/start_push
Authorization: Convex <admin-key>
Convex-Client: npm-cli-1.45.0
Content-Type: application/json
Content-Encoding: br
```

The uncompressed JSON top-level shape is:

```json
{
  "adminKey": "<per-install admin key>",
  "dryRun": false,
  "functions": "functions/",
  "appDefinition": {
    "changedModules": "<8 bundled modules plus schema and definition metadata>",
    "unchangedModuleHashes": []
  },
  "componentDefinitions": [],
  "nodeDependencies": [],
  "forCodegen": false
}
```

`start_push` returned HTTP 200. Replay then used:

```json
POST /api/deploy2/wait_for_schema
{"adminKey":"<per-install admin key>","schemaChange":"<start_push.schemaChange>","timeoutMs":10000,"dryRun":false}
```

That returned `{"type":"complete"}`. The final request was Brotli-compressed JSON:

```json
POST /api/deploy2/finish_push
{"adminKey":"<per-install admin key>","startPush":"<complete start_push response>","dryRun":false}
```

It returned HTTP 200 and a create diff for `convex.config.js`, `crons.js`, `http.js`, `intelligence.js`, `schema.js`, and `service.js`. `GET http://127.0.0.1:43103/v1/device/bootstrap` then returned HTTP 401 with `unauthorized`, proving the route existed and enforced authentication.

The bundled `push.sh` has two modes. `build` requires the pinned release-time Node/Convex toolchain and emits a self-contained payload with an admin-key placeholder. `replay` replaces that placeholder and makes the three HTTP calls without Node, npm, pnpm, or the Convex CLI. Lane 03 should implement the same steps in Go and should not ship `jq`, `brotli`, or `curl` as runtime dependencies.

With the resulting backend, this command completed successfully:

```text
env PATH=/usr/bin:/bin overgent --config-root <temp-profile> --api http://127.0.0.1:43103 create --label "Bundled backend spike" --device-label "Spike Mac" --root <temp-repo>
```

The first attempt against an empty Git repository failed honestly with `fatal: Needed a single revision`; after adding one synthetic initial commit, creation returned Project, device, workspace, workstream, and invite IDs.

## 3. Deployment environment variables

The admin endpoint and exact test body were:

```http
POST /api/update_environment_variables
Authorization: Convex <admin-key>
Convex-Client: npm-cli-1.45.0
Content-Type: application/json

{"changes":[{"name":"OPENAI_API_KEY","value":"synthetic"},{"name":"OVERGENT_SECRETS_KEY","value":"synthetic-secrets-key"}]}
```

The response was HTTP 200. A throwaway action returned only equality booleans, never the values:

```json
{"openAIConfigured":true,"secretsConfigured":true}
```

This proves action visibility while preserving the secret boundary.

## 4. Upgrade behavior

Five synthetic `projects` rows existed before the test. Adding optional string and boolean fields to `projects` succeeded; the measured second compatible push took 828.040 ms and the row count remained 5.

Adding a required string field failed closed in 632.470 ms. Convex reported schema validation failure, identified a `projects` document, and explained that `spikeRequiredMarker` was missing. The rejected deployment did not replace the prior bundle and the row count remained 5. App updates may re-push compatible schemas safely; destructive or required-field migrations need an explicit staged migration and must treat validation rejection as a non-upgraded state.

## 5. macOS bundle and signing

The 167,291,456-byte Mach-O was copied to `Overgent.app/Contents/Resources/backend/convex-local-backend`, ad-hoc signed, verified with `codesign --strict`, and started successfully from that path. The copied file had no quarantine attribute; `spctl --assess --type execute` rejected the stand-alone ad-hoc binary, as expected for an unnotarized developer artifact. That result does not predict the enclosing notarized app's Gatekeeper result.

Hardened-runtime signing without entitlements failed during V8 startup:

```text
Fatal process out of memory: Failed to reserve virtual memory for CodeRange
```

Re-signing with only this entitlement restored a healthy `/version` response:

```xml
<key>com.apple.security.cs.allow-jit</key>
<true/>
```

No `com.apple.security.cs.allow-unsigned-executable-memory` or library-validation entitlement was needed in this local test. Lane 03 must sign the nested backend with the release identity, hardened-runtime options, and `allow-jit` before signing/notarizing the enclosing app, then validate the actual notarized artifact on a clean Mac.

## 6. License and NOTICE draft

The release asset includes `LICENSE.md` with **FSL-1.1-Apache-2.0**, notice `Copyright 2026 Convex, Inc.`, and an Apache-2.0 future license effective two years after that version was made available. The release asset itself, rather than the npm package's Apache-2.0 license, governs the redistributed backend binary.

Draft paragraph for Lane 03:

> Overgent redistributes the Convex Local Backend, Copyright 2026 Convex, Inc., under the Functional Source License, Version 1.1, Apache 2.0 Future License (FSL-1.1-Apache-2.0). A copy of that license is included with the distributed backend artifact and is available from the pinned Convex release.

## Open risks and next step

- Fix or supersede the desktop activation conflict before claiming the desktop acceptance criterion. The relevant behavior is split between the loopback redirect in `convex/functions/http.ts` and the `isDesktopWebview` routing condition in `apps/dashboard/src/main.tsx`/`native.ts`; those files are outside this spike's edit ownership.
- Treat all `deploy2` wire shapes as pinned implementation details. A backend/CLI bump must rebuild the artifact and rerun create, env, upgrade, and live-suite checks.
- The backend warns that release-mode action `fetch` is unrestricted without `--convex-http-proxy`. Lane 03 needs either a loopback-safe explicit decision or an SSRF-screening proxy before provider calls are enabled.
- `spctl` and local ad-hoc signing do not replace notarization evidence. Clean-machine assessment remains required.
- Investigate the repeatable L2 live-suite assertion before Lane 03 uses the post-suite RSS as a green workload measurement.

The next unblocked work is owner review of this result, followed by a focused decision on the desktop redirect conflict and then Lane 03 only after acceptance.

## Command log

Commands are listed in execution order by experiment. Repeated read-only source
searches are consolidated, and generated temporary paths/admin keys are shown as
placeholders; every key used was synthetic and every server bound to loopback.

| Command | Result |
|---|---|
| `git -C /Users/khalidmohamud/stickguy worktree add /Users/khalidmohamud/overgent-oss-spike -b oss/spike main` | Passed; created the isolated worktree and branch. |
| `git -C /Users/khalidmohamud/overgent-oss-spike remote -v` | Passed; fetch and push origin are `https://github.com/khalidM3/overgent.git`. |
| `test -d /Users/khalidmohamud/overgent-oss-spike/docs/migration` | Passed. |
| `sed -n ... docs/migration/README.md docs/migration/RUN-PLAN.md docs/migration/01-spike-bundled-backend.md` | Passed; read before implementation. |
| `sed -n ... package.json scripts/dev.mjs docs/development.md internal/hosted/client.go internal/activation/activation_darwin.go convex/functions/intelligence.ts scripts/sign-darwin-artifact.sh scripts/build-desktop.mjs` | Passed; read all brief-listed source and signing files. |
| `sed -n ... docs/README.md` followed by its indexed documents in order | Passed; repository-required architecture/design reading completed. |
| `node --version`; `pnpm --version`; `go version`; `uname -m`; `sw_vers` | Node 23.6.0, pnpm 11.19.0, Go 1.26.7 darwin/arm64, arm64, macOS 27.0 beta. |
| `pnpm install --frozen-lockfile` | First sandboxed run failed `EPERM`; approved worktree run passed, installing 280 packages in 2.2 s. Final rerun passed in 232 ms, already up to date. |
| `node -p "require('./convex/node_modules/convex/package.json').version"` | `1.45.0`. |
| `curl -fsS https://github.com/get-convex/convex-backend/releases/download/latest/backend-version.json` | Returned `precompiled-2026-08-25-7cce8fb`. |
| `find ~/.cache/convex/binaries -name convex-local-backend -type f`; cached binary `--help`; cached binary `--version` | Located the pinned binary; help exposed the required flags; version printed `local_backend unknown`. |
| `stat -f '%z' <binary>`; `shasum -a 256 <binary>` | 167,291,456 bytes; SHA-256 `3fefa471e11eab56aabf86039ddf825ed1b4dbadadec2df6b88b6ffd9d604400`. |
| `curl -fsSIL <release-zip-url>`; release API/asset inspection; `shasum -a 256 <zip>` | 53,835,331 bytes; asset SHA-256 `98831b0f511f6eed70b0b4dfca62015df57877e08017d2b2979b39d62ae7317b`. |
| `unzip -p <release-zip> LICENSE.md` | FSL-1.1-Apache-2.0, Copyright 2026 Convex, Inc., Apache-2.0 future license after two years. |
| Cached backend start with temp SQLite, `--interface 127.0.0.1`, separate ports/origins, per-install name/secret, local storage, and `--disable-beacon` | Sandboxed run panicked in macOS system-configuration; approved run passed. |
| Millisecond timer plus repeated `curl -fsS http://127.0.0.1:<port>/version` | Healthy in 120.530 ms. |
| `sleep 72`; `ps -o rss= -p <pid>` | 57,200 KiB idle RSS. |
| `rg 'push_config|deploy2|update_environment_variables|Convex-Client|write-push-request' convex/node_modules/convex/dist` and focused `sed` reads | Passed; established the CLI wire shape and hidden capture option. |
| `convex deploy --url <loopback> --admin-key <synthetic> --typecheck disable --codegen disable --skip-workos-check --push-all-modules --write-push-request <scratch>/backend-push` | Passed; emitted a 909,367-byte request containing 8 modules and no external Node dependencies. |
| Brotli-compressed `curl` POSTs to `/api/deploy2/start_push`, `/api/deploy2/wait_for_schema`, and `/api/deploy2/finish_push` | HTTP 200 for all three; schema completed and functions were created. |
| `curl http://127.0.0.1:<site-port>/v1/device/bootstrap` | HTTP 401 `unauthorized`; route exists and enforces auth. |
| `go build -o bin/overgent ./cmd/overgent` | Passed. |
| `git init <temp-repo>` then `overgent --api <loopback-site> create ...` | First create failed honestly because the repository had no revision. |
| Add synthetic README, `git add`, `git commit`, then `env PATH=/usr/bin:/bin overgent --api <loopback-site> create ...` | Passed with Project/device/workspace/workstream/invite IDs and no Node on `PATH`. |
| Temporary `convex/.env.local` plus `pnpm test:live` | Failed at `live-l2.mjs:383`: missing expected `membership-role-schema` finding. |
| `pnpm test:live` (second run) | Same deterministic failure. |
| `ps -o rss= -p <pid>` after the second live run | 207,616 KiB. |
| `curl -X POST <backend>/api/update_environment_variables` with the exact JSON shown above | HTTP 200. |
| Release-time push of a throwaway action, then action invocation | Returned `{"openAIConfigured":true,"secretsConfigured":true}`; no secret values were returned. |
| Seed five synthetic `projects` rows, push scratch schema with optional fields, query row count | Push passed; measured repeat was 828.040 ms; row count stayed 5. |
| Push scratch schema with required `spikeRequiredMarker`, query row count | Failed closed in 632.470 ms with the expected schema error; row count stayed 5. |
| Copy binary into `Overgent.app/Contents/Resources/backend`, `codesign --force --sign -`, `codesign --verify --strict`, start, and curl `/version` | Signing and verification passed; backend started successfully from the bundle path. |
| `xattr -l <bundled-binary>`; `spctl --assess --type execute -vv <bundled-binary>` | No quarantine attribute; standalone ad-hoc binary rejected as unnotarized. |
| `codesign --force --options runtime --sign - <bundled-binary>` then start | Signing passed; startup failed with V8 CodeRange virtual-memory OOM. |
| Hardened-runtime re-sign with only `com.apple.security.cs.allow-jit=true`, then start and curl `/version` | Passed; healthy without unsigned-executable-memory or library-validation entitlements. |
| `node scripts/fetch-backend.mjs`; size/mode/hash inspection | Passed; downloaded and verified the pinned asset into `apps/desktop/build/backend/`. |
| Temporarily replace manifest SHA with zeros, rerun fetcher, restore manifest, re-hash existing destination | Failed closed with expected/actual checksum report; previous verified binary remained unchanged. |
| `pnpm desktop:assets`; `node scripts/build-desktop.mjs --development` | Passed; Vite emitted only the existing greater-than-500-kB chunk warning. |
| Start Vite with `OVERGENT_DASHBOARD_API_ORIGIN=<loopback-site>`, launch the desktop development app, and drive its webview | Project appeared and Open Project navigated to `/?live=1`; the destination rendered `LandingPage`, not `LiveApp`. |
| `rg`/`sed` inspection of `convex/functions/http.ts`, `apps/dashboard/src/main.tsx`, and `apps/dashboard/src/native.ts` | Confirmed the redirect/classification conflict; no Lane 03-owned files were edited. |
| `sh -n validation/spikes/bundled-backend/push.sh` | Passed. |
| `push.sh build <fresh-loopback> <synthetic-admin-key> <artifact-dir>` then `push.sh replay .../backend-push.json` | Passed; replay reported six created runtime modules. |
| Final `curl` of `/v1/device/bootstrap` | HTTP 401 with authenticated-route error body. |
| `git clean -nd -- apps/desktop/build`; `git clean -fd -- apps/desktop/build` | Dry run identified only generated backend output; cleanup removed it. |
| `go test ./...` | Passed. |
| `go vet ./...` | Passed. |
| `pnpm typecheck` | Passed across all workspace packages. |
| `pnpm test` | Passed: root 6, coordination 44, dashboard 101, protocol contract 1, Convex 60. |
| `pnpm build` | Passed; dashboard retained the existing chunk-size warning. |
| `pnpm desktop:test` | Passed (`go test` and `go vet` for `apps/desktop`). |
| `node --check scripts/fetch-backend.mjs`; parse `scripts/backend-version.json`; inspect file modes | Passed; manifest is valid JSON and `push.sh` is executable. |
| `git diff --check` | Passed. |
