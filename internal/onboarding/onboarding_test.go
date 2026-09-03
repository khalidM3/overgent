package onboarding

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/hosted"
)

type additionalProjectAPI struct {
	deviceID string
	project  hosted.Project
	revoked  bool
}

func (api *additionalProjectAPI) CreateProject(context.Context, string, string, string, string) (hosted.Project, error) {
	return api.project, nil
}
func (*additionalProjectAPI) CreateInvite(context.Context, string, int, int) (hosted.Invite, error) {
	return hosted.Invite{ID: "inv_fixture", Secret: "fixture-secret"}, nil
}
func (*additionalProjectAPI) Enroll(context.Context, string, string, string, string, string) (hosted.Enrollment, error) {
	return hosted.Enrollment{}, nil
}
func (api *additionalProjectAPI) Bootstrap(context.Context) (hosted.Bootstrap, error) {
	return hosted.Bootstrap{DeviceID: api.deviceID, Projects: []hosted.Project{api.project}}, nil
}
func (*additionalProjectAPI) CreateDashboardTicket(context.Context, string) (hosted.DashboardTicket, error) {
	return hosted.DashboardTicket{Ticket: "dashboard-ticket-fixture"}, nil
}
func (api *additionalProjectAPI) RevokeDevice(context.Context, string) error {
	api.revoked = true
	return nil
}

func TestCreateAdditionalReusesDeviceAndRegistersWorkspace(t *testing.T) {
	root := makeOnboardingRepository(t)
	api := &additionalProjectAPI{deviceID: "dev_existing", project: hosted.Project{ID: "prj_second", Label: "Second"}}
	var registered config.Workspace
	service := Service{
		Client: func(token string) (API, error) {
			if token != "existing-token" {
				t.Fatalf("token = %q", token)
			}
			return api, nil
		},
		Register: func(_ context.Context, _, _, deviceID string, workspace config.Workspace) error {
			if deviceID != api.deviceID {
				t.Fatalf("device = %q", deviceID)
			}
			registered = workspace
			return nil
		},
	}
	result, err := service.CreateAdditional(context.Background(), Options{
		ConfigRoot: t.TempDir(), RepositoryRoot: root, APIBaseURL: "https://api.overgent.com",
		ProjectLabel: "Second", DeviceLabel: "This Mac",
	}, api.deviceID, "existing-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectID != api.project.ID || result.DeviceID != api.deviceID || registered.ProjectID != api.project.ID || registered.Root != root {
		t.Fatalf("result=%+v registered=%+v", result, registered)
	}
	if result.JoinCode != "inv_fixture.fixture-secret" || result.DashboardTicket == "" {
		t.Fatalf("missing activation material: %+v", result)
	}
	if api.revoked {
		t.Fatal("shared existing device was revoked")
	}
}

func makeOnboardingRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	runGit("init")
	runGit("config", "user.email", "fixture@example.test")
	runGit("config", "user.name", "Fixture")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "fixture")
	runGit("remote", "add", "origin", "https://example.test/overgent/fixture.git")
	return root
}

// An invite travels as a link, and the same string must work wherever it is
// pasted: the bare code, the join-page URL whose fragment carries it (the
// fragment never reaches server logs), or the desktop deep link. A member
// should never have to dissect a URL to extract a code by hand.
func TestParseInviteCodeAcceptsEveryLinkForm(t *testing.T) {
	cases := map[string]string{
		"inv_49b778cd.sec_ret-42":                                "inv_49b778cd.sec_ret-42",
		"https://dash.example.com/join#inv_49b778cd.sec_ret-42":  "inv_49b778cd.sec_ret-42",
		"https://dash.example.com/join/#inv_49b778cd.sec_ret-42": "inv_49b778cd.sec_ret-42",
		"overgent://join/inv_49b778cd.sec_ret-42":                "inv_49b778cd.sec_ret-42",
	}
	for input, want := range cases {
		code, err := ParseInviteCode(input)
		if err != nil || code != want {
			t.Fatalf("ParseInviteCode(%q) = %q, %v; want %q", input, code, err, want)
		}
	}
	for _, invalid := range []string{
		"",
		"no-dot-here",
		"https://dash.example.com/join",
		"https://dash.example.com/join#not a code",
		"overgent://settings/inv_x.secret",
		"javascript:alert(1)#inv_x.secret",
	} {
		if _, err := ParseInviteCode(invalid); err == nil {
			t.Fatalf("ParseInviteCode(%q) accepted invalid input", invalid)
		}
	}
}
