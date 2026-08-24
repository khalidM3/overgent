package policy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type storedEvent struct {
	event     Event
	expiresAt time.Time
}

type memorySink struct {
	mu     sync.Mutex
	stored []storedEvent
	sent   []Event
}

func (sink *memorySink) Store(_ context.Context, event Event, expiresAt time.Time) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.stored = append(sink.stored, storedEvent{event: event, expiresAt: expiresAt})
	return nil
}

func (sink *memorySink) Send(_ context.Context, event Event) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.sent = append(sink.sent, event)
	return nil
}

func (sink *memorySink) DeleteProject(_ context.Context, projectID string) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	kept := sink.stored[:0]
	for _, record := range sink.stored {
		if record.event.ProjectID != projectID {
			kept = append(kept, record)
		}
	}
	sink.stored = kept
	return nil
}

func (sink *memorySink) DeleteExpired(_ context.Context, now time.Time) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	kept := sink.stored[:0]
	for _, record := range sink.stored {
		if record.expiresAt.After(now) {
			kept = append(kept, record)
		}
	}
	sink.stored = kept
	return nil
}

func TestBoundaryRejectsBeforeStoreOrSend(t *testing.T) {
	sink := &memorySink{}
	boundary, err := NewBoundary(consent(Conversation), 24*time.Hour, sink)
	if err != nil {
		t.Fatal(err)
	}
	candidate := baseCandidate("conversation.assistant")
	candidate.Text = "safe-looking summary"
	candidate.HasReasoning = true
	if err := boundary.Ingest(context.Background(), candidate); !errors.Is(err, errProhibitedContent) {
		t.Fatalf("error = %v", err)
	}
	if len(sink.stored) != 0 || len(sink.sent) != 0 {
		t.Fatalf("prohibited candidate crossed boundary: stored=%d sent=%d", len(sink.stored), len(sink.sent))
	}
}

func TestPreviewDoesNotStoreOrSend(t *testing.T) {
	sink := &memorySink{}
	boundary, _ := NewBoundary(consent(Activity), 24*time.Hour, sink)
	candidate := baseCandidate("session.status")
	candidate.Status = "running"
	event, err := boundary.Preview(candidate)
	if err != nil || event.Kind != candidate.Kind {
		t.Fatalf("preview = %#v, %v", event, err)
	}
	if len(sink.stored) != 0 || len(sink.sent) != 0 {
		t.Fatal("preview persisted or sent data")
	}
}

func TestPauseAndDowngradeAreSynchronous(t *testing.T) {
	sink := &memorySink{}
	boundary, _ := NewBoundary(consent(Conversation), 24*time.Hour, sink)
	candidate := baseCandidate("conversation.user")
	candidate.Text = "bounded fixture message"

	paused := consent(Conversation)
	paused.Paused = true
	boundary.SetConsent(paused)
	if err := boundary.Ingest(context.Background(), candidate); !errors.Is(err, errPaused) {
		t.Fatalf("paused error = %v", err)
	}
	boundary.SetConsent(consent(Activity))
	if err := boundary.Ingest(context.Background(), candidate); !errors.Is(err, errProfile) {
		t.Fatalf("downgrade error = %v", err)
	}
	if len(sink.stored) != 0 || len(sink.sent) != 0 {
		t.Fatal("paused or downgraded event crossed boundary")
	}
}

func TestRetentionAndProjectDeletion(t *testing.T) {
	sink := &memorySink{}
	boundary, _ := NewBoundary(consent(Activity), time.Hour, sink)
	first := baseCandidate("session.status")
	first.Status = "running"
	if err := boundary.Ingest(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ProjectID = "prj_other"
	sink.stored = append(sink.stored, storedEvent{event: Event{ProjectID: "prj_other"}, expiresAt: fixtureTime.Add(2 * time.Hour)})

	if err := boundary.DeleteExpired(context.Background(), fixtureTime.Add(90*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(sink.stored) != 1 || sink.stored[0].event.ProjectID != second.ProjectID {
		t.Fatalf("retention result = %#v", sink.stored)
	}
	if err := boundary.DeleteShared(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.stored) != 1 {
		t.Fatal("deleting one Project affected another Project")
	}
}
