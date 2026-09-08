# Release installers

Release automation renders the public trust anchors into `install.sh` and
`install.ps1`. The checked-in templates refuse ordinary network installation:
only rendered copies attached to a signed release are distribution artifacts.

The CLI and local backend are separate downloads. A normal install fetches the
small CLI archive first, then fetches the platform backend archive from the
same immutable release directory and records its executable and deploy bundle
with `overgent backend install`. `--skip-backend` / `-SkipBackend` installs a
team-Project-only CLI. `--download-only` / `-DownloadOnly` verifies and copies
everything needed for an offline install; the destination machine supplies the
copied manifests and archives by path.

Linux verifies the Ed25519 signature over each manifest with OpenSSL, then the
declared size and SHA-256 of each archive. It also requires a reachable Secret
Service implementation before writing anything. There is no plaintext or
encrypted-file credential fallback. macOS verifies size/SHA-256 plus the code
signature and exact Apple Team ID of both executables. Windows uses OpenSSL to
verify each Ed25519 manifest, then verifies size/SHA-256 plus Authenticode and
the exact release-certificate SHA-256 of both executables.

Windows signing may be omitted for development artifacts. The rendered public
installer then refuses them. The explicit
`-AllowUnsignedDevelopmentBuild` escape hatch accepts only local copied files;
it cannot weaken a network install. Setting `windows_signing` or
`WINDOWS_SIGNING_REQUIRED=true` makes the release workflow fail clearly unless
the certificate, password, and certificate anchor are configured.
Windows candidates require PowerShell 7.5 or newer plus OpenSSL on `PATH` for
strict JSON handling and Ed25519 manifest verification.

Uninstall removes only recognized managed agent bindings, unregisters the
per-user service, and removes the installed executable. State and credentials
remain by default. The explicit purge option moves state to Trash/Recycle Bin
and removes only local-backend credential records; hosted device revocation and
Project deletion remain separate authorized operations.

See `docs/release.md` for qualification status, artifact names, lifecycle
commands, offline transfer, trust properties, and release-owner setup.
