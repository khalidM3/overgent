# Gate B observed results

Run date: 2026-08-23
Primary OS: macOS arm64
Go: 1.26.7
Git: 2.50.1 (Apple Git-155)

## Verification

`go test -race -v ./...` passed all ten tests in 4.020 seconds. Covered results:

- unstaged, staged, untracked, renamed, deleted, and ignored paths;
- clean-worktree local commit after baseline;
- branch divergence and detached `HEAD`;
- an actual rebase that rewrites the captured commit, classified as
  `diverged_non_ancestor`;
- no remote, sanitized single remote, distinct multiple remotes, and a linked
  worktree sharing the primary common directory;
- 1,000 locally committed paths, five chunks, stable hash, incomplete activation
  rejection, then atomic activation;
- malicious path preservation, option-like baseline rejection, parent traversal,
  and symlink escape rejection; and
- rapid-edit coalescing plus overflow/full-rescan simulation.

`go vet ./...` passed with no findings.

## Latency and allocation evidence

The non-race 1,000-path fixture reported:

```text
manifest_paths=1000
chunks=5
content_hash=8411f58a580dccac6d9ac19c4084690f6b8d32b553e21385262667f7ed10a102
observe_elapsed=175.093583ms
total_alloc_delta_bytes=8715648
```

The race-enabled full suite's same observation reported 247.211125 ms and
8,852,784 total allocated bytes. These are bounded primary-machine observations,
not performance promises. `/usr/bin/time -l` could not read `kern.clockrate` in
the managed sandbox, so no trustworthy process peak-RSS figure is claimed.

## Failure classifications

| Condition | Classification / behavior |
|---|---|
| Captured baseline is ancestor of `HEAD` | `ancestor`; publish full current snapshot. |
| Branch switch/rebase makes baseline non-ancestor | `diverged_non_ancestor`; publish explicitly degraded tree delta and request a new explicit baseline at a lifecycle boundary. |
| Baseline object is absent | `missing`; do not claim baseline fidelity. |
| Repository has unborn `HEAD` | `unborn_head`; worktree-only observation needs a separately defined L1 path. |
| No remote | Require explicit Project/repository registration; never use folder name as identity. |
| Multiple distinct normalized remotes | Require explicit selection/registration; never guess `origin`. |
| Partial or hash-mismatched manifest revision | Keep prior active revision; reject activation. |
| Watcher overflow | Schedule one authoritative full Git rescan. |
| Git unavailable/cancelled/query error | Return operation-context error; caller degrades workspace while service remains healthy. |
| Path traversal/symlink escape/invalid baseline | Reject before publication/unsafe Git use. |

## Canonical output boundary

The preserved JSON fixtures contain only baseline/head classifications and
path/status/old-path coordination metadata. Commit hashes are used locally for
baseline mechanics but are intentionally absent from canonical hosted fixture
outputs. No content, patch, blob, diff, prompt, transcript, environment value,
credential, or raw command output is collected.
