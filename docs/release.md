# Overgent release

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
