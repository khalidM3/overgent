# Overgent release

## Release operator runbook

This section is the canonical end-to-end procedure for publishing the
owner-operated Overgent Cloud service and a downloadable Overgent release. It
separates four deployment paths that share a repository but are not one
operation:

| Change | Production trigger | Automated by the release tag? |
|---|---|---|
| Dashboard, Vercel API proxy, redirects, or `vercel.json` | Push or merge to `main`; the linked Vercel Git integration deploys the commit | No |
| Hosted Convex functions, schema, or crons | An operator explicitly runs `convex deploy` against the production deployment | No |
| CLI, desktop applications, installers, and bundled local backend | Push a `v*` Git tag; `.github/workflows/release.yml` builds an immutable draft GitHub Release | Yes |
| Public release/update channel | Run `.github/workflows/promote-release.yml` for a qualified draft version | No; promotion is a separate protected action |

The tagged application contains a deploy payload generated from that tag's
`convex/functions/` for its bundled loopback backend. That does **not** update
the hosted Convex deployment behind `api.overgent.com`. Conversely, deploying
hosted Convex does not create an application release.

Production deployment names, account identifiers, and credentials are private
operator configuration under `public-repository-boundary.md`; never substitute
their real values into this file or commit them anywhere in this repository.

### 1. Prepare one release commit

Start from the repository root on `main`, install the frozen dependencies, and
run the required gates:

```bash
git switch main
git pull --ff-only origin main
git status --short
corepack enable
pnpm install --frozen-lockfile
go test ./...
go vet ./...
pnpm protocol:generate
git diff --exit-code
pnpm protocol:check
pnpm typecheck
pnpm test
pnpm build
```

Do not continue with unexpected working-tree changes or a failed gate. The
normal pull-request/`main` CI adds race, multi-version, native desktop, CodeQL,
and platform checks; it must also be green for the commit being released.

Deploy each production surface from this same commit. Hosted contract changes
must remain compatible with clients already installed in the field; follow
`protocol.md` section 1.

### 2. Deploy hosted Convex when it changed

This is required when the release commit changes the hosted implementation in
`convex/`, including functions, schema, indexes, or crons. It is not required
for a dashboard-only or client-only change.

The development command deliberately writes an anonymous loopback target to
`convex/.env.local`. Do not replace that local-development configuration with a
production target. Instead, authenticate once and override the target for this
one command:

```bash
pnpm --dir convex exec convex login
CONVEX_DEPLOYMENT=prod:<production-deployment> \
  pnpm --dir convex exec convex deploy --dry-run --typecheck enable
```

The dry run must identify the expected Project and `[Production]` deployment,
pass schema validation, and list any index deletion. Stop and investigate an
unexpected target, destructive schema/index change, or type-check failure.

Run the production push only after reviewing that output:

```bash
CONVEX_DEPLOYMENT=prod:<production-deployment> \
  pnpm --dir convex exec convex deploy --typecheck enable
```

Use `pnpm --dir convex exec ...`, not `npx convex ...` from the repository
root: Convex is installed in the `convex/` workspace. The explicit
`CONVEX_DEPLOYMENT` override is also what makes the command work while ordinary
development remains pointed at the anonymous local backend.

After deployment, confirm the required production variable names without
printing their values:

```bash
CONVEX_DEPLOYMENT=prod:<production-deployment> \
  pnpm --dir convex exec convex env list --names-only
```

`OVERGENT_SECRETS_KEY` must remain present and stable. Do not rotate it as part
of a routine deploy: existing encrypted per-Project provider keys depend on it.
Overgent Cloud intentionally leaves operator AI-provider keys unset.

Smoke-test the public proxy. An unauthenticated protected route should reach
Convex and answer `401`, not return a Vercel `502`:

```bash
curl -i https://api.overgent.com/v1/device/bootstrap
```

### 3. Verify the automatic Vercel deployment

The owner-operated Vercel project is connected directly to this GitHub
repository. Pushes to `main` produce production deployments; other branches
produce previews. `vercel.json` supplies the dashboard build, output directory,
proxy functions, redirects, rewrites, and security headers. Neither
`.github/workflows/release.yml` nor `.github/workflows/promote-release.yml`
deploys Vercel.

After the release commit reaches `main`, verify that Vercel reports a `READY`
production deployment for that commit in the Vercel dashboard or with the
already-linked checkout:

```bash
npx --yes vercel@latest ls --limit 10
```

Confirm `api.overgent.com` serves the expected dashboard and that the API smoke
test above reaches Convex. If the Git integration is unavailable, an authorized
operator may use this manual fallback from the repository root:

```bash
npx --yes vercel@latest deploy --prod
```

The fallback is not a normal release step and must deploy the same reviewed
commit. Vercel production holds `CONVEX_SITE_URL` and
`OVERGENT_RELEASE_MANIFEST_URL`; their values stay in Vercel, not Git.

### 4. Build the draft application release

Choose a new `v`-prefixed semantic version after the commit is on `main`, then
create and push an annotated tag:

```bash
git tag -a v0.1.2 -m "v0.1.2"
git push origin v0.1.2
```

Replace `v0.1.2` with the new version. Do not manually create a release first.
The tag push triggers `.github/workflows/release.yml`, which builds, signs,
notarizes, packages, attests, and uploads the artifacts to a **draft** GitHub
Release. The protected `production-release` environment may require approval.

In GitHub, open **Actions → Release** and require the tag's run to complete.
Then open the draft under **Releases** and verify that the expected CLI,
desktop, backend, installer, manifest, checksum, SBOM, and Sigstore assets are
present. A green build creates a candidate; it does not qualify or publish it.

### 5. Qualify, then promote

Run the native clean-machine and two-person gates in this document before
promotion. Promotion is deliberately separate so a successful build cannot
publish an unqualified candidate.

The normal operator path is GitHub's interface:

1. Open **Actions → Promote Release**.
2. Choose **Run workflow**.
3. Enter the exact draft tag, such as `v0.1.2`.
4. Approve the protected `production-release` environment when required.

The equivalent GitHub CLI command is:

```bash
gh workflow run promote-release.yml -f version=v0.1.2
```

The promotion workflow publishes the draft GitHub Release and copies only its
verified signed `update-manifest.json` to the stable Vercel Blob location. It
does not rebuild artifacts, deploy Vercel, or deploy hosted Convex.

Verify that the release is public and the stable channel names the promoted
version:

```bash
gh release view v0.1.2
curl -fsSL https://releases.overgent.com/current/update-manifest.json
```

Finally exercise the public installer on a clean qualified machine:

```bash
curl -fsSL https://releases.overgent.com/install.sh | sh
overgent service status
```

### 6. Routine decision guide

- A change only under the dashboard, `api/`, or `vercel.json`: merge to `main`
  and verify the automatic Vercel production deployment. Do not deploy Convex
  or cut an app release unless that change is also intended for a new app.
- A hosted backend change under `convex/`: merge to `main`, allow Vercel to
  deploy the same commit, and explicitly deploy hosted Convex. Cut/promote an
  app release when installed clients or the bundled local backend also need the
  change.
- A Go, desktop, installer, updater, or bundled-backend change: complete any
  necessary Vercel/hosted-Convex deployment first, push the version tag, qualify
  the draft, then run Promote Release.
- Promotion changes only the public artifact/update channel. Never treat it as
  a general production deployment hook.

## Qualification status

Apple Silicon macOS 12 or newer is the only public supported platform. Its CLI,
LaunchAgent, backend, and labeled desktop beta are signed, notarized where
applicable, and covered by the clean-machine lifecycle gate.

Linux amd64/arm64 installer behavior is implementation-complete and exercised
in clean Linux containers, including real archive extraction, signed-manifest
verification, systemd-user service registration/removal, Secret Service
enforcement, update/rollback, offline transfer, and failure cases. Container
evidence is not a native desktop-distribution qualification. Linux remains a
candidate until the same lifecycle passes on owner-approved native machines
with their real login keyrings and logout/login recovery.

Windows amd64 installer behavior is implementation-complete and is
cross-checked by amd64 build/vet, strict archive/manifest handling, and the
existing scheduled-task/Credential Manager tests. It has not been executed on a
native Windows runner in this lane. Windows is not supported publicly until a
signed candidate passes the complete native lifecycle, Credential Manager,
Task Scheduler, update recovery, and uninstall checks. Windows ARM is not
published because the pinned Convex backend has no Windows ARM artifact. Intel
macOS remains unqualified.

Wails remains exact-pinned and prerelease. The root Go service has no Wails or
CGO dependency, and the hosted dashboard remains the desktop recovery path.

## Artifact selection and layout

The CLI updater manifest is discovered only at
`https://releases.overgent.com/current/update-manifest.json`. It selects one of:

| Machine | Manifest key | CLI archive |
|---|---|---|
| Apple Silicon macOS | `darwin_arm64` | `overgent_<version>_darwin_arm64.tar.gz` |
| Intel macOS (unqualified) | `darwin_amd64` | `overgent_<version>_darwin_amd64.tar.gz` |
| Linux x86-64 | `linux_amd64` | `overgent_<version>_linux_amd64.tar.gz` |
| Linux ARM64 | `linux_arm64` | `overgent_<version>_linux_arm64.tar.gz` |
| Windows x86-64 | `windows_amd64` | `overgent_<version>_windows_amd64.zip` |

The signed CLI manifest points to immutable GitHub Release assets. The
installer derives `backend-manifest.json` from that same authenticated,
immutable release directory; it never fetches Convex directly. That second
signed manifest selects `overgent_backend_<version>_<os>_<arch>.*`, containing
only the pinned `convex-local-backend` executable, its FSL license, and the
release-time `backend-push.json`. This is fetch-on-demand from an
Overgent-controlled release origin: the large backend is not inside the CLI
archive, and `--skip-backend` leaves local Project creation unavailable.

Installed runtime locations are versioned so an update never overwrites a
running backend:

- macOS: `~/Library/Application Support/Overgent/runtime/<version>`;
- Linux: `${XDG_CONFIG_HOME:-~/.config}/Overgent/runtime/<version>`;
- Windows: `%APPDATA%\Overgent\runtime\<version>`.

## Verification chain

| Platform | First-install verification | Update verification |
|---|---|---|
| macOS | HTTPS-only fetch, exact size/SHA-256, valid code signatures, exact Apple Team ID for CLI and backend | compiled Ed25519 channel key, exact size/SHA-256, process identity, service health, automatic rollback |
| Linux | HTTPS-only fetch, canonical manifest reconstruction, Ed25519 signature with the rendered release key, exact size/SHA-256 | the same compiled Ed25519 key, size/SHA-256, process identity, systemd-user health, automatic rollback |
| Windows | HTTPS-only fetch, Ed25519 manifests, exact size/SHA-256, valid Authenticode, exact signer-certificate SHA-256 for CLI and backend | compiled Ed25519 channel key, size/SHA-256, process identity, scheduled-task health, automatic rollback |

The manifest parsers bound document size, nesting/value count (POSIX), asset
count, URL scheme, checksums, and artifact size. Archives accept only their
expected root entries. Linux and Windows require OpenSSL for Ed25519
verification. Windows signing is optional only for explicit local development
copies; a public installer with no rendered signer anchor fails before
installation.

Sigstore bundles for checksums and both signed manifests are independent CI
provenance evidence. They supplement, rather than replace, verification built
into installers and the updater.

## Install, update, rollback, and uninstall

After a candidate is qualified and promoted:

```bash
curl -fsSL https://releases.overgent.com/install.sh | sh
overgent service status
overgent update
overgent update rollback
curl -fsSL https://releases.overgent.com/uninstall.sh | sh
```

For a candidate Linux release, download the rendered scripts from that draft
release and run `sh install.sh`; do not use the source template. Linux requires
a systemd user manager plus GNOME Keyring, KWallet, KeePassXC Secret Service,
or another compatible provider in the same login session. A missing/locked
Secret Service is an actionable hard failure. Overgent never writes the device
or backend secrets to a plaintext or encrypted fallback file.

On Windows, run the rendered script from PowerShell:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install.ps1
overgent service status
overgent update
overgent update rollback
.\uninstall.ps1
```

The Windows candidate installer requires PowerShell 7.5 or newer and
`openssl.exe` on `PATH`. PowerShell 7.5 preserves signed JSON timestamp strings
for canonical verification; OpenSSL verifies both Ed25519 manifests before the
installer trusts their archive checksums. These are explicit qualification
prerequisites, not bypassable public-install modes.

The per-user background process is a LaunchAgent on macOS, a systemd user unit
on Linux, and a logon-triggered Task Scheduler task on Windows. Installation is
idempotent and restarts the registered service onto the installed executable.
Uninstall calls `setup remove-all`, removes the service registration, then the
binary and rollback copy. Unknown adapter drift is preserved and reported.

State and OS credentials are preserved by default. Use
`uninstall.sh --purge-local-state` or
`uninstall.ps1 -PurgeLocalState` to stop the backend, move local state to the
platform trash, and remove only local-backend keychain records. Revoke a hosted
device or delete a Project separately from Settings.

## Prefetch, manual copy, and offline recovery

On a connected Linux/macOS machine of the same OS and architecture:

```bash
sh install.sh --download-only /absolute/path/overgent-offline
```

Copy that directory and the rendered installer to the offline machine, then:

```bash
sh install.sh \
  --manifest ./overgent-offline/update-manifest.json \
  --archive ./overgent-offline/overgent_<version>_<os>_<arch>.tar.gz \
  --backend-manifest ./overgent-offline/backend-manifest.json \
  --backend-archive ./overgent-offline/overgent_backend_<version>_<os>_<arch>.tar.gz
```

Windows uses the corresponding `-DownloadOnly`, `-Manifest`, `-Archive`,
`-BackendManifest`, and `-BackendArchive` parameters. Copied manifests remain
signature/trust inputs; copied archives are never accepted on checksum alone.
If a network fetch fails, the installer names this exact recovery rather than
leaving a partial install. `--skip-backend` / `-SkipBackend` is also available
for a deliberately team-only install.

## Release-owner setup and workflow

Protect the `production-release` GitHub environment with required reviewers.
Generate the offline Ed25519 update key with `cmd/release-keygen`, and configure
the existing Apple, Blob, and update-signing variables/secrets. Windows adds:

| Kind | Name | Meaning |
|---|---|---|
| Variable | `WINDOWS_SIGNING_REQUIRED` | `true` makes every release require Windows signing |
| Variable | `WINDOWS_SIGNER_CERT_SHA256` | lowercase SHA-256 of the DER signing certificate |
| Secret | `WINDOWS_CERTIFICATE_PFX` | base64 PKCS#12 code-signing certificate |
| Secret | `WINDOWS_CERTIFICATE_PASSWORD` | PKCS#12 password |

The manual `windows_signing` workflow input also requests signing. If either
mechanism requests it and any credential/anchor is absent or mismatched, the
workflow fails with the missing name. With signing off, it publishes an
explicitly unsigned Windows portability artifact and renders a Windows
installer that refuses public installation.

For a tag, the workflow generates and verifies the backend deploy payload,
builds Go archives with GoReleaser, fetches the pinned backend separately for
every published architecture, signs macOS and optionally Windows executables,
creates CLI/backend checksums and Ed25519 manifests, builds/notarizes/staples
the desktop, emits archive SBOMs and Sigstore bundles, and uploads everything
to the immutable draft GitHub Release. The existing protected promotion flow
publishes only a verified version's CLI manifest to the stable domain; the
backend stays addressed through the immutable directory named by that manifest.

## Qualification gates

A public support claim requires evidence from a clean native machine: install,
credential-store round trip, service survival across logout/login, forced
termination recovery, local backend create/use, authenticated update, rollback,
adapter cleanup, ordinary uninstall, purged uninstall, and the documented
failure cases. Cross-compilation, static review, or container execution alone
never becomes a native support claim.

The two-person beta gate remains unchanged: on qualified machines verify
create/join, supported agent sessions, collision and next-turn delivery,
pause/resume, revocation, export/deletion, and a voluntarily completed second
session without critical data loss.
