// Package wasmgrammar carries the built tree-sitter module as an embedded blob.
package wasmgrammar

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"sync"
)

// compressed is the tree-sitter runtime plus the Python, JavaScript,
// TypeScript, TSX, Java, Rust, PHP and C# grammars, linked into one standalone
// wasm32-wasi reactor module by guest/build.sh. Exact input commits are pinned
// in PROVENANCE.md and the artifact's hash is asserted by a test. Grammars are statically linked because the
// upstream web-tree-sitter design loads them as emscripten side modules
// through dlopen, which wazero cannot do.
//
// It is stored gzipped because grammar parse tables compress about 9.4x, which
// is the difference between a 3.75 MB and a 400 KB addition to a binary. The
// one-time inflate measured 8.3 ms against a 72 ms wasm compile, so it is not
// a meaningful share of startup.
//
//go:embed ts-multilang.wasm.gz
var compressed []byte

var (
	once       sync.Once
	inflated   []byte
	inflateErr error
)

// Multilang returns the wasm module, inflating it on first use.
func Multilang() ([]byte, error) {
	once.Do(func() {
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			inflateErr = fmt.Errorf("opening embedded wasm: %w", err)
			return
		}
		defer func() { _ = reader.Close() }()
		if inflated, inflateErr = io.ReadAll(reader); inflateErr != nil {
			inflateErr = fmt.Errorf("inflating embedded wasm: %w", inflateErr)
		}
	})
	return inflated, inflateErr
}
