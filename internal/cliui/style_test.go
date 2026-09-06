package cliui

import (
	"bytes"
	"testing"
)

func TestStylesAreLimitedAndOptional(t *testing.T) {
	colored := NewTerminal(Options{Out: &bytes.Buffer{}, Color: ColorAlways})
	tests := []struct {
		style Style
		want  string
	}{
		{StyleAlert, "\x1b[31mwarning\x1b[0m"},
		{StyleLive, "\x1b[32mwarning\x1b[0m"},
		{StyleMuted, "\x1b[2mwarning\x1b[0m"},
		{StyleBold, "\x1b[1mwarning\x1b[0m"},
	}
	for _, test := range tests {
		if got := colored.Style(test.style, "warning"); got != test.want {
			t.Errorf("style %d = %q", test.style, got)
		}
	}
	if got := colored.Style(Style(255), "plain"); got != "plain" {
		t.Fatalf("unknown style = %q", got)
	}
	plain := NewTerminal(Options{Out: &bytes.Buffer{}, Color: ColorNever})
	if got := plain.Style(StyleAlert, "warning"); got != "warning" {
		t.Fatalf("plain style = %q", got)
	}
}
