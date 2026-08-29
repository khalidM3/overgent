//go:build darwin

// Package service manages the current user's Stickguy LaunchAgent.
package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// bootstrapAttempts and bootstrapBackoff bound the wait for launchd to
	// release a label after a bootout.
	bootstrapAttempts = 12
	bootstrapBackoff  = 250 * time.Millisecond
)

// defaultLabel is the LaunchAgent label for the default profile. It must not
// change: an existing install already owns a job under this name.
const defaultLabel = "dev.stickguy.service"

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

// label scopes the LaunchAgent to the profile it manages. Without this a
// development build using an isolated STICKGUY_CONFIG_ROOT and a production
// install both claim "dev.stickguy.service": they overwrite each other's plist
// and only one can ever be bootstrapped. The default profile keeps the original
// label so upgrading does not orphan a job that is already installed.
func (m Manager) label() string {
	if m.ConfigRoot == "" || sameProfile(m.ConfigRoot, m.defaultConfigRoot()) {
		return defaultLabel
	}
	sum := sha256.Sum256([]byte(filepath.Clean(m.ConfigRoot)))
	return defaultLabel + "." + hex.EncodeToString(sum[:4])
}

// defaultConfigRoot mirrors config.DefaultRoot for this Manager's home, kept
// local so the service package stays free of a configuration dependency.
func (m Manager) defaultConfigRoot() string {
	return filepath.Join(m.Home, "Library", "Application Support", "Stickguy")
}

func sameProfile(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func (m Manager) Install(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	path := m.plistPath()
	// A plist on disk is what every later check reads as "installed", so track
	// whether this call is the one that created it.
	_, statErr := os.Stat(path)
	created := os.IsNotExist(statErr)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	body, err := m.plist()
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, body, 0o600); err != nil {
		return fmt.Errorf("write LaunchAgent: %w", err)
	}
	if err = os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace LaunchAgent: %w", err)
	}
	// bootstrap is idempotent only after a best-effort bootout of an older job.
	_ = m.launchctl(ctx, "bootout", m.domain()+"/"+m.label())
	// launchctl bootout returns before launchd has finished tearing the job
	// down. Bootstrapping into a domain that still holds the old job fails with
	// EIO, which launchctl reports only as "Input/output error", so wait for the
	// label to disappear and retry rather than surfacing that to the member.
	if err = m.bootstrapWhenFree(ctx, path); err != nil {
		// Leaving the plist behind would make a failed install indistinguishable
		// from a healthy one: Status reports installed, callers conclude a
		// service exists, and nothing ever starts it.
		m.discardPlist(path, created)
		return fmt.Errorf("bootstrap LaunchAgent: %w", err)
	}
	if err = m.launchctl(ctx, "kickstart", "-k", m.domain()+"/"+m.label()); err != nil {
		_ = m.launchctl(ctx, "bootout", m.domain()+"/"+m.label())
		m.discardPlist(path, created)
		return fmt.Errorf("start LaunchAgent: %w", err)
	}
	return nil
}

// discardPlist removes a LaunchAgent this call created, so a failed install
// leaves no trace that later reads as an installation. A plist that already
// existed is left alone; it belongs to an earlier, possibly working, install.
func (m Manager) discardPlist(path string, created bool) {
	if created {
		_ = os.Remove(path)
	}
}

// bootstrapWhenFree waits for a previous job under this label to finish
// unloading, then bootstraps, retrying while launchd reports the domain is
// still busy.
func (m Manager) bootstrapWhenFree(ctx context.Context, path string) error {
	var err error
	for attempt := 0; attempt < bootstrapAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(bootstrapBackoff):
			}
		}
		if m.loaded(ctx) {
			continue
		}
		if err = m.launchctl(ctx, "bootstrap", m.domain(), path); err == nil {
			return nil
		}
		if alreadyLoaded(err) {
			// Another bootstrap won the race; the job we wanted is running.
			return nil
		}
	}
	if err == nil {
		err = errors.New("a previous LaunchAgent under this label is still unloading")
	}
	return err
}

// loaded reports whether launchd currently holds a job under this label.
func (m Manager) loaded(ctx context.Context) bool {
	command := exec.CommandContext(ctx, "/bin/launchctl", "print", m.domain()+"/"+m.label())
	return command.Run() == nil
}

func (m Manager) Start(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if _, err := os.Stat(m.plistPath()); err != nil {
		return fmt.Errorf("LaunchAgent is not installed: %w", err)
	}
	if err := m.launchctl(ctx, "kickstart", "-k", m.domain()+"/"+m.label()); err != nil {
		return fmt.Errorf("start LaunchAgent: %w", err)
	}
	return nil
}

func (m Manager) Stop(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.launchctl(ctx, "kill", "SIGTERM", m.domain()+"/"+m.label()); err != nil && !notLoaded(err) {
		return fmt.Errorf("stop LaunchAgent: %w", err)
	}
	return nil
}

func (m Manager) Remove(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.launchctl(ctx, "bootout", m.domain()+"/"+m.label()); err != nil && !notLoaded(err) {
		return fmt.Errorf("unload LaunchAgent: %w", err)
	}
	if err := os.Remove(m.plistPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove LaunchAgent: %w", err)
	}
	return nil
}

func (m Manager) Status(ctx context.Context) (Status, error) {
	if err := m.validate(); err != nil {
		return Status{}, err
	}
	_, statErr := os.Stat(m.plistPath())
	status := Status{Installed: statErr == nil, Label: m.label()}
	if statErr != nil && !os.IsNotExist(statErr) {
		return status, fmt.Errorf("inspect LaunchAgent: %w", statErr)
	}
	cmd := exec.CommandContext(ctx, "/bin/launchctl", "print", m.domain()+"/"+m.label())
	if output, err := cmd.CombinedOutput(); err == nil {
		status.Running = bytes.Contains(output, []byte("state = running"))
	} else if !notLoaded(&commandError{err: err, output: string(output)}) {
		return status, fmt.Errorf("inspect LaunchAgent state: %w", err)
	}
	return status, nil
}

func (m Manager) plist() ([]byte, error) {
	arguments := []string{m.Executable, "--config-root", m.ConfigRoot, "service", "run"}
	type dict struct {
		XMLName xml.Name `xml:"plist"`
		Version string   `xml:"version,attr"`
		Dict    entries  `xml:"dict"`
	}
	value := dict{Version: "1.0", Dict: entries{Label: m.label(), Arguments: arguments, RunAtLoad: true, KeepAlive: true, ProcessType: "Background", ThrottleInterval: 5}}
	encoded, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode LaunchAgent: %w", err)
	}
	return append([]byte(xml.Header+`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`+"\n"), append(encoded, '\n')...), nil
}

// entries has a custom encoder because launchd plists are alternating key/value
// elements rather than ordinary XML objects.
type entries struct {
	Label, ProcessType   string
	Arguments            []string
	RunAtLoad, KeepAlive bool
	ThrottleInterval     int
}

func (e entries) MarshalXML(encoder *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "dict"
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	stringValue := func(key, value string) error {
		if err := encoder.EncodeElement(key, xml.StartElement{Name: xml.Name{Local: "key"}}); err != nil {
			return err
		}
		return encoder.EncodeElement(value, xml.StartElement{Name: xml.Name{Local: "string"}})
	}
	boolValue := func(key string, value bool) error {
		if err := encoder.EncodeElement(key, xml.StartElement{Name: xml.Name{Local: "key"}}); err != nil {
			return err
		}
		name := "false"
		if value {
			name = "true"
		}
		if err := encoder.EncodeToken(xml.StartElement{Name: xml.Name{Local: name}}); err != nil {
			return err
		}
		return encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: name}})
	}
	if err := stringValue("Label", e.Label); err != nil {
		return err
	}
	if err := encoder.EncodeElement("ProgramArguments", xml.StartElement{Name: xml.Name{Local: "key"}}); err != nil {
		return err
	}
	if err := encoder.EncodeToken(xml.StartElement{Name: xml.Name{Local: "array"}}); err != nil {
		return err
	}
	for _, argument := range e.Arguments {
		if err := encoder.EncodeElement(argument, xml.StartElement{Name: xml.Name{Local: "string"}}); err != nil {
			return err
		}
	}
	if err := encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: "array"}}); err != nil {
		return err
	}
	if err := boolValue("RunAtLoad", e.RunAtLoad); err != nil {
		return err
	}
	if err := boolValue("KeepAlive", e.KeepAlive); err != nil {
		return err
	}
	if err := stringValue("ProcessType", e.ProcessType); err != nil {
		return err
	}
	if err := encoder.EncodeElement("ThrottleInterval", xml.StartElement{Name: xml.Name{Local: "key"}}); err != nil {
		return err
	}
	if err := encoder.EncodeElement(strconv.Itoa(e.ThrottleInterval), xml.StartElement{Name: xml.Name{Local: "integer"}}); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func (m Manager) validate() error {
	if !filepath.IsAbs(m.Executable) || !filepath.IsAbs(m.ConfigRoot) || !filepath.IsAbs(m.Home) || m.UID <= 0 {
		return errors.New("service executable, config root, home, and uid must be explicit")
	}
	if strings.ContainsAny(m.Executable+m.ConfigRoot+m.Home, "\x00\r\n") {
		return errors.New("service paths contain invalid characters")
	}
	return nil
}

func (m Manager) plistPath() string {
	return filepath.Join(m.Home, "Library", "LaunchAgents", m.label()+".plist")
}
func (m Manager) domain() string { return "gui/" + strconv.Itoa(m.UID) }
func (m Manager) launchctl(ctx context.Context, arguments ...string) error {
	command := exec.CommandContext(ctx, "/bin/launchctl", arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		return &commandError{err: err, output: string(output)}
	}
	return nil
}

type commandError struct {
	err    error
	output string
}

func (e *commandError) Error() string { return strings.TrimSpace(e.output) }
func (e *commandError) Unwrap() error { return e.err }

// alreadyLoaded reports that launchd refused a bootstrap because a job under
// this label is already present, which means the desired end state holds.
func alreadyLoaded(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "service already loaded") ||
		strings.Contains(text, "already bootstrapped") ||
		strings.Contains(text, "37: operation already in progress") ||
		strings.Contains(text, "file exists")
}

func notLoaded(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "could not find service") || strings.Contains(text, "no such process") || strings.Contains(text, "service not found") || strings.Contains(text, "113: could not find specified service")
}
