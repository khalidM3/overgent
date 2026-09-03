# Lane H — L8 fleet management, data rights, and hardening

Goal: a real team can run Overgent without asking an engineer for help —
manage who is in a Project and which devices are trusted, get their data out
or delete it, and trust that the system survives load, restarts, and network
loss.

## Read first

- `docs/implementation-plan.md` (L8), `AGENTS.md`
- ADR-035 (member identity vs device identity), ADR-047 (membership is the
  sharing consent), ADR-023/024 (dashboard tickets and activation) in
  `docs/decisions.md`
- `docs/security-privacy.md`
- `convex/functions/service.ts` and `http.ts` — membership, devices, invites,
  retention, and the edge authorization/rate/size guards
- `apps/dashboard/src/` — the existing Settings surfaces, including
  Devices & security
- `docs/coordination-intelligence.md` §9 — a rate-limit defect class that
  matters for your load tests: several edge routes are keyed to one
  deployment-wide bucket rather than per caller

## Decisions already made — do not revisit

- **Members and devices are different things** (ADR-035). A member is a person
  with a chosen display name; a device is hardware that holds a credential and
  can be revoked independently. Management surfaces must not conflate them.
- **Removal is real.** Removing a member or revoking a device takes effect on
  the next request, not eventually: revoked credentials fail closed, and a
  removed member stops receiving briefs and stops appearing in coordination.
  Prove it with a test that a revoked device's next call is rejected.
- **Export and deletion are the member's right, not an admin favor.** A member
  can export everything Overgent holds about their own work and can delete it.
  A Project owner can delete Project-scoped data. Deletion removes the rows;
  it does not hide them behind a flag.
- **The owner cannot lock themselves out.** The last owner of a Project cannot
  remove themselves or revoke their last device without an explicit transfer.

## Deliver

1. **Member, device, and invite management** in the dashboard and through the
   API: list, invite, revoke an invite, remove a member, rename yourself,
   revoke a device, and see when each device was last active. Every mutation
   authorizes server-side against current membership, never against a
   client-supplied role.
2. **Export and deletion** covering coordination records, activity, session
   content, findings, and semantic objects, with a documented retention story
   that matches what `docs/security-privacy.md` claims.
3. **Hardening tests** — this is the half most likely to be skimped, so treat
   it as the deliverable it is:
   - *Load*: many workstreams and devices in one Project publishing
     concurrently. Watch specifically for the shared-rate-bucket defect class
     described in `docs/coordination-intelligence.md` §9 — if a route throttles
     a whole deployment instead of a caller, that is a bug, not a limit.
   - *Soak*: a long run that proves queues drain, retention jobs run, and
     memory and disk do not grow without bound.
   - *Reconnect*: network loss mid-publish queues locally and flushes exactly
     once on recovery, with no duplicated activity.
   - *Migration*: an older local SQLite state and an older hosted schema
     migrate forward without data loss. Test the real upgrade path, not a
     fresh install.
   - *Security*: cross-project access fails, revoked credentials fail, forged
     or replayed tickets fail, oversize and malformed payloads are rejected,
     and prohibited data (source, diffs, prompts, transcripts, credentials,
     tokens, environment values, command output) cannot reach durable storage
     through any route you can find.
4. Record evidence in `validation/evidence/` the way earlier levels did.

## Acceptance criteria

- Every management action above works from the dashboard against a real
  loopback stack, with server-side authorization tests for each.
- Revocation and removal take effect on the next request, proven by test.
- Export produces the member's data; deletion removes it; both are tested.
- The five hardening test categories exist, run, and pass, with results
  recorded as evidence rather than asserted in prose.
- No critical data loss in restart, network-loss, or migration tests.
- All standard checks: `go test ./...`, `go vet ./...`, `pnpm typecheck`,
  `pnpm test`, `pnpm build`, `pnpm protocol:check`.
- `pnpm eval:coordination` still passes all seven scenarios (needs Node 22+:
  `export PATH="$HOME/.nvm/versions/node/v22.23.2/bin:$PATH"`).

## Notes

- You own `protocol/` this round if your work needs a wire change; Lane G will
  not touch it. Edit `protocol/openapi.yaml` and `protocol/schemas/` only, then
  run `pnpm protocol:generate`; never hand-edit generated code.
- Lane G is running in parallel on distribution and updates. You will both
  touch `cmd/overgent/main.go` and the dashboard; keep changes in their own
  commands and components so the merge stays mechanical.
- The eval suite cannot run concurrently from two checkouts — its Convex
  backend binds fixed ports. Coordinate with the other lane or run it when the
  other is idle, and never kill a backend your run did not start.

## Out of scope

Signing, notarization, installers, the updater, and OS service lifecycle —
Lane G owns those.
