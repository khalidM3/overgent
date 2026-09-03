package onboarding

import (
	"context"
	"errors"
	"testing"

	"github.com/overgent/overgent/internal/config"
	"github.com/overgent/overgent/internal/hosted"
)

type resetAPI struct{ bootstrapErr error }

func (a *resetAPI) CreateProject(context.Context, string, string, string, string) (hosted.Project, error) {
	return hosted.Project{}, errors.New("unused")
}
func (a *resetAPI) CreateInvite(context.Context, string, int, int) (hosted.Invite, error) {
	return hosted.Invite{}, errors.New("unused")
}
func (a *resetAPI) Enroll(context.Context, string, string, string, string, string) (hosted.Enrollment, error) {
	return hosted.Enrollment{}, errors.New("unused")
}
func (a *resetAPI) Bootstrap(context.Context) (hosted.Bootstrap, error) {
	return hosted.Bootstrap{DeviceID: "dev_local"}, a.bootstrapErr
}
func (a *resetAPI) CreateDashboardTicket(context.Context, string) (hosted.DashboardTicket, error) {
	return hosted.DashboardTicket{}, errors.New("unused")
}
func (a *resetAPI) RevokeDevice(context.Context, string) error { return errors.New("unused") }

type fakeCreds struct {
	token   string
	getErr  error
	deleted []string
}

func (f *fakeCreds) Put(context.Context, string, string) error   { return nil }
func (f *fakeCreds) Get(context.Context, string) (string, error) { return f.token, f.getErr }
func (f *fakeCreds) Delete(_ context.Context, account string) error {
	f.deleted = append(f.deleted, account)
	return nil
}

func enrolledProfile(t *testing.T) (string, config.Paths) {
	t.Helper()
	root := t.TempDir()
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	cfg := config.Config{
		Version: 1, APIBaseURL: "https://api.overgent.com", DeviceID: "dev_local",
		Workspaces: []config.Workspace{{ID: "wsp_1", ProjectID: "prj_1", Root: root}},
	}
	if err := config.Save(paths, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	return root, paths
}

func serviceFor(api *resetAPI, creds *fakeCreds) Service {
	return Service{Client: func(string) (API, error) { return api, nil }, Creds: creds}
}

func TestResetClearsARejectedEnrollment(t *testing.T) {
	root, paths := enrolledProfile(t)
	creds := &fakeCreds{token: "stale-token"}
	service := serviceFor(&resetAPI{bootstrapErr: &hosted.APIError{Status: 401, Code: "credential_revoked"}}, creds)

	outcome, err := service.Reset(context.Background(), root, false)
	if err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if outcome.Status != hosted.CredentialRevoked || outcome.ClearedWorkspaces != 1 || !outcome.CredentialDeleted {
		t.Fatalf("outcome = %+v", outcome)
	}
	if len(creds.deleted) != 1 || creds.deleted[0] != "dev_local" {
		t.Fatalf("deleted = %v", creds.deleted)
	}
	after, err := config.Load(paths)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if after.DeviceID != "" || len(after.Workspaces) != 0 {
		t.Fatalf("device identity survived the reset: %+v", after)
	}
	if after.APIBaseURL != "https://api.overgent.com" {
		t.Fatalf("apiBaseURL = %q, want the origin kept for re-enrollment", after.APIBaseURL)
	}
}

func TestResetRefusesWhenTheCredentialStillWorks(t *testing.T) {
	root, paths := enrolledProfile(t)
	creds := &fakeCreds{token: "good-token"}
	service := serviceFor(&resetAPI{}, creds)

	if _, err := service.Reset(context.Background(), root, false); err == nil {
		t.Fatal("Reset() must refuse to erase a working enrollment")
	}
	after, _ := config.Load(paths)
	if after.DeviceID != "dev_local" || len(after.Workspaces) != 1 {
		t.Fatalf("a working enrollment was modified: %+v", after)
	}
	if len(creds.deleted) != 0 {
		t.Fatalf("a working credential was deleted: %v", creds.deleted)
	}
}

func TestResetRefusesWhenItCannotVerify(t *testing.T) {
	root, paths := enrolledProfile(t)
	creds := &fakeCreds{token: "unverified-token"}
	// Offline. This must never be mistaken for being locked out.
	service := serviceFor(&resetAPI{bootstrapErr: errors.New("call hosted API: connection refused")}, creds)

	if _, err := service.Reset(context.Background(), root, false); err == nil {
		t.Fatal("Reset() must refuse when the credential could not be verified")
	}
	after, _ := config.Load(paths)
	if after.DeviceID != "dev_local" {
		t.Fatalf("an unverified enrollment was erased: %+v", after)
	}
	if len(creds.deleted) != 0 {
		t.Fatalf("an unverified credential was deleted: %v", creds.deleted)
	}
}

func TestResetForceOverridesAnUnverifiableCheck(t *testing.T) {
	root, paths := enrolledProfile(t)
	creds := &fakeCreds{token: "unverified-token"}
	service := serviceFor(&resetAPI{bootstrapErr: errors.New("connection refused")}, creds)

	if _, err := service.Reset(context.Background(), root, true); err != nil {
		t.Fatalf("forced Reset() error = %v", err)
	}
	after, _ := config.Load(paths)
	if after.DeviceID != "" {
		t.Fatalf("forced reset left the device identity: %+v", after)
	}
}

func TestResetOnAnUnenrolledProfileIsANoOp(t *testing.T) {
	service := serviceFor(&resetAPI{}, &fakeCreds{})
	outcome, err := service.Reset(context.Background(), t.TempDir(), false)
	if err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if outcome.DeviceID != "" || outcome.ClearedWorkspaces != 0 {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestMissingKeychainEntryIsTreatedAsARejection(t *testing.T) {
	root, _ := enrolledProfile(t)
	// Nothing can authenticate with a secret that is gone, so the member gets
	// the same recovery as a rejection rather than a dead end.
	service := serviceFor(&resetAPI{}, &fakeCreds{getErr: errors.New("not found")})
	status, _, err := service.CredentialState(context.Background(), root)
	if err != nil {
		t.Fatalf("CredentialState() error = %v", err)
	}
	if status != hosted.CredentialUnknown {
		t.Fatalf("status = %q, want %q", status, hosted.CredentialUnknown)
	}
}
