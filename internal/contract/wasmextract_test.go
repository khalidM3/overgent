package contract_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/overgent/overgent/internal/contract"
	"github.com/overgent/overgent/internal/contract/multilang"
	"github.com/overgent/overgent/internal/contract/wasmgrammar"
)

// The wasm-backed languages must honor exactly the contract the Go and
// TypeScript extractors honor (ADR-063): never error, yield no fingerprint on
// anything unparseable or oversized, respect the bounds, gate every signature,
// and leave the hash unchanged for a body-only edit.

func TestPythonExtractsExportedSurface(t *testing.T) {
	source := []byte(`import os


def rotate(session_id, policy):
    return os.getpid()


class Session:
    def refresh(self, token):
        return token

    def _private(self):
        return None


def _helper():
    return 1
`)
	file, ok := contract.Extract("backend/session.py", source, nil)
	if !ok {
		t.Fatal("python source produced no fingerprint")
	}
	names := symbolNames(file)
	for _, want := range []string{"rotate", "Session", "Session.refresh"} {
		if !slicesContains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	// A leading underscore is Python's convention for a private name, so it is
	// not part of the exported surface a consumer can depend on.
	for _, unwanted := range []string{"_helper", "Session._private"} {
		if slicesContains(names, unwanted) {
			t.Fatalf("private %s must not be exported: %v", unwanted, names)
		}
	}
}

func TestJavaScriptExtractsESMAndCommonJS(t *testing.T) {
	esm := []byte(`export function equal(a, b) {
  return a === b;
}

export const SCHEMES = {};

export class Parser {
  parse(input) {
    return input;
  }
}
`)
	file, ok := contract.Extract("frontend/uri.js", esm, nil)
	if !ok {
		t.Fatal("ESM source produced no fingerprint")
	}
	for _, want := range []string{"equal", "SCHEMES", "Parser"} {
		if !slicesContains(symbolNames(file), want) {
			t.Fatalf("missing ESM export %s in %v", want, symbolNames(file))
		}
	}

	// Real-world JavaScript is largely CommonJS. Recovering nothing here is the
	// specific failure that ruled out reusing the TypeScript token scanner: an
	// empty exported surface reads as a stable contract rather than as an
	// unsupported file.
	commonjs := []byte(`exports.name = 'convertPathData';

exports.fn = (root, params) => {
  return root;
};

module.exports.description = 'optimizes path data';
`)
	file, ok = contract.Extract("plugins/convert.cjs", commonjs, nil)
	if !ok {
		t.Fatal("CommonJS source produced no fingerprint")
	}
	if len(file.Symbols) == 0 {
		t.Fatal("CommonJS source produced an empty exported surface")
	}
	for _, want := range []string{"name", "fn", "description"} {
		if !slicesContains(symbolNames(file), want) {
			t.Fatalf("missing CommonJS export %s in %v", want, symbolNames(file))
		}
	}
}

func TestWasmBodyOnlyChangeLeavesHashUnchanged(t *testing.T) {
	// The whole fingerprint design rests on this: editing a body must produce
	// no wire traffic and no finding.
	before, ok := contract.Extract("a.py", []byte("def rotate(session_id):\n    return 1\n"), nil)
	if !ok {
		t.Fatal("no fingerprint for the original")
	}
	after, ok := contract.Extract("a.py", []byte("def rotate(session_id):\n    # rewritten entirely\n    total = 2\n    return total\n"), nil)
	if !ok {
		t.Fatal("no fingerprint after the body edit")
	}
	if before.FileContractHash != after.FileContractHash {
		t.Fatalf("body-only edit moved the hash: %s vs %s", before.FileContractHash, after.FileContractHash)
	}

	renamed, ok := contract.Extract("a.py", []byte("def rotate(session_id, policy):\n    return 1\n"), nil)
	if !ok {
		t.Fatal("no fingerprint after the signature change")
	}
	if renamed.FileContractHash == before.FileContractHash {
		t.Fatal("a changed signature must move the hash")
	}
}

func TestJavaScriptBodyOnlyChangeLeavesHashUnchanged(t *testing.T) {
	before, ok := contract.Extract("a.js", []byte("export function equal(a, b) {\n  return a === b;\n}\n"), nil)
	if !ok {
		t.Fatal("no fingerprint for the original")
	}
	after, ok := contract.Extract("a.js", []byte("export function equal(a, b) {\n  // rewritten: compare strictly\n  const same = a === b;\n  return same;\n}\n"), nil)
	if !ok {
		t.Fatal("no fingerprint after the body edit")
	}
	if before.FileContractHash != after.FileContractHash {
		t.Fatalf("body-only edit moved the hash: %s vs %s", before.FileContractHash, after.FileContractHash)
	}
}

func TestPythonHeaderStopsAtTheBodyColon(t *testing.T) {
	// Each of these must reduce to the same declaration header. The colons in
	// annotations, defaults and the trailing comment are not the body colon.
	for _, source := range []string{
		"def load(mapping: dict[str, int] = {'a': 1}) -> dict[str, int]:\n    return mapping\n",
		"def load(mapping: dict[str, int] = {'a': 1}) -> dict[str, int]:  # note: rebuilt\n    return mapping\n",
		"def load(mapping: dict[str, int] = {'a': 1}) -> dict[str, int]:\n    # note: rebuilt\n    total = 0\n    return mapping\n",
	} {
		file, ok := contract.Extract("a.py", []byte(source), nil)
		if !ok {
			t.Fatalf("no fingerprint for %q", source)
		}
		if len(file.Symbols) != 1 {
			t.Fatalf("expected one symbol, got %d for %q", len(file.Symbols), source)
		}
		want := "def load(mapping: dict[str, int] = {'a': 1}) -> dict[str, int]:"
		if file.Symbols[0].Signature != want {
			t.Fatalf("signature %q\n    want %q", file.Symbols[0].Signature, want)
		}
	}
}

func TestWasmUnparseableAndOversizedYieldNoFingerprint(t *testing.T) {
	if _, ok := contract.Extract("a.py", []byte("def broken(:\n    pass\n"), nil); ok {
		t.Fatal("unparseable python produced a fingerprint")
	}
	if _, ok := contract.Extract("a.js", []byte("export function ( {\n"), nil); ok {
		t.Fatal("unparseable javascript produced a fingerprint")
	}
	oversized := append([]byte("def rotate():\n    return '"), make([]byte, contract.MaxSourceBytes)...)
	if _, ok := contract.Extract("a.py", oversized, nil); ok {
		t.Fatal("oversized source produced a fingerprint")
	}
}

func TestWasmDenyGateDropsSymbolAndHash(t *testing.T) {
	source := []byte("def rotate(token='ghp_examplevalue'):\n    return token\n\n\ndef safe(a):\n    return a\n")
	denied, ok := contract.Extract("a.py", source, func(signature string) bool {
		return strings.Contains(signature, "ghp_")
	})
	if !ok {
		t.Fatal("no fingerprint under the deny gate")
	}
	if slicesContains(symbolNames(denied), "rotate") {
		t.Fatalf("denied signature survived: %v", symbolNames(denied))
	}
	// The hash must agree with the symbols that were actually published, or a
	// gated symbol desynchronizes the two.
	withoutDenied, ok := contract.Extract("a.py", []byte("def safe(a):\n    return a\n"), nil)
	if !ok {
		t.Fatal("no fingerprint for the comparison source")
	}
	if denied.FileContractHash != withoutDenied.FileContractHash {
		t.Fatal("gated symbol left the hash disagreeing with the symbol list")
	}
}

func TestWasmStatusIsHonest(t *testing.T) {
	available, reason := contract.WasmStatus()
	if !available && reason == "" {
		t.Fatal("unavailable wasm runtime must report a reason")
	}
	if available && reason != "" {
		t.Fatalf("available runtime reported a reason: %q", reason)
	}
}

func symbolNames(file contract.File) []string {
	out := make([]string, 0, len(file.Symbols))
	for _, symbol := range file.Symbols {
		out = append(out, symbol.Name)
	}
	return out
}

func slicesContains(haystack []string, needle string) bool {
	for _, value := range haystack {
		if value == needle {
			return true
		}
	}
	return false
}

func TestGrammarsLoadLazilyAndOnlyOnce(t *testing.T) {
	// The whole point of ADR-063's per-language split: a repository pays for
	// the languages it contains, not for the ones Overgent supports. This is
	// asserted rather than assumed because the cost of getting it wrong is
	// invisible — everything still works, just slower and fatter.
	//
	// A fresh extractor is used rather than the process-wide one, because the
	// shared instance accumulates whatever other tests in this package loaded.
	extractor := multilang.New(wasmgrammar.Module, false)
	defer func() { _ = extractor.Close(context.Background()) }()

	if loaded := extractor.Loaded(); len(loaded) != 0 {
		t.Fatalf("a new extractor compiled something before any file: %v", loaded)
	}

	if _, ok := extractor.Extract(context.Background(), "a.py", []byte("def f():\n    return 1\n"), nil); !ok {
		t.Fatal("python extraction failed")
	}
	if loaded := extractor.Loaded(); !slices.Equal(loaded, []string{"python"}) {
		t.Fatalf("one python file compiled %v, want only python", loaded)
	}

	// A second file of the same language must reuse the compiled grammar.
	if _, ok := extractor.Extract(context.Background(), "b.py", []byte("def g():\n    return 2\n"), nil); !ok {
		t.Fatal("second python extraction failed")
	}
	if loaded := extractor.Loaded(); !slices.Equal(loaded, []string{"python"}) {
		t.Fatalf("a second python file changed the loaded set: %v", loaded)
	}

	// A different language compiles its own module and nothing else.
	if _, ok := extractor.Extract(context.Background(), "a.js", []byte("export function f() { return 1; }\n"), nil); !ok {
		t.Fatal("javascript extraction failed")
	}
	if loaded := extractor.Loaded(); !slices.Equal(loaded, []string{"javascript", "python"}) {
		t.Fatalf("loaded %v, want exactly javascript and python", loaded)
	}

	// A language with no rules never reaches the loader at all.
	if _, ok := extractor.Extract(context.Background(), "a.rb", []byte("def f\nend\n"), nil); ok {
		t.Fatal("ruby produced a fingerprint")
	}
	if slicesContains(extractor.Loaded(), "ruby") {
		t.Fatal("ruby was loaded despite having no rules")
	}
}

// TestUnroutedGrammarsAreNeverCompiled pins the other half of the saving: the
// TypeScript and TSX modules are embedded for a future migration, and until
// that migration they must cost binary size but no memory.
//
// The gate is in this package, not in multilang: multilang knows the grammar so
// the migration is a routing change rather than a rebuild, and contract decides
// that .ts still belongs to the hand-written scanner.
func TestUnroutedGrammarsAreNeverCompiled(t *testing.T) {
	if _, ok := contract.Extract("a.ts", []byte("export const a = 1;\n"), nil); !ok {
		t.Fatal("typescript produced no fingerprint at all")
	}
	for _, unrouted := range []string{"typescript", "tsx"} {
		if slicesContains(contract.LoadedGrammars(), unrouted) {
			t.Fatalf("%s grammar was compiled although .ts still uses the scanner", unrouted)
		}
	}

	// The discriminator: this repository's own domain.ts defeats the scanner's
	// documented regex-literal desync, and tree-sitter parses it cleanly. Still
	// getting no fingerprint proves the scanner is what ran.
	source, err := os.ReadFile("../../convex/src/domain.ts")
	if err != nil {
		t.Skipf("domain.ts unavailable: %v", err)
	}
	if _, ok := contract.Extract("convex/src/domain.ts", source, nil); ok {
		t.Fatal("domain.ts produced a fingerprint, so .ts is no longer on the scanner")
	}
}
