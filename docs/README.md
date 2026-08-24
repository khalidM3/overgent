# Stickguy documentation index

Read in order:

1. [`stickguy-v1-spec.md`](stickguy-v1-spec.md) — behavior, scope, acceptance.
2. [`decisions.md`](decisions.md) — settled decisions; never reopen silently.
3. [`architecture.md`](architecture.md) — components, state, reliability, failures.
4. [`coordination-intelligence.md`](coordination-intelligence.md) — V1 collision, semantic, and cross-device design.
5. [`coordination-harness.md`](coordination-harness.md) — V1 agent lifecycle, context routing, and advisory boundary.
6. [`protocol.md`](protocol.md) — HTTP/event/MCP semantics.
7. [`security-privacy.md`](security-privacy.md) — mandatory controls/prohibited data.
8. [`stickguy-tech-stack.md`](stickguy-tech-stack.md) — stack, repository, tooling.
9. [`open-source-strategy.md`](open-source-strategy.md) — public/private boundary, licensing, release trust, and repository model.
10. [`public-repository-boundary.md`](public-repository-boundary.md) — enforceable placement and prohibited-public-data boundary.
11. [`prebuild-validation.md`](prebuild-validation.md) — executable architecture/adapter assumptions to prove first.
12. [`implementation-plan.md`](implementation-plan.md) — continuous order/exit gates.

Use [`external-references.md`](external-references.md) when implementing third-party contracts; it is informational and lower precedence than Stickguy's own contracts.

Conflict precedence: security/privacy; accepted ADRs; product spec; protocol; architecture/stack; implementation plan. Resolve by updating docs and recording a superseding ADR, never silently in code.
