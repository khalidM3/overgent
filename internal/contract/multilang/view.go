package multilang

import (
	"path/filepath"
	"strings"

	"github.com/khalidM3/overgent/internal/contract/tsw"
)

// view navigates the flat pre-order dump the guest produced. The dump carries
// a depth per record, so the children of record i are the following records at
// depth i+1 up to the first record at depth i or shallower.
type view struct {
	records []tsw.Record
	source  []byte
	grammar *tsw.Language
}

func (v view) kind(index int) string { return v.grammar.Name(v.records[index].Symbol) }

func (v view) text(index int) string {
	record := v.records[index]
	if int(record.End) > len(v.source) || record.Start > record.End {
		return ""
	}
	return string(v.source[record.Start:record.End])
}

// textTo returns the declaration header: everything from the start of index up
// to the start of stop. It is how a body is dropped without ever copying it.
func (v view) textTo(index, stop int) string {
	start, end := v.records[index].Start, v.records[stop].Start
	if int(end) > len(v.source) || start > end {
		return ""
	}
	return string(v.source[start:end])
}

func (v view) children(index int) []int {
	depth := v.records[index].Depth
	var out []int
	for next := index + 1; next < len(v.records) && v.records[next].Depth > depth; next++ {
		if v.records[next].Depth == depth+1 {
			out = append(out, next)
		}
	}
	return out
}

// childOfKind returns the first direct child with one of the given node types.
func (v view) childOfKind(index int, kinds ...string) (int, bool) {
	for _, child := range v.children(index) {
		for _, kind := range kinds {
			if v.kind(child) == kind {
				return child, true
			}
		}
	}
	return 0, false
}

// header captures a declaration from its own start to the start of its body,
// or the whole node when it has no body. bodyKinds is per language.
func (v view) header(index int, bodyKinds ...string) string {
	if body, ok := v.childOfKind(index, bodyKinds...); ok {
		return v.textTo(index, body)
	}
	return v.text(index)
}

// languageFor maps a repository-relative path to a tree-sitter grammar name.
// It is the spike's stand-in for fingerprint.Fingerprintable.
func languageFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py", ".pyi":
		return "python"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".mts", ".cts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".cs":
		return "c_sharp"
	case ".php":
		return "php"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx":
		return "cpp"
	case ".scala", ".sc":
		return "scala"
	case ".kt", ".kts":
		return "kotlin"
	case ".dart":
		return "dart"
	}
	return ""
}
