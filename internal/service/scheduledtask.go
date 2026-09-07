package service

import (
	"encoding/csv"
	"encoding/xml"
	"errors"
	"strings"
	"unicode/utf16"
)

// Why Task Scheduler and not a Windows service.
//
// A real Windows service has to be registered by an administrator and runs in
// session 0, isolated from every interactive desktop session. Overgent's
// service observes the agent sessions a member is running in their own
// terminal and editor: it needs that member's session, that member's
// environment, and that member's home. A session-0 service would need a
// separate per-user agent process to see any of it, and would demand elevation
// during install for the privilege of being in the wrong session. A logon-
// triggered scheduled task running as the current user with LeastPrivilege is
// the same shape as the macOS LaunchAgent - per user, no elevation, starts at
// login, restarts on failure - which is what this package promises everywhere
// else. Do not re-litigate this without an ADR: the constraint is the session,
// not the API.

// taskSpec is everything the scheduled-task definition is rendered from.
type taskSpec struct {
	Label      string
	User       string
	Executable string
	ConfigRoot string
}

// renderTask produces the Task Scheduler XML registered by
// "schtasks /Create /XML".
//
// The element order matches what Task Scheduler itself writes when it exports a
// task, which is the order known to round-trip; the schema is a sequence and
// reordering these is how "The task XML is malformed" happens. The header
// declares UTF-16 because schtasks reads the file as Unicode - see
// encodeTaskXML, which is what actually writes the bytes.
func renderTask(spec taskSpec) (string, error) {
	if err := checkRenderablePaths(spec.Executable, spec.ConfigRoot); err != nil {
		return "", err
	}
	if err := checkRenderablePaths(spec.Label, spec.User); err != nil {
		return "", err
	}
	if strings.ContainsAny(spec.Label, `\/:*?<>|`) {
		return "", errors.New("service label is not a usable scheduled-task name")
	}
	arguments := serviceArguments(spec.Executable, spec.ConfigRoot)
	quoted := make([]string, 0, len(arguments)-1)
	for _, argument := range arguments[1:] {
		quoted = append(quoted, windowsArgument(argument))
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-16"?>` + "\n")
	b.WriteString(`<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">` + "\n")
	b.WriteString("  <RegistrationInfo>\n")
	b.WriteString("    <Description>Overgent coordination service</Description>\n")
	b.WriteString("    <URI>\\" + escapeXML(spec.Label) + "</URI>\n")
	b.WriteString("  </RegistrationInfo>\n")
	b.WriteString("  <Triggers>\n")
	// ON LOGON for this user only. The LaunchAgent equivalent is RunAtLoad in a
	// gui/<uid> domain; neither starts anything for anybody else on the machine.
	b.WriteString("    <LogonTrigger>\n")
	b.WriteString("      <Enabled>true</Enabled>\n")
	b.WriteString("      <UserId>" + escapeXML(spec.User) + "</UserId>\n")
	b.WriteString("    </LogonTrigger>\n")
	b.WriteString("  </Triggers>\n")
	b.WriteString("  <Principals>\n")
	b.WriteString(`    <Principal id="Author">` + "\n")
	b.WriteString("      <UserId>" + escapeXML(spec.User) + "</UserId>\n")
	b.WriteString("      <LogonType>InteractiveToken</LogonType>\n")
	// LeastPrivilege is the no-elevation promise: registering and running this
	// task must never require an administrator.
	b.WriteString("      <RunLevel>LeastPrivilege</RunLevel>\n")
	b.WriteString("    </Principal>\n")
	b.WriteString("  </Principals>\n")
	b.WriteString("  <Settings>\n")
	// One service per profile. IgnoreNew is what keeps a second logon from
	// starting a second copy against the same config root.
	b.WriteString("    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>\n")
	// A laptop on battery is the normal case for this product, so none of the
	// power gates may stop or refuse to start the service.
	b.WriteString("    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>\n")
	b.WriteString("    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>\n")
	b.WriteString("    <AllowHardTerminate>true</AllowHardTerminate>\n")
	b.WriteString("    <StartWhenAvailable>true</StartWhenAvailable>\n")
	b.WriteString("    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>\n")
	b.WriteString("    <IdleSettings>\n")
	b.WriteString("      <StopOnIdleEnd>false</StopOnIdleEnd>\n")
	b.WriteString("      <RestartOnIdle>false</RestartOnIdle>\n")
	b.WriteString("    </IdleSettings>\n")
	b.WriteString("    <AllowStartOnDemand>true</AllowStartOnDemand>\n")
	b.WriteString("    <Enabled>true</Enabled>\n")
	b.WriteString("    <Hidden>false</Hidden>\n")
	b.WriteString("    <RunOnlyIfIdle>false</RunOnlyIfIdle>\n")
	b.WriteString("    <WakeToRun>false</WakeToRun>\n")
	// PT0S is "no time limit". The default is three days, after which Task
	// Scheduler would terminate a perfectly healthy long-running service.
	b.WriteString("    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>\n")
	b.WriteString("    <Priority>7</Priority>\n")
	// The KeepAlive / Restart=always equivalent. PT1M is the shortest interval
	// Task Scheduler accepts - it has no five-second option - so recovery here
	// is coarser than launchd's ThrottleInterval=5 and systemd's RestartSec=5.
	b.WriteString("    <RestartOnFailure>\n")
	b.WriteString("      <Interval>PT1M</Interval>\n")
	b.WriteString("      <Count>999</Count>\n")
	b.WriteString("    </RestartOnFailure>\n")
	b.WriteString("  </Settings>\n")
	b.WriteString(`  <Actions Context="Author">` + "\n")
	b.WriteString("    <Exec>\n")
	// Command and Arguments are separate elements, so the executable path is
	// never parsed as part of the command line.
	b.WriteString("      <Command>" + escapeXML(spec.Executable) + "</Command>\n")
	b.WriteString("      <Arguments>" + escapeXML(strings.Join(quoted, " ")) + "</Arguments>\n")
	b.WriteString("    </Exec>\n")
	b.WriteString("  </Actions>\n")
	b.WriteString("</Task>\n")
	return b.String(), nil
}

// escapeXML escapes character data through encoding/xml rather than by hand, so
// the escaping rules are the standard library's and not this package's guess.
func escapeXML(value string) string {
	var out strings.Builder
	if err := xml.EscapeText(&out, []byte(value)); err != nil {
		// strings.Builder never fails a write.
		return ""
	}
	return out.String()
}

// windowsArgument quotes one argument for the single Arguments string.
//
// Task Scheduler hands Arguments to the process as a command line, and the
// process splits it with CommandLineToArgvW. A config root containing a space -
// which is the default on Windows for anyone whose account name has one - is
// two arguments unless it is quoted here. Backslashes are only special
// immediately before a quote, where each one has to be doubled; that is the
// rule this implements. Nothing here goes through cmd.exe, so cmd's own
// metacharacters are not in play, and the quote character itself is already
// rejected by checkRenderablePaths before this is reached.
func windowsArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t") {
		return value
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range value {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteByte('"')
			backslashes = 0
		default:
			b.WriteString(strings.Repeat(`\`, backslashes))
			backslashes = 0
			b.WriteRune(r)
		}
	}
	b.WriteString(strings.Repeat(`\`, backslashes*2))
	b.WriteByte('"')
	return b.String()
}

// encodeTaskXML turns the rendered document into the bytes schtasks reads.
//
// schtasks /Create /XML wants a Unicode file: given UTF-8 it reports "The task
// XML is malformed" for any document containing a non-ASCII character, which
// on this path means any member whose home directory has one in it. UTF-16LE
// with a byte-order mark is what Task Scheduler itself exports.
func encodeTaskXML(document string) []byte {
	units := utf16.Encode([]rune(document))
	out := make([]byte, 0, 2+len(units)*2)
	out = append(out, 0xFF, 0xFE) // UTF-16LE byte-order mark.
	for _, unit := range units {
		out = append(out, byte(unit), byte(unit>>8))
	}
	return out
}

// taskNotFound reports that schtasks refused because the task is not
// registered, which for Stop and Remove means the desired end state already
// holds. It is the Task Scheduler counterpart of the darwin notLoaded.
func taskNotFound(err error) bool {
	return mentions(err,
		"cannot find the file specified",
		"does not exist",
		"the system cannot find",
		"error: the specified task name",
		"0x80070002",
		"0x8004131f",
	)
}

// taskAlreadyRunning reports that schtasks refused a /Run because an instance
// is already running, which is the desired end state. It is the Task Scheduler
// counterpart of the darwin alreadyLoaded.
func taskAlreadyRunning(err error) bool {
	return mentions(err,
		"already running",
		"an instance of this task is already running",
		"0x00041301",
		"0x80041318",
	)
}

// taskNotRunning reports that schtasks refused an /End because nothing was
// running, which for Stop and Remove is the desired end state.
func taskNotRunning(err error) bool {
	return mentions(err, "is not running", "not currently running", "0x8004131b", "0x00041303")
}

// taskIsRunning reads the Status column out of "schtasks /Query /FO CSV /V".
//
// The column order of the verbose CSV is fixed - HostName, TaskName, Next Run
// Time, Status, ... - so the index is stable, but the values themselves are
// localised. On a non-English Windows this therefore falls back to reporting
// the service as not running rather than guessing. That is acceptable because
// it is the weaker of two signals: "overgent service status" asks the running
// daemon over its own endpoint first and only falls through to Task Scheduler
// when nothing answers.
func taskIsRunning(output string) bool {
	reader := csv.NewReader(strings.NewReader(output))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return false
	}
	const statusColumn = 3
	for _, record := range records {
		if len(record) <= statusColumn {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(record[statusColumn]), "running") {
			return true
		}
	}
	return false
}
