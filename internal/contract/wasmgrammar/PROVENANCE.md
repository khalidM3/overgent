# Grammar module provenance

`ts-multilang.wasm.gz` is a compiled binary committed to a public repository and
executed on every member's machine as part of contract extraction (ADR-063).
A binary blob cannot be reviewed by reading it, so every input is pinned to an
exact commit here and the artifact's hash is asserted by a test. Anyone can
rebuild it and compare.

## Pinned inputs

| Input | Version | Commit |
| --- | --- | --- |
| `tree-sitter` runtime | v0.26.13 | `d97971e24500218865c05ed1febdee2acf41bae1` |
| `tree-sitter-python` | HEAD 2026-08-29 | `26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64` |
| `tree-sitter-javascript` | HEAD 2026-08-29 | `58404d8cf191d69f2674a8fd507bd5776f46cb11` |
| `tree-sitter-typescript` (typescript, tsx) | HEAD 2026-08-29 | `75b3874edb2dc714fb1fd77a32013d0f8699989f` |
| `tree-sitter-java` | HEAD 2026-08-29 | `e10607b45ff745f5f876bfa3e94fbcc6b44bdc11` |
| `tree-sitter-rust` | v0.24.2 | `77a3747266f4d621d0757825e6b11edcbf991ca5` |
| `tree-sitter-php` (php variant) | HEAD 2026-08-29 | `3fda2fb9577166c6399834917f9844f30370beea` |
| `tree-sitter-c-sharp` | HEAD 2026-08-29 | `9150f7d56bb47f1a809fa23623f1ba1413e93fa9` |
| zig (WASI clang driver) | 0.16.0 | Homebrew `zig` bottle |
| wazero (host runtime) | v1.12.0 | `go.mod` |

All grammar repositories are under `github.com/tree-sitter/`.

Grammars are generated at **different parser ABI generations** — java, typescript
and tsx still declare `TSFieldMapSlice`, while python, javascript, rust, php and
c-sharp no longer do. The runtime loads any ABI within its supported range, so a
mixed set is fine, but each grammar must be compiled against the `tree_sitter/`
headers it ships with. `guest/build.sh` does this with a per-grammar include
path; one shared header silently miscompiles whichever half does not match it.

Ruby was deliberately excluded. It has no structural visibility marker — no
`public`/`pub` equivalent in the parse tree — so the exported surface would be a
guess rather than a fact, and it is also the largest grammar tested at roughly
220 KB compressed. Honest fidelity is worth more than a language count.

TypeScript and TSX grammars are linked but not routed: `.ts` and `.tsx` still use
the hand-written scanner in `../typescript.go`, because migrating them
re-baselines every stored fingerprint and is a separate decision under ADR-063.
They are already paid for when that migration happens.

## Expected artifact

```
sha256  302e3aeffa7a243691c85d22b29d0f6dc8b1272b480d23b78a578d76789dcd01
size    986162 bytes (gzip), 12240532 bytes raw
built   2026-08-29, macOS arm64
```

`TestEmbeddedGrammarMatchesProvenance` asserts this hash on every run, so the
committed blob cannot drift from this record without a failing test.

## Rebuilding

Clone each input at the commit above, arranged so every grammar keeps its own
`src/tree_sitter/` headers, then:

```sh
ZIG=$(which zig) TS_SRC=<runtime lib/src> GRAMMARS=<grammar root> \
  ./guest/build.sh /tmp/ts-multilang.wasm \
  python:python/src javascript:javascript/src \
  typescript:typescript/typescript/src tsx:typescript/tsx/src \
  java:java/src rust:rust/src php:php/php/src c_sharp:c_sharp/src
gzip -9 -n < /tmp/ts-multilang.wasm > ts-multilang.wasm.gz
```

`gzip -n` omits the timestamp, without which the hash differs on every build.
`TS_SRC` needs the runtime's `lib/src` contents plus its `unicode/`, `portable/`
and `tree_sitter/` headers. Each grammar argument is `name:dir`, where `name`
must match the `tree_sitter_<name>` symbol the grammar exports.

## Known gap

This blob was built on a developer machine, not in CI. The inputs are now pinned
to exact commits, so a rebuild is reproducible by hand, but nothing yet proves
that the committed bytes came from those commits. Moving the build into the
release workflow and comparing the hash is a prerequisite of the signed-release
gate in `docs/beta-release.md`.
