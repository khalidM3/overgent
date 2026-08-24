//go:build darwin

package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const label = "dev.stickguy.validation.gate-d"

type launchdManager struct {
	root, executable, plist string
	domain                  string
}

func New(root, executable string) Manager {
	return &launchdManager{root: root, executable: executable, plist: filepath.Join(root, label+".plist"), domain: "gui/" + strconv.Itoa(os.Getuid())}
}

func (m *launchdManager) Install(_ context.Context, out io.Writer) error {
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return fmt.Errorf("create service root: %w", err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>` + label + `</string>
<key>ProgramArguments</key><array><string>` + xmlEscape(m.executable) + `</string><string>service</string><string>run</string></array>
<key>EnvironmentVariables</key><dict><key>STICKGUY_SPIKE_STATE_DIR</key><string>` + xmlEscape(m.root) + `</string></dict>
<key>KeepAlive</key><true/><key>RunAtLoad</key><true/>
<key>StandardOutPath</key><string>` + xmlEscape(filepath.Join(m.root, "service.out.log")) + `</string>
<key>StandardErrorPath</key><string>` + xmlEscape(filepath.Join(m.root, "service.err.log")) + `</string>
</dict></plist>
`
	if err := os.WriteFile(m.plist, []byte(plist), 0o600); err != nil {
		return fmt.Errorf("write launchd plist: %w", err)
	}
	_, err := fmt.Fprintln(out, m.plist)
	return err
}

func (m *launchdManager) Start(ctx context.Context, out io.Writer) error {
	if _, err := os.Stat(m.plist); err != nil {
		return fmt.Errorf("service is not installed: %w", err)
	}
	if err := m.launchctl(ctx, "bootstrap", m.domain, m.plist); err != nil {
		if !strings.Contains(err.Error(), "service already loaded") {
			return err
		}
	}
	if err := m.launchctl(ctx, "kickstart", "-k", m.domain+"/"+label); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "started")
	return err
}

func (m *launchdManager) Status(ctx context.Context, out io.Writer) error {
	cmd := exec.CommandContext(ctx, "/bin/launchctl", "print", m.domain+"/"+label)
	data, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("service not running: %w", err)
	}
	state := "unknown"
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "state =") {
			state = strings.TrimSpace(line)
			break
		}
	}
	_, err = fmt.Fprintln(out, state)
	return err
}

func (m *launchdManager) Stop(ctx context.Context, out io.Writer) error {
	if err := m.launchctl(ctx, "bootout", m.domain+"/"+label); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "stopped")
	return err
}

func (m *launchdManager) Remove(ctx context.Context, out io.Writer) error {
	_ = m.launchctl(ctx, "bootout", m.domain+"/"+label)
	if err := os.Remove(m.plist); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove launchd plist: %w", err)
	}
	_, err := fmt.Fprintln(out, "removed")
	return err
}

func (m *launchdManager) launchctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "/bin/launchctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return nil
}

func xmlEscape(value string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return r.Replace(value)
}
