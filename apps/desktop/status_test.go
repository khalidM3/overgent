package main

import (
	"context"
	"testing"
)

type fakeService struct {
	status                  ServiceStatus
	pauseCalls, resumeCalls int
}

func (service *fakeService) Status(context.Context) ServiceStatus { return service.status }
func (service *fakeService) PauseAll(context.Context) error       { service.pauseCalls++; return nil }
func (service *fakeService) ResumeAll(context.Context) error      { service.resumeCalls++; return nil }
func (*fakeService) Scan(context.Context) error                   { return nil }

func TestStatusLabelsAreHonest(t *testing.T) {
	disconnected := ServiceStatus{}
	if disconnected.ServiceLabel() != "Service: disconnected" || disconnected.ActivityLabel() != "Activity: unavailable" {
		t.Fatalf("disconnected labels: %q %q", disconnected.ServiceLabel(), disconnected.ActivityLabel())
	}
	connected := ServiceStatus{Connected: true, WorkspaceCount: 2, PausedWorkspaces: 2, PendingEvents: 3}
	if connected.PauseLabel() != "Resume all sharing" || connected.Tooltip() != "Stickguy · sharing paused" {
		t.Fatalf("connected labels: %q %q", connected.PauseLabel(), connected.Tooltip())
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
