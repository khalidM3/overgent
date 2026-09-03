# Overgent beta release

## Supported boundary

The invited beta supports Apple Silicon Macs running macOS 12 or newer. The
standalone CLI/local service and the labeled desktop beta are signed,
notarized, and stapled by the release workflow. Linux, Windows, and Intel macOS
archives are portability artifacts only; do not offer them to testers.

Wails remains an exact-pinned desktop dependency and is still prerelease. Its
desktop API is in public beta, so the hosted dashboard remains the recovery
path. Overgent's root Go service does not import Wails or CGO.

## One-time owner setup

Protect the `production-release` GitHub environment with required reviewers.
Generate the update key on a trusted offline Mac and store the private file
outside the repository:

```bash
go run ./cmd/release-keygen -private-file /absolute/secure/path/overgent-update-private.txt
```

The command prints only the base64 public key. Add these environment variables:

| Kind | Name | Value |
|---|---|---|
| Variable | `OVERGENT_UPDATE_PUBLIC_KEY` | command stdout; base64 raw 32-byte Ed25519 public key |
| Variable | `APPLE_TEAM_ID` | Apple Developer team identifier |
| Variable | `APPLE_DEVELOPER_NAME` | name in the Developer ID Application certificate |
| Secret | `OVERGENT_UPDATE_SIGNING_PRIVATE_KEY` | contents of the mode-0600 private key file; base64 raw 64-byte key |
| Secret | `MACOS_CERTIFICATE_P12` | Developer ID Application certificate exported as base64 PKCS#12 |
| Secret | `MACOS_CERTIFICATE_PASSWORD` | PKCS#12 password |
| Secret | `APPLE_ID` | notarization Apple ID |
| Secret | `APPLE_APP_PASSWORD` | app-specific password for notarization |

Enable GitHub private vulnerability reporting and replace the temporary policy
in `SECURITY.md` with a monitored private address before inviting people who do
not already have a private support channel.

## Candidate publication

1. Deploy the reviewed Convex schema/functions and dashboard for
   `https://api.overgent.com`; keep private operations outside this repository.
2. Run every standard check plus `pnpm eval:coordination` on Node 22 or newer.
3. Tag an immutable candidate such as `v0.1.0-beta.1`. The release workflow
   produces a draft GitHub release, signed CLI archives, checksums, archive
   SBOMs, Sigstore bundles, GitHub build provenance, the signed update manifest,
   rendered installer/uninstaller, and the signed/notarized desktop zip.
4. Inspect the workflow's notarization and stapler output. Download the draft
   artifacts on a clean Apple Silicon Mac; never validate from the build tree.
5. Record the commands and results in `validation/evidence/` before publishing
   the draft.

The workflow reads signing material only from the protected environment. The
update private key is written to the ephemeral runner and is never placed in an
argument, artifact, log field, or repository file.

## Clean-machine lifecycle gate

Use `install.sh` attached to the draft release. The repository template refuses
to run because it has no production trust anchors.

The desktop zip may be installed in `/Applications` or `~/Applications`. On
first launch it copies its signed bundled CLI into `~/.local/bin` when no
installed CLI exists, so the LaunchAgent and updater always use a replaceable
binary outside the signed app bundle. Running the release installer first is
still the clean-machine validation path.

```bash
sh install.sh
overgent service status
overgent diagnostics
overgent update
overgent update rollback
sh uninstall.sh
sh uninstall.sh --purge-local-state
```

Verify service recovery after logout/login and after terminating the service;
the LaunchAgent uses `RunAtLoad` and `KeepAlive`. An update must pass Ed25519
metadata verification, exact size and SHA-256 verification, start as a process,
and return a valid artifact identity. If an installed service does not return
healthy after restart, Overgent restores the `.previous` executable and starts
the prior service. `update rollback` performs the same process and health gate.

Uninstall calls `setup remove-all`, which removes only recognized managed Codex
and Claude entries and refuses unknown drift. It unregisters the LaunchAgent
and removes the binary. State and Keychain credentials are preserved by
default; `--purge-local-state` moves local state to Trash. Revoke the device or
delete the Project from Settings when hosted access/data should also be
removed.

## Tester gate

Invite one real teammate with the one-use ten-minute code. On two Macs, verify
create/join, new Codex or Claude sessions, a collision finding, next-turn
context, pause/resume, invite revocation, device revocation, personal export,
member deletion, Project export, and Project deletion. L8 is not complete until
the team voluntarily starts and completes a second session without repository
owner intervention and no critical data loss is observed.
