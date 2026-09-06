package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// runInit is a small guided front door over the existing create/join flows.
// It never owns onboarding behavior and does not mutate anything until the
// member has selected an explicit path.
func runInit(args []string, configRoot, api string, apiProvided bool, stdin io.Reader, stdout, stderr io.Writer, invoke func([]string) error) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	local := flags.Bool("local", false, "create a local Project")
	team := flags.Bool("team", false, "create a team Project")
	invite := flags.String("join", "", "join using an invite link or code")
	label := flags.String("label", "", "Project label")
	repository := flags.String("root", ".", "Git repository root")
	noInput := flags.Bool("no-input", false, "never prompt")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("init accepts flags only")
	}
	choices := 0
	for _, selected := range []bool{*local, *team, *invite != ""} {
		if selected {
			choices++
		}
	}
	if choices > 1 {
		return errors.New("init accepts exactly one of --local, --team, or --join")
	}
	if choices == 0 {
		terminal := presentationTerminal(stdin, stdout, stderr)
		if *noInput || !interactive(terminal) {
			return errNoInput("setup needs to know what kind of Project to make", "one of `--local`, `--team`, or `--join INVITE`")
		}
		fmt.Fprintln(stdout, "Create or join a Project\n\n  1. Local Project (recommended; nothing leaves this computer)\n  2. Team Project (bounded coordination facts sync)\n  3. Join an existing Project\n\nPress Ctrl-C to cancel.")
		fmt.Fprint(stdout, "Choice [1]: ")
		answer, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && len(answer) == 0 {
			return errors.New("setup cancelled: input closed")
		}
		switch strings.TrimSpace(answer) {
		case "", "1":
			*local = true
		case "2":
			*team = true
		case "3":
			return errors.New("an invite is needed to join.\n\nNext: run `overgent init --join INVITE` with the code you were sent")
		default:
			return errors.New("setup cancelled: choose 1, 2, or 3")
		}
	}
	base := []string{"--config-root", configRoot}
	if *invite != "" {
		if apiProvided {
			base = append(base, "--api", api)
		}
		base = append(base, "join", "--root", *repository, *invite)
		return invoke(base)
	}
	if *team && apiProvided {
		base = append(base, "--api", api)
	}
	base = append(base, "create", "--root", *repository)
	if *label != "" {
		base = append(base, "--label", *label)
	}
	if *local {
		base = append(base, "--local")
	}
	return invoke(base)
}
