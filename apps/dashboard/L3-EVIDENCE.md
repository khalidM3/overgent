# L3 dashboard exit evidence

Status: lane-complete against synthetic, project-isolated fixtures. Backend activation, subscriptions, and mutations remain L2/L4 integration work.

## Delivered behavior

- One-time browser activation view with explicit metadata disclosure and no ticket input, URL persistence, or client-readable credential.
- Responsive hosted dashboard with Project switcher, repository/context revision, live synchronization state, presence, fidelity, active workstreams, path-only summaries, and a quiet 1,000-path large-change summary.
- Finding radar with kind, severity, confidence band, lifecycle, affected workstreams, provenance-labeled evidence, first/last seen, advisory boundary, acknowledge, and resolve interactions.
- Structured activity, synchronous fixture pause/resume state, device/privacy management entry point, and live fixture publication through a subscription-shaped source.
- Honest semantic `degraded` and `disabled` states that explicitly retain structural coverage.
- Intentional activation, loading, empty, ready, offline/stale-revision, unauthorized/no-data, and incompatible-version/no-data states.
- Two synthetic Projects with disjoint identifiers, workstreams, findings, activity, and devices. Unknown fixture Project IDs fail closed.

Fixture shell states are available with `?state=activation|loading|ready|empty|offline|unauthorized|version_mismatch`. They are test-only presentation inputs; no query value grants authorization.

## Verification

Run from the repository root:

```bash
pnpm install --frozen-lockfile
pnpm typecheck
pnpm test
pnpm build
pnpm protocol:check
PLAYWRIGHT_BROWSERS_PATH=/private/tmp/overgent-l3-playwright \
  pnpm --filter @overgent/dashboard test:e2e
```

Observed before commit:

- Dashboard TypeScript: pass.
- Vitest: 2 files, 9 unit/component tests, pass.
- Vite production build: pass; approximately 211 KiB JavaScript and 15 KiB CSS before gzip.
- Playwright: 8 tests across 1440×1000 laptop and 390×844 phone profiles, pass.
- Full-page laptop and phone screenshots were visually inspected: no horizontal clipping; Project controls, findings, evidence, pause, and large-change content remain readable and ordered.

## Accessibility review

- Semantic `main`, `nav`, `aside`, headings, lists, status/alert live regions, button names, `aria-current`, `aria-pressed`, and presence/status labels provide an intelligible hierarchy.
- The device/privacy panel uses native modal-dialog focus containment, focuses a named close button, and closes with Escape; Playwright covers focus and Escape at both widths.
- All interactions are native buttons with visible `:focus-visible`; color is not the sole status signal.
- Reduced-motion preference disables spinner animation.
- Phone layout preserves every control and uses a named device button even when visual profile text is hidden.
- Formal WCAG conformance and assistive-technology matrix testing remain beta hardening work.

## Security and privacy

- Fixtures contain only synthetic identifiers and coordination metadata. They contain no source, diffs, Git objects, prompts, transcripts, environment values, credentials, production data, or raw command/test output.
- Unauthorized and version-mismatch states render no Project names, repository labels, or cached Project content; component and browser tests assert this.
- Project switching retrieves one authorized snapshot by opaque Project ID; unknown IDs fail closed. Tests assert Atlas-only findings never render in Orchard.
- Offline state labels the last synchronized revision and disables pause/finding/live-update mutations.
- Semantic candidate evidence is labeled as probabilistic; it is never presented as authorization or deterministic proof.

## Known limits and integration boundary

- `FixtureProjectSource` is an in-memory L3 adapter. It proves rendering, scoped subscription updates, and interaction state, not hosted persistence or authorization enforcement.
- Activation is presentation-only until L2/L4 wires single-use ticket exchange and secure cookies.
- Pause state is immediate in the UI fixture; L4 must prove the local service stops payload transmission before returning success.
- Device manage buttons are intentionally disabled entry points until revocation/clear APIs are integrated.
- Finding lifecycle changes are local fixture mutations; backend idempotency and authorization remain L2/L4 responsibilities.
- No source contract, generated type, Go core, Convex backend, or legal file was changed in this lane.
