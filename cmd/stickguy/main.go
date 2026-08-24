package main

import (
	"encoding/json"
	"fmt"
	"os"
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
	if len(os.Args) == 3 && os.Args[1] == "version" && os.Args[2] == "--json" {
		if err := json.NewEncoder(os.Stdout).Encode(versionInfo{
			Version: version, Commit: commit, BuildTime: buildTime,
			SchemaMinimum: 1, SchemaMaximum: 1,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "encode version:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintln(os.Stderr, "usage: stickguy version --json")
	os.Exit(2)
}
