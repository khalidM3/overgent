//go:build !darwin && !linux && !windows

package main

import (
	"fmt"
	"os"
)

// The shell needs a native webview, and Wails v3 provides one for exactly three
// platforms. Anything else - a BSD, a musl target without WebKitGTK, wasm -
// gets this rather than a link error, so `go build` for such a target still
// says something a person can act on.
func main() {
	_, _ = fmt.Fprintln(os.Stderr, "Overgent desktop needs macOS, Linux with WebKitGTK, or Windows with WebView2")
	os.Exit(1)
}
