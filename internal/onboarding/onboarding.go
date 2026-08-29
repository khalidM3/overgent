package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/stickguy/stickguy/internal/app"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/credential"
	gitadapter "github.com/stickguy/stickguy/internal/git"
	"github.com/stickguy/stickguy/internal/hosted"
)

type API interface {
	CreateProject(context.Context, string, string, string, string) (hosted.Project, error)
	CreateInvite(context.Context, string, int, int) (hosted.Invite, error)
	Enroll(context.Context, string, string, string, string, string) (hosted.Enrollment, error)
	Bootstrap(context.Context) (hosted.Bootstrap, error)
	CreateDashboardTicket(context.Context, string) (hosted.DashboardTicket, error)
	RevokeDevice(context.Context, string) error
}

type CredentialStore interface {
	Put(context.Context, string, string) error
	Get(context.Context, string) (string, error)
	Delete(context.Context, string) error
}

type Options struct {
	ConfigRoot, RepositoryRoot, APIBaseURL, ProjectLabel, DeviceLabel, AppVersion string
	// DisplayName is optional; empty means the member has not chosen one yet.
	DisplayName string
}

type Result struct {
	ProjectID, DeviceID, WorkspaceID, WorkstreamID string
	JoinCode, DashboardTicket                      string
}

type Service struct {
	Client   func(token string) (API, error)
	Creds    CredentialStore
	Register func(context.Context, string, string, string, config.Workspace) error
}

type keychainStore struct{}

func (keychainStore) Put(ctx context.Context, account, secret string) error {
	return credential.Put(ctx, account, secret)
}
func (keychainStore) Get(ctx context.Context, account string) (string, error) {
	return credential.Get(ctx, account)
}
func (keychainStore) Delete(ctx context.Context, account string) error {
	return credential.Delete(ctx, account)
}

func New(apiBaseURL string) Service {
	return Service{
		Client:   func(token string) (API, error) { return hosted.New(apiBaseURL, token) },
		Creds:    keychainStore{},
		Register: app.Register,
	}
}

func (s Service) Create(ctx context.Context, options Options) (Result, error) {
	if err := validateOptions(options, true); err != nil {
		return Result{}, err
	}
	if err := preflightRepository(ctx, options.RepositoryRoot); err != nil {
		return Result{}, err
	}
	token, err := secret(32)
	if err != nil {
		return Result{}, err
	}
	client, err := s.Client(token)
	if err != nil {
		return Result{}, err
	}
	appVersion := options.AppVersion
	if appVersion == "" {
		appVersion = "stickguy/dev"
	}
	project, err := client.CreateProject(ctx, options.ProjectLabel, options.DeviceLabel, options.DisplayName, appVersion)
	if err != nil {
		return Result{}, fmt.Errorf("create Project: %w", err)
	}
	bootstrap, err := client.Bootstrap(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("bootstrap creator device: %w", err)
	}
	if bootstrap.DeviceID == "" || !containsProject(bootstrap.Projects, project.ID) {
		return Result{}, errors.New("creator bootstrap did not contain the new Project")
	}
	return s.finish(ctx, options, client, project.ID, bootstrap.DeviceID, token, "", true)
}

// CreateAdditional creates another Project for a device that is already
// enrolled in the local profile. The existing credential is deliberately
// reused: one per-user service has one device identity across its Projects.
// Unlike first enrollment, a local registration failure must never revoke the
// shared device and strand its existing Projects.
func (s Service) CreateAdditional(ctx context.Context, options Options, deviceID, token string) (Result, error) {
	if err := validateOptions(options, true); err != nil {
		return Result{}, err
	}
	if deviceID == "" || token == "" {
		return Result{}, errors.New("existing device ID and credential are required")
	}
	if err := preflightRepository(ctx, options.RepositoryRoot); err != nil {
		return Result{}, err
	}
	client, err := s.Client(token)
	if err != nil {
		return Result{}, err
	}
	appVersion := options.AppVersion
	if appVersion == "" {
		appVersion = "stickguy/dev"
	}
	project, err := client.CreateProject(ctx, options.ProjectLabel, options.DeviceLabel, options.DisplayName, appVersion)
	if err != nil {
		return Result{}, fmt.Errorf("create additional Project: %w", err)
	}
	bootstrap, err := client.Bootstrap(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("bootstrap existing device: %w", err)
	}
	if bootstrap.DeviceID != deviceID || !containsProject(bootstrap.Projects, project.ID) {
		return Result{}, errors.New("existing device bootstrap did not contain the new Project")
	}
	return s.finishExisting(ctx, options, client, project.ID, deviceID)
}

func (s Service) Join(ctx context.Context, options Options, joinCode string) (Result, error) {
	if err := validateOptions(options, false); err != nil {
		return Result{}, err
	}
	if err := preflightRepository(ctx, options.RepositoryRoot); err != nil {
		return Result{}, err
	}
	inviteID, inviteSecret, ok := strings.Cut(joinCode, ".")
	if !ok || inviteID == "" || inviteSecret == "" || strings.Contains(inviteSecret, ".") {
		return Result{}, errors.New("join code must have the form invite.secret")
	}
	publicClient, err := s.Client("")
	if err != nil {
		return Result{}, err
	}
	appVersion := options.AppVersion
	if appVersion == "" {
		appVersion = "stickguy/dev"
	}
	enrollment, err := publicClient.Enroll(ctx, inviteID, inviteSecret, options.DeviceLabel, options.DisplayName, appVersion)
	if err != nil {
		return Result{}, fmt.Errorf("enroll device: %w", err)
	}
	client, err := s.Client(enrollment.DeviceToken)
	if err != nil {
		return Result{}, err
	}
	bootstrap, err := client.Bootstrap(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("bootstrap joined device: %w", err)
	}
	if bootstrap.DeviceID != enrollment.DeviceID || len(bootstrap.Projects) != 1 {
		return Result{}, errors.New("joined device bootstrap was ambiguous")
	}
	return s.finish(ctx, options, client, bootstrap.Projects[0].ID, enrollment.DeviceID, enrollment.DeviceToken, enrollment.DashboardTicket, false)
}

func (s Service) finish(ctx context.Context, options Options, client API, projectID, deviceID, token, dashboardTicket string, createInvite bool) (Result, error) {
	if err := s.Creds.Put(ctx, deviceID, token); err != nil {
		_ = client.RevokeDevice(ctx, deviceID)
		return Result{}, fmt.Errorf("store device credential: %w", err)
	}
	stored := true
	rollback := func() {
		if stored {
			_ = s.Creds.Delete(context.WithoutCancel(ctx), deviceID)
		}
		_ = client.RevokeDevice(context.WithoutCancel(ctx), deviceID)
	}
	workspaceID, err := opaqueID("wsp_local_")
	if err != nil {
		rollback()
		return Result{}, err
	}
	workstreamID, err := opaqueID("wrk_local_")
	if err != nil {
		rollback()
		return Result{}, err
	}
	memberID, err := opaqueID("mem_local_")
	if err != nil {
		rollback()
		return Result{}, err
	}
	sessionID, err := opaqueID("ses_local_")
	if err != nil {
		rollback()
		return Result{}, err
	}
	workspace := config.Workspace{ID: workspaceID, ProjectID: projectID, WorkstreamID: workstreamID, MemberID: memberID, SessionID: sessionID, Root: options.RepositoryRoot}
	if dashboardTicket == "" {
		ticket, ticketErr := client.CreateDashboardTicket(ctx, projectID)
		if ticketErr != nil {
			rollback()
			return Result{}, fmt.Errorf("create dashboard ticket: %w", ticketErr)
		}
		dashboardTicket = ticket.Ticket
	}
	joinCode := ""
	if createInvite {
		invite, inviteErr := client.CreateInvite(ctx, projectID, 600, 1)
		if inviteErr != nil {
			rollback()
			return Result{}, fmt.Errorf("create invite: %w", inviteErr)
		}
		joinCode = invite.ID + "." + invite.Secret
	}
	if err := s.Register(ctx, options.ConfigRoot, options.APIBaseURL, deviceID, workspace); err != nil {
		rollback()
		return Result{}, fmt.Errorf("register workspace: %w", err)
	}
	stored = false
	return Result{ProjectID: projectID, DeviceID: deviceID, WorkspaceID: workspaceID, WorkstreamID: workstreamID, JoinCode: joinCode, DashboardTicket: dashboardTicket}, nil
}

func (s Service) finishExisting(ctx context.Context, options Options, client API, projectID, deviceID string) (Result, error) {
	workspaceID, err := opaqueID("wsp_local_")
	if err != nil {
		return Result{}, err
	}
	workstreamID, err := opaqueID("wrk_local_")
	if err != nil {
		return Result{}, err
	}
	memberID, err := opaqueID("mem_local_")
	if err != nil {
		return Result{}, err
	}
	sessionID, err := opaqueID("ses_local_")
	if err != nil {
		return Result{}, err
	}
	ticket, err := client.CreateDashboardTicket(ctx, projectID)
	if err != nil {
		return Result{}, fmt.Errorf("create dashboard ticket: %w", err)
	}
	invite, err := client.CreateInvite(ctx, projectID, 600, 1)
	if err != nil {
		return Result{}, fmt.Errorf("create invite: %w", err)
	}
	workspace := config.Workspace{ID: workspaceID, ProjectID: projectID, WorkstreamID: workstreamID, MemberID: memberID, SessionID: sessionID, Root: options.RepositoryRoot}
	if err := s.Register(ctx, options.ConfigRoot, options.APIBaseURL, deviceID, workspace); err != nil {
		return Result{}, fmt.Errorf("register additional workspace: %w", err)
	}
	return Result{
		ProjectID: projectID, DeviceID: deviceID, WorkspaceID: workspaceID,
		WorkstreamID: workstreamID, JoinCode: invite.ID + "." + invite.Secret,
		DashboardTicket: ticket.Ticket,
	}, nil
}

func validateOptions(options Options, requireLabel bool) error {
	if options.ConfigRoot == "" || options.RepositoryRoot == "" || options.APIBaseURL == "" || options.DeviceLabel == "" {
		return errors.New("config root, repository root, API base URL, and device label are required")
	}
	if requireLabel && options.ProjectLabel == "" {
		return errors.New("Project label is required")
	}
	return nil
}

func preflightRepository(ctx context.Context, root string) error {
	if _, err := gitadapter.CaptureBaseline(ctx, gitadapter.Runner{}, root); err != nil {
		return fmt.Errorf("capture repository baseline: %w", err)
	}
	if _, err := gitadapter.Fingerprint(ctx, gitadapter.Runner{}, root, "prj_preflight"); err != nil {
		return fmt.Errorf("validate repository identity: %w", err)
	}
	return nil
}

func containsProject(projects []hosted.Project, id string) bool {
	for _, project := range projects {
		if project.ID == id {
			return true
		}
	}
	return false
}

func secret(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate credential: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func opaqueID(prefix string) (string, error) {
	value, err := secret(16)
	if err != nil {
		return "", err
	}
	return prefix + value, nil
}
