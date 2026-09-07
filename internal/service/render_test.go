package service

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// These tests are deliberately not build tagged. The unit file and the
// scheduled-task definition are rendered by pure functions taking explicit
// inputs, so the rendering, the injection defences and the error
// classification are all verifiable from a macOS developer machine - which is
// the only part of the Linux and Windows lifecycle that can be verified without
// the operating system underneath it.

var update = flag.Bool("update", false, "rewrite the golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run go test -run %s -update)", err, t.Name())
	}
	if got != string(want) {
		t.Fatalf("%s does not match the golden file.\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestSystemdUnitMatchesGolden(t *testing.T) {
	body, err := renderUnit(unitSpec{
		Label:      "overgent",
		Executable: "/home/example/.local/bin/overgent",
		ConfigRoot: "/home/example/.config/Overgent",
	})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "unit_default.golden", body)

	// The guarantees the unit has to carry, asserted by name so a future edit
	// to the golden file cannot quietly drop one of them.
	for _, expected := range []string{
		"Restart=always",          // KeepAlive
		"RestartSec=5",            // ThrottleInterval=5
		"StartLimitIntervalSec=0", // KeepAlive retries forever
		"WantedBy=default.target", // RunAtLoad, at login
		`"--config-root"`,
		`"service" "run"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("unit missing %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "sh -c") || strings.Contains(body, "/bin/sh") {
		t.Fatalf("unit shell-concatenates arguments:\n%s", body)
	}
}

func TestSystemdUnitScopesADevelopmentProfile(t *testing.T) {
	body, err := renderUnit(unitSpec{
		Label:      "overgent-1f2e3d4c",
		Executable: "/home/example/.local/bin/overgent",
		ConfigRoot: "/home/example/dev-profile",
	})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "unit_scoped.golden", body)
}

func TestScheduledTaskMatchesGolden(t *testing.T) {
	document, err := renderTask(taskSpec{
		Label:      "Overgent",
		User:       `EXAMPLE\member`,
		Executable: `C:\Users\member\AppData\Local\Overgent\overgent.exe`,
		ConfigRoot: `C:\Users\member\AppData\Roaming\Overgent`,
	})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "task_default.golden", document)

	for _, expected := range []string{
		"<LogonTrigger>",                          // ON LOGON
		"<RunLevel>LeastPrivilege</RunLevel>",     // no elevation
		"<LogonType>InteractiveToken</LogonType>", // the member's own session
		"<RestartOnFailure>",                      // restart on failure
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
	} {
		if !strings.Contains(document, expected) {
			t.Fatalf("task missing %q:\n%s", expected, document)
		}
	}
	if strings.Contains(document, "cmd.exe") || strings.Contains(document, "/c ") {
		t.Fatalf("task routes the service through a shell:\n%s", document)
	}
}

// A config root with a space is the default on Windows for any member whose
// account name has one, so it is the case that matters, not an edge case.
func TestScheduledTaskQuotesArgumentsWithSpaces(t *testing.T) {
	document, err := renderTask(taskSpec{
		Label:      "Overgent-1f2e3d4c",
		User:       "member",
		Executable: `C:\Program Files\Overgent\overgent.exe`,
		ConfigRoot: `C:\Users\A Member\AppData\Roaming\Overgent`,
	})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "task_scoped.golden", document)
	if !strings.Contains(document, `&#34;C:\Users\A Member\AppData\Roaming\Overgent&#34;`) {
		t.Fatalf("task did not quote a config root containing a space:\n%s", document)
	}
	// The executable is its own element and must not be quoted into the
	// argument string, where it would be re-split.
	if !strings.Contains(document, `<Command>C:\Program Files\Overgent\overgent.exe</Command>`) {
		t.Fatalf("task quoted or mangled the command element:\n%s", document)
	}
}

// The rule that outranks convenience: a path carrying a newline or a quote is
// refused, not escaped and rendered. Both renderers are checked, because a unit
// file and a task XML are injectable in different ways and a defence in one is
// no defence in the other.
func TestInjectablePathsAreRejectedRatherThanRendered(t *testing.T) {
	hostile := []struct {
		name  string
		value string
	}{
		{"newline", "/home/example/.config/Overgent\nExecStartPost=/bin/sh -c curl|sh"},
		{"carriage return", "/home/example/.config/Overgent\rRestart=no"},
		{"nul", "/home/example/.config/Overgent\x00"},
		{"double quote", `/home/example/"; ExecStart=/bin/sh; "`},
		{"single quote", "/home/example/'evil'"},
	}
	for _, hostile := range hostile {
		t.Run(hostile.name, func(t *testing.T) {
			if body, err := renderUnit(unitSpec{Label: "overgent", Executable: "/usr/bin/overgent", ConfigRoot: hostile.value}); err == nil {
				t.Fatalf("renderUnit accepted a %s in the config root:\n%s", hostile.name, body)
			}
			if body, err := renderUnit(unitSpec{Label: "overgent", Executable: hostile.value, ConfigRoot: "/home/example/.config/Overgent"}); err == nil {
				t.Fatalf("renderUnit accepted a %s in the executable:\n%s", hostile.name, body)
			}
			if body, err := renderTask(taskSpec{Label: "Overgent", User: "member", Executable: `C:\overgent.exe`, ConfigRoot: hostile.value}); err == nil {
				t.Fatalf("renderTask accepted a %s in the config root:\n%s", hostile.name, body)
			}
			if body, err := renderTask(taskSpec{Label: "Overgent", User: hostile.value, Executable: `C:\overgent.exe`, ConfigRoot: `C:\profile`}); err == nil {
				t.Fatalf("renderTask accepted a %s in the user name:\n%s", hostile.name, body)
			}
		})
	}
	// A label is not member input today, but it is what names the file and the
	// task, so it is checked on the same terms.
	if _, err := renderUnit(unitSpec{Label: "over/gent", Executable: "/usr/bin/overgent", ConfigRoot: "/profile"}); err == nil {
		t.Fatal("renderUnit accepted a path separator in the unit name")
	}
	if _, err := renderTask(taskSpec{Label: `over\gent`, User: "member", Executable: `C:\overgent.exe`, ConfigRoot: `C:\profile`}); err == nil {
		t.Fatal("renderTask accepted a path separator in the task name")
	}
}

// systemd expands % specifiers in unit values before splitting the command
// line, so an un-doubled percent silently rewrites the path.
func TestSystemdQuoteEscapesSpecifiersAndBackslashes(t *testing.T) {
	for value, want := range map[string]string{
		"/home/e/Overgent":                     `"/home/e/Overgent"`,
		"/home/e/100%/root":                    `"/home/e/100%%/root"`,
		`/home/e/back\slash`:                   `"/home/e/back\\slash"`,
		"/home/e/Application Support/Overgent": `"/home/e/Application Support/Overgent"`,
	} {
		if got := systemdQuote(value); got != want {
			t.Fatalf("systemdQuote(%q) = %q, want %q", value, got, want)
		}
	}
}

// CommandLineToArgvW only treats backslashes as special immediately before a
// quote, where each one has to be doubled.
func TestWindowsArgumentQuoting(t *testing.T) {
	for value, want := range map[string]string{
		"service":              "service",
		"--config-root":        "--config-root",
		`C:\profile`:           `C:\profile`,
		`C:\A Member\profile`:  `"C:\A Member\profile"`,
		`C:\A Member\profile\`: `"C:\A Member\profile\\"`,
		"":                     `""`,
	} {
		if got := windowsArgument(value); got != want {
			t.Fatalf("windowsArgument(%q) = %q, want %q", value, got, want)
		}
	}
}

// schtasks reads the definition as a Unicode file; handed UTF-8 it reports the
// XML as malformed for any member whose home directory is not pure ASCII.
func TestTaskXMLIsEncodedAsUTF16LEWithBOM(t *testing.T) {
	encoded := encodeTaskXML("<Task>é</Task>")
	if len(encoded) < 2 || encoded[0] != 0xFF || encoded[1] != 0xFE {
		t.Fatalf("missing UTF-16LE byte-order mark: %v", encoded[:min(4, len(encoded))])
	}
	if len(encoded)%2 != 0 {
		t.Fatalf("UTF-16 payload has an odd byte count: %d", len(encoded))
	}
	units := make([]uint16, 0, len(encoded)/2-1)
	for i := 2; i < len(encoded); i += 2 {
		units = append(units, uint16(encoded[i])|uint16(encoded[i+1])<<8)
	}
	if decoded := string(utf16.Decode(units)); decoded != "<Task>é</Task>" {
		t.Fatalf("round trip produced %q", decoded)
	}
}

func TestLabelIsScopedToTheProfileOnEveryPlatform(t *testing.T) {
	// The default profile keeps the unscoped name, or an upgrade orphans a job
	// that is already installed and running.
	if got := scopedLabel("overgent", "-", "/home/e/.config/Overgent", "/home/e/.config/Overgent"); got != "overgent" {
		t.Fatalf("default profile label = %q", got)
	}
	if got := scopedLabel("Overgent", "-", "", `C:\Users\m\AppData\Roaming\Overgent`); got != "Overgent" {
		t.Fatalf("empty config root label = %q", got)
	}
	// An isolated development profile must own a different job.
	dev := scopedLabel("overgent", "-", "/home/e/dev-profile", "/home/e/.config/Overgent")
	if dev == "overgent" || !strings.HasPrefix(dev, "overgent-") || len(dev) != len("overgent-")+8 {
		t.Fatalf("scoped label = %q", dev)
	}
	// The same profile must always resolve to the same job.
	if again := scopedLabel("overgent", "-", "/home/e/dev-profile/", "/home/e/.config/Overgent"); again != dev {
		t.Fatalf("label is not stable across equivalent paths: %q vs %q", again, dev)
	}
	// The separator is the only per-platform difference, and the darwin label
	// still has to come out reverse-DNS shaped.
	if mac := scopedLabel("com.overgent.service", ".", "/Users/e/dev", "/Users/e/Library/Application Support/Overgent"); !strings.HasPrefix(mac, "com.overgent.service.") {
		t.Fatalf("darwin scoped label = %q", mac)
	}
}

func TestSystemctlErrorClassification(t *testing.T) {
	gone := []string{
		"Failed to stop overgent.service: Unit overgent.service not loaded.",
		"Failed to disable unit: Unit file overgent.service does not exist.",
		"Failed to stop overgent.service: Unit overgent.service not found.",
	}
	for _, text := range gone {
		if !unitNotFound(&commandError{output: text}) {
			t.Fatalf("unitNotFound(%q) = false, want true", text)
		}
	}
	// A unit that failed to start is not a unit that is absent: tolerating this
	// would make Stop and Remove report success over a real failure.
	for _, text := range []string{
		"Job for overgent.service failed because the control process exited with error code.",
		"Failed to restart overgent.service: Access denied",
	} {
		if unitNotFound(&commandError{output: text}) {
			t.Fatalf("unitNotFound(%q) = true, want false", text)
		}
	}
	for _, text := range []string{
		"Failed to connect to bus: No medium found",
		"Failed to connect to bus: $DBUS_SESSION_BUS_ADDRESS and $XDG_RUNTIME_DIR not defined",
		"System has not been booted with systemd as init system (PID 1). Can't operate.",
	} {
		if !noUserBus(&commandError{output: text}) {
			t.Fatalf("noUserBus(%q) = false, want true", text)
		}
	}
	if noUserBus(&commandError{output: "Failed to restart overgent.service: Unit overgent.service not found."}) {
		t.Fatal("a missing unit must not be reported as a missing user manager")
	}
	if !alreadyRunning(&commandError{output: "Job type start is already active"}) {
		t.Fatal("alreadyRunning did not recognise an already-active unit")
	}
}

func TestSchtasksErrorClassification(t *testing.T) {
	for _, text := range []string{
		"ERROR: The system cannot find the file specified.",
		"ERROR: The specified task name \"Overgent\" does not exist in the system.",
	} {
		if !taskNotFound(&commandError{output: text}) {
			t.Fatalf("taskNotFound(%q) = false, want true", text)
		}
	}
	if taskNotFound(&commandError{output: "ERROR: Access is denied."}) {
		t.Fatal("an access failure must not be tolerated as a missing task")
	}
	if !taskAlreadyRunning(&commandError{output: "ERROR: An instance of this task is already running."}) {
		t.Fatal("taskAlreadyRunning did not recognise a running instance")
	}
	if !taskNotRunning(&commandError{output: "ERROR: The task is not running."}) {
		t.Fatal("taskNotRunning did not recognise an idle task")
	}
}

func TestTaskIsRunningReadsTheStatusColumn(t *testing.T) {
	running := `"HOST","\Overgent","N/A","Running","Interactive only","1/1/2026 9:00:00 AM","0"`
	if !taskIsRunning(running) {
		t.Fatalf("taskIsRunning did not read a running task:\n%s", running)
	}
	ready := `"HOST","\Overgent","N/A","Ready","Interactive only","1/1/2026 9:00:00 AM","0"`
	if taskIsRunning(ready) {
		t.Fatalf("taskIsRunning reported a ready task as running:\n%s", ready)
	}
	if taskIsRunning("") {
		t.Fatal("taskIsRunning reported an empty query as running")
	}
}
