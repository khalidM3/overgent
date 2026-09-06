package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/khalidM3/overgent/internal/cliui"
	"github.com/khalidM3/overgent/internal/config"
)

func runContextualRoot(ctx context.Context, paths config.Paths, stdout io.Writer) error {
	terminal := presentationTerminal(nil, stdout, stdout)
	if !terminal.OutputIsTerminal() {
		return runHelp(nil, stdout)
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	if len(cfg.Projects) == 0 {
		return writeFirstRun(stdout, terminal)
	}
	if _, err = selectStatusWorkspace(cfg, "", os.Getwd); err == nil {
		return runStatus(ctx, paths, nil)
	}
	return runProjects(ctx, paths, nil)
}

// writeFirstRun is the only screen a member sees before anything exists, so it
// names one front door and shows the direct forms beneath it. Every other
// first-run recovery in the CLI points at `init` too: naming `init` here and
// `create` elsewhere is what makes one product feel like two. It reuses the
// shared definition writer so a narrow window degrades the same way help does,
// and it leaves the commands unstyled — dimming the thing a member must type
// is the one place muted styling actively hurts.
func writeFirstRun(stdout io.Writer, terminal cliui.Terminal) error {
	muted := func(text string) string { return terminal.Style(cliui.StyleMuted, text) }
	if _, err := fmt.Fprintf(stdout, "%s\n", terminal.Style(cliui.StyleBold, "OVERGENT")); err != nil {
		return err
	}
	if err := writeProse(stdout, terminal, muted, "Air traffic control for coding agents"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if err := writeProse(stdout, terminal, func(text string) string { return text }, "No Project is registered on this device yet."); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "\n%s\n", terminal.Style(cliui.StyleBold, "START HERE")); err != nil {
		return err
	}
	entries := []cliFlag{
		{"overgent init", "Set one up, guided"},
		{"overgent init --local", "Keep coordination on this computer"},
		{"overgent init --team", "Create a Project your team shares"},
		{"overgent init --join INVITE", "Join a Project you were invited to"},
	}
	nameWidth := 0
	for _, entry := range entries {
		if length := len(entry.Name); length > nameWidth {
			nameWidth = length
		}
	}
	if err := writeDefinitions(stdout, terminal, entries, nameWidth, func(text string) string { return text }); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	return writeProse(stdout, terminal, muted, "Nothing is shared until you choose a Project.")
}

func selectDashboardProject(ctx context.Context, cfg config.Config, stdin io.Reader, stdout io.Writer) (string, error) {
	if workspace, err := selectStatusWorkspace(cfg, "", os.Getwd); err == nil {
		return workspace.ProjectID, nil
	}
	if len(cfg.Projects) == 0 {
		return "", errors.New("no Project is registered on this device.\n\nNext: run `overgent init` inside a Git repository")
	}
	if len(cfg.Projects) == 1 {
		return cfg.Projects[0].ID, nil
	}
	terminal := presentationTerminal(stdin, stdout, stdout)
	if !interactive(terminal) {
		return "", errors.New("this directory is not registered with a Project.\n\nNext: run `overgent projects`, then re-run with `--project ID`")
	}
	labels, _ := projectLabels(ctx, cfg, defaultStatusCLI().bootstrap)
	choices := make([]cliui.Choice, 0, len(cfg.Projects))
	for _, project := range cfg.Projects {
		label := labels[project.ID]
		if label == "" {
			label = project.ID
		}
		backend, _ := cfg.BackendForProject(project.ID)
		choices = append(choices, cliui.Choice{Label: label, Description: backend.Kind})
	}
	choice, err := cliui.Select(terminal, "OPEN A PROJECT", "This directory is not registered. Choose where to go:", choices)
	if err != nil {
		return "", err
	}
	return cfg.Projects[choice].ID, nil
}

func renderCLIError(err error, stderr io.Writer) {
	terminal := presentationTerminal(nil, stderr, stderr)
	if !terminal.OutputIsTerminal() {
		fmt.Fprintln(stderr, err)
		return
	}
	fmt.Fprintf(stderr, "%s %s\n", terminal.Style(cliui.StyleAlert, terminal.Symbol("!", "!")), terminal.Style(cliui.StyleBold, "Overgent couldn't complete that"))
	for _, line := range cliui.Wrap(err.Error(), max(20, terminal.Width()-2)) {
		if line == "" {
			fmt.Fprintln(stderr)
			continue
		}
		fmt.Fprintf(stderr, "  %s\n", line)
	}
}

type privacyOutput struct {
	SchemaVersion int      `json:"schemaVersion"`
	ProjectID     string   `json:"projectId"`
	Mode          string   `json:"mode"`
	Server        string   `json:"server"`
	MaySync       []string `json:"maySync"`
	StaysLocal    []string `json:"staysLocal"`
	WireBlocked   []string `json:"wireBlocked"`
}

var maySyncFacts = []string{"intents and dependency claims", "contract fingerprints and manifests", "bounded diff summaries", "finding evidence and resolution outcomes", "classified session activity"}
var localInputs = []string{"source and Git data used to derive coordination facts", "vendor transcripts and local adapter state", "environment and credential stores"}
var prohibitedWire = []string{"raw source files or diffs", "Git objects", "environment values or credentials", "prompts or transcript files", "raw tool results, command lines, or command output"}

func runPrivacy(_ context.Context, paths config.Paths, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("privacy", flag.ContinueOnError)
	flags.SetOutput(stderr)
	projectID := flags.String("project", "", "select a Project by id")
	jsonOutput := flags.Bool("json", false, "emit stable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("privacy accepts no positional arguments")
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	workspace, err := selectStatusWorkspace(cfg, *projectID, os.Getwd)
	if err != nil {
		return err
	}
	backend, ok := cfg.BackendForWorkspace(workspace)
	if !ok {
		return errors.New("the selected Project has no configured backend; run `overgent doctor`")
	}
	out := privacyOutput{cliOutputSchemaVersion, workspace.ProjectID, backend.Kind, backend.APIBaseURL, maySyncFacts, localInputs, prohibitedWire}
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(out)
	}
	location := backend.APIBaseURL
	if backend.Kind == config.KindLocal {
		location = "this computer (loopback only)"
	}
	_, err = fmt.Fprintf(stdout, "Privacy · %s Project\n\nServer\n  %s\n\nMay sync\n  - %s\n\nRead locally, never sent raw\n  - %s\n\nBlocked at the wire\n  - %s\n\nSharing consent comes from Project membership. Use `overgent pause --project %s` to stop outbound sharing from this device.\n", backend.Kind, location, joinLines(maySyncFacts), joinLines(localInputs), joinLines(prohibitedWire), workspace.ProjectID)
	return err
}

func joinLines(items []string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for _, item := range items[1:] {
		result += "\n  - " + item
	}
	return result
}
