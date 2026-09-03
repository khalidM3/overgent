package codexsetup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/khalidM3/overgent/internal/codexappserver"
)

// Trust methods, ordered by how much of the work Codex itself performs.
const (
	// TrustMethodAppServer is the supported path: Codex computes the hash and
	// Codex writes it.
	TrustMethodAppServer = "app_server"
	// TrustMethodAppendedConfig is the degraded path used when Codex can report
	// hook hashes but refuses or no longer offers config/batchWrite. Overgent
	// appends the trust tables itself, and only ones that do not yet exist.
	TrustMethodAppendedConfig = "appended_config"
	// TrustMethodManual is the honest floor: hooks are installed but Codex will
	// not run them until the member reviews them.
	TrustMethodManual = "manual"
)

// ReviewGuidance is shown to a member who must trust the hooks by hand. Codex
// surfaces the same review in the desktop application and in the CLI.
const ReviewGuidance = "Open Codex → Settings → Hooks and choose Trust all, or run /hooks in the Codex CLI."

// TrustReport describes whether Codex will actually run Overgent's hooks.
//
// A binding whose files are on disk is not a binding that runs: Codex skips an
// untrusted hook silently. Setup and status both carry this so the desktop app
// can distinguish a working install from an inert one (ADR-051).
type TrustReport struct {
	Method   string   `json:"method"`
	Total    int      `json:"total"`
	Trusted  int      `json:"trusted"`
	Pending  []string `json:"pending,omitempty"`
	Guidance string   `json:"guidance,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// Satisfied reports whether every managed hook Codex resolved is trusted.
// A report that observed no hooks at all is not satisfied: that means Codex
// could not see the binding, which is exactly the silent failure to surface.
func (r TrustReport) Satisfied() bool { return r.Total > 0 && r.Trusted == r.Total }

// inspectTrust is the seam unit tests replace. Trust repair spawns a Codex
// child process and reads the member's real Codex home when it is not
// overridden, and no unit test may do either.
var inspectTrust = func(m Manager, ctx context.Context, hookPath, hookCommand string, repair bool) TrustReport {
	return m.ensureHookTrust(ctx, hookPath, hookCommand, repair)
}

// ensureHookTrust reports whether Codex trusts Overgent's hooks and, when
// repair is set, records the trust Codex is missing.
//
// It never returns an error. Trust repair is an accelerator on top of an
// install that already succeeded; a machine without Codex on disk, an
// experimental protocol that moved, or a refused write must all degrade to
// "the member reviews these by hand", never to a failed setup. With repair
// unset this only observes, so status never mutates the member's Codex config.
func (m Manager) ensureHookTrust(ctx context.Context, hookPath, hookCommand string, repair bool) TrustReport {
	report := TrustReport{Method: TrustMethodManual, Guidance: ReviewGuidance}

	executable := m.CodexExecutable
	if executable == "" {
		located, err := codexappserver.Locate()
		if err != nil {
			report.Detail = "Codex executable was not found; hooks stay pending until Codex is installed."
			if !errors.Is(err, codexappserver.ErrCodexNotFound) {
				report.Detail = err.Error()
			}
			return report
		}
		executable = located
	}
	home, err := codexappserver.Home(m.CodexHome)
	if err != nil {
		report.Detail = err.Error()
		return report
	}

	dialCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	client, err := codexappserver.Dial(dialCtx, codexappserver.Options{
		Executable: executable, CodexHome: home, ClientVersion: m.Version,
	})
	if err != nil {
		report.Detail = err.Error()
		return report
	}
	defer client.Close()

	cwds := []string{}
	if project, projectErr := filepath.Abs(m.ProjectRoot); projectErr == nil {
		cwds = append(cwds, project)
	}
	hooks, err := client.ListHooks(dialCtx, cwds)
	if err != nil {
		report.Detail = err.Error()
		return report
	}
	ours := selectManagedHooks(hooks, hookPath, hookCommand)
	report.Total = len(ours)
	if len(ours) == 0 {
		report.Detail = "Codex did not resolve any Overgent hooks for this project."
		return report
	}

	edits := make([]codexappserver.TrustEdit, 0, len(ours))
	for _, hook := range ours {
		if hook.Trusted() {
			continue
		}
		edits = append(edits, codexappserver.TrustEdit{Key: hook.Key, Hash: hook.CurrentHash})
	}
	if len(edits) == 0 {
		report.Method, report.Trusted, report.Guidance = TrustMethodAppServer, len(ours), ""
		return report
	}
	if !repair {
		report.Trusted, report.Pending = len(ours)-len(edits), pendingEvents(ours)
		return report
	}

	version, versionErr := client.UserConfigVersion(dialCtx)
	if versionErr != nil {
		// A missing concurrency token is not fatal; the write is a narrow
		// upsert either way. Record why the guard was unavailable.
		report.Detail = versionErr.Error()
	}
	method := TrustMethodAppServer
	if err = client.Trust(dialCtx, edits, version); err != nil {
		// Codex could still tell us the hashes, so fall back to appending the
		// trust tables ourselves rather than abandoning the repair.
		if appendErr := appendTrustTables(filepath.Join(home, "config.toml"), edits); appendErr != nil {
			report.Detail = fmt.Sprintf("%v; %v", err, appendErr)
			report.Pending = pendingEvents(ours)
			return report
		}
		method = TrustMethodAppendedConfig
		report.Detail = "config/batchWrite was unavailable: " + err.Error()
	}

	// Re-ask Codex rather than assuming the write took. This is the only
	// statement of success that is worth anything.
	verified, verifyErr := client.ListHooks(dialCtx, cwds)
	if verifyErr != nil {
		report.Detail = strings.TrimPrefix(report.Detail+"; "+verifyErr.Error(), "; ")
		report.Pending = pendingEvents(ours)
		return report
	}
	confirmed := selectManagedHooks(verified, hookPath, hookCommand)
	report.Method, report.Total, report.Trusted = method, len(confirmed), 0
	for _, hook := range confirmed {
		if hook.Trusted() {
			report.Trusted++
		}
	}
	report.Pending = pendingEvents(confirmed)
	if report.Satisfied() {
		report.Guidance = ""
	}
	return report
}

// selectManagedHooks narrows a hooks/list result to the handlers this profile
// installed. Matching on the exact command string is what keeps trust repair
// from ever touching a hook Overgent does not own — including another profile's
// Overgent hook, whose command carries a different config root.
func selectManagedHooks(hooks []codexappserver.Hook, hookPath, hookCommand string) []codexappserver.Hook {
	var ours []codexappserver.Hook
	for _, hook := range hooks {
		if hook.HandlerType != "command" || hook.Command != hookCommand || hook.IsManaged {
			continue
		}
		if hookPath != "" && hook.SourcePath != "" && !samePath(hook.SourcePath, hookPath) {
			continue
		}
		ours = append(ours, hook)
	}
	return ours
}

func pendingEvents(hooks []codexappserver.Hook) []string {
	var pending []string
	for _, hook := range hooks {
		if !hook.Trusted() {
			pending = append(pending, hook.EventName)
		}
	}
	sort.Strings(pending)
	return pending
}

func samePath(left, right string) bool {
	if left == right {
		return true
	}
	leftResolved, leftErr := filepath.EvalSymlinks(left)
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	return leftErr == nil && rightErr == nil && leftResolved == rightResolved
}

// appendTrustTables writes trust entries by appending whole TOML tables to the
// end of the user's config.
//
// This is the fallback for a Codex that can list hooks but not accept a
// configuration write. It is append-only on purpose: a `[table]` header at end
// of file always opens a new table, so no existing line is reinterpreted, and
// an entry whose key already appears is skipped rather than duplicated —
// a duplicate table would make the whole config unparseable and take Codex
// down with it. Overgent never rewrites a byte it did not add.
func appendTrustTables(configPath string, edits []codexappserver.TrustEdit) error {
	current, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read Codex config: %w", err)
	}
	if len(current) > 1<<20 {
		return errors.New("Codex config exceeds 1 MiB")
	}
	existing := string(current)
	var addition strings.Builder
	for _, edit := range edits {
		header := fmt.Sprintf("[hooks.state.%q]", edit.Key)
		if strings.Contains(existing, header) {
			continue
		}
		if addition.Len() == 0 && len(existing) > 0 && !strings.HasSuffix(existing, "\n") {
			addition.WriteString("\n")
		}
		fmt.Fprintf(&addition, "\n%s\ntrusted_hash = %q\n", header, edit.Hash)
	}
	if addition.Len() == 0 {
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create Codex config directory: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(configPath); statErr == nil {
		mode = info.Mode().Perm()
	}
	return atomicWrite(configPath, []byte(existing+addition.String()), mode)
}
