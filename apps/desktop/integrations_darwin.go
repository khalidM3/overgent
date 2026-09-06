//go:build darwin

package main

import (
	"errors"
	"fmt"

	"github.com/khalidM3/overgent/internal/claudesetup"
	"github.com/khalidM3/overgent/internal/codexsetup"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/cursorsetup"
)

// DisconnectAgent removes only this profile's recognized managed bindings.
// Codex hooks are user-scoped and are removed only after every Project binding.
func (service *OnboardingService) DisconnectAgent(vendor string) error {
	if vendor != "codex" && vendor != "claude" && vendor != "cursor" {
		return errors.New("unsupported coding agent")
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	executable, err := service.resolveCLI()
	if err != nil {
		return err
	}
	// Preflight every root before changing anything. Unknown or other-profile
	// bindings require their own recovery, not destructive guesswork here.
	for _, workspace := range cfg.Workspaces {
		for _, state := range service.adapterStates([]string{workspace.Root}) {
			if state.Name != vendorLabel(vendor) {
				continue
			}
			if state.Binding == "drifted" || state.Binding == "other_profile" {
				return errors.New("review the agent’s existing connection before disconnecting it")
			}
		}
	}
	for _, workspace := range cfg.Workspaces {
		switch vendor {
		case "codex":
			_, err = (codexsetup.Manager{ProjectRoot: workspace.Root, ConfigRoot: service.configRoot, Executable: executable}).Remove()
		case "claude":
			_, err = (claudesetup.Manager{ProjectRoot: workspace.Root, ConfigRoot: service.configRoot, Executable: executable}).Remove()
		case "cursor":
			_, err = (cursorsetup.Manager{ProjectRoot: workspace.Root, ConfigRoot: service.configRoot, Executable: executable}).Remove()
		}
		if err != nil {
			return fmt.Errorf("disconnect managed agent connection: %w", err)
		}
	}
	if vendor == "codex" {
		return (codexsetup.Manager{ConfigRoot: service.configRoot, Executable: executable}).RemoveHooks()
	}
	return nil
}
