package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/stickguy/stickguy/internal/app"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	"os"
	"os/signal"
	"syscall"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

type versionInfo struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildTime     string `json:"buildTime"`
	SchemaMinimum int    `json:"schemaMinimum"`
	SchemaMaximum int    `json:"schemaMaximum"`
}

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 2 && args[0] == "version" && args[1] == "--json" {
		return json.NewEncoder(os.Stdout).Encode(versionInfo{version, commit, buildTime, 1, 1})
	}
	fs := flag.NewFlagSet("stickguy", flag.ContinueOnError)
	root := fs.String("config-root", "", "isolated per-user state root")
	if e := fs.Parse(args); e != nil {
		return e
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("usage: stickguy [--config-root <dir>] service run|status | workspace add | pause|resume | doctor | scan")
	}
	if *root == "" {
		var err error
		*root, err = config.DefaultRoot()
		if err != nil {
			return err
		}
	}
	paths, e := config.Resolve(*root)
	if e != nil {
		return e
	}
	ctx := context.Background()
	switch rest[0] {
	case "service":
		if len(rest) != 2 {
			return errors.New("service requires run or status")
		}
		if rest[1] == "run" {
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			return app.Run(ctx, *root, nil)
		}
		if rest[1] == "status" {
			return printCall(ctx, paths.Socket, daemon.Request{Method: "health"})
		}
	case "workspace":
		if len(rest) < 2 || rest[1] != "add" {
			break
		}
		wf := flag.NewFlagSet("workspace add", flag.ContinueOnError)
		id := wf.String("id", "", "")
		project := wf.String("project", "", "")
		workstream := wf.String("workstream", "", "")
		repo := wf.String("root", "", "")
		if e = wf.Parse(rest[2:]); e != nil {
			return e
		}
		if *id == "" || *project == "" || *workstream == "" || *repo == "" {
			return errors.New("workspace add requires id, project, workstream, root")
		}
		return app.Register(ctx, *root, config.Workspace{ID: *id, ProjectID: *project, WorkstreamID: *workstream, Root: *repo})
	case "pause", "resume":
		pf := flag.NewFlagSet(rest[0], flag.ContinueOnError)
		id := pf.String("workspace", "", "")
		if e = pf.Parse(rest[1:]); e != nil {
			return e
		}
		if *id == "" {
			return errors.New("workspace id required")
		}
		return printCall(ctx, paths.Socket, daemon.Request{Method: rest[0], WorkspaceID: *id})
	case "doctor", "scan":
		return printCall(ctx, paths.Socket, daemon.Request{Method: rest[0]})
	}
	return errors.New("unsupported command")
}
func printCall(ctx context.Context, socket string, q daemon.Request) error {
	r, e := daemon.Call(ctx, socket, q)
	if e != nil {
		return e
	}
	if !r.OK {
		return errors.New(r.Error)
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}
