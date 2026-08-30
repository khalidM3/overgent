package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/stickguy/stickguy/internal/contract/multilang"
	"github.com/stickguy/stickguy/internal/contract/wasmgrammar"
)

// The wasm-backed extractor is a single lazily-initialized runtime shared by
// the whole process (ADR-063). One runtime is the right shape for three
// reasons: instantiating it costs about 80ms and 3.9MB of decompressed module,
// multilang.Extractor is not safe for concurrent use, and the callers are a
// long-lived per-user service (ADR-003) rather than a request path.
//
// Every failure mode here — an unsupported platform, a corrupt blob, a wasm
// trap — resolves to "no fingerprint", never to an error returned to a caller.
// Extraction must never block manifest publication.
var (
	wasmOnce      sync.Once
	wasmExtractor *multilang.Extractor
	wasmErr       error
	wasmMu        sync.Mutex
)

// loadWasmExtractor initializes the shared runtime exactly once. It is called
// under wasmMu by extractWasm and directly by WasmStatus.
func loadWasmExtractor() (*multilang.Extractor, error) {
	wasmOnce.Do(func() {
		module, err := wasmgrammar.Multilang()
		if err != nil {
			wasmErr = fmt.Errorf("loading embedded grammar module: %w", err)
			return
		}
		// Interpreter is false: ADR-063 requires an unsupported platform to
		// fail here rather than fall back to a one-second-per-file extractor.
		extractor, err := multilang.New(context.Background(), module, false)
		if err != nil {
			wasmErr = fmt.Errorf("starting wasm grammar runtime: %w", err)
			return
		}
		wasmExtractor = extractor
	})
	return wasmExtractor, wasmErr
}

// WasmStatus reports whether the wasm-backed languages are actually available
// on this platform, and why not when they are not.
//
// It exists so the service can honor the honest-fidelity rule: a platform
// without wazero's compiler produces no fingerprint for Python and JavaScript,
// and must say so rather than let an empty exported surface read as a stable
// contract. Callers should surface this rather than discard it.
func WasmStatus() (available bool, reason string) {
	wasmMu.Lock()
	defer wasmMu.Unlock()
	extractor, err := loadWasmExtractor()
	if err != nil {
		return false, err.Error()
	}
	if extractor == nil {
		return false, "wasm grammar runtime unavailable"
	}
	return true, ""
}

// extractWasm derives a fingerprint through the tree-sitter runtime. It
// mirrors Extract's contract exactly: the second result is false for a path
// with no grammar, an oversized or unparseable source, an unavailable runtime,
// or any wasm failure, and callers read that as "no fingerprint".
func extractWasm(path string, source []byte, deny func(signature string) bool) (File, bool) {
	wasmMu.Lock()
	defer wasmMu.Unlock()
	extractor, err := loadWasmExtractor()
	if err != nil || extractor == nil {
		return File{}, false
	}
	return extractor.Extract(context.Background(), path, source, deny)
}

// wasmFingerprintable reports whether a path belongs to a language handled by
// the tree-sitter runtime rather than by go/parser or the TypeScript scanner.
//
// Ruby is deliberately absent: it has no structural visibility marker, so its
// exported surface would be a guess rather than a fact, and a wrong guess here
// is a false interruption. See PROVENANCE.md.
//
// TypeScript and TSX are deliberately absent: the embedded module carries their
// grammars, but migrating them off the hand-written scanner re-baselines every
// stored fingerprint and is a separate decision under ADR-063.
func wasmFingerprintable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py", ".pyi", ".js", ".jsx", ".mjs", ".cjs",
		".java", ".rs", ".cs", ".php":
		return true
	}
	return false
}
