package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	scenarioFilter := flag.String("scenario", "", "comma-separated scenario IDs to run; empty runs the full gate")
	narrate := flag.Bool("narrate", false, "print what each scenario did, for showing the loop to a person")
	flag.Parse()

	selected, err := selectScenarios(*scenarioFilter)
	if err != nil {
		return err
	}
	// A filtered run is a demonstration, not the gate, and the report file is
	// what the precision claim is read from. Writing a partial run there would
	// quietly replace seven scenarios with one and make the aggregate wrong.
	fullRun := len(selected) == len(scenarioDefinitions)

	repositoryRoot, err := findRepositoryRoot()
	if err != nil {
		return err
	}
	required, err := loadCapabilityConfig(filepath.Join(repositoryRoot, "validation", "evals", "coordination", "capabilities.json"))
	if err != nil {
		return err
	}
	temporaryRoot, err := os.MkdirTemp("/tmp", "stickguy-coordination-eval-")
	if err != nil {
		return fmt.Errorf("create evaluation temp root: %w", err)
	}
	if os.Getenv("STICKGUY_EVAL_KEEP") == "" {
		defer os.RemoveAll(temporaryRoot)
	} else {
		fmt.Fprintf(os.Stderr, "keeping evaluation state in %s\n", temporaryRoot)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	binary := filepath.Join(temporaryRoot, "stickguy")
	if err := buildStickguy(ctx, repositoryRoot, binary); err != nil {
		return err
	}
	backend, siteURL, err := startBackend(ctx, repositoryRoot, temporaryRoot)
	if err != nil {
		return err
	}
	defer backend.stop()

	started := time.Now()
	report := evaluationReport{
		SchemaVersion: "coordination-eval/v1", GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		RequiredCapabilities: sortedRequired(required),
	}
	fixtureRoot := filepath.Join(repositoryRoot, "validation", "evals", "coordination", "fixtures", "repository")
	evaluation, err := newEvaluationEnvironment(ctx, fixtureRoot, siteURL, temporaryRoot)
	if err != nil {
		return err
	}
	defer evaluation.stop()
	for _, definition := range selected {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		result := runScenario(definition, evaluation, binary, required)
		if *narrate {
			narrateScenario(result)
		}
		report.Scenarios = append(report.Scenarios, result)
	}
	aggregateReport(&report, started)
	if !*narrate {
		printTable(report)
	}
	if !fullRun {
		fmt.Printf("\nRan %d of %d scenarios; the report file is left as the last full run wrote it.\n", len(selected), len(scenarioDefinitions))
		if reportFailed(report) {
			return errors.New("coordination evaluation failed")
		}
		return nil
	}
	reportPath := filepath.Join(repositoryRoot, "coordination-eval-report.json")
	if err := writeReport(reportPath, report); err != nil {
		return err
	}
	fmt.Printf("JSON report: %s\n", reportPath)
	if reportFailed(report) {
		return errors.New("coordination evaluation failed")
	}
	return nil
}

func findRepositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(directory, "validation", "evals", "coordination", "capabilities.json")); err == nil {
				return directory, nil
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("run the coordination evaluation inside the Stickguy repository")
		}
		directory = parent
	}
}

func buildStickguy(ctx context.Context, repositoryRoot, output string) error {
	command := exec.CommandContext(ctx, "go", "build", "-o", output, "./cmd/stickguy")
	command.Dir = repositoryRoot
	contents, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build Stickguy evaluation binary: %w: %s", err, string(contents))
	}
	return nil
}

func writeReport(path string, report evaluationReport) error {
	contents, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode coordination report: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".coordination-eval-report-*")
	if err != nil {
		return fmt.Errorf("create coordination report: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure coordination report: %w", err)
	}
	_, err = temporary.Write(append(contents, '\n'))
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write coordination report: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("activate coordination report: %w", err)
	}
	return nil
}
