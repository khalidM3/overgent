//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/stickguy/stickguy/internal/activation"
	"github.com/stickguy/stickguy/internal/app"
	"github.com/stickguy/stickguy/internal/claudesetup"
	"github.com/stickguy/stickguy/internal/codexsetup"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/credential"
	"github.com/stickguy/stickguy/internal/cursorsetup"
	"github.com/stickguy/stickguy/internal/daemon"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/stickguy/stickguy/internal/onboarding"
	servicemanager "github.com/stickguy/stickguy/internal/service"
	"github.com/stickguy/stickguy/internal/store"
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

type OnboardingState struct {
	Available       bool           `json:"available"`
	Development     bool           `json:"development"`
	Enrolled        bool           `json:"enrolled"`
	ProjectID       string         `json:"projectId"`
	RepositoryRoot  string         `json:"repositoryRoot"`
	RepositoryLabel string         `json:"repositoryLabel"`
	DeviceLabel     string         `json:"deviceLabel"`
	APIBaseURL      string         `json:"apiBaseUrl"`
	Adapters        []AdapterState `json:"adapters"`
	Limitation      string         `json:"limitation"`
	// Credential reports whether this Mac's stored credential is still accepted.
	// "revoked" and "unknown" both arrive as HTTP 401 but need different copy,
	// and "uncertain" (offline, timeout, server fault) must never be presented
	// as a reason to erase an enrollment.
	Credential string `json:"credential"`
}

type EnrollmentRequest struct {
	RepositoryRoot string `json:"repositoryRoot"`
	ProjectLabel   string `json:"projectLabel"`
	DeviceLabel    string `json:"deviceLabel"`
	DisplayName    string `json:"displayName"`
	JoinCode       string `json:"joinCode"`
	EnableCodex    bool   `json:"enableCodex"`
	EnableClaude   bool   `json:"enableClaude"`
	EnableCursor   bool   `json:"enableCursor"`
}

type EnrollmentResult struct {
	ProjectID string   `json:"projectId"`
	JoinCode  string   `json:"joinCode"`
	Warnings  []string `json:"warnings"`
}

type OnboardingService struct {
	configRoot, apiBaseURL, activationBaseURL, cliBinary string
	// Test seams for the local-only session opener. Production leaves these nil
	// and uses the OS URL handler / an argument-array child process.
	homeDirectory       string
	openSessionURL      func(string) error
	startSessionCommand func(string, []string, string) error

	credentialMu      sync.Mutex
	credentialStatus  hosted.CredentialStatus
	credentialSubject string
	credentialAt      time.Time
}

// credentialTTL keeps State() cheap. The webview polls it every two seconds
// while an adapter restart is pending, and a rejected credential does not
// un-reject itself in between.
const credentialTTL = 15 * time.Second

// credentialHealth answers whether the stored credential still authenticates,
// reusing a recent answer rather than calling out on every State(). The check
// itself lives in internal/onboarding so the desktop and the CLI cannot drift.
func (service *OnboardingService) credentialHealth(ctx context.Context, deviceID, apiBaseURL string) hosted.CredentialStatus {
	subject := deviceID + "@" + apiBaseURL
	service.credentialMu.Lock()
	if service.credentialSubject == subject && time.Since(service.credentialAt) < credentialTTL {
		cached := service.credentialStatus
		service.credentialMu.Unlock()
		return cached
	}
	service.credentialMu.Unlock()

	status, _, err := onboarding.New(apiBaseURL).CredentialState(ctx, service.configRoot)
	if err != nil {
		status = hosted.CredentialUncertain
	}

	service.credentialMu.Lock()
	service.credentialStatus, service.credentialSubject, service.credentialAt = status, subject, time.Now()
	service.credentialMu.Unlock()
	return status
}

// forgetCredentialHealth drops the cached answer so the next State() re-checks.
func (service *OnboardingService) forgetCredentialHealth() {
	service.credentialMu.Lock()
	service.credentialSubject, service.credentialAt = "", time.Time{}
	service.credentialMu.Unlock()
}

func newOnboardingService() *OnboardingService {
	root := desktopConfigRoot()
	return &OnboardingService{configRoot: root, apiBaseURL: desktopAPIBaseURL(), activationBaseURL: desktopActivationBaseURL(), cliBinary: desktopCLIBinary()}
}

func (service *OnboardingService) ChooseRepository() (string, error) {
	selected, err := application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Choose the Git repository Stickguy should coordinate").
		PromptForSingleSelection()
	if err != nil || selected == "" {
		return selected, err
	}
	return canonicalRepository(selected)
}

func (service *OnboardingService) State() (OnboardingState, error) {
	state := OnboardingState{Available: true, Development: desktopDevelopment, APIBaseURL: service.apiBaseURL, DeviceLabel: defaultDeviceLabel(), Limitation: "Start new Codex or Claude Code sessions in this repository after connecting an adapter. Existing sessions must restart once so the agent can load the Project hooks."}
	if service.configRoot == "" {
		return state, errors.New("local Stickguy configuration is unavailable")
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return state, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return state, err
	}
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
	state.Adapters = service.adapterStates(roots)
	// Report credential health with the rest of the state so a locked-out Mac
	// shows a recovery path on open, instead of only after an action fails.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	state.Credential = string(service.credentialHealth(ctx, cfg.DeviceID, cfg.APIBaseURL))
	return state, nil
}

// ResetEnrollment forgets this Mac's device identity so the member can enroll
// again from the app, without a terminal. The safety gate - refusing unless the
// hosted API actually rejected the credential - lives in internal/onboarding
// and is shared with "stickguy reset".
func (service *OnboardingService) ResetEnrollment() (OnboardingState, error) {
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return OnboardingState{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return OnboardingState{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := onboarding.New(cfg.APIBaseURL).Reset(ctx, service.configRoot, false); err != nil {
		return OnboardingState{}, err
	}
	service.forgetCredentialHealth()
	return service.State()
}

func (service *OnboardingService) CreateProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.enroll(request, true)
}

// CreateAdditionalProject reuses this Mac's enrolled device credential and
// hot-registers the repository with the one running service. The webview sees
// only the resulting Project ID and one-use invite, never the credential.
func (service *OnboardingService) CreateAdditionalProject(request EnrollmentRequest) (EnrollmentResult, error) {
	root, err := canonicalRepository(request.RepositoryRoot)
	if err != nil {
		return EnrollmentResult{}, err
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return EnrollmentResult{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return EnrollmentResult{}, err
	}
	if cfg.DeviceID == "" || cfg.APIBaseURL == "" || len(cfg.Workspaces) == 0 {
		return EnrollmentResult{}, errors.New("add a Project after this Mac has completed enrollment")
	}
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			return EnrollmentResult{}, errors.New("this repository is already connected to a Project")
		}
	}
	request.DeviceLabel = boundedLabel(request.DeviceLabel, defaultDeviceLabel())
	request.ProjectLabel = boundedLabel(request.ProjectLabel, filepath.Base(root))
	request.DisplayName, err = boundedDisplayName(request.DisplayName)
	if err != nil {
		return EnrollmentResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	token, err := credential.Get(ctx, cfg.DeviceID)
	if err != nil {
		return EnrollmentResult{}, fmt.Errorf("read existing device credential: %w", err)
	}
	flow := onboarding.New(cfg.APIBaseURL)
	flow.Register = func(registerContext context.Context, configRoot, apiBaseURL, deviceID string, workspace config.Workspace) error {
		response, callErr := daemon.Call(registerContext, paths.Socket, daemon.Request{
			Method: "add_project_workspace", WorkspaceID: workspace.ID, ProjectID: workspace.ProjectID,
			WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID,
			SessionID: workspace.SessionID, Root: workspace.Root,
		})
		if callErr == nil {
			if !response.OK {
				return errors.New(response.Error)
			}
			return nil
		}
		return app.Register(registerContext, configRoot, apiBaseURL, deviceID, workspace)
	}
	result, err := flow.CreateAdditional(ctx, onboarding.Options{
		ConfigRoot: service.configRoot, RepositoryRoot: root, APIBaseURL: cfg.APIBaseURL,
		ProjectLabel: request.ProjectLabel, DeviceLabel: request.DeviceLabel,
		DisplayName: request.DisplayName, AppVersion: "stickguy/desktop-beta",
	}, cfg.DeviceID, token)
	if err != nil {
		return EnrollmentResult{}, err
	}
	warnings := append([]string{}, service.configureAdapters(root, request.EnableCodex, request.EnableClaude, request.EnableCursor)...)
	if serviceErr := service.ensureService(ctx); serviceErr != nil {
		warnings = append(warnings, "Background service: "+serviceErr.Error())
	}
	return EnrollmentResult{ProjectID: result.ProjectID, JoinCode: result.JoinCode, Warnings: warnings}, nil
}

func (service *OnboardingService) JoinProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.enroll(request, false)
}

func (service *OnboardingService) enroll(request EnrollmentRequest, create bool) (EnrollmentResult, error) {
	root, err := canonicalRepository(request.RepositoryRoot)
	if err != nil {
		return EnrollmentResult{}, err
	}
	request.RepositoryRoot = root
	request.DeviceLabel = boundedLabel(request.DeviceLabel, defaultDeviceLabel())
	// An empty display name is passed through as empty so the member is asked to
	// choose one rather than silently inheriting this machine's hostname.
	displayName, err := boundedDisplayName(request.DisplayName)
	if err != nil {
		return EnrollmentResult{}, err
	}
	request.DisplayName = displayName
	request.ProjectLabel = boundedLabel(request.ProjectLabel, filepath.Base(root))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	options := onboarding.Options{ConfigRoot: service.configRoot, RepositoryRoot: root, APIBaseURL: service.apiBaseURL, ProjectLabel: request.ProjectLabel, DeviceLabel: request.DeviceLabel, DisplayName: request.DisplayName, AppVersion: "stickguy/desktop-beta"}
	flow := onboarding.New(service.apiBaseURL)
	var result onboarding.Result
	if create {
		result, err = flow.Create(ctx, options)
	} else {
		result, err = flow.Join(ctx, options, strings.TrimSpace(request.JoinCode))
	}
	if err != nil {
		return EnrollmentResult{}, err
	}
	warnings := append([]string{}, service.configureAdapters(root, request.EnableCodex, request.EnableClaude, request.EnableCursor)...)
	if serviceErr := service.ensureService(ctx); serviceErr != nil {
		warnings = append(warnings, "Background service: "+serviceErr.Error())
	}
	return EnrollmentResult{ProjectID: result.ProjectID, JoinCode: result.JoinCode, Warnings: warnings}, nil
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
	found := false
	for _, workspace := range cfg.Workspaces {
		if workspace.ProjectID == projectID {
			found = true
			break
		}
	}
	if !found || cfg.DeviceID == "" {
		return "", errors.New("Project is not enrolled on this device")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	token, err := credential.Get(ctx, cfg.DeviceID)
	if err != nil {
		return "", err
	}
	client, err := hosted.New(cfg.APIBaseURL, token)
	if err != nil {
		return "", err
	}
	ticket, err := client.CreateDashboardTicket(ctx, projectID)
	if err != nil {
		return "", err
	}
	handoff, err := activation.Start(service.activationBaseURL, ticket.Ticket)
	if err != nil {
		return "", err
	}
	go func() {
		waitContext, stop := context.WithTimeout(context.Background(), 35*time.Second)
		defer stop()
		_ = handoff.Wait(waitContext)
	}()
	return handoff.URL(), nil
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
		states[index].RuntimeVerified = service.agentRuntimeVerified(roots, vendor)
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
		state.Detail = "Connected to a different Stickguy profile. Reconnect explicitly to move only Stickguy-managed configuration to this Project."
	case "partial":
		state.Detail = "Stickguy found an incomplete connection and can repair the managed entries automatically."
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
		return "", errors.New("Stickguy development CLI is missing; restart pnpm dev so it can be rebuilt")
	}
	value, err := exec.LookPath("stickguy")
	if err != nil {
		return "", errors.New("Stickguy CLI is not installed; agent setup is unavailable")
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
