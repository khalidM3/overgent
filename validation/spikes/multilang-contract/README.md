# Multi-language contract extraction spike

Status: **POSITIVE** on macOS arm64, 2026-08-29.

A time-boxed, disposable spike that answers one question: can Stickguy
fingerprint more than `.go`, `.ts`, and `.tsx` without breaking the CGO-free,
never-invoke-Node boundary that ADR-019 depends on?

The primary hypothesis was tree-sitter grammars compiled to WebAssembly and
executed by [wazero](https://github.com/tetratelabs/wazero). It holds. See
[OUTCOME.md](OUTCOME.md) for the decision and evidence, and
[ADR-DRAFT.md](ADR-DRAFT.md) for the decision record it proposes.

This is not production L-anything. It owns no watcher, queue, service, wire
connection, or manifest pipeline, and it is not wired into
`internal/contract`. The decision comes first.

## Layout

| Path | What it is |
| --- | --- |
| `wasm/guest/shim.c` | The guest half: one bulk pre-order walk, so host/guest crossings are O(1) per file instead of O(1) per node accessor. |
| `wasm/guest/build.sh` | Links the tree-sitter runtime plus a chosen grammar set into one `wasm32-wasi` reactor module. |
| `wasm/ts-multilang.wasm.gz` | The built artifact: runtime plus Python, JavaScript, TypeScript, and TSX, gzipped to 398 KB from 3.75 MB. Checked in so `go test` needs no C toolchain, and stored compressed because that is what the outcome recommends shipping. |
| `tsw/` | A thin wazero binding. Grammars resolved once, guest buffers reused, one bulk call per file. |
| `multilang/` | The extractor: per-language rules plus a copy of `contract.Extract`'s gate/sort/bound/hash tail. |
| `cmd/census/` | Reports how much of a repository the current production extractor actually fingerprints. |
| `evidence/` | Recorded sizes, benchmarks, and the coverage census. |
| `testdata/` | Real source files: Python stdlib, installed npm packages, and this repository's own Go and TypeScript. Provenance and licences in `testdata/README.md`. |

The spike imports `github.com/stickguy/stickguy/internal/contract` through a
`replace` directive, so it measures the production limits and hashing rather
than a reimplementation of them. The one duplicated piece is the unexported
tail of `contract.Extract`, copied into `multilang/assemble.go`; any drift
there is a defect in the spike, not a proposed change.

## Reproduce

Use isolated Go caches so contributor state is untouched:

```sh
GOCACHE=/private/tmp/stickguy-ml-gocache GOMODCACHE=/private/tmp/stickguy-ml-gomod go test ./...
```

```sh
GOCACHE=/private/tmp/stickguy-ml-gocache GOMODCACHE=/private/tmp/stickguy-ml-gomod go vet ./...
```

Latency, against this repository's own Go and TypeScript for comparison:

```sh
GOCACHE=/private/tmp/stickguy-ml-gocache go test ./multilang/ -run XXX -bench BenchmarkExtract -benchtime 300x -count 3
```

One-time runtime cost, and the interpreter fallback:

```sh
GOCACHE=/private/tmp/stickguy-ml-gocache go test ./multilang/ -run XXX -bench 'BenchmarkRuntimeStartup|BenchmarkInterpreterExtract' -benchtime 5x
```

Current production coverage across this repository:

```sh
GOCACHE=/private/tmp/stickguy-ml-gocache go run ./cmd/census ../../..
```

## Rebuilding the wasm module

Only needed to change the grammar set. `go test` uses the checked-in artifact
and needs no C toolchain.

The grammar and runtime C sources come from the vendored `src/` tree of
`github.com/malivvan/tree-sitter@v0.0.1` (tree-sitter v0.24.7, MIT), which
carries 73 upstream grammars. Any tree-sitter checkout with grammar
`parser.c`/`scanner.c` files works the same way.

```sh
ZIG=/path/to/zig TS_SRC=/path/to/tree-sitter/src ./wasm/guest/build.sh /tmp/ts-multilang.wasm python javascript typescript tsx
gzip -9 -c /tmp/ts-multilang.wasm > wasm/ts-multilang.wasm.gz
```

The measured build used zig 0.15.2 as the WASI clang driver. `wasi-sdk`
substitutes directly; the flags are ordinary `clang --target=wasm32-wasi`.

## Result boundary

- PASS: wazero loads and runs a statically linked tree-sitter build and
  produces usable parse trees, with no CGO and no Node. Proven end to end on
  real Python (`dataclasses.py`, `argparse.py`) and real JavaScript (`uri.js`
  ESM, `convertPathData.js` CommonJS), and additionally on TypeScript and TSX.
- PASS: the extractor honors every rule in `internal/contract` — never returns
  an error, treats unparseable and oversized files as "no fingerprint",
  respects `MaxSourceBytes`, `MaxSymbols`, and `MaxSignatureRunes`, and applies
  the `deny` gate before both publication and the file hash.
- MEASURED, NOT FREE: extraction is 33x slower per file than the production
  extractors (7.9 ms versus 0.24 ms on the same 22 KB TypeScript file), and the
  binary grows about 3.7 MB.
- NARROW: only macOS arm64 was executed. wazero's compiler covers
  linux/darwin/freebsd/netbsd/windows on arm64 and amd64-with-SSE4.1; anywhere
  else falls back to the interpreter, which measured 1.01 s for one 64 KB file
  and is not viable. This is a portability cliff, not a cross-compilation
  failure.
- NOT DONE: nothing is wired into `internal/contract`, `internal/app`, the
  wire, or the protocol schemas. The required hosted-side changes are listed in
  OUTCOME.md and ADR-DRAFT.md but were not made.
