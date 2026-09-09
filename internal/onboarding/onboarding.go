package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/khalidM3/overgent/internal/app"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	gitadapter "github.com/khalidM3/overgent/internal/git"
	"github.com/khalidM3/overgent/internal/hosted"
)

type API interface {
	CreateProject(context.Context, hosted.NewProject) (hosted.Project, error)
	CreateInvite(context.Context, string, int, int) (hosted.Invite, error)
	Enroll(context.Context, string, string, string, string, string) (hosted.Enrollment, error)
	JoinProject(context.Context, string, string, string, string, string) (hosted.Membership, error)
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
	ConfigRoot, RepositoryRoot, ProjectLabel, DeviceLabel, AppVersion string
	// DisplayName is optional; empty means the member has not chosen one yet.
	DisplayName string
	// SkipInvite creates the Project without minting a one-use invite. A local
	// Project has no second member to hand one to, and offering a code is how a
	// screen implies that inviting somebody is the next step (ADR-072).
	SkipInvite bool
	// ProjectID reuses an identifier the caller already holds rather than being
	// issued a new one. Empty - which is every path today - means the server
	// issues one, and that is what an ordinary creation wants. It is set when a
	// Project is being re-created on a second backend and has to stay the same
	// Project: its identifier is salted into every repository fingerprint and
	// stamped on every event envelope the device has queued, so changing it
	// would orphan all of that evidence from the Project it describes.
	ProjectID string
}

type Result struct {
	ProjectID, DeviceID, WorkspaceID, WorkstreamID string
	JoinCode, DashboardTicket                      string
}

// Service is one onboarding flow against one backend. The backend travels
// with the flow rather than with the profile: a Mac holds a device identity
// per backend (ADR-069, ADR-074), so "which server" and "which credential"
// are one question and are answered together.
type Service struct {
	Backend  config.Backend
	Client   func(token string) (API, error)
	Creds    CredentialStore
	Register func(context.Context, string, string, string, config.Workspace) error
	// Rebind moves an existing Project onto this flow's backend. Like Register
	// it is a seam: the desktop points it at the running service so a move takes
	// effect without a restart, and it falls back to editing the profile.
	Rebind func(ctx context.Context, configRoot, projectID, apiBaseURL, deviceID string) error
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

func New(backend config.Backend) Service {
	return Service{
		Backend:  backend,
		Client:   func(token string) (API, error) { return hosted.New(backend.APIBaseURL, token) },
		Creds:    keychainStore{},
		Register: app.Register,
		Rebind: func(ctx context.Context, configRoot, projectID, apiBaseURL, deviceID string) error {
			_, err := app.RebindProject(ctx, configRoot, projectID, apiBaseURL, deviceID)
			return err
		},
	}
}

// CreateOnNewBackend creates a Project on this flow's backend, whether or not
// the profile has used that backend before.
//
// A backend the profile has never seen gets a device identity minted for it;
// one it already has an identity for reuses that identity, because a second
// credential for the same server would strand the Projects the first one
// holds. Both cases are one member action - "add a Project on this server" -
// so the choice is made here rather than in each caller.
func (s Service) CreateOnNewBackend(ctx context.Context, options Options) (Result, error) {
	if s.Backend.DeviceID == "" {
		return s.Create(ctx, options)
	}
	token, err := s.deviceToken(ctx)
	if err != nil {
		return Result{}, err
	}
	return s.CreateAdditional(ctx, options, s.Backend.DeviceID, token)
}

// JoinOnNewBackend redeems an invite on this flow's backend, minting a device
// identity for a backend the profile has never used and reusing the one it has
// otherwise. Joining a friend's team Project from a purely local profile is
// the first case, and it is the common one.
func (s Service) JoinOnNewBackend(ctx context.Context, options Options, joinCode string) (Result, error) {
	if s.Backend.DeviceID == "" {
		return s.Join(ctx, options, joinCode)
	}
	token, err := s.deviceToken(ctx)
	if err != nil {
		return Result{}, err
	}
	return s.JoinAdditional(ctx, options, s.Backend.DeviceID, token, joinCode)
}

func (s Service) deviceToken(ctx context.Context) (string, error) {
	if s.Creds == nil {
		return "", errors.New("credential store is unavailable")
	}
	token, err := s.Creds.Get(ctx, s.Backend.DeviceID)
	if err != nil {
		return "", fmt.Errorf("read this backend's device credential: %w", err)
	}
	return token, nil
}

func (s Service) Create(ctx context.Context, options Options) (Result, error) {
	if err := s.validateOptions(options, true); err != nil {
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
		appVersion = "overgent/dev"
	}
	project, err := client.CreateProject(ctx, hosted.NewProject{Label: options.ProjectLabel, DeviceLabel: options.DeviceLabel, DisplayName: options.DisplayName, AppVersion: appVersion, ID: options.ProjectID})
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
	return s.finish(ctx, options, client, project.ID, bootstrap.DeviceID, token, "", !options.SkipInvite)
}

// CreateAdditional creates another Project for a device that is already
// enrolled in the local profile. The existing credential is deliberately
// reused: one per-user service has one device identity across its Projects.
// Unlike first enrollment, a local registration failure must never revoke the
// shared device and strand its existing Projects.
func (s Service) CreateAdditional(ctx context.Context, options Options, deviceID, token string) (Result, error) {
	if err := s.validateOptions(options, true); err != nil {
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
		appVersion = "overgent/dev"
	}
	project, err := client.CreateProject(ctx, hosted.NewProject{Label: options.ProjectLabel, DeviceLabel: options.DeviceLabel, DisplayName: options.DisplayName, AppVersion: appVersion, ID: options.ProjectID})
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

// JoinAdditional redeems an invite for a device that is already enrolled in the
// local profile, adding a second Project without a second device identity.
//
// It is the join counterpart of CreateAdditional and shares its rule: the
// existing credential is reused, and a local registration failure must never
// revoke the shared device and strand the Projects this Mac already has. That
// is why it does not route through Join, whose rollback revokes the device it
// just created - here that device is the one holding every other Project.
func (s Service) JoinAdditional(ctx context.Context, options Options, deviceID, token, joinCode string) (Result, error) {
	if err := s.validateOptions(options, false); err != nil {
		return Result{}, err
	}
	if deviceID == "" || token == "" {
		return Result{}, errors.New("existing device ID and credential are required")
	}
	code, _, err := ParseInviteCode(joinCode)
	if err != nil {
		return Result{}, err
	}
	if err = preflightRepository(ctx, options.RepositoryRoot); err != nil {
		return Result{}, err
	}
	inviteID, inviteSecret, _ := strings.Cut(code, ".")
	client, err := s.Client(token)
	if err != nil {
		return Result{}, err
	}
	appVersion := options.AppVersion
	if appVersion == "" {
		appVersion = "overgent/dev"
	}
	membership, err := client.JoinProject(ctx, inviteID, inviteSecret, options.DeviceLabel, options.DisplayName, appVersion)
	if err != nil {
		return Result{}, fmt.Errorf("join Project: %w", err)
	}
	bootstrap, err := client.Bootstrap(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("bootstrap existing device: %w", err)
	}
	if bootstrap.DeviceID != deviceID || !containsProject(bootstrap.Projects, membership.ProjectID) {
		return Result{}, errors.New("existing device bootstrap did not contain the joined Project")
	}
	workspace, err := newWorkspace(membership.ProjectID, options.RepositoryRoot)
	if err != nil {
		return Result{}, err
	}
	if err = s.Register(ctx, options.ConfigRoot, s.Backend.APIBaseURL, deviceID, workspace); err != nil {
		return Result{}, fmt.Errorf("register joined workspace: %w", err)
	}
	// No invite is minted here. A member who just accepted one has not been
	// given anything to hand on, and offering a code is how the previous screen
	// implied that inviting somebody was the next step.
	return Result{
		ProjectID: membership.ProjectID, DeviceID: deviceID, WorkspaceID: workspace.ID,
		WorkstreamID: workspace.WorkstreamID, DashboardTicket: membership.DashboardTicket,
	}, nil
}

// newWorkspace mints the local identifiers one repository registration needs.
// The four are always created together; creating them inline three times is how
// finishExisting and finish drifted apart.
func newWorkspace(projectID, root string) (config.Workspace, error) {
	workspaceID, err := opaqueID("wsp_local_")
	if err != nil {
		return config.Workspace{}, err
	}
	workstreamID, err := opaqueID("wrk_local_")
	if err != nil {
		return config.Workspace{}, err
	}
	memberID, err := opaqueID("mem_local_")
	if err != nil {
		return config.Workspace{}, err
	}
	sessionID, err := opaqueID("ses_local_")
	if err != nil {
		return config.Workspace{}, err
	}
	return config.Workspace{ID: workspaceID, ProjectID: projectID, WorkstreamID: workstreamID, MemberID: memberID, SessionID: sessionID, Root: root}, nil
}

// An invite must survive until the recipient actually sits down: ten minutes
// forced a synchronous exchange ("are you at your computer right now?"), which
// is the failure mode of a code, not a link. Seven days matches GitHub's org
// invites; the invite stays one-use and revocable from the members screen.
const inviteLifetimeSeconds = 7 * 24 * 3600

var inviteCodePattern = regexp.MustCompile(`^inv_[A-Za-z0-9]+\.[A-Za-z0-9_-]+$`)

// ParseInviteCode extracts the invite code, and the backend origin it names,
// from whatever form the member pasted.
//
// Three forms are accepted: the bare code, the desktop deep link, and the
// join-page URL whose fragment carries the code (a fragment never reaches
// server logs). Only the https form knows where the Project lives, so only it
// returns an origin; the other two return an empty one, meaning "the backend
// the caller selected". This is what lets a member on a purely local profile
// paste a link and join a team Project on a server this Mac has never used,
// without being asked which server that was.
func ParseInviteCode(raw string) (code, origin string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > 2048 {
		return "", "", errors.New("invite code is empty or too large")
	}
	candidate := trimmed
	if strings.Contains(trimmed, "://") {
		parsed, parseErr := url.Parse(trimmed)
		if parseErr != nil {
			return "", "", errors.New("invite link could not be parsed")
		}
		switch {
		case strings.EqualFold(parsed.Scheme, "https"):
			if strings.Trim(parsed.Path, "/") != "join" || parsed.Fragment == "" {
				return "", "", errors.New("invite link must be a /join URL carrying the code after #")
			}
			// Userinfo in a link is how a host is made to read as something
			// else, and no legitimate invite carries it.
			if parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" {
				return "", "", errors.New("invite link must name a plain https host with no credentials or query")
			}
			candidate, origin = parsed.Fragment, "https://"+parsed.Host
		case strings.EqualFold(parsed.Scheme, "overgent"):
			// A scheme URL puts the first segment in Host ("overgent://join/…").
			if !strings.EqualFold(parsed.Host, "join") {
				return "", "", errors.New("invite deep link must use overgent://join/")
			}
			candidate = strings.Trim(parsed.Path, "/")
		default:
			return "", "", errors.New("invite link must be https or overgent scheme")
		}
	}
	if !inviteCodePattern.MatchString(candidate) {
		return "", "", errors.New("join code must have the form invite.secret")
	}
	return candidate, origin, nil
}

func (s Service) Join(ctx context.Context, options Options, joinCode string) (Result, error) {
	if err := s.validateOptions(options, false); err != nil {
		return Result{}, err
	}
	if err := preflightRepository(ctx, options.RepositoryRoot); err != nil {
		return Result{}, err
	}
	code, _, err := ParseInviteCode(joinCode)
	if err != nil {
		return Result{}, err
	}
	inviteID, inviteSecret, _ := strings.Cut(code, ".")
	publicClient, err := s.Client("")
	if err != nil {
		return Result{}, err
	}
	appVersion := options.AppVersion
	if appVersion == "" {
		appVersion = "overgent/dev"
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

// pendingPromotionAccount is where the credential for a promotion in flight is
// held before the device it belongs to has been named.
//
// A device credential is stored under its device id, and that id only comes
// back from the server after the Project has been created with the credential.
// That leaves a window - create succeeded, crash, nothing stored - in which the
// Project exists on the new backend under an identifier that can never be
// reused (the server refuses a duplicate) and can never be reached (the only
// credential that owned it is gone). The Project would be stranded permanently.
// Writing the credential first, under a name derived from the Project, closes
// it: a retry finds the credential, authenticates as the same device, and
// continues.
func pendingPromotionAccount(projectID string) string { return "pending-promotion:" + projectID }

// Promote moves a Project that lives on one backend onto this flow's backend,
// keeping its identifier, its repositories, and its agent bindings.
//
// This is how a local Project becomes a shared one. Nothing about the
// repository changes and nothing is re-enrolled: the Project is re-created on
// the new backend under the identifier it already has, this device takes an
// identity there, and the profile is repointed. The coordination record does
// not travel - the new backend learns the current state from the next scan -
// which is what keeps this a change of address rather than a migration.
//
// It is written to be safe to run twice. Every step short of the rebind is
// either idempotent or recoverable, because the failure that matters here is
// the one that happens after the new Project exists.
func (s Service) Promote(ctx context.Context, options Options, projectID string) (Result, error) {
	if options.ConfigRoot == "" || projectID == "" {
		return Result{}, errors.New("a config root and the Project to move are required")
	}
	if s.Rebind == nil {
		return Result{}, errors.New("this flow cannot move a Project")
	}
	if s.Backend.DeviceID != "" {
		// This profile already has an identity on the destination. Reusing it is
		// correct - one device identity per backend - but creating a Project
		// with it is a different call, and the promotion of a Project onto a
		// backend this Mac already uses is not a path any caller has yet.
		return Result{}, errors.New("this device already has an identity on that server; moving a Project onto it is not supported yet")
	}
	appVersion := options.AppVersion
	if appVersion == "" {
		appVersion = "overgent/dev"
	}
	pending := pendingPromotionAccount(projectID)
	token, err := s.Creds.Get(ctx, pending)
	if err != nil || token == "" {
		if token, err = secret(32); err != nil {
			return Result{}, err
		}
		// Stored before the Project is created, never after: see the note on
		// pendingPromotionAccount.
		if err = s.Creds.Put(ctx, pending, token); err != nil {
			return Result{}, fmt.Errorf("store the credential for the move: %w", err)
		}
	}
	client, err := s.Client(token)
	if err != nil {
		return Result{}, err
	}
	if _, err = client.CreateProject(ctx, hosted.NewProject{
		Label: options.ProjectLabel, DeviceLabel: options.DeviceLabel,
		DisplayName: options.DisplayName, AppVersion: appVersion, ID: projectID,
	}); err != nil && !errors.Is(err, hosted.ErrProjectIDUnavailable) {
		return Result{}, fmt.Errorf("re-create the Project on the new server: %w", err)
	}
	// Either the Project was just created, or the identifier was already taken.
	// Bootstrap decides which: if this credential owns it, the taken identifier
	// is this move's own earlier attempt and the work carries on. If it does
	// not, the identifier belongs to somebody else on that server and the move
	// must stop before anything local is changed.
	bootstrap, err := client.Bootstrap(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("confirm the moved Project: %w", err)
	}
	if bootstrap.DeviceID == "" || !containsProject(bootstrap.Projects, projectID) {
		return Result{}, errors.New("that Project identifier is already in use on the destination server")
	}
	if err = s.Creds.Put(ctx, bootstrap.DeviceID, token); err != nil {
		return Result{}, fmt.Errorf("store device credential: %w", err)
	}
	if err = s.Rebind(ctx, options.ConfigRoot, projectID, s.Backend.APIBaseURL, bootstrap.DeviceID); err != nil {
		// The credential stays under both names. The Project on the destination
		// is real and this device owns it, so the move is resumable; deleting
		// the credential here is what would strand it.
		return Result{}, fmt.Errorf("point this device at the moved Project: %w", err)
	}
	// Only now, with the profile pointing at the new backend, is the in-flight
	// credential no longer the only way back to the Project.
	_ = s.Creds.Delete(ctx, pending)
	joinCode := ""
	if !options.SkipInvite {
		if invite, inviteErr := client.CreateInvite(ctx, projectID, inviteLifetimeSeconds, 1); inviteErr == nil {
			joinCode = invite.ID + "." + invite.Secret
		}
	}
	ticket, err := client.CreateDashboardTicket(ctx, projectID)
	if err != nil {
		// The move is done; only the browser hand-off is missing, and reopening
		// the Project mints another. Reporting failure here would say the
		// promotion failed when it did not.
		return Result{ProjectID: projectID, DeviceID: bootstrap.DeviceID, JoinCode: joinCode}, nil
	}
	return Result{ProjectID: projectID, DeviceID: bootstrap.DeviceID, JoinCode: joinCode, DashboardTicket: ticket.Ticket}, nil
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
		invite, inviteErr := client.CreateInvite(ctx, projectID, inviteLifetimeSeconds, 1)
		if inviteErr != nil {
			rollback()
			return Result{}, fmt.Errorf("create invite: %w", inviteErr)
		}
		joinCode = invite.ID + "." + invite.Secret
	}
	if err := s.Register(ctx, options.ConfigRoot, s.Backend.APIBaseURL, deviceID, workspace); err != nil {
		rollback()
		return Result{}, fmt.Errorf("register workspace: %w", err)
	}
	stored = false
	return Result{ProjectID: projectID, DeviceID: deviceID, WorkspaceID: workspaceID, WorkstreamID: workstreamID, JoinCode: joinCode, DashboardTicket: dashboardTicket}, nil
}

func (s Service) finishExisting(ctx context.Context, options Options, client API, projectID, deviceID string) (Result, error) {
	workspace, err := newWorkspace(projectID, options.RepositoryRoot)
	if err != nil {
		return Result{}, err
	}
	ticket, err := client.CreateDashboardTicket(ctx, projectID)
	if err != nil {
		return Result{}, fmt.Errorf("create dashboard ticket: %w", err)
	}
	// A local Project has no second member to hand a code to, and offering one
	// is how a screen implies that inviting somebody is the next step
	// (ADR-072). The same rule that governs a first local Project governs the
	// second one.
	joinCode := ""
	if !options.SkipInvite {
		invite, inviteErr := client.CreateInvite(ctx, projectID, inviteLifetimeSeconds, 1)
		if inviteErr != nil {
			return Result{}, fmt.Errorf("create invite: %w", inviteErr)
		}
		joinCode = invite.ID + "." + invite.Secret
	}
	if err := s.Register(ctx, options.ConfigRoot, s.Backend.APIBaseURL, deviceID, workspace); err != nil {
		return Result{}, fmt.Errorf("register additional workspace: %w", err)
	}
	return Result{
		ProjectID: projectID, DeviceID: deviceID, WorkspaceID: workspace.ID,
		WorkstreamID: workspace.WorkstreamID, JoinCode: joinCode,
		DashboardTicket: ticket.Ticket,
	}, nil
}

func (s Service) validateOptions(options Options, requireLabel bool) error {
	if options.ConfigRoot == "" || options.RepositoryRoot == "" || s.Backend.APIBaseURL == "" || options.DeviceLabel == "" {
		return errors.New("config root, repository root, backend API base URL, and device label are required")
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
