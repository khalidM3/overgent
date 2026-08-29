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
	// FocusedSessions counts agent sessions currently receiving no coordination
	// because the member asked for quiet. It is surfaced here because the tray
	// is where someone notices a mute they have forgotten they set.
	FocusedSessions int
	PendingEvents   int
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

// PauseLabel names the scope this control actually has. It stops sharing for
// every workspace on this Mac, across every Project, which is a different
// request from pausing the Project someone happens to be reading; saying the
// count is what keeps the two from being confused for each other.
func (status ServiceStatus) PauseLabel() string {
	if !status.Connected || status.WorkspaceCount == 0 {
		return "Pause sharing everywhere"
	}
	if status.PausedWorkspaces == status.WorkspaceCount {
		return fmt.Sprintf("Resume sharing · %s", workspaceCount(status.WorkspaceCount))
	}
	return fmt.Sprintf("Pause sharing everywhere · %s", workspaceCount(status.WorkspaceCount))
}

// FocusLabel is shown only while something is muted, because a control that
// says "0 sessions" every day is a control nobody reads on the day it matters.
func (status ServiceStatus) FocusLabel() string {
	if status.FocusedSessions == 0 {
		return ""
	}
	if status.FocusedSessions == 1 {
		return "1 session is muted · let it hear again"
	}
	return fmt.Sprintf("%d sessions are muted · let them hear again", status.FocusedSessions)
}

func workspaceCount(count int) string {
	if count == 1 {
		return "1 workspace"
	}
	return fmt.Sprintf("%d workspaces", count)
}

func (status ServiceStatus) Tooltip() string {
	if !status.Connected {
		return "Stickguy · service disconnected"
	}
	if status.WorkspaceCount > 0 && status.PausedWorkspaces == status.WorkspaceCount {
		return "Stickguy · sharing paused"
	}
	if status.FocusedSessions > 0 {
		return "Stickguy · connected, some sessions muted"
	}
	return "Stickguy · connected"
}

type localService interface {
	Status(context.Context) ServiceStatus
	PauseAll(context.Context) error
	ResumeAll(context.Context) error
	ClearAllFocus(context.Context) error
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

func (controller controller) clearFocus(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return controller.service.ClearAllFocus(ctx)
}
