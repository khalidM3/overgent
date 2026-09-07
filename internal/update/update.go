// Package update verifies signed release metadata and performs recoverable
// executable replacement. It never trusts a checksum that was not covered by
// the release signature.
package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	manifestLimit = 1 << 20
	artifactLimit = 250 << 20
)

var (
	moveExecutableFile = moveFile
	syncExecutableDir  = syncDirectory
)

type Manifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Version       string           `json:"version"`
	PublishedAt   string           `json:"publishedAt"`
	Assets        map[string]Asset `json:"assets"`
	Signature     string           `json:"signature"`
}

type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type unsignedManifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Version       string           `json:"version"`
	PublishedAt   string           `json:"publishedAt"`
	Assets        map[string]Asset `json:"assets"`
}

type Client struct {
	PublicKey ed25519.PublicKey
	HTTP      *http.Client
	GOOS      string
	GOARCH    string
}

type Result struct {
	Version      string `json:"version"`
	PreviousPath string `json:"previousPath,omitempty"`
	Updated      bool   `json:"updated"`
}

func ParsePublicKey(encoded string) (ed25519.PublicKey, error) {
	bytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(bytes) != ed25519.PublicKeySize {
		return nil, errors.New("release update public key is not configured")
	}
	return ed25519.PublicKey(bytes), nil
}

func (c Client) FetchManifest(ctx context.Context, rawURL string) (Manifest, error) {
	parsed, err := secureURL(rawURL)
	if err != nil {
		return Manifest{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("create update metadata request: %w", err)
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return Manifest{}, fmt.Errorf("fetch update metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("fetch update metadata: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, manifestLimit+1))
	if err != nil {
		return Manifest{}, fmt.Errorf("read update metadata: %w", err)
	}
	if len(body) > manifestLimit {
		return Manifest{}, errors.New("update metadata exceeds limit")
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode update metadata: %w", err)
	}
	if err = Verify(manifest, c.PublicKey); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Verify(manifest Manifest, publicKey ed25519.PublicKey) error {
	if manifest.SchemaVersion != 1 || !validVersion(manifest.Version) || len(manifest.Assets) == 0 || len(manifest.Assets) > 12 {
		return errors.New("invalid update metadata")
	}
	if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil {
		return errors.New("invalid update publish time")
	}
	for key, asset := range manifest.Assets {
		if key == "" || asset.Size < 1 || asset.Size > artifactLimit || len(asset.SHA256) != sha256.Size*2 {
			return errors.New("invalid update asset")
		}
		if _, err := hex.DecodeString(asset.SHA256); err != nil {
			return errors.New("invalid update asset checksum")
		}
		if _, err := secureURL(asset.URL); err != nil {
			return fmt.Errorf("invalid update asset URL: %w", err)
		}
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("update metadata signature is unavailable")
	}
	payload, err := SigningPayload(manifest)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return errors.New("update metadata signature verification failed")
	}
	return nil
}

func SigningPayload(manifest Manifest) ([]byte, error) {
	return json.Marshal(unsignedManifest{SchemaVersion: manifest.SchemaVersion, Version: manifest.Version, PublishedAt: manifest.PublishedAt, Assets: manifest.Assets})
}

func (c Client) Apply(ctx context.Context, manifest Manifest, executable string) (Result, error) {
	if err := Verify(manifest, c.PublicKey); err != nil {
		return Result{}, err
	}
	goos, goarch := c.GOOS, c.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	asset, ok := manifest.Assets[goos+"_"+goarch]
	if !ok {
		return Result{}, fmt.Errorf("release %s has no asset for %s/%s", manifest.Version, goos, goarch)
	}
	if !filepath.IsAbs(executable) {
		return Result{}, errors.New("current executable path must be absolute")
	}
	if err := requireRegularFile(executable, "current executable"); err != nil {
		return Result{}, err
	}
	directory := filepath.Dir(executable)
	archive, err := os.CreateTemp(directory, ".overgent-update-*.archive")
	if err != nil {
		return Result{}, fmt.Errorf("stage update archive: %w", err)
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)
	defer archive.Close()
	if err = c.download(ctx, asset, archive); err != nil {
		return Result{}, err
	}
	if err = archive.Close(); err != nil {
		return Result{}, fmt.Errorf("close update archive: %w", err)
	}
	staged, err := extractExecutable(archivePath, directory, asset.URL)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(staged)
	if err = os.Chmod(staged, 0o755); err != nil {
		return Result{}, fmt.Errorf("secure staged executable: %w", err)
	}
	previous := executable + ".previous"
	rollbackBackup, err := preserveExisting(previous)
	if err != nil {
		return Result{}, fmt.Errorf("preserve existing rollback executable: %w", err)
	}
	if err = moveExecutableFile(executable, previous); err != nil {
		return Result{}, joinRestoreError(fmt.Errorf("preserve current executable: %w", err), restoreExisting(rollbackBackup, previous))
	}
	if err = moveExecutableFile(staged, executable); err != nil {
		return Result{}, restoreFailedActivation(fmt.Errorf("activate update: %w", err), executable, previous, rollbackBackup)
	}
	if err = syncExecutableDir(directory); err != nil {
		return Result{}, restoreFailedActivation(fmt.Errorf("sync activated update: %w", err), executable, previous, rollbackBackup)
	}
	removePreserved(rollbackBackup)
	return Result{Version: manifest.Version, PreviousPath: previous, Updated: true}, nil
}

func Rollback(executable string) (Result, error) {
	if !filepath.IsAbs(executable) {
		return Result{}, errors.New("current executable path must be absolute")
	}
	if err := requireRegularFile(executable, "current executable"); err != nil {
		return Result{}, err
	}
	previous := executable + ".previous"
	if err := requireRegularFile(previous, "rollback executable"); err != nil {
		return Result{}, fmt.Errorf("no rollback executable is available: %w", err)
	}
	failed, err := vacantPath(filepath.Dir(executable), ".overgent-failed-*.bin")
	if err != nil {
		return Result{}, fmt.Errorf("reserve failed executable path: %w", err)
	}
	if err = moveExecutableFile(executable, failed); err != nil {
		return Result{}, fmt.Errorf("preserve failed update: %w", err)
	}
	if err = moveExecutableFile(previous, executable); err != nil {
		return Result{}, joinRestoreError(fmt.Errorf("restore previous executable: %w", err), moveExecutableFile(failed, executable))
	}
	if err = syncExecutableDir(filepath.Dir(executable)); err != nil {
		// The rollback is already active, but retain the failed image so a
		// caller can recover it if the directory flush did not persist.
		return Result{}, fmt.Errorf("sync restored executable: %w", err)
	}
	_ = os.Remove(failed) // A running Windows image may remain until process exit.
	return Result{PreviousPath: previous, Updated: true}, nil
}

func requireRegularFile(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", label)
	}
	return nil
}

func preserveExisting(path string) (string, error) {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	if err := requireRegularFile(path, "existing rollback executable"); err != nil {
		return "", err
	}
	backup, err := vacantPath(filepath.Dir(path), ".overgent-rollback-*.bin")
	if err != nil {
		return "", err
	}
	if err = moveExecutableFile(path, backup); err != nil {
		return "", err
	}
	return backup, nil
}

func vacantPath(directory, pattern string) (string, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	name := file.Name()
	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(name)
		return "", closeErr
	}
	if err = os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func restoreExisting(backup, destination string) error {
	if backup == "" {
		return nil
	}
	return moveExecutableFile(backup, destination)
}

func removePreserved(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

func restoreFailedActivation(primary error, executable, previous, rollbackBackup string) error {
	failed := ""
	var preserveFailedErr error
	if _, err := os.Lstat(executable); err == nil {
		failed, preserveFailedErr = vacantPath(filepath.Dir(executable), ".overgent-failed-*.bin")
		if preserveFailedErr == nil {
			preserveFailedErr = moveExecutableFile(executable, failed)
		}
	} else if !os.IsNotExist(err) {
		preserveFailedErr = err
	}
	restoreCurrentErr := moveExecutableFile(previous, executable)
	var restorePreviousErr error
	if restoreCurrentErr == nil {
		restorePreviousErr = restoreExisting(rollbackBackup, previous)
	}
	if failed != "" && preserveFailedErr == nil {
		_ = os.Remove(failed)
	}
	return errors.Join(primary, preserveFailedErr, restoreCurrentErr, restorePreviousErr)
}

func joinRestoreError(primary, restore error) error {
	return errors.Join(primary, restore)
}

func (c Client) download(ctx context.Context, asset Asset, destination io.Writer) error {
	parsed, _ := secureURL(asset.URL)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("create update download request: %w", err)
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download update: HTTP %d", response.StatusCode)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(response.Body, asset.Size+1))
	if err != nil {
		return fmt.Errorf("write update archive: %w", err)
	}
	if written != asset.Size {
		return fmt.Errorf("update asset size mismatch: got %d want %d", written, asset.Size)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.SHA256) {
		return errors.New("update asset checksum verification failed")
	}
	return nil
}

func extractExecutable(archivePath, directory, rawURL string) (string, error) {
	staged, err := os.CreateTemp(directory, ".overgent-update-*.bin")
	if err != nil {
		return "", fmt.Errorf("stage update executable: %w", err)
	}
	path := staged.Name()
	defer func() { _ = staged.Close() }()
	if strings.HasSuffix(strings.ToLower(newURLPath(rawURL)), ".zip") {
		reader, err := zip.OpenReader(archivePath)
		if err != nil {
			os.Remove(path)
			return "", fmt.Errorf("open update zip: %w", err)
		}
		defer reader.Close()
		for _, file := range reader.File {
			if filepath.Base(file.Name) != "overgent.exe" || file.FileInfo().IsDir() {
				continue
			}
			input, err := file.Open()
			if err != nil {
				os.Remove(path)
				return "", err
			}
			written, copyErr := io.Copy(staged, io.LimitReader(input, artifactLimit+1))
			_ = input.Close()
			if copyErr != nil {
				os.Remove(path)
				return "", copyErr
			}
			if written < 1 || written > artifactLimit {
				os.Remove(path)
				return "", errors.New("update executable exceeds limit")
			}
			if err = staged.Sync(); err != nil {
				os.Remove(path)
				return "", fmt.Errorf("sync staged update executable: %w", err)
			}
			return path, nil
		}
	} else {
		input, err := os.Open(archivePath)
		if err != nil {
			os.Remove(path)
			return "", err
		}
		defer input.Close()
		gzipReader, err := gzip.NewReader(input)
		if err != nil {
			os.Remove(path)
			return "", fmt.Errorf("open update archive: %w", err)
		}
		defer gzipReader.Close()
		tarReader := tar.NewReader(gzipReader)
		for {
			header, err := tarReader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				os.Remove(path)
				return "", fmt.Errorf("read update archive: %w", err)
			}
			if filepath.Base(header.Name) != "overgent" || header.Typeflag != tar.TypeReg {
				continue
			}
			if header.Size < 1 || header.Size > artifactLimit {
				os.Remove(path)
				return "", errors.New("update executable exceeds limit")
			}
			if _, err = io.Copy(staged, io.LimitReader(tarReader, header.Size)); err != nil {
				os.Remove(path)
				return "", err
			}
			if err = staged.Sync(); err != nil {
				os.Remove(path)
				return "", fmt.Errorf("sync staged update executable: %w", err)
			}
			return path, nil
		}
	}
	os.Remove(path)
	return "", errors.New("update archive does not contain the Overgent executable")
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func secureURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("update URL is invalid")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	loopback := host == "localhost" || ip != nil && ip.IsLoopback()
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return nil, errors.New("update URL requires HTTPS")
	}
	return parsed, nil
}
func newURLPath(raw string) string {
	parsed, _ := url.Parse(raw)
	if parsed == nil {
		return raw
	}
	return parsed.Path
}
func validVersion(value string) bool {
	if len(value) < 2 || len(value) > 64 || value[0] != 'v' {
		return false
	}
	for _, char := range value[1:] {
		if !(char == '.' || char == '-' || char == '+' || char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z') {
			return false
		}
	}
	return true
}
