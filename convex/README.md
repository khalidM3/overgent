# Stickguy hosted Project service

This package contains the public Convex backend for the frozen Stickguy `/v1`
HTTP contract. Convex functions are in `functions/`; domain validation and
deterministic helpers are in `src/`.

The service stores coordination metadata only. Event validation rejects source,
diff, patch, Git-object, prompt, transcript, environment, and raw command/test
output fields. Semantic provider activation is intentionally absent at L2.

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
CI=true ./node_modules/.bin/convex dev --tail-logs disable
# In another terminal:
CI=true pnpm test:live
```

`.env.local`, local deployment state, generated Convex bindings, dependencies,
and build output are ignored and must not be committed.
