package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/localbackend"
	"github.com/khalidM3/overgent/internal/onboarding"
)

func TestCreateRefusesBothLocalAndAPI(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	// --local and --api both name where the Project's coordination data goes.
	// Accepting both would mean silently ignoring one of them.
	err := run([]string{"--config-root", t.TempDir(), "--api", "https://api.example.com", "create", "--local", "--label", "Atlas"})
	if err == nil || !strings.Contains(err.Error(), "--local or --api") {
		t.Fatalf("create accepted both placements: %v", err)
	}
}

func TestBackendStatusJSONShapeIsStableAndCarriesNoInstanceName(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	root := t.TempDir()
	bundle := filepath.Join(root, "backend-push.json")
	binary := filepath.Join(root, "convex-local-backend")
	for _, path := range []string{bundle, binary} {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := localbackend.Install(root, binary, bundle); err != nil {
		t.Fatal(err)
	}
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	status, err := backendStatus(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if status.Running {
		t.Fatal("a backend that was never started reported as running")
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["running"]; !ok {
		t.Fatalf("backend status has no running field: %s", encoded)
	}
	if !strings.Contains(string(encoded), "databasePath") {
		t.Fatalf("backend status omits the database path: %s", encoded)
	}
	// instanceName names the Keychain item holding the instance secret, so it
	// is deliberately absent from anything a member can paste into an issue.
	state, err := os.ReadFile(localbackend.StatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), "instanceName") {
		t.Fatal("backend.json did not record an instance name")
	}
	if strings.Contains(string(encoded), "instanceName") {
		t.Fatalf("backend status disclosed the instance name: %s", encoded)
	}
}

func TestBackendCommandsRejectAnUninstalledProfile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	root := t.TempDir()
	if err := run([]string{"--config-root", root, "backend", "status", "--json"}); err == nil {
		t.Fatal("backend status succeeded on a profile with no backend")
	}
	if err := run([]string{"--config-root", root, "backend"}); err == nil {
		t.Fatal("bare backend command succeeded")
	}
}

func TestServerOriginValidationIsOneRuleAndOneMessage(t *testing.T) {
	// The desktop field and --api must accept and refuse the same things, or a
	// member is told the server is fine in one place and wrong in the other.
	for _, accepted := range []string{"https://api.overgent.com", "https://api.example.com/", "http://127.0.0.1:3211"} {
		if _, err := onboarding.ValidateAPIOrigin(accepted); err != nil {
			t.Fatalf("rejected %q: %v", accepted, err)
		}
	}
	for _, refused := range []string{"", "http://example.com", "https://api.example.com/v1", "https://user:pw@api.example.com", "not a url", "ftp://example.com"} {
		if _, err := onboarding.ValidateAPIOrigin(refused); err == nil {
			t.Fatalf("accepted %q", refused)
		}
	}
	if _, err := onboarding.ValidateAPIOrigin("http://example.com"); err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("the refusal does not name what is accepted: %v", err)
	}
	trimmed, err := onboarding.ValidateAPIOrigin("  https://api.example.com/  ")
	if err != nil || trimmed != "https://api.example.com" {
		t.Fatalf("origin was not canonicalized: %q %v", trimmed, err)
	}
}

func TestDiagnosticsStillExcludesTheBackendDatabase(t *testing.T) {
	// The local backend's database holds this Mac's coordination facts, so the
	// allowlisted doctor summary must not start carrying it just because the
	// health response grew a backend block.
	report := safeDoctorSummary(map[string]any{
		"status": "ok", "workspaces": 1,
		"backend": map[string]any{"running": true, "port": 43103, "databasePath": "/Users/person/Library/Application Support/Overgent/backend/state.sqlite3"},
	})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"backend", "databasePath", "43103", "/Users/person"} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("diagnostics disclosed %q in %s", prohibited, encoded)
		}
	}
}

func TestBackendResetForgetsOnlyALocalEnrollment(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	// A team profile's Projects live on a server this command did not touch, so
	// resetting a local backend must never erase them.
	team := t.TempDir()
	teamPaths, err := config.Resolve(team)
	if err != nil {
		t.Fatal(err)
	}
	teamConfig := config.Config{Version: 1, APIBaseURL: "https://api.overgent.com", DeviceID: "dev_team",
		Workspaces: []config.Workspace{{ID: "wsp_team", ProjectID: "prj_team", Root: t.TempDir()}}}
	if err = config.Save(teamPaths, teamConfig); err != nil {
		t.Fatal(err)
	}
	cleared, err := clearLocalEnrollment(t.Context(), teamPaths)
	if err != nil || cleared != 0 {
		t.Fatalf("a team enrollment was cleared: %d %v", cleared, err)
	}
	after, err := config.Load(teamPaths)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Workspaces) != 1 || after.DeviceID != "dev_team" {
		t.Fatalf("the team enrollment was modified: %+v", after)
	}

	// A local profile's Projects lived in the database that was just deleted,
	// so the enrollment naming them has to go with it or the app comes back
	// showing a Project its backend has never heard of.
	local := t.TempDir()
	localPaths, err := config.Resolve(local)
	if err != nil {
		t.Fatal(err)
	}
	localConfig := config.Config{Version: 1, APIBaseURL: "http://127.0.0.1:51601", DeviceID: "dev_local",
		Workspaces: []config.Workspace{{ID: "wsp_local", ProjectID: "prj_local", Root: t.TempDir()}}}
	if err = config.Save(localPaths, localConfig); err != nil {
		t.Fatal(err)
	}
	cleared, err = clearLocalEnrollment(t.Context(), localPaths)
	if err != nil || cleared != 1 {
		t.Fatalf("the local enrollment was not cleared: %d %v", cleared, err)
	}
	after, err = config.Load(localPaths)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Workspaces) != 0 || after.DeviceID != "" || after.APIBaseURL != "" {
		t.Fatalf("first run was not restored: %+v", after)
	}
}
