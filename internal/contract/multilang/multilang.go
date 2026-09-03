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
	"sort"

	"github.com/khalidM3/overgent/internal/contract/fingerprint"
	"github.com/khalidM3/overgent/internal/contract/tsw"
)

// walkDepth bounds the guest-side pre-order walk. Ten levels are needed because
// the deepest name this has to reach is a C++ method inside a class inside a
// namespace: translation unit, namespace, declaration list, class, field list,
// field declaration, function declarator, then the identifier itself. Six was
// enough for the flatter languages and silently truncated that one, which
// dropped public methods rather than failing.
const walkDepth = 10

// Extractor owns one wasm runtime per language, each created on first use.
//
// Lazy per-language loading is the point (ADR-063): compiling every grammar up
// front costs startup time and resident memory proportional to how many
// languages Overgent supports, not to how many the member's repository
// actually contains. A Go and TypeScript repository never pays for the C#
// grammar.
//
// It is not safe for concurrent use; callers serialize, and the underlying
// runtimes serialize anyway.
type Extractor struct {
	loader      func(string) ([]byte, bool, error)
	interpreter bool
	rules       map[string]*rules
	runtimes    map[string]*tsw.Runtime
	failed      map[string]struct{}
	scratch     []tsw.Record
}

// New prepares an extractor. It deliberately loads nothing: no wasm is
// compiled until a file of that language is actually fingerprinted, so
// constructing an extractor is free even on a platform where the runtime will
// later turn out to be unavailable.
//
// loader resolves a tree-sitter language name to its module bytes. Its second
// result reports whether a module for that language exists at all, which is
// distinct from failing to load one that does.
func New(loader func(string) ([]byte, bool, error), interpreter bool) *Extractor {
	return &Extractor{
		loader:      loader,
		interpreter: interpreter,
		rules:       languageRules,
		runtimes:    map[string]*tsw.Runtime{},
		failed:      map[string]struct{}{},
	}
}

// runtimeFor returns the runtime for one language, compiling its module on
// first use. A language that has already failed is not retried: the failure is
// a missing module or an unsupported platform, neither of which changes within
// a process, and retrying would pay the compile cost on every file.
func (e *Extractor) runtimeFor(ctx context.Context, language string) (*tsw.Runtime, bool) {
	if runtime, ok := e.runtimes[language]; ok {
		return runtime, true
	}
	if _, dead := e.failed[language]; dead {
		return nil, false
	}
	module, exists, err := e.loader(language)
	if err != nil || !exists {
		e.failed[language] = struct{}{}
		return nil, false
	}
	runtime, err := tsw.New(ctx, module, []string{language}, tsw.Config{
		SourceBytes: fingerprint.MaxSourceBytes,
		Records:     1 << 17,
		Interpreter: e.interpreter,
	})
	if err != nil {
		e.failed[language] = struct{}{}
		return nil, false
	}
	e.runtimes[language] = runtime
	return runtime, true
}

// Close releases every runtime that was actually created. It reports the first
// failure but always attempts all of them, so one stuck runtime cannot leak the
// rest.
func (e *Extractor) Close(ctx context.Context) error {
	var firstErr error
	for language, runtime := range e.runtimes {
		if err := runtime.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(e.runtimes, language)
	}
	return firstErr
}

// Loaded reports which languages have actually been compiled. It exists so the
// laziness can be asserted by a test rather than assumed.
func (e *Extractor) Loaded() []string {
	names := make([]string, 0, len(e.runtimes))
	for language := range e.runtimes {
		names = append(names, language)
	}
	sort.Strings(names)
	return names
}

// Fingerprintable reports whether this extractor has rules for the path. It
// does not load anything, so it stays cheap and stays true on a platform where
// extraction will fail: a platform gap must never be reported as a language
// gap.
func (e *Extractor) Fingerprintable(path string) bool {
	language := languageFor(path)
	if language == "" {
		return false
	}
	_, ok := e.rules[language]
	return ok
}

// Extract mirrors fingerprint.Extract for a tree-sitter language. It never
// returns an error: a path with no grammar, a source over MaxSourceBytes, a
// parse containing ERROR or MISSING nodes, an unavailable runtime, and any wasm
// failure all yield (File{}, false), which callers read as "no fingerprint".
func (e *Extractor) Extract(ctx context.Context, path string, source []byte, deny func(string) bool) (fingerprint.File, bool) {
	language := languageFor(path)
	if language == "" || len(source) > fingerprint.MaxSourceBytes {
		return fingerprint.File{}, false
	}
	rule, ok := e.rules[language]
	if !ok {
		return fingerprint.File{}, false
	}
	runtime, ok := e.runtimeFor(ctx, language)
	if !ok {
		return fingerprint.File{}, false
	}
	grammar, ok := runtime.Language(language)
	if !ok {
		return fingerprint.File{}, false
	}
	e.scratch = e.scratch[:0]
	records, clean, err := runtime.Parse(ctx, grammar, source, walkDepth, e.scratch)
	e.scratch = records
	if err != nil || !clean {
		return fingerprint.File{}, false
	}
	tree := view{records: records, source: source, grammar: grammar}
	return assemble(path, rule.collect(tree), deny), true
}
