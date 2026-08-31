# Draft ADR — superseded by the accepted ADR-063

**Accepted 2026-08-29 and merged into `docs/decisions.md` as ADR-063.** This
draft is kept as the spike's own record; the accepted text differs from it in
two ways, both from the review before acceptance:

- it requires the vendored module's inputs, hash and size to be recorded in
  `internal/contract/wasmgrammar/PROVENANCE.md` and asserted by a test, because
  a committed binary that runs on every member's machine cannot be reviewed by
  reading it; and
- it names `convex/functions/schema.ts` — the Convex database schema, silently
  reduced to an empty exported surface by the scanner's `export default` blind
  spot — alongside the two files that yield no fingerprint at all.

Read `docs/decisions.md` for the authoritative text.

---

## ADR-063: Contract extraction runs real grammars in WebAssembly, not hand-written scanners

Contract fingerprints gain languages by linking tree-sitter's runtime and a
chosen grammar set into one standalone `wasm32-wasi` module and executing it
with wazero, a pure-Go runtime. This preserves the CGO-free, never-invoke-Node
boundary ADR-019 rests on: the C toolchain is a build-time dependency producing
a vendored, reviewable `.wasm` blob, and `go build` stays toolchain-free.
Grammars are statically linked because upstream `web-tree-sitter` loads them as
emscripten side modules through `dlopen`, which wazero does not implement; a
grammar set is therefore chosen at build time and compiled lazily per module at
runtime. Measured on macOS arm64: the runtime plus Python, JavaScript,
TypeScript and TSX costs 3.88 MB gzip-embedded on a 23.8 MB binary, of which
3.27 MB is wazero itself; cold start is 80 ms once; and extraction is 33x
slower per file than the existing extractors — 7.9 ms versus 0.24 ms on the
same 22 KB file — which the 20-entry wire batch bounds to roughly 460 ms of
background work per event. Python and JavaScript/JSX are added first. Go stays
on `go/parser`, which fingerprints 137 of 137 files at 73 MB/s and costs
nothing. Migrating `.ts`/`.tsx` off the hand-written scanner is a separate
later decision because it re-baselines every stored fingerprint.

Hand-written scanners are rejected as the expansion path, not merely
deprioritized. The existing 430-line TypeScript scanner already yields no
fingerprint for `convex/src/domain.ts` and `apps/dashboard/test/app.test.tsx`
in this repository, and reusing it for JavaScript recovers zero symbols from
three real JavaScript files — real-world JavaScript is largely CommonJS, which
a token scanner cannot distinguish from an ordinary assignment without scope
tracking. A path that fingerprints to an empty surface is worse than an
unsupported path, because it reads as a stable contract.

wazero's compiler supports linux/darwin/freebsd/netbsd/windows on arm64 and
amd64 with SSE4.1; elsewhere it falls back to an interpreter measured at
1.01 s for one 64 KB file. Outside compiler-supported platforms the service
must report no fingerprint for wasm-backed languages and say so, never degrade
silently to a one-second-per-file extractor. This narrows ADR-019's language
limit and adds a platform condition to it; it does not reverse ADR-019's
CGO-free rule, ADR-044's wire privacy boundary, or ADR-048's fingerprint
semantics. Byte offsets, not source text, cross the wasm boundary, and the
ADR-038 deny gate still applies to every derived signature before publication
and before the file hash. `github.com/odvcencio/gotreesitter`, a pure-Go
tree-sitter reimplementation needing no build toolchain, is the named fallback
if maintaining a WASI build step proves worse than expected; it was rejected
for costing 6.73 MB for the same four grammars and being slower on the largest
file tested. Proposed 2026-08-29 on the evidence in
`validation/spikes/multilang-contract`.

---

## Companion changes required by this ADR

An ADR alone does not lift the limit. The language gate is enforced in two
independent places, one of them hosted.

**Local (Go).**

- `internal/contract/contract.go:60` — `Fingerprintable` returns true only for
  `.go`, `.ts`, `.tsx`. Widen it, and route the new extensions to the wasm
  extractor while `.go` keeps `go/parser`.
- `internal/contract/contract.go` package doc and `internal/contract/typescript.go`
  header both cite ADR-019 as the reason no grammar is linked. Update to cite
  this ADR.

**Hosted (TypeScript).**

- `convex/src/domain.ts:362` — `validateFingerprintablePath` hard-codes
  `/\.(go|ts|tsx)$/i` and throws `path_not_fingerprintable`. It rejects the
  batch independently of anything the Go side does.
- `convex/src/domain.ts:71` — `CONTRACT_SYMBOL_KINDS` must accept every kind
  the extractor emits. The spike emits `reexport` (for `export { … }`,
  `export * from`, `export default`, and `module.exports` members) and
  `namespace`; neither is in the set, so both would be rejected as
  `validation_failed`.

**Protocol (source of truth).**

- `protocol/schemas/event-envelope.schema.json` — `$defs.contractSymbol.kind`
  is the authoritative enum that `domain.ts` mirrors. Add the same kinds there
  first. The `path` field already uses `safePath` and needs no change; the
  language gate lives only in `domain.ts`.
- Regenerate `protocol/generated/go/types.gen.go` and
  `protocol/generated/typescript/schema.d.ts` with `pnpm protocol:generate`,
  then `pnpm protocol:check`. Never hand-edit generated files (AGENTS.md).
- `protocol/fixtures/contract-fingerprints-reported.json` — consider adding a
  non-Go entry so conformance covers a new language kind.

A cheaper alternative exists: map `reexport` to `var` and `namespace` to
`type` and change no schema. It avoids a protocol revision but misdescribes
the surface in stored evidence a human will read in a finding. Prefer widening
the enum.

**Documentation.**

- `docs/protocol.md:68` and `docs/coordination-intelligence.md:43,69` both
  state that only `.go`, `.ts`, and `.tsx` are fingerprinted and both name
  ADR-019 as the cause.

**Ordering.** Protocol schema, then `pnpm protocol:generate`, then the Convex
validators, then the Go extractor, then docs. A Go extractor that emits a kind
the wire has not learned yet stalls a device queue on `validation_failed`.
