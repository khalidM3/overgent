package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSecretLogValueIsRedacted(t *testing.T) {
	var out bytes.Buffer
	New(&out, slog.LevelDebug).Info("fixture", "credential", NewSecret("do-not-print"))
	if strings.Contains(out.String(), "do-not-print") || !strings.Contains(out.String(), "[REDACTED]") {
		t.Fatalf("unsafe secret rendering: %s", out.String())
	}
}
