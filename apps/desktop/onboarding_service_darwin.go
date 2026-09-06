//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/khalidM3/overgent/internal/adapterrepair"
	"github.com/khalidM3/overgent/internal/app"
	"github.com/khalidM3/overgent/internal/claudesetup"
	"github.com/khalidM3/overgent/internal/codexsetup"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/cursorsetup"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/hosted"
	"github.com/khalidM3/overgent/internal/localbackend"
	"github.com/khalidM3/overgent/internal/onboarding"
	servicemanager "github.com/khalidM3/overgent/internal/service"
	"github.com/khalidM3/overgent/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type AdapterState struct {
	Name             string `json:"name"`
	Installed        bool   `json:"installed"`
	Configured       bool   `json:"configured"`
	Fidelity         string `json:"fidelity"`
	Detail           string `json:"detail"`
	Binding          string `json:"binding"`
	PreviousProfile  string `json:"previousProfile,omitempty"`
	CurrentProfile   string `json:"currentProfile"`
	RuntimeVerified  bool   `json:"runtimeVerified"`
	RestartRequired  bool   `json:"restartRequired"`
	ReconnectAllowed bool   `json:"reconnectAllowed"`
	// HooksNeedReview reports a Codex binding whose files are installed but
	// whose hooks Codex will not run until the member reviews them. Without
	// this the adapter reads as connected while observing nothing (ADR-051).
	HooksNeedReview bool   `json:"hooksNeedReview"`
	ReviewGuidance  string `json:"reviewGuidance,omitempty"`
}

// ProjectState is one Project on this profile and the backend it lives on.
// The origin is shown because "where does this Project's coordination data
// live" is the one question a member cannot answer from the Project's name,
// and after ADR-074 two Projects side by side can answer it differently.
type ProjectState struct {
	ProjectID       string `json:"projectId"`
	RepositoryRoot  string `json:"repositoryRoot"`
	RepositoryLabel string `json:"repositoryLabel"`
	BackendID       string `json:"backendId"`
	Kind            string `json:"kind"`
	APIBaseURL      string `json:"apiBaseUrl"`
	Credential      string `json:"credential"`
}

type OnboardingState struct {
	Available       bool   `json:"available"`
	Development     bool   `json:"development"`
	Enrolled        bool   `json:"enrolled"`
	ProjectID       string `json:"projectId"`
	RepositoryRoot  string `json:"repositoryRoot"`
	RepositoryLabel string `json:"repositoryLabel"`
	DeviceLabel     string `json:"deviceLabel"`
	APIBaseURL      string `json:"apiBaseUrl"`
	// BackendID is the backend the selected Project lives on, so a recovery
	// action names one enrollment rather than the whole Mac.
	BackendID  string         `json:"backendId"`
	Projects   []ProjectState `json:"projects"`
	Adapters   []AdapterState `json:"adapters"`
	Limitation string         `json:"limitation"`
	// Credential reports whether this Mac's stored credential is still accepted.
	// "revoked" and "unknown" both arrive as HTTP 401 but need different copy,
	// and "uncertain" (offline, timeout, server fault) must never be presented
	// as a reason to erase an enrollment.
	Credential string `json:"credential"`
	// LocalAvailable reports that this build carries a backend, so "Use on this
	// Mac" is a real choice rather than a button that fails when pressed.
	LocalAvailable bool `json:"localAvailable"`
	// MemberName is the name this Mac has been told the member goes by, used to
	// seed a new Project rather than to override an existing one.
	MemberName string `json:"memberName,omitempty"`
	// Backend is the bundled backend's state, shown beside service health.
	Backend BackendStatus `json:"backend"`
}

type EnrollmentRequest struct {
	RepositoryRoot string `json:"repositoryRoot"`
	ProjectLabel   string `json:"projectLabel"`
	DeviceLabel    string `json:"deviceLabel"`
	DisplayName    string `json:"displayName"`
	JoinCode       string `json:"joinCode"`
	// ServerOrigin is the "Advanced: connect to a different server" field. Empty
	// means the build default. Validated with exactly hosted.New's rule so the
	// desktop and `overgent create --api` accept the same thing (Lane 05).
	ServerOrigin string `json:"serverOrigin"`
	EnableCodex  bool   `json:"enableCodex"`
	EnableClaude bool   `json:"enableClaude"`
	EnableCursor bool   `json:"enableCursor"`
}

type EnrollmentResult struct {
	ProjectID string   `json:"projectId"`
	JoinCode  string   `json:"joinCode"`
	Warnings  []string `json:"warnings"`
}

type OnboardingService struct {
	dashboardMu                       sync.Mutex
	dashboardConnections              map[string]*dashboardConnection
	configRoot, apiBaseURL, cliBinary string
	// localAvailable is whether this build carries a bundled backend.
	localAvailable bool
	// Test seams for the local-only session opener. Production leaves these nil
	// and uses the OS URL handler / an argument-array child process.
	homeDirectory       string
	openSessionURL      func(string) error
	startSessionCommand func(string, []string, string) error

	// credentials caches one answer per backend. A profile holds several after
	// ADR-074, and one revoked team Project says nothing about the local
	// Project beside it, so a single cached value would report the wrong one.
	credentialMu sync.Mutex
	credentials  map[string]cachedCredential
	// repairOnce keeps the launch-time adoption pass to one run per launch. It
	// touches files on disk, and State() is polled every two seconds while an
	// adapter restart is pending.
	repairOnce sync.Once
}

// credentialTTL keeps State() cheap. The webview polls it every two seconds
// while an adapter restart is pending, and a rejected credential does not
// un-reject itself in between.
const credentialTTL = 15 * time.Second

type cachedCredential struct {
	status  hosted.CredentialStatus
	subject string
	at      time.Time
}

// credentialHealth answers whether the credential stored for one backend still
// authenticates against it, reusing a recent answer rather than calling out on
// every State(). The check itself lives in internal/onboarding so the desktop
// and the CLI cannot drift.
func (service *OnboardingService) credentialHealth(ctx context.Context, backend config.Backend) hosted.CredentialStatus {
	subject := backend.DeviceID + "@" + backend.APIBaseURL
	service.credentialMu.Lock()
	if cached, known := service.credentials[backend.ID]; known && cached.subject == subject && time.Since(cached.at) < credentialTTL {
		service.credentialMu.Unlock()
		return cached.status
	}
	service.credentialMu.Unlock()

	status, _, err := onboarding.New(backend).CredentialState(ctx, service.configRoot)
	if err != nil {
		status = hosted.CredentialUncertain
	}

	service.credentialMu.Lock()
	if service.credentials == nil {
		service.credentials = map[string]cachedCredential{}
	}
	service.credentials[backend.ID] = cachedCredential{status: status, subject: subject, at: time.Now()}
	service.credentialMu.Unlock()
	return status
}

// forgetCredentialHealth drops the cached answers so the next State() re-checks.
func (service *OnboardingService) forgetCredentialHealth() {
	service.credentialMu.Lock()
	service.credentials = nil
	service.credentialMu.Unlock()
}

func newOnboardingService() *OnboardingService {
	root := desktopConfigRoot()
	// Recording the bundled artifact paths on every launch is what lets the
	// service find the backend again after an app update replaced the bundle.
	recordBundledBackend(root)
	return &OnboardingService{
		configRoot: root, apiBaseURL: desktopAPIBaseURL(), cliBinary: desktopCLIBinary(),
		localAvailable: localbackend.Configured(root),
	}
}

func (service *OnboardingService) ChooseRepository() (string, error) {
	selected, err := application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Choose the Git repository Overgent should coordinate").
		PromptForSingleSelection()
	if err != nil || selected == "" {
		return selected, err
	}
	return canonicalRepository(selected)
}

// RecheckState answers the "Check again" button.
//
// It is State with the cached credential answers dropped first. The cache
// exists so that every State() does not call out to every backend, but an
// explicit re-check is the one moment the member is saying they believe the
// cached answer is wrong - handing it back is the one thing the button must not
// do. Without this a transient "could not confirm this Mac's access", which is
// exactly what a backend that is still starting produces, survived every press
// until the 15-second window happened to lapse.
func (service *OnboardingService) RecheckState() (OnboardingState, error) {
	service.forgetCredentialHealth()
	return service.State()
}

func (service *OnboardingService) State() (OnboardingState, error) {
	state := OnboardingState{Available: true, Development: desktopDevelopment, APIBaseURL: service.apiBaseURL, DeviceLabel: defaultDeviceLabel(), Limitation: "Start new Codex or Claude Code sessions in this repository after connecting an adapter. Existing sessions must restart once so the agent can load the Project hooks."}
	if service.configRoot == "" {
		return state, errors.New("local Overgent configuration is unavailable")
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return state, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return state, err
	}
	state.LocalAvailable = service.localAvailable
	state.MemberName = service.rememberedDisplayName()
	state.Backend = service.backendStatus()
	if len(cfg.Workspaces) == 0 {
		state.Adapters = service.adapterStates(nil)
		return state, nil
	}
	state.Enrolled = true
	// The newest registration is the Project the member most recently added.
	// A specific Project can always be opened through OpenLiveProject.
	selected := cfg.Workspaces[len(cfg.Workspaces)-1]
	state.ProjectID = selected.ProjectID
	state.RepositoryRoot = selected.Root
	state.RepositoryLabel = filepath.Base(selected.Root)
	roots := make([]string, 0, len(cfg.Workspaces))
	for _, workspace := range cfg.Workspaces {
		roots = append(roots, workspace.Root)
	}
	// Adopt anything an earlier Overgent left behind before the rows are
	// computed, so a member is never shown "connected to a different Overgent
	// profile" for a profile that is their own. This is the whole reason a fresh
	// install on a fresh repository could report Codex as somebody else's: the
	// Codex hook file is user-level and outlives every reinstall.
	service.repairAdapters(roots)
	state.Adapters = service.adapterStates(roots)
	// Report credential health with the rest of the state so a locked-out Mac
	// shows a recovery path on open, instead of only after an action fails. It
	// is asked once per backend: a profile holds several, and a revoked team
	// Project must not present the local Project beside it as broken.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	health := map[string]string{}
	for _, backend := range cfg.Backends {
		health[backend.ID] = string(service.credentialHealth(ctx, backend))
	}
	seen := map[string]bool{}
	for index := len(cfg.Workspaces) - 1; index >= 0; index-- {
		workspace := cfg.Workspaces[index]
		if seen[workspace.ProjectID] {
			continue
		}
		seen[workspace.ProjectID] = true
		backend, _ := cfg.BackendForWorkspace(workspace)
		state.Projects = append(state.Projects, ProjectState{
			ProjectID: workspace.ProjectID, RepositoryRoot: workspace.Root,
			RepositoryLabel: filepath.Base(workspace.Root), BackendID: backend.ID,
			Kind: backend.Kind, APIBaseURL: backend.APIBaseURL, Credential: health[backend.ID],
		})
	}
	if backend, bound := cfg.BackendForWorkspace(selected); bound {
		state.APIBaseURL = backend.APIBaseURL
		state.BackendID = backend.ID
		state.Credential = health[backend.ID]
	}
	return state, nil
}

// repairAdapters adopts agent bindings an earlier Overgent left behind, once
// per launch.
//
// It is deliberately silent. A repair that succeeded is not news - the member
// never knew there was anything to fix - and a repair that could not run is
// already described by the adapter row it failed to change, in terms of what is
// actually wrong rather than in terms of this pass.
func (service *OnboardingService) repairAdapters(roots []string) {
	service.repairOnce.Do(func() {
		executable, err := service.resolveCLI()
		if err != nil {
			return
		}
		for _, outcome := range adapterrepair.Run(service.configRoot, executable, roots) {
			if outcome.Err != nil {
				slog.Warn("repair agent binding", "vendor", outcome.Vendor, "error", outcome.Err)
				continue
			}
			slog.Info("adopted an agent binding left by an earlier Overgent", "vendor", outcome.Vendor)
		}
	})
}

// ResetEnrollment forgets this Mac's device identity on one backend so the
// member can enroll against it again, without a terminal. It is scoped to a
// backend because a profile holds several (ADR-074): a revoked team Project
// must not take the local Project beside it with it. An empty backend id means
// every backend, which is the "forget this Mac entirely" form.
//
// The safety gate - refusing unless the backend actually rejected the
// credential - lives in internal/onboarding and is shared with `overgent reset`.
func (service *OnboardingService) ResetEnrollment(backendID string) (OnboardingState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if strings.TrimSpace(backendID) == "" {
		if _, err := onboarding.ResetAll(ctx, service.configRoot, false); err != nil {
			return OnboardingState{}, err
		}
		service.forgetCredentialHealth()
		return service.State()
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return OnboardingState{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return OnboardingState{}, err
	}
	backend, known := cfg.BackendByID(backendID)
	if !known {
		return OnboardingState{}, errors.New("this Mac has no such backend")
	}
	if _, err := onboarding.New(backend).Reset(ctx, service.configRoot, false); err != nil {
		return OnboardingState{}, err
	}
	service.forgetCredentialHealth()
	return service.State()
}

func (service *OnboardingService) CreateProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.addProject(request, false, false)
}

// CreateLocalProject sets up a Project that never leaves this Mac.
//
// It is the same enrollment as CreateProject with two differences, both of
// which follow from there being no second member and no remote: the API origin
// is the loopback backend the service just started, and no invite is minted,
// because a code offered on the success screen is how a member is told that
// inviting somebody is the next step.
//
// It is available whenever the build carries a backend, not only on a fresh
// profile: after ADR-074 a local Project sits beside a team Project rather
// than replacing it.
func (service *OnboardingService) CreateLocalProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.addProject(request, true, false)
}

// CreateAdditionalProject is the same call from the "Add a Project" screen.
// The distinction it used to carry - whether this Mac had enrolled yet - is
// now answered per backend inside the flow.
func (service *OnboardingService) CreateAdditionalProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.addProject(request, false, false)
}

func (service *OnboardingService) JoinProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.addProject(request, false, true)
}

func (service *OnboardingService) JoinAdditionalProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.addProject(request, false, true)
}

// addProject is the one enrollment path: choose the backend, then create or
// join on it.
//
// Creating, joining, adding a second Project and adding the first were four
// methods that differed only in which backend they targeted and whether this
// Mac already had a device identity there. Both of those are now questions
// about a backend rather than about the Mac, and both are answered in one
// place, so the four cannot drift apart again.
func (service *OnboardingService) addProject(request EnrollmentRequest, local, join bool) (EnrollmentResult, error) {
	if local && !service.localAvailable {
		return EnrollmentResult{}, errors.New("this build does not carry a backend to run on this Mac")
	}
	root, err := canonicalRepository(request.RepositoryRoot)
	if err != nil {
		return EnrollmentResult{}, err
	}
	request.RepositoryRoot = root
	request.DeviceLabel = boundedLabel(request.DeviceLabel, defaultDeviceLabel())
	// An empty display name is passed through as empty so the member is asked to
	// choose one rather than silently inheriting this machine's hostname.
	if request.DisplayName, err = boundedDisplayName(request.DisplayName); err != nil {
		return EnrollmentResult{}, err
	}
	// A local Project never asks for a name, because it has no collaborators to
	// show one to. It still has to attribute the member's own sessions, so
	// without this it fell through to the device label and a member appeared as
	// their hostname in every Project after the first.
	if request.DisplayName == "" {
		request.DisplayName = service.rememberedDisplayName()
	}
	request.ProjectLabel = boundedLabel(request.ProjectLabel, filepath.Base(root))
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return EnrollmentResult{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return EnrollmentResult{}, err
	}
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			return EnrollmentResult{}, errors.New("this repository is already connected to a Project")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	apiBaseURL, err := service.resolveOrigin(ctx, paths, request, local, join)
	if err != nil {
		return EnrollmentResult{}, err
	}
	options := onboarding.Options{
		ConfigRoot: service.configRoot, RepositoryRoot: root, ProjectLabel: request.ProjectLabel,
		DeviceLabel: request.DeviceLabel, DisplayName: request.DisplayName,
		AppVersion: "overgent/desktop-beta", SkipInvite: local,
	}
	// Registering through the running service rather than restarting it is what
	// keeps every live agent session it is observing alive.
	flow := onboarding.New(cfg.BackendTarget(apiBaseURL))
	flow.Register = service.hotRegister(paths)
	var result onboarding.Result
	if join {
		result, err = flow.JoinOnNewBackend(ctx, options, strings.TrimSpace(request.JoinCode))
	} else {
		result, err = flow.CreateOnNewBackend(ctx, options)
	}
	if err != nil {
		return EnrollmentResult{}, err
	}
	// This profile now holds a backend it did not a moment ago, and for a local
	// Project that backend was still starting while the enrollment ran. Any
	// credential answer cached before this point describes a server that was not
	// listening yet, and serving it back would report a Project the member just
	// successfully created as one this Mac cannot reach.
	service.forgetCredentialHealth()
	// Recorded only after an enrollment accepted it, and only when the member
	// supplied one: remembering a substituted device label would make the
	// fallback permanent rather than fix it.
	service.rememberDisplayName(request.DisplayName)
	warnings := append([]string{}, service.configureAdapters(root, request.EnableCodex, request.EnableClaude, request.EnableCursor)...)
	// A Project on this Mac starts from this Mac's defaults, so intelligence is
	// configured once rather than re-entered per Project. Only local: applying
	// them to a shared Project would upload the member's key to a server other
	// members' sessions spend it from, which is a decision they take on the
	// Project's own settings screen, in front of the disclosure that says so.
	if local {
		if applyErr := service.applyAIDefaults(ctx, result.ProjectID); applyErr != nil {
			warnings = append(warnings, "Intelligence defaults: "+applyErr.Error())
		}
	}
	if serviceErr := service.ensureService(ctx); serviceErr != nil {
		warnings = append(warnings, "Background service: "+serviceErr.Error())
	}
	return EnrollmentResult{ProjectID: result.ProjectID, JoinCode: result.JoinCode, Warnings: warnings}, nil
}

// resolveOrigin answers which backend this Project will live on.
//
// A local Project lives on the loopback backend the service starts here. An
// https invite link names its own origin, which is what lets a member on a
// purely local profile join a friend's team Project without being asked which
// server it is on. Otherwise it is the "connect to a different server" field,
// or this build's default.
func (service *OnboardingService) resolveOrigin(ctx context.Context, paths config.Paths, request EnrollmentRequest, local, join bool) (string, error) {
	if local {
		// The service owns the backend process, so it has to exist before the
		// backend does. On a fresh profile this is the call that installs it.
		if serviceErr := service.ensureService(ctx); serviceErr != nil {
			return "", fmt.Errorf("start the Overgent background service: %w", serviceErr)
		}
		endpoint, err := ensureLocalBackend(ctx, paths)
		if err != nil {
			return "", err
		}
		return endpoint.SiteOrigin, nil
	}
	if join {
		_, origin, err := onboarding.ParseInviteCode(strings.TrimSpace(request.JoinCode))
		if err != nil {
			return "", err
		}
		if origin != "" {
			return origin, nil
		}
	}
	if strings.TrimSpace(request.ServerOrigin) != "" {
		return onboarding.ValidateAPIOrigin(request.ServerOrigin)
	}
	return service.apiBaseURL, nil
}

// hotRegister registers a repository with the service already running on this
// Mac, falling back to writing the profile directly when no service answers.
// Restarting the service to pick up a new Project would drop every live agent
// session it is currently observing.
//
// The backend origin and device identity travel with the call because the
// Project may be the first one this profile has on that server.
func (service *OnboardingService) hotRegister(paths config.Paths) func(context.Context, string, string, string, config.Workspace) error {
	return func(registerContext context.Context, configRoot, apiBaseURL, deviceID string, workspace config.Workspace) error {
		response, callErr := daemon.Call(registerContext, paths.Socket, daemon.Request{
			Method: "add_project_workspace", WorkspaceID: workspace.ID, ProjectID: workspace.ProjectID,
			WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID,
			SessionID: workspace.SessionID, Root: workspace.Root,
			APIBaseURL: apiBaseURL, DeviceID: deviceID,
		})
		if callErr == nil {
			if !response.OK {
				return errors.New(response.Error)
			}
			return nil
		}
		return app.Register(registerContext, configRoot, apiBaseURL, deviceID, workspace)
	}
}

func (service *OnboardingService) ensureService(ctx context.Context) error {
	// The development harness (scripts/dev.mjs) runs the service in the
	// foreground against this same profile. Installing a LaunchAgent for it too
	// produces two services competing for one profile lock, and the agent can
	// never win. One service per profile means the harness owns it here.
	if desktopDevelopment {
		return nil
	}
	executable, err := service.resolveCLI()
	if err != nil {
		return err
	}
	account, err := user.Current()
	if err != nil {
		return fmt.Errorf("resolve current user: %w", err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid <= 0 || !filepath.IsAbs(account.HomeDir) {
		return errors.New("current user has invalid home or uid")
	}
	return (servicemanager.Manager{Executable: executable, ConfigRoot: service.configRoot, Home: account.HomeDir, UID: uid}).Install(ctx)
}

func (service *OnboardingService) ConfigureAdapters(repositoryRoot string, enableCodex, enableClaude, enableCursor bool) ([]AdapterState, error) {
	root, err := canonicalRepository(repositoryRoot)
	if err != nil {
		return nil, err
	}
	warnings := service.configureAdapters(root, enableCodex, enableClaude, enableCursor)
	states := service.adapterStates([]string{root})
	if len(warnings) > 0 {
		return states, errors.New(strings.Join(warnings, "; "))
	}
	return states, nil
}

func (service *OnboardingService) ReconnectAdapter(repositoryRoot, agent string) (AdapterState, error) {
	root, err := canonicalRepository(repositoryRoot)
	if err != nil {
		return AdapterState{}, err
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return AdapterState{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return AdapterState{}, err
	}
	registered := false
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			registered = true
			break
		}
	}
	if !registered {
		return AdapterState{}, errors.New("adapter reconnect requires a repository registered to this Project")
	}
	executable, err := service.resolveCLI()
	if err != nil {
		return AdapterState{}, err
	}
	switch agent {
	case "codex":
		if _, err = (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Rebind(); err != nil {
			return AdapterState{}, err
		}
		if err = service.clearAgentRuntimeVerification(cfg, root, "codex"); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[0], nil
	case "claude":
		if _, err = (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Rebind(); err != nil {
			return AdapterState{}, err
		}
		if err = service.clearAgentRuntimeVerification(cfg, root, "claude"); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[1], nil
	case "cursor":
		if _, err = (cursorsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Rebind(); err != nil {
			return AdapterState{}, err
		}
		if err = service.clearAgentRuntimeVerification(cfg, root, "cursor"); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[2], nil
	default:
		return AdapterState{}, errors.New("agent must be codex, claude, or cursor")
	}
}

func (service *OnboardingService) clearAgentRuntimeVerification(cfg config.Config, root, vendor string) error {
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return err
	}
	db, err := store.Open(paths.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			return db.ClearAgentObservation(ctx, workspace.ID, vendor)
		}
	}
	return nil
}

func (service *OnboardingService) ConnectAgentWorktree(repositoryRoot, agent string) (AdapterState, error) {
	root, err := canonicalRepository(repositoryRoot)
	if err != nil {
		return AdapterState{}, err
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return AdapterState{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return AdapterState{}, err
	}
	if len(cfg.Workspaces) == 0 {
		return AdapterState{}, errors.New("create or join a Project before assigning agent worktrees")
	}
	if root == cfg.Workspaces[0].Root {
		return AdapterState{}, errors.New("choose a distinct linked worktree; the enrolled checkout already has combined observation")
	}
	if err = requireLinkedWorktree(cfg.Workspaces[0].Root, root); err != nil {
		return AdapterState{}, err
	}
	agent = strings.ToLower(strings.TrimSpace(agent))
	if agent != "codex" && agent != "claude" && agent != "cursor" {
		return AdapterState{}, errors.New("agent must be codex, claude, or cursor")
	}
	executable, err := service.resolveCLI()
	if err != nil {
		return AdapterState{}, err
	}
	// A linked worktree attributes work to exactly one vendor, so any other
	// vendor already bound here blocks the assignment. This is a loop rather
	// than a pair of checks because a third vendor made the pairwise form wrong:
	// it would have let Cursor be assigned to a worktree Codex already owned.
	for _, other := range []string{"codex", "claude", "cursor"} {
		if other == agent {
			continue
		}
		if service.agentConfiguredAt(root, other, executable) {
			return AdapterState{}, fmt.Errorf("this worktree is already assigned to %s; choose a different linked worktree for %s attribution", vendorLabel(other), vendorLabel(agent))
		}
	}
	registered := false
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			registered = true
			break
		}
	}
	if !registered {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "--config-root", service.configRoot, "workspace", "add", "--development", "--root", root)
		output, commandErr := command.CombinedOutput()
		if commandErr != nil {
			message := strings.TrimSpace(string(output))
			if len(message) > 300 {
				message = message[:300]
			}
			if message == "" {
				message = commandErr.Error()
			}
			return AdapterState{}, fmt.Errorf("register linked worktree through the running service: %s", message)
		}
	}
	switch agent {
	case "codex":
		if _, err = (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[0], nil
	case "claude":
		if _, err = (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[1], nil
	case "cursor":
		if _, err = (cursorsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[2], nil
	}
	return AdapterState{}, errors.New("unreachable agent selection")
}

// agentConfiguredAt reports whether a vendor already owns a worktree. A status
// error is not a claim of ownership: a drifted or unreadable configuration must
// not silently block an assignment with a message about a different vendor.
func (service *OnboardingService) agentConfiguredAt(root, vendor, executable string) bool {
	switch vendor {
	case "codex":
		status, err := (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status()
		return err == nil && status.Configured
	case "claude":
		status, err := (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status()
		return err == nil && status.Configured
	case "cursor":
		status, err := (cursorsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status()
		return err == nil && status.Configured
	}
	return false
}

func vendorLabel(vendor string) string {
	switch vendor {
	case "codex":
		return "Codex"
	case "claude":
		return "Claude Code"
	case "cursor":
		return "Cursor"
	}
	return vendor
}

func (service *OnboardingService) OpenLiveProject(projectID string) (string, error) {
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return "", err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return "", err
	}
	if _, bound := cfg.BackendForProject(projectID); !bound {
		return "", errors.New("Project is not enrolled on this Mac")
	}
	return "/?live=1&project=" + projectID, nil
}

func (service *OnboardingService) configureAdapters(root string, codex, claude, cursor bool) []string {
	if !codex && !claude && !cursor {
		return nil
	}
	executable, err := service.resolveCLI()
	if err != nil {
		return []string{err.Error()}
	}
	var warnings []string
	if codex {
		if _, err = (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			warnings = append(warnings, "Codex setup: "+err.Error())
		}
	}
	if claude {
		if _, err = (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			warnings = append(warnings, "Claude setup: "+err.Error())
		}
	}
	if cursor {
		if _, err = (cursorsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			warnings = append(warnings, "Cursor setup: "+err.Error())
		}
	}
	return warnings
}

func (service *OnboardingService) adapterStates(roots []string) []AdapterState {
	executable, cliErr := service.resolveCLI()
	currentProfile := filepath.Base(service.configRoot)
	states := []AdapterState{
		{Name: "Codex", Binding: "not_configured", CurrentProfile: currentProfile, Fidelity: "Live sessions + bounded title intent + tools + safe paths", Detail: "Not connected to this Project yet."},
		{Name: "Claude Code", Binding: "not_configured", CurrentProfile: currentProfile, Fidelity: "Live sessions + bounded title intent + tools + safe paths", Detail: "Not connected to this Project yet."},
		// Cursor's fidelity line names observed reads because its beforeReadFile
		// hook states the file before it is read. That is stronger evidence than
		// either other vendor provides, and it is the difference that decides
		// whether a session can receive a stale-assumption correction at all.
		{Name: "Cursor", Binding: "not_configured", CurrentProfile: currentProfile, Fidelity: "Live sessions + bounded prompt intent + edits + observed reads", Detail: "Not connected to this Project yet."},
	}
	for index, command := range []string{"codex", "claude", "cursor"} {
		_, states[index].Installed = agentExecutable(command)
	}
	if len(roots) == 0 || cliErr != nil {
		return states
	}
	for _, root := range roots {
		if status, statusErr := (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status(); statusErr == nil {
			applyAdapterBinding(&states[0], status.Configured, status.Binding, status.PreviousProfile)
			if status.Hooks == "needs_review" {
				states[0].HooksNeedReview = true
				states[0].ReviewGuidance = status.Trust.Guidance
			}
		} else if !states[0].Configured {
			states[0].Binding = "drifted"
			states[0].Detail = "Configuration needs review: " + statusErr.Error()
		}
		if status, statusErr := (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status(); statusErr == nil {
			applyAdapterBinding(&states[1], status.Configured, status.Binding, status.PreviousProfile)
		} else if !states[1].Configured {
			states[1].Binding = "drifted"
			states[1].Detail = "Configuration needs review: " + statusErr.Error()
		}
		if status, statusErr := (cursorsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status(); statusErr == nil {
			applyAdapterBinding(&states[2], status.Configured, status.Binding, status.PreviousProfile)
		} else if !states[2].Configured {
			states[2].Binding = "drifted"
			states[2].Detail = "Configuration needs review: " + statusErr.Error()
		}
	}
	for index, vendor := range []string{"codex", "claude", "cursor"} {
		states[index].RuntimeVerified = states[index].Configured && service.agentRuntimeVerified(roots, vendor)
		states[index].RestartRequired = states[index].Configured && !states[index].RuntimeVerified
		states[index].ReconnectAllowed = states[index].Binding == "other_profile"
		if states[index].HooksNeedReview && !states[index].RuntimeVerified {
			// Codex parses these hooks and skips them. Saying "configured" here
			// would repeat the failure this state exists to expose.
			states[index].Detail = "Connected, but Codex has not trusted the activity hooks yet, so no session activity can be observed. " + codexsetup.ReviewGuidance
		} else if states[index].RuntimeVerified {
			states[index].Detail = "Verified by a live session event from this Project. Relevant briefs use MCP pull."
			if vendor == "cursor" {
				// Cursor takes a correction back through its own hook response
				// rather than waiting to be asked for one, so saying "MCP pull"
				// here would understate what this adapter does.
				states[index].Detail = "Verified by a live session event from this Project. Relevant briefs are pushed into the next turn."
			}
		} else if states[index].RestartRequired {
			states[index].Detail = "Configured for this Project. Restart the agent, then start a new task in this repository to verify the connection."
		}
	}
	return states
}

func applyAdapterBinding(state *AdapterState, configured bool, binding, previousProfile string) {
	state.Configured = state.Configured || configured
	if bindingPriority(binding) < bindingPriority(state.Binding) {
		return
	}
	state.Binding = binding
	if previousProfile != "" {
		state.PreviousProfile = filepath.Base(previousProfile)
	}
	switch binding {
	case "other_profile":
		state.Detail = "Connected to a different Overgent profile. Reconnect explicitly to move only Overgent-managed configuration to this Project."
	case "partial":
		state.Detail = "Overgent found an incomplete connection and can repair the managed entries automatically."
	case "not_configured":
		state.Detail = "Not connected to this Project yet."
	}
}

func bindingPriority(binding string) int {
	switch binding {
	case "drifted":
		return 5
	case "other_profile":
		return 4
	case "partial":
		return 3
	case "current":
		return 2
	default:
		return 1
	}
}

func (service *OnboardingService) agentRuntimeVerified(roots []string, vendor string) bool {
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return false
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return false
	}
	db, err := store.Open(paths.DB)
	if err != nil {
		return false
	}
	defer db.Close()
	allowed := map[string]bool{}
	for _, root := range roots {
		allowed[root] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, workspace := range cfg.Workspaces {
		if !allowed[workspace.Root] {
			continue
		}
		if _, observed, observationErr := db.AgentObserved(ctx, workspace.ID, vendor); observationErr == nil && observed {
			return true
		}
	}
	return false
}

func requireLinkedWorktree(primary, candidate string) error {
	primaryCommon, err := gitCommonDirectory(primary)
	if err != nil {
		return fmt.Errorf("inspect enrolled repository: %w", err)
	}
	candidateCommon, err := gitCommonDirectory(candidate)
	if err != nil {
		return fmt.Errorf("inspect selected repository: %w", err)
	}
	if primaryCommon != candidateCommon {
		return errors.New("selected directory must be an existing linked worktree of the enrolled Git repository")
	}
	return nil
}

func gitCommonDirectory(root string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", errors.New("directory is not a Git worktree")
	}
	value := strings.TrimSpace(string(output))
	if value == "" || len(value) > 4096 {
		return "", errors.New("Git common directory is invalid")
	}
	return filepath.EvalSymlinks(value)
}

func (service *OnboardingService) resolveCLI() (string, error) {
	if service.cliBinary != "" {
		absolute, err := filepath.Abs(service.cliBinary)
		if err == nil {
			if info, statErr := os.Stat(absolute); statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return absolute, nil
			}
		}
		return "", errors.New("Overgent development CLI is missing; restart pnpm dev so it can be rebuilt")
	}
	value, err := exec.LookPath("overgent")
	if err != nil {
		return "", errors.New("Overgent CLI is not installed; agent setup is unavailable")
	}
	return filepath.Abs(value)
}

func agentExecutable(command string) (string, bool) {
	if value, err := exec.LookPath(command); err == nil && executableFile(value) {
		return value, true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	var candidates []string
	switch command {
	case "codex":
		candidates = []string{
			filepath.Join(home, ".local", "bin", "codex"),
			filepath.Join(home, ".codex", "bin", "codex"),
			filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"),
			filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex"),
			"/Applications/Codex.app/Contents/Resources/codex",
			"/Applications/ChatGPT.app/Contents/Resources/codex",
		}
	case "claude":
		candidates = []string{
			filepath.Join(home, ".local", "bin", "claude"),
			filepath.Join(home, ".npm-global", "bin", "claude"),
		}
		nvmCandidates, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "claude"))
		candidates = append(candidates, nvmCandidates...)
	case "cursor":
		// Cursor ships an editor rather than a CLI-first tool; its `cursor`
		// shell command is installed from inside the app and is often absent
		// even when Cursor is. The app bundle is therefore checked too, so a
		// working Cursor is not reported as "not detected".
		candidates = []string{
			filepath.Join(home, "Applications", "Cursor.app", "Contents", "Resources", "app", "bin", "cursor"),
			"/Applications/Cursor.app/Contents/Resources/app/bin/cursor",
		}
	default:
		return "", false
	}
	for _, candidate := range candidates {
		if executableFile(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func canonicalRepository(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("choose a repository")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve repository: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize repository: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("repository is not a directory")
	}
	return resolved, nil
}

func defaultDeviceLabel() string {
	host, _ := os.Hostname()
	return boundedLabel(host, "This Mac")
}

// boundedDisplayName enforces the ADR-035 identity rules before any network
// call so the desktop reports a clear reason instead of a hosted error code.
func boundedDisplayName(value string) (string, error) {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "", nil
	}
	if utf8.RuneCountInString(value) < 2 || utf8.RuneCountInString(value) > 60 {
		return "", errors.New("choose a display name between 2 and 60 characters")
	}
	if strings.ContainsRune(value, '@') {
		return "", errors.New("choose a display name; an email address cannot be your Project identity")
	}
	return value, nil
}

func boundedLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	characters := []rune(value)
	if len(characters) > 80 {
		value = string(characters[:80])
	}
	return value
}

// SessionDetail exposes the caller's own local session content to the embedded
// dashboard. Content is read on demand from the vendor transcript and is never
// uploaded; sharing it with the Project stays a separate, explicit choice.
func (service *OnboardingService) SessionDetail(workstreamID string) (SessionDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return newDaemonService().SessionDetail(ctx, workstreamID)
}

// ProjectPaused reports whether this device is still sharing one Project's
// workspaces, and SetProjectPaused changes it.
//
// The workroom is scoped to a Project, so its pause control has to be too. The
// menu-bar switch stops sharing for every Project on this Mac, which is a
// different request and is why the two are not the same control.
func (service *OnboardingService) SetProjectPaused(projectID string, paused bool) error {
	if projectID == "" {
		return errors.New("project id required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return newDaemonService().SetProjectPaused(ctx, projectID, paused)
}

/*
SessionFocus reads and sets the quiet period on one of this device's own agent
sessions.

Focus is the inbound control and pause is the outbound one. Pausing hides this
device's work from the Project, which makes teammates less able to avoid it;
focus stops the Project reaching one agent's turns and leaves everything this
device publishes exactly as it was. The member who wants quiet therefore
carries the risk of missing a correction instead of transferring it to people
who never asked for it.

The state is local and never crosses the wire. It can only ever be read or set
for a session on this machine, and it always expires.
*/
func (service *OnboardingService) SessionFocus(workstreamID string) (SessionFocus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return newDaemonService().FocusState(ctx, workstreamID)
}

func (service *OnboardingService) SetSessionFocus(workstreamID string, minutes int) (SessionFocus, error) {
	if workstreamID == "" {
		return SessionFocus{}, errors.New("session id required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	local := newDaemonService()
	if minutes <= 0 {
		return local.Unfocus(ctx, workstreamID)
	}
	return local.Focus(ctx, workstreamID, minutes)
}
