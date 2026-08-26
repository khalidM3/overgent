# Lane D — M6 sharing simplification (deletion lane)

Goal: implement ADR-047. Project membership plus the pause switch becomes the
sharing consent model. The per-session consent ceremony (consent records,
preview flows, versioned consent schemas, share toggles) is **deleted, not
hidden**. The secret classifier and pause remain untouched hard gates.

This is primarily a deletion/simplification lane — mechanical, wide, and
low-judgment. When something looks load-bearing beyond consent, stop and
flag it in the handoff instead of improvising.

## Read first

- ADR-047 (and ADR-034/036/038 it supersedes/preserves) in
  `docs/decisions.md`
- `internal/sessiontranscript/` — session detail reading and the share
  classification pipeline
- `internal/agentactivity/` — activity profiles/opt-in state
- `convex/functions/service.ts`, `schema.ts` — consent/session-share tables
  and authorization
- `apps/dashboard/src/` — consent/preview/share UI surfaces
- Find the full surface with: `grep -ri "consent" --include="*.go"
  --include="*.ts" --include="*.tsx" -l` and the same for `session-share`
  and `preview` (filter noise by reading matches).

## What changes

1. **Delete** per-session consent records, consent versioning, audience
   selection, expiry, and the side-effect-free preview flow — code, tables/
   fields, API surface, MCP surface (if any), dashboard UI, and their tests.
   Convex schema: follow the existing migration conventions found in the
   repo for removing/retiring tables.
2. **Flip the gate.** Wherever enqueue/projection of session content or
   activity previously required an active consent record, the condition
   becomes: member is enrolled in the Project AND sharing is not paused AND
   the candidate passes the secret classifier. Activity sharing already
   follows adapter connection (ADR-033) — remove any residual per-profile
   opt-in ceremony beyond connecting the adapter.
3. **Session detail** (ADR-036 local transcript reading) stays: the owner
   always sees their own sessions. Its *projection to the Project* now
   follows the flipped gate above instead of a share toggle.
4. **Keep, with tests green and unmodified semantics:**
   - the secret classifier (ADR-038: environment assignments, credentials,
     tokens, private keys, raw tool results, command output — whole-message
     rejection, no redaction),
   - the synchronous pause switch,
   - hosted authorization/retention/deletion of shared rows (member and
     owner can still delete shared messages).
5. **Protocol:** removal-only edits to `protocol/openapi.yaml`/`schemas/`
   for deleted surfaces are allowed in this lane (exception to Lane B's
   ownership because it is delete-only). Run `pnpm protocol:generate` and
   commit. The integrator resolves any overlap with Lane B at merge and
   regenerates.
6. **Docs:** update `docs/security-privacy.md` and
   `docs/agent-activity-sharing.md` to describe the ADR-047 model. Delete
   sections describing the removed ceremony rather than annotating them.

## Acceptance criteria

1. `grep -ri` for consent-version/preview-flow/share-toggle identifiers
   returns nothing outside `docs/decisions.md` history and this brief.
2. Loopback test: enroll a member, connect an adapter, run a session — the
   session's activity and (classifier-passing) detail are visible to another
   Project member with zero additional consent steps.
3. Pause test: pausing stops new shared content synchronously; classifier
   tests all pass unmodified.
4. Deletion test: a member deletes a shared message; it is gone for the
   other member.
5. All standard checks pass, including `pnpm protocol:check` after
   regeneration.

## Out of scope

Anything additive: no new findings, no injection, no protocol additions, no
UI redesign. Do not touch `internal/contract` (doesn't exist yet — Lane B),
`internal/hookconfig` behavior, or the eval suite.
