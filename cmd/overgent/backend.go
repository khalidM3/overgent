package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/localbackend"
)

// runBackend implements `overgent backend …`, the headless half of local mode.
//
// Every subcommand prefers the running service over acting directly: the
// service owns the backend process, so asking it is the difference between
// stopping the backend and stopping a backend something is still supervising.
// When no service answers - a fresh profile, a CLI-only install - the command
// falls back to a manager of its own.
func runBackend(ctx context.Context, paths config.Paths, args []string) error {
	if len(args) == 0 {
		return errors.New("backend requires status, start, stop, install, verify, reset, or export")
	}
	switch args[0] {
	case "status":
		statusFlags := flag.NewFlagSet("backend status", flag.ContinueOnError)
		asJSON := statusFlags.Bool("json", false, "print the status as JSON")
		if err := statusFlags.Parse(args[1:]); err != nil {
			return err
		}
		status, err := backendStatus(ctx, paths)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(status)
		}
		return printBackendStatus(status)
	case "start":
		endpoint, err := backendEnsure(ctx, paths)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(endpoint)
	case "stop":
		if response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "backend_stop"}); err == nil {
			if !response.OK {
				return errors.New(response.Error)
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"running": false})
		}
		manager, err := newBackendManager(paths)
		if err != nil {
			return err
		}
		if err = manager.Stop(ctx); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"running": false})
	case "install":
		installFlags := flag.NewFlagSet("backend install", flag.ContinueOnError)
		binary := installFlags.String("binary", "", "path to the convex-local-backend executable")
		bundle := installFlags.String("bundle", "", "path to the release-time deploy payload")
		if err := installFlags.Parse(args[1:]); err != nil {
			return err
		}
		if err := localbackend.Install(paths.Root, *binary, *bundle); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"installed": true, "configRoot": paths.Root})
	case "reset":
		resetFlags := flag.NewFlagSet("backend reset", flag.ContinueOnError)
		yes := resetFlags.Bool("yes", false, "skip the confirmation prompt")
		if err := resetFlags.Parse(args[1:]); err != nil {
			return err
		}
		if !*yes && !confirmBackendReset() {
			return errors.New("backend reset cancelled")
		}
		// The service holds the process. Ask it to let go first, so the reset
		// is not racing a supervisor that will restart what it just deleted.
		if response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "backend_stop"}); err == nil && !response.OK {
			return errors.New(response.Error)
		}
		manager, err := newBackendManager(paths)
		if err != nil {
			return err
		}
		if err = manager.Reset(ctx); err != nil {
			return err
		}
		// The Projects on that database are gone, and so is the device row the
		// stored credential authenticates against, so the enrollment naming
		// them is now a reference to nothing. Clearing it here is what makes
		// "reset, then relaunch" return to first run instead of showing an
		// enrolled Project whose backend has never heard of it. A team-mode
		// profile is left alone: its Projects live on a server this command
		// did not touch.
		cleared, err := clearLocalEnrollment(ctx, paths)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"reset": true, "configRoot": paths.Root, "clearedWorkspaces": cleared})
	case "verify":
		// The release gate: replay the payload that is about to ship against a
		// fresh backend, so a broken deploy2 pin fails the release rather than
		// an install.
		verifyFlags := flag.NewFlagSet("backend verify", flag.ContinueOnError)
		binary := verifyFlags.String("binary", "", "path to the convex-local-backend executable")
		bundle := verifyFlags.String("bundle", "", "path to the release-time deploy payload")
		if err := verifyFlags.Parse(args[1:]); err != nil {
			return err
		}
		if err := localbackend.Verify(ctx, *binary, *bundle); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"verified": true})
	case "export":
		exportFlags := flag.NewFlagSet("backend export", flag.ContinueOnError)
		out := exportFlags.String("out", "", "directory to copy the stopped backend database into")
		if err := exportFlags.Parse(args[1:]); err != nil {
			return err
		}
		manager, err := newBackendManager(paths)
		if err != nil {
			return err
		}
		// The copy is taken with the backend stopped: a live SQLite file copied
		// underneath a running writer is not a database, it is a guess.
		if response, callErr := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "backend_stop"}); callErr == nil && !response.OK {
			return errors.New(response.Error)
		} else if callErr != nil {
			if err = manager.Stop(ctx); err != nil {
				return err
			}
		}
		path, err := manager.Export(*out)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"database": path})
	}
	return errors.New("backend requires status, start, stop, install, verify, reset, or export")
}

func backendStatus(ctx context.Context, paths config.Paths) (localbackend.Status, error) {
	if response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "backend_status"}); err == nil {
		if !response.OK {
			return localbackend.Status{}, errors.New(response.Error)
		}
		encoded, marshalErr := json.Marshal(response.Data)
		if marshalErr != nil {
			return localbackend.Status{}, marshalErr
		}
		var status localbackend.Status
		if err = json.Unmarshal(encoded, &status); err != nil {
			return localbackend.Status{}, err
		}
		return status, nil
	}
	manager, err := newBackendManager(paths)
	if err != nil {
		return localbackend.Status{}, err
	}
	return manager.Status(ctx), nil
}

func backendEnsure(ctx context.Context, paths config.Paths) (localbackend.Endpoint, error) {
	if response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "backend_ensure"}); err == nil {
		if !response.OK {
			return localbackend.Endpoint{}, errors.New(response.Error)
		}
		encoded, marshalErr := json.Marshal(response.Data)
		if marshalErr != nil {
			return localbackend.Endpoint{}, marshalErr
		}
		var endpoint localbackend.Endpoint
		if err = json.Unmarshal(encoded, &endpoint); err != nil {
			return localbackend.Endpoint{}, err
		}
		return endpoint, nil
	}
	manager, err := newBackendManager(paths)
	if err != nil {
		return localbackend.Endpoint{}, err
	}
	return manager.Ensure(ctx)
}

func newBackendManager(paths config.Paths) (*localbackend.Manager, error) {
	if !localbackend.Configured(paths.Root) {
		return nil, errors.New("this profile has no local backend; run overgent backend install --binary <path> --bundle <path>")
	}
	return localbackend.New(paths.Root, localbackend.Keychain{}, slog.Default())
}

func printBackendStatus(status localbackend.Status) error {
	state := "stopped"
	if status.Running {
		state = "running"
	}
	fmt.Printf("Backend: %s\n", state)
	if status.Origin != "" {
		fmt.Printf("Origin: %s\nSite: %s\n", status.Origin, status.SiteOrigin)
	}
	if status.Version != "" {
		fmt.Printf("Release: %s\nBundle: %s\n", status.Version, status.BundleRevision)
	}
	fmt.Printf("Database: %s (%d bytes)\n", status.DatabasePath, status.DatabaseBytes)
	if status.LastError != "" {
		fmt.Printf("Last error: %s\n", status.LastError)
	}
	return nil
}

// confirmBackendReset asks before deleting the only copy of a local Project's
// coordination history. `--yes` is the scripted path.
func confirmBackendReset() bool {
	fmt.Print("Delete this Mac's local backend database and file storage? Local Projects on it are not recoverable. [y/N]: ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(answer), "y")
}

// clearLocalEnrollment forgets a profile's local Projects after its backend
// database has been deleted, and returns how many it forgot.
//
// It does not go through onboarding.Reset, whose gate is "the hosted API
// actually rejected this credential". That gate protects a member from erasing
// a working enrollment because a remote server answered 401 once. Here there is
// nothing to protect: the database those Projects lived in was deleted by the
// command that is calling this.
func clearLocalEnrollment(ctx context.Context, paths config.Paths) (int, error) {
	cfg, err := config.Load(paths)
	if err != nil {
		return 0, err
	}
	if cfg.DeviceID == "" || !localbackend.IsLoopbackOrigin(cfg.APIBaseURL) {
		return 0, nil
	}
	cleared := len(cfg.Workspaces)
	deviceID := cfg.DeviceID
	cfg.Workspaces = nil
	cfg.DeviceID = ""
	cfg.APIBaseURL = ""
	if err = config.Save(paths, cfg); err != nil {
		return 0, err
	}
	// A credential for a device that no longer exists is not worth keeping, and
	// a failure to delete it must not fail a reset that already succeeded.
	_ = credential.Delete(ctx, deviceID)
	return cleared, nil
}
