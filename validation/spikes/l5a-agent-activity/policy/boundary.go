package policy

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Sink interface {
	Store(context.Context, Event, time.Time) error
	Send(context.Context, Event) error
	DeleteProject(context.Context, string) error
	DeleteExpired(context.Context, time.Time) error
}

type Boundary struct {
	mu        sync.RWMutex
	consent   Consent
	retention time.Duration
	sink      Sink
}

func NewBoundary(consent Consent, retention time.Duration, sink Sink) (*Boundary, error) {
	if retention <= 0 || sink == nil {
		return nil, errors.New("retention and sink are required")
	}
	return &Boundary{consent: consent, retention: retention, sink: sink}, nil
}

func (boundary *Boundary) SetConsent(consent Consent) {
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	boundary.consent = consent
}

func (boundary *Boundary) Preview(candidate Candidate) (Event, error) {
	boundary.mu.RLock()
	defer boundary.mu.RUnlock()
	return Project(candidate, boundary.consent)
}

func (boundary *Boundary) Ingest(ctx context.Context, candidate Candidate) error {
	boundary.mu.RLock()
	defer boundary.mu.RUnlock()
	event, err := Project(candidate, boundary.consent)
	if err != nil {
		return err
	}
	if err := boundary.sink.Store(ctx, event, candidate.ObservedAt.Add(boundary.retention)); err != nil {
		return err
	}
	return boundary.sink.Send(ctx, event)
}

func (boundary *Boundary) DeleteShared(ctx context.Context) error {
	boundary.mu.RLock()
	defer boundary.mu.RUnlock()
	return boundary.sink.DeleteProject(ctx, boundary.consent.ProjectID)
}

func (boundary *Boundary) DeleteExpired(ctx context.Context, now time.Time) error {
	return boundary.sink.DeleteExpired(ctx, now)
}
