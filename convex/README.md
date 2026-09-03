# Overgent hosted Project service

This package contains the public Convex backend for the frozen Overgent `/v1`
HTTP contract. Convex functions are in `functions/`; domain validation and
deterministic helpers are in `src/`.

The service stores coordination metadata only. Event validation rejects source,
diff, patch, Git-object, prompt, transcript, environment, and raw command/test
output fields. L6 adds a bounded deterministic concept provider, separate
vectors, scoped retrieval, revision-safe reload, and structural fallback; it
uses the deterministic provider by default. When `OPENAI_API_KEY` is configured
on the deployment, approved bounded summaries are also embedded asynchronously
with OpenAI and provider failure preserves the deterministic path.

## Verification

```bash
pnpm --filter @overgent/hosted typecheck
pnpm --filter @overgent/hosted test
pnpm --filter @overgent/hosted build
```

The live suite requires an anonymous local Convex development deployment and
refuses non-loopback URLs:

```bash
cd convex
CI=true ./node_modules/.bin/convex dev --tail-logs disable --typecheck enable
# In another terminal:
CI=true pnpm test:live
```

The live suite retains every L2 regression assertion and adds the L6 two-device
duplicate-behavior, pre-edit assumption conflict, scoped shared dependency,
unrelated routing, staleness, cross-Project vector isolation, and radar-feedback
exit cases. It also covers the M2 contract-watch loop: a changed exported
signature invalidates another live session's read set and reaches that session's
brief, while a body-only edit, a redelivered fingerprint, and a newly added
exported symbol raise nothing.

`.env.local`, local deployment state, generated Convex bindings, dependencies,
and build output are ignored and must not be committed.
