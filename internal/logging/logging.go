// Package logging defines privacy-safe logging composition shared by local modes.
package logging

import (
	"io"
	"log/slog"
)

// New returns a JSON logger. Callers must use allowlisted fields; prohibited
// content and secret values must never be passed to logging in the first place.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// Secret is intentionally non-serializable credential material.
type Secret struct{ value string }

func NewSecret(value string) Secret { return Secret{value: value} }
func (Secret) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }
func (s Secret) Bytes() []byte      { return []byte(s.value) }
