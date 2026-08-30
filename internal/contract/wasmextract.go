package contract

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/stickguy/stickguy/internal/contract/multilang"
	"github.com/stickguy/stickguy/internal/contract/wasmgrammar"
)

// The wasm-backed extractor is a single process-wide instance (ADR-063) that
// compiles each language's grammar only when a file of that language is first
// fingerprinted. Constructing it is free, so the cost a member pays is
// proportional to the languages their repository actually contains rather than
// to the number Stickguy supports.
//
// Every failure mode here — an unsupported platform, a corrupt module, a wasm
// trap — resolves to "no fingerprint", never to an error returned to a caller.
// Extraction must never block manifest publication.
var (
	wasmOnce      sync.Once
	wasmExtractor *multilang.Extractor
	wasmMu        sync.Mutex
)

// loadWasmExtractor builds the shared extractor exactly once. It cannot fail:
// no wasm is compiled here, so an unsupported platform is discovered later, per
// language, and reported as a missing fingerprint.
func loadWasmExtractor() *multilang.Extractor {
	wasmOnce.Do(func() {
		// Interpreter is false: ADR-063 requires an unsupported platform to
		// fail rather than fall back to a one-second-per-file extractor.
		wasmExtractor = multilang.New(wasmgrammar.Module, false)
	})
	return wasmExtractor
}

// WasmStatus reports whether wasm-backed extraction actually works on this
// platform, and why not when it does not.
//
// It answers the question by compiling one real grammar, because that is the
// only honest test: wazero's compiler support is what decides this, and it is
// not knowable without asking it. The probe language is loaded lazily like any
// other, so calling this costs one grammar rather than all of them.
//
// It exists so the service can honor the honest-fidelity rule: a platform
// without wazero's compiler produces no fingerprint for the wasm-backed
// languages, and must say so rather than let an empty exported surface read as
// a stable contract.
func WasmStatus() (available bool, reason string) {
	wasmMu.Lock()
	defer wasmMu.Unlock()
	extractor := loadWasmExtractor()
	if _, ok := extractor.Extract(context.Background(), "probe.py", []byte("def probe():\n    return 1\n"), nil); !ok {
		return false, "wasm grammar runtime unavailable on this platform"
	}
	return true, ""
}

// LoadedGrammars reports which grammars this process has actually compiled. It
// exists so the laziness is observable rather than assumed.
func LoadedGrammars() []string {
	wasmMu.Lock()
	defer wasmMu.Unlock()
	return loadWasmExtractor().Loaded()
}

// extractWasm derives a fingerprint through the tree-sitter runtime, compiling
// that language's grammar if this is the first file of its kind.
func extractWasm(path string, source []byte, deny func(signature string) bool) (File, bool) {
	wasmMu.Lock()
	defer wasmMu.Unlock()
	return loadWasmExtractor().Extract(context.Background(), path, source, deny)
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
		".java", ".rs", ".cs", ".php",
		".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx",
		".scala", ".sc", ".kt", ".kts", ".dart":
		return true
	}
	return false
}
