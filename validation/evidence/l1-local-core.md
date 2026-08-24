# L1 local Go core evidence

Branch: `lane/l1-core`
Primary runtime: macOS arm64, Go 1.26.7
Scope: Go local core only; no protocol, generated contract, Convex, dashboard, or legal-file edits.

## Delivered exit behavior

- `stickguy` routes version, workspace registration, service run/status, pause,
  resume, forced scan, and doctor commands. Production defaults to the macOS
  per-user config directory; tests always pass isolated roots. Registration
  validates and persists Project/member/device/workspace/session/workstream IDs.
- The service acquires a nonblocking OS lock before config/SQLite mutation,
  then serves health/state commands on a mode `0600` Unix socket. A second
  instance exits; a stopped instance leaves only a harmless lock inode whose
  released flock is reacquired on restart.
- Workspace registration uses the same lock, so config cannot be rewritten
  concurrently with the running service; registration while running fails
  explicitly and can be retried after stopping the service.
- SQLite migrations cover projects, workspaces/workstreams and baselines,
  active manifests, durable event queue, acknowledgement cursors, and service
  boot state. Active-manifest replacement, revision/sequence advance, and one
  started/bounded-chunk/completed publication events commit atomically, with
  completion ordered last. Every durable queue payload is a complete schema-v1
  EventEnvelope ready for `/v1/events/batch`, including stable scoped IDs,
  timestamps, source/type, and per-workspace sequence. Flush batches never mix
  workspaces and remain within the 100-event HTTP contract bound; larger
  workspace queues continue in sequence across batches.
- First workspace insertion atomically queues one `workspace.registered`
  envelope before any manifest event. Its label is the stable workspace ID, not
  a local path/basename; a persistent marker prevents duplication after restart
  or acknowledged-event cleanup.
- Git is executed through `exec.CommandContext` argument arrays with global and
  system config disabled. Observation unions baseline-to-HEAD, worktree, index,
  rename/delete, and untracked name-only evidence while preserving independent
  `baseline`, `index`, and `worktree` change states for the same path; ignored
  files remain absent.
  Paths are normalized, UTF-8/512-character bounded, and symlink escape is
  rejected. Persisted, domain-separated repository fingerprints bind exactly
  one distinct sanitized remote to explicit Project registration, remain stable
  across local paths and linked worktrees, and never hash the local common-dir.
  Zero/multiple distinct remotes fail registration with an explicit outcome;
  the common-dir remains local-only association metadata.
- Filesystem events are hints. Recursive fsnotify watches debounce rapid events;
  new directories are added; watcher errors request a full Git-authoritative
  rescan; IPC can force a rescan.
- Pause is committed synchronously before the IPC response. The sender boundary
  filters paused workspace events; tests prove another workspace continues.
- A locally committed 1,000-path fixture produces one active revision and 13
  atomically queued events: registration, start, 10 chunks of at most 100 paths,
  then completion. Reopening SQLite preserves all 1,000 entries, baseline,
  revision, publication queue, and IDs.
- Acknowledgements advance a monotonic cursor and acknowledged queue rows have
  an explicit cleanup operation with a recovery cutoff.

## Verification

```text
go test ./...                         PASS
go vet ./...                          PASS
go test -race ./...                   PASS
go mod tidy                           PASS (go.mod/go.sum updated intentionally)
```

The full suite uses temporary Git repositories and config roots. Unix-socket
tests required execution outside the Codex filesystem sandbox; sockets remained
inside isolated `/private/tmp/sg-l1-*` roots and were removed on shutdown.
The SHA-256 fixture ran (not skipped) on `git version 2.50.1 (Apple Git-155)` and
asserted 64-character baseline and current `HEAD` object IDs.

Specific assertions include two-repository observation, second-instance
  rejection, state-root/file/socket permissions, pause/no-send, unaffected-peer send, stale-lock
restart, boot/state recovery, 1,000 committed paths, single atomic queue event,
  13-event registration-first atomic 1,000-path publication, queue/cursor restart,
  acknowledgement cleanup, rapid-event debounce, explicit
rescan, ignored paths, rename/delete/staged/unstaged/untracked state, SHA-256
  hash/fingerprint, no/single/multiple-remote and linked-worktree identity,
  an actual Git SHA-256 object-format repository with 64-character baseline and
  head IDs, generated-contract EventBatch decoding, zero-path manifest clearing,
  external ID/root validation, and symlink escape.
- Go manifest hashing matches the hosted canonical layered encoding and rejects
  unordered/duplicate paths. Cross-language vector
  `a.ts`/`z.ts` =>
  `cb3fc754d48edb8d7be868df86d249942d8832811e0af83fb2f24f022328ea4d`.

## Idle resource observation

One built service with zero registered workspaces was observed after 20 seconds:

```text
PID   RSS KiB  CPU %  elapsed
8962  13264    0.0    00:20
```

Health reported `bootCount=1`, `pending=0`, `scans=0`, `workspaces=0`. PID is
ephemeral fixture metadata only.

## Security and privacy

- No source bytes, diffs, Git objects, transcripts, prompts, environment values,
  command output, or credentials enter a manifest or queue payload.
- Git stderr is returned only as local operation context and is never queued.
- State/config/socket/lock roots use current-user permissions. There is no
  loopback HTTP listener and no credential plaintext fallback.
- No production network sender was added. A generic unauthenticated HTTPS
  sender was rejected during review; the production service has no egress at
  L1. Tests inject an in-memory sender to prove queue/pause semantics. The
  authenticated generated `/v1` transport belongs to later integration.
- Non-macOS service/config/IPC implementations fail closed per ADR-019. Cross
  compilation is not treated as runtime support.

## Honest limits

- The observer matches the contract owner's independent change-state decision,
  including automated simultaneous baseline/worktree and index/worktree cases.
  This lane did not modify the shared schema or generated contracts.
- Watcher overflow is represented by the fsnotify error channel plus explicit
  full rescan; inducing a real kernel queue overflow deterministically was not
  attempted. Debounce and full-rescan behavior are automated.
- The local sender is an injected boundary only. Offline queue durability is
  complete; authenticated hosted delivery/retry policy is not claimed here.
- macOS LaunchAgent installation and Keychain enrollment were validated in L-1
  ADR-019 but are not duplicated in L1 command routing; enrollment/distribution
  remain later product levels.
- Dynamic workspace registration is intentionally not hot-reloaded in L1;
  registration is a stopped-service operation protected by the service lock.
- Registration currently requires exactly one distinct normalized remote. A
  repository with no remote or multiple distinct remotes is rejected rather
  than deriving a hosted identity from a local filesystem path; a future CLI
  may add explicit remote selection without changing fingerprint semantics.
