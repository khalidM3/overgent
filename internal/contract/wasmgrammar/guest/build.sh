#!/bin/sh
# Compile the tree-sitter runtime plus a grammar set into one standalone
# wasm32-wasi reactor module.
#
# Each grammar is compiled against the tree_sitter/ headers it ships with,
# never against a shared copy: grammars are generated at different parser ABI
# generations (some still declare TSFieldMapSlice, newer ones do not), and one
# global include path silently miscompiles whichever half does not match. The
# runtime loads any ABI within its supported range, so a mixed set is fine as
# long as each grammar sees its own header.
#
# Usage: TS_SRC=<runtime src> GRAMMARS=<grammar root> ZIG=<zig> ./build2.sh out.wasm lang...
set -eu
OUT="$1"; shift
: "${TS_SRC:?set TS_SRC to the tree-sitter lib/src directory}"
: "${GRAMMARS:?set GRAMMARS to the grammar root}"
: "${ZIG:?set ZIG to a zig binary}"
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

CFLAGS="--target=wasm32-wasi-musl -Os -fPIC -include assert.h"
OBJECTS=""

# Runtime and shim compile against the runtime's own headers.
"$ZIG" cc $CFLAGS -I "$TS_SRC" -c "$TS_SRC/lib.c" -o "$TMP/lib.o"
"$ZIG" cc $CFLAGS -I "$TS_SRC" -c "$HERE/shim.c" -o "$TMP/shim.o"
OBJECTS="$TMP/lib.o $TMP/shim.o"

EXPORTS="
-Wl,--export=malloc -Wl,--export=free
-Wl,--export=ts_parser_new -Wl,--export=ts_parser_delete
-Wl,--export=ts_parser_set_language -Wl,--export=ts_parser_parse_string
-Wl,--export=ts_parser_reset -Wl,--export=ts_tree_delete
-Wl,--export=ts_language_symbol_name -Wl,--export=ts_language_symbol_count
-Wl,--export=ts_language_abi_version
-Wl,--export=sg_dump -Wl,--export=sg_tree_has_error -Wl,--export=sg_record_size
"

# Each argument is name:dir, where dir is relative to GRAMMARS. The directory
# is explicit rather than derived because several grammar repositories keep a
# shared scanner header at a fixed relative path (../../common/scanner.h), so
# their source tree cannot be flattened.
for entry in "$@"; do
  lang="${entry%%:*}"
  dir="$GRAMMARS/${entry#*:}"
  [ -d "$dir" ] || { echo "missing grammar directory: $dir" >&2; exit 1; }
  symbol="tree_sitter_$lang"
  "$ZIG" cc $CFLAGS -I "$dir" -c "$dir/parser.c" -o "$TMP/$lang-parser.o"
  OBJECTS="$OBJECTS $TMP/$lang-parser.o"
  if [ -f "$dir/scanner.c" ]; then
    "$ZIG" cc $CFLAGS -I "$dir" -c "$dir/scanner.c" -o "$TMP/$lang-scanner.o"
    OBJECTS="$OBJECTS $TMP/$lang-scanner.o"
  fi
  EXPORTS="$EXPORTS -Wl,--export=$symbol"
done

# shellcheck disable=SC2086
"$ZIG" cc --target=wasm32-wasi-musl -mexec-model=reactor $OBJECTS -o "$OUT" \
  -Wl,--no-entry -Wl,-z -Wl,stack-size=1048576 -Wl,--strip-debug $EXPORTS
