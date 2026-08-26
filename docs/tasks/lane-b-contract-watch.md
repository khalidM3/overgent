# Lane B — M2 read sets, contract fingerprints, stale-assumption findings

Goal: deterministic detection of "the contract your session read has changed
since you read it," end to end: local extraction → sync as derived facts →
hosted comparison → `stale_assumption` finding → brief/radar rendering.

This lane **owns all protocol changes** for the V2 lanes. No other lane may
touch `protocol/`.

## Read first

- ADR-044 and ADR-048 in `docs/decisions.md`
- `internal/manifest/`, `internal/watcher/`, `internal/git/` — how path
  change state is observed and published today (ADR-017/022 model)
- `internal/agentactivity/` — how hook events carry safe repository-relative
  paths and tool categories; this is the read-set source
- `internal/events/`, `internal/store/`, `internal/sync/` — the durable queue
  and envelope model
- `protocol/openapi.yaml`, `protocol/schemas/`, `docs/protocol.md` — contract
  conventions; `pnpm protocol:generate` is the only generation path
- `convex/functions/intelligence.ts`, `schema.ts` — finding upsert and brief
  candidate pipeline (L6)

## Design (decided — do not revisit)

### Contract fingerprints (new package `internal/contract`)

- Per-file extraction of **exported** API surface only:
  - **Go**: stdlib `go/parser`/`go/ast`. Exported functions, methods, types,
    struct fields, interface methods, consts, vars. Signature text is the
    normalized declaration without body or comments.
  - **TypeScript/TSX**: pure-Go bounded scanner (no CGO, no Node invocation —
    ADR-019 keeps the root module pure Go). Recognize top-level `export`
    declarations (`function`, `class`, `interface`, `type`, `const`, `enum`)
    and capture the declaration header up to body start. Best-effort is
    acceptable; document limitations in the package doc. Do not add a parser
    dependency without flagging it in the handoff first.
- Shapes:
  - symbol: `{ name, kind, signature (normalized, max 500 chars, truncated
    with marker), signatureHash (sha256 of normalized signature) }`
  - file fingerprint: sorted symbol list plus `fileContractHash` = sha256
    over the sorted `name:signatureHash` pairs.
- Only files with extensions `.go`, `.ts`, `.tsx` are fingerprinted; others
  have no fingerprint and never produce contract findings.
- Extraction runs where the manifest pipeline already processes changed
  paths; a parse failure yields "no fingerprint" for that file (never an
  error that blocks manifest publication).

### Wire contracts (this lane adds them to `protocol/`)

- Event `contract_fingerprints/v1`: `{ workspaceId, entries: [{ path,
  fileContractHash, symbols: [symbol] }] }`, bounded entry count per event
  consistent with existing chunking conventions.
- Event `read_set/v1`: `{ workspaceId, sessionWorkstreamId, entries:
  [{ path, fileContractHashAtRead, observedAt }] }`. Deduped locally: one
  entry per (session, path), re-observation updates the hash and time.
- Finding kind `stale_assumption` with evidence `{ path, changedSymbols:
  [{ name, oldSignature, newSignature }], changedByWorkstreamId,
  readAt, changedAt }`. Reuse the existing finding contract's
  kind/severity/evidence extension pattern.

### Read-set capture

A session's read set is fed from existing agent-activity events whose tool
category is a read/inspection over a safe repository-relative fingerprintable
path, plus the paths consumed at `begin_work` when the MCP client reports
them. At capture time the local service records the file's current
`fileContractHash` (compute on demand if not cached).

### Hosted comparison (Convex)

- Store latest fingerprints per (project, repository, path) with revision;
  store read-set entries per live session workstream.
- On fingerprint change: for every *other* live session in the same
  project/repository whose read set contains the path with
  `fileContractHashAtRead != new hash`, diff the symbol lists and upsert one
  `stale_assumption` finding per (session, path, new hash) — idempotent, no
  duplicates on redelivery. Body-only changes (same `fileContractHash`)
  produce nothing.
- Findings flow into the existing L6 finding/brief/radar pipeline untouched;
  severity: high when a symbol the session read changed signature, none when
  only symbols were added.

## Acceptance criteria

1. Two-workstream loopback integration test (follow the existing L6 live
   suite pattern): WS2 changes an exported Go signature read by WS1 →
   exactly one `stale_assumption` finding for WS1 naming the symbol with
   old/new signature; WS1's brief contains it.
2. Same test with a body-only edit → zero contract findings.
3. Same test where WS2 only adds a new exported symbol → no high-severity
   finding for WS1.
4. TS extractor unit tests over fixture files covering each recognized
   declaration form plus one unparseable file (yields no fingerprint,
   no error).
5. `pnpm protocol:generate` then `pnpm protocol:check` pass; generated code
   committed; all standard checks pass.
6. Update `docs/coordination-intelligence.md` with a short section on
   contract evidence.

## Out of scope

Injection/delivery (Lane C), LLM judgment (M4), `waiting_on` (M5), transcript
or consent code (Lane D), languages beyond Go/TS.
