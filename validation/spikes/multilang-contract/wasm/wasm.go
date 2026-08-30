// Package wasm forwards to the production grammar module.
//
// The spike built and embedded its own copy of the module. On acceptance of
// ADR-063 that artifact moved to internal/contract/wasmgrammar, which is now
// the single authoritative copy with its provenance and hash assertion. This
// package forwards there rather than committing the same 398 KB binary twice.
package wasm

import "github.com/stickguy/stickguy/internal/contract/wasmgrammar"

// Multilang returns the wasm module, inflating it on first use.
func Multilang() ([]byte, error) { return wasmgrammar.Multilang() }
