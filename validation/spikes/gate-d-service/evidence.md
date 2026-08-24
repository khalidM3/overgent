# Gate D evidence

Captured on 2026-08-23, primary OS `darwin/arm64`. Values below contain no credential material.

## Toolchain

```text
go version go1.26.7 darwin/arm64
git version 2.50.1 (Apple Git-155)
go env: GOOS=darwin GOARCH=arm64 GOVERSION=go1.26.7
```

The module pins `modernc.org/sqlite v1.46.1`. Dependency download was the only network requirement and required explicit approval. Module/build caches were isolated under `/private/tmp/stickguy-gate-d-*`.

## Automated checks

Commands:

```text
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o bin/gate-d-service .
```

Results: test, vet, and build exited 0. `internal/store.TestRestartRecovery` opened one database twice and observed boot counts 1 then 2.

## Single service, CLI, MCP, and recovery

An isolated service started with `STICKGUY_SPIKE_STATE_DIR=/private/tmp/stickguy-gate-d-runtime-final-20260823`:

```json
{"level":"INFO","msg":"service ready","boot_count":1}
{"status":"ok","pid":76928,"bootCount":1}
{"id":7,"result":{"status":"ok","pid":76928,"bootCount":1}}
```

A concurrent `service run` exited 1 with:

```text
healthy service instance already running (pid redacted): another service instance owns lock: resource temporarily unavailable
```

The owning service still reported `bootCount:1`, proving rejected startup did not mutate the database. After SIGINT and restart, the service reported `boot_count:2`. The lock is OS-owned and released on process exit; no PID-file decision is used.

Observed filesystem modes during the run were designed as state directory `0700`, lock `0600`, socket `0600`, and SQLite files within the protected directory.

## macOS credential-store probe

Account name: `gate-d-20260823-76928`
Keychain service: `dev.stickguy.validation.gate-d`

Sequence: `credential put`, `credential get`, `credential delete`, then a second `credential get`.

```text
credential-present bytes=33
post-delete lookup: exit status 44 (not found)
```

The credential value was disposable, was not printed into this evidence, and the record was deleted. There is no filesystem fallback in the code.

## macOS user-service lifecycle

State/plist root: `/private/tmp/stickguy-gate-d-launchd-20260823`
Label: `dev.stickguy.validation.gate-d`

```text
install -> plist path returned
plutil -lint -> OK
start -> started
status -> state = running
cli ping -> {"status":"ok","pid":78458,"bootCount":1}
stop -> stopped
status after stop -> exit status 113 (not loaded)
remove -> removed
```

The plist path was absent after remove, and `launchctl print` had already established that the label was unloaded. The plist referenced the single compiled executable and required no Go/Node/Python runtime.

## Cross-compilation and artifacts

All commands used `CGO_ENABLED=0` and `-trimpath`.

| Target | Build | Approximate size | Runtime-specific result |
|---|---:|---:|---|
| macOS arm64 | pass | 10 MiB | CLI/service/IPC/SQLite/Keychain/LaunchAgent exercised |
| macOS amd64 | pass | 11 MiB | cross-build only; same build-tagged adapter |
| Linux amd64 | pass | 10 MiB | static ELF; service/credential adapter deliberately unsupported |
| Linux arm64 | pass | 9.7 MiB | static ELF; service/credential adapter deliberately unsupported |
| Windows amd64 | pass | 9.8 MiB | PE32+; named pipe, lock, service, credential adapters deliberately unsupported |

SHA-256 checks were captured for generated temporary binaries to demonstrate distinct complete artifacts; they are not retained because release signing/checksums are outside this gate.

## Resource observation

After 15 seconds idle on macOS arm64:

```text
RSS: 13,520 KiB
CPU: 0.0%
```

This is a skeletal process with SQLite open and a Unix listener. It is evidence for feasibility, not a production budget or soak result.

## Failure classification

- Sandbox initially denied Unix-socket bind. The exact isolated binary run was approved outside the sandbox; this is a harness restriction, not an application failure.
- The first live iteration revealed that database boot state was incremented before lock acquisition. The code was corrected so lock ownership precedes database open/mutation, and the final evidence above confirms the fix.
- Non-macOS platform lifecycle/credential behavior is intentionally `unsupported`, not silently emulated or called supported because compilation succeeded.
