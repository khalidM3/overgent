package cliui

import (
	"bytes"
	"os"
	"testing"
)

func TestTerminalCapabilitiesRespectStreamsAndEnvironment(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	environment := map[string]string{"LANG": "en_US.UTF-8"}
	terminal := NewTerminal(Options{
		In:         in,
		Out:        out,
		Err:        &bytes.Buffer{},
		LookupEnv:  lookup(environment),
		IsTerminal: func(stream any) bool { return stream == in || stream == out },
	})
	if !terminal.Interactive() || !terminal.ColorEnabled() || !terminal.UnicodeEnabled() {
		t.Fatalf("capabilities: interactive=%v color=%v unicode=%v", terminal.Interactive(), terminal.ColorEnabled(), terminal.UnicodeEnabled())
	}

	environment["NO_COLOR"] = ""
	if terminal.ColorEnabled() {
		t.Fatal("NO_COLOR did not disable color")
	}
	if !terminal.UnicodeEnabled() {
		t.Fatal("NO_COLOR incorrectly disabled Unicode")
	}

	delete(environment, "NO_COLOR")
	environment["TERM"] = "dumb"
	if terminal.ColorEnabled() || terminal.UnicodeEnabled() {
		t.Fatal("TERM=dumb retained terminal decoration")
	}
}

func TestTerminalExplicitModesAndLocale(t *testing.T) {
	buffer := &bytes.Buffer{}
	terminal := NewTerminal(Options{
		Out:        buffer,
		LookupEnv:  lookup(map[string]string{"LC_ALL": "C"}),
		IsTerminal: func(any) bool { return false },
		Color:      ColorAlways,
		Unicode:    UnicodeAuto,
	})
	if !terminal.ColorEnabled() {
		t.Fatal("ColorAlways did not override non-terminal output")
	}
	if terminal.UnicodeEnabled() {
		t.Fatal("C locale enabled Unicode")
	}
	if got := terminal.Symbol("✓", "OK"); got != "OK" {
		t.Fatalf("symbol = %q", got)
	}

	forced := NewTerminal(Options{
		Out:       buffer,
		LookupEnv: lookup(map[string]string{"TERM": "dumb", "NO_COLOR": "1"}),
		Color:     ColorNever,
		Unicode:   UnicodeAlways,
	})
	if forced.ColorEnabled() || !forced.UnicodeEnabled() {
		t.Fatalf("forced capabilities: color=%v unicode=%v", forced.ColorEnabled(), forced.UnicodeEnabled())
	}
}

func TestTerminalWidthUsesColumnsAndDefault(t *testing.T) {
	terminal := NewTerminal(Options{
		Out:       &bytes.Buffer{},
		LookupEnv: lookup(map[string]string{"COLUMNS": "112"}),
	})
	if got := terminal.Width(); got != 112 {
		t.Fatalf("width = %d", got)
	}

	invalid := NewTerminal(Options{
		Out:          &bytes.Buffer{},
		LookupEnv:    lookup(map[string]string{"COLUMNS": "4"}),
		DefaultWidth: 72,
	})
	if got := invalid.Width(); got != 72 {
		t.Fatalf("fallback width = %d", got)
	}
}

func TestIsTerminalRejectsNonFilesAndRegularFiles(t *testing.T) {
	if IsTerminal(&bytes.Buffer{}) {
		t.Fatal("buffer detected as a terminal")
	}
	file, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if IsTerminal(file) {
		t.Fatal("regular file detected as a terminal")
	}
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func TestAnimatedRequiresARealNonDumbTerminal(t *testing.T) {
	terminal := func(isTTY bool, term string) Terminal {
		return NewTerminal(Options{
			Out:        os.Stdout,
			IsTerminal: func(any) bool { return isTTY },
			LookupEnv:  func(name string) (string, bool) { return term, name == "TERM" },
		})
	}
	if !terminal(true, "xterm-256color").Animated() {
		t.Error("a real terminal should permit in-place updates")
	}
	if terminal(true, "dumb").Animated() {
		t.Error("a dumb terminal cannot be driven with cursor control")
	}
	if terminal(false, "xterm-256color").Animated() {
		t.Error("a pipe must never receive cursor control")
	}
}
