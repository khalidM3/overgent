//go:build darwin

package main

import (
	"context"
	"encoding/json"

	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
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
		PendingEvents:    integer(data["pending"]),
	}
}

func (service daemonService) PauseAll(ctx context.Context) error {
	return service.forEachWorkspace(ctx, "pause")
}

func (service daemonService) ResumeAll(ctx context.Context) error {
	return service.forEachWorkspace(ctx, "resume")
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
