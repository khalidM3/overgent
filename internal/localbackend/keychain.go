package localbackend

import (
	"context"

	"github.com/khalidM3/overgent/internal/credential"
)

// Keychain is the production credential store: the backend's instance secret
// and the deployment secrets key live in the macOS Keychain, never in a file
// under the profile root (docs/security-privacy.md, "Local").
type Keychain struct{}

func (Keychain) Put(ctx context.Context, account, secret string) error {
	return credential.Put(ctx, account, secret)
}

func (Keychain) Get(ctx context.Context, account string) (string, error) {
	return credential.Get(ctx, account)
}

func (Keychain) Delete(ctx context.Context, account string) error {
	return credential.Delete(ctx, account)
}
