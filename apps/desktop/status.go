package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type ServiceStatus struct {
	Connected        bool
	WorkspaceCount   int
	PausedWorkspaces int
	PendingEvents    int
}

func (status ServiceStatus) ServiceLabel() string {
	if !status.Connected {
		return "Service: disconnected"
	}
	return fmt.Sprintf("Service: connected · %d workspaces", status.WorkspaceCount)
}

func (status ServiceStatus) ActivityLabel() string {
	if !status.Connected {
		return "Activity: unavailable"
	}
	return fmt.Sprintf("Activity: %d pending events", status.PendingEvents)
}

func (status ServiceStatus) PauseLabel() string {
	if status.Connected && status.WorkspaceCount > 0 && status.PausedWorkspaces == status.WorkspaceCount {
		return "Resume all sharing"
	}
	return "Pause all sharing"
}

func (status ServiceStatus) Tooltip() string {
	if !status.Connected {
		return "Stickguy · service disconnected"
	}
	if status.WorkspaceCount > 0 && status.PausedWorkspaces == status.WorkspaceCount {
		return "Stickguy · sharing paused"
	}
	return "Stickguy · connected"
}

type localService interface {
	Status(context.Context) ServiceStatus
	PauseAll(context.Context) error
	ResumeAll(context.Context) error
	Scan(context.Context) error
}

type controller struct {
	service localService
}

func (controller controller) status(ctx context.Context) ServiceStatus {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	return controller.service.Status(ctx)
}

func (controller controller) togglePause(ctx context.Context, status ServiceStatus) error {
	if !status.Connected || status.WorkspaceCount == 0 {
		return errors.New("no connected workspaces")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if status.PausedWorkspaces == status.WorkspaceCount {
		return controller.service.ResumeAll(ctx)
	}
	return controller.service.PauseAll(ctx)
}
