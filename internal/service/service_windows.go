// Package service manages the current user's Overgent background service. On
// Windows that is a logon-triggered Task Scheduler task driven by schtasks.exe;
// see the note in scheduledtask.go for why it is not a Windows service.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// taskBaseName is the scheduled-task name for the default profile. It is
// stable: an installed release owns the task under this name, and an upgrade
// must not orphan it.
const taskBaseName = "Overgent"

// Manager installs and controls the one per-user production service.
type Manager struct {
	Executable string
	ConfigRoot string
	Home       string
	User       string
}

type Status struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Label     string `json:"label"`
}

// label scopes the task to the profile it manages, using the scheme shared with
// the LaunchAgent: the default profile keeps the unscoped name, anything else
// gets the config root hashed in.
func (m Manager) label() string {
	return scopedLabel(taskBaseName, "-", m.ConfigRoot, m.defaultConfigRoot())
}

// defaultConfigRoot mirrors config.DefaultRoot for this Manager's home, kept
// local so the service package stays free of a configuration dependency.
// os.UserConfigDir is %AppData% on Windows.
func (m Manager) defaultConfigRoot() string {
	if dir := os.Getenv("AppData"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "Overgent")
	}
	return filepath.Join(m.Home, "AppData", "Roaming", "Overgent")
}

func (m Manager) Install(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	document, err := renderTask(taskSpec{Label: m.label(), User: m.User, Executable: m.Executable, ConfigRoot: m.ConfigRoot})
	if err != nil {
		return err
	}
	// Unlike the plist and the systemd unit, the artifact that reads as
	// "installed" here is the registered task, not a file: the XML is only the
	// input schtasks parses. Writing it to a temporary file that is always
	// removed is therefore what keeps a failed install from leaving anything
	// behind at all - there is no partially-installed state to clean up.
	created := !m.registered(ctx)
	path, err := m.writeTaskXML(document)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	// /F replaces an existing registration, which is what makes Install
	// idempotent and what makes an upgrade rewrite the executable path.
	if err = m.schtasks(ctx, "/Create", "/TN", m.label(), "/XML", path, "/F"); err != nil {
		m.discardTask(ctx, created)
		return fmt.Errorf("register scheduled task: %w", err)
	}
	// /Create replaces the definition but leaves any running instance alone, so
	// an update would otherwise leave the previous executable running under the
	// new definition. Ending then running is the counterpart of the darwin
	// "kickstart -k".
	if err = m.schtasks(ctx, "/End", "/TN", m.label()); err != nil && !taskNotFound(err) && !taskNotRunning(err) {
		m.discardTask(ctx, created)
		return fmt.Errorf("stop scheduled task: %w", err)
	}
	if err = m.schtasks(ctx, "/Run", "/TN", m.label()); err != nil && !taskAlreadyRunning(err) {
		_ = m.schtasks(ctx, "/End", "/TN", m.label())
		m.discardTask(ctx, created)
		return fmt.Errorf("start scheduled task: %w", err)
	}
	return nil
}

// discardTask deletes a registration this call created, so a failed install
// leaves no task that later reads as an installation. A task that already
// existed is left alone; it belongs to an earlier, possibly working, install.
func (m Manager) discardTask(ctx context.Context, created bool) {
	if created {
		_ = m.schtasks(ctx, "/Delete", "/TN", m.label(), "/F")
	}
}

// writeTaskXML puts the definition somewhere only this member can read it. The
// document names the executable and the config root, which are not secret, but
// a world-readable temporary file that schtasks is about to register is worth
// avoiding on principle.
func (m Manager) writeTaskXML(document string) (string, error) {
	file, err := os.CreateTemp("", "overgent-task-*.xml")
	if err != nil {
		return "", fmt.Errorf("create scheduled task definition: %w", err)
	}
	path := file.Name()
	if _, err = file.Write(encodeTaskXML(document)); err != nil {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("write scheduled task definition: %w", err)
	}
	if err = file.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("write scheduled task definition: %w", err)
	}
	return path, nil
}

func (m Manager) Start(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if !m.registered(ctx) {
		return errors.New("scheduled task is not installed")
	}
	if err := m.schtasks(ctx, "/End", "/TN", m.label()); err != nil && !taskNotFound(err) && !taskNotRunning(err) {
		return fmt.Errorf("stop scheduled task: %w", err)
	}
	if err := m.schtasks(ctx, "/Run", "/TN", m.label()); err != nil && !taskAlreadyRunning(err) {
		return fmt.Errorf("start scheduled task: %w", err)
	}
	return nil
}

func (m Manager) Stop(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.schtasks(ctx, "/End", "/TN", m.label()); err != nil && !taskNotFound(err) && !taskNotRunning(err) {
		return fmt.Errorf("stop scheduled task: %w", err)
	}
	return nil
}

func (m Manager) Remove(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.schtasks(ctx, "/End", "/TN", m.label()); err != nil && !taskNotFound(err) && !taskNotRunning(err) {
		return fmt.Errorf("stop scheduled task: %w", err)
	}
	if err := m.schtasks(ctx, "/Delete", "/TN", m.label(), "/F"); err != nil && !taskNotFound(err) {
		return fmt.Errorf("remove scheduled task: %w", err)
	}
	return nil
}

func (m Manager) Status(ctx context.Context) (Status, error) {
	if err := m.validate(); err != nil {
		return Status{}, err
	}
	status := Status{Label: m.label()}
	output, err := m.schtasksOutput(ctx, "/Query", "/TN", m.label(), "/FO", "CSV", "/NH", "/V")
	if err != nil {
		if taskNotFound(err) {
			return status, nil
		}
		return status, fmt.Errorf("inspect scheduled task: %w", err)
	}
	status.Installed = true
	status.Running = taskIsRunning(output)
	return status, nil
}

// registered reports whether Task Scheduler currently holds this task. It is
// the Windows equivalent of stat-ing the plist.
func (m Manager) registered(ctx context.Context) bool {
	return m.schtasks(ctx, "/Query", "/TN", m.label()) == nil
}

func (m Manager) validate() error {
	if !filepath.IsAbs(m.Executable) || !filepath.IsAbs(m.ConfigRoot) || !filepath.IsAbs(m.Home) || strings.TrimSpace(m.User) == "" {
		return errors.New("service executable, config root, home, and user must be explicit")
	}
	// Same rejection set as the darwin Manager, plus the quote characters: the
	// task XML carries the arguments as one string that Windows re-splits with
	// CommandLineToArgvW, so a quote in a path changes the argument vector.
	if err := checkRenderablePaths(m.Executable, m.ConfigRoot, m.Home, m.User); err != nil {
		return err
	}
	return nil
}

// schtasksPath prefers the absolute path under %SystemRoot% over a PATH lookup,
// so a directory earlier in a member's PATH cannot decide what "schtasks" means
// for a call that installs a background service.
func schtasksPath() string {
	root := os.Getenv("SystemRoot")
	if !filepath.IsAbs(root) {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "schtasks.exe")
}

func (m Manager) schtasks(ctx context.Context, arguments ...string) error {
	_, err := m.schtasksOutput(ctx, arguments...)
	return err
}

func (m Manager) schtasksOutput(ctx context.Context, arguments ...string) (string, error) {
	// An argument array, never a command line this package assembles: the task
	// name and the definition path go to CreateProcess as separate arguments
	// and are never parsed by a shell.
	command := exec.CommandContext(ctx, schtasksPath(), arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), &commandError{err: err, output: string(output)}
	}
	return string(output), nil
}
