package multilang

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/stickguy/stickguy/internal/contract"
)

// declaration is one raw find from a language rule, before normalization.
type declaration struct {
	name string
	kind string
	raw  string
}

// assemble mirrors the tail of contract.Extract: gate, sort, bound, hash. It
// is duplicated rather than imported because the production tail is unexported
// and this spike must not modify the production extraction path. Any drift
// between the two is a defect in this spike, not a proposed change.
func assemble(path string, found []declaration, deny func(signature string) bool) contract.File {
	symbols := make([]contract.Symbol, 0, len(found))
	for _, item := range found {
		symbol := newSymbol(item.name, item.kind, item.raw)
		if deny != nil && deny(symbol.Signature) {
			continue
		}
		symbols = append(symbols, symbol)
	}
	slices.SortFunc(symbols, func(left, right contract.Symbol) int {
		if c := strings.Compare(left.Name, right.Name); c != 0 {
			return c
		}
		if c := strings.Compare(left.Kind, right.Kind); c != 0 {
			return c
		}
		return strings.Compare(left.SignatureHash, right.SignatureHash)
	})
	if len(symbols) > contract.MaxSymbols {
		symbols = symbols[:contract.MaxSymbols]
	}
	return contract.File{Path: path, FileContractHash: fileContractHash(symbols), Symbols: symbols}
}

func fileContractHash(symbols []contract.Symbol) string {
	digest := sha256.New()
	for _, symbol := range symbols {
		digest.Write([]byte(symbol.Name + ":" + symbol.SignatureHash + "\n"))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func newSymbol(name, kind, rawSignature string) contract.Symbol {
	signature := normalizeSignature(rawSignature)
	sum := sha256.Sum256([]byte(signature))
	return contract.Symbol{Name: name, Kind: kind, Signature: signature, SignatureHash: hex.EncodeToString(sum[:])}
}

func normalizeSignature(raw string) string {
	signature := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(signature) <= contract.MaxSignatureRunes {
		return signature
	}
	runes := []rune(signature)
	keep := contract.MaxSignatureRunes - utf8.RuneCountInString(contract.TruncationMarker)
	return string(runes[:keep]) + contract.TruncationMarker
}
