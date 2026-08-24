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
	root, err := config.DefaultRoot()
	if err != nil {
		return daemonService{}
	}
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
