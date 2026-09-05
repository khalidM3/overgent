package onboarding

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/hosted"
)

// testBackend is the team backend every fixture flow targets. A flow now
// carries its backend, because "which server" and "which credential" are one
// question (ADR-074).
var testBackend = config.Backend{ID: config.BackendID("https://api.overgent.com"), APIBaseURL: "https://api.overgent.com", Kind: config.KindTeam}

type additionalProjectAPI struct {
	deviceID string
	// enrollDeviceID is the identity /v1/enrollments mints. It is set only by
	// the tests that exercise a backend this profile has never used.
	enrollDeviceID string
	project        hosted.Project
	revoked        bool
	joinErr        error
}

func (api *additionalProjectAPI) CreateProject(context.Context, string, string, string, string) (hosted.Project, error) {
	return api.project, nil
}
func (*additionalProjectAPI) CreateInvite(context.Context, string, int, int) (hosted.Invite, error) {
	return hosted.Invite{ID: "inv_fixture", Secret: "fixture-secret"}, nil
}
func (api *additionalProjectAPI) Enroll(context.Context, string, string, string, string, string) (hosted.Enrollment, error) {
	return hosted.Enrollment{DeviceID: api.enrollDeviceID, DeviceToken: "minted-token", DashboardTicket: "dashboard-ticket-fixture"}, nil
}
func (api *additionalProjectAPI) JoinProject(context.Context, string, string, string, string, string) (hosted.Membership, error) {
	return hosted.Membership{ProjectID: api.project.ID, DashboardTicket: "dashboard-ticket-fixture"}, api.joinErr
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
		Backend: testBackend,
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
		ConfigRoot: t.TempDir(), RepositoryRoot: root,
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

// Joining a second Project must reuse this Mac's device rather than mint a new
// one. Before /v1/memberships existed the only join path enrolled a fresh
// device, which app.Register then rejected because the profile already held a
// different device ID - so an invite was burned and nothing was joined.
func TestJoinAdditionalReusesDeviceAndNeverRevokesIt(t *testing.T) {
	root := makeOnboardingRepository(t)
	api := &additionalProjectAPI{deviceID: "dev_existing", project: hosted.Project{ID: "prj_invited", Label: "Invited"}}
	var registered config.Workspace
	service := Service{
		Backend: testBackend,
		Client: func(token string) (API, error) {
			if token != "existing-token" {
				t.Fatalf("token = %q", token)
			}
			return api, nil
		},
		Register: func(_ context.Context, _, _, deviceID string, workspace config.Workspace) error {
			registered = workspace
			if deviceID != api.deviceID {
				t.Fatalf("device = %q", deviceID)
			}
			return nil
		},
	}
	options := Options{ConfigRoot: t.TempDir(), RepositoryRoot: root, DeviceLabel: "This Mac"}
	result, err := service.JoinAdditional(context.Background(), options, api.deviceID, "existing-token", "inv_abcdef.secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectID != api.project.ID || registered.ProjectID != api.project.ID || registered.Root != root {
		t.Fatalf("result=%+v registered=%+v", result, registered)
	}
	if result.DashboardTicket == "" {
		t.Fatalf("no activation material: %+v", result)
	}
	// A member who just accepted an invite has not been given one to pass on.
	if result.JoinCode != "" {
		t.Fatalf("joining minted an invite: %+v", result)
	}
	if api.revoked {
		t.Fatal("the device holding every other Project was revoked")
	}
}

// The rollback that protects a first enrollment would be catastrophic here, so
// a failed join must leave the shared device alone and register nothing.
func TestJoinAdditionalLeavesTheSharedDeviceAloneWhenTheInviteFails(t *testing.T) {
	root := makeOnboardingRepository(t)
	api := &additionalProjectAPI{deviceID: "dev_existing", project: hosted.Project{ID: "prj_invited"}, joinErr: errors.New("E:invite_expired")}
	registered := false
	service := Service{
		Backend:  testBackend,
		Client:   func(string) (API, error) { return api, nil },
		Register: func(context.Context, string, string, string, config.Workspace) error { registered = true; return nil },
	}
	options := Options{ConfigRoot: t.TempDir(), RepositoryRoot: root, DeviceLabel: "This Mac"}
	if _, err := service.JoinAdditional(context.Background(), options, api.deviceID, "existing-token", "inv_abcdef.secret-value"); err == nil {
		t.Fatal("an expired invite was accepted")
	}
	if api.revoked {
		t.Fatal("a failed join revoked the device holding every other Project")
	}
	if registered {
		t.Fatal("a failed join registered a workspace")
	}
}

// A malformed code is refused before anything reaches the network, so a typo
// cannot spend one of the invite's rate-limited attempts.
func TestJoinAdditionalRejectsAMalformedCodeLocally(t *testing.T) {
	api := &additionalProjectAPI{deviceID: "dev_existing"}
	called := false
	service := Service{Backend: testBackend, Client: func(string) (API, error) { called = true; return api, nil }}
	options := Options{ConfigRoot: t.TempDir(), RepositoryRoot: t.TempDir(), DeviceLabel: "This Mac"}
	if _, err := service.JoinAdditional(context.Background(), options, "dev_existing", "existing-token", "not-an-invite"); err == nil {
		t.Fatal("a malformed code was accepted")
	}
	if called {
		t.Fatal("a malformed code reached the hosted API")
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
		code, _, err := ParseInviteCode(input)
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
		if _, _, err := ParseInviteCode(invalid); err == nil {
			t.Fatalf("ParseInviteCode(%q) accepted invalid input", invalid)
		}
	}
}

// An https invite link names the server the Project lives on, and that is the
// difference between a member pasting a link and being asked which server
// their friend uses. The other two forms name no server and fall back to
// whichever backend the caller selected.
func TestParseInviteCodeReportsTheOriginOnlyForALink(t *testing.T) {
	cases := map[string]struct{ code, origin string }{
		"inv_49b778cd.sec_ret-42":                                {"inv_49b778cd.sec_ret-42", ""},
		"overgent://join/inv_49b778cd.sec_ret-42":                {"inv_49b778cd.sec_ret-42", ""},
		"https://dash.example.com/join#inv_49b778cd.sec_ret-42":  {"inv_49b778cd.sec_ret-42", "https://dash.example.com"},
		"https://dash.example.com/join/#inv_49b778cd.sec_ret-42": {"inv_49b778cd.sec_ret-42", "https://dash.example.com"},
		"https://team.example.com:8443/join#inv_a1.b2":           {"inv_a1.b2", "https://team.example.com:8443"},
	}
	for input, want := range cases {
		code, origin, err := ParseInviteCode(input)
		if err != nil || code != want.code || origin != want.origin {
			t.Fatalf("ParseInviteCode(%q) = %q, %q, %v; want %q, %q", input, code, origin, err, want.code, want.origin)
		}
	}
}

// Parsing stays strict, because the origin it returns is where a credential is
// about to be sent. A link that names a path other than /join is not an
// invite; userinfo is how a host is made to read as something it is not; and a
// scheme carrying a host must still be https.
func TestParseInviteCodeRefusesLinksThatCouldMisnameTheServer(t *testing.T) {
	for _, invalid := range []string{
		"https://dash.example.com/settings#inv_49b778cd.sec_ret-42",
		"https://dash.example.com/join/extra#inv_49b778cd.sec_ret-42",
		"https://attacker@dash.example.com/join#inv_49b778cd.sec_ret-42",
		"https://dash.example.com/join?next=x#inv_49b778cd.sec_ret-42",
		"http://dash.example.com/join#inv_49b778cd.sec_ret-42",
		"ftp://dash.example.com/join#inv_49b778cd.sec_ret-42",
		"overgent://dash.example.com/join/inv_49b778cd.sec_ret-42",
	} {
		if code, origin, err := ParseInviteCode(invalid); err == nil {
			t.Fatalf("ParseInviteCode(%q) accepted it as %q on %q", invalid, code, origin)
		}
	}
}

// Joining a friend's team Project from a profile that has only a local one is
// the common path this lane exists for. The local backend's device identity
// must not be reused on a server that has never heard of it, so a backend the
// profile has not seen mints its own.
func TestJoinOnNewBackendMintsAnIdentityForAnUnseenServer(t *testing.T) {
	root := makeOnboardingRepository(t)
	api := &additionalProjectAPI{deviceID: "dev_new", enrollDeviceID: "dev_new", project: hosted.Project{ID: "prj_invited"}}
	creds := &recordingCreds{}
	var registeredOrigin, registeredDevice string
	service := Service{
		// The profile has never used this server, so the backend record it
		// targets carries no device identity.
		Backend: config.Backend{ID: config.BackendID("https://team.example.com"), APIBaseURL: "https://team.example.com", Kind: config.KindTeam},
		Client:  func(string) (API, error) { return api, nil },
		Creds:   creds,
		Register: func(_ context.Context, _, apiBaseURL, deviceID string, _ config.Workspace) error {
			registeredOrigin, registeredDevice = apiBaseURL, deviceID
			return nil
		},
	}
	options := Options{ConfigRoot: t.TempDir(), RepositoryRoot: root, DeviceLabel: "This Mac"}
	result, err := service.JoinOnNewBackend(context.Background(), options, "https://team.example.com/join#inv_abcdef.secret-value")
	if err != nil {
		t.Fatalf("JoinOnNewBackend() error = %v", err)
	}
	if result.DeviceID != "dev_new" {
		t.Fatalf("result = %+v, want a freshly minted identity", result)
	}
	if registeredOrigin != "https://team.example.com" || registeredDevice != "dev_new" {
		t.Fatalf("registered %q with %q", registeredOrigin, registeredDevice)
	}
	// The credential is stored under the new device, beside whatever the local
	// backend already holds; it never replaces it.
	if len(creds.stored) != 1 || creds.stored[0] != "dev_new" {
		t.Fatalf("stored credentials = %v", creds.stored)
	}
}

// A backend the profile already has an identity on reuses it. Minting a second
// credential for one server would strand the Projects the first one holds.
func TestJoinOnNewBackendReusesTheIdentityItAlreadyHas(t *testing.T) {
	root := makeOnboardingRepository(t)
	api := &additionalProjectAPI{deviceID: "dev_existing", project: hosted.Project{ID: "prj_invited"}}
	creds := &recordingCreds{token: "existing-token"}
	service := Service{
		Backend:  config.Backend{ID: testBackend.ID, APIBaseURL: testBackend.APIBaseURL, DeviceID: "dev_existing", Kind: config.KindTeam},
		Client:   func(string) (API, error) { return api, nil },
		Creds:    creds,
		Register: func(context.Context, string, string, string, config.Workspace) error { return nil },
	}
	options := Options{ConfigRoot: t.TempDir(), RepositoryRoot: root, DeviceLabel: "This Mac"}
	result, err := service.JoinOnNewBackend(context.Background(), options, "inv_abcdef.secret-value")
	if err != nil {
		t.Fatalf("JoinOnNewBackend() error = %v", err)
	}
	if result.DeviceID != "dev_existing" {
		t.Fatalf("result = %+v, want the identity this profile already holds", result)
	}
	if len(creds.stored) != 0 {
		t.Fatalf("a second credential was minted for one backend: %v", creds.stored)
	}
	if api.revoked {
		t.Fatal("the device holding every other Project on this backend was revoked")
	}
}

type recordingCreds struct {
	token  string
	stored []string
}

func (c *recordingCreds) Put(_ context.Context, account, _ string) error {
	c.stored = append(c.stored, account)
	return nil
}
func (c *recordingCreds) Get(context.Context, string) (string, error) { return c.token, nil }
func (c *recordingCreds) Delete(context.Context, string) error        { return nil }
