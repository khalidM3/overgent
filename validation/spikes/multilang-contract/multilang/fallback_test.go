package multilang

import (
	"context"
	"testing"

	"github.com/stickguy/stickguy/internal/contract"
)

// TestFallbackJavaScriptThroughTheTypeScriptScanner measures the spike's first
// fallback option: reuse internal/contract's TypeScript scanner for .js and
// .jsx by widening Fingerprintable. The scanner is token-based rather than
// type-aware, so it does run on JavaScript. This test feeds real JavaScript to
// it under a .ts path and records what it recovers next to what a real grammar
// recovers, so the fallback's cost is a number rather than an assumption.
func TestFallbackJavaScriptThroughTheTypeScriptScanner(t *testing.T) {
	extractor := newExtractor(t)
	for _, fixture := range []string{"uri.js", "convertPathData.js", "_path.js"} {
		source := read(t, fixture)
		scanner, scannerOK := contract.Extract("fallback/"+fixture+".ts", source, nil)
		grammar, grammarOK := extractor.Extract(context.Background(), "real/"+fixture, source, nil)
		scannerNames := map[string]bool{}
		for _, symbol := range scanner.Symbols {
			scannerNames[symbol.Name] = true
		}
		var missed []string
		for _, symbol := range grammar.Symbols {
			if !scannerNames[symbol.Name] {
				missed = append(missed, symbol.Name)
			}
		}
		t.Logf("%s: scanner ok=%v symbols=%d | grammar ok=%v symbols=%d | grammar-only symbols=%d %v",
			fixture, scannerOK, len(scanner.Symbols), grammarOK, len(grammar.Symbols), len(missed), missed)
	}
}
