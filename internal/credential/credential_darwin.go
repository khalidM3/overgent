//go:build darwin

package credential

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
)

func put(ctx context.Context, account, secret string) error {
	if account == "" || secret == "" || strings.ContainsAny(account, "\r\n") {
		return errors.New("credential account and secret are required")
	}
	// Apple documents a trailing -w with no value as the secure prompt form.
	// Supplying stdin keeps the secret out of argv and ordinary error strings.
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-U", "-s", serviceName, "-a", account, "-w")
	terminal, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("start secure macOS Keychain prompt: %w", err)
	}
	defer terminal.Close()
	reader := bufio.NewReader(terminal)
	for prompt := 0; prompt < 2; prompt++ {
		_ = terminal.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, err = reader.ReadString(':'); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return fmt.Errorf("wait for macOS Keychain password prompt %d: %w", prompt+1, err)
		}
		_ = terminal.SetReadDeadline(time.Time{})
		if _, err = terminal.Write([]byte(secret + "\r")); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return fmt.Errorf("write macOS Keychain password prompt %d: %w", prompt+1, err)
		}
	}
	// Drain the terminal until the child exits. A PTY has a small output buffer;
	// waiting without a reader can leave the Security CLI's terminal session open.
	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		close(drained)
	}()
	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("store device credential in macOS Keychain: %w", err)
	}
	<-drained
	return nil
}

func get(ctx context.Context, account string) (string, error) {
	if account == "" || strings.ContainsAny(account, "\r\n") {
		return "", errors.New("credential account is required")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", serviceName, "-a", account, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read device credential from macOS Keychain: %w", err)
	}
	secret := strings.TrimSpace(string(out))
	if secret == "" {
		return "", errors.New("macOS Keychain returned an empty device credential")
	}
	return secret, nil
}

func remove(ctx context.Context, account string) error {
	if account == "" || strings.ContainsAny(account, "\r\n") {
		return errors.New("credential account is required")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "delete-generic-password", "-s", serviceName, "-a", account)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("delete device credential from macOS Keychain: %w", err)
	}
	return nil
}
