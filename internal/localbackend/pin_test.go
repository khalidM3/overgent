package localbackend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

// The deploy2 replay is only safe while the backend release, the CLI version
// that recorded the payload, and this code are one pin. This is that check.
func TestPinMatchesThePackagingManifest(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "backend-version.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version    string            `json:"version"`
		CLIVersion string            `json:"cliVersion"`
		SHA256     map[string]string `json:"sha256"`
	}
	if err = json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != backendRelease {
		t.Fatalf("backend release drifted: manifest %q, Go %q", manifest.Version, backendRelease)
	}
	if manifest.CLIVersion != backendCLI {
		t.Fatalf("Convex CLI version drifted: manifest %q, Go %q", manifest.CLIVersion, backendCLI)
	}
	if convexClientVersion != "npm-cli-"+backendCLI {
		t.Fatalf("Convex-Client header %q does not name the pinned CLI", convexClientVersion)
	}

	// The platform half of the same pin. A hash in the manifest with no Go
	// platform is a machine the Go side will never admit to supporting; a Go
	// platform with no hash is a machine fetch-backend.mjs cannot download for.
	// Either way the two halves have drifted.
	if len(manifest.SHA256) != len(platforms) {
		t.Fatalf("the manifest pins %d platforms, Go knows %d", len(manifest.SHA256), len(platforms))
	}
	hexadecimal := regexp.MustCompile(`^[a-f0-9]{64}$`)
	seen := map[string]bool{}
	for _, candidate := range platforms {
		hash, ok := manifest.SHA256[candidate.Key]
		if !ok {
			t.Fatalf("the manifest has no backend hash for %s (%s/%s)", candidate.Key, candidate.GOOS, candidate.GOARCH)
		}
		if !hexadecimal.MatchString(hash) {
			t.Fatalf("the backend hash for %s is not a SHA-256: %q", candidate.Key, hash)
		}
		if seen[candidate.GOOS+"/"+candidate.GOARCH] {
			t.Fatalf("two platforms claim %s/%s", candidate.GOOS, candidate.GOARCH)
		}
		seen[candidate.GOOS+"/"+candidate.GOARCH] = true
	}
	// The release publishes no aarch64-pc-windows-msvc asset, so nothing may
	// quietly start claiming a bundled backend for Windows on ARM.
	if seen["windows/arm64"] {
		t.Fatal("windows/arm64 claims a bundled backend the release does not publish")
	}
	// The Windows archive contains convex-local-backend.exe. Every path built
	// to the binary has to agree with that or the file is simply not found.
	if binaryNameFor("windows") != "convex-local-backend.exe" {
		t.Fatalf("the Windows backend binary is named %q", binaryNameFor("windows"))
	}
	if binaryNameFor("linux") != "convex-local-backend" || binaryNameFor("darwin") != "convex-local-backend" {
		t.Fatal("the Unix backend binary must carry no suffix")
	}
	if !Supported() {
		t.Fatalf("this test is running on %s/%s, which claims no bundled backend", runtime.GOOS, runtime.GOARCH)
	}
}
