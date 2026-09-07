package localbackend

import "runtime"

// The bundled-backend pin.
//
// The deploy2 request shape is an internal Convex detail rather than a
// promised API, so it is only safe while the backend
// release, the CLI version that produced the recorded payload, and the Go
// replay above are one pin. These constants are the Go half of that pin;
// scripts/backend-version.json is the packaging half, and pin_test.go fails if
// the two ever drift.
const (
	backendRelease = "precompiled-2026-08-25-7cce8fb"
	backendCLI     = "1.45.0"
)

// platform is one machine the pinned release publishes a backend binary for.
// Key is how scripts/backend-version.json names it (Node's
// `${process.platform}-${process.arch}`, which is what fetch-backend.mjs sees);
// GOOS and GOARCH are how Go names the same machine.
type platform struct {
	Key    string
	GOOS   string
	GOARCH string
}

// platforms is the Go half of the fetch script's target map. It exists so the
// pin test can prove the two halves cover exactly the same machines: a hash
// added to the manifest without a Go platform, or a platform added here that
// nothing can download, is drift of the same kind the release/CLI constants
// above guard against.
//
// Windows on ARM is absent because the pinned release publishes no
// aarch64-pc-windows-msvc asset; GoReleaser excludes windows/arm64 from the CLI
// matrix for the same reason.
var platforms = []platform{
	{Key: "darwin-arm64", GOOS: "darwin", GOARCH: "arm64"},
	{Key: "darwin-x64", GOOS: "darwin", GOARCH: "amd64"},
	{Key: "linux-x64", GOOS: "linux", GOARCH: "amd64"},
	{Key: "linux-arm64", GOOS: "linux", GOARCH: "arm64"},
	{Key: "win32-x64", GOOS: "windows", GOARCH: "amd64"},
}

// BinaryName is what the backend executable is called on this platform.
//
// Every path Overgent builds to the backend - the desktop's Resources
// directory, the CLI's install hint, the release workflow's fetch output - has
// to agree with what the archive actually contains, and the Windows archive
// contains convex-local-backend.exe. Nothing else about the binary changes.
func BinaryName() string { return binaryNameFor(runtime.GOOS) }

func binaryNameFor(goos string) string {
	if goos == "windows" {
		return "convex-local-backend.exe"
	}
	return "convex-local-backend"
}

// Supported reports whether this machine has a pinned backend binary at all. A
// machine without one (Windows on ARM) can still run Overgent against a hosted
// backend; it just cannot run local mode.
func Supported() bool {
	for _, candidate := range platforms {
		if candidate.GOOS == runtime.GOOS && candidate.GOARCH == runtime.GOARCH {
			return true
		}
	}
	return false
}
