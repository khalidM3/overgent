// Package wasmgrammar carries the built tree-sitter modules as embedded blobs,
// one per language.
//
// One module per language rather than one module for all of them (ADR-063).
// A combined module has to be compiled in full before any language can be
// used, which measured 555ms and 110MB resident at seventeen grammars and
// scales to roughly 3.3s and 650MB at a hundred — paid by every member whether
// their repository contains that language or not. Split, a module is compiled
// only when a file of that language is actually fingerprinted, so the cost is
// proportional to what a repository really contains.
//
// The runtime is duplicated into each module, which costs about 37KB
// compressed apiece. That is the whole price of the split, and it buys a
// startup and memory profile that no longer depends on how many languages
// Stickguy supports.
//
// Exact input commits for every module are pinned in PROVENANCE.md, and their
// hashes and sizes are asserted by a test.
package wasmgrammar

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

//go:embed modules/*.wasm.gz
var modules embed.FS

var (
	mu       sync.Mutex
	inflated = map[string][]byte{}
)

// Module returns the wasm module for one tree-sitter language name, inflating
// it on first use and caching the result. The bool is false when no module for
// that language is embedded, which callers read as "no fingerprint" rather than
// as an error.
//
// The inflated bytes are cached because wazero needs them again if a runtime is
// ever rebuilt, and they are small relative to the compiled module they
// produce.
func Module(language string) ([]byte, bool, error) {
	mu.Lock()
	defer mu.Unlock()
	if cached, ok := inflated[language]; ok {
		return cached, true, nil
	}
	compressed, err := modules.ReadFile("modules/" + language + ".wasm.gz")
	if err != nil {
		return nil, false, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, true, fmt.Errorf("opening embedded %s module: %w", language, err)
	}
	defer func() { _ = reader.Close() }()
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, true, fmt.Errorf("inflating embedded %s module: %w", language, err)
	}
	inflated[language] = raw
	return raw, true, nil
}

// Languages lists the embedded module names in a stable order. It exists so
// tests and diagnostics can enumerate what this binary actually carries rather
// than what a document claims it carries.
func Languages() []string {
	entries, err := fs.ReadDir(modules, "modules")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, strings.TrimSuffix(entry.Name(), ".wasm.gz"))
	}
	sort.Strings(names)
	return names
}
