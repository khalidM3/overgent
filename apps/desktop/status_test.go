package main

import (
	"context"
	"testing"
)

type fakeService struct {
	status                              ServiceStatus
	pauseCalls, resumeCalls, focusCalls int
}

func (service *fakeService) Status(context.Context) ServiceStatus { return service.status }
func (service *fakeService) PauseAll(context.Context) error       { service.pauseCalls++; return nil }
func (service *fakeService) ResumeAll(context.Context) error      { service.resumeCalls++; return nil }
func (service *fakeService) ClearAllFocus(context.Context) error  { service.focusCalls++; return nil }
func (*fakeService) Scan(context.Context) error                   { return nil }

func TestStatusLabelsAreHonest(t *testing.T) {
	disconnected := ServiceStatus{}
	if disconnected.ServiceLabel() != "Service: disconnected" || disconnected.ActivityLabel() != "Activity: unavailable" {
		t.Fatalf("disconnected labels: %q %q", disconnected.ServiceLabel(), disconnected.ActivityLabel())
	}
	connected := ServiceStatus{Connected: true, WorkspaceCount: 2, PausedWorkspaces: 2, PendingEvents: 3}
	if connected.PauseLabel() != "Resume sharing · 2 workspaces" || connected.Tooltip() != "Stickguy · sharing paused" {
		t.Fatalf("connected labels: %q %q", connected.PauseLabel(), connected.Tooltip())
	}
	// This control stops sharing for every Project on the machine, so its label
	// has to say so: pausing the Project someone is reading is a different
	// request, and the dashboard offers that one separately.
	active := ServiceStatus{Connected: true, WorkspaceCount: 3}
	if active.PauseLabel() != "Pause sharing everywhere · 3 workspaces" {
		t.Fatalf("scope label: %q", active.PauseLabel())
	}
	if single := (ServiceStatus{Connected: true, WorkspaceCount: 1}); single.PauseLabel() != "Pause sharing everywhere · 1 workspace" {
		t.Fatalf("singular label: %q", single.PauseLabel())
	}
}

func TestFocusIsOnlyAnnouncedWhileSomethingIsMuted(t *testing.T) {
	// A line that reads "0 sessions" every day is a line nobody reads on the
	// day it matters, so silence is the whole point of the empty label.
	quiet := ServiceStatus{Connected: true, WorkspaceCount: 2}
	if quiet.FocusLabel() != "" || quiet.Tooltip() != "Stickguy · connected" {
		t.Fatalf("unmuted: %q %q", quiet.FocusLabel(), quiet.Tooltip())
	}
	one := ServiceStatus{Connected: true, WorkspaceCount: 2, FocusedSessions: 1}
	if one.FocusLabel() != "1 session is muted · let it hear again" || one.Tooltip() != "Stickguy · connected, some sessions muted" {
		t.Fatalf("one muted: %q %q", one.FocusLabel(), one.Tooltip())
	}
	many := ServiceStatus{Connected: true, WorkspaceCount: 2, FocusedSessions: 3}
	if many.FocusLabel() != "3 sessions are muted · let them hear again" {
		t.Fatalf("many muted: %q", many.FocusLabel())
	}
	service := &fakeService{}
	if err := (controller{service: service}).clearFocus(context.Background()); err != nil || service.focusCalls != 1 {
		t.Fatalf("clear focus calls=%d err=%v", service.focusCalls, err)
	}
}

func TestTogglePauseUsesCurrentAggregateState(t *testing.T) {
	service := &fakeService{}
	controller := controller{service: service}
	if err := controller.togglePause(context.Background(), ServiceStatus{}); err == nil {
		t.Fatal("disconnected toggle succeeded")
	}
	if err := controller.togglePause(context.Background(), ServiceStatus{Connected: true, WorkspaceCount: 2}); err != nil {
		t.Fatal(err)
	}
	if service.pauseCalls != 1 || service.resumeCalls != 0 {
		t.Fatalf("pause=%d resume=%d", service.pauseCalls, service.resumeCalls)
	}
	if err := controller.togglePause(context.Background(), ServiceStatus{Connected: true, WorkspaceCount: 2, PausedWorkspaces: 2}); err != nil {
		t.Fatal(err)
	}
	if service.resumeCalls != 1 {
		t.Fatalf("resume=%d", service.resumeCalls)
	}
}
