# Phase 3 — Launch checklist: first public release

Status: checklist
Last updated: 2026-09-04
Executor: Sonnet 5 for the writing; the owner performs the steps marked
**owner** because they need account access or are irreversible.

Preconditions: Lanes 02, 03, 04, 05 merged to `main`; Lane 06 merged or its
limitation recorded in the README (see "Known limitations" below).

## 1. README rewrite

Replace `README.md` with a public-audience document in this order. Keep it
under about 200 lines; everything deeper links into `docs/`.

1. One paragraph: what Overgent does (air traffic control for multiple coding
   agents in one repository; catches overlapping edits, stale contract
   assumptions, and duplicated work while agents are working, not at merge
   time) and what it does not do (it never edits code, merges, or steers
   agents).
2. Install: `curl -fsSL https://overgent.com/install.sh | sh`. Homebrew is
   deferred (see §4). Apple Silicon macOS only (ADR-050) stated
   plainly; "build from source" link for others.
3. Local by default: "Nothing leaves your Mac" with one sentence on the
   bundled backend and the data location.
4. Team mode: create or join a team Project on Overgent Cloud; one paragraph
   on what syncs (link `docs/security-privacy.md` prohibited-data list) and
   one on self-hosting (link `docs/self-hosting.md`).
5. Bring your own model: three lines showing `overgent ai set …`; link
   `docs/ai-providers.md`.
6. Supported agents today (Codex, Claude Code; Cursor status from
   `internal/cursorsetup` honesty), and how to add an adapter
   (`docs/adapter-development.md`).
7. What it catches, with honest fidelity: deterministic findings always;
   semantic findings only with a configured provider.
8. Development checks (the existing block), contributing, security, license.
9. Known limitations: platform; if Lane 06 has not shipped, "a profile is
   either local or team; run `overgent reset` to switch" in exactly those
   words.

## 2. Documentation index and cross-links

- `docs/README.md`: final reading order includes `self-hosting.md`,
  `hosted-operations.md`, `ai-providers.md`, and `migration/README.md`
  (kept as history until every lane is merged, then moved to
  `docs/history/`).
- `AGENTS.md` mission paragraph and rules reflect ADR-071…074 (done in
  Phase 0; re-verify).
- `docs/implementation-plan.md`: add "L9 — Open-source rewire" summarizing
  Lanes 01–06 with their exit evidence files.

## 3. Repository visibility (**owner**)

1. Confirm Lane 02's rewritten history is what `origin/main` holds and every
   collaborator has re-cloned.
2. Enable GitHub private vulnerability reporting, Dependabot alerts, and
   secret scanning with push protection.
3. Flip visibility to public. Add topics (`coding-agents`, `claude-code`,
   `codex`, `multi-agent`, `developer-tools`, `go`, `convex`).
4. Confirm CI is green on the public repository and that the release
   environment still requires approval.

## 4. Release (**owner** runs; agent prepares)

- Tag `v0.x.0` from `main`; `release.yml` builds, signs, notarizes, and
  publishes to the blob store; `promote-release.yml` promotes after the
  clean-machine gate in `docs/beta-release.md` (rename that file to
  `docs/release.md` in this phase; contents unchanged apart from the title).
- Homebrew: **deferred past launch.** The install script already verifies
  the Apple team id, size, checksum, and signed manifest, and releases are
  published to the blob store rather than GitHub Releases, so a cask would
  need a tap repository, a token, and a second download path to keep in
  sync. Add it after launch if people ask; a cask must download the same
  signed zip.
- `overgent version --json` on the released binary reports the public
  commit; paste it into the release notes.
- Release notes: link the source tag, workflow run, SBOM, checksums, and
  `docs/release.md` verification steps (open-source-strategy §6 item 8).

## 5. Landing page

The landing page is served at `/` by the dashboard app
(`apps/dashboard/index.html`, `apps/dashboard/src/landing.css`; commit
`b847761` describes how it reads the signed release manifest at runtime). It
must match the README's four claims: local by default, team mode optional, bring your own model, open
source under Apache-2.0. Remove any "beta", "waitlist", or "pricing"
language. Keep the design-system rules.

## 6. Announcement copy (agent drafts, owner edits)

One paragraph for the livestream description and one for a GitHub
Discussions "Welcome" post: what it is, the install line, what it never
uploads, and where to report issues. No claims about semantic detection
precision beyond what `validation/evals/coordination` currently passes.

## 7. Post-launch watch (first two weeks)

- Track: installs (blob download count), Projects created on Cloud, issues
  labeled `false-positive`. These three numbers decide whether
  `08-post-launch-product.md` starts.
- Do not add analytics to the client. The count of Cloud Projects comes from
  the backend; local installs are estimated from downloads only.
