package localbackend

// The bundled-backend pin.
//
// Lane 01 established that the deploy2 request shape is an internal Convex
// detail rather than a promised API, so it is only safe while the backend
// release, the CLI version that produced the recorded payload, and the Go
// replay above are one pin. These constants are the Go half of that pin;
// scripts/backend-version.json is the packaging half, and pin_test.go fails if
// the two ever drift.
const (
	backendRelease = "precompiled-2026-08-25-7cce8fb"
	backendCLI     = "1.45.0"
)
