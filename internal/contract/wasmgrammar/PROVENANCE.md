# Grammar module provenance

`modules/*.wasm.gz` are compiled binaries committed to a public repository and
executed on every member's machine as part of contract extraction (ADR-063).
A binary cannot be reviewed by reading it, so every input is pinned to an exact
commit here and every artifact's hash is asserted by a test. Anyone can rebuild
them and compare.

There is one module per language rather than one for all of them, so a grammar
is compiled only when a file of that language is actually fingerprinted.

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

TypeScript and TSX modules are embedded but not routed: `.ts` and `.tsx` still
use the hand-written scanner in `../typescript.go`, because migrating them
re-baselines every stored fingerprint and is a separate decision under ADR-063.
Under per-language loading they cost binary size but no memory until that
migration happens, because nothing ever asks for them.

## Expected artifacts

One module per language, each carrying its own copy of the runtime (about 37 KB
compressed) so it can be compiled independently of the others.

| Module | sha256 | Bytes (gzip) |
| --- | --- | --- |
| `python` | `0de9a5848a549ae2f538b82127a94ab8941bb7d4817b2e5fe9bc4ecdaad468a8` | 98,998 |
| `javascript` | `6e3283152cd82f3dab11e03c73c040ec39b33700b18ce5311f9d9bcc1b9b47d1` | 85,103 |
| `typescript` | `b0c3ad46cfad4abdf3ce721d96fa7593b5f0b0a31f352e5ce4c9dc875302205d` | 167,716 |
| `tsx` | `2c76e268c442a62fbfbb30786eca0cdf9f7cd5a6be6fb2c25abefaedfd4c1577` | 170,993 |
| `java` | `e7da5238278c9810f8916b83bb9b9b722b575464138fa43c5292f76e8837cdc6` | 83,990 |
| `rust` | `cec541058d7dd0de1d680ee4056d8f56db2ffa6328ffd7ca43ccd64575a4cc7e` | 149,257 |
| `php` | `8f1df367dfc53aa915a33bb364bc84d3f0d16eaf588b136e3894ef9dd9f3d9e6` | 139,784 |
| `c_sharp` | `1788f3641ffbb9115f6770f7117af1c4b8c42114ec2a7b387fd60ffa75789764` | 370,358 |

Built 2026-08-30 on macOS arm64. Total 1.24 MB compressed.

`TestEmbeddedModulesMatchProvenance` asserts every hash and size, and
`TestLanguagesMatchesTheRecord` fails if a module is added or removed without
this table being updated.

## Rebuilding

Clone each input at the commit above, arranged so every grammar keeps its own
`src/tree_sitter/` headers, then build one module per language:

```sh
ZIG=$(which zig) TS_SRC=<runtime lib/src> GRAMMARS=<grammar root> \
  ./guest/build.sh /tmp/python.wasm python:python/src
gzip -9 -n < /tmp/python.wasm > modules/python.wasm.gz
```

Repeat per language, using the grammar's `tree_sitter_<name>` symbol as the
module name. `gzip -n` omits the timestamp, without which the hash differs on
every build. `TS_SRC` needs the runtime's `lib/src` contents plus its
`unicode/`, `portable/` and `tree_sitter/` headers.

`build.sh` still accepts several grammars at once; that is how the combined
module used to be produced, and it remains useful for measuring a grammar set
before splitting it.

## Known gap

These modules were built on a developer machine, not in CI. The inputs are
pinned to exact commits, so a rebuild is reproducible by hand, but nothing yet
proves that the committed bytes came from those commits. Moving the build into the
release workflow and comparing the hash is a prerequisite of the signed-release
gate in `docs/beta-release.md`.
