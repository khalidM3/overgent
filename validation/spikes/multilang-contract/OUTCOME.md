# Outcome — adopt wazero plus statically linked tree-sitter

Recommendation: **one integration unlocks N languages.** Do not write
hand-rolled scanners.

Measured on macOS arm64 (Apple M1), Go 1.26.7, wazero v1.12.0, tree-sitter
v0.24.7 grammars, zig 0.15.2 as the WASI clang driver, 2026-08-29.

---

## 1. Does wazero run tree-sitter, and does it produce a usable tree?

Yes, end to end, with no CGO and no Node.

`go test ./multilang/` parses real source files and derives real fingerprints:

| File | Source | Result |
| --- | --- | --- |
| `dataclasses.py` (64 KB) | CPython 3.13 stdlib | 66 exported symbols; `dataclass`, `field`, `asdict`, `replace`, `Field`, `FrozenInstanceError` present; `_process_class` correctly absent |
| `argparse.py` (101 KB) | CPython 3.13 stdlib | parses clean, fingerprint derived |
| `uri.js` (20 KB) | installed `uri-js` package, ESM | `parse`, `serialize`, `resolve`, `normalize`, `SCHEMES` and 7 more |
| `convertPathData.js` (28 KB) | installed `svgo` package, CommonJS | `name`, `type`, `fn`, `description`, `active`, `params` |

Signatures carry the declaration header only. `dataclass` extracts as
`def dataclass(cls=None, /, *, init=True, repr=True, …)` with no body.

The same integration also handles TypeScript and TSX, which matters below.

## 2. Integration shape

**Both the runtime and the grammars, statically linked into one module.**

This is the load-bearing technical finding. Upstream `web-tree-sitter` compiles
the runtime as an emscripten main module and each grammar as a **side module**
loaded through `dlopen`. wazero does not implement emscripten dynamic linking,
so the npm `tree-sitter-*.wasm` files cannot be loaded as-is. The shape that
works is a single standalone `wasm32-wasi` reactor module containing
`lib.c` plus every grammar's `parser.c`/`scanner.c`, built with any WASI clang
(`zig cc --target=wasm32-wasi-musl -mexec-model=reactor`).

Consequences:

- A grammar set is chosen **at build time**, not at runtime. Grammars are
  embedded selectively by naming them in `wasm/guest/build.sh`.
- Runtime lazy loading is possible only at whole-module granularity: compile
  the module on first use, or ship one module per language. Per-language
  modules cost a duplicated ~94 KB runtime each (4 separate modules: 520 KB
  gzip versus 398 KB gzip combined).
- The C toolchain is a **build-time** dependency producing a vendored `.wasm`
  blob. `go build` and `go test` stay toolchain-free, which is what ADR-019
  actually protects.

### Should an existing library be used instead?

Two exist. Neither should be adopted as-is, but one was reused as a source.

**`github.com/malivvan/tree-sitter`** (MIT, wazero) proves the mechanism and is
where this spike's approach came from. It is not adoptable: v0.0.1, 3 commits,
5 stars, one author, self-described pre-release. More concretely, its binding
`malloc`s a 24-byte node struct per accessor call and never frees it, never
calls `ts_tree_delete`, and crosses the wasm boundary once per node — so a
tree walk leaks and is slow. Its prebuilt `ts.wasm` ships only C and C++. What
is genuinely valuable is its vendored `src/` tree: the tree-sitter runtime plus
73 upstream grammars with a working build recipe. This spike wrote its own
~250-line binding and its own guest shim, and reused only those sources.

**`github.com/odvcencio/gotreesitter`** (MIT) is a pure-Go tree-sitter
reimplementation with 206 grammars, no wasm and no C toolchain at all. It was
tested on the same fixtures and it works: Python, JavaScript, TypeScript, TSX,
and Go all parse cleanly with the same node type names, so this spike's
`rules.go` would port unchanged. It loses on the two axes that matter here:

| | wazero + wasm | gotreesitter |
| --- | --- | --- |
| Binary cost, 4 grammars | **+3.88 MB** (gzip-embedded) | +6.73 MB (`grammar_subset` build tags) |
| Binary cost, all grammars | n/a | +25.1 MB |
| `argparse.py` (101 KB) | **23.0 ms** | 46.5 ms |
| `dataclasses.py` (64 KB) | **13.8 ms** | 15.4 ms |
| `intelligence.ts` (22 KB) | **7.9 ms** | 8.3 ms |
| Build-time C toolchain | required | **none** |
| Parse tables | upstream C, byte-identical | reimplemented; 119 of 206 grammars carry hand-written Go external scanners |

Its no-toolchain story is a real advantage and its API is nicer. But it is a
young, very large, unconventional codebase reimplementing a parser whose
correctness Stickguy would then depend on, it costs nearly twice the binary
for the same four languages, and it is slower on the largest file. **Prefer
the wasm path; keep gotreesitter as the named fallback** if maintaining a WASI
build step proves worse than expected.

## 3. Binary size

The shipped `stickguy` binary is 23,826,706 bytes today.

Linked binary sizes, `-trimpath -ldflags "-s -w"`, darwin/arm64:

| Variant | Bytes | Delta over baseline |
| --- | --- | --- |
| baseline Go program, no parser | 1,641,346 | — |
| + wazero runtime, no grammar blob | 4,911,618 | +3,270,272 |
| + wazero + 4-grammar wasm, embedded raw | 8,695,202 | +7,053,856 |
| **+ wazero + 4-grammar wasm, embedded gzip** | **5,525,746** | **+3,884,400** |

wazero's own code is a fixed **+3.27 MB** and does not compress — it is Go.
Grammar tables compress about 9.4x, so gzip-embedding is the decisive lever:
inflating at startup costs 8.3 ms once and saves 3.17 MB.

Per-grammar cost. The "marginal raw" column is the single-grammar module minus
the runtime-only module; gzip is not additive, so gzip is reported only for
whole modules that were actually built and compressed.

| Module | Raw | Gzip | Marginal raw grammar cost |
| --- | --- | --- | --- |
| runtime only, no grammar | 93,989 | 36,557 | — |
| runtime + go | 302,335 | 71,448 | 208,346 |
| runtime + javascript | 455,339 | 81,489 | 361,350 |
| runtime + java | 503,697 | 84,327 | 409,708 |
| runtime + python | 548,441 | 99,693 | 454,452 |
| runtime + rust | 1,139,475 | 144,258 | 1,045,486 |
| runtime + typescript | 1,507,024 | 168,025 | 1,413,035 |
| runtime + tsx | 1,539,211 | 171,323 | 1,445,222 |
| runtime + ruby | 2,220,420 | 200,369 | 2,126,431 |

Multi-grammar modules, as built:

| Module | Raw | Gzip |
| --- | --- | --- |
| python + javascript | 909,158 | 144,325 |
| python + javascript + typescript + tsx | 3,752,029 | 398,165 |
| python + javascript + rust + java + ruby | 4,479,689 | 455,291 |

Grammars are near-perfectly additive in raw bytes: the five-grammar module is
4,479,689 bytes against 4,490,783 predicted by summing the marginal costs, a
0.2% difference. There is essentially no sharing to exploit, which is what
makes selective embedding worth doing.

**Answer: the runtime plus 4–5 grammars costs about 3.9 MB gzip-embedded,
roughly +16% on a 23.8 MB binary, of which 3.27 MB is wazero itself and only
about 400–460 KB is the grammars.** Grammars can be selected at build time and
compiled lazily at runtime per module. Adding Python and JavaScript alone
(144 KB gzip) would cost about 3.4 MB.

## 4. Extraction latency

`go test -bench BenchmarkExtract -benchtime 300x -count 3`, best of three:

| Extractor | File | Bytes | ns/op | MB/s |
| --- | --- | --- | --- | --- |
| production `contract.Extract` (go/parser) | `internal/contract/typescript.go` | 11,832 | 161,688 | 73.18 |
| production `contract.Extract` (TS scanner) | `packages/coordination/src/intelligence.ts` | 22,055 | 238,533 | 92.46 |
| **wasm tree-sitter** | **same `intelligence.ts`** | 22,055 | **7,520,554** | **2.93** |
| wasm tree-sitter | `uri.js` | 20,153 | 5,075,142 | 3.97 |
| wasm tree-sitter | `convertPathData.js` | 28,389 | 7,730,631 | 3.67 |
| wasm tree-sitter | `dataclasses.py` | 64,545 | 13,752,714 | 4.69 |
| wasm tree-sitter | `argparse.py` | 101,648 | 22,969,305 | 4.43 |

**On the same file, tree-sitter under wasm is 33x slower than the hand-written
scanner** — 7.9 ms versus 0.24 ms. This is inherent to running a real parser
under a wasm JIT, not to the binding: the guest shim keeps host/guest crossings
at a fixed handful per file, and the Go side allocates 20 KB / 304 allocations
for a 64 KB Python file versus the scanner's 218 KB / 4,734 allocations for a
22 KB TypeScript file.

Whether 33x matters depends on batch size, and the wire already bounds it: a
`workspace.contract_fingerprints_reported` event carries at most 20 entries, so
a full batch costs roughly 160–460 ms of background work. That is acceptable
for a long-lived per-user service (ADR-003). It would not be acceptable on an
interactive path.

One-time costs, paid per process:

| | Time |
| --- | --- |
| gzip inflate of the 4-grammar blob | 8.3 ms |
| wazero compile of the module | 71.8 ms |
| **total cold start** | **~80 ms** |
| interpreter-mode instantiate (no compile) | 11.0 ms |

**Interpreter mode is not viable.** One 64 KB Python file took 1.01 s
(0.06 MB/s, 1.46 M allocations) — 73x slower than compiler mode. wazero's
compiler covers linux/darwin/freebsd/netbsd/windows on arm64 and amd64 with
SSE4.1. Everything outside that set silently degrades to unusable. This is a
portability cliff that ADR-050's Apple Silicon scope currently hides.

## 5. Can the existing contract be honored?

Yes. `TestContractRulesHold` proves each rule against the production
constants:

| Rule | Result |
| --- | --- |
| never returns an error | the API returns `(File, bool)`; every wasm failure path maps to `false` |
| no grammar for the path | no fingerprint |
| source over `MaxSourceBytes` (1 MiB) | no fingerprint |
| source exactly at `MaxSourceBytes` | still extracted |
| unparseable source (Python and JavaScript) | no fingerprint, via `ts_node_has_error` on the root |
| `MaxSymbols` (200) | 600 declarations truncate to exactly 200 |
| `MaxSignatureRunes` (500) | a 400-parameter signature truncates to exactly 500 runes ending in `…` |
| `deny` wire gate | denied symbols omitted from both the symbol list and `fileContractHash`; the gated and ungated hashes differ |
| determinism | repeated extraction of the same file yields the same hash |

Two mechanisms carry this. The guest reports `sizeof(SGRecord)` so a layout
drift fails at startup rather than silently misreading. The bulk dump returns
the count it *would* have written, so a buffer overrun is detected and becomes
"no fingerprint" rather than a truncated, wrong answer.

---

## What the spike found that it was not looking for

### The production TypeScript scanner already fails on this repository

`go run ./cmd/census ../../..` over the whole repository:

| Extension | Files | Fingerprinted | Symbols |
| --- | --- | --- | --- |
| `.go` | 137 | **137** | 1,225 |
| `.ts` | 44 | **42** | 225 |
| `.tsx` | 8 | **7** | 14 |
| `.mjs` | 13 | 0 | 0 |

(The census excludes this spike's own directory, so its testdata is not
counted as repository source. The 13 `.mjs` files carry no contract today.)

The three fingerprintable files that yield nothing:

- `internal/contract/testdata/unparseable.ts` — intentional.
- `apps/dashboard/test/app.test.tsx`
- **`convex/src/domain.ts`** — the file that contains
  `validateFingerprintablePath` itself.

Both real failures contain regular-expression literals, which `typescript.go`
documents as a desynchronizing case. `TestScannerBlindSpotsAreRepaired`
confirms the same integration repairs them: tree-sitter recovers **39 symbols
from `domain.ts` where the scanner recovers none**, and parses `app.test.tsx`
cleanly (0 exported symbols, which is correct for a test file, and is
different from "no fingerprint"). On `intelligence.ts`, which the scanner does
handle, tree-sitter is a strict superset: 30 symbols including all 25 the
scanner found.

### The stated fallback recovers nothing on real JavaScript

The fallback plan assumed `.js`/`.jsx` would largely reuse `typescript.go`
because the scanner is token-based rather than type-aware.
`TestFallbackJavaScriptThroughTheTypeScriptScanner` fed real JavaScript to the
production scanner under a `.ts` path:

| File | Scanner | Tree-sitter |
| --- | --- | --- |
| `uri.js` (ESM) | **no fingerprint at all** | 12 symbols |
| `convertPathData.js` (CommonJS) | fingerprint, **0 symbols** | 6 symbols |
| `_path.js` (CommonJS) | fingerprint, **0 symbols** | 3 symbols |

Zero symbols on all three. Two reasons: real JavaScript is largely CommonJS
(`exports.x = …`, `module.exports = { … }`), which the scanner has no rule for
and which cannot be distinguished from an ordinary assignment without scope
tracking; and files with regex literals desynchronize the scan entirely.

Widening `Fingerprintable` to `.js` would therefore ship a language that
produces no contract evidence — worse than not shipping it, because a path
that fingerprints to an empty surface looks like a stable contract rather than
an unsupported one. The fallback is not "largely free"; it is a new CommonJS
rule set plus regex-literal tokenization added to a 430-line scanner that
already mis-handles two files in this repository. Python has no reuse path at
all and would be a second scanner from scratch, in a language whose block
structure is indentation-sensitive.

---

## Recommendation

**Adopt wazero plus a statically linked tree-sitter build**, sequenced:

1. **Now — Python and JavaScript/JSX.** The two cheapest grammars
   (144 KB gzip together) plus the fixed 3.27 MB wazero cost. This is the
   language expansion the spike was asked to price.
2. **Next, as a separate decision — migrate `.ts`/`.tsx` off the scanner.**
   Evidence says the scanner is losing real files today. This is a
   fingerprint-format change: the two extractors normalize headers differently,
   so hashes differ and every stored fingerprint must be re-baselined. It also
   adds 266 KB gzip and, unlike step 1, it changes behavior for paths that
   already work. Sequence it after step 1, not with it.
3. **Keep Go on `go/parser`.** 147 of 147 files, 73 MB/s, zero added bytes.
   There is no case for moving it.

**Reject the hand-written fallback.** It was priced on an assumption the
evidence contradicts.

### Required changes outside `internal/contract`

The limit is enforced in two places and the second one is hosted:

| Location | Change |
| --- | --- |
| `internal/contract/contract.go:60` | widen `Fingerprintable`; route non-Go extensions to the wasm extractor |
| `convex/src/domain.ts:362` | `validateFingerprintablePath` hard-codes `/\.(go\|ts\|tsx)$/i` and throws `path_not_fingerprintable` |
| `convex/src/domain.ts:71` | `CONTRACT_SYMBOL_KINDS` — the spike emits `reexport` and `namespace`, which are **not** in the set and would be rejected as `validation_failed` |
| `protocol/schemas/event-envelope.schema.json` | `$defs.contractSymbol.kind` enum, same two additions; this is the source of truth, `domain.ts` mirrors it |
| `protocol/generated/go/types.gen.go`, `protocol/generated/typescript/schema.d.ts` | regenerate only via `pnpm protocol:generate`; never hand-edit (AGENTS.md) |
| `docs/protocol.md:68`, `docs/coordination-intelligence.md:43,69` | both state "only `.go`, `.ts`, and `.tsx`" and name ADR-019 as the reason |

`protocol/schemas/event-envelope.schema.json` already uses `safePath` for the
path itself, so no path-pattern change is needed there — the language gate
lives only in `domain.ts`.

Alternatively, map `reexport` to `var` and `namespace` to `type` and change no
schema at all. That is cheaper but lies about the surface; prefer widening the
enum.

### What this does not change

Nothing here touches the six language-agnostic finding kinds
(`direct_collision`, `redundant_work`, `shared_dependency`,
`assumption_conflict`, `dependency_ready`, and the declared-contract path of
`downstream_impact`). This is strictly about extracted contract fingerprints
and therefore strictly about `stale_assumption` and the extracted path of
`downstream_impact`.

The privacy boundary is unchanged. Byte offsets, not source text, cross the
wasm boundary; the guest never returns source; and the `deny` gate is applied
to every derived signature before publication, as `TestContractRulesHold`
proves.

### Open risks to carry into implementation

- **The 33x latency** is real and permanent. Bound it: cap concurrent
  extractions, keep the 20-entry wire batch, and never put extraction on an
  interactive path.
- **The interpreter cliff.** Gate on `wazero`'s compiler support at startup and
  degrade to "no fingerprint for wasm languages" rather than to a 1-second-per-
  file extractor. Honest fidelity (AGENTS.md) requires the degraded state be
  visible, not silent.
- **The WASI build step** becomes a release dependency. The `.wasm` is a
  vendored, reviewable artifact, but it needs a reproducible build and a
  documented provenance for the grammar sources.
- **Python has no `export` keyword.** This spike uses the leading-underscore
  convention and ignores `__all__`, so a module that narrows its surface with
  `__all__` will over-report. Decide that explicitly before shipping Python.
