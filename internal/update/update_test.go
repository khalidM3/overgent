package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSignedUpdateAndRollback(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	archivePath := filepath.Join(directory, "overgent.tar.gz")
	archive, _ := os.Create(archivePath)
	compressed := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(compressed)
	next := []byte("next executable")
	if err = tarWriter.WriteHeader(&tar.Header{Name: "overgent", Mode: 0o755, Size: int64(len(next)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = tarWriter.Write(next)
	_ = tarWriter.Close()
	_ = compressed.Close()
	_ = archive.Close()
	archiveBytes, _ := os.ReadFile(archivePath)
	sum := sha256.Sum256(archiveBytes)
	manifest := Manifest{SchemaVersion: 1, Version: "v1.2.3", PublishedAt: time.Now().UTC().Format(time.RFC3339), Assets: map[string]Asset{"darwin_arm64": {URL: "https://releases.example/overgent.tar.gz", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(archiveBytes))}}}
	payload, _ := SigningPayload(manifest)
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	executable := filepath.Join(directory, "overgent")
	if err = os.WriteFile(executable, []byte("old executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := Client{PublicKey: publicKey, GOOS: "darwin", GOARCH: "arm64", HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(archiveBytes))), Header: make(http.Header)}, nil
	})}}
	if _, err = client.Apply(context.Background(), manifest, executable); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(executable); string(got) != string(next) {
		t.Fatalf("updated executable=%q", got)
	}
	if _, err = Rollback(executable); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(executable); string(got) != "old executable" {
		t.Fatalf("rolled back executable=%q", got)
	}
}

func TestFailedActivationRestoresCurrentAndExistingRollback(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	directory := t.TempDir()
	archiveBytes := tarGzipExecutable(t, []byte("next executable"))
	sum := sha256.Sum256(archiveBytes)
	manifest := Manifest{SchemaVersion: 1, Version: "v2.0.0", PublishedAt: time.Now().UTC().Format(time.RFC3339), Assets: map[string]Asset{
		"linux_amd64": {URL: "https://releases.example/overgent.tar.gz", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(archiveBytes))},
	}}
	payload, _ := SigningPayload(manifest)
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	executable := filepath.Join(directory, "overgent")
	if err := os.WriteFile(executable, []byte("current executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable+".previous", []byte("older rollback"), 0o755); err != nil {
		t.Fatal(err)
	}

	originalMove := moveExecutableFile
	moveExecutableFile = func(source, destination string) error {
		if destination == executable && strings.Contains(filepath.Base(source), ".overgent-update-") {
			return errors.New("injected activation failure")
		}
		return originalMove(source, destination)
	}
	t.Cleanup(func() { moveExecutableFile = originalMove })
	client := Client{PublicKey: publicKey, GOOS: "linux", GOARCH: "amd64", HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(archiveBytes))), Header: make(http.Header)}, nil
	})}}
	if _, err := client.Apply(context.Background(), manifest, executable); err == nil || !strings.Contains(err.Error(), "activate update") {
		t.Fatalf("Apply error=%v", err)
	}
	if got, _ := os.ReadFile(executable); string(got) != "current executable" {
		t.Fatalf("current executable after rollback=%q", got)
	}
	if got, _ := os.ReadFile(executable + ".previous"); string(got) != "older rollback" {
		t.Fatalf("prior rollback after failure=%q", got)
	}
}

func TestSyncFailureRollsBackActivatedExecutable(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	directory := t.TempDir()
	archiveBytes := tarGzipExecutable(t, []byte("next"))
	sum := sha256.Sum256(archiveBytes)
	manifest := Manifest{SchemaVersion: 1, Version: "v2.0.0", PublishedAt: time.Now().UTC().Format(time.RFC3339), Assets: map[string]Asset{
		"linux_amd64": {URL: "https://releases.example/overgent.tar.gz", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(archiveBytes))},
	}}
	payload, _ := SigningPayload(manifest)
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	executable := filepath.Join(directory, "overgent")
	if err := os.WriteFile(executable, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	originalSync := syncExecutableDir
	syncExecutableDir = func(string) error { return errors.New("injected sync failure") }
	t.Cleanup(func() { syncExecutableDir = originalSync })
	client := Client{PublicKey: publicKey, GOOS: "linux", GOARCH: "amd64", HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(archiveBytes))), Header: make(http.Header)}, nil
	})}}
	_, err := client.Apply(context.Background(), manifest, executable)
	if err == nil || !strings.Contains(err.Error(), "sync activated update") {
		t.Fatalf("rollback error=%v", err)
	}
	if got, _ := os.ReadFile(executable); string(got) != "current" {
		t.Fatalf("current executable after sync rollback=%q", got)
	}
}

func TestFailedRestoreKeepsBothRecoveryExecutables(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "overgent")
	previous := executable + ".previous"
	backup := filepath.Join(directory, "older-rollback")
	if err := os.WriteFile(previous, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("older"), 0o755); err != nil {
		t.Fatal(err)
	}
	originalMove := moveExecutableFile
	moveExecutableFile = func(source, destination string) error {
		if source == previous && destination == executable {
			return errors.New("injected restore failure")
		}
		return originalMove(source, destination)
	}
	t.Cleanup(func() { moveExecutableFile = originalMove })
	err := restoreFailedActivation(errors.New("activation failed"), executable, previous, backup)
	if err == nil || !strings.Contains(err.Error(), "injected restore failure") {
		t.Fatalf("restore error=%v", err)
	}
	if got, _ := os.ReadFile(previous); string(got) != "current" {
		t.Fatalf("recoverable current executable was overwritten: %q", got)
	}
	if got, _ := os.ReadFile(backup); string(got) != "older" {
		t.Fatalf("older rollback was overwritten: %q", got)
	}
}

func tarGzipExecutable(t *testing.T, contents []byte) []byte {
	t.Helper()
	var output strings.Builder
	compressed := gzip.NewWriter(&output)
	archive := tar.NewWriter(compressed)
	if err := archive.WriteHeader(&tar.Header{Name: "overgent", Mode: 0o755, Size: int64(len(contents)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(output.String())
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestManifestTamperingAndInsecureURLFail(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	manifest := Manifest{SchemaVersion: 1, Version: "v1.0.0", PublishedAt: time.Now().UTC().Format(time.RFC3339), Assets: map[string]Asset{"darwin_arm64": {URL: "https://example.com/overgent.tar.gz", SHA256: string(make([]byte, 64)), Size: 10}}}
	payload, _ := SigningPayload(manifest)
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := Verify(manifest, publicKey); err == nil {
		t.Fatal("invalid checksum accepted")
	}
	manifest.Assets["darwin_arm64"] = Asset{URL: "http://example.com/overgent.tar.gz", SHA256: hex.EncodeToString(make([]byte, 32)), Size: 10}
	payload, _ = SigningPayload(manifest)
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := Verify(manifest, publicKey); err == nil {
		t.Fatal("insecure update URL accepted")
	}
}

func TestPublicKeyParsing(t *testing.T) {
	publicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := ParsePublicKey(base64.StdEncoding.EncodeToString(publicKey)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKey("placeholder"); err == nil {
		t.Fatal("placeholder key accepted")
	}
}
