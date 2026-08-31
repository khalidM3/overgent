package multilang

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/stickguy/stickguy/internal/contract/fingerprint"
)

// declaration is one raw find from a language rule, before normalization.
type declaration struct {
	name string
	kind string
	raw  string
}

// assemble mirrors the tail of contract.Extract: gate, sort, dedupe, bound,
// hash. It is duplicated rather than imported because that tail is unexported
// in the parent package, and importing it would be an import cycle. Any drift
// between the two is a defect.
func assemble(path string, found []declaration, deny func(signature string) bool) fingerprint.File {
	symbols := make([]fingerprint.Symbol, 0, len(found))
	for _, item := range found {
		symbol := newSymbol(item.name, item.kind, item.raw)
		if deny != nil && deny(symbol.Signature) {
			continue
		}
		symbols = append(symbols, symbol)
	}
	slices.SortFunc(symbols, func(left, right fingerprint.Symbol) int {
		if c := strings.Compare(left.Name, right.Name); c != 0 {
			return c
		}
		if c := strings.Compare(left.Kind, right.Kind); c != 0 {
			return c
		}
		return strings.Compare(left.SignatureHash, right.SignatureHash)
	})
	// One declaration can legitimately be seen twice — a C prototype in a
	// header and its definition in the same translation unit reduce to the same
	// signature — and publishing it twice would inflate the symbol count and
	// make an unchanged file look different from itself. Identical entries are
	// adjacent after the sort, so compaction is enough; entries that share a
	// name but not a hash are genuinely different surface and both survive.
	symbols = slices.Compact(symbols)
	if len(symbols) > fingerprint.MaxSymbols {
		symbols = symbols[:fingerprint.MaxSymbols]
	}
	return fingerprint.File{Path: path, FileContractHash: fileContractHash(symbols), Symbols: symbols}
}

func fileContractHash(symbols []fingerprint.Symbol) string {
	digest := sha256.New()
	for _, symbol := range symbols {
		digest.Write([]byte(symbol.Name + ":" + symbol.SignatureHash + "\n"))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func newSymbol(name, kind, rawSignature string) fingerprint.Symbol {
	signature := normalizeSignature(rawSignature)
	sum := sha256.Sum256([]byte(signature))
	return fingerprint.Symbol{Name: name, Kind: kind, Signature: signature, SignatureHash: hex.EncodeToString(sum[:])}
}

func normalizeSignature(raw string) string {
	signature := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(signature) <= fingerprint.MaxSignatureRunes {
		return signature
	}
	runes := []rune(signature)
	keep := fingerprint.MaxSignatureRunes - utf8.RuneCountInString(fingerprint.TruncationMarker)
	return string(runes[:keep]) + fingerprint.TruncationMarker
}
