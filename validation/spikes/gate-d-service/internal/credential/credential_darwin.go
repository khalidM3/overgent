//go:build darwin

package credential

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const serviceName = "dev.stickguy.validation.gate-d"

func Put(ctx context.Context, account, secret string) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-U", "-s", serviceName, "-a", account, "-w", secret)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("store credential in macOS keychain: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func Get(ctx context.Context, account string) (string, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", serviceName, "-a", account, "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read credential from macOS keychain: %w", err)
	}
	return strings.TrimSuffix(string(output), "\n"), nil
}

func Delete(ctx context.Context, account string) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "delete-generic-password", "-s", serviceName, "-a", account)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("delete credential from macOS keychain: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
