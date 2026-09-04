# Overgent Cloud operations

This is the public runbook for `api.overgent.com` and `releases.overgent.com`,
the owner-operated deployment of this same public code. It contains no
account identifiers, deployment names, tokens, or hostnames beyond those two.
For running your own backend instead, see `self-hosting.md`.

## 1. What these are

- **`api.overgent.com`** is the default team backend: a Convex production
  deployment running the functions in `convex/functions/`, fronted by the
  Vercel project defined in `vercel.json` and `api/`. `api/v1/[...].js`
  proxies `/v1/*` requests to the Convex deployment's HTTP Actions origin;
  `api/releases/current-manifest.js` serves the signed update manifest that
  `releases.overgent.com` publishes.
- **`releases.overgent.com`** is the public release channel: signed,
  notarized desktop builds and the CLI, published by
  `.github/workflows/release.yml` and promoted through the scripts under
  `scripts/`.
- **Deployment shape:** one Vercel project (dashboard SPA plus the two proxy
  functions above) in front of one Convex production deployment. There is no
  separate application server; Convex is the database, live-query, and
  function runtime, and Vercel only routes and serves static assets.

## 2. Environment variables

Names and meaning only — values are operational and live outside this
repository.

| Variable | Meaning |
|---|---|
| `CONVEX_SITE_URL` | The Convex production deployment's HTTP Actions origin. `api/v1/[...].js` proxies every `/v1/*` request there. |
| `OVERGENT_SECRETS_KEY` | Encrypts per-Project AI provider keys at rest (ADR-073). Stable for the life of the deployment; rotating it invalidates every stored key. |
| `OVERGENT_OPERATOR_KEYS_ENABLED` | Unset on Cloud by policy (ADR-073): Overgent Cloud ships with no operator key, so every Project brings its own AI provider key, or runs with semantic features degraded. |
| `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` | Operator-level AI provider keys. Not set on Cloud, per the policy above; documented here because a self-hoster may choose to set them on their own deployment. |
| `BLOB_READ_WRITE_TOKEN` | Write credential for the public release Blob store used by the release-publishing workflow only; unrelated to the `/v1` API surface. |

## 3. Data handling

- **What is stored:** coordination metadata only — Projects, memberships,
  devices, events, findings, briefs, and sync cards — under the same
  prohibited-data boundary as any other deployment of this code
  (`security-privacy.md`).
- **Retention:** a bounded sweep (`convex/functions/crons.ts`) runs every
  five minutes; hosted activity, findings, semantic objects, deliveries, and
  shared session messages default to 30-day retention.
- **Export and deletion:** any Project owner can export their Project
  (`GET /v1/projects/{id}/export`, a JSON attachment) or delete it
  (`DELETE /v1/projects/{id}`); an ordinary member can export or delete the
  records about their own work. Authorization is revoked synchronously
  before deletion proceeds in bounded batches.
- **Security reporting:** see this repository's `SECURITY.md` for how to
  report a vulnerability in either the code or the hosted deployment.

## 4. Cost posture

Overgent Cloud runs on the owner's Convex account. There is no paid tier
today. If costs become material the options are a sponsors link or a team
tier on the hosted side; the source stays open either way.

## 5. What stays out of this document

Account identifiers, specific deployment names, incident details, and any
other operational specifics that would let someone target the running
deployment stay in the owner's private notes, not here. This document
describes the shape of the deployment, not how to reach or attack it.
