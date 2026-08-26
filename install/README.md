# Installing the beta

Release automation renders `install.sh` with the release update public key and
Apple Developer Team ID, then attaches that rendered copy to the GitHub release.
The checked-in source is intentionally non-installable: it has no production
trust anchors.

The macOS installer verifies the manifest's bounded asset metadata, archive
size and SHA-256, the executable's code signature, and the exact expected Apple
Team ID before installing into `~/.local/bin`. It then installs the current-user
LaunchAgent. No language runtime or package manager is required.

The beta is not advertised on Linux or Windows. GoReleaser builds those
archives to continuously catch portability regressions, but ADR-019 requires
native credential-store, IPC, service lifecycle, install/update/uninstall, and
recovery evidence before they become supported downloads.

Uninstall preserves local state and Keychain credentials by default. The
explicit `--purge-local-state` option moves state to Trash rather than deleting
it irrecoverably; hosted device revocation and Project deletion are separate
authorized operations.
