# Lane E — M4 LLM judgment layer

Goal: findings stop being purely structural. A judgment layer explains why two
workstreams are colliding, decides whether something is worth delivering, and
communicates uncertainty. Unlocks eval scenarios B (semantic duplication) and
G (WIP uncertainty) per ADR-045.

**This lane must not modify `protocol/`.** Everything it needs is already on
the wire: intents, checkpoints with verification state, contract fingerprints,
read sets, and path manifests. If you believe a wire change is required, STOP
and report it instead of adding one — Lane F owns protocol this round.

## Read first

- ADR-045 and ADR-030 (the deterministic concept provider it supersedes as the
  primary path) in `docs/decisions.md`
- `docs/implementation-plan.md`, M4 section
- `convex/functions/intelligence.ts` and the finding pipeline in
  `convex/functions/service.ts` — especially `upsertContractFindings`,
  `upsertPathFindings`, and the brief/candidate retrieval path
- `docs/openai-embeddings.md` — the ADR-040 managed-provider pattern
  (hosted-only secret, async scheduling, degraded fallback). Your adjudicator
  follows the same shape.
- `validation/evals/coordination/scenarios.go` — scenarios B and G and their
  existing assertions
- **Invoke the `claude-api` skill before writing any Anthropic client code.**
  Do not write model ids, SDK calls, or request shapes from memory.

## Design (decided — do not revisit)

### The adjudicator is a provider behind an interface

Mirror the `EmbeddingProvider` boundary exactly:

- A `JudgmentProvider` interface with one operation: given a bounded,
  policy-passed description of two or more candidate workstream states, return
  a structured verdict `{ relationship, confidence, severity, explanation,
  delivery }` where `delivery` is one of `next_turn`, `dashboard`, `silent`.
- **Managed provider**: Anthropic, called only from a hosted asynchronous
  Convex action, never from the local core, dashboard, or agent config. The
  API key is a hosted deployment secret exactly as ADR-040 defines. Default to
  Claude Sonnet 5 for adjudication (high volume, cost-sensitive); confirm the
  exact model id via the `claude-api` skill.
- **Deterministic fallback**: when no key is configured or the provider fails,
  fall back to the existing `stickguy-concepts/v1` concept engine plus lexical
  and structural signals. Failure marks semantic processing degraded and never
  removes a deterministic finding.

### The eval suite must stay hermetic

`pnpm eval:coordination` runs against an anonymous loopback deployment with no
API key and **must pass scenarios B and G on the deterministic path**. A suite
that needs a live model is not a gate. Structure the work so the managed
provider improves explanation quality and precision, while the deterministic
path is sufficient to identify the relationship and its severity.

### What the judgment layer must produce

- **Scenario B (duplication)**: a `redundant_work` finding naming both
  workstreams, with a reason and evidence explaining what they share, at a
  severity that does not interrupt either agent. The existing L6 semantic
  machinery already retrieves candidates; this lane turns a candidate into an
  explained finding with a delivery decision.
- **Scenario G (WIP uncertainty)**: when the workstream that changed a contract
  reported an *unverified* checkpoint (verification state not passed), the
  resulting finding must communicate work-in-progress explicitly — the words
  `unverified`, `work-in-progress`, or `WIP` must appear in its reason or
  evidence — and carry a severity strictly below the verified scenario A/C
  case. Do not weaken A or C to achieve this.
- **Delivery decisions**: route every finding through one place that returns
  `next_turn` / `dashboard` / `silent`, so scenarios E and F keep their
  silence. E must still produce zero findings and inject nothing; F must stay
  non-interrupting.

### Cost and outage

Adjudication is scheduled after the coordination object is durable, is bounded
per project, and is skipped entirely when candidates are structurally
unambiguous (an exact same-path collision needs no model). Provider outage
preserves M2 and M3 behavior exactly.

## Acceptance criteria

1. `pnpm eval:coordination` with `semantic` added to the required list in
   `validation/evals/coordination/capabilities.json` passes scenarios B and G,
   with A, C, E, F still passing, on three consecutive runs and with no API key
   present.
2. Scenario E still yields zero findings and zero injected context; scenario F
   still produces no interrupt and no `stale_assumption`.
3. Unit tests cover the verdict parser (including a malformed model response
   falling back deterministically) and the delivery decision.
4. Aggregate `precision` in the JSON report improves over the current 0.529 —
   the `shared_dependency` finding that currently fires alongside contract
   findings in A and C is the obvious target: judge whether it adds anything
   beyond the contract finding and suppress or merge it when it does not.
5. `docs/coordination-intelligence.md` gains a section on the judgment layer;
   add an ADR only if you deviate from ADR-045, in which case STOP and report
   first.
6. All standard checks pass: `go test ./...`, `go vet ./...`, `pnpm typecheck`,
   `pnpm test`, `pnpm build`. No `protocol/` changes, so `protocol:check` must
   report no drift.

## Out of scope

Dependency readiness (Lane F), any protocol change, the `deliveryMillis`
latency tail (separately owned), replacing the embedding provider.
