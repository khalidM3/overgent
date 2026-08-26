# Lane F — M5 dependency readiness

Goal: a workstream can say it is blocked on something another workstream has
not built yet, and Stickguy tells it the moment that thing exists — including
the honest intermediate state where the contract exists but is not yet
verified. This is the throughput half of the product (ADR-048), not a planning
surface (ADR-037 stands: no plan items, no boards).

**This lane owns all `protocol/` changes this round.** Lane E is running in
parallel and will not touch protocol. You will both edit
`convex/functions/service.ts`; keep your changes in their own functions so the
merge stays mechanical.

## Read first

- ADR-048 (dependency claims) and ADR-037 (why planning stays out) in
  `docs/decisions.md`
- `docs/implementation-plan.md`, M5 section
- `internal/mcp/` — the seven lifecycle tools, especially `begin_work` and
  `update_intent` argument handling
- `convex/functions/service.ts` — `upsertContractFingerprints` /
  `upsertContractFindings` (Lane B's work; readiness is its mirror image) and
  the checkpoint projection with its verification state
- `protocol/openapi.yaml`, `protocol/schemas/`, `docs/protocol.md` — and
  remember `pnpm protocol:generate` is the only generation path
- `validation/evals/coordination/scenarios.go` — scenario D and its assertions

## Design (decided — do not revisit)

### Claims are declared, not inferred

`begin_work` and `update_intent` gain an optional `waiting_on` argument: an
array of at most 8 bounded strings, each naming a contract, symbol, or path the
session cannot proceed without (scenario D uses `session-api`). Validate and
bound them exactly as `contracts` is validated today. LLM inference from intent
text is explicitly **out of scope** for this lane — it belongs with the
judgment layer and would couple you to Lane E.

Wire shape: extend the existing intent payload with optional `waitingOn`, and
add a `dependency_ready` finding kind with evidence
`{ claim, satisfiedByWorkstreamId, satisfiedBy: { path, symbols }, state }`
where `state` is `stable_wip` or `ready`. Keep both additions optional so a
device on the older shape still publishes.

### Readiness is observed, never guessed

A claim is satisfied when, in the same project and repository scope, another
live workstream produces evidence naming it:

- **`ready`** — a contract fingerprint from another workstream contains an
  exported symbol matching the claim (case-insensitive match on the symbol
  name, the file's base name, or the claim appearing in the path), **and** that
  workstream's most recent checkpoint reports verification passed.
- **`stable_wip`** — the same symbol/path evidence exists, but the producing
  workstream's latest checkpoint is unverified or absent. This is the honest
  intermediate: the contract is stable enough to build against, the
  implementation is not finished. Severity strictly below `ready`, and its
  reason must say so using the words `unverified` or `work-in-progress`.

Emit at most one open finding per (claiming workstream, claim). A claim that
moves from `stable_wip` to `ready` updates the same finding to a new revision
rather than creating a second one — the M3 injection channel treats a new
revision as newly deliverable, which is exactly the desired "now you can
proceed" moment. Resolve the finding when the claiming workstream drops the
claim or finishes.

Delivery needs no new channel: `dependency_ready` findings flow through the
existing brief and injection path automatically. Verify that they do rather
than adding a parallel route.

### Scenario D

Update scenario D's script to pass `waiting_on: ["session-api"]` through the
new argument instead of relying on the free-text approach line. You may adjust
D's script and assertions only to supply the claim and to assert the two
states; do not weaken what it checks. Its existing assertions already require
a `dependency_ready` finding targeting WS1 naming `session-api`, plus a
stable-but-WIP notice from the unverified checkpoint that precedes it.

## Acceptance criteria

1. `pnpm eval:coordination` with `dependency` added to the required list in
   `validation/evals/coordination/capabilities.json` passes scenario D on three
   consecutive runs, with A, C, E, F still passing.
2. Scenario D proves both states in order: the unverified checkpoint yields a
   `stable_wip` notice, and the verified one upgrades the same finding to
   `ready` at a new revision (assert the finding id is stable across the
   upgrade).
3. Hosted tests cover: a claim satisfied by another workstream; a claim that
   nothing satisfies producing no finding; a workstream's own output never
   satisfying its own claim; and cross-project isolation (a matching contract
   in another project satisfies nothing).
4. Go tests cover `waiting_on` validation and bounding at the MCP boundary.
5. Scenario E still yields zero findings and zero injected context.
6. `pnpm protocol:generate` then `pnpm protocol:check` pass with generated code
   committed; all standard checks pass.
7. `docs/coordination-intelligence.md` gains a short dependency-readiness
   section.

## Out of scope

LLM inference of claims from intent text, the judgment layer (Lane E), any
planning or task-board surface, the `deliveryMillis` latency tail.
