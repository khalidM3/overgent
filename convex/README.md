# Stickguy hosted Project service

This package contains the public Convex backend for the frozen Stickguy `/v1`
HTTP contract. Convex functions are in `functions/`; domain validation and
deterministic helpers are in `src/`.

The service stores coordination metadata only. Event validation rejects source,
diff, patch, Git-object, prompt, transcript, environment, and raw command/test
output fields. L6 adds a bounded deterministic concept provider, separate
vectors, scoped retrieval, revision-safe reload, and structural fallback; it
does not send summaries to a third-party model provider.

## Verification

```bash
pnpm --filter @stickguy/hosted typecheck
pnpm --filter @stickguy/hosted test
pnpm --filter @stickguy/hosted build
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
exit cases.

`.env.local`, local deployment state, generated Convex bindings, dependencies,
and build output are ignored and must not be committed.
