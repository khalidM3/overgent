//go:build darwin

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The name this member goes by, remembered for the next Project.
//
// Live-work identity is per Project by construction: `members` is a table in a
// Project's own backend, and a Mac enrolled in two backends genuinely has two
// member rows. That is correct — a team server has no business learning the
// name a member uses on an unrelated one — but it made the *default* wrong.
// First run asks for a name; adding a Project afterwards does not, because a
// Project on this Mac has no collaborators to show it to. The name was
// therefore seeded from the device label, and a member ended up as themselves
// in the Project they set up and as "Khalids-MacBook-Air.local" in the two they
// added later.
//
// This is the same tier as the intelligence defaults: a preference on this Mac
// that seeds a new Project, never an authority over one that exists. Renaming
// inside a Project still changes only that Project.
type memberIdentity struct {
	DisplayName string `json:"displayName,omitempty"`
}

var memberIdentityMu sync.Mutex

func memberIdentityPath(configRoot string) (string, error) {
	absolute, err := filepath.Abs(configRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(absolute, "member-identity.json"), nil
}

// rememberedDisplayName is "" when this Mac has never been told a name, which
// is the same as it was before this existed.
func (service *OnboardingService) rememberedDisplayName() string {
	if service.configRoot == "" {
		return ""
	}
	path, err := memberIdentityPath(service.configRoot)
	if err != nil {
		return ""
	}
	memberIdentityMu.Lock()
	body, readErr := os.ReadFile(path)
	memberIdentityMu.Unlock()
	if readErr != nil {
		return ""
	}
	var identity memberIdentity
	if json.Unmarshal(body, &identity) != nil {
		return ""
	}
	return boundedLabel(identity.DisplayName, "")
}

// rememberDisplayName records a name the member actually chose. It is called
// only with a non-empty name that an enrollment accepted, so a device label
// that was substituted downstream never becomes the remembered identity — that
// would make the fallback permanent instead of fixing it.
func (service *OnboardingService) rememberDisplayName(name string) {
	trimmed := strings.TrimSpace(name)
	if service.configRoot == "" || trimmed == "" {
		return
	}
	path, err := memberIdentityPath(service.configRoot)
	if err != nil {
		return
	}
	body, err := json.Marshal(memberIdentity{DisplayName: trimmed})
	if err != nil {
		return
	}
	memberIdentityMu.Lock()
	_ = os.WriteFile(path, body, 0o600)
	memberIdentityMu.Unlock()
}
