package localbackend

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// ephemeralCredentials holds a throwaway instance secret for Verify. It never
// touches the Keychain: the check is about the payload, not about this machine,
// and a release runner must not leave credentials behind.
type ephemeralCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func (store *ephemeralCredentials) Put(_ context.Context, account, secret string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[account] = secret
	return nil
}

func (store *ephemeralCredentials) Get(_ context.Context, account string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if secret, ok := store.values[account]; ok {
		return secret, nil
	}
	return "", os.ErrNotExist
}

func (store *ephemeralCredentials) Delete(_ context.Context, account string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.values, account)
	return nil
}

// Verify replays a freshly built deploy payload against a fresh backend and
// reports whether it worked.
//
// This is the release gate the bundled-backend spike asked for. The deploy2
// endpoints are internal Convex detail, so a backend or CLI bump can change the
// wire shape without any announcement; running the actual Go replay against the
// actual binary at release time is what keeps a broken pin from reaching a
// member as an app that starts and then coordinates nothing.
func Verify(ctx context.Context, binaryPath, bundlePath string) error {
	root, err := os.MkdirTemp("", "overgent-backend-verify-")
	if err != nil {
		return fmt.Errorf("create verification profile: %w", err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	manager, err := New(root, &ephemeralCredentials{values: map[string]string{}}, nil)
	if err != nil {
		return err
	}
	if err = manager.SetArtifacts(binaryPath, bundlePath); err != nil {
		return err
	}
	defer func() { _ = manager.Stop(context.WithoutCancel(ctx)) }()
	endpoint, err := manager.Ensure(ctx)
	if err != nil {
		return fmt.Errorf("replay the release payload against a fresh backend: %w", err)
	}
	// A backend with the functions deployed answers the contract's routes. An
	// unauthenticated bootstrap must be refused, not 404: that is the
	// difference between "the functions are there" and "the server is up".
	if status := probe(ctx, endpoint.SiteOrigin+"/v1/device/bootstrap"); status != 401 {
		return fmt.Errorf("deployed backend answered /v1/device/bootstrap with HTTP %d, want 401", status)
	}
	return nil
}
