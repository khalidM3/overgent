# L4 deterministic vertical-slice evidence

Date: 2026-08-23

## Outcome

PASS on the supported macOS/loopback validation path, with the deployment and platform limits below carried forward. This is the first dogfoodable deterministic slice; it does not claim L5 adapter or L6 semantic-intelligence completion.

## Delivered behavior

- `create` and `join` preflight a real Git repository, create or enroll a device, store its credential in macOS Keychain, persist stable local IDs/configuration, and register a workspace without source or diff transfer.
- The one per-user Go service publishes complete schema-v1 envelopes through the frozen `/v1` client, acknowledges every ID before local cleanup, retries outages with capped exponential backoff plus jitter, and emits separate 15-second presence health.
- `stickguy intent` durably queues bounded manual intent; `pause` immediately prevents activity-payload transmission while allowing only minimal paused connection health.
- `stickguy dashboard --project` mints a ticket with the Keychain-backed credential and opens a nonce-only loopback handoff. The escaped hidden form POSTs to `/v1/dashboard-activations`; the ticket is absent from argv, URLs, page inputs, browser storage, logs, and retained evidence.
- Hosted activation consumes the ticket once, stores only session/ticket hashes, sets `Secure; HttpOnly; SameSite=Strict`, and redirects without the ticket. Browser session and snapshot reads reauthorize the session's member and one Project on every request.
- The React live source uses credentialed same-origin reads and two-second bounded refresh. It renders real workstreams, presence, path counts/examples, structural findings, activity, devices, context revision, and explicit failure states. Fixture-only mutations are not shown as live operations; local pause remains a CLI action.

## Executable proof

The disposable two-device test uses two config roots, two real Git repositories sharing one sanitized remote, two Go services, and the anonymous loopback Convex deployment. Device B is forced offline, both devices report intent and the same path, B reconnects, its queue drains once, and the hosted service produces one deterministic direct-collision finding. The test then uses the creator ticket in the top-level form activation route, verifies the cookie flags, reads the authorized browser session/snapshot, and observes exactly two workstreams, two devices, and at least one finding. It revokes the originating device and proves the same browser cookie immediately receives `401`, rather than retaining stale privilege.

Final live command:

```text
STICKGUY_L4_SITE_URL=http://127.0.0.1:3211 go test -count=1 -timeout=60s -v ./internal/integration -run TestL4TwoDeviceGoToHostedVerticalSlice
PASS; 1.49s test time
```

Disposable Keychain proof:

```text
STICKGUY_KEYCHAIN_LIVE=1 go test -count=1 -timeout=20s -v ./internal/credential -run TestKeychainRoundTrip
PASS
```

The Keychain adapter supplies the synthetic password only through a PTY's two non-echoed prompts and drains the PTY before waiting. The test verifies the exact value and deletes its unique item. A disposable interactive prompt diagnostic item was also explicitly deleted.

Final frozen checks:

```text
go test ./...
go vet ./...
go test -race ./...
CI=true pnpm install --frozen-lockfile
CI=true pnpm typecheck
CI=true pnpm test
CI=true pnpm build
CI=true pnpm protocol:check
```

All pass. Dashboard unit coverage is 10 tests, including credentialed live-session transport. Hosted unit coverage is 13 tests. The loopback Convex compiler reports functions ready.

## Security and privacy review

- No source, diffs, Git objects, raw transcripts, prompts, environment values, or raw command/test output enter event payloads or browser snapshots.
- Hosted URLs require HTTPS except explicit loopback validation. HTTP bodies and responses are bounded; public operations authenticate or consume a single-use ticket, authorize Project scope, validate exact shapes, and apply rate guards.
- Keychain secrets are never command arguments. Dashboard tickets are never device credentials and remain single-use/short-lived; browser sessions are hashed, expiring, Project-scoped, and cookie-only.
- The loopback activation listener binds `127.0.0.1` on an ephemeral port, uses an unpredictable path, serves `no-store`/`no-referrer`/CSP headers, and contains no general local API.
- Test repositories/configuration use disposable roots. Retained evidence contains identifiers, outcomes, timings, and counts only—no credential, ticket, session, URL value beyond the documented loopback origin, source content, or private user path outside the repository.

## Accepted decisions and honest limits

- ADR-024 selects device-initiated hidden-form POST activation and same-origin cookie sessions; it preserves ADR-023's prohibition on dashboard ticket inputs and ticket URLs.
- macOS is the only native runtime proven. Other OS activation/service/credential adapters continue to fail closed per ADR-019.
- The local validation redirect targets the Vite dashboard on loopback. Production same-origin static routing, DNS/TLS, installers, notarization, and release automation remain later release gates and were not inferred from loopback success.
- Live dashboard refresh is bounded two-second polling, not a Convex browser subscription. It meets the deterministic L4 visibility bound; transport optimization can occur without changing the public snapshot contract.
- Dashboard pause/finding/device mutations remain disabled in the live browser because their trusted local/hosted command paths are not yet contracted. Immediate pause is proven through local IPC/CLI.
- Optional semantic providers remain absent; the dashboard honestly reports semantic processing disabled while structural findings remain active.
