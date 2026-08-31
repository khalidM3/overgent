// Package multilang is the spike's tree-sitter-backed contract extractor. It
// answers question 5 of the spike: whether a wasm-backed extractor can honor
// the same contract as internal/contract — never return an error, treat an
// unparseable or oversized file as "no fingerprint", respect MaxSourceBytes,
// MaxSymbols and MaxSignatureRunes, and apply the deny wire-gate to every
// derived signature before it is published.
//
// It deliberately reuses internal/contract for the hashing, normalization,
// sorting and gating so the spike measures the parser, not a reimplementation
// of the fingerprint format.
package multilang

import (
	"context"

	"github.com/stickguy/stickguy/internal/contract"
	"github.com/stickguy/stickguy/validation/spikes/multilang-contract/tsw"
)

// walkDepth bounds the guest-side pre-order walk. Six levels reach a method
// inside a class body in every language handled here; anything deeper is
// implementation detail rather than API surface.
const walkDepth = 6

// Extractor holds one wasm runtime plus the per-language rules. It is not safe
// for concurrent use; the underlying runtime serializes anyway.
type Extractor struct {
	runtime *tsw.Runtime
	rules   map[string]*rules
	scratch []tsw.Record
}

// New loads the wasm module and prepares every language the spike supports
// that the module actually carries.
func New(ctx context.Context, wasmModule []byte, interpreter bool) (*Extractor, error) {
	names := make([]string, 0, len(languageRules))
	for name := range languageRules {
		names = append(names, name)
	}
	runtime, err := tsw.New(ctx, wasmModule, names, tsw.Config{
		SourceBytes: contract.MaxSourceBytes,
		Records:     1 << 17,
		Interpreter: interpreter,
	})
	if err != nil {
		return nil, err
	}
	return &Extractor{runtime: runtime, rules: languageRules}, nil
}

// Close releases the wasm runtime.
func (e *Extractor) Close(ctx context.Context) error { return e.runtime.Close(ctx) }

// Fingerprintable reports whether this extractor has a grammar for the path.
func (e *Extractor) Fingerprintable(path string) bool { return languageFor(path) != "" }

// Extract mirrors contract.Extract for a tree-sitter language. It never
// returns an error: a path with no grammar, a source over MaxSourceBytes, a
// parse containing ERROR or MISSING nodes, and any wasm failure all yield
// (File{}, false), which callers read as "no fingerprint".
func (e *Extractor) Extract(ctx context.Context, path string, source []byte, deny func(string) bool) (contract.File, bool) {
	language := languageFor(path)
	if language == "" || len(source) > contract.MaxSourceBytes {
		return contract.File{}, false
	}
	rule, ok := e.rules[language]
	if !ok {
		return contract.File{}, false
	}
	grammar, ok := e.runtime.Language(language)
	if !ok {
		return contract.File{}, false
	}
	e.scratch = e.scratch[:0]
	records, clean, err := e.runtime.Parse(ctx, grammar, source, walkDepth, e.scratch)
	e.scratch = records
	if err != nil || !clean {
		return contract.File{}, false
	}
	tree := view{records: records, source: source, grammar: grammar}
	return assemble(path, rule.collect(tree), deny), true
}
