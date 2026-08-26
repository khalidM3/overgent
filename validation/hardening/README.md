# L8 hardening suites

`pnpm hardening` runs the deterministic security, migration, reconnect,
updater, service, classifier, protocol, and UI suites with the stateful Go
packages under the race detector.

With the anonymous loopback Convex backend already running, `pnpm
hardening:live` repeats the real HTTP-action suite five times. Override the
bounded iteration count with `STICKGUY_HARDENING_ITERATIONS=20` for a longer
soak. The live suite covers:

- load: interleaved concurrent heartbeats from two device credentials, large
  event batches, and atomic 1,000-path manifests;
- soak: repeated full Project create/publish/export/revoke/delete cycles, each
  with fresh IDs and queue/backend teardown;
- reconnect: network recovery, idempotent event/delivery replay, and managed
  Codex/Claude reconnect rollback tests;
- migration: opening the real older SQLite schema and preserving queued state;
- security: cross-Project access, ticket/invite replay, revoked credentials,
  owner lockout, malformed/oversize payloads, prohibited-content gates,
  retention, exports, and member/Project deletion.

Record the command, iteration count, commit, host architecture, durations, and
peak service/backend memory in `validation/evidence/` for a release candidate.
The loopback suite intentionally refuses a remote Convex origin.
