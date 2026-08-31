// Package fingerprint holds the contract-fingerprint value types and their
// bounds. It exists so that both the parent contract package and the
// tree-sitter-backed extractor under it can share one definition of a
// fingerprint without an import cycle (ADR-063): multilang needs File, Symbol
// and the bounds, while contract needs to route to multilang.
//
// It is deliberately a leaf: types, constants, and nothing that parses.
package fingerprint

// MaxSignatureRunes bounds a normalized signature. A longer declaration is
// truncated and marked so a reader can tell the text is not the whole header.
const MaxSignatureRunes = 500

// TruncationMarker terminates a signature that exceeded MaxSignatureRunes.
const TruncationMarker = "…"

// MaxSourceBytes refuses files large enough to make extraction unbounded work.
const MaxSourceBytes = 1 << 20

// MaxSymbols bounds the exported surface recorded for one file.
const MaxSymbols = 200

// Symbol is one exported declaration in a file's API surface.
type Symbol struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Signature     string `json:"signature"`
	SignatureHash string `json:"signatureHash"`
}

// File is the fingerprint of one repository-relative path.
type File struct {
	Path             string   `json:"path"`
	FileContractHash string   `json:"fileContractHash"`
	Symbols          []Symbol `json:"symbols"`
}
