//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/daemon"
)

type daemonService struct {
	paths config.Paths
}

func newDaemonService() daemonService {
	root := desktopConfigRoot()
	paths, err := config.Resolve(root)
	if err != nil {
		return daemonService{}
	}
	return daemonService{paths: paths}
}

func (service daemonService) Status(ctx context.Context) ServiceStatus {
	if service.paths.Socket == "" {
		return ServiceStatus{}
	}
	response, err := daemon.Call(ctx, service.paths.Socket, daemon.Request{Method: "health"})
	if err != nil || !response.OK {
		return ServiceStatus{}
	}
	data, ok := response.Data.(map[string]any)
	if !ok {
		return ServiceStatus{}
	}
	return ServiceStatus{
		Connected:        true,
		WorkspaceCount:   integer(data["workspaces"]),
		PausedWorkspaces: integer(data["pausedWorkspaces"]),
		FocusedSessions:  integer(data["focusedSessions"]),
		PendingEvents:    integer(data["pending"]),
		Backend:          backendHealth(data["backend"]),
	}
}

// backendHealth reads the backend block the service adds to health for a
// local-mode profile. A team-mode profile has no block and reports Present
// false, which is what keeps the menu line off that Mac entirely.
func backendHealth(value any) BackendHealth {
	encoded, err := json.Marshal(value)
	if err != nil || value == nil {
		return BackendHealth{}
	}
	var reported struct {
		Running   bool   `json:"running"`
		Port      int    `json:"port"`
		Version   string `json:"version"`
		LastError string `json:"lastError"`
		IdleSince string `json:"idleSince"`
	}
	if err = json.Unmarshal(encoded, &reported); err != nil {
		return BackendHealth{}
	}
	health := BackendHealth{Present: true, Running: reported.Running, Port: reported.Port, Version: reported.Version, LastError: reported.LastError}
	if since, parseErr := time.Parse(time.RFC3339, reported.IdleSince); parseErr == nil {
		health.Since = since
	}
	return health
}

func (service daemonService) PauseAll(ctx context.Context) error {
	return service.forEachWorkspace(ctx, "pause")
}

func (service daemonService) ResumeAll(ctx context.Context) error {
	return service.forEachWorkspace(ctx, "resume")
}

// SetProjectPaused stops or resumes sharing for one Project's workspaces on
// this device. The dashboard is scoped to a Project, so the control it offers
// has to be too; PauseAll remains the machine-wide switch behind the tray.
func (service daemonService) SetProjectPaused(ctx context.Context, projectID string, paused bool) error {
	method := "resume"
	if paused {
		method = "pause"
	}
	return service.call(ctx, daemon.Request{Method: method, ProjectID: projectID})
}

// ClearAllFocus lets every muted agent session hear coordination again.
func (service daemonService) ClearAllFocus(ctx context.Context) error {
	return service.call(ctx, daemon.Request{Method: "unfocus_all"})
}

// SessionFocus reads, sets, or clears the quiet period on one agent session.
// The state is local to this machine and never crosses the wire: muting is
// asymmetric by design, so a teammate sees no change and loses no visibility.
type SessionFocus struct {
	SessionID string `json:"sessionId"`
	Focused   bool   `json:"focused"`
	Until     string `json:"until,omitempty"`
}

func (service daemonService) Focus(ctx context.Context, workstreamID string, minutes int) (SessionFocus, error) {
	return service.focusCall(ctx, daemon.Request{Method: "focus", AgentWorkstreamID: workstreamID, FocusSeconds: int64(minutes) * 60})
}

func (service daemonService) Unfocus(ctx context.Context, workstreamID string) (SessionFocus, error) {
	return service.focusCall(ctx, daemon.Request{Method: "unfocus", AgentWorkstreamID: workstreamID})
}

func (service daemonService) FocusState(ctx context.Context, workstreamID string) (SessionFocus, error) {
	return service.focusCall(ctx, daemon.Request{Method: "focus_state", AgentWorkstreamID: workstreamID})
}

func (service daemonService) focusCall(ctx context.Context, request daemon.Request) (SessionFocus, error) {
	empty := SessionFocus{SessionID: request.AgentWorkstreamID}
	if service.paths.Socket == "" {
		return empty, nil
	}
	response, err := daemon.Call(ctx, service.paths.Socket, request)
	if err != nil {
		return empty, err
	}
	if !response.OK {
		return empty, &serviceError{message: response.Error}
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		return empty, err
	}
	var state SessionFocus
	if err = json.Unmarshal(encoded, &state); err != nil {
		return empty, err
	}
	return state, nil
}

func (service daemonService) Scan(ctx context.Context) error {
	return service.call(ctx, daemon.Request{Method: "scan"})
}

func (service daemonService) forEachWorkspace(ctx context.Context, method string) error {
	loaded, err := config.Load(service.paths)
	if err != nil {
		return err
	}
	for _, workspace := range loaded.Workspaces {
		if err := service.call(ctx, daemon.Request{Method: method, WorkspaceID: workspace.ID}); err != nil {
			return err
		}
	}
	return nil
}

func (service daemonService) call(ctx context.Context, request daemon.Request) error {
	response, err := daemon.Call(ctx, service.paths.Socket, request)
	if err != nil {
		return err
	}
	if !response.OK {
		return &serviceError{message: response.Error}
	}
	return nil
}

type serviceError struct{ message string }

func (err *serviceError) Error() string {
	if err.message == "" {
		return "local service rejected request"
	}
	return err.message
}

func integer(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case json.Number:
		parsed, _ := number.Int64()
		return int(parsed)
	default:
		return 0
	}
}

// SessionMessage is one entry of the caller's own agent session. It is read
// from the local transcript and never leaves this machine (ADR-036).
type SessionMessage struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	Tool string `json:"tool,omitempty"`
	At   string `json:"at,omitempty"`
}

type SessionDetail struct {
	Available bool             `json:"available"`
	Title     string           `json:"title,omitempty"`
	Branch    string           `json:"branch,omitempty"`
	Messages  []SessionMessage `json:"messages"`
}

// SessionDetail returns the local content of one of this device's own agent
// sessions. Sessions observed on another device have no local transcript here,
// so this can only ever return the caller's own work.
func (service daemonService) SessionDetail(ctx context.Context, workstreamID string) (SessionDetail, error) {
	empty := SessionDetail{Messages: []SessionMessage{}}
	if service.paths.Socket == "" {
		return empty, nil
	}
	response, err := daemon.Call(ctx, service.paths.Socket, daemon.Request{Method: "session_detail", AgentWorkstreamID: workstreamID})
	if err != nil || !response.OK {
		return empty, nil
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		return empty, nil
	}
	var detail SessionDetail
	if err = json.Unmarshal(encoded, &detail); err != nil {
		return empty, nil
	}
	if detail.Messages == nil {
		detail.Messages = []SessionMessage{}
	}
	return detail, nil
}
