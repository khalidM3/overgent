# Closed-test distribution (legacy)

Unsupported and kept for reference only until the first public release; do not
use it to distribute builds.

This is an **unsigned** channel for handing a build to a small number of people
who already trust the person sending the link. It is not the beta, and nothing
here may be advertised as an install.

The production channel is [`../install.sh`](../install.sh): it verifies Apple
notarization and the exact expected Team ID, and the binary it installs verifies
an Ed25519 update manifest on every `overgent update`. The source copy refuses
to run precisely so a checkout can never become a distribution channel. None of
that applies here. This installer verifies one pinned SHA-256 and the bundle's
own ad-hoc signature, which proves the download was not corrupted or altered in
transit and nothing more. It does not establish who built it.

## Publishing

```sh
OVERGENT_DOGFOOD_ORIGIN=https://example.vercel.app \
OVERGENT_DOGFOOD_CONVEX=https://your-deployment.convex.site \
./install/dogfood/publish.sh --deploy
```

Needs Node 22 and a signed-in `vercel` CLI. Without `--deploy` it stages
`dist-dogfood/` and stops. The script stamps the current commit into both
binaries, so `overgent version --json` on a member's Mac reports exactly what
they installed; it warns if the tree is dirty, because a stamped commit that
does not match the artifacts is worse than no stamp.

## Why one origin

Everything is served from a single host: the installer, the app bundle, the CLI,
the dashboard SPA, and a proxy to Convex under `/api/*` and `/v1/*`. That is
load-bearing rather than tidy. Dashboard activation exchanges a one-time ticket
and answers with a 303 to `/dashboard` on whichever host authenticated it, and
the session cookie it sets is scoped to that host, so the SPA and the API have
to answer on the same origin.

The app is built with `OVERGENT_PRODUCTION_API_ORIGIN`, which bakes the origin
in at link time (see `scripts/build-desktop.mjs`).

## What members do

```sh
curl -fsSL https://example.vercel.app/install.sh | sh
```

The app installs to `/Applications`, opens, and walks them through first-run
setup: create a Project for a repository of their own, or join one with an
invite code. Invite codes are one-use and minted per person from the app.

## Cleaning up

`../uninstall.sh` removes managed agent bindings and the LaunchAgent. Revoking a
member's device and deleting a Project are separate authorized operations
against the deployment, not something the installer or uninstaller can do.
