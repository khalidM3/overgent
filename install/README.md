# Installing the beta

Release automation renders `install.sh` with the release update public key and
Apple Developer Team ID, then publishes that rendered copy with the other public
artifacts at the immutable Vercel Blob path for the release.
The checked-in source is intentionally non-installable: it has no production
trust anchors.

The macOS installer parses bounded manifest asset metadata, verifies archive
size and SHA-256, and then verifies the executable's code signature and exact
expected Apple Team ID before installing into `~/.local/bin`. The installed
binary independently verifies the manifest's Ed25519 signature on every
`overgent update`. It then installs the current-user LaunchAgent. No language
runtime or package manager is required.

The beta is not advertised on Linux or Windows. GoReleaser builds those
archives to continuously catch portability regressions, but ADR-019 requires
native credential-store, IPC, service lifecycle, install/update/uninstall, and
recovery evidence before they become supported downloads.

Uninstall first removes recognized managed Codex/Claude bindings through
`overgent setup remove-all`, then unregisters the LaunchAgent. Unknown binding
drift is left untouched and reported. Uninstall preserves local state and
Keychain credentials by default. The
explicit `--purge-local-state` option moves state to Trash rather than deleting
it irrecoverably; hosted device revocation and Project deletion are separate
authorized operations.

A legacy, unsupported, unsigned channel kept for reference lives in
[`legacy-dogfood/`](legacy-dogfood/README.md). It shares none of the trust
properties described above and must never be advertised as an install.
