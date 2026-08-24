# L-1 final integration report

Date: 2026-08-23

## Outcome

L-1 passes its go/no-go rule. Gate A narrows optional Codex hook fidelity and
uses the existing official-MCP plus Git/manual fallback. Gates B, C, D, and E
pass within their recorded scopes; Gate D withholds non-macOS runtime claims.
No result replaces or leaves an unresolved threat to Go, the versioned
Stickguy protocol, the baseline/current manifest model, Project isolation, or
the coordination-harness lifecycle.

Accepted outcomes are ADR-016 through ADR-020 in `docs/decisions.md`. Production
L0 has not started.

## Delivered evidence

- Bootstrap: ordered document review, baseline Git commit, Go 1.26 toolchain,
  and shared fixture freeze in `validation/bootstrap-evidence.md`.
- Gate A: official MCP Go SDK/Codex lifecycle smoke, redacted JSONL metadata,
  reviewed project config/hook shapes, privacy review, and hook narrowing.
- Gate B: real temporary-Git fixtures, path-only canonical outputs, manifest
  hashing/chunk activation, failure classifications, and resource observations.
- Gate C: pinned local Convex project, two-client live suite, vector isolation,
  race fallback, retention behavior, limits/cost note, and redacted timings.
- Gate D: single executable/service/IPC/SQLite/Keychain/LaunchAgent proof,
  cross-build matrix, cleanup confirmation, and platform narrowing.
- Gate E: versioned synthetic corpus, executable candidate/routing evaluation,
  expected labels, isolation checks, and retained false-positive evidence.

## Security and privacy

All repositories, paths, summaries, vectors, identities, service records, and
credentials used by the spikes were synthetic or uniquely disposable. Evidence
contains no secret values, source/diff content, Git objects, raw transcripts,
system prompts, environment values, or raw command/test output. Local servers
used Unix sockets or loopback only. The disposable Keychain, LaunchAgent,
Convex backend, generated deployment state, and dependency/build artifacts were
stopped or removed after verification.

## Verification

The integrator reran the following from the committed spike sources:

- Gate A: `go test -race ./...`, `go vet ./...`, and `go mod verify`;
- Gate B: `go test -race ./...` and `go vet ./...`;
- Gate C: frozen pnpm install, strict typecheck, and the full live suite against
  a restarted anonymous loopback deployment, followed by shutdown and cleanup;
- Gate D: `go test -race ./...`, `go vet ./...`, `go mod verify`, and CGO-free
  builds for macOS arm64/amd64, Linux arm64/amd64, and Windows amd64; and
- Gate E: `go test -race ./...` and `go vet ./...`.

All passed. Every tracked JSON fixture parses, `git diff --check` is clean, the
repository has no root production `go.mod`, `package.json`, or `cmd/stickguy`,
and the final worktree is clean after this integration record is committed.

## Required carry-forwards

- L0/L5: accept only the exact proven Codex MCP build range until revalidated;
  keep hooks disabled until trusted-worktree and desktop/CLI delivery plus
  structured setup/status/removal tests pass.
- L0/L1: define simultaneous staged/unstaged status encoding; add real watcher,
  platform-path, and Git SHA-256 fixtures.
- L1/L8: validate Linux and Windows runtime-specific service, IPC, and credential
  adapters on native runners before advertising support.
- L2/L6: derive scope from authenticated server state, index/batch retention,
  measure hosted load/cost, evaluate real provider dimensions, and keep semantic
  alerts quiet until precision thresholds pass.
