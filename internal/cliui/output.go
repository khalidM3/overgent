package cliui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// OutputMode distinguishes prose intended for a person from stable structured
// output intended for another program.
type OutputMode uint8

const (
	OutputHuman OutputMode = iota
	OutputJSON
	OutputJSONLines
)

func (mode OutputMode) MachineReadable() bool {
	return mode == OutputJSON || mode == OutputJSONLines
}

// ParseOutputMode converts the public flag spelling to a mode.
func ParseOutputMode(value string) (OutputMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "human", "text":
		return OutputHuman, nil
	case "json":
		return OutputJSON, nil
	case "jsonl", "ndjson":
		return OutputJSONLines, nil
	default:
		return OutputHuman, fmt.Errorf("unknown output format %q (use human, json, or jsonl)", value)
	}
}

// Output owns one command's output contract. Its writers are supplied by the
// caller; it never reads global process output or sends data elsewhere.
type Output struct {
	Mode OutputMode
	Out  io.Writer
	Err  io.Writer
}

func NewOutput(terminal Terminal, mode OutputMode) Output {
	return Output{Mode: mode, Out: terminal.Out(), Err: terminal.Err()}
}

// Printf emits human prose and refuses to contaminate structured stdout.
func (output Output) Printf(format string, args ...any) error {
	if output.Mode != OutputHuman {
		return errors.New("cannot write human output in machine-readable mode")
	}
	_, err := fmt.Fprintf(output.Out, format, args...)
	return err
}

// Encode writes exactly one JSON value followed by a newline.
func (output Output) Encode(value any) error {
	if output.Mode != OutputJSON {
		return errors.New("JSON encoding requires json output mode")
	}
	return json.NewEncoder(output.Out).Encode(value)
}

// EncodeLine writes one compact JSON value for a JSON Lines stream.
func (output Output) EncodeLine(value any) error {
	if output.Mode != OutputJSONLines {
		return errors.New("JSON Lines encoding requires jsonl output mode")
	}
	return json.NewEncoder(output.Out).Encode(value)
}
