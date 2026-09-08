//go:build unix

package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// serveSlow starts a service whose handler takes longer than defaultCallTimeout
// and returns its endpoint.
func serveSlow(t *testing.T, work time.Duration) string {
	t.Helper()
	// Not t.TempDir(): it embeds the test's name, and a unix socket path is
	// capped at 104 bytes on macOS by sockaddr_un. These names are long enough
	// to cross that, and the bind fails with "invalid argument".
	dir, err := os.MkdirTemp("", "ogd")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		_ = Serve(ctx, socket, func(ctx context.Context, _ Request) Response {
			select {
			case <-time.After(work):
				return Response{OK: true}
			case <-ctx.Done():
				return Response{Error: "cancelled"}
			}
		})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := dial(context.Background(), socket); err == nil {
			return socket
		}
		if time.Now().After(deadline) {
			t.Fatal("the listener never came up")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A caller that knows its method is slow must be able to say so. Call used to
// take the minimum of the caller's deadline and its own three seconds, which
// made three seconds a ceiling nothing could raise: "backend_ensure" is allowed
// a ten-second health budget by the service, so the client gave up while the
// service was still doing exactly what it was asked.
func TestCallHonoursADeadlineLongerThanTheDefault(t *testing.T) {
	socket := serveSlow(t, 2*defaultCallTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	response, err := Call(ctx, socket, Request{Method: "slow"})
	if err != nil {
		t.Fatalf("Call gave up after %s on a caller deadline of 30s: %v", time.Since(start), err)
	}
	if !response.OK {
		t.Fatalf("handler refused the request: %q", response.Error)
	}
	if elapsed := time.Since(start); elapsed < defaultCallTimeout {
		t.Fatalf("Call returned in %s, before the handler could have finished; it did not wait", elapsed)
	}
}

// The default still applies to the callers that pass no deadline, which is most
// of the CLI. They want a wedged service to fail rather than hang.
func TestCallWithNoDeadlineStillGivesUpAtTheDefault(t *testing.T) {
	socket := serveSlow(t, time.Minute)

	start := time.Now()
	if _, err := Call(context.Background(), socket, Request{Method: "slow"}); err == nil {
		t.Fatal("Call waited out a handler that never answers")
	}
	if elapsed := time.Since(start); elapsed > defaultCallTimeout+2*time.Second {
		t.Fatalf("Call took %s to give up, want about %s", elapsed, defaultCallTimeout)
	}
}

// A deadline shorter than the default still wins, as it always did.
func TestCallHonoursADeadlineShorterThanTheDefault(t *testing.T) {
	socket := serveSlow(t, time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Call(ctx, socket, Request{Method: "slow"}); err == nil {
		t.Fatal("Call ignored a deadline shorter than its default")
	}
	if elapsed := time.Since(start); elapsed > defaultCallTimeout {
		t.Fatalf("Call took %s to give up, want about 500ms", elapsed)
	}
}
