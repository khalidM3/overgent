# Gate B — Git/worktree observation spike

Outcome recommendation: **pass** for the V1 manifest assumption, with explicit
failure classifications for rewritten/missing baselines and ambiguous repository
identity. This directory is disposable validation code, not production L0/L1.

## What this proves

- All Git execution uses `exec.CommandContext` argument arrays. No shell parses a
  ref, path, repository root, or remote name.
- The manifest is the sorted union of baseline-to-`HEAD`, unstaged, staged, and
  untracked path/status queries. Ignored paths remain absent.
- A clean worktree still reports a locally committed path after the captured
  workstream baseline; no push, fetch, source, diff, blob, or Git object upload is
  needed.
- A branch switch, detached `HEAD`, or rebase does not mutate or guess. A captured
  commit that is no longer an ancestor is labeled `diverged_non_ancestor` while
  the tree delta remains observable. A missing object/unborn `HEAD` has a separate
  classification.
- Linked worktrees share a Git common directory. A single sanitized remote is a
  useful identity input; no remote or multiple distinct remotes requires explicit
  project registration instead of guessing.
- 1,000 sorted path/status entries split into five 200-entry chunks. An incomplete
  or hash-mismatched revision never replaces the active revision.
- A deterministic watcher-hint simulation coalesces 100 rapid edits into one scan
  request and upgrades an overflow batch to a full Git rescan request. Real OS
  watcher integration remains L1 work.
- Newlines, spaces, leading dashes, shell metacharacters, and option-shaped names
  remain opaque path values. Parent traversal, absolute paths, NUL, and symlink
  escape are rejected.

## Reproduce

From this directory:

```bash
GOCACHE=/private/tmp/stickguy-gate-b-go-cache go test -race -v ./...
GOCACHE=/private/tmp/stickguy-gate-b-go-cache go vet ./...
GOCACHE=/private/tmp/stickguy-gate-b-go-cache go test -run TestManifest_ThousandCommittedPathsChunkHashAtomicityAndResources -count=1 -v
```

The tests create only isolated `testing.T.TempDir` repositories and configure a
synthetic local Git identity. They never inspect or mutate contributor Git state.

Canonical path-only outcomes are in `fixtures/canonical/`. Commands, measurements,
failure classifications, and the privacy review are in `evidence/`.

## Deliberate limits

- This spike proves the Git CLI query and manifest model, not an `fsnotify`
  implementation or long-running debounce scheduler.
- A path has one effective status in this fixture. L0 must define how simultaneous
  index/worktree states serialize before generating external types.
- Remote normalization is only a repository-identity input. L0 must create an
  opaque fingerprint bound to explicit Project registration; local common-directory
  paths must never be uploaded.
- SHA-256 object IDs are accepted structurally, but the local fixture repository
  uses Git's default SHA-1 object format.
