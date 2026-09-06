package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/khalidM3/overgent/internal/cliui"
)

// cliFlag pairs a flag with the sentence a member needs to choose it. Help that
// lists bare flag names forces a trip to the source, so a flag without a
// description is treated as an incomplete command entry.
type cliFlag struct {
	Name        string
	Description string
}

// cliCommand is deliberately presentation-only. It describes the command
// surface without reaching configuration, the service, or a backend, so help
// and completion remain safe on a brand-new or offline installation.
type cliCommand struct {
	Name        string
	Category    string
	Summary     string
	Usage       string
	Subcommands []string
	Flags       []cliFlag
	// Internal marks a managed integration or maintainer surface. It stays a
	// first-class command for completion and `overgent help NAME`, but never
	// leads ordinary help (cli-experience.md section 4).
	Internal bool
}

var (
	flagProject  = cliFlag{"--project", "Act on this Project id instead of the current directory"}
	flagJSON     = cliFlag{"--json", "Emit one stable JSON document on stdout"}
	flagRepoRoot = cliFlag{"--root", "Git repository root to register (default: current directory)"}
	flagLabel    = cliFlag{"--label", "Human-readable Project name"}
	flagDevice   = cliFlag{"--device-label", "Device name shared with Project members"}
)

var cliCommands = []cliCommand{
	// Daily
	{Name: "status", Category: "Daily", Summary: "Show whether anything needs you", Usage: "overgent [global flags] status [--project ID] [--json]\n  overgent [global flags] status --watch [--jsonl] [--interval DURATION]", Flags: []cliFlag{
		flagProject,
		flagJSON,
		{"--watch", "Follow the Project until Ctrl-C instead of printing once"},
		{"--jsonl", "With --watch, emit one JSON record per refresh"},
	}},
	{Name: "watch", Category: "Daily", Summary: "Follow agents and findings until Ctrl-C", Usage: "overgent [global flags] watch [--project ID] [--jsonl] [--once] [--interval DURATION]", Flags: []cliFlag{
		flagProject,
		{"--jsonl", "Emit one JSON record per refresh instead of a live view"},
		{"--once", "Render a single snapshot and exit"},
		{"--interval", "Refresh interval, at least 500ms (default 2s)"},
	}},
	{Name: "open", Category: "Daily", Summary: "Open this Project in the app, or a browser", Usage: "overgent [global flags] open [--project ID] [--web]", Flags: []cliFlag{
		flagProject,
		{"--web", "Open the browser dashboard instead of the Overgent app"},
	}},
	// `dashboard` predates `open` and routes to the same implementation. It is
	// kept addressable for scripts and old notes, but does not lead help.
	{Name: "dashboard", Summary: "Deprecated alias for `open --web`", Usage: "overgent [global flags] dashboard [--project ID]", Internal: true, Flags: []cliFlag{flagProject}},
	{Name: "pause", Category: "Daily", Summary: "Stop sharing outward from a workspace or Project", Usage: "overgent [global flags] pause (--workspace ID | --project ID)", Flags: []cliFlag{
		{"--workspace", "Pause sharing from this registered checkout"},
		{"--project", "Pause sharing from every checkout of this Project on this device"},
	}},
	{Name: "resume", Category: "Daily", Summary: "Resume sharing from a workspace or Project", Usage: "overgent [global flags] resume (--workspace ID | --project ID)", Flags: []cliFlag{
		{"--workspace", "Resume sharing from this registered checkout"},
		{"--project", "Resume sharing from every checkout of this Project on this device"},
	}},

	// Projects
	{Name: "init", Category: "Projects", Summary: "Create or join a Project, guided", Usage: "overgent [global flags] init [--local | --team | --join INVITE] [--label NAME] [--root PATH] [--no-input]", Flags: []cliFlag{
		{"--local", "Create a Project that never leaves this computer"},
		{"--team", "Create a Project that syncs bounded coordination facts"},
		{"--join", "Join an existing Project with an invite code"},
		flagLabel,
		flagRepoRoot,
		{"--no-input", "Fail instead of prompting when a choice is missing"},
	}},
	{Name: "create", Category: "Projects", Summary: "Create a Project and register this repository", Usage: "overgent [global flags] create [--local] [--label NAME] [--root PATH] [--json]", Flags: []cliFlag{
		{"--local", "Use this Mac's bundled backend; nothing leaves this computer"},
		flagLabel,
		flagDevice,
		flagRepoRoot,
		flagJSON,
	}},
	{Name: "join", Category: "Projects", Summary: "Join a Project from an invite", Usage: "overgent [global flags] join [--root PATH] [--json] INVITE", Flags: []cliFlag{
		flagDevice,
		flagRepoRoot,
		flagJSON,
	}},
	{Name: "projects", Category: "Projects", Summary: "List Projects registered on this device", Usage: "overgent [global flags] projects [--json]", Flags: []cliFlag{flagJSON}},
	{Name: "workspace", Category: "Projects", Summary: "List or register local checkouts", Usage: "overgent [global flags] workspace list\n  overgent [global flags] workspace add [flags]", Subcommands: []string{"list", "add"}, Flags: []cliFlag{
		{"--id", "Workspace id to register"},
		{"--project", "Project id this checkout belongs to"},
		{"--workstream", "Workstream id for this checkout"},
		{"--member", "Member id recorded on the workspace"},
		{"--device", "Device id recorded on the workspace"},
		{"--session", "Session id recorded on the workspace"},
		{"--api", "Backend origin serving this workspace"},
		{"--root", "Absolute path of the checkout"},
		{"--development", "Derive a second workstream from the development profile"},
	}},

	// Agents
	{Name: "setup", Category: "Agents", Summary: "Connect, inspect, repair, or remove agent adapters", Usage: "overgent [global flags] setup COMMAND [flags]", Subcommands: []string{"codex", "claude", "cursor", "status", "reconnect", "repair", "remove", "remove-all"}, Flags: []cliFlag{
		{"--project-root", "Trusted coding-agent project root (default: current directory)"},
		{"--agent", "Agent for status and remove: codex, claude, or cursor"},
		{"--development", "Install the local development MCP adapter instead"},
	}},
	{Name: "focus", Category: "Agents", Summary: "Temporarily quiet inbound coordination for a session", Usage: "overgent [global flags] focus --session ID [--minutes N]", Flags: []cliFlag{
		{"--session", "Agent session workstream id to quiet"},
		{"--minutes", "How long to stay quiet; default 60, maximum 480"},
	}},
	{Name: "unfocus", Category: "Agents", Summary: "Resume coordination delivery to a session", Usage: "overgent [global flags] unfocus --session ID", Flags: []cliFlag{
		{"--session", "Agent session workstream id to un-quiet"},
	}},
	{Name: "intent", Category: "Agents", Summary: "Report what this workstream is trying to do", Usage: "overgent [global flags] intent --workspace ID --title TEXT --outcome TEXT [--approach TEXT]", Flags: []cliFlag{
		{"--workspace", "Workspace id this intent describes"},
		{"--title", "Short workstream title"},
		{"--outcome", "Intended outcome"},
		{"--approach", "Optional approach summary"},
	}},
	{Name: "scan", Category: "Agents", Summary: "Refresh local repository evidence now", Usage: "overgent [global flags] scan"},

	// Configuration
	{Name: "ai", Category: "Configuration", Summary: "Inspect or configure optional Project intelligence", Usage: "overgent [global flags] ai status [--project ID] [--json]\n  overgent [global flags] ai set [flags]\n  overgent [global flags] ai clear [--judgment] [--embeddings]", Subcommands: []string{"status", "set", "clear"}, Flags: []cliFlag{
		flagProject,
		flagJSON,
		{"--judgment-provider", "anthropic, openai-compatible, or none"},
		{"--judgment-model", "Judgment model name"},
		{"--judgment-base-url", "Judgment provider origin"},
		{"--judgment-key-stdin", "Read the judgment key from stdin"},
		{"--judgment-key-env", "Read the judgment key from this environment variable"},
		{"--embedding-provider", "openai or deterministic"},
		{"--embedding-model", "Embedding model name"},
		{"--embedding-base-url", "Embedding provider origin"},
		{"--embedding-key-stdin", "Read the embedding key from stdin"},
		{"--embedding-key-env", "Read the embedding key from this environment variable"},
		{"--judgment", "With clear: disable judgment and delete its key"},
		{"--embeddings", "With clear: use deterministic embeddings and delete the key"},
	}},
	{Name: "privacy", Category: "Configuration", Summary: "Explain what may sync and what stays local", Usage: "overgent [global flags] privacy [--project ID] [--json]", Flags: []cliFlag{flagProject, flagJSON}},

	// Maintenance
	{Name: "doctor", Category: "Maintenance", Summary: "Check service, Project, adapter, and AI health", Usage: "overgent [global flags] doctor"},
	{Name: "diagnostics", Category: "Maintenance", Summary: "Write a privacy-safe diagnostic bundle", Usage: "overgent [global flags] diagnostics"},
	{Name: "update", Category: "Maintenance", Summary: "Install an authenticated update or roll it back", Usage: "overgent [global flags] update [rollback] [--manifest URL]", Subcommands: []string{"rollback"}, Flags: []cliFlag{
		{"--manifest", "Signed update metadata URL"},
	}},
	{Name: "service", Category: "Maintenance", Summary: "Manage the per-user Overgent service", Usage: "overgent [global flags] service COMMAND", Subcommands: []string{"status", "install", "start", "stop", "remove", "run"}},
	{Name: "backend", Category: "Maintenance", Summary: "Manage local and configured Project backends", Usage: "overgent [global flags] backend COMMAND [flags]", Subcommands: []string{"list", "status", "start", "stop", "install", "verify", "reset", "export"}, Flags: []cliFlag{
		flagJSON,
		{"--binary", "Path to the convex-local-backend executable"},
		{"--bundle", "Path to the release-time deploy payload"},
		{"--yes", "Skip the confirmation prompt"},
		{"--out", "Directory to copy the stopped backend database into"},
	}},
	{Name: "reset", Category: "Maintenance", Summary: "Clear enrollment for one or all backends", Usage: "overgent [global flags] reset [--backend ID | --all] [--force]", Flags: []cliFlag{
		{"--backend", "Backend id to reset; see `overgent backend list`"},
		{"--all", "Reset every backend on this profile"},
		{"--force", "Clear local enrollment even if the credential is unverified"},
	}},
	{Name: "version", Category: "Maintenance", Summary: "Print build information", Usage: "overgent version [--json]", Flags: []cliFlag{flagJSON}},
	{Name: "help", Category: "Maintenance", Summary: "Show help for Overgent or one command", Usage: "overgent help [COMMAND]"},
	{Name: "completion", Category: "Maintenance", Summary: "Generate a shell completion script", Usage: "overgent completion bash|zsh|fish", Subcommands: []string{"bash", "zsh", "fish"}},

	// Internal: managed integration surfaces, reachable but never advertised.
	{Name: "mcp", Summary: "Run the managed stdio MCP adapter", Usage: "overgent [global flags] mcp", Internal: true},
	{Name: "agent-hook", Summary: "Receive a managed coding-agent hook event", Usage: "overgent [global flags] agent-hook --vendor VENDOR [--event EVENT]", Internal: true, Flags: []cliFlag{
		{"--vendor", "Supported coding-agent vendor"},
		{"--event", "Vendor hook event name, when the payload omits it"},
	}},
}

var cliCategories = []string{"Daily", "Projects", "Agents", "Configuration", "Maintenance"}

var cliGlobalFlags = []cliFlag{
	{"--config-root", "Use an isolated per-user state root"},
	{"--api", "Use this Overgent API origin"},
	{"--no-color", "Never emit ANSI color (NO_COLOR is honored too)"},
	{"--no-input", "Never prompt; fail with the flag to pass instead"},
}

// runHelp writes human-first help. The caller may pass no arguments for root
// help or one command name for focused help.
func runHelp(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return writeRootHelp(stdout)
	}
	if len(args) != 1 {
		return errors.New("help accepts at most one command; run `overgent help` to see every command")
	}
	command, ok := findCLICommand(args[0])
	if !ok {
		return unknownCommandError(args[0])
	}
	return writeCommandHelp(stdout, command)
}

func writeRootHelp(stdout io.Writer) error {
	terminal := presentationTerminal(nil, stdout, stdout)
	bold := func(text string) string { return terminal.Style(cliui.StyleBold, text) }
	muted := func(text string) string { return terminal.Style(cliui.StyleMuted, text) }
	if _, err := fmt.Fprintf(stdout, "%s\n", bold("OVERGENT")); err != nil {
		return err
	}
	if err := writeProse(stdout, terminal, muted, "Air traffic control for coding agents"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "\n%s\n  %s\n\n", bold("USAGE"), muted("overgent [global flags] COMMAND [flags]")); err != nil {
		return err
	}
	width := commandNameWidth()
	for _, category := range cliCategories {
		if _, err := fmt.Fprintln(stdout, bold(strings.ToUpper(category))); err != nil {
			return err
		}
		entries := make([]cliFlag, 0, len(cliCommands))
		for _, command := range cliCommands {
			if command.Internal || command.Category != category {
				continue
			}
			entries = append(entries, cliFlag{command.Name, command.Summary})
		}
		if err := writeDefinitions(stdout, terminal, entries, width, bold); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(stdout, bold("GLOBAL FLAGS")); err != nil {
		return err
	}
	if err := writeFlagList(stdout, terminal, cliGlobalFlags); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	return writeProse(stdout, terminal, muted, "Run `overgent help COMMAND` for one command's flags.")
}

func writeCommandHelp(stdout io.Writer, command cliCommand) error {
	terminal := presentationTerminal(nil, stdout, stdout)
	bold := func(text string) string { return terminal.Style(cliui.StyleBold, text) }
	muted := func(text string) string { return terminal.Style(cliui.StyleMuted, text) }
	if _, err := fmt.Fprintf(stdout, "%s\n", bold(strings.ToUpper(command.Name))); err != nil {
		return err
	}
	if err := writeProse(stdout, terminal, muted, command.Summary); err != nil {
		return err
	}
	// Usage lines are meant to be copied, so they are printed verbatim even when
	// they overhang a narrow window; wrapping them would produce a command that
	// does not run.
	if _, err := fmt.Fprintf(stdout, "\n%s\n  %s\n", bold("USAGE"), command.Usage); err != nil {
		return err
	}
	if command.Internal {
		note := "Managed integration surface. Agent setup configures this for you."
		if strings.HasPrefix(command.Summary, "Deprecated") {
			note = "Kept for compatibility. Prefer the command named above."
		}
		if _, err := fmt.Fprintf(stdout, "\n%s\n", terminal.Style(cliui.StyleMuted, note)); err != nil {
			return err
		}
	}
	if len(command.Subcommands) > 0 {
		if _, err := fmt.Fprintf(stdout, "\n%s\n  %s\n", bold("COMMANDS"), strings.Join(command.Subcommands, "  ")); err != nil {
			return err
		}
	}
	if len(command.Flags) > 0 {
		if _, err := fmt.Fprintf(stdout, "\n%s\n", bold("FLAGS")); err != nil {
			return err
		}
		if err := writeFlagList(stdout, terminal, command.Flags); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "\n%s\n", bold("GLOBAL FLAGS")); err != nil {
		return err
	}
	return writeFlagList(stdout, terminal, cliGlobalFlags)
}

// writeFlagList renders a command's flags as a definition list.
func writeFlagList(stdout io.Writer, terminal cliui.Terminal, flags []cliFlag) error {
	nameWidth := 0
	for _, item := range flags {
		if length := len(item.Name); length > nameWidth {
			nameWidth = length
		}
	}
	return writeDefinitions(stdout, terminal, flags, nameWidth, func(text string) string { return text })
}

// writeDefinitions aligns descriptions into a second column, then falls back to
// wrapping under the name when the terminal is too narrow to hold both. Root
// help and command help share it so a 40-column window degrades identically on
// both surfaces (cli-experience.md section 11). decorate styles the name only;
// it must not change the name's visible length, since column arithmetic is done
// on the undecorated string.
func writeDefinitions(stdout io.Writer, terminal cliui.Terminal, entries []cliFlag, nameWidth int, decorate func(string) string) error {
	width := terminal.Width()
	stacked := width < nameWidth+30
	for _, item := range entries {
		if item.Description == "" {
			if _, err := fmt.Fprintf(stdout, "  %s\n", decorate(item.Name)); err != nil {
				return err
			}
			continue
		}
		if stacked {
			if _, err := fmt.Fprintf(stdout, "  %s\n", decorate(item.Name)); err != nil {
				return err
			}
			for _, line := range cliui.Wrap(item.Description, max(16, width-6)) {
				if _, err := fmt.Fprintf(stdout, "      %s\n", line); err != nil {
					return err
				}
			}
			continue
		}
		indent := strings.Repeat(" ", nameWidth+4)
		lines := cliui.Wrap(item.Description, max(16, width-len(indent)))
		if _, err := fmt.Fprintf(stdout, "  %s%s%s\n", decorate(item.Name), pad(item.Name, nameWidth), lines[0]); err != nil {
			return err
		}
		for _, line := range lines[1:] {
			if _, err := fmt.Fprintf(stdout, "%s%s\n", indent, line); err != nil {
				return err
			}
		}
	}
	return nil
}

// pad returns the spacing that follows text in a two-column layout. It is
// computed separately from Printf width verbs because styled names carry ANSI
// escapes that %-*s would count as visible characters.
func pad(text string, width int) string {
	spaces := width - len(text) + 2
	if spaces < 1 {
		spaces = 1
	}
	return strings.Repeat(" ", spaces)
}

// writeProse wraps a line of explanation to the window.
func writeProse(stdout io.Writer, terminal cliui.Terminal, decorate func(string) string, text string) error {
	for _, line := range cliui.Wrap(text, max(16, terminal.Width())) {
		if _, err := fmt.Fprintf(stdout, "%s\n", decorate(line)); err != nil {
			return err
		}
	}
	return nil
}

func commandNameWidth() int {
	width := 0
	for _, command := range cliCommands {
		if !command.Internal && len(command.Name) > width {
			width = len(command.Name)
		}
	}
	return width
}

func findCLICommand(name string) (cliCommand, bool) {
	for _, command := range cliCommands {
		if command.Name == name {
			return command, true
		}
	}
	return cliCommand{}, false
}

// unknownCommandError suggests a near neighbour. It never suggests the name the
// member already typed: an exact catalogue hit reaching here means the command
// exists and failed to route, and echoing it back reads as nonsense. A prefix
// match wins over an equal-distance unrelated word, so `vers` reaches `version`
// rather than whichever same-distance command sorts first.
func unknownCommandError(name string) error {
	best, bestScore := "", 1<<30
	for _, candidate := range sortedRootCommands() {
		if candidate == name {
			continue
		}
		score := editDistance(name, candidate) * 2
		if strings.HasPrefix(candidate, name) {
			score -= 3
		}
		if score < bestScore {
			best, bestScore = candidate, score
		}
	}
	if best != "" && len(name) > 1 && bestScore <= 6 {
		return fmt.Errorf("unknown command %q; did you mean %q? Run `overgent help` to see every command", name, best)
	}
	return fmt.Errorf("unknown command %q; run `overgent help` to see every command", name)
}

func editDistance(left, right string) int {
	previous := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range []rune(left) {
		current := []int{leftIndex + 1}
		for rightIndex, rightRune := range []rune(right) {
			cost := 1
			if leftRune == rightRune {
				cost = 0
			}
			insert := current[rightIndex] + 1
			remove := previous[rightIndex+1] + 1
			replace := previous[rightIndex] + cost
			current = append(current, min(insert, remove, replace))
		}
		previous = current
	}
	return previous[len(previous)-1]
}

// runVersion answers from build-time constants only. It never loads
// configuration or contacts a service, so it stays usable for support triage on
// a broken installation, and `--json` remains the stable machine contract.
func runVersion(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOutput := flags.Bool("json", false, "emit stable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("version accepts no positional arguments")
	}
	identity, _ := executableIdentity()
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(versionInfo{version, commit, buildTime, 1, 1, identity})
	}
	terminal := presentationTerminal(nil, stdout, stderr)
	if _, err := fmt.Fprintf(stdout, "%s %s\n", terminal.Style(cliui.StyleBold, "overgent"), version); err != nil {
		return err
	}
	return cliui.WriteFields(stdout, terminal.Width(), []cliui.Field{
		{Label: "Commit", Value: commit},
		{Label: "Built", Value: buildTime},
	})
}

// runCompletion emits a static script and never reads user state or contacts a
// backend. Static generation also makes installation and packaging repeatable.
func runCompletion(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("completion requires one shell: bash, zsh, or fish")
	}
	var script string
	switch args[0] {
	case "bash":
		script = bashCompletion()
	case "zsh":
		script = zshCompletion()
	case "fish":
		script = fishCompletion()
	default:
		return fmt.Errorf("unsupported shell %q; choose bash, zsh, or fish", args[0])
	}
	_, err := io.WriteString(stdout, script)
	return err
}

func sortedRootCommands() []string {
	names := make([]string, 0, len(cliCommands))
	for _, command := range cliCommands {
		names = append(names, command.Name)
	}
	sort.Strings(names)
	return names
}

func flagNames(flags []cliFlag) []string {
	names := make([]string, 0, len(flags))
	for _, item := range flags {
		names = append(names, item.Name)
	}
	return names
}

func completionCaseLines(prefix, suffix string) string {
	var b strings.Builder
	for _, command := range cliCommands {
		words := append(append([]string{}, command.Subcommands...), flagNames(command.Flags)...)
		words = append(words, flagNames(cliGlobalFlags)...)
		fmt.Fprintf(&b, prefix, command.Name, strings.Join(words, " "))
		b.WriteString(suffix)
	}
	return b.String()
}

func bashCompletion() string {
	root := strings.Join(append(sortedRootCommands(), flagNames(cliGlobalFlags)...), " ")
	return "# bash completion for overgent\n_overgent_complete() {\n" +
		"  local cur command words\n  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n  command=\"${COMP_WORDS[1]}\"\n" +
		"  words=\"" + root + "\"\n  case \"$command\" in\n" +
		completionCaseLines("    %s) words=\"%s\" ;;\n", "") +
		"  esac\n  COMPREPLY=( $(compgen -W \"$words\" -- \"$cur\") )\n}\ncomplete -F _overgent_complete overgent\n"
}

func zshCompletion() string {
	var b strings.Builder
	b.WriteString("#compdef overgent\n_overgent() {\n  local command=$words[2]\n  local -a choices\n  case $command in\n")
	b.WriteString(completionCaseLines("    %s) choices=( ${(z)\"%s\"} ) ;;\n", ""))
	b.WriteString("    *) choices=( ")
	b.WriteString(strings.Join(append(sortedRootCommands(), flagNames(cliGlobalFlags)...), " "))
	b.WriteString(" ) ;;\n  esac\n  compadd -- $choices\n}\ncompdef _overgent overgent\n")
	return b.String()
}

func fishCompletion() string {
	var b strings.Builder
	b.WriteString("# fish completion for overgent\ncomplete -c overgent -f\n")
	for _, command := range cliCommands {
		fmt.Fprintf(&b, "complete -c overgent -n '__fish_use_subcommand' -a %s -d '%s'\n", command.Name, strings.ReplaceAll(command.Summary, "'", "\\'"))
		for _, subcommand := range command.Subcommands {
			fmt.Fprintf(&b, "complete -c overgent -n '__fish_seen_subcommand_from %s' -a %s\n", command.Name, subcommand)
		}
		for _, item := range command.Flags {
			fmt.Fprintf(&b, "complete -c overgent -n '__fish_seen_subcommand_from %s' -l %s -d '%s'\n", command.Name, strings.TrimPrefix(item.Name, "--"), strings.ReplaceAll(item.Description, "'", "\\'"))
		}
	}
	for _, item := range cliGlobalFlags {
		fmt.Fprintf(&b, "complete -c overgent -l %s -d '%s'\n", strings.TrimPrefix(item.Name, "--"), strings.ReplaceAll(item.Description, "'", "\\'"))
	}
	return b.String()
}
