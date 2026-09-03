// shim.c — the guest half of the Overgent contract-extraction spike.
//
// A straightforward wazero binding crosses the host/guest boundary once per
// tree-sitter node accessor, which is thousands of crossings for one source
// file. This shim instead performs the whole bounded pre-order walk inside the
// guest and returns one packed record array, so extraction costs O(1)
// crossings per file. Language-specific interpretation stays in Go: the shim
// emits only numeric symbol ids and byte offsets and never inspects them.

#include "api.h"
#include <stdint.h>

// One node. Byte offsets index the source the host already holds, so no source
// text is ever copied back across the boundary.
typedef struct {
  uint32_t symbol;
  uint32_t start;
  uint32_t end;
  uint32_t depth;
  uint32_t field;
  uint32_t named;
} SGRecord;

// sg_dump writes a bounded pre-order walk of tree into out and returns the
// number of records the walk produced. A return greater than cap means the
// caller's buffer was too small and the dump is truncated; the caller treats
// that as "no fingerprint" rather than as a partial answer.
uint32_t sg_dump(const TSTree *tree, uint32_t max_depth, SGRecord *out, uint32_t cap) {
  TSNode root = ts_tree_root_node(tree);
  TSTreeCursor cursor = ts_tree_cursor_new(root);
  uint32_t count = 0;
  uint32_t depth = 0;
  for (;;) {
    TSNode node = ts_tree_cursor_current_node(&cursor);
    if (count < cap) {
      out[count].symbol = ts_node_symbol(node);
      out[count].start = ts_node_start_byte(node);
      out[count].end = ts_node_end_byte(node);
      out[count].depth = depth;
      out[count].field = ts_tree_cursor_current_field_id(&cursor);
      out[count].named = ts_node_is_named(node) ? 1u : 0u;
    }
    count++;
    if (depth < max_depth && ts_tree_cursor_goto_first_child(&cursor)) {
      depth++;
      continue;
    }
    for (;;) {
      if (ts_tree_cursor_goto_next_sibling(&cursor)) break;
      if (depth == 0) {
        ts_tree_cursor_delete(&cursor);
        return count;
      }
      ts_tree_cursor_goto_parent(&cursor);
      depth--;
    }
  }
}

// sg_tree_has_error reports whether the parse contains any ERROR or MISSING
// node. The extractor refuses a fingerprint for such a file, matching the
// existing Go and TypeScript extractors' "unparseable yields nothing" rule.
uint32_t sg_tree_has_error(const TSTree *tree) {
  return ts_node_has_error(ts_tree_root_node(tree)) ? 1u : 0u;
}

// sg_record_size lets the host assert its struct layout matches the guest's
// rather than assuming a wasm32 packing.
uint32_t sg_record_size(void) { return (uint32_t)sizeof(SGRecord); }
