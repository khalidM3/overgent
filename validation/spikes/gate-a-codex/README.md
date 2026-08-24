# Gate A — Codex coordination adapter spike

Status: **NARROW** on Codex `0.148.0-alpha.15` (macOS arm64), 2026-08-23.

This disposable spike validates a project-scoped Go stdio MCP surface using
the official `github.com/modelcontextprotocol/go-sdk` v1.7.0 and the
allowlisted shape of bounded `SessionStart`/`SubagentStart` hooks. It is not
production L0 code and owns no watcher, queue, local service, hosted connection,
or agent loop.

## Reproduce

Use isolated Go caches so contributor state is untouched:

```sh
GOCACHE=/private/tmp/stickguy-gate-a-gocache \
GOMODCACHE=/private/tmp/stickguy-gate-a-gomod \
go test ./...

GOCACHE=/private/tmp/stickguy-gate-a-gocache \
GOMODCACHE=/private/tmp/stickguy-gate-a-gomod \
go vet ./...

GOCACHE=/private/tmp/stickguy-gate-a-gocache-race \
GOMODCACHE=/private/tmp/stickguy-gate-a-gomod \
go test -race ./...
```

Build disposable binaries at the exact paths referenced by project config:

```sh
mkdir -p /private/tmp/stickguy-gate-a/bin
GOCACHE=/private/tmp/stickguy-gate-a-gocache go build \
  -o /private/tmp/stickguy-gate-a/bin/gate-a-sdk-mcp ./cmd/sdkserver
GOCACHE=/private/tmp/stickguy-gate-a-gocache go build \
  -o /private/tmp/stickguy-gate-a/bin/gate-a-hook ./cmd/hook
codex mcp list
```

The real-client smoke invocation used `codex exec --ephemeral --json` only in
the harness. `jq` discarded every prompt, agent message, tool argument, tool
result, and path in-stream. Only the event/tool metadata in
`evidence/codex-exec.redacted.jsonl` was retained.

## Result boundary

- PASS: trusted project `.codex/config.toml` discovery in the CLI; official SDK
  v1.7.0 stdio MCP
  initialization and instructions; the four fixture lifecycle calls; explicit
  cwd resolution/ambiguity failure; idempotency; no child service ownership.
- PASS by official contract: the ChatGPT desktop app, Codex CLI, and IDE share
  MCP configuration on the same Codex host. The desktop GUI was not manipulated
  in this lane.
- PASS in direct fixture execution: bounded `SessionStart` and `SubagentStart`
  JSON input/output shapes, including ignored `transcript_path`.
- NARROW: an end-to-end project-hook injection could not be proven. A disposable
  Git project under `/private/tmp` successfully loaded MCP configuration when a
  one-invocation inline-table trust override was supplied, but neither
  `hooks.json` nor equivalent inline hooks produced a `SessionStart` marker in
  ephemeral `codex exec`. A synthetic subagent attempt produced a redacted Codex
  runtime diagnostic: the ephemeral parent had no stored rollout from which to
  resolve the hook's parent transcript path. Persisting a non-ephemeral session
  would violate this gate's no-transcript evidence boundary, so testing stopped.
  L5 must repeat in a trusted isolated worktree with an approved privacy-safe
  client mode before enabling either hook.
- NARROW: setup/status/removal merging with pre-existing project hook config is
  not implemented by this disposable server. The reviewed install/remove diff
  is exact, but production setup must use structured merge and ownership markers
  with conformance fixtures.
- NARROW: service absence, hook timeout, stale brief, and oversized-output
  behavior have bounded visible fixture forms but were not all exercised by a
  real Codex client. MCP-only coordination remains the selected fallback.

No production L0 code was started.
