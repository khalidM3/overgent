package multilang

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stickguy/stickguy/internal/contract"
	"github.com/stickguy/stickguy/validation/spikes/multilang-contract/wasm"
)

// mustModule inflates the embedded wasm once per test binary.
func mustModule(t testing.TB) []byte {
	t.Helper()
	module, err := wasm.Multilang()
	if err != nil {
		t.Fatalf("loading embedded wasm: %v", err)
	}
	return module
}

func newExtractor(t testing.TB) *Extractor {
	t.Helper()
	extractor, err := New(context.Background(), mustModule(t), false)
	if err != nil {
		t.Fatalf("loading wasm extractor: %v", err)
	}
	t.Cleanup(func() { _ = extractor.Close(context.Background()) })
	return extractor
}

func read(t testing.TB, name string) []byte {
	t.Helper()
	source, err := os.ReadFile("../testdata/" + name)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return source
}

// TestRealPythonFile is the end-to-end proof for question 1 on Python: a real
// stdlib source file parsed by a tree-sitter grammar running under wazero.
func TestRealPythonFile(t *testing.T) {
	extractor := newExtractor(t)
	file, ok := extractor.Extract(context.Background(), "lib/dataclasses.py", read(t, "dataclasses.py"), nil)
	if !ok {
		t.Fatal("expected a fingerprint for a real Python file")
	}
	byName := map[string]contract.Symbol{}
	for _, symbol := range file.Symbols {
		byName[symbol.Name] = symbol
	}
	for _, want := range []string{"dataclass", "field", "asdict", "replace", "Field", "FrozenInstanceError"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("expected exported Python symbol %q", want)
		}
	}
	if _, ok := byName["_process_class"]; ok {
		t.Error("underscore-prefixed Python names are not exported surface")
	}
	if got := byName["dataclass"].Signature; !strings.HasPrefix(got, "def dataclass(") {
		t.Errorf("dataclass signature = %q", got)
	}
	if strings.Contains(byName["dataclass"].Signature, "return") {
		t.Error("a signature must not contain the body")
	}
	if len(file.FileContractHash) != 64 {
		t.Errorf("file contract hash = %q", file.FileContractHash)
	}
}

// TestRealJavaScriptFile is the same proof for JavaScript, and shows the three
// forms the existing TypeScript scanner documents as blind spots.
func TestRealJavaScriptFile(t *testing.T) {
	extractor := newExtractor(t)
	// An ESM file and a CommonJS file, both taken from installed packages.
	for fixture, want := range map[string][]string{
		"uri.js":             {"parse", "serialize", "resolve", "normalize", "SCHEMES"},
		"convertPathData.js": {"name", "type", "fn", "description"},
	} {
		file, ok := extractor.Extract(context.Background(), "plugins/"+fixture, read(t, fixture), nil)
		if !ok {
			t.Fatalf("%s: expected a fingerprint for a real JavaScript file", fixture)
		}
		names := map[string]bool{}
		for _, symbol := range file.Symbols {
			names[symbol.Name] = true
		}
		for _, symbol := range want {
			if !names[symbol] {
				t.Errorf("%s: expected exported symbol %q, got %v", fixture, symbol, names)
			}
		}
	}
}

func TestExportFormsTheScannerMisses(t *testing.T) {
	extractor := newExtractor(t)
	source := []byte(`
const internal = 1;
export default function main(a, b) { return a + b; }
export { internal as shared, main };
export * from "./other.js";
export class Widget extends Base {
  render(props) { return null; }
}
export const LIMIT = 3;
`)
	file, ok := extractor.Extract(context.Background(), "src/index.js", source, nil)
	if !ok {
		t.Fatal("expected a fingerprint")
	}
	// A name can legitimately appear twice with different kinds: `main` is
	// both the default-exported declaration and a member of an export clause.
	present := map[string]bool{}
	var have []string
	for _, symbol := range file.Symbols {
		present[symbol.Name+"/"+symbol.Kind] = true
		have = append(have, symbol.Name+"/"+symbol.Kind)
	}
	for _, want := range []string{
		"main/" + kindFunction, "shared/" + kindReexport, "*/" + kindReexport,
		"Widget/" + kindClass, "Widget.render/" + kindMethod, "LIMIT/" + kindConst,
	} {
		if !present[want] {
			t.Errorf("missing symbol %q (have %v)", want, have)
		}
	}
	if signature := findSignature(file, "LIMIT"); strings.Contains(signature, "3") {
		t.Errorf("const initializer must not be part of the contract: %q", signature)
	}
}

func findSignature(file contract.File, name string) string {
	for _, symbol := range file.Symbols {
		if symbol.Name == name {
			return symbol.Signature
		}
	}
	return ""
}

// TestContractRulesHold covers question 5: the wasm extractor must obey every
// bound and gate that internal/contract obeys, and must never return an error.
func TestContractRulesHold(t *testing.T) {
	extractor := newExtractor(t)
	ctx := context.Background()

	t.Run("unfingerprintable path", func(t *testing.T) {
		if _, ok := extractor.Extract(ctx, "README.md", []byte("# hi"), nil); ok {
			t.Error("a path with no grammar must have no fingerprint")
		}
	})

	t.Run("oversized source", func(t *testing.T) {
		big := append(bytes(contract.MaxSourceBytes+1, 'x'), []byte("\n")...)
		if _, ok := extractor.Extract(ctx, "big.py", big, nil); ok {
			t.Error("a source over MaxSourceBytes must have no fingerprint")
		}
	})

	t.Run("at the size limit", func(t *testing.T) {
		source := []byte("def f():\n    return 0\n")
		padding := contract.MaxSourceBytes - len(source) - 2
		source = append(source, []byte("# ")...)
		source = append(source, bytes(padding, 'p')...)
		if len(source) != contract.MaxSourceBytes {
			t.Fatalf("fixture is %d bytes", len(source))
		}
		if _, ok := extractor.Extract(ctx, "edge.py", source, nil); !ok {
			t.Error("a source exactly at MaxSourceBytes must still be extracted")
		}
	})

	t.Run("unparseable source", func(t *testing.T) {
		if _, ok := extractor.Extract(ctx, "broken.py", []byte("def (:\n  ???\n"), nil); ok {
			t.Error("an unparseable file must have no fingerprint")
		}
		if _, ok := extractor.Extract(ctx, "broken.js", []byte("export function ( { ] }\n"), nil); ok {
			t.Error("an unparseable file must have no fingerprint")
		}
	})

	t.Run("MaxSymbols", func(t *testing.T) {
		var builder strings.Builder
		for index := 0; index < contract.MaxSymbols*3; index++ {
			builder.WriteString("def f")
			builder.WriteString(string(rune('a' + index%26)))
			builder.WriteString(pad(index))
			builder.WriteString("():\n    pass\n")
		}
		file, ok := extractor.Extract(ctx, "many.py", []byte(builder.String()), nil)
		if !ok {
			t.Fatal("expected a fingerprint")
		}
		if len(file.Symbols) != contract.MaxSymbols {
			t.Errorf("symbols = %d, want %d", len(file.Symbols), contract.MaxSymbols)
		}
	})

	t.Run("MaxSignatureRunes", func(t *testing.T) {
		var builder strings.Builder
		builder.WriteString("def wide(")
		for index := 0; index < 400; index++ {
			builder.WriteString("parameter")
			builder.WriteString(pad(index))
			builder.WriteString(", ")
		}
		builder.WriteString("last):\n    pass\n")
		file, ok := extractor.Extract(ctx, "wide.py", []byte(builder.String()), nil)
		if !ok {
			t.Fatal("expected a fingerprint")
		}
		signature := findSignature(file, "wide")
		if runes := len([]rune(signature)); runes != contract.MaxSignatureRunes {
			t.Errorf("signature runes = %d, want %d", runes, contract.MaxSignatureRunes)
		}
		if !strings.HasSuffix(signature, contract.TruncationMarker) {
			t.Errorf("a truncated signature must be marked: %q", signature)
		}
	})

	t.Run("deny gate", func(t *testing.T) {
		source := []byte("def keep(a):\n    pass\n\ndef drop(b):\n    pass\n")
		denied, ok := extractor.Extract(ctx, "gate.py", source, func(signature string) bool {
			return strings.Contains(signature, "drop")
		})
		if !ok {
			t.Fatal("expected a fingerprint")
		}
		for _, symbol := range denied.Symbols {
			if symbol.Name == "drop" {
				t.Error("a denied signature must not be published")
			}
		}
		all, _ := extractor.Extract(ctx, "gate.py", source, nil)
		if denied.FileContractHash == all.FileContractHash {
			t.Error("the file hash must be computed over the published symbols only")
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		source := read(t, "dataclasses.py")
		first, _ := extractor.Extract(ctx, "lib/dataclasses.py", source, nil)
		second, _ := extractor.Extract(ctx, "lib/dataclasses.py", source, nil)
		if first.FileContractHash != second.FileContractHash {
			t.Error("extraction must be deterministic across calls on one runtime")
		}
	})
}

func bytes(n int, b byte) []byte {
	out := make([]byte, n)
	for index := range out {
		out[index] = b
	}
	return out
}

func pad(index int) string {
	return strings.Repeat("x", 1+index%7) + string(rune('A'+index%26)) + string(rune('a'+(index/26)%26))
}
