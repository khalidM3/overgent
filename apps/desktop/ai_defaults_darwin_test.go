//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/credential"
)

// A provider key must never be written to a file Overgent controls. The whole
// reason these defaults are safe to keep on disk is that the disk half carries
// no secret: the key lives in the login Keychain, where the member can revoke
// it without this application's help.
func TestAIDefaultsKeepKeysOutOfTheProfileDirectory(t *testing.T) {
	root := t.TempDir()
	service := &OnboardingService{configRoot: root}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = credential.Delete(ctx, defaultKeyAccount(root, "judgment"))
		_ = credential.Delete(ctx, defaultKeyAccount(root, "embeddings"))
	})

	var write DesktopAIDefaultsWrite
	secret := "sk-test-defaults-must-not-be-written-to-disk"
	write.Judgment.Provider = "anthropic"
	write.Judgment.Model = "claude-opus-5"
	write.Judgment.APIKey = &secret
	write.Embeddings.Provider = "deterministic"
	write.Embeddings.Model = "overgent-concepts/v1"
	write.Embeddings.Dimensions = 1024

	stored, err := service.PutAIDefaults(write)
	if err != nil {
		if strings.Contains(err.Error(), "Keychain") {
			t.Skip("this environment has no usable login Keychain")
		}
		t.Fatal(err)
	}
	if !stored.Judgment.KeyStored {
		t.Fatal("a saved key must be reported as stored")
	}

	// Every file this profile now holds, checked for the secret.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		body, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
		if readErr != nil {
			continue
		}
		if strings.Contains(string(body), secret) {
			t.Fatalf("provider key was written to %s", entry.Name())
		}
	}

	// And the readable half must not carry it either, since that shape is what
	// reaches the settings page.
	body, err := json.Marshal(service.readAIDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), secret) {
		t.Fatalf("provider key reached the reported defaults: %s", body)
	}
}

// Turning a provider off must not leave its key behind. A member who selects
// "Off" has said they do not want that provider used; keeping the secret on
// this Mac for it would be storing a credential for something switched off.
func TestAIDefaultsForgetTheKeyOfAProviderTurnedOff(t *testing.T) {
	root := t.TempDir()
	service := &OnboardingService{configRoot: root}
	ctx := context.Background()
	t.Cleanup(func() { _ = credential.Delete(ctx, defaultKeyAccount(root, "judgment")) })

	var write DesktopAIDefaultsWrite
	secret := "sk-test-turned-off"
	write.Judgment.Provider = "anthropic"
	write.Judgment.Model = "claude-opus-5"
	write.Judgment.APIKey = &secret
	write.Embeddings.Provider = "deterministic"
	write.Embeddings.Dimensions = 1024
	if _, err := service.PutAIDefaults(write); err != nil {
		if strings.Contains(err.Error(), "Keychain") {
			t.Skip("this environment has no usable login Keychain")
		}
		t.Fatal(err)
	}

	write.Judgment.Provider = "none"
	write.Judgment.Model = ""
	write.Judgment.APIKey = nil
	after, err := service.PutAIDefaults(write)
	if err != nil {
		t.Fatal(err)
	}
	if after.Judgment.KeyStored {
		t.Fatal("a provider turned off must not report a stored key")
	}
	if secret, getErr := credential.Get(ctx, defaultKeyAccount(root, "judgment")); getErr == nil && secret != "" {
		t.Fatal("a provider turned off left its key in the Keychain")
	}
}

// A blank field means "leave the saved key alone". Treating it as a value would
// overwrite a working key with one that authenticates nothing, and the member
// would only find out when intelligence silently degraded.
func TestAIDefaultsRejectAnEmptyKeyRatherThanStoringOne(t *testing.T) {
	root := t.TempDir()
	service := &OnboardingService{configRoot: root}
	var write DesktopAIDefaultsWrite
	empty := ""
	write.Judgment.Provider = "anthropic"
	write.Judgment.Model = "claude-opus-5"
	write.Judgment.APIKey = &empty
	write.Embeddings.Provider = "deterministic"
	write.Embeddings.Dimensions = 1024
	if _, err := service.PutAIDefaults(write); err == nil {
		t.Fatal("an empty API key must be refused")
	}
}

func TestAIDefaultsRefuseUnsupportedProviders(t *testing.T) {
	service := &OnboardingService{configRoot: t.TempDir()}
	var write DesktopAIDefaultsWrite
	write.Judgment.Provider = "totally-made-up"
	write.Embeddings.Provider = "deterministic"
	write.Embeddings.Dimensions = 1024
	if _, err := service.PutAIDefaults(write); err == nil {
		t.Fatal("an unsupported judgment provider must be refused")
	}
	write.Judgment.Provider = "none"
	write.Embeddings.Provider = "made-up-embeddings"
	if _, err := service.PutAIDefaults(write); err == nil {
		t.Fatal("an unsupported embedding provider must be refused")
	}
}

// Nothing configured means nothing sent. A member who never opened this screen
// gets exactly the behaviour they had before the tier existed.
func TestUnconfiguredAIDefaultsAreNotAppliedToANewProject(t *testing.T) {
	service := &OnboardingService{configRoot: t.TempDir()}
	// applyAIDefaults returns before it needs a backend when nothing is set;
	// reaching one would fail here, which is what makes this assertion real.
	if err := service.applyAIDefaults(context.Background(), "prj_test"); err != nil {
		t.Fatalf("unset defaults must be a no-op, got %v", err)
	}
}
