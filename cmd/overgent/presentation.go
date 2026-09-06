package main

import (
	"errors"
	"io"

	"github.com/khalidM3/overgent/internal/cliui"
)

// Presentation is decided once per invocation, from the global flags, before
// any command runs. It lives at package scope because run() dispatches a large
// switch over process streams rather than threading a session value; keeping
// one write, at one place, is what makes that safe. Commands read it through
// presentationOptions so no command re-derives the decision from the
// environment and disagrees with another.
var (
	presentationNoColor bool
	presentationNoInput bool
)

// setPresentation records the global presentation flags. It is called exactly
// once, immediately after the global flag set is parsed.
func setPresentation(noColor, noInput bool) {
	presentationNoColor, presentationNoInput = noColor, noInput
}

// presentationOptions returns terminal options carrying the invocation's
// presentation decision. Callers fill in the streams they own.
func presentationOptions() cliui.Options {
	options := cliui.Options{}
	if presentationNoColor {
		options.Color = cliui.ColorNever
	}
	return options
}

// presentationTerminal builds a terminal for one command's streams.
func presentationTerminal(in io.Reader, out, err io.Writer) cliui.Terminal {
	options := presentationOptions()
	options.In, options.Out, options.Err = in, out, err
	return cliui.NewTerminal(options)
}

// interactive reports whether a command may prompt. --no-input is a promise
// that the process will never block on a person, so it is checked before the
// terminal: a TTY does not override an explicit refusal to be asked.
func interactive(terminal cliui.Terminal) bool {
	return !presentationNoInput && terminal.Interactive()
}

// errNoInput is the shared recovery for a choice that cannot be made without a
// person. It names the flag that would have supplied the answer.
func errNoInput(what, flagHint string) error {
	return errors.New(what + ".\n\nNext: pass " + flagHint + ", or re-run in a terminal without --no-input")
}
