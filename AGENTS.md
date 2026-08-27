# Stickguy agent instructions

## Mission and required reading

Build a persistent air-traffic-control layer for teams working with coding agents. The product maintains a live shared world model of what every agent session believes and is building (intents, read sets, write sets, contract fingerprints, dependency claims), detects divergence (stale contracts, semantic duplication, collisions, dependency readiness), and routes corrections into affected agent turns before work is wasted. Deterministic evidence is the trigger layer and always works offline; a hosted LLM is the judgment layer (ADR-045). See ADR-044 through ADR-048 for the V2 reboot.

Stickguy is a coordination harness, not a coding-agent harness. Do not absorb model loops, repository editing, shell/test execution, coding-model routing, or coding-agent permission management. Implement the lifecycle and context-router contracts in `docs/coordination-harness.md`.

Before implementation, read every document in `docs/README.md` order. Do not replace Go, Convex, Project terminology, one-service architecture, or privacy boundaries without an owner-approved superseding ADR.

## Non-negotiable rules

- User-facing container is `Project`, never `Room`.
- Local core is Go; React/Convex layers are TypeScript.
- One per-user service manages multiple projects/workspaces.
- Go calls hosted backend only through versioned Stickguy HTTP contracts.
- OpenAPI/JSON Schema is external contract source of truth; never hand-edit generated code.
- The privacy boundary is the wire, not local reads (ADR-044). The local
  service may read source, diffs, and vendor transcripts on the member's
  machine. What syncs is derived, structured coordination facts: contract
  fingerprints, bounded diff summaries, intents, dependency claims, manifests,
  finding evidence, and classified session content. Never upload Git objects,
  raw source files, raw diffs, environment values, credentials, tokens,
  cookies, private keys, protected credential paths, raw tool results, or
  command output; the secret classifier (ADR-038 semantics) is a mandatory
  non-disableable wire gate. Project membership plus the pause switch is the
  sharing consent model (ADR-047). Each vendor needs its own adapter (ADR-039).
- Never automatically merge/rebase/cherry-pick/reset/checkout/apply patches or mutate teammate work.
- Never shell-concatenate Git/user input; use argument arrays and validate refs/paths.
- Never bind local servers beyond loopback.
- Every public hosted operation authenticates, authorizes, validates, and applies size/rate guards.
- Core behavior works with AI disabled.
- Preserve honest fidelity: unsupported/disabled semantic processing degrades to structural evidence and is never presented as full intelligence.
- Dashboard and desktop UI follows `docs/design-system.md`: hairlines and space instead of filled cards, `--alert` as the only colour and only for work converging on the viewer, no pulsing status dots, elapsed time always via `formatElapsed`, monospace for machine facts and sans for human statements.
- Installed code, collection behavior, protocols, adapters, core dashboard/backend, installers, and release workflows are public; private cloud operations live in a separate repository.
- Never add a production secret, private customer data, internal incident detail, or abuse-detection secret to the public repository.

## Execution

Follow `docs/implementation-plan.md`. Verify an exit gate, then continue to the next unblocked level. Time estimates are not stopping conditions. Parallelize only using documented lanes; one integrator owns shared contracts.

The scaffold must make these meaningful and keep this list current:

```bash
go test ./...
go vet ./...
pnpm install --frozen-lockfile
pnpm typecheck
pnpm test
pnpm build
pnpm protocol:generate
pnpm protocol:check
```

Add race/lint/integration/Playwright commands as implementations exist. Never document a no-op check.

## Go conventions

- Baseline Go 1.26; CI also Go 1.27.
- Prefer standard library/small explicit packages; use context cancellation across I/O/processes.
- Wrap errors with operation context; avoid log-and-return duplication.
- Use `log/slog`; secret values must not be ordinary serializable log fields.
- Narrow build-tagged platform interfaces; no release CGO without ADR.
- Tests use isolated temp config roots/repos, never real contributor state.

## TypeScript conventions

- Strict TypeScript and typed public boundaries.
- Convex wrappers thin; domain logic separately testable.
- Dashboard renders explicit loading/offline/unauthorized/version states.
- No state library or SSR framework without demonstrated need.

## Handoff

Report delivered behavior/criterion, files/contracts changed, verification, security/privacy, known limits, and next unblocked task. If evidence disproves an assumption, stop expanding it and propose a focused ADR.
