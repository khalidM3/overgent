//go:build windows

package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/khalidM3/overgent/internal/config"
	"golang.org/x/sys/windows"
)

// endpoint resolves a real profile endpoint rather than inventing a pipe name,
// so these tests exercise the same string config.endpointFor hands the service.
// If the two halves ever disagree about the namespace, this is where it shows.
func endpoint(t *testing.T) string {
	t.Helper()
	paths, e := config.Resolve(t.TempDir())
	if e != nil {
		t.Fatalf("resolve profile: %v", e)
	}
	if !strings.HasPrefix(paths.Socket, `\\.\pipe\`) {
		t.Fatalf("endpoint %q is not a named pipe", paths.Socket)
	}
	return paths.Socket
}

// The whole point of the transport: a request encoded by Call reaches the
// handler and its response comes back. The unix side gets this for free from
// net.Listen("unix"); here it runs through winio's overlapped I/O, so it is
// worth asserting end to end rather than assuming.
func TestServeAnswersACallOverTheNamedPipe(t *testing.T) {
	name := endpoint(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	served := make(chan error, 1)
	go func() {
		served <- Serve(ctx, name, func(_ context.Context, q Request) Response {
			return Response{OK: true, Data: q.Method}
		})
	}()

	response, e := callWhenListening(t, ctx, name, Request{Method: "health"})
	if e != nil {
		t.Fatalf("call over %s: %v", name, e)
	}
	if !response.OK {
		t.Fatalf("handler refused the request: %q", response.Error)
	}
	if data, _ := response.Data.(string); data != "health" {
		t.Fatalf("response carried %#v, want the method back", response.Data)
	}

	// Cancelling the context is what stops the loop, and it must read as a
	// clean shutdown rather than an accept failure.
	cancel()
	select {
	case e := <-served:
		if e != nil {
			t.Fatalf("Serve returned %v after its context was cancelled, want nil", e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after its context was cancelled")
	}
}

// Serve listens asynchronously, so the first dial can legitimately land before
// the pipe exists. Retry until it does, then return whatever Call reports.
func callWhenListening(t *testing.T, ctx context.Context, name string, q Request) (Response, error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, e := Call(ctx, name, q)
		if e == nil || time.Now().After(deadline) {
			return response, e
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// acquire is what keeps a second service off a profile. On unix that is flock
// refusing a conflicting LOCK_EX; here it is LockFileEx refusing the same byte
// range. The caller only ever sees the error text, so that is what is pinned.
func TestSecondAcquireIsRefusedWhileTheFirstIsHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.lock")

	first, e := Acquire(path)
	if e != nil {
		t.Fatalf("first acquire: %v", e)
	}

	second, e := Acquire(path)
	if e == nil {
		second.Close()
		first.Close()
		t.Fatal("a second acquire succeeded while the first was held")
	}
	if !strings.Contains(e.Error(), "service already running") {
		t.Fatalf("second acquire reported %q, want it to say the service is already running", e)
	}

	// Releasing has to actually release, or a restart after a clean stop would
	// be refused forever.
	if e = first.Close(); e != nil {
		t.Fatalf("release the first lock: %v", e)
	}
	third, e := Acquire(path)
	if e != nil {
		t.Fatalf("acquire after release: %v", e)
	}
	third.Close()
}

// The security property of the entire local IPC surface. On unix it is a 0600
// socket in a 0700 directory; here it is this descriptor, and nothing else
// restricts the pipe. Assert the shape directly: a present, protected DACL with
// exactly one allow ACE, for this process's own user and nobody else.
func TestPipeDescriptorGrantsTheCurrentUserAlone(t *testing.T) {
	sddl, e := ownerOnlySecurityDescriptor()
	if e != nil {
		t.Fatalf("build the descriptor: %v", e)
	}
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		t.Fatalf("read the current user: %v", e)
	}
	self := user.User.Sid

	sd, e := windows.SecurityDescriptorFromString(sddl)
	if e != nil {
		t.Fatalf("parse %q: %v", sddl, e)
	}
	control, _, e := sd.Control()
	if e != nil {
		t.Fatalf("read the descriptor control bits: %v", e)
	}
	if control&windows.SE_DACL_PRESENT == 0 {
		t.Fatalf("%q has no DACL; an absent DACL grants everyone full control", sddl)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("%q has an unprotected DACL and could inherit foreign ACEs", sddl)
	}

	dacl, _, e := sd.DACL()
	if e != nil {
		t.Fatalf("read the DACL: %v", e)
	}
	if dacl == nil {
		t.Fatalf("%q parsed to a NULL DACL, which grants everyone full control", sddl)
	}
	if dacl.AceCount != 1 {
		t.Fatalf("%q carries %d ACEs, want exactly one for the owning user", sddl, dacl.AceCount)
	}

	var ace *windows.ACCESS_ALLOWED_ACE
	if e = windows.GetAce(dacl, 0, &ace); e != nil {
		t.Fatalf("read the only ACE: %v", e)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatalf("the only ACE has type %d, want an allow ACE", ace.Header.AceType)
	}
	trustee := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !trustee.Equals(self) {
		t.Fatalf("the only ACE grants %s, want this process's user %s", trustee, self)
	}
	// GENERIC_ALL is what the SDDL asks for; anything less would leave the
	// service unable to open further instances of its own pipe.
	if ace.Mask != windows.GENERIC_ALL {
		t.Fatalf("the ACE grants mask %#x, want GENERIC_ALL %#x", ace.Mask, windows.GENERIC_ALL)
	}
}

// dial must fail rather than block when nothing is listening. Call reports a
// dead service by way of this error, and several callers pass a context with no
// deadline of its own, so an unbounded wait here would hang the CLI.
func TestDialFailsPromptlyWhenNoServiceIsListening(t *testing.T) {
	name := endpoint(t)
	start := time.Now()
	c, e := dial(context.Background(), name)
	if e == nil {
		c.Close()
		t.Fatalf("dial reached %s with no service listening", name)
	}
	if elapsed := time.Since(start); elapsed > dialTimeout+2*time.Second {
		t.Fatalf("dial took %s to refuse an absent pipe", elapsed)
	}
}

// A cancelled context must be honoured even when the pipe is right there and
// would otherwise connect. Call layers its own deadline on top of whatever the
// caller passed in, and dial is the half that has to respect the caller's.
func TestDialHonoursACancelledContextAgainstALiveListener(t *testing.T) {
	name := endpoint(t)
	serveCtx, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		_ = Serve(serveCtx, name, func(context.Context, Request) Response { return Response{OK: true} })
	}()
	if _, e := callWhenListening(t, serveCtx, name, Request{Method: "health"}); e != nil {
		t.Fatalf("the listener never came up: %v", e)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, e := dial(ctx, name)
	if e == nil {
		c.Close()
		t.Fatal("dial connected despite a cancelled context")
	}
	if !errors.Is(e, context.Canceled) {
		t.Fatalf("dial refused with %v, want context.Canceled", e)
	}
}
