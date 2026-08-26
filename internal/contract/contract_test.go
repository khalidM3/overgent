package contract_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stickguy/stickguy/internal/contract"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func extract(t *testing.T, path, fixture string) contract.File {
	t.Helper()
	file, ok := contract.Extract(path, read(t, fixture), nil)
	if !ok {
		t.Fatalf("%s produced no fingerprint", fixture)
	}
	return file
}

func signature(t *testing.T, file contract.File, name string) string {
	t.Helper()
	for _, symbol := range file.Symbols {
		if symbol.Name == name {
			return symbol.Signature
		}
	}
	t.Fatalf("%s has no symbol %q; got %v", file.Path, name, names(file))
	return ""
}

func names(file contract.File) []string {
	out := make([]string, 0, len(file.Symbols))
	for _, symbol := range file.Symbols {
		out = append(out, symbol.Name)
	}
	return out
}

func TestFingerprintableExtensionsOnly(t *testing.T) {
	for _, path := range []string{"a.go", "src/b.ts", "src/c.tsx", "SRC/D.TS"} {
		if !contract.Fingerprintable(path) {
			t.Fatalf("%s must be fingerprintable", path)
		}
	}
	for _, path := range []string{"README.md", "a.py", "a.json", "Makefile", "a.gox", "a.go.txt"} {
		if contract.Fingerprintable(path) {
			t.Fatalf("%s must not be fingerprintable", path)
		}
		if _, ok := contract.Extract(path, []byte("package sample\n"), nil); ok {
			t.Fatalf("%s produced a fingerprint", path)
		}
	}
}

func TestGoExtractionRecordsExportedSurfaceOnly(t *testing.T) {
	file := extract(t, "internal/sample/sample.go", "sample.go")
	want := []string{
		"Alias", "Config", "Config.Apply", "Config.Name", "Default", "MaxItems",
		"Pair", "Pair.First", "Reader", "Reader.Close", "Reader.Read", "Rotate",
	}
	if got := names(file); !slices.Equal(got, want) {
		t.Fatalf("exported surface = %v, want %v", got, want)
	}
	for name, wantSignature := range map[string]string{
		"Rotate":       "func Rotate(ctx context.Context, key string) (string, error)",
		"Config.Apply": "func (c *Config) Apply(value int) error",
		"Config":       "type Config struct",
		"Config.Name":  "Name string `json:\"name\"`",
		"Reader.Read":  "Read(ctx context.Context, count int) ([]byte, error)",
		"Pair":         "type Pair[T any] struct",
		"Alias":        "type Alias = Config",
		"MaxItems":     "const MaxItems = 10",
		"Default":      "var Default = Config{}",
	} {
		if got := signature(t, file, name); got != wantSignature {
			t.Fatalf("%s signature = %q, want %q", name, got, wantSignature)
		}
	}
}

func TestGoBodyAndCommentChangesDoNotChangeTheContract(t *testing.T) {
	before, ok := contract.Extract("a.go", []byte("package a\n\n// Doc.\nfunc Rotate(key string) error { return nil }\n"), nil)
	if !ok {
		t.Fatal("no fingerprint")
	}
	after, ok := contract.Extract("a.go", []byte("package a\n\n// A different comment entirely.\nfunc Rotate(key string) error {\n\tprintln(key)\n\treturn nil\n}\n"), nil)
	if !ok {
		t.Fatal("no fingerprint")
	}
	if before.FileContractHash != after.FileContractHash {
		t.Fatal("a body-only or comment-only edit changed the file contract hash")
	}
	renamed, ok := contract.Extract("a.go", []byte("package a\n\nfunc Rotate(key string, at int) error { return nil }\n"), nil)
	if !ok {
		t.Fatal("no fingerprint")
	}
	if renamed.FileContractHash == before.FileContractHash {
		t.Fatal("a signature change left the file contract hash unchanged")
	}
}

func TestGoSyntaxErrorYieldsNoFingerprintAndNoError(t *testing.T) {
	if _, ok := contract.Extract("a.go", []byte("package a\n\nfunc Broken( {\n"), nil); ok {
		t.Fatal("an unparseable Go file produced a fingerprint")
	}
}

func TestTypeScriptExtractionCoversEachRecognizedForm(t *testing.T) {
	file := extract(t, "src/declarations.ts", "declarations.ts")
	want := map[string]struct{ kind, signature string }{
		"rotate":    {"function", "export function rotate<T extends { id: string }>(input: T, at: number): { ok: boolean }"},
		"refresh":   {"function", "export async function refresh(token: string): Promise<void>"},
		"Store":     {"class", "export abstract class Store extends Base<{ key: string }> implements Lifecycle"},
		"Session":   {"interface", "export interface Session extends Something"},
		"Mode":      {"type", `export type Mode = | "structural" | "semantic"`},
		"Predicate": {"type", "export type Predicate = (value: string) => boolean"},
		"LIMIT":     {"const", "export const LIMIT: number"},
		"Layer":     {"enum", "export const enum Layer"},
		"Fidelity":  {"enum", "export enum Fidelity"},
	}
	if len(file.Symbols) != len(want) {
		t.Fatalf("symbols = %v, want exactly %d entries", names(file), len(want))
	}
	for _, symbol := range file.Symbols {
		expected, ok := want[symbol.Name]
		if !ok {
			t.Fatalf("unexpected symbol %q", symbol.Name)
		}
		if symbol.Kind != expected.kind {
			t.Fatalf("%s kind = %q, want %q", symbol.Name, symbol.Kind, expected.kind)
		}
		if symbol.Signature != expected.signature {
			t.Fatalf("%s signature = %q, want %q", symbol.Name, symbol.Signature, expected.signature)
		}
	}
}

func TestTypeScriptUnparseableFileYieldsNoFingerprintAndNoError(t *testing.T) {
	if _, ok := contract.Extract("src/unparseable.ts", read(t, "unparseable.ts"), nil); ok {
		t.Fatal("an unterminated block comment produced a fingerprint")
	}
	if _, ok := contract.Extract("src/a.ts", []byte("export interface A {\n  id: string;\n"), nil); ok {
		t.Fatal("unbalanced braces produced a fingerprint")
	}
	if _, ok := contract.Extract("src/a.ts", []byte("export const label = \"unterminated\n"), nil); ok {
		t.Fatal("an unterminated string produced a fingerprint")
	}
}

func TestTypeScriptBodyOnlyEditKeepsTheContract(t *testing.T) {
	before, _ := contract.Extract("a.tsx", []byte("export function View(props: Props) {\n  return null;\n}\n"), nil)
	after, ok := contract.Extract("a.tsx", []byte("export function View(props: Props) {\n  // rewritten\n  return <div />;\n}\n"), nil)
	if !ok {
		t.Fatal("no fingerprint")
	}
	if before.FileContractHash != after.FileContractHash {
		t.Fatal("a TSX body-only edit changed the file contract hash")
	}
}

func TestEmptySurfaceHashesTheEmptyStream(t *testing.T) {
	file, ok := contract.Extract("a.go", []byte("package a\n\nfunc unexported() {}\n"), nil)
	if !ok {
		t.Fatal("no fingerprint")
	}
	if len(file.Symbols) != 0 {
		t.Fatalf("symbols = %v, want none", names(file))
	}
	const emptyStream = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if file.FileContractHash != emptyStream {
		t.Fatalf("empty surface hash = %s, want %s", file.FileContractHash, emptyStream)
	}
}

func TestSignatureIsBoundedAndMarked(t *testing.T) {
	long := "package a\n\nfunc Wide(" + strings.Repeat("argument int, ", 200) + "last int) {}\n"
	file, ok := contract.Extract("a.go", []byte(long), nil)
	if !ok {
		t.Fatal("no fingerprint")
	}
	got := signature(t, file, "Wide")
	if length := len([]rune(got)); length != contract.MaxSignatureRunes {
		t.Fatalf("signature length = %d, want %d", length, contract.MaxSignatureRunes)
	}
	if !strings.HasSuffix(got, contract.TruncationMarker) {
		t.Fatalf("truncated signature is not marked: %q", got)
	}
}

func TestDeniedSignatureLeavesTheHashConsistent(t *testing.T) {
	source := []byte("package a\n\nconst Token = \"ghp_aaaaaaaaaaaaaaaaaaaa\"\n\nfunc Rotate() {}\n")
	denied, ok := contract.Extract("a.go", source, func(value string) bool { return strings.Contains(value, "ghp_") })
	if !ok {
		t.Fatal("no fingerprint")
	}
	if got := names(denied); !slices.Equal(got, []string{"Rotate"}) {
		t.Fatalf("denied surface = %v, want [Rotate]", got)
	}
	withoutToken, _ := contract.Extract("a.go", []byte("package a\n\nfunc Rotate() {}\n"), nil)
	if denied.FileContractHash != withoutToken.FileContractHash {
		t.Fatal("a denied symbol left the file contract hash out of step with the published symbols")
	}
}

func TestOversizedSourceHasNoFingerprint(t *testing.T) {
	oversized := append([]byte("package a\n"), make([]byte, contract.MaxSourceBytes)...)
	if _, ok := contract.Extract("a.go", oversized, nil); ok {
		t.Fatal("an oversized file produced a fingerprint")
	}
}
