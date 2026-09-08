// Package service manages the current user's Overgent background service. On
// Linux that is a systemd user unit driven by "systemctl --user".
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// unitBaseName is the systemd unit name for the default profile, without the
// ".service" suffix. It is stable: an installed release owns the unit under
// this name, and an upgrade must not orphan it.
const unitBaseName = "overgent"

// Manager installs and controls the one per-user production service.
type Manager struct {
	Executable string
	ConfigRoot string
	Home       string
	UID        int
}

type Status struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Label     string `json:"label"`
}

// label scopes the unit to the profile it manages, using the scheme shared with
// the LaunchAgent: the default profile keeps the unscoped name, anything else
// gets the config root hashed in.
func (m Manager) label() string {
	return scopedLabel(unitBaseName, "-", m.ConfigRoot, m.defaultConfigRoot())
}

// unitName is the label as systemctl wants it.
func (m Manager) unitName() string { return m.label() + ".service" }

// configHome mirrors os.UserConfigDir for this Manager's home. systemd reads
// user units from $XDG_CONFIG_HOME/systemd/user when that is set, so honouring
// it here is what keeps the unit somewhere systemd will actually look.
func (m Manager) configHome() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(m.Home, ".config")
}

// defaultConfigRoot mirrors config.DefaultRoot for this Manager's home, kept
// local so the service package stays free of a configuration dependency.
func (m Manager) defaultConfigRoot() string {
	return filepath.Join(m.configHome(), "Overgent")
}

func (m Manager) unitPath() string {
	return filepath.Join(m.configHome(), "systemd", "user", m.unitName())
}

func (m Manager) Install(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	// Refuse before writing anything if the unit provably will not survive.
	// These two environments otherwise fail as an opaque "Failed to connect to
	// bus" at enable time, or as a service that silently disappears the next
	// time the member logs out.
	if err := m.environmentError(ctx); err != nil {
		return err
	}
	body, err := renderUnit(unitSpec{Label: m.label(), Executable: m.Executable, ConfigRoot: m.ConfigRoot})
	if err != nil {
		return err
	}
	path := m.unitPath()
	// A unit file on disk is what every later check reads as "installed", so
	// track whether this call is the one that created it.
	_, statErr := os.Stat(path)
	created := os.IsNotExist(statErr)
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write systemd unit: %w", err)
	}
	if err = os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace systemd unit: %w", err)
	}
	if err = m.systemctl(ctx, "daemon-reload"); err != nil {
		// Leaving the unit behind would make a failed install
		// indistinguishable from a healthy one: Status reports installed,
		// callers conclude a service exists, and nothing ever starts it.
		m.discardUnit(path, created)
		return fmt.Errorf("reload systemd user manager: %w", m.explain(ctx, err))
	}
	if err = m.systemctl(ctx, "enable", "--now", m.unitName()); err != nil && !alreadyRunning(err) {
		m.discardUnit(path, created)
		_ = m.systemctl(ctx, "daemon-reload")
		return fmt.Errorf("enable systemd unit: %w", m.explain(ctx, err))
	}
	// "enable --now" starts a stopped unit but does nothing to a running one,
	// and Install is also the upgrade path: without an explicit restart an
	// update leaves the previous executable running under the new unit file.
	// This is the counterpart of the darwin "kickstart -k".
	if err = m.systemctl(ctx, "restart", m.unitName()); err != nil {
		_ = m.systemctl(ctx, "disable", "--now", m.unitName())
		m.discardUnit(path, created)
		_ = m.systemctl(ctx, "daemon-reload")
		return fmt.Errorf("start systemd unit: %w", m.explain(ctx, err))
	}
	return nil
}

// discardUnit removes a unit file this call created, so a failed install leaves
// no trace that later reads as an installation. A unit that already existed is
// left alone; it belongs to an earlier, possibly working, install.
func (m Manager) discardUnit(path string, created bool) {
	if created {
		_ = os.Remove(path)
	}
}

func (m Manager) Start(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if _, err := os.Stat(m.unitPath()); err != nil {
		return fmt.Errorf("systemd unit is not installed: %w", err)
	}
	if err := m.systemctl(ctx, "restart", m.unitName()); err != nil {
		return fmt.Errorf("start systemd unit: %w", m.explain(ctx, err))
	}
	return nil
}

func (m Manager) Stop(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.systemctl(ctx, "stop", m.unitName()); err != nil && !unitNotFound(err) {
		return fmt.Errorf("stop systemd unit: %w", m.explain(ctx, err))
	}
	return nil
}

func (m Manager) Remove(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.systemctl(ctx, "disable", "--now", m.unitName()); err != nil && !unitNotFound(err) {
		return fmt.Errorf("disable systemd unit: %w", m.explain(ctx, err))
	}
	if err := os.Remove(m.unitPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove systemd unit: %w", err)
	}
	// Best effort: the unit is gone either way, and a stale entry in the user
	// manager is not worth failing an uninstall over.
	_ = m.systemctl(ctx, "daemon-reload")
	return nil
}

func (m Manager) Status(ctx context.Context) (Status, error) {
	if err := m.validate(); err != nil {
		return Status{}, err
	}
	_, statErr := os.Stat(m.unitPath())
	status := Status{Installed: statErr == nil, Label: m.unitName()}
	if statErr != nil && !os.IsNotExist(statErr) {
		return status, fmt.Errorf("inspect systemd unit: %w", statErr)
	}
	// "show" answers for an unknown unit with ActiveState=inactive and exit 0,
	// so unlike is-active it needs no not-found classification: an installed
	// unit that is not running and a unit that does not exist are already
	// distinguished by the file check above.
	output, err := m.systemctlOutput(ctx, "show", "--property=ActiveState", m.unitName())
	if err != nil {
		return status, fmt.Errorf("inspect systemd unit state: %w", m.explain(ctx, err))
	}
	state := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(output), "ActiveState="))
	status.Running = state == "active" || state == "activating" || state == "reloading"
	return status, nil
}

func (m Manager) validate() error {
	if !filepath.IsAbs(m.Executable) || !filepath.IsAbs(m.ConfigRoot) || !filepath.IsAbs(m.Home) || m.UID <= 0 {
		return errors.New("service executable, config root, home, and uid must be explicit")
	}
	// Same rejection set as the darwin Manager, plus the quote characters:
	// a systemd unit file is line-oriented and its command line is word-split
	// with shell-like quoting, so either would let a path rewrite the job.
	if err := checkRenderablePaths(m.Executable, m.ConfigRoot, m.Home); err != nil {
		return err
	}
	return nil
}

// systemctlPath prefers an absolute path over PATH lookup, so a directory
// earlier in a member's PATH cannot decide what "systemctl" means for a call
// that installs a background service. Distributions disagree on which of the
// two locations is real and which is a symlink, hence both.
func systemctlPath() string { return toolPath("systemctl") }
func loginctlPath() string  { return toolPath("loginctl") }

func toolPath(name string) string {
	for _, candidate := range []string{"/usr/bin/" + name, "/bin/" + name} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if resolved, err := exec.LookPath(name); err == nil {
		return resolved
	}
	return "/usr/bin/" + name
}

func (m Manager) systemctl(ctx context.Context, arguments ...string) error {
	_, err := m.systemctlOutput(ctx, arguments...)
	return err
}

func (m Manager) systemctlOutput(ctx context.Context, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, systemctlPath(), append([]string{"--user"}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), &commandError{err: err, output: string(output)}
	}
	return string(output), nil
}

// explain replaces an opaque systemd failure with the environment problem
// behind it when there is one. "Failed to connect to bus: No medium found" is
// what a member sees today on a WSL2 distribution without systemd and on a
// headless box with no user manager, and it tells them nothing they can act on.
func (m Manager) explain(ctx context.Context, err error) error {
	if !noUserBus(err) {
		return err
	}
	if environmentErr := m.environmentError(ctx); environmentErr != nil {
		return environmentErr
	}
	return fmt.Errorf("%w: no systemd user manager is running for this account", err)
}

// environmentError reports the two environments where a systemd user unit will
// not survive, as something the member can act on. It returns nil when the
// environment is fine or when the probes could not tell - an unknown answer
// never blocks an install.
func (m Manager) environmentError(ctx context.Context) error {
	env := m.probeEnvironment(ctx)
	if wslWithoutSystemd(env) {
		return errors.New("this WSL2 distribution is running without systemd, so there is no user manager to run the Overgent service under.\n" +
			"Enable it by adding\n\n" +
			"    [boot]\n" +
			"    systemd=true\n\n" +
			"to /etc/wsl.conf, then run \"wsl.exe --shutdown\" from Windows and reopen the distribution")
	}
	if lingerRequired(env) {
		return fmt.Errorf("the Overgent service runs as a systemd user unit, and this machine has no graphical login session and no lingering enabled, "+
			"so systemd stops the user manager - and the service with it - when your last session ends.\n"+
			"Enable lingering with\n\n"+
			"    loginctl enable-linger %s\n\n"+
			"then run \"overgent service install\" again", env.UserDisplayName)
	}
	return nil
}

func (m Manager) probeEnvironment(ctx context.Context) userEnvironment {
	env := userEnvironment{UserDisplayName: m.accountName()}
	procVersion, _ := os.ReadFile("/proc/version")
	osRelease, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	env.WSL = looksLikeWSL(string(procVersion), string(osRelease))
	if comm, err := os.ReadFile("/proc/1/comm"); err == nil {
		env.InitKnown = true
		env.SystemdIsInit = initIsSystemd(string(comm))
	}
	if conf, err := os.ReadFile("/etc/wsl.conf"); err == nil {
		env.WSLConfSystemd = wslConfEnablesSystemd(string(conf))
	}
	if output, err := m.loginctlUser(ctx); err == nil {
		linger, display := parseLoginctlUser(output)
		env.LingerKnown, env.LingerEnabled = true, linger
		env.GraphicalKnown, env.GraphicalLogin = true, display != ""
		if !env.GraphicalLogin && graphicalSessionFromEnv(os.Getenv) {
			env.GraphicalLogin = true
		}
	}
	return env
}

func (m Manager) loginctlUser(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, loginctlPath(), "show-user", strconv.Itoa(m.UID), "--property=Linger", "--property=Display")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", &commandError{err: err, output: string(output)}
	}
	return string(output), nil
}

// accountName is only ever interpolated into the enable-linger instruction.
// loginctl accepts a uid there too, so the numeric form is a correct fallback
// rather than a guess when the name cannot be resolved.
func (m Manager) accountName() string {
	if account, err := user.LookupId(strconv.Itoa(m.UID)); err == nil && account.Username != "" {
		return account.Username
	}
	return strconv.Itoa(m.UID)
}
