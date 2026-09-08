//go:build windows

package daemon

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
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

// The pipe this service creates is owned by the user running it, so a client
// that reached the real endpoint must accept it. This is the half of the owner
// check that would break every Windows install if it were wrong, and it runs
// against a live connected handle rather than a synthesised descriptor.
func TestVerifyPipeOwnerAcceptsThePipeThisUserServes(t *testing.T) {
	name := endpoint(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = Serve(ctx, name, func(context.Context, Request) Response { return Response{OK: true} })
	}()
	if _, e := callWhenListening(t, ctx, name, Request{Method: "health"}); e != nil {
		t.Fatalf("the listener never came up: %v", e)
	}

	c, e := dial(ctx, name)
	if e != nil {
		t.Fatalf("dial refused a pipe this user owns: %v", e)
	}
	defer c.Close()
	if e = verifyPipeOwner(c); e != nil {
		t.Fatalf("verifyPipeOwner rejected a pipe this user owns: %v", e)
	}
}

// The rejection the whole change exists for. A pipe owned by anyone else must
// be refused, and refused before the request is written — a client that leaks
// its Request to an impostor has already lost, whatever it does afterwards.
//
// Owning that pipe genuinely requires a second Windows account, so the two
// halves are asserted separately: the SID comparison against a foreign SID
// here, and the ordering with a stubbed verifier in
// TestDialWritesNothingToAPipeItRejects.
func TestOwnerIsCurrentUserRejectsAForeignSID(t *testing.T) {
	// Local System: a real, well-known principal that this process is
	// guaranteed not to be running as when the tests run as a user.
	foreign, e := windows.StringToSid("S-1-5-18")
	if e != nil {
		t.Fatalf("parse the foreign SID: %v", e)
	}
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		t.Fatalf("read the current user: %v", e)
	}
	if user.User.Sid.Equals(foreign) {
		t.Skip("these tests are running as Local System, which is the SID they use as the foreign one")
	}

	e = ownerIsCurrentUser(foreign)
	if e == nil {
		t.Fatal("ownerIsCurrentUser accepted a pipe owned by Local System")
	}
	if !strings.Contains(e.Error(), foreign.String()) {
		t.Fatalf("the rejection %q does not name the offending owner %s", e, foreign)
	}

	// The same function must accept this process's own user, or the check
	// above would be passing for the wrong reason.
	if e = ownerIsCurrentUser(user.User.Sid); e != nil {
		t.Fatalf("ownerIsCurrentUser rejected this process's own user: %v", e)
	}
	// An unreadable owner is a failure, not a pass.
	if ownerIsCurrentUser(nil) == nil {
		t.Fatal("ownerIsCurrentUser accepted a pipe with no owner SID")
	}
}

// A connection that cannot be checked is a connection that gets closed. If
// winio ever stops exposing the kernel handle, this must fail closed rather
// than quietly skip verification and reopen the hole.
func TestVerifyPipeOwnerRejectsAConnectionWithNoHandle(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	e := verifyPipeOwner(local)
	if e == nil {
		t.Fatal("verifyPipeOwner accepted a connection that exposes no handle")
	}
	if !strings.Contains(e.Error(), "no handle") {
		t.Fatalf("the rejection %q does not say the handle was missing", e)
	}
}

// The ordering property, stated as a test: when verification fails, the caller
// gets no connection, the connection it would have got is closed, and the
// server read nothing. dial is the only door into the pipe and Call writes only
// after dial returns, so nothing written here means nothing written anywhere.
func TestDialWritesNothingToAPipeItRejects(t *testing.T) {
	name := endpoint(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Accept by hand rather than through Serve, so the bytes the server side
	// saw can be counted instead of inferred from a handler that never ran.
	sd, e := ownerOnlySecurityDescriptor()
	if e != nil {
		t.Fatalf("build the descriptor: %v", e)
	}
	ln, e := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: sd})
	if e != nil {
		t.Fatalf("listen on %s: %v", name, e)
	}
	defer ln.Close()

	read := make(chan int, 1)
	go func() {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		var buf [64]byte
		n, _ := c.Read(buf[:])
		read <- n
	}()

	refused := errors.New("owner mismatch")
	var checked net.Conn
	c, e := dialVerified(ctx, name, func(conn net.Conn) error {
		checked = conn
		return refused
	})
	if e == nil {
		c.Close()
		t.Fatal("dialVerified returned a connection its verifier refused")
	}
	if c != nil {
		t.Fatalf("dialVerified returned a non-nil connection (%T) alongside its error", c)
	}
	if !errors.Is(e, refused) {
		t.Fatalf("dialVerified reported %v, want the verifier's own error", e)
	}
	if checked == nil {
		t.Fatal("dialVerified never ran the verifier")
	}
	// The refused connection must be closed, not merely dropped; a live handle
	// to an attacker's pipe is still a handle to an attacker's pipe.
	if _, e = checked.Write([]byte("x")); e == nil {
		t.Fatal("the refused connection was still writable, so dialVerified left it open")
	}

	select {
	case n := <-read:
		if n != 0 {
			t.Fatalf("the server side read %d bytes from a connection dial refused, want none", n)
		}
	case <-time.After(3 * time.Second):
		// The read deadline expired with nothing to read, which is the same
		// verdict: no request reached the far end.
	}
}
