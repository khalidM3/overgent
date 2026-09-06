package cliui

import (
	"errors"
	"fmt"
	"io"
	"os"
)

var ErrCancelled = errors.New("selection cancelled")

type Choice struct {
	Label       string
	Description string
}

// Select presents a compact native terminal picker. It uses arrow keys (or
// j/k), Enter to accept, and Escape/q/Ctrl-C to cancel. Callers should retain a
// numbered-line fallback for non-interactive streams.
func Select(terminal Terminal, title, detail string, choices []Choice) (int, error) {
	input, inputOK := terminal.in.(*os.File)
	output, outputOK := terminal.out.(*os.File)
	if !terminal.Interactive() || !inputOK || !outputOK || len(choices) == 0 {
		return -1, errors.New("interactive selection requires a terminal")
	}
	restore, err := makeRaw(input)
	if err != nil {
		return -1, err
	}
	defer restore()
	defer fmt.Fprint(output, "\x1b[?25h\n")
	fmt.Fprint(output, "\x1b[?25l")
	selected := 0
	render := func(first bool) {
		if !first {
			fmt.Fprintf(output, "\x1b[%dA", len(choices))
		}
		for index, choice := range choices {
			fmt.Fprint(output, "\x1b[2K")
			if index == selected {
				fmt.Fprintf(output, "  %s %s", terminal.Style(StyleLive, terminal.Symbol("›", ">")), terminal.Style(StyleBold, choice.Label))
			} else {
				fmt.Fprintf(output, "    %s", choice.Label)
			}
			if choice.Description != "" {
				fmt.Fprintf(output, "  %s", terminal.Style(StyleMuted, choice.Description))
			}
			fmt.Fprintln(output)
		}
	}
	fmt.Fprintf(output, "%s\n", terminal.Style(StyleBold, title))
	if detail != "" {
		fmt.Fprintf(output, "%s\n", terminal.Style(StyleMuted, detail))
	}
	fmt.Fprintf(output, "%s\n\n", terminal.Style(StyleMuted, "↑/↓ move  enter select  esc cancel"))
	render(true)
	for {
		key, err := readSelectionKey(input)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return -1, ErrCancelled
			}
			return -1, err
		}
		switch key {
		case keyUp:
			selected = (selected - 1 + len(choices)) % len(choices)
			render(false)
		case keyDown:
			selected = (selected + 1) % len(choices)
			render(false)
		case keyEnter:
			return selected, nil
		case keyCancel:
			return -1, ErrCancelled
		}
	}
}

type selectionKey uint8

const (
	keyIgnore selectionKey = iota
	keyUp
	keyDown
	keyEnter
	keyCancel
)

func readSelectionKey(input *os.File) (selectionKey, error) {
	var first [1]byte
	if _, err := input.Read(first[:]); err != nil {
		return keyIgnore, err
	}
	switch first[0] {
	case '\r', '\n':
		return keyEnter, nil
	case 3, 'q':
		return keyCancel, nil
	case 'k':
		return keyUp, nil
	case 'j':
		return keyDown, nil
	case 27:
		if !inputReady(input, 35) {
			return keyCancel, nil
		}
		var sequence [2]byte
		if _, err := io.ReadFull(input, sequence[:]); err != nil {
			return keyCancel, nil
		}
		if sequence[0] == '[' && sequence[1] == 'A' {
			return keyUp, nil
		}
		if sequence[0] == '[' && sequence[1] == 'B' {
			return keyDown, nil
		}
		return keyIgnore, nil
	default:
		return keyIgnore, nil
	}
}
