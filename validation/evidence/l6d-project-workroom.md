# L6D Project Workroom evidence

Date: 2026-08-24

## Outcome

PASS. The user-facing dashboard was replaced with a Project Workroom organized around Projects, people, live Codex/Claude sessions, contextual collisions, recent activity, and a selectable detail inspector. The hosted and local-core contracts were not changed.

## Delivered behavior

- Collapsible Project sidebar with repository labels, live-session counts, collision counts, settings, and a working command palette (`Command-K` / `Control-K`).
- `Now` surface groups sessions by person and presents Codex, Claude Code, shared task, presence, action, tool, subagent, path-count, and freshness metadata without user-facing `Workstream` terminology.
- People and sessions now use a collapsible tree: each Project member is a root node and their Codex, Claude Code, or shared-task sessions are indented children with visible hierarchy connectors.
- Codex and Claude Code use distinct, labeled vendor marks; the text label remains present so recognition never depends on icon or color alone.
- Open collisions appear inline with affected people and safe path evidence. Selecting one opens lifecycle, feedback, evidence, and affected-session detail in the inspector.
- Selecting a session opens a consolidated status header, optional branch, current action, tool, safe paths, active subagents, fidelity, presence, and large-change summary. Empty subagent sections are omitted.
- The hosted dashboard projection now attaches up to 12 already-collected, bounded `agent.activity_reported` events to the matching session. The inspector renders those safe actions as a chronological session-activity rail rather than implying access to a raw transcript or private reasoning.
- `Recent` is a quiet chronological feed rather than a metric dashboard.
- Light and dark monochrome themes, compact high-contrast status colors, responsive Project navigation, and redesigned activation/onboarding/settings states.
- The fixture demonstrates simultaneous Codex and Claude Code sessions on the same safe path and the resulting deterministic collision.

## Verification

- `pnpm install --frozen-lockfile`: PASS; lockfile supply-chain policy PASS.
- `pnpm --recursive typecheck`: PASS.
- `pnpm --recursive test`: PASS; dashboard 19 tests, coordination 8 tests, hosted 14 tests, protocol fixture test PASS.
- `pnpm --recursive build`: PASS; dashboard production bundle 241.49 kB JS / 74.05 kB gzip and 34.14 kB CSS / 6.89 kB gzip.
- `pnpm --dir apps/dashboard test:e2e`: PASS; 12 flows across 1440x1000 laptop and iPhone 13 viewport.
- `node scripts/protocol-check.mjs` under Node 22 with the documented Go toolchain on `PATH`: PASS.
- `pnpm desktop:test`: PASS.
- `go test ./...` outside the filesystem sandbox for loopback/socket integration tests: PASS.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.
- Native desktop assets and `Stickguy.app` build: PASS.

## Visual review

The fixture Project Workroom was inspected in a real browser at 1440x1000 and 390x844, including the people/session tree, Codex and Claude marks, Codex activity rail, optional branch display, and Claude inspector with no empty subagent section.

## Privacy and security

The redesign only consumes existing bounded Project snapshots and existing bounded agent-activity events. It does not add collection fields, routes, storage, source/diff rendering, prompts, transcripts, environment values, raw command output, private reasoning, or secrets. Unauthorized and version-mismatch states still render no Project metadata. Collision and session details remain limited to disclosed action summaries and safe repository-relative paths.

## Known limits

- The inspector is a third column at desktop widths and follows the main activity on narrow layouts; a future native split-view can make it resizable without changing data contracts.
- Theme selection is session-local in this pass and defaults to light when the view is reopened.
- Fixture-only `Simulate activity` remains absent from the live source.
- A branch is displayed only when an adapter reports it; the current local hook event contract does not yet collect branch names, so live sessions honestly omit the field rather than inferring one.
- Project member identity still originates in enrollment data. The intended hierarchy is a member-controlled Project display name in the live view, with device labels confined to Settings/Security and email omitted from the workroom; a separate profile/enrollment change is needed to migrate older Projects created with a hostname as the member name.
