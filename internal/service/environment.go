package service

import "strings"

// This file holds the pure part of the two Linux environment checks: given the
// text of the files and command output involved, decide whether a systemd user
// unit will actually survive. Reading /proc, /etc/wsl.conf and loginctl is in
// service_linux.go. Split this way the decisions are testable on any host,
// which matters because both of these are conditions a macOS developer cannot
// reproduce and would otherwise ship unverified.

// userEnvironment is what the probes found. A field being false may mean "no"
// or "could not tell"; the Known flags say which, and nothing is reported as
// broken unless it was positively observed.
type userEnvironment struct {
	WSL             bool
	SystemdIsInit   bool
	InitKnown       bool
	WSLConfSystemd  bool
	LingerEnabled   bool
	LingerKnown     bool
	GraphicalLogin  bool
	GraphicalKnown  bool
	UserDisplayName string
}

// looksLikeWSL recognises a WSL kernel. Microsoft's kernel puts "microsoft" in
// /proc/version on WSL1 and WSL2 alike, and WSL2 kernels since 2022 also carry
// "WSL2" in the release string.
func looksLikeWSL(procVersion, osRelease string) bool {
	text := strings.ToLower(procVersion + "\n" + osRelease)
	return strings.Contains(text, "microsoft") || strings.Contains(text, "wsl")
}

// initIsSystemd reads /proc/1/comm. On a WSL2 distribution without systemd this
// is "init" (Microsoft's own shim); with systemd enabled it is "systemd".
func initIsSystemd(pid1Comm string) bool {
	return strings.TrimSpace(pid1Comm) == "systemd"
}

// wslConfEnablesSystemd parses the systemd flag out of /etc/wsl.conf. The file
// is INI-shaped, so only the key inside a [boot] section counts.
func wslConfEnablesSystemd(contents string) bool {
	section := ""
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || section != "boot" {
			continue
		}
		if strings.ToLower(strings.TrimSpace(key)) != "systemd" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "yes", "1", "on":
			return true
		}
	}
	return false
}

// parseLoginctlUser reads the Linger and Display properties out of
// "loginctl show-user". Display names the user's graphical session and is empty
// when there is none, which is a locale-independent signal where parsing
// session type strings is not.
func parseLoginctlUser(output string) (linger bool, display string) {
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "Linger":
			linger = strings.EqualFold(value, "yes")
		case "Display":
			display = value
		}
	}
	return linger, display
}

// graphicalSessionFromEnv is the fallback when loginctl is unavailable: a
// session with a display server set is a graphical login.
func graphicalSessionFromEnv(lookup func(string) string) bool {
	if lookup("WAYLAND_DISPLAY") != "" || lookup("DISPLAY") != "" {
		return true
	}
	switch strings.ToLower(lookup("XDG_SESSION_TYPE")) {
	case "wayland", "x11":
		return true
	}
	return false
}

// wslWithoutSystemd reports the first environment a user unit cannot live in:
// a WSL2 distribution booted without systemd, where there is no user manager at
// all. Both signals have to agree that systemd is not PID 1 before this is
// claimed, so a systemd-enabled WSL2 distribution is never refused.
func wslWithoutSystemd(env userEnvironment) bool {
	if !env.WSL || env.WSLConfSystemd {
		return false
	}
	return env.InitKnown && !env.SystemdIsInit
}

// lingerRequired reports the second: a machine with no graphical login and no
// lingering, where systemd tears the user manager down with the member's last
// session and takes the service with it. Both facts have to be positively
// observed - an unknown answer never blocks an install.
func lingerRequired(env userEnvironment) bool {
	if !env.LingerKnown || env.LingerEnabled {
		return false
	}
	return env.GraphicalKnown && !env.GraphicalLogin
}
