// Package adapterrepair adopts agent bindings that an earlier Overgent left
// behind on this Mac.
//
// The bindings Overgent writes name the executable that runs them and the
// profile they belong to, so anything that moves either - a rename, a rebuilt
// app bundle, a CLI copied to a new path - makes an existing binding stop
// matching the expected one. Structurally that is indistinguishable from a
// second Overgent profile competing for the same agent, and the product used to
// treat it as exactly that: it refused to configure the agent and offered a
// Reconnect confirmation that named the same member on both sides.
//
// Nobody can answer that question, because it is not a question. This package
// answers it instead, on every launch and before every setup, and only escalates
// to the member when another profile is genuinely still in use.
package adapterrepair

import (
	"github.com/khalidM3/overgent/internal/claudesetup"
	"github.com/khalidM3/overgent/internal/codexsetup"
	"github.com/khalidM3/overgent/internal/cursorsetup"
)

// Vendors are the agents a repair pass covers, in the order it reports them.
var Vendors = []string{"codex", "claude", "cursor"}

// Outcome is what a repair pass did to one vendor in one repository.
type Outcome struct {
	Vendor string
	Root   string
	// Adopted reports that a leftover binding was moved onto this profile.
	// A pass that finds nothing to fix reports no outcomes at all, which is the
	// normal case and must stay silent all the way to the interface.
	Adopted bool
	// Err is why a repair could not be completed. It is never fatal to the
	// caller: an agent that cannot be repaired is reported through the ordinary
	// adapter status, which says what is actually wrong with it.
	Err error
}

// Run repairs every vendor across every registered repository.
//
// It never creates a binding. An agent that was never connected here stays
// unconnected, because connecting one is the member's decision and clearing up
// after an earlier build is not.
func Run(configRoot, executable string, roots []string) []Outcome {
	if configRoot == "" || executable == "" || len(roots) == 0 {
		return nil
	}
	var outcomes []Outcome
	for _, root := range roots {
		for _, vendor := range Vendors {
			if outcome, changed := repair(configRoot, executable, root, vendor); changed {
				outcomes = append(outcomes, outcome)
			}
		}
	}
	return outcomes
}

func repair(configRoot, executable, root, vendor string) (Outcome, bool) {
	outcome := Outcome{Vendor: vendor, Root: root}
	var (
		before, after string
		err           error
	)
	switch vendor {
	case "codex":
		manager := codexsetup.Manager{ProjectRoot: root, ConfigRoot: configRoot, Executable: executable}
		before, after, err = codexBindings(manager)
	case "claude":
		manager := claudesetup.Manager{ProjectRoot: root, ConfigRoot: configRoot, Executable: executable}
		before, after, err = claudeBindings(manager)
	case "cursor":
		manager := cursorsetup.Manager{ProjectRoot: root, ConfigRoot: configRoot, Executable: executable}
		before, after, err = cursorBindings(manager)
	}
	if err != nil {
		outcome.Err = err
		return outcome, true
	}
	// Only a binding that actually moved is worth reporting. Repair is meant to
	// be invisible when there is nothing to repair.
	if before == "other_profile" && after == "current" {
		outcome.Adopted = true
		return outcome, true
	}
	return outcome, false
}

func codexBindings(manager codexsetup.Manager) (string, string, error) {
	status, err := manager.Status()
	if err != nil {
		return "", "", err
	}
	if status.Binding != "other_profile" {
		return status.Binding, status.Binding, nil
	}
	repaired, err := manager.Repair()
	if err != nil {
		return status.Binding, status.Binding, err
	}
	return status.Binding, repaired.Binding, nil
}

func claudeBindings(manager claudesetup.Manager) (string, string, error) {
	status, err := manager.Status()
	if err != nil {
		return "", "", err
	}
	if status.Binding != "other_profile" {
		return status.Binding, status.Binding, nil
	}
	repaired, err := manager.Repair()
	if err != nil {
		return status.Binding, status.Binding, err
	}
	return status.Binding, repaired.Binding, nil
}

func cursorBindings(manager cursorsetup.Manager) (string, string, error) {
	status, err := manager.Status()
	if err != nil {
		return "", "", err
	}
	if status.Binding != "other_profile" {
		return status.Binding, status.Binding, nil
	}
	repaired, err := manager.Repair()
	if err != nil {
		return status.Binding, status.Binding, err
	}
	return status.Binding, repaired.Binding, nil
}
