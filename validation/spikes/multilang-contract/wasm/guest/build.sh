#!/bin/sh
# build.sh — compile the tree-sitter runtime plus a chosen grammar set into one
# standalone wasm32-wasi reactor module.
#
# Grammars are statically linked, not dynamically loaded: the upstream
# web-tree-sitter design loads grammars as emscripten SIDE modules through
# dlopen, which wazero cannot do. One module per grammar set is the only shape
# a pure-Go runtime can execute.
#
# Usage: TS_SRC=<tree-sitter src dir> ZIG=<zig binary> ./build.sh <out.wasm> [lang...]
set -eu

OUT="$1"; shift
: "${TS_SRC:?set TS_SRC to a tree-sitter src/ directory}"
: "${ZIG:?set ZIG to a zig binary}"
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

SOURCES="$TS_SRC/lib.c $HERE/shim.c"
EXPORTS="
-Wl,--export=malloc
-Wl,--export=free
-Wl,--export=ts_parser_new
-Wl,--export=ts_parser_delete
-Wl,--export=ts_parser_set_language
-Wl,--export=ts_parser_parse_string
-Wl,--export=ts_parser_reset
-Wl,--export=ts_tree_delete
-Wl,--export=ts_language_symbol_name
-Wl,--export=ts_language_symbol_count
-Wl,--export=ts_language_version
-Wl,--export=sg_dump
-Wl,--export=sg_tree_has_error
-Wl,--export=sg_record_size
"

for lang in "$@"; do
  case "$lang" in
    typescript) dir="$TS_SRC/typescript/typescript"; symbol=tree_sitter_typescript ;;
    tsx)        dir="$TS_SRC/typescript/tsx";        symbol=tree_sitter_tsx ;;
    go)         dir="$TS_SRC/golang";                symbol=tree_sitter_go ;;
    *)          dir="$TS_SRC/$lang";                 symbol="tree_sitter_$lang" ;;
  esac
  SOURCES="$SOURCES $dir/parser.c"
  [ -f "$dir/scanner.c" ] && SOURCES="$SOURCES $dir/scanner.c"
  EXPORTS="$EXPORTS -Wl,--export=$symbol"
done

# shellcheck disable=SC2086
"$ZIG" cc --target=wasm32-wasi-musl -mexec-model=reactor -I "$TS_SRC" \
  $SOURCES -o "$OUT" \
  -include assert.h -Os -fPIC -Wl,--no-entry -Wl,-z -Wl,stack-size=1048576 -Wl,--strip-debug \
  $EXPORTS
