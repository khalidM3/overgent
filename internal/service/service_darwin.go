//go:build darwin

// Package service manages the current user's Stickguy LaunchAgent.
package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const label = "dev.stickguy.service"

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

func (m Manager) Install(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	path := m.plistPath()
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
	_ = m.launchctl(ctx, "bootout", m.domain()+"/"+label)
	if err = m.launchctl(ctx, "bootstrap", m.domain(), path); err != nil {
		return fmt.Errorf("bootstrap LaunchAgent: %w", err)
	}
	if err = m.launchctl(ctx, "kickstart", "-k", m.domain()+"/"+label); err != nil {
		return fmt.Errorf("start LaunchAgent: %w", err)
	}
	return nil
}

func (m Manager) Start(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if _, err := os.Stat(m.plistPath()); err != nil {
		return fmt.Errorf("LaunchAgent is not installed: %w", err)
	}
	if err := m.launchctl(ctx, "kickstart", "-k", m.domain()+"/"+label); err != nil {
		return fmt.Errorf("start LaunchAgent: %w", err)
	}
	return nil
}

func (m Manager) Stop(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.launchctl(ctx, "kill", "SIGTERM", m.domain()+"/"+label); err != nil && !notLoaded(err) {
		return fmt.Errorf("stop LaunchAgent: %w", err)
	}
	return nil
}

func (m Manager) Remove(ctx context.Context) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.launchctl(ctx, "bootout", m.domain()+"/"+label); err != nil && !notLoaded(err) {
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
	status := Status{Installed: statErr == nil, Label: label}
	if statErr != nil && !os.IsNotExist(statErr) {
		return status, fmt.Errorf("inspect LaunchAgent: %w", statErr)
	}
	cmd := exec.CommandContext(ctx, "/bin/launchctl", "print", m.domain()+"/"+label)
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
	value := dict{Version: "1.0", Dict: entries{Label: label, Arguments: arguments, RunAtLoad: true, KeepAlive: true, ProcessType: "Background", ThrottleInterval: 5}}
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
	return filepath.Join(m.Home, "Library", "LaunchAgents", label+".plist")
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
func notLoaded(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "could not find service") || strings.Contains(text, "no such process") || strings.Contains(text, "service not found") || strings.Contains(text, "113: could not find specified service")
}
