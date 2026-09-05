package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPaths(t *testing.T) Paths {
	t.Helper()
	paths, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return paths
}

// A profile written before ADR-074 named one server and one device identity
// for the whole Mac. It has to keep working, keep its Projects, and keep the
// Keychain entry it already has - the migrated backend carries the same device
// id, so the account name that entry is stored under does not change.
func TestVersionOneLoadsAsOneBackendHoldingEveryProject(t *testing.T) {
	paths := testPaths(t)
	legacy := `{
  "version": 1,
  "apiBaseUrl": "https://api.overgent.com",
  "deviceId": "dev_legacy",
  "workspaces": [
    {"id": "wsp_a", "projectId": "prj_a", "workstreamId": "wrk_a", "root": "/tmp/a"},
    {"id": "wsp_b", "projectId": "prj_a", "workstreamId": "wrk_b", "root": "/tmp/b"},
    {"id": "wsp_c", "projectId": "prj_c", "workstreamId": "wrk_c", "root": "/tmp/c"}
  ]
}`
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(paths)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Version != Version || len(cfg.Backends) != 1 {
		t.Fatalf("migrated config = %+v", cfg)
	}
	backend := cfg.Backends[0]
	if backend.APIBaseURL != "https://api.overgent.com" || backend.DeviceID != "dev_legacy" || backend.Kind != KindTeam {
		t.Fatalf("migrated backend = %+v", backend)
	}
	// Two Projects, not three workspaces: the binding is per Project.
	if len(cfg.Projects) != 2 {
		t.Fatalf("migrated Projects = %+v", cfg.Projects)
	}
	for _, workspace := range cfg.Workspaces {
		resolved, bound := cfg.BackendForWorkspace(workspace)
		if !bound || resolved.ID != backend.ID {
			t.Fatalf("workspace %s resolved to %+v", workspace.ID, resolved)
		}
	}
}

// A loopback origin is a Project that lives on this Mac, and the kind is
// decided once, when the binding is written, rather than re-derived by every
// reader.
func TestMigrationRecordsTheBackendKind(t *testing.T) {
	cfg := Single("http://127.0.0.1:43103", "dev_local", []Workspace{{ID: "wsp_a", ProjectID: "prj_a"}})
	if cfg.Backends[0].Kind != KindLocal {
		t.Fatalf("loopback backend kind = %q", cfg.Backends[0].Kind)
	}
	for _, team := range []string{"https://api.overgent.com", "http://example.com", "https://127.0.0.1"} {
		if got := BackendKind(team); got != KindTeam {
			t.Fatalf("BackendKind(%q) = %q", team, got)
		}
	}
}

func TestSaveAndLoadRoundTripAVersionTwoProfile(t *testing.T) {
	paths := testPaths(t)
	cfg := Single("https://api.overgent.com", "dev_team", []Workspace{{ID: "wsp_team", ProjectID: "prj_team", Root: "/tmp/team"}})
	cfg, local, err := cfg.UpsertBackend("http://127.0.0.1:43103", "dev_local")
	if err != nil {
		t.Fatalf("UpsertBackend() error = %v", err)
	}
	cfg = cfg.BindProject("prj_local", local.ID)
	cfg.Workspaces = append(cfg.Workspaces, Workspace{ID: "wsp_local", ProjectID: "prj_local", Root: "/tmp/local"})
	if err := Save(paths, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load(paths)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded.Backends) != 2 || len(loaded.Projects) != 2 || len(loaded.Workspaces) != 2 {
		t.Fatalf("round trip = %+v", loaded)
	}
	team, bound := loaded.BackendForProject("prj_team")
	if !bound || team.Kind != KindTeam || team.DeviceID != "dev_team" {
		t.Fatalf("team binding = %+v", team)
	}
	localBackend, bound := loaded.BackendForProject("prj_local")
	if !bound || localBackend.Kind != KindLocal || localBackend.DeviceID != "dev_local" {
		t.Fatalf("local binding = %+v", localBackend)
	}
	// The version 1 fields must not survive a save: a later build reading them
	// back would resurrect a profile-wide origin that no longer means anything.
	raw, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if _, present := stored["apiBaseUrl"]; present {
		t.Fatalf("saved config kept the version 1 origin: %s", raw)
	}
	if _, present := stored["deviceId"]; present {
		t.Fatalf("saved config kept the version 1 device: %s", raw)
	}
	if version, _ := stored["version"].(float64); int(version) != Version {
		t.Fatalf("saved version = %v", stored["version"])
	}
}

// A configuration written by a newer build is refused rather than half-read.
// Silently dropping fields it does not understand is how a profile loses
// Projects.
func TestLoadRefusesAVersionItCannotUnderstand(t *testing.T) {
	paths := testPaths(t)
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config, []byte(`{"version": 3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(paths); err == nil || !strings.Contains(err.Error(), "unsupported config version") {
		t.Fatalf("Load() error = %v, want an unsupported-version refusal", err)
	}
}

// A workspace whose Project has no backend cannot publish anywhere. The
// service reports it and carries on with the rest of the profile, so this has
// to be an answer rather than a panic.
func TestBackendForWorkspaceReportsAnOrphanRatherThanGuessing(t *testing.T) {
	cfg := Single("https://api.overgent.com", "dev_team", []Workspace{{ID: "wsp_a", ProjectID: "prj_a"}})
	orphan := Workspace{ID: "wsp_orphan", ProjectID: "prj_orphan"}
	if backend, bound := cfg.BackendForWorkspace(orphan); bound || backend.ID != "" {
		t.Fatalf("an orphan workspace resolved to %+v", backend)
	}
	if _, bound := cfg.BackendForProject("prj_missing"); bound {
		t.Fatal("a Project with no binding resolved to a backend")
	}
}

// One profile holds one device identity per backend (ADR-069). A second
// identity for a server it already knows would strand the Projects the first
// one holds, so it is refused rather than merged.
func TestUpsertBackendRefusesASecondIdentityForOneServer(t *testing.T) {
	cfg := Single("https://api.overgent.com", "dev_first", nil)
	if _, _, err := cfg.UpsertBackend("https://api.overgent.com", "dev_second"); err == nil {
		t.Fatal("a second device identity was accepted for one backend")
	}
	// Re-recording the identity it already has is not a conflict, and neither
	// is a trailing slash on the same origin.
	next, backend, err := cfg.UpsertBackend("https://api.overgent.com/", "dev_first")
	if err != nil {
		t.Fatalf("UpsertBackend() error = %v", err)
	}
	if len(next.Backends) != 1 || backend.ID != cfg.Backends[0].ID {
		t.Fatalf("one server became two backends: %+v", next.Backends)
	}
}

// Reset is per backend, so removing one must take exactly its Projects and
// their repositories and leave everything else untouched.
func TestRemoveBackendTakesOnlyItsOwnProjects(t *testing.T) {
	cfg := Single("https://api.overgent.com", "dev_team", []Workspace{{ID: "wsp_team", ProjectID: "prj_team", Root: filepath.Join("/tmp", "team")}})
	cfg, local, err := cfg.UpsertBackend("http://127.0.0.1:43103", "dev_local")
	if err != nil {
		t.Fatal(err)
	}
	cfg = cfg.BindProject("prj_local", local.ID)
	cfg.Workspaces = append(cfg.Workspaces, Workspace{ID: "wsp_local", ProjectID: "prj_local", Root: filepath.Join("/tmp", "local")})
	next, cleared := cfg.RemoveBackend(local.ID)
	if cleared != 1 {
		t.Fatalf("cleared = %d", cleared)
	}
	if len(next.Backends) != 1 || next.Backends[0].Kind != KindTeam {
		t.Fatalf("backends = %+v", next.Backends)
	}
	if len(next.Workspaces) != 1 || next.Workspaces[0].ID != "wsp_team" {
		t.Fatalf("workspaces = %+v", next.Workspaces)
	}
	if len(next.Projects) != 1 || next.Projects[0].ID != "prj_team" {
		t.Fatalf("projects = %+v", next.Projects)
	}
}
