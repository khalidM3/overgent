package wasmgrammar_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"slices"
	"testing"

	"github.com/khalidM3/overgent/internal/contract/wasmgrammar"
)

// Each module is a compiled binary in a public repository that runs on every
// member's machine, so an artifact must not drift from the record in
// PROVENANCE.md without someone noticing.
var expected = []struct {
	name   string
	digest string
	size   int
}{
	{name: "c", digest: "d49243c502fb6b2a2a3810fa917f71e90575b413056c7e1e1f40eac8f06c1e52", size: 106295},
	{name: "c_sharp", digest: "1788f3641ffbb9115f6770f7117af1c4b8c42114ec2a7b387fd60ffa75789764", size: 370358},
	{name: "cpp", digest: "643df66b2db836cf23db921797140bc12c9711af86fca0e5644f5544cde5fe6f", size: 455798},
	{name: "dart", digest: "7f7a24906927e32ba0b36d7004e8906354579db52337ab6bcf356ee9af11e7ee", size: 143513},
	{name: "java", digest: "e7da5238278c9810f8916b83bb9b9b722b575464138fa43c5292f76e8837cdc6", size: 83990},
	{name: "javascript", digest: "6e3283152cd82f3dab11e03c73c040ec39b33700b18ce5311f9d9bcc1b9b47d1", size: 85103},
	{name: "kotlin", digest: "836ffc9b46b7ddadc40fee36a5496cec2209cd7b4ced127160db607b95a0d421", size: 426463},
	{name: "php", digest: "8f1df367dfc53aa915a33bb364bc84d3f0d16eaf588b136e3894ef9dd9f3d9e6", size: 139784},
	{name: "python", digest: "0de9a5848a549ae2f538b82127a94ab8941bb7d4817b2e5fe9bc4ecdaad468a8", size: 98998},
	{name: "rust", digest: "cec541058d7dd0de1d680ee4056d8f56db2ffa6328ffd7ca43ccd64575a4cc7e", size: 149257},
	{name: "scala", digest: "13fffbe3b60b46f7fbc0afa7b0d42b4171a94527e5a4378570bfed209614451e", size: 464498},
	{name: "tsx", digest: "2c76e268c442a62fbfbb30786eca0cdf9f7cd5a6be6fb2c25abefaedfd4c1577", size: 170993},
	{name: "typescript", digest: "b0c3ad46cfad4abdf3ce721d96fa7593b5f0b0a31f352e5ce4c9dc875302205d", size: 167716},
}

func TestEmbeddedModulesMatchProvenance(t *testing.T) {
	for _, want := range expected {
		raw, err := os.ReadFile("modules/" + want.name + ".wasm.gz")
		if err != nil {
			t.Errorf("reading %s: %v", want.name, err)
			continue
		}
		if len(raw) != want.size {
			t.Errorf("%s is %d bytes, PROVENANCE.md records %d", want.name, len(raw), want.size)
		}
		sum := sha256.Sum256(raw)
		if digest := hex.EncodeToString(sum[:]); digest != want.digest {
			t.Errorf("%s digest %s does not match PROVENANCE.md %s", want.name, digest, want.digest)
		}
	}
}

// TestLanguagesMatchesTheRecord catches a module added to or removed from the
// directory without the record being updated, which the per-module hash check
// above cannot see on its own.
func TestLanguagesMatchesTheRecord(t *testing.T) {
	got := wasmgrammar.Languages()
	names := make([]string, 0, len(expected))
	for _, want := range expected {
		names = append(names, want.name)
	}
	if !slices.Equal(got, names) {
		t.Fatalf("embedded modules %v do not match the recorded set %v", got, names)
	}
}

func TestModuleInflatesAndReportsAbsence(t *testing.T) {
	module, exists, err := wasmgrammar.Module("python")
	if err != nil || !exists {
		t.Fatalf("loading python: exists=%v err=%v", exists, err)
	}
	// A wasm module starts with the four-byte magic \0asm.
	if len(module) < 4 || string(module[:4]) != "\x00asm" {
		t.Fatal("inflated bytes are not a wasm module")
	}

	// A language with no embedded module is absent, not an error: the caller
	// reads that as "no fingerprint" rather than as a failure.
	if _, exists, err := wasmgrammar.Module("cobol"); exists || err != nil {
		t.Fatalf("unknown language reported exists=%v err=%v", exists, err)
	}
}
