//go:build linux

package localbackend

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The kernel records a process's comm in sixteen bytes including the
// terminator, so ps reports "convex-local-backend" as "convex-local-ba". An
// identity check built on ps therefore refuses to recognise the backend by its
// own name, killStale declines to signal a process this profile started, and
// the stale backend keeps holding its port while the next start moves to a new
// one. The name here is deliberately longer than fifteen characters, because a
// shorter one passes under either implementation and proves nothing.
func TestProcessMatchesRecognisesANameLongerThanTheKernelCommField(t *testing.T) {
	const name = "convex-local-backend"
	if len(name) <= 15 {
		t.Fatalf("this regression needs a name longer than the kernel comm field, got %d characters", len(name))
	}

	source, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("no sleep binary to stand in for the backend: %v", err)
	}
	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), name)
	if err = os.WriteFile(binary, body, 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(binary, "30")
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	})
	pid := command.Process.Pid

	if !processMatches(pid, name) {
		t.Fatalf("processMatches did not recognise pid %d as %q; a stale backend would be left running", pid, name)
	}
	// The check must stay narrow enough to refuse a recycled pid that now
	// belongs to something else, which is the reason it exists at all.
	if processMatches(pid, "something-else") {
		t.Fatal("processMatches accepted a process running a different command")
	}
	if processMatches(pid, name[:15]) {
		t.Fatal("processMatches accepted the truncated comm name, so it is still comparing against ps output")
	}
}
