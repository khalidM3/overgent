# Phase 1 — Spike: ship the Convex backend inside the desktop release

Status: experimental brief; produces a written result, not a merged feature
Last updated: 2026-09-04
Executor: Sonnet 5. Report honestly; a "does not work" result is a valid result.

## Question

Can the desktop release bundle `convex-local-backend`, start it on loopback
from the Go service, deploy this repository's Convex functions to it **without
Node, npm, pnpm, or the Convex CLI on the member's machine**, and set its
deployment environment variables, so that a first-run Project can be created
against it exactly as `pnpm dev` does today?

Everything in Lane 03 depends on the answer. Do not start Lane 03 until this
spike's result is accepted by the owner.

## What already exists (read first)

- `package.json` script `dev:backend`: `CONVEX_AGENT_MODE=anonymous CI=true
  convex dev --tail-logs disable --typecheck enable` in `convex/`. This
  downloads a precompiled backend to
  `~/.cache/convex/binaries/precompiled-<date>-<sha>/convex-local-backend`
  (about 160 MB on this machine) and pushes `convex/functions/` to it.
- `scripts/dev.mjs` points the desktop at `http://127.0.0.1:3211`
  (`OVERGENT_API_ORIGIN`) and the service creates Projects against it via
  `./bin/overgent --api http://127.0.0.1:3211 create ...`
  (`docs/development.md` "First local Project").
- `internal/hosted/client.go` `New` accepts plain HTTP for loopback hosts.
  `internal/activation/activation_darwin.go` accepts loopback for dashboard
  activation. No client change is needed for a loopback backend.
- `convex-local-backend --help` (run the cached binary) shows the flags that
  matter: positional SQLite path, `--interface`, `--port`,
  `--site-proxy-port`, `--convex-origin`, `--convex-site`, `--instance-name`,
  `--instance-secret`, `--local-storage`, and a `keygen` subcommand that
  derives the admin key from instance name and secret.
- `convex/functions/intelligence.ts` reads `process.env.OPENAI_API_KEY` and
  `process.env.ANTHROPIC_API_KEY`: deployment environment variables must be
  settable on the local backend for Lane 04 to work in local mode.
- The Convex CLI source is in `convex/node_modules/convex/dist/`. It contains
  the HTTP calls used to push code and set env vars; grep it for
  `push_config`, `deploy2`, `update_environment_variables`, and
  `Convex-Client` headers. Treat it as the reference for the wire shape.

## Tasks

Work in a scratch directory outside the repository for experiments; commit
only the deliverables listed at the end.

1. **Standalone start.** Start the cached binary with a per-install instance
   name and secret, `--interface 127.0.0.1`, free ports, an SQLite path under a
   temp directory, `--convex-origin http://127.0.0.1:<port>`,
   `--convex-site http://127.0.0.1:<sitePort>`. Record cold-start time to a
   healthy `GET /version` (or whichever health route the binary exposes),
   idle RSS after 60 s, and RSS after the L2 live suite (`pnpm test:live` in
   `convex/`) has run against it.

2. **Push without the CLI.** Capture what `convex dev --once` sends to the
   local backend. Two acceptable capture methods: read the CLI source, or
   run the backend behind a logging reverse proxy and point the CLI at it via
   `CONVEX_URL`. Then reproduce the push with `curl` only: build the function
   bundle once (the CLI's bundler output, or `convex deploy --dry-run`-style
   output if available), save the exact request body as
   `backend-push.json`, and replay it against a fresh backend with the admin
   key from `keygen`. Success means `GET /v1/device/bootstrap` on the site port
   answers 401 (route exists, auth enforced) and `overgent --api ... create`
   completes.
   - If the push API is versioned and stable enough to replay, this is
     **Option A** (Go replays a release-time payload).
   - If it is not, test **Option B**: run the CLI push once at release time,
     stop the backend, and ship the resulting SQLite file as the seed. Verify
     that a seed created under instance secret S1 still works when the backend
     starts with secret S2 (per-install secrets matter for the admin key); if
     it does not, record whether a per-install secret is actually required
     when the interface is loopback-only and the database is `0600`.

3. **Environment variables.** Set `OPENAI_API_KEY=synthetic` and a new
   `OVERGENT_SECRETS_KEY` on the local deployment through the admin HTTP API
   (not the CLI), then confirm an action reads them (`process.env` inside a
   throwaway internal action, or the existing `embedSemanticObject` path with
   the live suite). Record the exact endpoint and body.

4. **Upgrade path.** Push a trivially changed bundle (add one field to a
   table in `schema.ts` in a scratch copy) to a backend that already holds
   data. Record whether the push succeeds, whether existing rows survive, and
   what happens on a schema-incompatible change. Lane 03 needs to know whether
   "app update re-pushes the bundle" is safe.

5. **Bundle shape.** Copy the binary into a throwaway
   `Overgent.app/Contents/Resources/backend/` on this Mac, sign it ad hoc with
   `codesign --force --sign -`, and start it from there. Record whether
   Gatekeeper or the hardened runtime blocks it, and whether the binary needs
   any entitlement when the app is notarized (read
   `scripts/sign-darwin-artifact.sh` and `scripts/build-desktop.mjs` for the
   current signing flow; do not run the notarization workflow).

6. **License check.** Record the license of the `get-convex/convex-backend`
   release you used (expected FSL-1.1-Apache-2.0) and draft the `NOTICE`
   paragraph Lane 03 will add.

## Deliverables (commit these)

- `validation/spikes/bundled-backend/README.md`: results for each task with
  measured numbers, exact endpoints and request shapes, the chosen option
  (A or B) with the reason, and open risks.
- `validation/spikes/bundled-backend/push.sh` (or `.mjs` for release-time
  use only): the reproducible steps that produce the release-time artifact
  (the push payload for A, the seed database for B).
- `scripts/fetch-backend.mjs`: downloads the pinned `convex-local-backend`
  release for `darwin/arm64` into `apps/desktop/build/backend/` and verifies a
  SHA-256 recorded in `scripts/backend-version.json`
  (`{ "version": "<release tag>", "sha256": { "darwin-arm64": "..." } }`).
  Fail closed on mismatch. No network call anywhere else.

## Acceptance

- A fresh backend started by hand from the copied binary, with functions
  deployed by the chosen option and no Node on `PATH`, accepts
  `overgent --api http://127.0.0.1:<sitePort> create --root <temp repo>`
  and the desktop development build opens the Project dashboard against it.
- Cold start, idle RSS, and bundle size are recorded. If idle RSS exceeds
  300 MB or cold start exceeds 5 s, say so; Lane 03 then makes idle shutdown
  mandatory rather than optional.
- The README states clearly whether Option A or B is recommended, or that
  neither works and why. In the last case also record the cheapest fallback
  you found (for example bundling a Node runtime and the CLI), with its size.

## Out of scope

Supervision code in Go, desktop UI, installer changes, per-Project mode.
Those are Lane 03.
