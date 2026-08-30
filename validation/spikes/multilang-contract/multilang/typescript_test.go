package multilang

import (
	"context"
	"testing"

	"github.com/stickguy/stickguy/internal/contract"
)

// TestScannerBlindSpotsAreRepaired compares the production hand-written
// TypeScript scanner against the tree-sitter extractor on the two files in
// this repository that the scanner currently drops. Both contain regular
// expression literals, which the scanner documents as a known desynchronizing
// case; a real grammar tokenizes them correctly.
func TestScannerBlindSpotsAreRepaired(t *testing.T) {
	extractor := newExtractor(t)
	for _, testCase := range []struct {
		fixture string
		path    string
		want    []string
	}{
		{"domain.ts.txt", "convex/src/domain.ts", []string{"validateContractSignature", "validateSessionMessageText"}},
		{"app.test.tsx.txt", "apps/dashboard/test/app.test.tsx", nil},
	} {
		source := read(t, testCase.fixture)
		if _, ok := contract.Extract(testCase.path, source, nil); ok {
			t.Fatalf("%s: the production scanner now fingerprints this file; the comparison is stale", testCase.path)
		}
		file, ok := extractor.Extract(context.Background(), testCase.path, source, nil)
		if !ok {
			t.Errorf("%s: tree-sitter also produced no fingerprint", testCase.path)
			continue
		}
		names := map[string]bool{}
		for _, symbol := range file.Symbols {
			names[symbol.Name] = true
		}
		t.Logf("%s: tree-sitter recovered %d symbols where the scanner recovered none", testCase.path, len(file.Symbols))
		for _, want := range testCase.want {
			if !names[want] {
				t.Errorf("%s: expected symbol %q, got %v", testCase.path, want, names)
			}
		}
	}
}

// TestTypeScriptAgreesWithTheScanner checks that on a file both extractors
// handle, tree-sitter finds at least the surface the scanner finds. Exact hash
// equality is not expected: the two normalize headers differently, so adopting
// tree-sitter is a fingerprint-format change, not a drop-in swap.
func TestTypeScriptAgreesWithTheScanner(t *testing.T) {
	extractor := newExtractor(t)
	source := read(t, "typescript-sample.ts.txt")
	native, ok := contract.Extract("sample.ts", source, nil)
	if !ok {
		t.Fatal("the production scanner should fingerprint this file")
	}
	tree, ok := extractor.Extract(context.Background(), "sample.ts", source, nil)
	if !ok {
		t.Fatal("tree-sitter should fingerprint this file")
	}
	names := map[string]bool{}
	for _, symbol := range tree.Symbols {
		names[symbol.Name] = true
	}
	var missing []string
	for _, symbol := range native.Symbols {
		if !names[symbol.Name] {
			missing = append(missing, symbol.Name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("tree-sitter missed symbols the scanner found: %v", missing)
	}
	t.Logf("scanner symbols=%d tree-sitter symbols=%d hashes equal=%v",
		len(native.Symbols), len(tree.Symbols), native.FileContractHash == tree.FileContractHash)
}
