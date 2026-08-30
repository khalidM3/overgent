package contract_test

import (
	"strings"
	"testing"

	"github.com/stickguy/stickguy/internal/contract"
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
