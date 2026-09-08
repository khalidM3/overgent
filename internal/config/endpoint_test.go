package config

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Resolve used to refuse every platform but darwin, which meant no command at
// all could run elsewhere — not even the ones that never touch the service.
// The refusals now live where the capability actually stops (service,
// credential, daemon-on-Windows), so path resolution has to succeed here on
// every target Overgent builds for.
func TestResolveProducesAProfileOnEveryBuildTarget(t *testing.T) {
	root := t.TempDir()
	paths, err := Resolve(root)
	if err != nil {
		t.Fatalf("Resolve on %s: %v", runtime.GOOS, err)
	}
	for name, value := range map[string]string{
		"Root": paths.Root, "Config": paths.Config, "DB": paths.DB,
		"Lock": paths.Lock, "Socket": paths.Socket,
	} {
		if value == "" {
			t.Fatalf("%s is empty", name)
		}
	}
	// Lock stays a real file on every platform: it is what the Windows
	// implementation locks with LockFileEx, exactly as unix flocks it.
	if filepath.Dir(paths.Lock) != paths.Root {
		t.Fatalf("Lock %q is not inside the profile root %q", paths.Lock, paths.Root)
	}
}

// The endpoint is derived from the profile root rather than fixed, so a
// development profile under an isolated OVERGENT_CONFIG_ROOT and a production
// install cannot claim the same one. Only one of them could ever listen.
func TestEndpointIsDistinctPerProfileAndStable(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	a, err := Resolve(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Resolve(second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Socket == b.Socket {
		t.Fatalf("two profiles share one endpoint: %q", a.Socket)
	}
	again, err := Resolve(first)
	if err != nil {
		t.Fatal(err)
	}
	if again.Socket != a.Socket {
		t.Fatalf("endpoint is not stable: %q then %q", a.Socket, again.Socket)
	}
	if runtime.GOOS == "windows" {
		// A pipe is not a filesystem object, so the name must be in the pipe
		// namespace and must not carry separators or a drive letter.
		if !strings.HasPrefix(a.Socket, `\\.\pipe\`) {
			t.Fatalf("Windows endpoint %q is not a named pipe", a.Socket)
		}
		if strings.ContainsAny(strings.TrimPrefix(a.Socket, `\\.\pipe\`), `\/:`) {
			t.Fatalf("Windows pipe name %q contains path separators", a.Socket)
		}
	} else if filepath.Dir(a.Socket) != a.Root {
		t.Fatalf("unix endpoint %q is not inside the profile root %q", a.Socket, a.Root)
	}
}
