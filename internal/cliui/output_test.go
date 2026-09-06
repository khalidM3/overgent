package cliui

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseOutputMode(t *testing.T) {
	for input, want := range map[string]OutputMode{
		"":       OutputHuman,
		"text":   OutputHuman,
		"JSON":   OutputJSON,
		"ndjson": OutputJSONLines,
	} {
		got, err := ParseOutputMode(input)
		if err != nil || got != want {
			t.Errorf("ParseOutputMode(%q) = %v, %v", input, got, err)
		}
	}
	if _, err := ParseOutputMode("xml"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestOutputKeepsHumanAndMachineContractsSeparate(t *testing.T) {
	var out bytes.Buffer
	terminal := NewTerminal(Options{Out: &out, Err: &bytes.Buffer{}})
	human := NewOutput(terminal, OutputHuman)
	if err := human.Printf("Project %s\n", "Acme"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "Project Acme\n" {
		t.Fatalf("human output = %q", got)
	}
	if err := human.Encode(map[string]string{"status": "clear"}); err == nil {
		t.Fatal("human mode accepted JSON")
	}

	out.Reset()
	jsonOutput := NewOutput(terminal, OutputJSON)
	if err := jsonOutput.Printf("progress"); err == nil {
		t.Fatal("JSON mode accepted prose")
	}
	if err := jsonOutput.Encode(map[string]string{"status": "clear"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "{\"status\":\"clear\"}\n" {
		t.Fatalf("JSON output = %q", got)
	}

	out.Reset()
	lines := NewOutput(terminal, OutputJSONLines)
	if err := lines.EncodeLine(struct {
		Event string `json:"event"`
	}{Event: "updated"}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("JSONL output = %q", out.String())
	}
}
