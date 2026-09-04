# Lane 02 — Public readiness: make the repository safe and coherent to publish

Status: brief
Last updated: 2026-09-04
Executor: Haiku 4.5 or Sonnet 5. Mechanical, but every step has a verification.
Depends on: ADR-071 accepted. Blocks: flipping visibility (Lane 07).
Owner inputs needed before starting: the public GitHub organization/repo name
(referred to below as `<org>`), and confirmation that a history rewrite plus
force-push is acceptable (every clone and worktree is re-created afterwards).

## Read first

- `docs/public-repository-boundary.md`, `docs/open-source-strategy.md` §4, §7, §8, §10
- `SECURITY.md`, `CONTRIBUTING.md`, `NOTICE`, `.gitignore`, `.vercelignore`
- `.github/workflows/ci.yml`, `release.yml`, `promote-release.yml`
- `go.mod`, `apps/desktop/go.mod`

## Findings this brief is based on (verified 2026-09-04)

- A 23 MB binary named `stickguy` is **tracked** at the repository root
  (`git ls-files stickguy`). `.git` is 110 MB. It must leave history, not
  just the tree.
- A pattern scan of the full history (`git log --all -p` for `sk-`, `AKIA`,
  private-key headers, GitHub tokens, Convex prod keys) found one hit and it
  is a synthetic fixture (`"Use api_key: sk-abcdef0123456789abcdef ..."` in a
  classifier test). Re-run with real tools anyway (step 2).
- The Go module path is `github.com/khalidM3/overgent` in 58 tracked files.
- `Stickguy` (capitalized) appears in `internal/hookconfig/hooks.go`
  (`legacyProfileNames`), `internal/adapterrepair`, and setup tests. That is
  **migration code for installs that predate the ADR-065 rename**. Keep it.
- `.vercelignore` line 23 lists `stickguy`; the Vercel Blob host id
  `7y8t6bjdbq682vo5` appears in `vercel.json` and one other file. Public blob
  URLs are not secrets; they are fine to publish.
- `LICENSE` is already Apache-2.0; `NOTICE` exists; `CONTRIBUTING.md` already
  names DCO sign-off. `SECURITY.md` says the private channel is not yet set up.

## Tasks

### 1. Remove the tracked binary and rewrite history

```bash
git ls-files stickguy coordination            # expect: stickguy
pip3 install --user git-filter-repo            # or brew install git-filter-repo
git filter-repo --invert-paths --path stickguy --path Stickguy.zip --path dist-dogfood --path coordination-eval-report.json
git count-objects -vH; du -sh .git             # record before/after
```

Add `/stickguy` to `.gitignore` next to `/coordination`. `filter-repo` removes
the origin remote; re-add it and force-push **only after the owner says go**,
then delete every stale worktree (`git worktree list`) and re-create them.
Record the before/after size in the handoff.

### 2. Secret scan of the rewritten history

Run both, from a clean clone of the rewritten repository:

```bash
gitleaks git --no-banner .            # brew install gitleaks
trufflehog git file://. --only-verified   # brew install trufflehog
```

Expected: gitleaks reports the synthetic `sk-abcdef…` fixture; add a
`.gitleaks.toml` allowlist for that test path with a comment. trufflehog
verified findings must be zero. Paste the summary lines into the handoff.

### 3. Module path and organization name

Replace `github.com/khalidM3/overgent` with `github.com/<org>/overgent` in
`go.mod`, `apps/desktop/go.mod` (module line and any `replace` directive), and
every `.go` import. Then:

```bash
go build ./... && go test ./... && go vet ./...
pnpm desktop:test
```

Also grep docs and scripts for `khalidM3` and fix any URL.

### 4. Repository metadata files

- `SECURITY.md`: rewrite to say reports go through GitHub private vulnerability
  reporting (Settings → Code security → Private vulnerability reporting must
  be enabled by the owner when the repository becomes public), supported
  version is the latest release, target first response within 7 days. Keep
  the "never include credentials, source, transcripts" paragraph.
- `CONTRIBUTING.md`: keep DCO; add a "Local development without any
  credentials" pointer to `docs/development.md` and a "Which model/agent
  workflow" pointer to `AGENTS.md`. Add labels list from
  `docs/open-source-strategy.md` §8.
- Add `.github/ISSUE_TEMPLATE/bug_report.md`, `feature_request.md`,
  `adapter_request.md`, and `.github/PULL_REQUEST_TEMPLATE.md` (checklist:
  tests, DCO, security/privacy review needed?, protocol regenerated?).
- Add `.github/CODEOWNERS` with the owner for `protocol/`, `convex/`,
  `internal/hosted/`, `internal/agentactivity/`, `install/`, `.github/`.
- `NOTICE`: unchanged here; Lane 03 adds the Convex backend entry.
- Delete `.vercelignore` line 23 (`stickguy`).

### 5. CI hygiene for a public repository

- `ci.yml`: add top-level `permissions: contents: read`. Confirm no step uses
  a secret (the Go matrix on `macos-14` and the TypeScript job on
  `ubuntu-latest` do not). Add `pull_request` trigger if missing so forks get
  checks.
- `release.yml` and `promote-release.yml`: they run on `v*` tags and manual
  dispatch and use signing secrets from a protected environment; confirm the
  environment is required so forks cannot trigger them. No change expected;
  state that in the handoff.
- `dependabot.yml`: add `gomod` for `apps/desktop` if only the root module is
  listed, and `npm` for `convex/`, `apps/dashboard/`, `packages/*`.
- Add a `codeql.yml` workflow for `go` and `javascript-typescript`
  (default query pack; weekly schedule plus pull requests).

### 6. Docs sweep for private-beta language

`git grep -nE "private beta|invited beta|owner gates|closed test|friends"`
across `README.md`, `docs/`, `install/`, `apps/desktop/README.md`,
`convex/README.md`. Rewrite each hit to describe the public release. Do not
touch `docs/decisions.md` history (ADRs are records). The README itself is
rewritten in Lane 07; here only remove statements that are now false.

`install/dogfood/` is the pre-release closed-test channel. Move it to
`install/legacy-dogfood/` with a one-line README saying it is unsupported and
kept for reference until the first public release, or delete it if the owner
confirms nobody is still on that channel. Ask; do not decide.

### 7. Local leftovers (not committed, still worth cleaning)

Untracked and ignored files at the root (`stickguy`, `coordination`,
`coordination-eval-report.json`, `dist-dogfood/`, `.vercel/`) are not in the
repository. Leave them; mention in the handoff that they are local only.

## Acceptance

- `git ls-files | xargs -I{} stat -f "%z {}" {} | sort -rn | head -3` shows
  the largest tracked file is a `.wasm.gz` grammar under
  `internal/contract/wasmgrammar/`, not a binary.
- `.git` size is recorded before and after; gitleaks/trufflehog output is
  recorded; the only allowed finding is the synthetic fixture.
- `go test ./... && go vet ./... && pnpm typecheck && pnpm test && pnpm build
  && pnpm desktop:test` pass after the module rename.
- `git grep -n khalidM3` returns nothing.
- `SECURITY.md` no longer says the channel is temporary.
- CI runs green on a pull request from a fork with no secrets available.

## Do not

- Do not remove the `Stickguy` legacy-profile handling; existing installs need it.
- Do not change `LICENSE`.
- Do not force-push before the owner confirms.
- Do not touch `protocol/`, `convex/functions/`, or `internal/app/`.
