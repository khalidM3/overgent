// Command release-metadata creates the small signed manifest consumed by the
// updater. The signing key is read from a file descriptor environment path,
// never an argument or log field.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	updateclient "github.com/overgent/overgent/internal/update"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	directory := flag.String("dist", "dist", "GoReleaser output directory")
	baseURL := flag.String("base-url", "", "HTTPS release download base URL")
	version := flag.String("version", "", "release version including v prefix")
	output := flag.String("output", "dist/update-manifest.json", "manifest output path")
	flag.Parse()
	if *baseURL == "" || *version == "" {
		return errors.New("base-url and version are required")
	}
	keyPath := os.Getenv("OVERGENT_UPDATE_SIGNING_KEY_FILE")
	if keyPath == "" {
		return errors.New("OVERGENT_UPDATE_SIGNING_KEY_FILE is required")
	}
	encodedKey, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read update signing key: %w", err)
	}
	keyBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedKey)))
	if err != nil || len(keyBytes) != ed25519.PrivateKeySize {
		return errors.New("update signing key is invalid")
	}
	entries, err := os.ReadDir(*directory)
	if err != nil {
		return fmt.Errorf("read release directory: %w", err)
	}
	assets := map[string]updateclient.Asset{}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		key, ok := platformKey(name)
		if !ok {
			continue
		}
		path := filepath.Join(*directory, name)
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		assets[key] = updateclient.Asset{URL: strings.TrimRight(*baseURL, "/") + "/" + name, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size}
	}
	if len(assets) == 0 {
		return errors.New("no release archives found")
	}
	manifest := updateclient.Manifest{SchemaVersion: 1, Version: *version, PublishedAt: time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), Assets: assets}
	payload, err := updateclient.SigningPayload(manifest)
	if err != nil {
		return err
	}
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(keyBytes), payload))
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(*output, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write update manifest: %w", err)
	}
	return nil
}

func platformKey(name string) (string, bool) {
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, "overgent_") || !(strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".zip")) {
		return "", false
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			if strings.Contains(lower, "_"+goos+"_"+goarch) {
				return goos + "_" + goarch, true
			}
		}
	}
	return "", false
}
