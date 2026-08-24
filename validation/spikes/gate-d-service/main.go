package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"stickguy.dev/validation/gate-d-service/internal/credential"
	"stickguy.dev/validation/gate-d-service/internal/daemon"
	"stickguy.dev/validation/gate-d-service/internal/service"
	"stickguy.dev/validation/gate-d-service/internal/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: gate-d-service <cli|service|mcp|credential>")
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	switch args[0] {
	case "cli":
		if len(args) != 2 || args[1] != "ping" {
			return errors.New("usage: gate-d-service cli ping")
		}
		return daemon.Ping(ctx, filepath.Join(root, "service.sock"), out)
	case "mcp":
		return runMCP(ctx, in, out, filepath.Join(root, "service.sock"))
	case "service":
		return runServiceCommand(ctx, args[1:], root, out, errOut)
	case "credential":
		return runCredential(ctx, args[1:], out)
	default:
		return fmt.Errorf("unknown mode %q", args[0])
	}
}

func stateRoot() (string, error) {
	if root := os.Getenv("STICKGUY_SPIKE_STATE_DIR"); root != "" {
		return filepath.Abs(root)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(dir, "stickguy-gate-d-spike"), nil
}

func runServiceCommand(ctx context.Context, args []string, root string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: gate-d-service service <run|install|start|status|stop|remove>")
	}
	if args[0] == "run" {
		return runService(ctx, root, errOut)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	mgr := service.New(root, exe)
	switch args[0] {
	case "install":
		return mgr.Install(ctx, out)
	case "start":
		return mgr.Start(ctx, out)
	case "status":
		return mgr.Status(ctx, out)
	case "stop":
		return mgr.Stop(ctx, out)
	case "remove":
		return mgr.Remove(ctx, out)
	default:
		return fmt.Errorf("unknown service operation %q", args[0])
	}
}

func runService(parent context.Context, root string, errOut io.Writer) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	lock, err := daemon.Acquire(filepath.Join(root, "service.lock"))
	if err != nil {
		var health daemon.Health
		if healthErr := daemon.Query(parent, filepath.Join(root, "service.sock"), &health); healthErr == nil && health.Status == "ok" {
			return fmt.Errorf("healthy service instance already running (pid %d): %w", health.PID, err)
		}
		return fmt.Errorf("service instance lock is held but IPC health check failed: %w", err)
	}
	defer lock.Close()
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := store.Open(filepath.Join(root, "state.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	bootCount, err := db.RecordBoot(ctx)
	if err != nil {
		return err
	}
	slog.New(slog.NewJSONHandler(errOut, nil)).Info("service ready", "boot_count", bootCount)
	return daemon.Serve(ctx, filepath.Join(root, "service.sock"), bootCount)
}

func runMCP(ctx context.Context, in io.Reader, out io.Writer, socket string) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = enc.Encode(map[string]any{"error": "invalid_json"})
			continue
		}
		if req.Method != "ping" {
			_ = enc.Encode(map[string]any{"id": req.ID, "error": "method_not_found"})
			continue
		}
		var response daemon.Health
		if err := daemon.Query(ctx, socket, &response); err != nil {
			_ = enc.Encode(map[string]any{"id": req.ID, "error": "service_unavailable"})
			continue
		}
		_ = enc.Encode(map[string]any{"id": req.ID, "result": response})
	}
	return scanner.Err()
}

func runCredential(ctx context.Context, args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: gate-d-service credential <put|get|delete> <account> [secret]")
	}
	switch args[0] {
	case "put":
		if len(args) != 3 {
			return errors.New("credential put requires account and secret")
		}
		return credential.Put(ctx, args[1], args[2])
	case "get":
		if len(args) != 2 {
			return errors.New("credential get requires account")
		}
		value, err := credential.Get(ctx, args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "credential-present bytes="+strconv.Itoa(len(value)))
		return err
	case "delete":
		if len(args) != 2 {
			return errors.New("credential delete requires account")
		}
		return credential.Delete(ctx, args[1])
	default:
		return fmt.Errorf("unknown credential operation %q", args[0])
	}
}
