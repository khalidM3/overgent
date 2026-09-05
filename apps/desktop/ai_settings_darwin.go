//go:build darwin

package main

import (
	"context"
	"errors"
	"time"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/hosted"
	protocoltypes "github.com/khalidM3/overgent/protocol/generated/go"
)

type DesktopAISettingsWrite struct {
	Judgment struct {
		Provider string  `json:"provider"`
		Model    string  `json:"model"`
		BaseURL  *string `json:"baseUrl,omitempty"`
		APIKey   *string `json:"apiKey,omitempty"`
	} `json:"judgment"`
	Embeddings struct {
		Provider   string  `json:"provider"`
		Model      string  `json:"model"`
		Dimensions int     `json:"dimensions"`
		BaseURL    *string `json:"baseUrl,omitempty"`
		APIKey     *string `json:"apiKey,omitempty"`
	} `json:"embeddings"`
}

func (service *OnboardingService) AISettings(projectID string) (hosted.AISettings, error) {
	client, err := service.aiSettingsClient(projectID)
	if err != nil {
		return hosted.AISettings{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return client.AISettings(ctx, projectID)
}

func (service *OnboardingService) PutAISettings(projectID string, input DesktopAISettingsWrite) (hosted.AISettings, error) {
	client, err := service.aiSettingsClient(projectID)
	if err != nil {
		return hosted.AISettings{}, err
	}
	var write hosted.AISettingsWrite
	write.Judgment.Provider = input.Judgment.Provider
	write.Judgment.Model = input.Judgment.Model
	write.Judgment.BaseUrl = input.Judgment.BaseURL
	write.Embeddings.Provider = input.Embeddings.Provider
	write.Embeddings.Model = input.Embeddings.Model
	write.Embeddings.Dimensions = input.Embeddings.Dimensions
	write.Embeddings.BaseUrl = input.Embeddings.BaseURL
	if input.Judgment.APIKey != nil {
		var key protocoltypes.PutProjectAISettingsJSONBody_Judgment_ApiKey
		if err := key.FromPutProjectAISettingsJSONBodyJudgmentApiKey1(*input.Judgment.APIKey); err != nil {
			return hosted.AISettings{}, err
		}
		write.Judgment.ApiKey = &key
	}
	if input.Embeddings.APIKey != nil {
		var key protocoltypes.PutProjectAISettingsJSONBody_Embeddings_ApiKey
		if err := key.FromPutProjectAISettingsJSONBodyEmbeddingsApiKey1(*input.Embeddings.APIKey); err != nil {
			return hosted.AISettings{}, err
		}
		write.Embeddings.ApiKey = &key
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return client.PutAISettings(ctx, projectID, write)
}

func (service *OnboardingService) aiSettingsClient(projectID string) (*hosted.Client, error) {
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return nil, err
	}
	member := false
	for _, workspace := range cfg.Workspaces {
		if workspace.ProjectID == projectID {
			member = true
			break
		}
	}
	// Settings are a property of the Project (ADR-073), so they are written to
	// the backend that Project lives on, with the credential this Mac holds
	// for that backend.
	backend, bound := cfg.BackendForProject(projectID)
	if !member || !bound || backend.DeviceID == "" {
		return nil, errors.New("Project is not enrolled on this device")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token, err := credential.Get(ctx, backend.DeviceID)
	if err != nil {
		return nil, err
	}
	return hosted.New(backend.APIBaseURL, token)
}
