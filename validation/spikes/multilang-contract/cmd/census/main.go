// Command census reports how much of a repository the current production
// extractor actually fingerprints. It is evidence for the spike, not a tool:
// the coverage question ("how many observed paths carry a contract at all")
// is what decides whether adding languages is worth the integration.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stickguy/stickguy/internal/contract"
)

type bucket struct {
	Extension     string `json:"extension"`
	Files         int    `json:"files"`
	Fingerprinted int    `json:"fingerprinted"`
	Bytes         int64  `json:"bytes"`
	Symbols       int    `json:"symbols"`
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	buckets := map[string]*bucket{}
	var failures []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			if entry != nil && entry.IsDir() && skipDir(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == "" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		item, ok := buckets[extension]
		if !ok {
			item = &bucket{Extension: extension}
			buckets[extension] = item
		}
		item.Files++
		item.Bytes += info.Size()
		if !contract.Fingerprintable(path) {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		file, ok := contract.Extract(path, source, nil)
		if !ok {
			failures = append(failures, path)
			return nil
		}
		item.Fingerprinted++
		item.Symbols += len(file.Symbols)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ordered := make([]*bucket, 0, len(buckets))
	for _, item := range buckets {
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Files > ordered[j].Files })
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(map[string]any{"buckets": ordered, "fingerprintableFailures": failures})
}

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", ".next", "vendor", "_gen":
		return true
	// The spike's own testdata would otherwise be counted as repository source.
	case "multilang-contract":
		return true
	}
	return false
}
