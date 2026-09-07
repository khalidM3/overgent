package service

import "testing"

func TestWSLWithoutSystemdIsDetectedAndSystemdWSLIsNot(t *testing.T) {
	wslKernel := "Linux version 5.15.153.1-microsoft-standard-WSL2 (root@build) #1 SMP"
	wslRelease := "5.15.153.1-microsoft-standard-WSL2"

	// WSL2 booted with Microsoft's init shim: no user manager exists at all.
	broken := userEnvironment{
		WSL:           looksLikeWSL(wslKernel, wslRelease),
		InitKnown:     true,
		SystemdIsInit: initIsSystemd("init\n"),
	}
	if !wslWithoutSystemd(broken) {
		t.Fatal("a WSL2 distribution without systemd was not detected")
	}

	// The same distribution once systemd is PID 1 must install normally.
	working := broken
	working.SystemdIsInit = initIsSystemd("systemd\n")
	if wslWithoutSystemd(working) {
		t.Fatal("a WSL2 distribution running systemd was refused")
	}

	// /etc/wsl.conf already asking for systemd means the member has done the
	// fix and is mid-restart; telling them to do it again is noise.
	declared := broken
	declared.WSLConfSystemd = wslConfEnablesSystemd("[boot]\nsystemd=true\n")
	if wslWithoutSystemd(declared) {
		t.Fatal("a distribution whose wsl.conf enables systemd was refused")
	}

	// An ordinary Linux box is never a WSL diagnosis, whatever its init is.
	ordinary := userEnvironment{WSL: looksLikeWSL("Linux version 6.8.0-45-generic", "6.8.0-45-generic"), InitKnown: true}
	if wslWithoutSystemd(ordinary) {
		t.Fatal("a non-WSL kernel was diagnosed as WSL")
	}

	// An unreadable /proc/1/comm is an unknown answer, and unknown never blocks.
	unknown := broken
	unknown.InitKnown = false
	if wslWithoutSystemd(unknown) {
		t.Fatal("an unknown init was treated as a missing systemd")
	}
}

func TestWSLConfSystemdParsing(t *testing.T) {
	for contents, want := range map[string]bool{
		"[boot]\nsystemd=true":              true,
		"[boot]\n  systemd = True  ":        true,
		"[boot]\nsystemd=yes":               true,
		"[boot]\nsystemd=false":             false,
		"[automount]\nsystemd=true":         false, // wrong section
		"systemd=true":                      false, // no section
		"# systemd=true\n[boot]\ncommand=x": false,
		"":                                  false,
	} {
		if got := wslConfEnablesSystemd(contents); got != want {
			t.Fatalf("wslConfEnablesSystemd(%q) = %v, want %v", contents, got, want)
		}
	}
}

func TestLingerIsRequiredOnlyWhenTheUnitProvablyDies(t *testing.T) {
	linger, display := parseLoginctlUser("Linger=no\nDisplay=\n")
	headless := userEnvironment{
		LingerKnown: true, LingerEnabled: linger,
		GraphicalKnown: true, GraphicalLogin: display != "",
		UserDisplayName: "member",
	}
	if !lingerRequired(headless) {
		t.Fatal("a headless machine without lingering was not detected")
	}

	// A graphical login is the LaunchAgent equivalent: the user manager comes
	// up with the session, so the unit starts at every login without lingering.
	desktop := headless
	_, desktopDisplay := parseLoginctlUser("Linger=no\nDisplay=2\n")
	desktop.GraphicalLogin = desktopDisplay != ""
	if lingerRequired(desktop) {
		t.Fatal("a graphical login was told to enable lingering")
	}

	// Lingering already on is the fix already applied.
	lingering := headless
	lingering.LingerEnabled, _ = parseLoginctlUser("Linger=yes\nDisplay=\n")
	if lingerRequired(lingering) {
		t.Fatal("a lingering account was told to enable lingering")
	}

	// loginctl missing or failing is an unknown answer, and unknown never
	// blocks an install: refusing on a guess is worse than installing.
	for _, unknown := range []userEnvironment{
		{LingerKnown: false, GraphicalKnown: true},
		{LingerKnown: true, LingerEnabled: false, GraphicalKnown: false},
	} {
		if lingerRequired(unknown) {
			t.Fatalf("an unknown environment blocked an install: %+v", unknown)
		}
	}
}

func TestGraphicalSessionFromEnv(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	for _, present := range []map[string]string{
		{"WAYLAND_DISPLAY": "wayland-0"},
		{"DISPLAY": ":0"},
		{"XDG_SESSION_TYPE": "wayland"},
		{"XDG_SESSION_TYPE": "x11"},
	} {
		if !graphicalSessionFromEnv(env(present)) {
			t.Fatalf("graphical session not recognised: %v", present)
		}
	}
	for _, absent := range []map[string]string{
		{},
		{"XDG_SESSION_TYPE": "tty"},
		{"SSH_CONNECTION": "10.0.0.1 22 10.0.0.2 22"},
	} {
		if graphicalSessionFromEnv(env(absent)) {
			t.Fatalf("non-graphical session reported as graphical: %v", absent)
		}
	}
}
