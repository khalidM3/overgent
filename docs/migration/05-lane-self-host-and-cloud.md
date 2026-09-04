# Lane 05 — Self-hosting and Overgent Cloud from the same public code

Status: brief
Last updated: 2026-09-04
Executor: Haiku 4.5 or Sonnet 5. Mostly documentation plus one small client
change. Depends on ADR-071/072 accepted. Independent of Lanes 03, 04, 06,
but the docs must mention Lane 04's env vars, so write those sections last.

## Goal

Anyone can run their own backend from this repository and point a stock
Overgent build at it, and the owner can run `api.overgent.com` from the same
code with a public, secret-free runbook. Team mode works against either.

## Read first

- `convex/README.md`, `convex/convex.json`, `convex/package.json`,
  `convex/functions/crons.ts` (retention), `convex/functions/http.ts` (the
  full route list is the public surface)
- `api/v1/[...].js` and `api/releases/current-manifest.js`, `vercel.json`,
  `tests/vercel-proxy.test.cjs` (why the dashboard needs same-origin `/v1`)
- `apps/desktop/desktop_production.go` (`apiBaseURL`, "Activation rejects
  anything that is not HTTPS"), `desktop_development.go`
- `cmd/overgent/main.go` `create`/`join` (`--api` flag already exists)
- `internal/hosted/client.go` `New` (HTTPS required except loopback)
- `docs/beta-release.md`, `docs/development.md` "Shared two-Mac dogfood"
  (ADR-041), `docs/security-privacy.md` "Hosted"
- `.github/workflows/release.yml` (what the release publishes and where)

## Deliverable 1: `docs/self-hosting.md`

Sections, in this order:

1. **What you are hosting.** One paragraph: the backend stores coordination
   metadata only (link the prohibited-data list in `security-privacy.md`);
   the desktop and CLI talk to it over `/v1` (link `protocol.md`); Convex
   provides the database, live queries, and function runtime.
2. **Option A — your own Convex Cloud project (recommended).**
   `cd convex && npx convex deploy` with the reader's own Convex account;
   set `OVERGENT_SECRETS_KEY` (generate: `openssl rand -base64 32`), optional
   `OVERGENT_OPERATOR_KEYS_ENABLED`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`
   via `npx convex env set`; the resulting `*.convex.site` URL is the API
   origin for HTTP routes. State the free-tier fit honestly (coordination
   metadata is small; the cron retention in `crons.ts` bounds growth).
3. **Option B — self-hosted `convex-backend` server.** The
   `ghcr.io/get-convex/convex-backend` image (pin the same release as
   `scripts/backend-version.json`), `--convex-origin` / `--convex-site` set to
   the public HTTPS URLs, a reverse proxy (Caddy example) terminating TLS,
   `npx convex deploy --url <origin> --admin-key <key>` where the key comes
   from the image's `generate_admin_key.sh`; the same env vars set with
   `npx convex env set --url ... --admin-key ...`. Note the FSL license of
   the backend image and that Overgent only redistributes it.
4. **Dashboard hosting.** Two choices: (a) skip it, the desktop app embeds
   the dashboard and opens it against your origin; (b) host the SPA
   (`pnpm --dir apps/dashboard build`) behind the same origin with `/v1`
   proxied to the Convex site URL, using `api/v1/[...].js` on Vercel or an
   equivalent reverse-proxy rule elsewhere. Explain in two sentences why
   activation needs same-origin (`303` to `/dashboard` on the host it
   authenticated against).
5. **Pointing clients at it.** Desktop: the "Connect to a server" field
   (Deliverable 3). CLI: `overgent create --api https://your-origin ...` and
   `overgent join --api ...`. HTTPS is mandatory except loopback; say why.
6. **Operating it.** Retention crons, the ADR-070 rate ceilings and how to
   change them, export and deletion endpoints (`GET /v1/projects/{id}/export`
   returns a JSON attachment; `DELETE /v1/projects/{id}` deletes; both are in
   `http.ts` and rate-limited under `projects.export`), backups (Convex
   Cloud snapshot export; for Option B, the SQLite/Postgres path), upgrading
   (deploy the functions from the release tag that matches the clients;
   protocol compatibility rules in `protocol.md`).
7. **What is not included.** Accounts, SSO, billing, multi-tenant admin.

## Deliverable 2: `docs/hosted-operations.md` (public runbook, no secrets)

- What `api.overgent.com` and `releases.overgent.com` are, which repository
  paths implement them (`api/`, `vercel.json`, `.github/workflows/`), and the
  deployment shape (Vercel project → Convex production deployment).
- The environment variables the deployment uses (names only, with meaning):
  `CONVEX_SITE_URL`, `OVERGENT_SECRETS_KEY`, `OVERGENT_OPERATOR_KEYS_ENABLED`
  (unset on Cloud by policy per ADR-073), `ANTHROPIC_API_KEY`,
  `OPENAI_API_KEY`, `BLOB_READ_WRITE_TOKEN` (release publishing only).
- Data handling: what is stored, retention, export and deletion, the
  security reporting channel.
- Cost posture: "Overgent Cloud runs on the owner's Convex account. There is
  no paid tier today. If costs become material the options are a sponsors
  link or a team tier on the hosted side; the source stays open either way."
- Move anything operational that names account identifiers or incident
  detail to the owner's private notes, not here.

## Deliverable 3: configurable production API origin (executed by Lane 03's session; kept here as the spec)

Today `apps/desktop/desktop_production.go` hard-codes `apiBaseURL` (ldflag
override at build time). Change:

- `desktopAPIBaseURL()` returns, in order: the origin stored for the profile
  (`config.APIBaseURL` when non-empty), else the build default. Do not add a
  new config field; the field exists.
- The team onboarding path gets an "Advanced: connect to a different server"
  disclosure with one text field. Validation is exactly `hosted.New`'s rule
  (HTTPS origin, or loopback HTTP) plus no path. On success the origin is
  written to `config.APIBaseURL` by the existing `enroll` path (it already
  persists the origin the Project was created against; confirm by reading
  `hotRegister` / `app.Register`).
- `overgent create --api` / `join --api` already exist; add the same
  validation error text so CLI and desktop agree.
- Lane 06 will move this from profile level to Project level; keep the field
  read in one function so that move is local.

## Deliverable 4: retire the ADR-041 shared profile from docs

Once Deliverable 3 lands, `docs/development.md` "Shared two-Mac dogfood"
becomes "Team mode against a development deployment": same instructions,
but the origin is entered in the app or passed with `--api`, not through a
separate profile script. Keep `pnpm dev:shared` working until Lane 06 deletes
it; say in the doc that it is a development convenience.

## Acceptance

- A second Mac (or a second profile with `OVERGENT_CONFIG_ROOT`) follows
  `docs/self-hosting.md` Option A against a throwaway Convex dev deployment
  and completes create → invite → join → collision without reading any other
  document. Record what was unclear and fix the doc.
- `docs/hosted-operations.md` contains no account identifier, deployment
  name, token, or hostname other than the two public ones.
- `pnpm test` (which includes `tests/vercel-proxy.test.cjs`) and
  `pnpm desktop:test` pass.
- `docs/README.md` lists `self-hosting.md` and `hosted-operations.md`.
