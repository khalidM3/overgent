// Package cliui contains the terminal-facing presentation primitives shared by
// Overgent commands. It deliberately knows nothing about Projects, credentials,
// repositories, or the service protocol.
package cliui

import (
	"io"
	"os"
	"strconv"
	"strings"
)

const defaultTerminalWidth = 80

// ColorMode controls ANSI styling. Auto is suitable for normal command use.
type ColorMode uint8

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

// UnicodeMode controls whether presentation glyphs may use Unicode.
type UnicodeMode uint8

const (
	UnicodeAuto UnicodeMode = iota
	UnicodeAlways
	UnicodeNever
)

// Options defines the streams and environment used for terminal detection.
// LookupEnv and IsTerminal are injectable so command tests never need to alter
// the process environment or depend on the terminal running the test suite.
type Options struct {
	In           io.Reader
	Out          io.Writer
	Err          io.Writer
	LookupEnv    func(string) (string, bool)
	IsTerminal   func(any) bool
	Color        ColorMode
	Unicode      UnicodeMode
	DefaultWidth int
}

// Terminal is an immutable description of one command invocation's terminal.
type Terminal struct {
	in           io.Reader
	out          io.Writer
	err          io.Writer
	lookupEnv    func(string) (string, bool)
	isTerminal   func(any) bool
	colorMode    ColorMode
	unicodeMode  UnicodeMode
	defaultWidth int
}

// NewTerminal returns a terminal description with safe process defaults.
func NewTerminal(options Options) Terminal {
	if options.In == nil {
		options.In = os.Stdin
	}
	if options.Out == nil {
		options.Out = os.Stdout
	}
	if options.Err == nil {
		options.Err = os.Stderr
	}
	if options.LookupEnv == nil {
		options.LookupEnv = os.LookupEnv
	}
	if options.IsTerminal == nil {
		options.IsTerminal = IsTerminal
	}
	if options.DefaultWidth <= 0 {
		options.DefaultWidth = defaultTerminalWidth
	}
	return Terminal{
		in:           options.In,
		out:          options.Out,
		err:          options.Err,
		lookupEnv:    options.LookupEnv,
		isTerminal:   options.IsTerminal,
		colorMode:    options.Color,
		unicodeMode:  options.Unicode,
		defaultWidth: options.DefaultWidth,
	}
}

func (terminal Terminal) In() io.Reader  { return terminal.in }
func (terminal Terminal) Out() io.Writer { return terminal.out }
func (terminal Terminal) Err() io.Writer { return terminal.err }

// InputIsTerminal reports whether input is an interactive terminal.
func (terminal Terminal) InputIsTerminal() bool { return terminal.isTerminal(terminal.in) }

// OutputIsTerminal reports whether human output is attached to a terminal.
func (terminal Terminal) OutputIsTerminal() bool { return terminal.isTerminal(terminal.out) }

// Interactive is true only when both input and output are terminals. Callers
// should use this before prompting; output being a TTY alone is insufficient.
func (terminal Terminal) Interactive() bool {
	return terminal.InputIsTerminal() && terminal.OutputIsTerminal()
}

// ColorEnabled applies common CLI conventions: automatic color requires a TTY,
// is disabled by NO_COLOR even when its value is empty, and is disabled for a
// dumb terminal. An explicit ColorAlways overrides environment detection.
func (terminal Terminal) ColorEnabled() bool {
	switch terminal.colorMode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	}
	if !terminal.OutputIsTerminal() || terminal.termIsDumb() {
		return false
	}
	_, noColor := terminal.lookupEnv("NO_COLOR")
	return !noColor
}

// UnicodeEnabled reports whether glyphs should use their Unicode form.
// Explicit C/POSIX or non-UTF locales get ASCII, as does TERM=dumb. A missing
// locale is treated as UTF-8 because Go strings and supported modern terminals
// are UTF-8; callers can always request UnicodeNever.
func (terminal Terminal) UnicodeEnabled() bool {
	switch terminal.unicodeMode {
	case UnicodeAlways:
		return true
	case UnicodeNever:
		return false
	}
	if terminal.termIsDumb() {
		return false
	}
	locale := terminal.locale()
	if locale == "" {
		return true
	}
	normalized := strings.ToLower(strings.TrimSpace(locale))
	if normalized == "c" || normalized == "posix" {
		return false
	}
	return strings.Contains(normalized, "utf-8") || strings.Contains(normalized, "utf8")
}

// Animated reports whether the caller may rewrite the current line in place.
// It requires a real terminal and refuses a dumb one, where cursor control is
// not dependable. Callers must render a correct, complete result without it:
// in-place updating is an enhancement, never the only way a fact is shown.
func (terminal Terminal) Animated() bool {
	return terminal.OutputIsTerminal() && !terminal.termIsDumb()
}

// Symbol selects a Unicode glyph or its plain ASCII equivalent.
func (terminal Terminal) Symbol(unicodeValue, asciiValue string) string {
	if terminal.UnicodeEnabled() {
		return unicodeValue
	}
	return asciiValue
}

// Width returns the output terminal width, then a valid COLUMNS value, then the
// configured default. Implausible widths are ignored to keep layouts usable.
func (terminal Terminal) Width() int {
	if file, ok := terminal.out.(*os.File); ok {
		if width, ok := terminalWidth(file); ok && validWidth(width) {
			return width
		}
	}
	if raw, ok := terminal.lookupEnv("COLUMNS"); ok {
		if width, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && validWidth(width) {
			return width
		}
	}
	return terminal.defaultWidth
}

func (terminal Terminal) termIsDumb() bool {
	value, _ := terminal.lookupEnv("TERM")
	return strings.EqualFold(strings.TrimSpace(value), "dumb")
}

func (terminal Terminal) locale() string {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if value, ok := terminal.lookupEnv(name); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func validWidth(width int) bool { return width >= 20 && width <= 1000 }

// IsTerminal checks real file-backed streams. Non-file readers and writers are
// deliberately non-interactive so buffers, pipes, and test doubles never cause
// an accidental prompt.
func IsTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	return ok && fileIsTerminal(file)
}
