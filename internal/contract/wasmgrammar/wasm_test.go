package wasmgrammar_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/stickguy/stickguy/internal/contract/wasmgrammar"
)

// expectedDigest pins the committed grammar module. It is a compiled binary in
// a public repository that runs on every member's machine, so the artifact must
// not drift from the record in PROVENANCE.md without someone noticing.
const (
	expectedDigest = "302e3aeffa7a243691c85d22b29d0f6dc8b1272b480d23b78a578d76789dcd01"
	expectedSize   = 986162
)

func TestEmbeddedGrammarMatchesProvenance(t *testing.T) {
	raw, err := os.ReadFile("ts-multilang.wasm.gz")
	if err != nil {
		t.Fatalf("reading committed module: %v", err)
	}
	if len(raw) != expectedSize {
		t.Fatalf("module is %d bytes, PROVENANCE.md records %d", len(raw), expectedSize)
	}
	sum := sha256.Sum256(raw)
	if digest := hex.EncodeToString(sum[:]); digest != expectedDigest {
		t.Fatalf("module digest %s does not match PROVENANCE.md %s", digest, expectedDigest)
	}
}

func TestMultilangInflates(t *testing.T) {
	module, err := wasmgrammar.Multilang()
	if err != nil {
		t.Fatalf("inflating module: %v", err)
	}
	if len(module) == 0 {
		t.Fatal("inflated module is empty")
	}
	// A wasm module starts with the four-byte magic \0asm.
	if len(module) < 4 || string(module[:4]) != "\x00asm" {
		t.Fatal("inflated bytes are not a wasm module")
	}
}
