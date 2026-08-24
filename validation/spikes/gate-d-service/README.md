# Gate D — installation and local-service feasibility

Status: **PASS on the primary development OS (macOS arm64), with explicit platform narrowing**. This is a disposable architecture spike, not production L0/L1 code.

## What this proves

- One Go executable exposes separate CLI (`cli ping`), long-running service (`service run` and lifecycle operations), and line-oriented stdio MCP-fixture (`mcp`) modes.
- The service acquires a non-blocking OS file lock before opening or mutating its database. A second instance exits non-zero without incrementing persistent boot state.
- The macOS service listens on a Unix-domain socket inside a `0700` state directory and changes the socket to `0600`. CLI and MCP clients perform a bounded health request rather than trusting a PID file.
- `modernc.org/sqlite` persists a boot record with `database/sql`; closing and reopening the database recovers and advances it. Cross-builds set `CGO_ENABLED=0`.
- macOS credentials use Keychain through `/usr/bin/security`; all failures are errors and there is no config-file/plaintext fallback. The probe printed only secret length and deleted its uniquely named disposable record.
- A disposable macOS LaunchAgent installed, started, reported `running`, served IPC, stopped, became absent, and removed its plist. Its executable and state root were explicit absolute paths under this spike and `/private/tmp`; no language runtime was involved.
- The executable cross-compiles for all planned initial release triples.

## Deliberate limits

- The stdio mode is a minimal JSON-lines fixture, not the MCP SDK or the production MCP contract.
- macOS Keychain access shells out with an argument array. The disposable value can briefly be visible in the local process argument list; production must use the selected reviewed native/keyring adapter and keep secret values out of argv/logs.
- macOS LaunchAgent behavior is proven only for an interactive `gui/<uid>` domain. Login/logout recovery, installer placement, signing/notarization, updates, and rollback remain distribution work.
- Linux amd64/arm64 compile, but this spike returns an honest unsupported error for credential storage and user-service lifecycle. Production needs Secret Service/keyring and systemd-user (plus documented headless behavior) validation before those targets are advertised.
- Windows amd64 compiles, but named-pipe instance/IPC, Credential Manager, and user-service/task lifecycle are honest unsupported errors in this spike. They require a Windows runner before release support is advertised.
- Cross-compilation proves build portability, not runtime correctness on non-macOS systems.
- The socket directory permissions are the primary peer boundary. Production should additionally verify peer identity where the platform exposes it and harden path/symlink handling.

## Reproduction

Use an isolated cache because contributor state must not be touched:

```bash
env GOCACHE=/private/tmp/stickguy-gate-d-gocache \
  GOMODCACHE=/private/tmp/stickguy-gate-d-gomodcache go test ./...
env GOCACHE=/private/tmp/stickguy-gate-d-gocache \
  GOMODCACHE=/private/tmp/stickguy-gate-d-gomodcache go vet ./...
env GOCACHE=/private/tmp/stickguy-gate-d-gocache \
  GOMODCACHE=/private/tmp/stickguy-gate-d-gomodcache \
  CGO_ENABLED=0 go build -trimpath -o bin/gate-d-service .
```

For a disposable foreground run:

```bash
env STICKGUY_SPIKE_STATE_DIR=/private/tmp/stickguy-gate-d-runtime \
  ./bin/gate-d-service service run
env STICKGUY_SPIKE_STATE_DIR=/private/tmp/stickguy-gate-d-runtime \
  ./bin/gate-d-service cli ping
printf '%s\n' '{"id":7,"method":"ping"}' | \
  env STICKGUY_SPIKE_STATE_DIR=/private/tmp/stickguy-gate-d-runtime \
  ./bin/gate-d-service mcp
```

The macOS credential and LaunchAgent integration commands intentionally mutate OS state briefly. Run only with unique disposable account/state names and always execute delete/stop/remove cleanup. Exact commands and redacted outputs are in `evidence.md`.

## Security/privacy review

- No source, diffs, Git objects, transcripts, prompts, environment values, or command output are persisted/uploaded by the spike.
- No network listener is created. Local IPC is Unix-domain only on the primary OS.
- Lock and socket files are mode `0600`; their parent is mode `0700`.
- SQLite contains only the synthetic boot counter.
- Keychain failure never writes a fallback secret. Unsupported OS implementations fail closed.
- Service management uses fixed executable/label values and `exec.CommandContext` argument arrays; it does not concatenate shell input.
- The only real OS records were a uniquely labeled disposable LaunchAgent and Keychain item; both were removed, with post-removal absence verified.

See `ADR-outcome.md` for the decision and `evidence.md` for observations.
