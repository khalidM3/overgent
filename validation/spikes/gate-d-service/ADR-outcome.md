# Draft ADR outcome — Gate D local service and distribution

Status: proposed Gate D result for integrator/owner acceptance
Date: 2026-08-23

## Decision

Accept the existing Go single-executable, one-per-user-service, pure-Go SQLite, OS-credential-store, and platform user-service boundaries for production planning. No replacement ADR is needed.

Gate D passes on the primary development platform, macOS arm64. Preserve the following capability narrowing:

1. macOS is the only runtime-proven platform in this spike.
2. Linux amd64/arm64 and Windows amd64 are build-proven but not runtime-supported by this spike. Their production service, credential, and Windows IPC adapters stay unavailable until native-runner gates pass.
3. Never introduce a plaintext credential fallback to make an unsupported platform appear supported.
4. Use a reviewed production keyring/native adapter that does not place credential values in process arguments.
5. Keep user-service adapters build-tagged and narrow; do not let platform lifecycle APIs leak into service/domain code.

## Evidence behind the decision

- macOS arm64 ran CLI, service, and stdio modes from one binary.
- OS locking rejected a second service before SQLite mutation; IPC health remained available from the owning process.
- SQLite restart recovery advanced the same persisted counter from 1 to 2.
- A disposable Keychain item completed put/read/delete, and a later lookup returned not found.
- A disposable LaunchAgent completed install/start/status/IPC/stop/absent/remove.
- `CGO_ENABLED=0` cross-builds completed for macOS arm64/amd64, Linux amd64/arm64, and Windows amd64.

## Consequences

- L0 contracts/scaffold may retain Go 1.26 and the one-service architecture once every L-1 gate is resolved by the integrator.
- This spike does not authorize production L0 or advertising untested Linux/Windows runtime support.
- Signing, notarization, package installation, update metadata/rollback, login recovery, Windows named pipes/Credential Manager, and Linux Secret Service/systemd-user behavior remain later gated work.
