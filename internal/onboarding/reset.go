package onboarding

import (
	"context"
	"errors"
	"fmt"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/hosted"
)

// ResetOutcome describes what a reset changed on this device.
type ResetOutcome struct {
	Status            hosted.CredentialStatus
	BackendID         string
	APIBaseURL        string
	DeviceID          string
	ClearedWorkspaces int
	CredentialDeleted bool
}

// CredentialState reports whether the credential stored for this flow's
// backend is still accepted by it. A backend this profile has no device
// identity for is reported as OK because there is nothing that could have been
// rejected.
//
// It is per backend, not per profile: one revoked team Project says nothing
// about the local Project beside it.
func (s Service) CredentialState(ctx context.Context, configRoot string) (hosted.CredentialStatus, config.Config, error) {
	paths, err := config.Resolve(configRoot)
	if err != nil {
		return hosted.CredentialUncertain, config.Config{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return hosted.CredentialUncertain, config.Config{}, err
	}
	if s.Backend.DeviceID == "" {
		return hosted.CredentialOK, cfg, nil
	}
	if s.Creds == nil {
		return hosted.CredentialUncertain, cfg, errors.New("credential store is unavailable")
	}
	token, err := s.Creds.Get(ctx, s.Backend.DeviceID)
	if err != nil || token == "" {
		// The stored secret is gone, so nothing can authenticate with it. That is
		// the same dead end as a rejection and has the same recovery.
		return hosted.CredentialUnknown, cfg, nil
	}
	client, err := s.Client(token)
	if err != nil {
		return hosted.CredentialUncertain, cfg, err
	}
	_, callErr := client.Bootstrap(ctx)
	return hosted.ClassifyCredentialError(callErr), cfg, nil
}

// Reset forgets this device's identity on one backend so the member can enroll
// against it again. It deletes that backend's stored credential and removes
// the backend, its Projects, and the repositories registered to them. Every
// other backend on the profile is untouched, which is the whole point of
// binding Projects to backends: a revoked team Project must not take the local
// Project down with it.
//
// It refuses unless that backend has actually rejected the credential. Being
// offline is not being locked out, and erasing a working enrollment cannot be
// undone. force exists for support and headless recovery, where the operator
// has already established that the enrollment is dead.
func (s Service) Reset(ctx context.Context, configRoot string, force bool) (ResetOutcome, error) {
	status, cfg, err := s.CredentialState(ctx, configRoot)
	if err != nil && !force {
		return ResetOutcome{Status: status}, err
	}
	if s.Backend.DeviceID == "" {
		// Already reset, or never enrolled. Doing nothing is the correct result.
		return ResetOutcome{Status: status}, nil
	}
	if !force && !status.Recoverable() {
		if status == hosted.CredentialOK {
			return ResetOutcome{Status: status}, errors.New("this device's credential is still valid, so there is nothing to reset")
		}
		return ResetOutcome{Status: status}, errors.New("could not confirm this device is locked out; check the connection and try again, or pass --force")
	}
	paths, err := config.Resolve(configRoot)
	if err != nil {
		return ResetOutcome{Status: status}, err
	}
	outcome := ResetOutcome{Status: status, BackendID: s.Backend.ID, APIBaseURL: s.Backend.APIBaseURL, DeviceID: s.Backend.DeviceID}
	if s.Creds != nil {
		// A secret that is already gone is not a failure, and must not stop the
		// profile from being cleared.
		if deleteErr := s.Creds.Delete(ctx, s.Backend.DeviceID); deleteErr == nil {
			outcome.CredentialDeleted = true
		}
	}
	next, cleared := cfg.RemoveBackend(s.Backend.ID)
	outcome.ClearedWorkspaces = cleared
	if err := config.Save(paths, next); err != nil {
		return ResetOutcome{Status: status}, fmt.Errorf("clear local enrollment: %w", err)
	}
	return outcome, nil
}

// ResetAll runs Reset for every backend on the profile, which is what a member
// asking to forget this Mac entirely means. It reports each backend's outcome
// and stops at the first refusal, so a working enrollment is never erased on
// the way to a broken one.
func ResetAll(ctx context.Context, configRoot string, force bool) ([]ResetOutcome, error) {
	paths, err := config.Resolve(configRoot)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return nil, err
	}
	outcomes := make([]ResetOutcome, 0, len(cfg.Backends))
	for _, backend := range cfg.Backends {
		outcome, resetErr := New(backend).Reset(ctx, configRoot, force)
		outcomes = append(outcomes, outcome)
		if resetErr != nil {
			return outcomes, resetErr
		}
	}
	return outcomes, nil
}
