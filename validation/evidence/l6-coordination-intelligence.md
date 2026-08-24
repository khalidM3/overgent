# L6 coordination intelligence evidence

Date: 2026-08-24
Outcome: PASS, with the provider capability narrowing in ADR-030

## Delivered boundary

- Bounded semantic intent/change projection rejects secret assignments, private
  keys, code fences, prompt-override text, and `.env` references before storage
  or embedding. Objects, vectors, provider/model revisions, lifecycle, and
  retention remain separate.
- `stickguy-concepts/v1` implements the public embedding-provider boundary. The
  in-memory index proves portable exact Project/repository isolation; the hosted
  adapter uses a 32-dimensional Convex vector index with mandatory opaque
  `scopeKey` filtering.
- Hosted retrieval authenticates before search, reauthorizes and reloads current
  objects after search, retries a revision race once, and excludes semantic
  candidates if the revision changes before brief assembly. Provider/index
  failure records degraded fidelity and returns the structural brief.
- `coordination/v1` fuses path, dependency, schema/contract, component, lexical,
  and semantic evidence into all seven public finding kinds. Findings have
  stable fingerprints, material-only revisions, explanations/provenance, and
  close when current evidence disappears.
- `brief-router/v1` deterministically renders only relevant current items within
  the requested 128–800 token budget, preserves compact references for required
  items, records item revisions/delivery acknowledgement, and creates
  `stale_assumption` only for a materially changed relevant high-priority item.
- Radar feedback is Project-authorized, rate guarded, retained separately from
  project content, versioned with the engine, and exposed in the dashboard.
  Existing MCP brief/item delivery receives the same backend projection.

## Public evaluation

`packages/coordination/test/intelligence.test.ts` is the executable labeled
corpus; `validation/evals/l6/README.md` documents its scope and command. Eight
tests cover the L6 positive and negative labels, every finding kind, unrelated
routing, repository isolation, budget/truncation, relevant-only staleness,
strict optional-adjudication output, adversarial semantic input, and a provider
that fails both bounded attempts while structural evaluation remains live.

The dashboard suite adds explicit usefulness feedback and live HTTP transport
coverage. Hosted domain tests include feedback retention ordering. Protocol
generation adds the closed feedback request and the checkpoint
`affectedInterfaces`/`dependencies` fields already specified by the MCP
lifecycle contract.

The real Convex `1.45.0` compiler accepted the schema, vector index, actions,
queries, and mutations against an anonymous local deployment. The combined
L2/L6 live suite then passed creator/invite enrollment and two-device
publication plus duplicate behavior under different paths, incompatible intent
before edits, scoped shared dependency delivery, an empty unrelated brief,
stale-assumption creation, cross-Project semantic isolation, and authorized
radar feedback. Representative local timings were 15 ms for the second
manifest/finding transaction and 72 ms for atomic activation of 1,000 paths;
they are feasibility observations, not hosted performance claims.

Convex documents that vector indexes require a fixed dimension and may declare
filter fields, and that vector search runs from an action. Stickguy additionally
reloads and reauthorizes current rows after retrieval and checks the repository
context revision before rendering. See
<https://docs.convex.dev/search/vector-search>.

## Privacy and security review

- No source, diffs, Git objects, raw transcripts, system/developer prompts,
  hidden reasoning, environment values, raw commands/output, or credentials are
  semantic inputs.
- Vector filtering uses the server-derived composite Project/repository scope;
  post-search reads repeat authorization, scope, active-state, workstream-state,
  and content-revision checks.
- The optional adjudicator is an interface plus closed output validator only;
  no model is required and no project content is sent to one.
- Feedback is not training data and is not joined to private content silently.

## Honest limits

The deterministic concept provider is intentionally vocabulary-bounded; it is
not a general embedding model. Semantic findings remain quiet radar/brief data,
not proactive interruptions. Hosted cost/load with a larger provider and corpus
remain L9 gates. One repository scope currently fails closed above 64 active
semantic objects or 256 findings rather than attempting an unbounded mutation;
larger-team indexing is a measured L9 scale gate. Project deletion still lacks a frozen public endpoint from L2,
although retention ordering deletes embeddings before semantic objects and
includes finding feedback. The live proof used anonymous loopback state and
synthetic inputs; it does not measure hosted-cloud availability or cost.

## Verification

PASS:

```text
go test ./...
go vet ./...
go test -race ./...
pnpm typecheck
pnpm test
pnpm build
pnpm protocol:generate
pnpm protocol:check
pnpm --dir apps/dashboard test:e2e
pnpm desktop:test
pnpm desktop:build
cd convex && ./node_modules/.bin/convex dev --once --tail-logs disable --typecheck enable
cd convex && pnpm test:live
```

The disposable Convex backend was stopped, its loopback ports no longer
responded, and `.env.local` plus `.convex/` local admin/state were deleted after
the run.
