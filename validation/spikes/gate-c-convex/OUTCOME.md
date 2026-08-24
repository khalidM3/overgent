# Gate C decision record — draft for integrator acceptance

Status: **Pass proposed**
Date: 2026-08-23
Scope: L-1 Gate C only; this is not production L0/L2 code.

## Decision

Retain ADR-004 (Convex hosts coordination state) and ADR-012 (hosted semantic
index behind portable interfaces). The bounded spike found no need to replace or
narrow either architecture boundary.

The production design must preserve the constraints demonstrated here:

1. Go continues to call only versioned Stickguy HTTP contracts; Convex IDs and
   client APIs do not enter the Go boundary.
2. Manifest chunks stage separately and become the current revision only in the
   completion mutation that validates the entire set.
3. Material repository changes advance a monotonic repository-scope context
   revision in the same transaction as their projection change.
4. Semantic vectors remain separate from readable semantic objects. Every vector
   query requires the opaque composite project/repository `scopeKey` as an
   indexed filter.
5. Candidate IDs are not authorization evidence. After retrieval, production
   code derives membership/project/repository scope from authenticated server
   context, reloads current objects, and validates lifecycle, content revision,
   and model version before use.
6. Revision-safe semantic assembly retries a bounded number of times and returns
   an honestly labeled structural fallback when writes keep racing it.
7. Supersession, object deletion, retention expiry, and Project deletion remove
   the matching vector rows as well as findings and delivery receipts.

## Evidence

The executable harness and redacted run are in `scripts/live-test.mjs` and
`evidence/2026-08-23-live-run.md`. Strict typechecking and all live assertions
passed against Convex 1.45.0 on an anonymous local loopback deployment.

## Limits and cost note

The spike deliberately used 5-dimensional synthetic vectors; a provider's real
dimension remains a model-versioned adapter choice. Convex documents vector
dimensions from 2–4096, a maximum vector result set of 256, 1 MiB documents,
16 MiB function arguments, and transaction limits including 16 MiB read/write,
32,000 scanned documents, and 16,000 written documents. The 1,000-path fixture
used ten bounded 100-path mutations, with a measured maximum encoded argument of
3,050 bytes.

Hosted cost was not measured because the required disposable deployment was
anonymous and local. Current Convex pricing counts explicit calls and
subscription updates as function calls; database/index storage, database I/O,
and vector search/storage are metered by plan. This gate proves feasibility, not
the eventual cost per active member-day. L2/L6 load tests must measure the real
event rate, subscription fan-out, production embedding dimensions, index size,
and retention volume before selecting hosted capacity or alert budgets.

Primary vendor references:

- <https://docs.convex.dev/cli/local-deployments>
- <https://docs.convex.dev/search/vector-search>
- <https://docs.convex.dev/production/state/limits>

## Honest limitations carried forward

- Public mutations in this disposable spike intentionally expose narrow test
  seams. The `authorizedProjectId` and `authorizedRepositoryId` arguments
  simulate the post-retrieval check; production must never trust client-supplied
  authorization and must satisfy the full L2 authentication/authorization gate.
- Cleanup uses bounded synthetic-table scans to demonstrate deletion semantics.
  Production retention must use indexed expiry selection, scheduled bounded
  batches, retries, and deletion observability so transaction scan/write limits
  cannot strand data.
- Local deployments are a beta development facility and are not evidence of
  hosted availability, regional behavior, abuse controls, or production
  operations.
- The race is deterministically injected at the repository revision boundary. L6
  still needs integration/load coverage for naturally concurrent semantic writes.
- This result does not activate an embedding provider or optional adjudicator;
  structural operation remains mandatory when either is absent.

No superseding ADR is proposed. The next integrator action is to review and mark
this Gate C result accepted alongside the other L-1 gates; production L0 remains
blocked until every L-1 gate passes, narrows honestly, or selects its documented
fallback.
