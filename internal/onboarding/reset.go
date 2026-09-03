package onboarding

import (
	"context"
	"errors"
	"fmt"

	"github.com/overgent/overgent/internal/config"
	"github.com/overgent/overgent/internal/hosted"
)

// ResetOutcome describes what a reset changed on this device.
type ResetOutcome struct {
	Status            hosted.CredentialStatus
	DeviceID          string
	ClearedWorkspaces int
	CredentialDeleted bool
}

// CredentialState reports whether the credential stored for the local profile
// is still accepted by the hosted API. A device with no enrollment is reported
// as OK because there is nothing that could have been rejected.
func (s Service) CredentialState(ctx context.Context, configRoot string) (hosted.CredentialStatus, config.Config, error) {
	paths, err := config.Resolve(configRoot)
	if err != nil {
		return hosted.CredentialUncertain, config.Config{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return hosted.CredentialUncertain, config.Config{}, err
	}
	if cfg.DeviceID == "" {
		return hosted.CredentialOK, cfg, nil
	}
	if s.Creds == nil {
		return hosted.CredentialUncertain, cfg, errors.New("credential store is unavailable")
	}
	token, err := s.Creds.Get(ctx, cfg.DeviceID)
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

// Reset forgets this device's identity so the member can enroll again. It
// deletes the stored credential and clears the device and workspace bindings,
// keeping the API origin so re-enrollment targets the same deployment.
//
// It refuses unless the hosted API has actually rejected the credential. Being
// offline is not being locked out, and erasing a working enrollment cannot be
// undone. force exists for support and headless recovery, where the operator
// has already established that the enrollment is dead.
func (s Service) Reset(ctx context.Context, configRoot string, force bool) (ResetOutcome, error) {
	status, cfg, err := s.CredentialState(ctx, configRoot)
	if err != nil && !force {
		return ResetOutcome{Status: status}, err
	}
	if cfg.DeviceID == "" {
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
	outcome := ResetOutcome{Status: status, DeviceID: cfg.DeviceID, ClearedWorkspaces: len(cfg.Workspaces)}
	if s.Creds != nil {
		// A secret that is already gone is not a failure, and must not stop the
		// profile from being cleared.
		if deleteErr := s.Creds.Delete(ctx, cfg.DeviceID); deleteErr == nil {
			outcome.CredentialDeleted = true
		}
	}
	if err := config.Save(paths, config.Config{Version: 1, APIBaseURL: cfg.APIBaseURL}); err != nil {
		return ResetOutcome{Status: status}, fmt.Errorf("clear local enrollment: %w", err)
	}
	return outcome, nil
}
