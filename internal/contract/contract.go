// Package contract derives per-file API-surface fingerprints for the paths the
// manifest pipeline already observes (ADR-044, ADR-048). A fingerprint names
// only the exported surface of a file: symbol names, a normalized declaration
// signature with the body and comments removed, and a hash per signature. Raw
// source, bodies, and diffs never leave this package.
//
// Go, TypeScript and TSX use their own extractors; Python, JavaScript, Java,
// Rust, C#, PHP, C, C++, Scala, Kotlin and Dart are parsed by tree-sitter.
// Every other extension has no fingerprint and therefore never produces a
// contract finding.
//
// Go extraction uses the standard library go/parser and go/ast. TypeScript and
// TSX extraction uses a bounded scanner written in pure Go; it is deliberately
// best-effort, and typescript.go documents the exact recognized forms and their
// limitations. Python and JavaScript are parsed by tree-sitter grammars running
// as WebAssembly under wazero (ADR-063), which keeps ADR-019's CGO-free,
// never-invoke-Node boundary because the C toolchain is a build-time dependency
// producing a vendored module rather than a link-time one.
//
// Extraction never returns an error. A file that cannot be parsed, is too
// large, or is not fingerprintable simply has no fingerprint, so manifest
// publication is never blocked by a parse failure.
package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/khalidM3/overgent/internal/contract/fingerprint"
)

// The fingerprint value types and their bounds live in the leaf package
// internal/contract/fingerprint so the tree-sitter extractor under this package
// can share them without an import cycle (ADR-063). They are aliased rather
// than wrapped so every existing caller of contract.File and contract.Symbol
// keeps compiling against the same type.

// MaxSignatureRunes bounds a normalized signature. A longer declaration is
// truncated and marked so a reader can tell the text is not the whole header.
const MaxSignatureRunes = fingerprint.MaxSignatureRunes

// TruncationMarker terminates a signature that exceeded MaxSignatureRunes.
const TruncationMarker = fingerprint.TruncationMarker

// MaxSourceBytes refuses files large enough to make extraction unbounded work.
const MaxSourceBytes = fingerprint.MaxSourceBytes

// MaxSymbols bounds the exported surface recorded for one file.
const MaxSymbols = fingerprint.MaxSymbols

// Symbol is one exported declaration in a file's API surface.
type Symbol = fingerprint.Symbol

// File is the fingerprint of one repository-relative path.
type File = fingerprint.File

// Fingerprintable reports whether a repository-relative path has a contract
// fingerprint at all. Extraction and comparison are limited to these languages.
//
// It answers the static question "is this a language Overgent fingerprints",
// not "will extraction succeed here and now". A wasm-backed language stays
// fingerprintable on a platform without wazero's compiler; Extract returns no
// fingerprint there and WasmStatus explains why. Reporting the language as
// unsupported instead would hide a platform gap behind a language gap, which
// ADR-063 forbids.
func Fingerprintable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".ts", ".tsx":
		return true
	}
	return wasmFingerprintable(path)
}

// Extract derives the fingerprint of one file. The second result is false when
// the path is not fingerprintable, the source is too large, or the source could
// not be parsed; callers treat that as "no fingerprint", never as an error.
//
// deny is the wire gate for derived signature text. A symbol whose normalized
// signature is denied is omitted from the symbol list and from the file
// contract hash, so a gated symbol can never desynchronize the hash from the
// symbols that are actually published. A nil deny filters nothing.
func Extract(path string, source []byte, deny func(signature string) bool) (File, bool) {
	if !Fingerprintable(path) || len(source) > MaxSourceBytes {
		return File{}, false
	}
	// A wasm-backed language assembles its own File, because the tree-sitter
	// extractor already applies the same gate, sort, bound and hash tail.
	if wasmFingerprintable(path) {
		return extractWasm(path, source, deny)
	}
	var symbols []Symbol
	var ok bool
	if strings.EqualFold(filepath.Ext(path), ".go") {
		symbols, ok = extractGo(source)
	} else {
		symbols, ok = extractTypeScript(source)
	}
	if !ok {
		return File{}, false
	}
	kept := symbols[:0]
	for _, symbol := range symbols {
		if deny != nil && deny(symbol.Signature) {
			continue
		}
		kept = append(kept, symbol)
	}
	symbols = kept
	slices.SortFunc(symbols, func(left, right Symbol) int {
		if c := strings.Compare(left.Name, right.Name); c != 0 {
			return c
		}
		if c := strings.Compare(left.Kind, right.Kind); c != 0 {
			return c
		}
		return strings.Compare(left.SignatureHash, right.SignatureHash)
	})
	if len(symbols) > MaxSymbols {
		symbols = symbols[:MaxSymbols]
	}
	if symbols == nil {
		symbols = []Symbol{}
	}
	return File{Path: path, FileContractHash: fileContractHash(symbols), Symbols: symbols}, true
}

// fileContractHash is SHA-256 over the sorted symbol list. Each symbol
// contributes exactly "name:signatureHash\n"; the empty surface hashes the
// empty byte string. This encoding is the comparison key and must not be
// inferred from ordinary JSON serialization.
func fileContractHash(symbols []Symbol) string {
	digest := sha256.New()
	for _, symbol := range symbols {
		digest.Write([]byte(symbol.Name + ":" + symbol.SignatureHash + "\n"))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// newSymbol normalizes a raw declaration and hashes it.
func newSymbol(name, kind, rawSignature string) Symbol {
	signature := normalizeSignature(rawSignature)
	sum := sha256.Sum256([]byte(signature))
	return Symbol{Name: name, Kind: kind, Signature: signature, SignatureHash: hex.EncodeToString(sum[:])}
}

// normalizeSignature collapses every run of whitespace to a single space so
// reformatting a declaration is not mistaken for a contract change, then bounds
// the result to MaxSignatureRunes.
func normalizeSignature(raw string) string {
	signature := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(signature) <= MaxSignatureRunes {
		return signature
	}
	runes := []rune(signature)
	keep := MaxSignatureRunes - utf8.RuneCountInString(TruncationMarker)
	return string(runes[:keep]) + TruncationMarker
}
