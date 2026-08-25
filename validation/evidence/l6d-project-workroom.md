# L6D Project Workroom evidence

Date: 2026-08-24

## Outcome

PASS. The user-facing dashboard was replaced with a Project Workroom organized around Projects, people, live Codex/Claude sessions, contextual collisions, recent activity, and a selectable detail inspector. The hosted and local-core contracts were not changed.

## Delivered behavior

- Collapsible Project sidebar with repository labels, live-session counts, collision counts, settings, and a working command palette (`Command-K` / `Control-K`).
- `Now` surface groups sessions by person and presents Codex, Claude Code, shared task, presence, action, tool, subagent, path-count, and freshness metadata without user-facing `Workstream` terminology.
- Open collisions appear inline with affected people and safe path evidence. Selecting one opens lifecycle, feedback, evidence, and affected-session detail in the inspector.
- Selecting a session opens its current action, tool, safe paths, active subagents, fidelity, presence, and large-change summary.
- `Recent` is a quiet chronological feed rather than a metric dashboard.
- Light and dark monochrome themes, compact high-contrast status colors, responsive Project navigation, and redesigned activation/onboarding/settings states.
- The fixture demonstrates simultaneous Codex and Claude Code sessions on the same safe path and the resulting deterministic collision.

## Verification

- `pnpm install --frozen-lockfile`: PASS; lockfile supply-chain policy PASS.
- `pnpm --recursive typecheck`: PASS.
- `pnpm --recursive test`: PASS; dashboard 19 tests, coordination 8 tests, hosted 14 tests, protocol fixture test PASS.
- `pnpm --recursive build`: PASS; dashboard production bundle 238.81 kB JS / 73.25 kB gzip and 31.97 kB CSS / 6.48 kB gzip.
- `pnpm --dir apps/dashboard test:e2e`: PASS; 12 flows across 1440x1000 laptop and iPhone 13 viewport.
- `node scripts/protocol-check.mjs` under Node 22 with the documented Go toolchain on `PATH`: PASS.
- `pnpm desktop:test`: PASS.
- `go test ./...` outside the filesystem sandbox for loopback/socket integration tests: PASS.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.
- Native desktop assets and `Stickguy.app` build: PASS.

## Visual review

The fixture Project Workroom was inspected in a real browser at desktop width in light and dark themes, including the Codex inspector and collision inspector. The responsive view was inspected at 390x844 after refining the Project strip so repository labels, session counts, and collision counts remain legible.

## Privacy and security

The redesign only consumes existing bounded Project snapshots. It does not add collection fields, routes, storage, source/diff rendering, prompts, transcripts, environment values, raw command output, or secrets. Unauthorized and version-mismatch states still render no Project metadata. Collision and session details remain limited to disclosed action summaries and safe repository-relative paths.

## Known limits

- The inspector is a third column at desktop widths and follows the main activity on narrow layouts; a future native split-view can make it resizable without changing data contracts.
- Theme selection is session-local in this pass and defaults to light when the view is reopened.
- Fixture-only `Simulate activity` remains absent from the live source.
