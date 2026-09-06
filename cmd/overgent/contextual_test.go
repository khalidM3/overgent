package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/cliui"
	"github.com/khalidM3/overgent/internal/config"
)

func TestRunInitNonInteractiveRoutesToExistingLocalCreate(t *testing.T) {
	var called []string
	err := runInit([]string{"--local", "--label", "Atlas", "--root", "/repo"}, "/state", "https://api.example.com", false, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, func(args []string) error {
		called = append([]string(nil), args...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--config-root", "/state", "create", "--root", "/repo", "--label", "Atlas", "--local"}
	if !reflect.DeepEqual(called, want) {
		t.Fatalf("called = %#v, want %#v", called, want)
	}
}

func TestRunInitNeverPromptsOnRedirectedInput(t *testing.T) {
	err := runInit(nil, "/state", "https://api.example.com", false, strings.NewReader("1\n"), &bytes.Buffer{}, &bytes.Buffer{}, func([]string) error { t.Fatal("unexpected mutation"); return nil })
	// The refusal must name the flag that would have answered the question.
	if err == nil || !strings.Contains(err.Error(), "--local") || !strings.Contains(err.Error(), "--join") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunPrivacyJSONNamesBoundaryWithoutSensitiveLocalValues(t *testing.T) {
	state, repo := t.TempDir(), t.TempDir()
	t.Chdir(repo)
	paths, _ := config.Resolve(state)
	cfg := config.Single("http://127.0.0.1:43103", "dev_secret", []config.Workspace{{ID: "wsp_1", ProjectID: "prj_1", Root: repo}})
	if err := config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := runPrivacy(context.Background(), paths, []string{"--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var got privacyOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != "prj_1" || got.Mode != "local" || len(got.WireBlocked) == 0 {
		t.Fatalf("privacy = %+v", got)
	}
	if strings.Contains(stdout.String(), "dev_secret") || strings.Contains(stdout.String(), repo) {
		t.Fatalf("privacy leaked local identifier/path: %s", stdout.String())
	}
}

// --no-input is a promise the process will never block on a person, so a real
// terminal must not override it.
func TestRunInitRefusesToPromptUnderGlobalNoInput(t *testing.T) {
	setPresentation(false, true)
	t.Cleanup(func() { setPresentation(false, false) })
	err := runInit(nil, "/state", "https://api.example.com", false, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, func([]string) error {
		t.Fatal("unexpected mutation")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "--no-input") {
		t.Fatalf("error = %v", err)
	}
}

// The first-run screen is the only thing a member sees before anything exists.
// It must name one front door, stay inside a narrow window, and never dim the
// commands it is asking them to type.
func TestFirstRunScreenOffersOneFrontDoorAtEveryWidth(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		var output bytes.Buffer
		terminal := cliui.NewTerminal(cliui.Options{Out: &output, Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: width})
		if err := writeFirstRun(&output, terminal); err != nil {
			t.Fatal(err)
		}
		text := output.String()
		for _, want := range []string{"overgent init", "--local", "--team", "--join", "Nothing is shared"} {
			if !strings.Contains(text, want) {
				t.Errorf("at %d columns first run omits %q", width, want)
			}
		}
		// `create` and `join` are the direct forms; the front door is `init`.
		// Naming both here is what makes a first run feel like two products.
		if strings.Contains(text, "overgent create") || strings.Contains(text, "overgent join") {
			t.Errorf("at %d columns first run advertises a second front door:\n%s", width, text)
		}
		for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			if len(line) > width {
				t.Errorf("at %d columns a line ran to %d: %q", width, len(line), line)
			}
		}
	}
}
