# L0 scaffold evidence

Status: PASS
Observed: 2026-08-23 on macOS arm64

## Delivered behavior

- Go 1.26 module and single-binary command scaffold with JSON version/schema compatibility output.
- pnpm workspaces for the React dashboard, Convex hosted boundary, provider/index/router contracts, and cross-language protocol conformance.
- OpenAPI 3.1 `/v1` operations and bounded JSON Schema 2020-12 contracts for events, manifests, semantic objects, findings, briefs, checkpoints, and verification.
- Pinned, deterministic dereference/generation pipeline with Go and TypeScript generated outputs and isolated byte-for-byte drift detection.
- One synthetic manifest-completion fixture validated against the same event schema by Go and TypeScript, then decoded through the generated Go event-batch type.
- Provider-neutral embedding/index/router interfaces; deterministic structural-first router; structural fidelity when semantic dependencies are absent.
- Canonical Go package boundaries, privacy-safe `slog` construction, redacted secret log values, and a source-free synthetic producer.
- Least-privilege CI across Go 1.26/1.27, dependency update configuration, and a draft provenance/SBOM/checksum release workflow that fails closed unless legal files exist.
- Public/private repository placement boundary plus contribution, conduct, and private security-reporting policies.

## Verification

The following commands passed from the working tree:

```text
go test ./...
go vet ./...
pnpm install --frozen-lockfile
pnpm protocol:check
pnpm typecheck
pnpm test
pnpm build
```

The Go protocol test also compiles every JSON Schema before validating the fixture. The TypeScript test validates the same fixture with AJV 2020 and date-time formats. The release workflow was inspected only; it was not published or granted credentials.

A deliberate comment was inserted into the generated Go type as a negative probe. `pnpm protocol:check` exited nonzero and named that file as drift. `pnpm protocol:generate` restored it, after which the check passed.

Commit `844cd86` was cloned with `--no-local` into `/private/tmp/stickguy-l0-clean-844cd86`. A frozen pnpm install and the documented Go, protocol, typecheck, test, and build commands all passed there, and the clone remained Git-clean after ignored build output was produced.

## Security and privacy review

All fixtures are synthetic. No production credentials or example secret values are committed, and no source/diff/Git-object/transcript/prompt/environment/raw-command payload appears in a public event. Semantic interfaces require both Project and repository scope; hosted fidelity is structural when providers are absent. Release permissions exist only in the tag/manual release job and the job refuses to run without `LICENSE` and `NOTICE`.

## Outcome and next gate

The owner accepted Apache-2.0 and the initial `Copyright 2026 Stickguy contributors` attribution on 2026-08-23. The unmodified license and literal notice are committed, satisfying the remaining L0 implementation gate. A real private security-reporting and conduct-enforcement channel remains mandatory before public launch; no unmonitored address is advertised. L1–L3 are now unblocked under their documented parallel ownership lanes.
