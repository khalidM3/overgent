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

// Removing a Project must leave the backend it lived on standing. Two Projects
// on one loopback backend is the ordinary shape of a profile after ADR-074, and
// taking the backend down with the first of them would strand the second.
func TestRemoveProjectKeepsTheBackendAndItsOtherProjects(t *testing.T) {
	cfg := Single("http://127.0.0.1:43103", "dev_local", []Workspace{
		{ID: "wsp_first", ProjectID: "prj_first", Root: filepath.Join("/tmp", "first")},
		{ID: "wsp_second", ProjectID: "prj_second", Root: filepath.Join("/tmp", "second")},
	})
	next, cleared := cfg.RemoveProject("prj_first")
	if len(cleared) != 1 || cleared[0].ID != "wsp_first" {
		t.Fatalf("cleared = %+v", cleared)
	}
	if len(next.Backends) != 1 {
		t.Fatalf("the backend was taken down with the Project: %+v", next.Backends)
	}
	if len(next.Projects) != 1 || next.Projects[0].ID != "prj_second" {
		t.Fatalf("projects = %+v", next.Projects)
	}
	if len(next.Workspaces) != 1 || next.Workspaces[0].ID != "wsp_second" {
		t.Fatalf("workspaces = %+v", next.Workspaces)
	}
}

// The root a disconnected Project held is what makes it re-connectable: the
// enrollment guard refuses a repository that any workspace still names, and a
// deleted Project used to keep naming it forever.
func TestRemoveProjectReleasesTheRepositoryRoot(t *testing.T) {
	root := filepath.Join("/tmp", "checkout")
	cfg := Single("http://127.0.0.1:43103", "dev_local", []Workspace{{ID: "wsp_only", ProjectID: "prj_only", Root: root}})
	next, _ := cfg.RemoveProject("prj_only")
	for _, workspace := range next.Workspaces {
		if workspace.Root == root {
			t.Fatalf("root %s is still connected after the Project holding it was removed", root)
		}
	}
	if _, cleared := next.RemoveProject("prj_only"); len(cleared) != 0 {
		t.Fatal("removing an already-removed Project must clear nothing")
	}
}

// Promoting a local Project is a change of address: the Project and its
// repositories stay exactly as they are, and only the backend under them
// changes. The backend it leaves stays up for whatever else is on it.
func TestRebindProjectMovesOneProjectAndLeavesTheRestAlone(t *testing.T) {
	cfg := Single("http://127.0.0.1:43103", "dev_local", []Workspace{
		{ID: "wsp_moving", ProjectID: "prj_moving", Root: filepath.Join("/tmp", "moving")},
		{ID: "wsp_staying", ProjectID: "prj_staying", Root: filepath.Join("/tmp", "staying")},
	})
	cfg, cloud, err := cfg.UpsertBackend("https://api.overgent.com", "dev_cloud")
	if err != nil {
		t.Fatal(err)
	}
	next, err := cfg.RebindProject("prj_moving", cloud.ID)
	if err != nil {
		t.Fatal(err)
	}
	moved, bound := next.BackendForProject("prj_moving")
	if !bound || moved.ID != cloud.ID || moved.Kind != KindTeam {
		t.Fatalf("the Project did not move: %+v", moved)
	}
	stayed, bound := next.BackendForProject("prj_staying")
	if !bound || stayed.Kind != KindLocal {
		t.Fatalf("the Project beside it moved too: %+v", stayed)
	}
	if len(next.Backends) != 2 {
		t.Fatalf("the backend it left was taken down: %+v", next.Backends)
	}
	// The repositories are untouched, which is what makes this cheap: no
	// re-enrollment, no new fingerprint, no agent bindings to rewrite.
	if held := next.WorkspacesForProject("prj_moving"); len(held) != 1 || held[0].ID != "wsp_moving" {
		t.Fatalf("workspaces = %+v", held)
	}
	if _, err = next.RebindProject("prj_moving", cloud.ID); err == nil {
		t.Fatal("moving a Project to the backend it already lives on must be refused")
	}
	if _, err = next.RebindProject("prj_absent", cloud.ID); err == nil {
		t.Fatal("moving a Project this device does not hold must be refused")
	}
	if _, err = next.RebindProject("prj_moving", "bk_nonexistent"); err == nil {
		t.Fatal("moving a Project to an unknown backend must be refused")
	}
}
