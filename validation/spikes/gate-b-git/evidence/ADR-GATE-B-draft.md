# Draft validation ADR — accept Git baseline/current manifest model

Status: proposed Gate B outcome for integrator/owner acceptance
Date: 2026-08-23

## Decision

Accept the existing V1 Git observation boundary. Capture a full commit object ID at
`begin_work`; publish a revisioned snapshot that unions baseline-to-current `HEAD`
path changes with staged, unstaged, and untracked path state. Git filesystem events
remain hints, and overflow triggers a full rescan. Chunked revisions become visible
only after count and content-hash verification.

Repository identity combines an opaque normalization of the selected remote with
explicit Project registration. No-remote and multiple-distinct-remote repositories
require explicit registration/selection. Linked worktrees are associated locally by
their Git common directory, which is never uploaded.

## Evidence

The executable real-Git fixtures demonstrate all Gate B states, including a clean
worktree after a local commit, rewritten history after rebase, detached `HEAD`, a
linked worktree, malicious names, symlink escape, and a 1,000-path atomic revision.
See `results.md` and `fixtures/canonical/`.

## Consequences

- Locally committed work remains visible before push without fetching peer state or
  uploading source/diffs/Git objects.
- Non-ancestor/missing baselines are explicit fidelity states, not silent empty
  manifests and never trigger automatic checkout/reset/rebase.
- L0 must settle the external encoding for simultaneous staged and unstaged status.
- L1 must implement real watcher debounce/overflow plumbing and cross-platform
  integration tests; the spike only proves the deterministic scheduling policy.
- Git SHA-256 repositories need a fixture during L1 even though the validator accepts
  both 40- and 64-hex full object IDs.

## Outcome

Recommended Gate B classification: **pass**. The remaining items are contained
implementation details and do not replace Go, the protocol boundary, the manifest
model, Project isolation, or the coordination-harness lifecycle.
