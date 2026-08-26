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
	archivePath := filepath.Join(directory, "stickguy.tar.gz")
	archive, _ := os.Create(archivePath)
	compressed := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(compressed)
	next := []byte("next executable")
	if err = tarWriter.WriteHeader(&tar.Header{Name: "stickguy", Mode: 0o755, Size: int64(len(next)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = tarWriter.Write(next)
	_ = tarWriter.Close()
	_ = compressed.Close()
	_ = archive.Close()
	archiveBytes, _ := os.ReadFile(archivePath)
	sum := sha256.Sum256(archiveBytes)
	manifest := Manifest{SchemaVersion: 1, Version: "v1.2.3", PublishedAt: time.Now().UTC().Format(time.RFC3339), Assets: map[string]Asset{"darwin_arm64": {URL: "https://releases.example/stickguy.tar.gz", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(archiveBytes))}}}
	payload, _ := SigningPayload(manifest)
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	executable := filepath.Join(directory, "stickguy")
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestManifestTamperingAndInsecureURLFail(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	manifest := Manifest{SchemaVersion: 1, Version: "v1.0.0", PublishedAt: time.Now().UTC().Format(time.RFC3339), Assets: map[string]Asset{"darwin_arm64": {URL: "https://example.com/stickguy.tar.gz", SHA256: string(make([]byte, 64)), Size: 10}}}
	payload, _ := SigningPayload(manifest)
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := Verify(manifest, publicKey); err == nil {
		t.Fatal("invalid checksum accepted")
	}
	manifest.Assets["darwin_arm64"] = Asset{URL: "http://example.com/stickguy.tar.gz", SHA256: hex.EncodeToString(make([]byte, 32)), Size: 10}
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
