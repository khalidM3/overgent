package cliui

import (
	"os"
	"testing"
)

func TestSelectionKeysSupportArrowsEnterAndEscape(t *testing.T) {
	for _, test := range []struct {
		input string
		want  selectionKey
	}{{"\x1b[A", keyUp}, {"\x1b[B", keyDown}, {"\r", keyEnter}, {"j", keyDown}, {"k", keyUp}} {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = writer.WriteString(test.input); err != nil {
			t.Fatal(err)
		}
		_ = writer.Close()
		got, err := readSelectionKey(reader)
		_ = reader.Close()
		if err != nil || got != test.want {
			t.Fatalf("key %q = (%v, %v), want %v", test.input, got, err, test.want)
		}
	}
	reader, writer, _ := os.Pipe()
	_, _ = writer.WriteString("\x1b")
	_ = writer.Close()
	got, err := readSelectionKey(reader)
	_ = reader.Close()
	if err != nil || got != keyCancel {
		t.Fatalf("escape = (%v, %v)", got, err)
	}
}
