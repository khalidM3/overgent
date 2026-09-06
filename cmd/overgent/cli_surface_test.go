package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/cliui"
)

func TestRootHelpGroupsEveryPublicCommandAndHidesIntegrationSurfaces(t *testing.T) {
	var output bytes.Buffer
	if err := runHelp(nil, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, category := range cliCategories {
		if !strings.Contains(text, "\n"+strings.ToUpper(category)+"\n") {
			t.Errorf("help omits category %q", category)
		}
	}
	for _, command := range cliCommands {
		listed := strings.Contains(text, "  "+command.Name+" ")
		if command.Internal && listed {
			t.Errorf("root help advertises internal command %q", command.Name)
		}
		if !command.Internal && !listed {
			t.Errorf("help omits command %q", command.Name)
		}
	}
	if strings.Contains(text, "Room") {
		t.Error("help uses prohibited user-facing Room terminology")
	}
}

// Internal commands stay reachable by name even though root help omits them, so
// an operator following a support note or a hook trace is never dead-ended.
func TestInternalCommandsRemainAddressableByName(t *testing.T) {
	for _, command := range cliCommands {
		if !command.Internal {
			continue
		}
		var output bytes.Buffer
		if err := runHelp([]string{command.Name}, &output); err != nil {
			t.Fatalf("help %s = %v", command.Name, err)
		}
		text := output.String()
		if !strings.Contains(text, "Managed integration surface") && !strings.Contains(text, "Kept for compatibility") {
			t.Errorf("help %s does not explain why it is unlisted:\n%s", command.Name, text)
		}
	}
}

// Help that lists a bare flag name forces a trip to the source, so an entry
// without a description is treated as an incomplete command.
func TestEveryCatalogueFlagCarriesADescription(t *testing.T) {
	for _, command := range cliCommands {
		for _, item := range command.Flags {
			if !strings.HasPrefix(item.Name, "--") {
				t.Errorf("%s flag %q is not spelled with a leading --", command.Name, item.Name)
			}
			if strings.TrimSpace(item.Description) == "" {
				t.Errorf("%s flag %q has no description", command.Name, item.Name)
			}
		}
	}
	for _, item := range cliGlobalFlags {
		if strings.TrimSpace(item.Description) == "" {
			t.Errorf("global flag %q has no description", item.Name)
		}
	}
}

// Every public command must land in one of the documented job groups; a typo in
// Category would otherwise silently drop it out of root help.
func TestEveryPublicCommandHasADocumentedCategory(t *testing.T) {
	known := map[string]bool{}
	for _, category := range cliCategories {
		known[category] = true
	}
	for _, command := range cliCommands {
		if command.Internal {
			continue
		}
		if !known[command.Category] {
			t.Errorf("command %q has undocumented category %q", command.Name, command.Category)
		}
	}
}

func TestCommandHelp(t *testing.T) {
	var output bytes.Buffer
	if err := runHelp([]string{"create"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Create a Project", "--local", "--label", "--config-root"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("create help omits %q", want)
		}
	}
	if err := runHelp([]string{"missing"}, &output); err == nil || !strings.Contains(err.Error(), "overgent help") {
		t.Fatalf("unknown command error = %v", err)
	}
}

func TestCompletionScriptsCoverCatalog(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			var output bytes.Buffer
			if err := runCompletion([]string{shell}, &output); err != nil {
				t.Fatal(err)
			}
			text := output.String()
			for _, command := range cliCommands {
				if !strings.Contains(text, command.Name) {
					t.Errorf("%s completion omits command %q", shell, command.Name)
				}
				for _, item := range command.Flags {
					if !strings.Contains(text, strings.TrimPrefix(item.Name, "--")) {
						t.Errorf("%s completion omits %s flag %q", shell, command.Name, item.Name)
					}
				}
			}
			for _, item := range cliGlobalFlags {
				if !strings.Contains(text, strings.TrimPrefix(item.Name, "--")) {
					t.Errorf("%s completion omits global flag %q", shell, item.Name)
				}
			}
		})
	}
}

func TestCompletionRejectsInvalidInvocation(t *testing.T) {
	for _, args := range [][]string{nil, {"bash", "zsh"}, {"powershell"}} {
		if err := runCompletion(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("runCompletion(%q) succeeded", args)
		}
	}
}

func TestCommandCatalogIsUniqueAndUnknownCommandSuggests(t *testing.T) {
	seen := map[string]bool{}
	for _, command := range cliCommands {
		if seen[command.Name] {
			t.Fatalf("duplicate command %q", command.Name)
		}
		seen[command.Name] = true
	}
	for _, test := range []struct{ typed, want string }{{"statsu", "status"}, {"vers", "version"}, {"proj", "projects"}} {
		err := unknownCommandError(test.typed)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("unknownCommandError(%q) = %v, want a %q suggestion", test.typed, err, test.want)
		}
	}
	// A command that routes badly must never be offered back to the member.
	if err := unknownCommandError("version"); err == nil || strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("exact catalogue hit suggested itself: %v", err)
	}
}

// Section 11 requires usable layouts at 40, 80, and 120 columns. Help is the
// widest two-column surface, so it is the one that breaks first.
func TestHelpStaysReadableAtEveryDocumentedWidth(t *testing.T) {
	command, ok := findCLICommand("ai")
	if !ok {
		t.Fatal("ai command missing from catalogue")
	}
	for _, width := range []int{40, 80, 120} {
		var output bytes.Buffer
		terminal := cliui.NewTerminal(cliui.Options{Out: &output, Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: width})
		if err := writeFlagList(&output, terminal, command.Flags); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimRight(output.String(), "\n"), "\n") {
			if len(line) > width {
				t.Errorf("at %d columns a flag line ran to %d: %q", width, len(line), line)
			}
		}
		// Descriptions must survive the narrow fallback, not be dropped.
		if !strings.Contains(output.String(), "deterministic") {
			t.Errorf("at %d columns flag descriptions were lost:\n%s", width, output.String())
		}
	}
}

// Root help must never contain ANSI escapes when color is refused, in either
// spelling: the global flag or the environment.
func TestRootHelpEmitsNoAnsiWhenColorIsRefused(t *testing.T) {
	setPresentation(true, false)
	t.Cleanup(func() { setPresentation(false, false) })
	var output bytes.Buffer
	if err := runHelp(nil, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Errorf("help emitted ANSI under --no-color:\n%q", output.String())
	}
}

// Root help must degrade the same way command help does. Usage lines are the
// one documented exception: they stay verbatim so they remain copyable.
func TestRootHelpFitsEveryDocumentedWidth(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		var output bytes.Buffer
		setPresentation(true, false)
		t.Cleanup(func() { setPresentation(false, false) })
		terminal := cliui.NewTerminal(cliui.Options{Out: &output, Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: width})
		entries := make([]cliFlag, 0, len(cliCommands))
		for _, command := range cliCommands {
			if !command.Internal {
				entries = append(entries, cliFlag{command.Name, command.Summary})
			}
		}
		if err := writeDefinitions(&output, terminal, entries, commandNameWidth(), func(text string) string { return text }); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimRight(output.String(), "\n"), "\n") {
			if len(line) > width {
				t.Errorf("at %d columns a summary line ran to %d: %q", width, len(line), line)
			}
		}
	}
}

// `dashboard` and `open` must remain one behavior. A second entry that did its
// own thing is exactly the divergence section 4 forbids.
func TestDeprecatedAliasesDoNotLeadHelpButStayAddressable(t *testing.T) {
	alias, ok := findCLICommand("dashboard")
	if !ok {
		t.Fatal("dashboard alias was removed; existing scripts would break")
	}
	if !alias.Internal {
		t.Error("a deprecated alias is leading ordinary help")
	}
	if !strings.Contains(alias.Summary, "open") {
		t.Errorf("the alias does not name its replacement: %q", alias.Summary)
	}
	primary, ok := findCLICommand("open")
	if !ok || primary.Internal {
		t.Fatal("open is not the listed destination command")
	}
}

// Section 4 allows an alias only when it routes to the same implementation.
// Both spellings must therefore appear in exactly one dispatch case.
func TestOpenAndDashboardShareOneDispatch(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, `case "open", "dashboard":`) {
		t.Error("open and dashboard are no longer one dispatch case")
	}
	if strings.Contains(text, `case "dashboard":`) {
		t.Error("dashboard grew a second implementation")
	}
}
