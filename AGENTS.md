# Stickguy agent instructions

## Mission and required reading

Build a persistent air-traffic-control layer for teams working with coding agents. Deterministic live coordination is the foundation; V1 also requires semantic candidate retrieval and evidence-backed collision findings over bounded shared summaries. Optional model adjudication may improve ambiguous findings but must not be required for structural detection.

Stickguy is a coordination harness, not a coding-agent harness. Do not absorb model loops, repository editing, shell/test execution, coding-model routing, or coding-agent permission management. Implement the lifecycle and context-router contracts in `docs/coordination-harness.md`.

Before implementation, read every document in `docs/README.md` order. Do not replace Go, Convex, Project terminology, one-service architecture, or privacy boundaries without an owner-approved superseding ADR.

## Non-negotiable rules

- User-facing container is `Project`, never `Room`.
- Local core is Go; React/Convex layers are TypeScript.
- One per-user service manages multiple projects/workspaces.
- Go calls hosted backend only through versioned Stickguy HTTP contracts.
- OpenAPI/JSON Schema is external contract source of truth; never hand-edit generated code.
- Never upload Git objects, environment values, credentials, tokens, cookies,
  private keys, protected credential paths, raw tool results, or command output.
  ADR-036 permits reading the vendor transcript named by a supported hook for a
  session in a registered repository: it is read locally, bounded, never copied
  to a second store, and always shown to that session's own member. Projecting it
  to other members stays off by default and requires per-session preview and
  versioned consent; quoted code and file *names* are allowed in a consented
  conversation, the secret material above is not, and sharing is revocable with
  deletion. Each vendor needs its own adapter (ADR-039).
- Never automatically merge/rebase/cherry-pick/reset/checkout/apply patches or mutate teammate work.
- Never shell-concatenate Git/user input; use argument arrays and validate refs/paths.
- Never bind local servers beyond loopback.
- Every public hosted operation authenticates, authorizes, validates, and applies size/rate guards.
- Core behavior works with AI disabled.
- Preserve honest fidelity: unsupported/disabled semantic processing degrades to structural evidence and is never presented as full intelligence.
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
