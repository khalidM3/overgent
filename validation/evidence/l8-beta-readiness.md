# L8 beta readiness evidence

Status on 2026-08-26: repository implementation gate passed; credentialed
publication and real-team exit gates remain owner-blocked.

Implemented evidence:

- Ed25519-signed update metadata; tamper, insecure URL, checksum, replacement,
  and rollback tests in `internal/update`.
- current-user macOS LaunchAgent with `RunAtLoad`, `KeepAlive`, bounded restart
  throttling, explicit argument arrays, and plist safety tests.
- updater validates the new executable identity and installed-service health;
  failure restores and restarts the prior executable.
- diagnostics emits only allowlisted health counts and artifact/config sizes;
  prohibited Project/path/environment/token/output candidates are tested.
- dashboard/API fleet controls distinguish members and devices, authorize from
  current server-side membership, block last-owner-device revocation, revoke
  sessions immediately, and expose owner/member-scoped export and deletion.
- Project and member deletion remove dependent manifests, embeddings,
  deliveries, session messages, memberships, and orphan devices in bounded
  scheduled batches.
- authenticated high-frequency edge routes use a deployment ceiling plus a
  per-credential bucket, avoiding a single honest-fleet rate bucket.

Local checks are recorded in the implementation handoff. Socket/HTTP integration
tests require a runner that permits loopback sockets; the managed workspace
sandbox used for this pass forbids them. The credentialed release workflow,
notarization, clean-machine lifecycle, restart/reconnect soak, and two-person
second session cannot be claimed until the owner supplies the inputs described
in `docs/release.md` and records their results here.
