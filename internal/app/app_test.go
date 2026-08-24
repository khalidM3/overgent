//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	gitobs "github.com/stickguy/stickguy/internal/git"
	"github.com/stickguy/stickguy/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSender struct {
	mu      sync.Mutex
	events  int
	batches map[string][][]byte
}

func TestCommittedThousandPathManifestIsAtomicAndRestartSafe(t *testing.T) {
	state := t.TempDir()
	repo := makeRepo(t)
	ctx := context.Background()
	if e := Register(ctx, state, "https://api.stickguy.dev", "dev_fixture", config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: repo}); e != nil {
		t.Fatal(e)
	}
	for i := range 1000 {
		p := filepath.Join(repo, "bulk", fmt.Sprintf("%04d.txt", i))
		if e := os.MkdirAll(filepath.Dir(p), 0o700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte("fixture"), 0o600); e != nil {
			t.Fatal(e)
		}
	}
	gitcmd(t, repo, "add", ".")
	gitcmd(t, repo, "commit", "-q", "-m", "bulk")
	paths, _ := config.Resolve(state)
	db, e := store.Open(paths.DB)
	if e != nil {
		t.Fatal(e)
	}
	cfg, _ := config.Load(paths)
	w := cfg.Workspaces[0]
	if len(w.Fingerprint) != 64 {
		t.Fatalf("repository fingerprint=%q", w.Fingerprint)
	}
	if e = db.UpsertWorkspace(ctx, store.Workspace{ID: w.ID, ProjectID: w.ProjectID, WorkstreamID: w.WorkstreamID, Root: w.Root, Baseline: w.Baseline, Fingerprint: w.Fingerprint}); e != nil {
		t.Fatal(e)
	}
	s := &Service{store: db, cfg: cfg}
	s.scanAll(ctx)
	rev, _, raw, e := db.ActiveManifest(ctx, "wsp_fixture")
	if e != nil {
		t.Fatal(e)
	}
	var entries []gitobs.Entry
	if e = json.Unmarshal(raw, &entries); e != nil {
		t.Fatal(e)
	}
	if rev != 1 || len(entries) != 1000 {
		t.Fatalf("revision=%d paths=%d", rev, len(entries))
	}
	pending, _ := db.Pending(ctx)
	if len(pending) != 13 {
		t.Fatalf("atomic queue events=%d", len(pending))
	}
	if pending[0].Kind != "workspace.registered" || pending[1].Kind != "workspace.manifest_started" || pending[len(pending)-1].Kind != "workspace.manifest_completed" {
		t.Fatalf("publication ordering: first=%s last=%s", pending[0].Kind, pending[len(pending)-1].Kind)
	}
	for i, event := range pending[2 : len(pending)-1] {
		if event.Kind != "workspace.manifest_chunk" || event.Sequence != int64(i+3) {
			t.Fatalf("chunk %d: %#v", i, event)
		}
	}
	db.Close()
	db, e = store.Open(paths.DB)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	rev, _, raw, e = db.ActiveManifest(ctx, "wsp_fixture")
	if e != nil || rev != 1 {
		t.Fatal(rev, e)
	}
	entries = nil
	_ = json.Unmarshal(raw, &entries)
	if len(entries) != 1000 {
		t.Fatalf("restart paths=%d", len(entries))
	}
}

func (f *fakeSender) Send(_ context.Context, workspaceID string, batch []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body struct {
		Events []json.RawMessage `json:"events"`
	}
	if json.Unmarshal(batch, &body) != nil {
		return fmt.Errorf("invalid batch")
	}
	f.events += len(body.Events)
	if f.batches == nil {
		f.batches = map[string][][]byte{}
	}
	f.batches[workspaceID] = append(f.batches[workspaceID], append([]byte(nil), batch...))
	return nil
}
func (f *fakeSender) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.events }
func (f *fakeSender) workspaceBatches(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches[id])
}
func TestTwoRepositoriesLockPauseRestart(t *testing.T) {
	state, e := os.MkdirTemp("/private/tmp", "sg-l1-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(state) })
	r1 := makeRepo(t)
	r2 := makeRepo(t)
	ctx := context.Background()
	for i, r := range []string{r1, r2} {
		id := string(rune('a' + i))
		if e := Register(ctx, state, "https://api.stickguy.dev", "dev_fixture", config.Workspace{ID: "wsp_" + id, ProjectID: "prj_fixture", WorkstreamID: "wrk_" + id, MemberID: "mem_fixture", SessionID: "ses_" + id, Root: r}); e != nil {
			t.Fatal(e)
		}
	}
	send := &fakeSender{}
	cancel, done := start(t, state, send)
	paths, _ := config.Resolve(state)
	waitHealth(t, paths.Socket, done)
	if st, e := os.Stat(paths.Socket); e != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("socket permissions: %v %v", st, e)
	}
	for _, p := range []string{paths.Config, paths.DB, paths.Lock} {
		if st, e := os.Stat(p); e != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("state file permissions for %s: %v %v", filepath.Base(p), st, e)
		}
	}
	if st, e := os.Stat(paths.Root); e != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("state root permissions: %v %v", st, e)
	}
	if e := Run(context.Background(), state, nil); e == nil || !strings.Contains(e.Error(), "already running") {
		t.Fatalf("second instance: %v", e)
	}
	if e := Register(ctx, state, "https://api.stickguy.dev", "dev_fixture", config.Workspace{ID: "wsp_c", ProjectID: "prj_fixture", WorkstreamID: "wrk_c", MemberID: "mem_fixture", SessionID: "ses_c", Root: makeRepo(t)}); e == nil || !strings.Contains(e.Error(), "already running") {
		t.Fatalf("concurrent registration: %v", e)
	}
	wait(t, func() bool { return send.workspaceBatches("wsp_a") > 0 && send.workspaceBatches("wsp_b") > 0 })
	initial := send.count()
	_, e = daemon.Call(ctx, paths.Socket, daemon.Request{Method: "pause", WorkspaceID: "wsp_a"})
	if e != nil {
		t.Fatal(e)
	}
	intentResponse, e := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "intent", WorkspaceID: "wsp_a", Title: "Synthetic intent", IntendedOutcome: "Prove paused intent remains durable"})
	if e != nil || !intentResponse.OK {
		t.Fatal(intentResponse, e)
	}
	writeFile(t, r1, "paused.txt")
	_, _ = daemon.Call(ctx, paths.Socket, daemon.Request{Method: "scan"})
	time.Sleep(700 * time.Millisecond)
	if send.count() != initial {
		t.Fatalf("pause sent payload: %d -> %d", initial, send.count())
	}
	writeFile(t, r2, "live.txt")
	_, _ = daemon.Call(ctx, paths.Socket, daemon.Request{Method: "scan"})
	wait(t, func() bool { return send.count() > initial })
	cancel()
	<-done
	cancel2, done2 := start(t, state, nil)
	waitHealth(t, paths.Socket, done2)
	resp, e := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "doctor"})
	if e != nil || !resp.OK {
		t.Fatal(resp, e)
	}
	cancel2()
	<-done2
}

func TestRetryDelayIsBoundedAndJittered(t *testing.T) {
	if got := retryDelay(0); got != 500*time.Millisecond {
		t.Fatalf("initial retry delay=%s", got)
	}
	for range 100 {
		if got := retryDelay(20); got < 24*time.Second || got > 36*time.Second {
			t.Fatalf("capped retry delay=%s", got)
		}
	}
}
func start(t *testing.T, state string, s Sender) (context.CancelFunc, chan error) {
	t.Helper()
	ctx, c := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, state, s) }()
	return c, done
}
func waitHealth(t *testing.T, s string, done chan error) {
	wait(t, func() bool {
		select {
		case e := <-done:
			t.Fatalf("service exited: %v", e)
		default:
		}
		r, e := daemon.Call(context.Background(), s, daemon.Request{Method: "health"})
		return e == nil && r.OK
	})
}
func wait(t *testing.T, f func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if f() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timeout")
}
func makeRepo(t *testing.T) string {
	t.Helper()
	r := t.TempDir()
	gitcmd(t, r, "init", "-q")
	gitcmd(t, r, "config", "user.email", "fixture@example.invalid")
	gitcmd(t, r, "config", "user.name", "Fixture")
	gitcmd(t, r, "remote", "add", "origin", fmt.Sprintf("https://example.invalid/fixture/repo-%d.git", repoSerial.Add(1)))
	writeFile(t, r, "base.txt")
	gitcmd(t, r, "add", ".")
	gitcmd(t, r, "commit", "-q", "-m", "base")
	return r
}

var repoSerial atomic.Uint64

func TestRegisterRejectsExternalIdentifiersAndRoot(t *testing.T) {
	state := t.TempDir()
	valid := config.Workspace{ID: "wsp_valid", ProjectID: "prj_valid", WorkstreamID: "wrk_valid", MemberID: "mem_valid", SessionID: "ses_valid", Root: makeRepo(t)}
	for name, mutate := range map[string]func(*config.Workspace){
		"workspace":  func(w *config.Workspace) { w.ID = "../bad" },
		"Project":    func(w *config.Workspace) { w.ProjectID = "bad" },
		"workstream": func(w *config.Workspace) { w.WorkstreamID = "bad" },
		"root":       func(w *config.Workspace) { w.Root = filepath.Join(t.TempDir(), "missing") },
	} {
		t.Run(name, func(t *testing.T) {
			w := valid
			mutate(&w)
			if e := Register(context.Background(), state, "https://api.stickguy.dev", "dev_valid", w); e == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
	if e := Register(context.Background(), state, "https://api.stickguy.dev", "BAD", valid); e == nil {
		t.Fatal("invalid device ID accepted")
	}
}
func writeFile(t *testing.T, r, p string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(r, p), []byte("fixture"), 0o600); e != nil {
		t.Fatal(e)
	}
}
func gitcmd(t *testing.T, r string, a ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", r}, a...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("git: %v %s", e, b)
	}
}
