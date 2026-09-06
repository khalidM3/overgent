//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/khalidM3/overgent/internal/credential"
)

// Intelligence defaults for new Projects, held on this Mac.
//
// A Project's own settings remain the only thing that runs (ADR-073): they live
// in that Project's backend, encrypted with that backend's secret, and nothing
// here changes that. This is one tier above — what a member typed once, so the
// next Project does not ask again. It is the tier that was missing, and its
// absence is why configuring intelligence was per-Project data entry rather
// than a preference.
//
// Two rules keep it from becoming a way to leak a key somewhere unintended:
//
//   - The non-secret half is a plain file in the profile. The keys are not:
//     they go to the login Keychain under their own accounts, so a readable
//     config never carries a provider key and a member can revoke them in
//     Keychain Access without this application's help.
//   - Defaults are applied automatically only to a Project on this Mac, where
//     the backend is the member's own machine and applying them sends the key
//     nowhere. For a shared Project they are offered to the settings form and
//     saved only when the member says so, because saving there uploads the key
//     to a server other members' sessions spend it from.
type aiDefaults struct {
	Judgment struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		BaseURL  string `json:"baseUrl,omitempty"`
		// KeyStored reports that a key exists in the Keychain for this
		// provider. The key itself is never in this struct, so it can never be
		// returned to a page or written to the file below.
		KeyStored bool `json:"keyStored,omitempty"`
	} `json:"judgment"`
	Embeddings struct {
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		Dimensions int    `json:"dimensions"`
		BaseURL    string `json:"baseUrl,omitempty"`
		KeyStored  bool   `json:"keyStored,omitempty"`
	} `json:"embeddings"`
}

// DesktopAIDefaultsWrite is what the settings form sends. A nil APIKey leaves
// the stored key alone, an empty one is rejected, and RemoveKey deletes it —
// the same three-way contract the per-Project form already uses, so the two
// screens cannot disagree about what a blank field means.
type DesktopAIDefaultsWrite struct {
	Judgment struct {
		Provider  string  `json:"provider"`
		Model     string  `json:"model"`
		BaseURL   *string `json:"baseUrl,omitempty"`
		APIKey    *string `json:"apiKey,omitempty"`
		RemoveKey bool    `json:"removeKey,omitempty"`
	} `json:"judgment"`
	Embeddings struct {
		Provider   string  `json:"provider"`
		Model      string  `json:"model"`
		Dimensions int     `json:"dimensions"`
		BaseURL    *string `json:"baseUrl,omitempty"`
		APIKey     *string `json:"apiKey,omitempty"`
		RemoveKey  bool    `json:"removeKey,omitempty"`
	} `json:"embeddings"`
}

// Keychain accounts for the two default keys. They are per profile, so a
// development profile and a release profile do not share a key, matching how
// every other credential on this Mac is scoped.
func defaultKeyAccount(configRoot, kind string) string {
	return "overgent.ai-default." + kind + "." + filepath.Base(configRoot)
}

var aiDefaultsMu sync.Mutex

func aiDefaultsPath(configRoot string) (string, error) {
	absolute, err := filepath.Abs(configRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(absolute, "ai-defaults.json"), nil
}

// readAIDefaults never fails the caller: every field is a preference with a
// working default, and a Project configures itself perfectly well without any
// of them.
func (service *OnboardingService) readAIDefaults() aiDefaults {
	var defaults aiDefaults
	defaults.Embeddings.Dimensions = 1024
	if service.configRoot == "" {
		return defaults
	}
	path, err := aiDefaultsPath(service.configRoot)
	if err != nil {
		return defaults
	}
	aiDefaultsMu.Lock()
	body, readErr := os.ReadFile(path)
	aiDefaultsMu.Unlock()
	if readErr != nil {
		return defaults
	}
	if json.Unmarshal(body, &defaults) != nil {
		var empty aiDefaults
		empty.Embeddings.Dimensions = 1024
		return empty
	}
	if defaults.Embeddings.Dimensions == 0 {
		defaults.Embeddings.Dimensions = 1024
	}
	// The file records that a key was stored; the Keychain decides whether one
	// still is. A member who deleted it in Keychain Access must not be shown a
	// key this Mac cannot produce.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defaults.Judgment.KeyStored = defaults.Judgment.KeyStored && service.hasDefaultKey(ctx, "judgment")
	defaults.Embeddings.KeyStored = defaults.Embeddings.KeyStored && service.hasDefaultKey(ctx, "embeddings")
	return defaults
}

func (service *OnboardingService) hasDefaultKey(ctx context.Context, kind string) bool {
	secret, err := credential.Get(ctx, defaultKeyAccount(service.configRoot, kind))
	return err == nil && secret != ""
}

// AIDefaults reports this Mac's defaults for new Projects. Keys are reported
// only as stored-or-not; nothing here can return one.
func (service *OnboardingService) AIDefaults() (aiDefaults, error) {
	if service.configRoot == "" {
		return aiDefaults{}, errors.New("local Overgent configuration is unavailable")
	}
	return service.readAIDefaults(), nil
}

// PutAIDefaults records what new Projects should start from.
func (service *OnboardingService) PutAIDefaults(write DesktopAIDefaultsWrite) (aiDefaults, error) {
	if service.configRoot == "" {
		return aiDefaults{}, errors.New("local Overgent configuration is unavailable")
	}
	judgmentProvider := strings.TrimSpace(write.Judgment.Provider)
	embeddingProvider := strings.TrimSpace(write.Embeddings.Provider)
	switch judgmentProvider {
	case "anthropic", "openai-compatible", "none":
	default:
		return aiDefaults{}, errors.New("judgment provider is not supported")
	}
	switch embeddingProvider {
	case "openai", "deterministic":
	default:
		return aiDefaults{}, errors.New("embedding provider is not supported")
	}

	current := service.readAIDefaults()
	next := aiDefaults{}
	next.Judgment.Provider = judgmentProvider
	next.Judgment.Model = bounded(write.Judgment.Model, 120)
	next.Judgment.BaseURL = bounded(derefString(write.Judgment.BaseURL), 512)
	next.Embeddings.Provider = embeddingProvider
	next.Embeddings.Model = bounded(write.Embeddings.Model, 120)
	next.Embeddings.BaseURL = bounded(derefString(write.Embeddings.BaseURL), 512)
	next.Embeddings.Dimensions = 1024

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	judgmentStored, err := service.applyDefaultKey(ctx, "judgment", write.Judgment.APIKey, write.Judgment.RemoveKey, current.Judgment.KeyStored)
	if err != nil {
		return aiDefaults{}, err
	}
	embeddingStored, err := service.applyDefaultKey(ctx, "embeddings", write.Embeddings.APIKey, write.Embeddings.RemoveKey, current.Embeddings.KeyStored)
	if err != nil {
		return aiDefaults{}, err
	}
	// A provider that is switched off holds no key. Leaving one behind would
	// keep a secret on this Mac for something the member turned off.
	if judgmentProvider == "none" {
		_ = credential.Delete(ctx, defaultKeyAccount(service.configRoot, "judgment"))
		judgmentStored = false
	}
	if embeddingProvider == "deterministic" {
		_ = credential.Delete(ctx, defaultKeyAccount(service.configRoot, "embeddings"))
		embeddingStored = false
	}
	next.Judgment.KeyStored = judgmentStored
	next.Embeddings.KeyStored = embeddingStored

	path, err := aiDefaultsPath(service.configRoot)
	if err != nil {
		return aiDefaults{}, err
	}
	body, err := json.Marshal(next)
	if err != nil {
		return aiDefaults{}, err
	}
	aiDefaultsMu.Lock()
	err = os.WriteFile(path, body, 0o600)
	aiDefaultsMu.Unlock()
	if err != nil {
		return aiDefaults{}, err
	}
	return next, nil
}

// applyDefaultKey resolves the three-way contract for one key and reports
// whether a key is stored afterwards.
func (service *OnboardingService) applyDefaultKey(ctx context.Context, kind string, key *string, remove, stored bool) (bool, error) {
	account := defaultKeyAccount(service.configRoot, kind)
	if remove {
		if err := credential.Delete(ctx, account); err != nil {
			return stored, errors.New("could not remove the saved key from this Mac's Keychain")
		}
		return false, nil
	}
	if key == nil {
		return stored, nil
	}
	value := strings.TrimSpace(*key)
	if value == "" {
		// A blank field means "leave it alone", which the nil case above
		// already covers. Reaching here means an empty string was sent
		// deliberately, which would store a key that authenticates nothing.
		return stored, errors.New("an API key cannot be empty")
	}
	if len(value) > 512 {
		return stored, errors.New("that API key is longer than any provider issues")
	}
	if err := credential.Put(ctx, account, value); err != nil {
		return stored, errors.New("could not save the key to this Mac's Keychain")
	}
	return true, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func bounded(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > limit {
		return trimmed[:limit]
	}
	return trimmed
}

// applyAIDefaults writes this Mac's defaults into a Project that has just been
// created on it.
//
// It reads the keys out of the Keychain here rather than holding them anywhere
// longer-lived, and it is called only for a local Project, whose backend is the
// loopback server on this same machine — so the key travels from the Keychain
// to a database on the member's own disk and nowhere else.
//
// Nothing configured means nothing sent: a member who has set no defaults gets
// a Project with the deployment's own defaults, which is what happened before
// this tier existed.
func (service *OnboardingService) applyAIDefaults(ctx context.Context, projectID string) error {
	defaults := service.readAIDefaults()
	judgmentOff := defaults.Judgment.Provider == "" || defaults.Judgment.Provider == "none"
	embeddingsOff := defaults.Embeddings.Provider == "" || defaults.Embeddings.Provider == "deterministic"
	if judgmentOff && embeddingsOff {
		return nil
	}
	var write DesktopAIDefaultsWriteToProject
	write.Judgment.Provider = defaults.Judgment.Provider
	if judgmentOff {
		write.Judgment.Provider = "none"
	}
	write.Judgment.Model = defaults.Judgment.Model
	write.Embeddings.Provider = defaults.Embeddings.Provider
	if embeddingsOff {
		write.Embeddings.Provider = "deterministic"
	}
	write.Embeddings.Model = defaults.Embeddings.Model
	write.Embeddings.Dimensions = 1024
	if defaults.Judgment.BaseURL != "" {
		value := defaults.Judgment.BaseURL
		write.Judgment.BaseURL = &value
	}
	if defaults.Embeddings.BaseURL != "" {
		value := defaults.Embeddings.BaseURL
		write.Embeddings.BaseURL = &value
	}
	if !judgmentOff && defaults.Judgment.KeyStored {
		key, err := credential.Get(ctx, defaultKeyAccount(service.configRoot, "judgment"))
		if err == nil && key != "" {
			write.Judgment.APIKey = &key
		}
	}
	if !embeddingsOff && defaults.Embeddings.KeyStored {
		key, err := credential.Get(ctx, defaultKeyAccount(service.configRoot, "embeddings"))
		if err == nil && key != "" {
			write.Embeddings.APIKey = &key
		}
	}
	_, err := service.PutAISettings(projectID, DesktopAISettingsWrite(write))
	return err
}

// DesktopAIDefaultsWriteToProject is structurally the per-Project write. The
// alias exists so the conversion above is a declared cast rather than a field
// copy that would silently drop anything added to either shape later.
type DesktopAIDefaultsWriteToProject = DesktopAISettingsWrite
