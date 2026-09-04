# Self-hosting Overgent

This document is for anyone who wants to run their own Overgent backend
instead of using the hosted default at `api.overgent.com`, and point a stock
Overgent desktop or CLI build at it. It assumes only a checkout of this
repository; it does not assume any private access.

## 1. What you are hosting

The backend is the Convex deployment in `convex/`: the database, live
queries, and function runtime behind the frozen `/v1` HTTP contract
(`protocol.md`). It stores coordination metadata only — Project, membership,
device, event, finding, brief, and sync-card records — never source, diffs,
file or tool-result content, Git objects, transcript files, prompts, hidden
reasoning, environment values, or credentials; the full prohibited-data list
is in `security-privacy.md`. The desktop app and CLI never talk to Convex
directly — every client speaks the same `/v1` HTTP contract, so anything that
implements that contract can stand in for `api.overgent.com`.

There are two ways to run it: your own Convex Cloud project, or a
self-hosted `convex-backend` server. Both deploy the same `convex/functions/`
code; neither requires forking this repository.

## 2. Option A — your own Convex Cloud project (recommended)

This is the fastest path and needs no server of your own.

1. Create a Convex account and deploy the functions in this repository to a
   project of your own:

   ```bash
   cd convex && npx convex deploy
   ```

2. Generate a secrets key and set it, along with any semantic-provider keys
   you want the deployment to offer, using `npx convex env set` (never a
   `.env` file committed to a fork — these are deployment secrets):

   ```bash
   openssl rand -base64 32 | npx convex env set OVERGENT_SECRETS_KEY
   npx convex env set ANTHROPIC_API_KEY sk-...      # optional
   npx convex env set OPENAI_API_KEY sk-...         # optional
   npx convex env set OVERGENT_OPERATOR_KEYS_ENABLED true   # optional
   ```

   `OVERGENT_SECRETS_KEY` encrypts per-Project AI provider keys at rest
   (ADR-073); generate it once and keep it stable — rotating it invalidates
   every stored key. `ANTHROPIC_API_KEY` and `OPENAI_API_KEY` are operator
   keys: a deployment-wide fallback used only when a Project has not
   configured its own key and `OVERGENT_OPERATOR_KEYS_ENABLED` is set. Leave
   `OVERGENT_OPERATOR_KEYS_ENABLED` unset if you would rather every Project
   bring its own key or run without semantic features.

3. The deployment's HTTP Actions URL — the `*.convex.site` origin shown by
   `npx convex deploy`, not the `*.convex.cloud` client URL — is the API
   origin clients connect to.

**Free-tier fit.** Coordination metadata is small: events, findings, and
sync-card records, not source or transcripts. The bounded retention sweep in
`convex/functions/crons.ts` runs every five minutes and keeps old records
from accumulating, so storage stays roughly proportional to your team's
active window rather than growing without bound. A handful of active
Projects fits comfortably inside Convex's free tier; heavier usage or a large
number of Projects may need a paid Convex plan.

## 3. Option B — self-hosted `convex-backend` server

Run the same open-source backend Convex publishes as a container, on
infrastructure you control.

1. Pull the image pinned to the same release Overgent's bundled local backend
   uses, recorded in `scripts/backend-version.json` (this file is written by
   the bundled-backend packaging work; if it is not yet present in your
   checkout, use the latest tagged release of
   `ghcr.io/get-convex/convex-backend` and note the version you chose):

   ```bash
   docker pull ghcr.io/get-convex/convex-backend:<pinned-tag>
   ```

2. Run it with `--convex-origin` and `--convex-site` set to the public HTTPS
   URLs you will expose (the client-facing origin and the HTTP Actions
   origin), and put a reverse proxy in front that terminates TLS. A minimal
   Caddy example:

   ```
   convex.example.com {
       reverse_proxy 127.0.0.1:3210
   }
   convex-site.example.com {
       reverse_proxy 127.0.0.1:3211
   }
   ```

3. Generate an admin key with the image's bundled script
   (`generate_admin_key.sh`, documented in the `convex-backend` repository),
   then deploy this repository's functions and set the same environment
   variables as Option A, pointing the CLI at your server instead of Convex
   Cloud:

   ```bash
   npx convex deploy --url https://convex-site.example.com --admin-key <key>
   openssl rand -base64 32 | npx convex env set --url https://convex-site.example.com --admin-key <key> OVERGENT_SECRETS_KEY
   ```

The `convex-backend` image is licensed under FSL-1.1-Apache-2.0. Overgent
redistributes the pinned release for its bundled local mode but does not
modify it; running your own copy from Option B is subject to that license
directly from Convex, not from Overgent.

## 4. Dashboard hosting

You have two choices, and most self-hosters want the first:

- **(a) Skip it.** The desktop app embeds the dashboard and opens it against
  whatever origin you configured (§5). No separate deployment is needed.
- **(b) Host the dashboard SPA yourself**, for members who want to open a
  live Project in a browser without the desktop app. Build it —

  ```bash
  pnpm --dir apps/dashboard build
  ```

  — and serve `apps/dashboard/dist` behind the same origin as your API, with
  `/v1` proxied to your Convex site URL. `api/v1/[...].js` is Overgent's
  reference proxy for Vercel; on another host, replicate the same rule with
  your reverse proxy (forward `/v1/*` to the Convex HTTP Actions origin,
  preserving method, headers, and body).

Activation — the flow that turns an invite or a create/join call into a live
dashboard session — needs the dashboard on the **same origin** as the API.
The backend sets the session cookie on the API origin and responds with a
`303` redirect to `/dashboard` on that same host; a dashboard served from a
different origin would not have the cookie and the redirect would land
nowhere useful. Choice (a) sidesteps this because the desktop app supplies
its own window; choice (b) requires the proxy rule above for the same
reason.

## 5. Pointing clients at it

- **Desktop app:** the onboarding flow's "Advanced: connect to a different
  server" field. Enter your API origin there; it is validated and stored the
  same way the built-in default is.
- **CLI:**

  ```bash
  overgent create --api https://your-origin.example.com --label "My Project"
  overgent join --api https://your-origin.example.com <invite>
  ```

**HTTPS is mandatory** for every origin except loopback (`localhost`,
`127.0.0.1`, `::1`), which may use plain HTTP. This is enforced by
`internal/hosted.New` for every client, desktop and CLI alike, because device
tokens and invite secrets travel as bearer credentials over that connection;
loopback is exempted only because traffic to it never leaves the machine.

## 6. Operating it

- **Retention.** `convex/functions/crons.ts` runs a bounded retention sweep
  every five minutes; hosted activity, findings, semantic objects,
  deliveries, and shared session messages default to 30-day retention
  (`security-privacy.md`).
- **Rate ceilings.** Every public route is guarded by a per-route call to
  `consumeEdgeRate` in `convex/functions/http.ts` (ADR-070); the ceiling is
  the numeric argument at each call site, e.g. `"projects.create", 5`. These
  are abuse ceilings shared across all unauthenticated callers on a route,
  not per-member quotas. Change them by editing the call site and
  redeploying your functions.
- **Export and deletion.** `GET /v1/projects/{id}/export` returns the
  Project's records as a JSON attachment; `DELETE /v1/projects/{id}` deletes
  a Project a caller owns. Both are implemented in `convex/functions/http.ts`
  and rate-limited under `projects.export` / the project-deletion route.
- **Backups.** On Convex Cloud, use Convex's own snapshot export. On a
  self-hosted `convex-backend` server (Option B), back up its underlying
  SQLite (or Postgres, if you configured that backend) storage using your
  usual database backup tooling.
- **Upgrading.** Deploy the Convex functions from the same release tag as
  the clients you support; `protocol.md` §1 states the compatibility rules
  (additive optional fields are compatible, removal/rename/meaning changes
  require a major version) that let you upgrade the backend slightly ahead
  of clients without breaking them.

## 7. What is not included

There are no accounts, no SSO, and no billing — membership is the existing
device-and-invite model, and there is no multi-tenant admin surface. Anyone
who can reach your API origin and holds a valid invite or device credential
can create or join a Project on it exactly as they would on
`api.overgent.com`; access control beyond that is your reverse proxy's job,
not Overgent's.
